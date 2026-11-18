import { render } from "../runtime/render";
import type { Compiled, Model, State, Value } from "../runtime/types";
import { qtyString } from "../runtime/units";
import { recipeURL, renderEquipment, renderIngredients, renderNotes, renderSteps, renderTimes } from "./blocks";
import { t } from "./i18n";
import { load, save } from "./storage";
import { startTimer } from "./timers";
import { toast } from "./toast";
import { keepAwake } from "./wakelock";

interface PageData {
  slug: string;
  title: string;
  compiled: Compiled;
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
    set("times", renderTimes(m));
    const y = $("yield");
    if (y) y.textContent = m.yield;
    set("ingredients", renderIngredients(m, base));
    set("equipment", renderEquipment(m));
    const ew = $("equipment-wrap");
    if (ew) ew.hidden = m.equipment.length === 0;
    set("steps", renderSteps(m, base));
    set("warnings", renderNotes(m, true));
    set("tips", renderNotes(m, false));
    for (const el of document.querySelectorAll<HTMLElement>("[data-opt]")) {
      el.hidden = !m.options.includes(el.dataset.opt ?? "");
    }
    for (const box of document.querySelectorAll<HTMLInputElement>(".ing-check")) {
      box.checked = checked.has(box.dataset.id ?? "");
    }
    paintSteps();
    syncSheet();
    const badge = $("changes");
    if (badge) badge.hidden = isDefault() && state.system === c.system;
    save(keyState, isDefault() ? undefined : { servings: state.servings, options: state.options });
    syncURL();
  }

  function paintSteps(): void {
    const steps = [...document.querySelectorAll<HTMLElement>(".step")];
    if (cooking && (current < 1 || current > steps.length)) {
      current = steps.find((s) => !done.has(Number(s.dataset.n)))?.dataset.n
        ? Number(steps.find((s) => !done.has(Number(s.dataset.n)))?.dataset.n)
        : 1;
    }
    for (const s of steps) {
      const n = Number(s.dataset.n);
      s.classList.toggle("done", done.has(n));
      s.classList.toggle("current", cooking && n === current);
    }
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
        if (out) out.textContent = String(v);
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
    const t = e.target as Element;
    const sv = t.closest<HTMLElement>("[data-servings]");
    if (sv) return changeServings(Number(sv.dataset.servings));
    const chip = t.closest<HTMLElement>(".timer-chip");
    if (chip) {
      const label = `${data.title} · ${chip.dataset.label ?? ""}`;
      return startTimer(Number(chip.dataset.min), label);
    }
    const num = t.closest<HTMLElement>(".step-num");
    const step = t.closest<HTMLElement>(".step");
    if (step && !t.closest("a, button:not(.step-num)")) {
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
      if (t.checked) checked.add(id);
      else checked.delete(id);
      save(keyChecked, checked.size ? [...checked] : undefined);
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
  $("print")?.addEventListener("click", () => print());
  $("copy-ingredients")?.addEventListener("click", async () => {
    if (!model) return;
    const lines = [`${data.title} (${model.yield})`];
    for (const s of model.ingredients) {
      if (s.title) lines.push("", s.title);
      for (const it of s.items)
        lines.push(`- ${it.q ? qtyString(it.q) + it.j : ""}${it.name}${it.note ? `, ${it.note}` : ""}`);
    }
    try {
      await navigator.clipboard.writeText(lines.join("\n"));
      toast(t("copied"));
    } catch {
      toast(t("copy_failed"));
    }
  });
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
    if (e.metaKey || e.ctrlKey || e.altKey || t.matches("input, textarea, select") || sheet?.open) return;
    if (e.key === "+" || e.key === "=") changeServings(1);
    else if (e.key === "-") changeServings(-1);
    else if (e.key === "c") void setCooking(!cooking);
  });

  if (fromURL || remembered || state.system !== c.system) paint();
  else {
    // Server-rendered markup already matches the default state.
    model = render(c, state);
    paintSteps();
    syncSheet();
    for (const box of document.querySelectorAll<HTMLInputElement>(".ing-check"))
      box.checked = checked.has(box.dataset.id ?? "");
  }
}
