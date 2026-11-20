import { render } from "../runtime/render";
import type { Compiled, Model, State, Value } from "../runtime/types";
import { qtyString } from "../runtime/units";
import {
  esc,
  fr,
  recipeURL,
  renderEquipment,
  renderIngredients,
  renderNotes,
  renderSteps,
  renderTimes,
} from "./blocks";
import { conversions } from "./convert";
import { download, type Meta, summary, toMarkdown } from "./export";
import { asciiDigits, num, t } from "./i18n";
import { addRecipe, inList, itemsFrom } from "./list";
import { openSheet } from "./sheet";
import { load, save } from "./storage";
import { pickTimer, startTimer } from "./timers";
import { toast } from "./toast";
import { keepAwake } from "./wakelock";

interface PageData extends Meta {
  slug: string;
  compiled: Compiled;
}

/** Parses "3", "1 1/2", "1½", "0,5" or "3/4". */
export function parseAmount(s: string): number {
  s = asciiDigits(s);
  const vulgar: Record<string, number> = { "½": 0.5, "⅓": 1 / 3, "⅔": 2 / 3, "¼": 0.25, "¾": 0.75, "⅛": 0.125 };
  let total = 0;
  for (const part of s.trim().replace(",", ".").split(/\s+/)) {
    const m = part.match(/^(\d*\.?\d*)([½⅓⅔¼¾⅛])?$/);
    const f = part.match(/^(\d+)\/(\d+)$/);
    if (f) total += Number(f[1]) / Number(f[2]);
    else if (m && (m[1] || m[2])) total += (m[1] ? Number(m[1]) : 0) + (m[2] ? (vulgar[m[2]] ?? 0) : 0);
    else return Number.NaN;
  }
  return total;
}

const $ = <T extends HTMLElement>(id: string) => document.getElementById(id) as T | null;

export function initRecipe(): void {
  const dataEl = $("recipe-data");
  if (!dataEl?.textContent) return;
  const data = JSON.parse(dataEl.textContent) as PageData;
  const c = data.compiled;
  const base = document.documentElement.dataset.base ?? "/";
  const keyState = `gc:r:${data.slug}`;
  const keyChecked = `gc:c:${data.slug}`;
  const keyDone = `gc:d:${data.slug}`;

  const defaults = (): State => ({
    servings: c.servings.default,
    options: Object.fromEntries((c.options ?? []).map((o) => [o.id, o.default])),
    system: c.system,
  });

  // State precedence: defaults < remembered < URL.
  const state: State = defaults();
  const remembered = load<State | null>(keyState, null);
  if (remembered) {
    if (remembered.servings) state.servings = remembered.servings;
    Object.assign(state.options as object, remembered.options ?? {});
  }
  state.system = load<string>("gc:units", c.system);
  const params = new URLSearchParams(location.search);
  const fromURL = [...params.keys()].some((k) => k === "s" || k === "u" || k.startsWith("o."));
  if (params.has("s")) state.servings = Number(params.get("s")) || state.servings;
  if (params.has("u")) state.system = params.get("u") ?? state.system;
  for (const o of c.options ?? []) {
    const raw = params.get(`o.${o.id}`);
    if (raw === null) continue;
    const opts = state.options as Record<string, Value>;
    if (o.type === "toggle") opts[o.id] = raw === "1" || raw === "true";
    else if (o.type === "number") opts[o.id] = Number(raw);
    else opts[o.id] = raw;
  }

  // Servings and options that came in a link (another recipe's ingredient,
  // a shared link) apply to this visit only: they don't replace what the
  // reader set for this recipe until they change something here.
  let persist = !fromURL;
  let linked: string | null = null;

  let model: Model | null = null;
  let last: State = structuredClone(state);
  const checked = new Set(load<string[]>(keyChecked, []));
  const done = new Set(load<number[]>(keyDone, []));
  let cooking = false;
  let current = 0;

  const isDefault = (): boolean => {
    const d = defaults();
    if (state.servings !== d.servings) return false;
    for (const [k, v] of Object.entries(d.options ?? {})) if (state.options?.[k] !== v) return false;
    return true;
  };

  function syncURL(): void {
    const u = isDefault() && state.system === c.system ? `${location.pathname}` : shareURL(false);
    history.replaceState(null, "", u + location.hash);
  }

  function shareURL(absolute: boolean): string {
    const changed: Record<string, Value> = {};
    const d = defaults();
    for (const [k, v] of Object.entries(state.options ?? {})) if (d.options?.[k] !== v) changed[k] = v;
    let u = recipeURL(base, data.slug, state.servings !== d.servings ? (state.servings ?? 0) : 0, changed);
    if (state.system !== c.system) u += `${u.includes("?") ? "&" : "?"}u=${state.system}`;
    return absolute ? new URL(u, location.href).href : u;
  }

  function paint(): void {
    let m: Model;
    try {
      m = render(c, state);
    } catch (err) {
      console.error(err);
      toast(t("unsupported"));
      Object.assign(state, structuredClone(last));
      return;
    }
    model = m;
    last = structuredClone(state);
    state.servings = m.servings;
    const set = (id: string, html: string) => {
      const el = $(id);
      if (el && el.innerHTML !== html) el.innerHTML = html;
    };
    set("times", fr(renderTimes(m)));
    const y = $("yield");
    if (y) y.textContent = m.yield;
    set("ingredients", fr(renderIngredients(m, base)));
    set("equipment", fr(renderEquipment(m)));
    const ew = $("equipment-wrap");
    if (ew) ew.hidden = m.equipment.length === 0;
    set("print-equipment", fr(renderEquipment(m)));
    const pe = $("print-equip");
    if (pe) pe.hidden = m.equipment.length === 0;
    set("steps", fr(renderSteps(m, base)));
    set("warnings", fr(renderNotes(m, true)));
    set("tips", fr(renderNotes(m, false)));
    paintImage(m);
    for (const el of document.querySelectorAll<HTMLElement>("[data-opt]")) {
      el.hidden = !m.options.includes(el.dataset.opt ?? "");
    }
    paintChecked();
    paintSteps();
    syncSheet();
    paintMeta();
    const badge = $("changes");
    if (badge) badge.hidden = isDefault() && state.system === c.system;
    const snap = JSON.stringify([state.servings, state.options]);
    if (!persist) {
      if (linked === null) linked = snap;
      else if (snap !== linked) persist = true;
    }
    if (persist) save(keyState, isDefault() ? undefined : { servings: state.servings, options: state.options });
    syncURL();
  }

  function paintSteps(): void {
    const steps = [...document.querySelectorAll<HTMLElement>(".step")];
    if (cooking && (current < 1 || current > steps.length + 1)) {
      const next = steps.find((s) => !done.has(Number(s.dataset.n)));
      current = next ? Number(next.dataset.n) : 1;
    }
    for (const s of steps) {
      const n = Number(s.dataset.n);
      s.classList.toggle("done", done.has(n));
      s.classList.toggle("current", cooking && n === current);
    }
    paintProgress();
    const of = $("step-of");
    if (of)
      of.textContent =
        current > steps.length ? t("all_done") : t("step_of", { n: num(current), total: num(steps.length) });
  }

  // Ticked ingredients, in the list and wherever a step mentions them.
  function paintChecked(): void {
    for (const box of document.querySelectorAll<HTMLInputElement>(".ing-check")) {
      box.checked = checked.has(box.dataset.id ?? "");
    }
    for (const ref of document.querySelectorAll<HTMLElement>(".ref[data-ing]")) {
      const got = checked.has(ref.dataset.ing ?? "");
      ref.classList.toggle("got", got);
      // A toggle once scripts run: tap (or Enter/Space) to tick it off.
      ref.setAttribute("role", "button");
      ref.tabIndex = 0;
      ref.setAttribute("aria-pressed", String(got));
    }
  }

  function toggleChecked(id: string): void {
    if (checked.has(id)) checked.delete(id);
    else checked.add(id);
    save(keyChecked, checked.size ? [...checked] : undefined);
    paintChecked();
    paintProgress();
  }

  /** The compiled ingredient behind a rendered one (variants share an id). */
  function compiledIng(id: string, name: string) {
    const all = c.ingredients.flatMap((s) => s.items);
    const ic = all.find((x) => x.id === id && (x.name === name || x.plural === name)) ?? all.find((x) => x.id === id);
    return { ic, info: ic?.ref ? c.ings?.[ic.ref] : undefined };
  }

  // Tapping an amount: the same amount in other units, and scaling the
  // recipe to how much the reader has.
  function openConvert(id: string): void {
    const it = model?.ingredients.flatMap((s) => s.items).find((x) => x.id === id);
    if (!it?.q || it.q.v <= 0) return;
    const { info } = compiledIng(it.id, it.name);
    const rows = conversions(it.q, info, c.units);
    const label = (sys: string, dim: string) =>
      `${t(`units_${sys}`)}${dim === "mass" ? ` · ${t("by_weight")}` : dim === "volume" ? ` · ${t("by_volume")}` : ""}`;
    const body = `<p class="convert-now"><span class="qty">${esc(fr(qtyString(it.q)))}</span>${esc(it.j)}${esc(it.name)}</p>
  ${rows.length ? `<h3 class="opt-label">${esc(t("convert"))}</h3><dl class="convert-list">${rows.map((r) => `<div><dt>${esc(label(r.sys, r.dim))}</dt><dd>${fr(esc(r.text))}</dd></div>`).join("")}</dl>` : ""}
  <h3 class="opt-label">${esc(t("scale_to"))}</h3>
  <div class="convert-scale"><input type="text" inputmode="decimal" id="scale-have" class="field" aria-label="${esc(t("scale_amount", { name: it.name }))}" placeholder="${esc(qtyString(it.q))}" autocomplete="off">
    <button type="button" class="pill" id="scale-go">${esc(t("scale_go"))}</button></div>`;
    const d = openSheet("convert-sheet", it.name, body);
    const input = d.querySelector<HTMLInputElement>("#scale-have") as HTMLInputElement;
    const go = () => {
      const want = parseAmount(input.value);
      if (!(want > 0) || !it.q) return;
      const s = state.servings ?? c.servings.default;
      state.servings = Math.round(((s * want) / it.q.v) * 100) / 100;
      d.close();
      paint();
    };
    d.querySelector("#scale-go")?.addEventListener("click", go);
    input.addEventListener("keydown", (e) => {
      if (e.key === "Enter") {
        e.preventDefault();
        go();
      }
    });
  }

  // The "Swap" button on an ingredient with substitutes.
  function openSwap(optID: string): void {
    const o = c.options?.find((x) => x.id === optID);
    if (!o?.choices) return;
    const v = state.options?.[o.id] ?? o.default;
    const body = `<div class="swap-list" role="radiogroup">${o.choices
      .map(
        (ch) =>
          `<label class="swap-choice"><input type="radio" name="swap-pick" value="${esc(ch.id)}"${ch.id === v ? " checked" : ""}><span>${esc(ch.label)}</span></label>`,
      )
      .join("")}</div>`;
    const d = openSheet("swap-sheet", o.label, body);
    d.querySelector(".swap-list")?.addEventListener("change", (e) => {
      (state.options as Record<string, Value>)[o.id] = (e.target as HTMLInputElement).value;
      d.close();
      paint();
    });
  }

  // "Start over": clear ticked ingredients and finished steps.
  function paintProgress(): void {
    const btn = $("start-over");
    if (btn) btn.hidden = checked.size === 0 && done.size === 0;
  }

  function goStep(delta: number): void {
    const total = document.querySelectorAll(".step").length;
    if (delta > 0 && current <= total) done.add(current);
    if (delta < 0) done.delete(current - 1);
    current = Math.min(total + 1, Math.max(1, current + delta));
    save(keyDone, done.size ? [...done] : undefined);
    paintSteps();
    document.querySelector(".step.current")?.scrollIntoView({ behavior: "smooth", block: "center" });
  }

  // Swap the hero image when an image variant matches the reader's choices.
  function paintImage(m: Model): void {
    const hero = $<HTMLImageElement>("hero");
    if (!hero) return;
    const d = hero.dataset;
    const src = m.image?.src || d.defaultSrc || "";
    const srcset = m.image ? m.image.srcset : d.defaultSrcset || "";
    if (hero.getAttribute("src") !== src) {
      if (srcset) hero.setAttribute("srcset", srcset);
      else hero.removeAttribute("srcset");
      if (src) hero.setAttribute("src", src);
      else hero.removeAttribute("src");
    }
    hero.alt = m.image?.alt || d.defaultAlt || "";
    hero.hidden = !src;
  }

  function paintMeta(): void {
    const el = $("print-meta");
    if (el && model) el.textContent = [model.yield, summary(c, model, state.options ?? {})].filter(Boolean).join(" · ");
    const add = $("add-to-list")?.querySelector("span");
    if (add) add.textContent = inList(data.slug) ? t("list_update") : t("list_add");
  }

  function syncSheet(): void {
    for (const input of document.querySelectorAll<HTMLInputElement>("#sheet input[name]")) {
      if (input.name === "__units") {
        input.checked = input.value === state.system;
        continue;
      }
      const v = state.options?.[input.name];
      if (input.type === "checkbox") input.checked = v === true;
      else if (input.type === "radio") input.checked = input.value === v;
      else if (input.type === "range") {
        input.value = String(v);
        const out = document.querySelector(`output[data-for="${input.name}"]`);
        if (out) out.textContent = num(String(v));
      }
    }
  }

  function changeServings(dir: number): void {
    const { step, min, max } = c.servings;
    const s = state.servings ?? c.servings.default;
    const snapped = dir > 0 ? Math.floor(s / step + 1e-9) * step + step : Math.ceil(s / step - 1e-9) * step - step;
    state.servings = Math.min(max, Math.max(min, snapped));
    paint();
  }

  async function setCooking(on: boolean): Promise<void> {
    cooking = on;
    document.body.classList.toggle("cooking", on);
    $("cook-mode")?.setAttribute("aria-pressed", String(on));
    current = 0;
    paintSteps();
    if (on) {
      const ok = await keepAwake(true);
      toast(ok ? t("cook_on") : t("cook_on_unsupported"));
      document.querySelector(".step.current")?.scrollIntoView({ behavior: "smooth", block: "center" });
    } else {
      await keepAwake(false);
    }
  }

  // ── Events ──────────────────────────────────────────────
  document.addEventListener("click", (e) => {
    const el = e.target as Element;
    const sv = el.closest<HTMLElement>("[data-servings]");
    if (sv) return changeServings(Number(sv.dataset.servings));
    const chip = el.closest<HTMLElement>(".timer-chip");
    if (chip) {
      // The step first: it's what gets cut off on a narrow screen otherwise.
      const label = `${chip.dataset.label ?? ""} · ${data.title}`;
      const from = {
        url: location.pathname + location.search,
        step: Number(chip.closest<HTMLElement>(".step")?.dataset.n) || 0,
      };
      if (chip.dataset.max) return pickTimer(Number(chip.dataset.min), Number(chip.dataset.max), label, from);
      return startTimer(Number(chip.dataset.min), label, from);
    }
    const qty = el.closest<HTMLElement>(".ing .qty");
    if (qty && model) {
      e.preventDefault();
      return openConvert(qty.closest<HTMLElement>(".ing")?.dataset.id ?? "");
    }
    const swap = el.closest<HTMLElement>(".swap-btn");
    if (swap) return openSwap(swap.dataset.swap ?? "");
    // Tapping an ingredient in a step ticks it off.
    const ref = el.closest<HTMLElement>(".step .ref[data-ing]");
    if (ref) return toggleChecked(ref.dataset.ing ?? "");
    const num = el.closest<HTMLElement>(".step-num");
    const step = el.closest<HTMLElement>(".step");
    if (step && !el.closest("a, button:not(.step-num)")) {
      const n = Number(step.dataset.n);
      if (num || !cooking) {
        if (done.has(n)) done.delete(n);
        else done.add(n);
        if (cooking && done.has(n) && n === current) current = n + 1;
      } else {
        current = n;
      }
      save(keyDone, done.size ? [...done] : undefined);
      paintSteps();
    }
  });

  document.addEventListener("change", (e) => {
    const t = e.target as HTMLInputElement;
    if (t.classList.contains("ing-check")) {
      const id = t.dataset.id ?? "";
      if (t.checked !== checked.has(id)) toggleChecked(id);
    }
  });

  const sheet = $<HTMLDialogElement>("sheet");
  $("customize")?.addEventListener("click", () => sheet?.showModal());
  sheet?.addEventListener("click", (e) => {
    if (e.target === sheet) sheet.close();
  });
  sheet?.addEventListener("input", (e) => {
    const t = e.target as HTMLInputElement;
    if (!t.name) return;
    if (t.name === "__units") {
      state.system = t.value;
      save("gc:units", t.value === c.system ? undefined : t.value);
    } else {
      const opts = state.options as Record<string, Value>;
      if (t.type === "checkbox") opts[t.name] = t.checked;
      else if (t.type === "range") opts[t.name] = Number(t.value);
      else opts[t.name] = t.value;
    }
    paint();
  });
  $("reset")?.addEventListener("click", () => {
    Object.assign(state, defaults());
    save("gc:units", undefined);
    paint();
  });
  $("cook-mode")?.addEventListener("click", () => void setCooking(!cooking));
  $("start-over")?.addEventListener("click", () => {
    checked.clear();
    done.clear();
    save(keyChecked, undefined);
    save(keyDone, undefined);
    paintChecked();
    current = cooking ? 1 : 0;
    paintSteps();
    toast(t("progress_cleared"));
    if (cooking) document.querySelector(".step.current")?.scrollIntoView({ behavior: "smooth", block: "center" });
  });
  // Swipe left/right on the steps in cook mode to move between steps.
  let touch: { x: number; y: number } | null = null;
  $("steps")?.addEventListener(
    "touchstart",
    (e) => {
      const p = e.touches[0];
      touch = cooking && p ? { x: p.clientX, y: p.clientY } : null;
    },
    { passive: true },
  );
  $("steps")?.addEventListener(
    "touchend",
    (e) => {
      const p = e.changedTouches[0];
      if (!touch || !p) return;
      const dx = p.clientX - touch.x;
      const dy = p.clientY - touch.y;
      touch = null;
      if (Math.abs(dx) > 60 && Math.abs(dy) < 40) goStep(dx < 0 ? 1 : -1);
    },
    { passive: true },
  );
  $("exit-cook")?.addEventListener("click", () => void setCooking(false));
  $("next-step")?.addEventListener("click", () => goStep(1));
  $("prev-step")?.addEventListener("click", () => goStep(-1));

  // Saved recipes (shown as a filter on the index).
  const saveBtn = $("save");
  const paintSaved = () =>
    saveBtn?.setAttribute("aria-pressed", String(load<string[]>("gc:saved", []).includes(data.slug)));
  saveBtn?.addEventListener("click", () => {
    const saved = new Set(load<string[]>("gc:saved", []));
    if (saved.has(data.slug)) saved.delete(data.slug);
    else saved.add(data.slug);
    save("gc:saved", saved.size ? [...saved] : undefined);
    paintSaved();
  });
  paintSaved();

  $("add-to-list")?.addEventListener("click", () => {
    if (!model) return;
    addRecipe({
      slug: data.slug,
      title: data.title,
      url: shareURL(false),
      yield: model.yield,
      items: itemsFrom(c, model, (l) => recipeURL(base, l.slug, l.servings, l.options)),
    });
    paintMeta();
  });

  // Export: print/PDF, Markdown download, plain text.
  const menu = $<HTMLDetailsElement>("export-menu");
  menu?.addEventListener("click", async (e) => {
    const kind = (e.target as Element).closest<HTMLElement>("[data-export]")?.dataset.export;
    if (!kind || !model) return;
    menu.open = false;
    const md = toMarkdown(data, c, model, state.options ?? {});
    if (kind === "print") print();
    else if (kind === "md") download(`${data.slug}.md`, md);
    else {
      try {
        await navigator.clipboard.writeText(md.replace(/^#+ |\*\*|\*|^> /gm, ""));
        toast(t("copied"));
      } catch {
        toast(t("copy_failed"));
      }
    }
  });
  document.addEventListener("click", (e) => {
    if (menu?.open && !menu.contains(e.target as Node)) menu.open = false;
  });

  // Personal notes, kept on this device.
  const notes = $<HTMLTextAreaElement>("my-notes-text");
  if (notes) {
    notes.value = load<string>(`gc:n:${data.slug}`, "");
    if (notes.value) ($("my-notes") as HTMLDetailsElement).open = true;
    notes.addEventListener("input", () => save(`gc:n:${data.slug}`, notes.value.trim() ? notes.value : undefined));
  }
  $("share")?.addEventListener("click", async () => {
    const url = shareURL(true);
    try {
      if (navigator.share) await navigator.share({ title: data.title, url });
      else {
        await navigator.clipboard.writeText(url);
        toast(t("link_copied"));
      }
    } catch {
      // cancelled
    }
  });
  document.addEventListener("keydown", (e) => {
    const t = e.target as HTMLElement;
    if (t.matches?.(".step .ref[data-ing]") && (e.key === "Enter" || e.key === " ")) {
      e.preventDefault();
      return toggleChecked(t.dataset.ing ?? "");
    }
    if (e.metaKey || e.ctrlKey || e.altKey || t.matches("input, textarea, select") || sheet?.open) return;
    if (cooking && (e.key === "ArrowRight" || e.key === " ")) {
      e.preventDefault();
      goStep(1);
    } else if (cooking && e.key === "ArrowLeft") goStep(-1);
    else if (cooking && e.key === "Escape") void setCooking(false);
    else if (e.key === "+" || e.key === "=") changeServings(1);
    else if (e.key === "-") changeServings(-1);
    else if (e.key === "c") void setCooking(!cooking);
  });

  if (fromURL || remembered || state.system !== c.system) paint();
  else {
    // Server-rendered markup already matches the default state.
    model = render(c, state);
    paintChecked();
    paintSteps();
    syncSheet();
    paintMeta();
  }
  // Opened from a timer: show the step it was started from.
  const fromStep = location.hash.match(/^#step-(\d+)$/);
  if (fromStep) document.querySelector(`.step[data-n="${fromStep[1]}"]`)?.scrollIntoView({ block: "center" });
}
