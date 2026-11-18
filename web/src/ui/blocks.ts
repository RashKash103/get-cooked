// Mirror of internal/site/blocks.go. The markup must stay identical so the
// server-rendered page and live updates look the same.
import { formatNumber } from "../runtime/expr";
import type { LinkM, Model, Seg, Value } from "../runtime/types";
import { qtyString } from "../runtime/units";

/** Escapes like Go's html.EscapeString. */
export function esc(s: string): string {
  return s.replace(/[&'<>"]/g, (c) => ({ "&": "&amp;", "'": "&#39;", "<": "&lt;", ">": "&gt;", '"': "&#34;" })[c] as string);
}

export function recipeURL(base: string, slug: string, servings: number, options: Record<string, Value>): string {
  let u = `${base}recipes/${slug}/`;
  const q: string[] = [];
  if (servings > 0) q.push(`s=${formatNumber(servings)}`);
  for (const k of Object.keys(options).sort()) {
    const x = options[k] as Value;
    const v = typeof x === "boolean" ? (x ? "1" : "0") : typeof x === "number" ? formatNumber(x) : x;
    q.push(`o.${encodeURIComponent(k)}=${encodeURIComponent(v)}`);
  }
  if (q.length > 0) u += `?${q.join("&")}`;
  return u;
}

const linkHref = (base: string, l: LinkM) => recipeURL(base, l.slug, l.servings, l.options);

export function renderTimes(m: Model): string {
  return m.times
    .map(
      (t) =>
        `<div class="${t.id === "total" ? "time time-total" : "time"}"><dt>${esc(t.label)}</dt><dd>${esc(t.text)}</dd></div>`,
    )
    .join("");
}

export function renderEquipment(m: Model): string {
  return m.equipment
    .map((e) => `<li class="equip">${esc(e.name)}${e.note ? ` <span class="equip-note">${esc(e.note)}</span>` : ""}</li>`)
    .join("");
}

export function renderIngredients(m: Model, base: string): string {
  let s = "";
  for (const sec of m.ingredients) {
    s += `<section class="ing-section">`;
    if (sec.title) s += `<h3 class="ing-title">${esc(sec.title)}</h3>`;
    s += `<ul class="ing-list">`;
    for (const it of sec.items) {
      s += `<li class="ing" data-id="${esc(it.id)}"><label class="ing-label"><input type="checkbox" class="ing-check" data-id="${esc(it.id)}"><span class="ing-text">`;
      if (it.q) s += `<span class="qty">${esc(qtyString(it.q))}</span> `;
      s += it.link
        ? `<a class="ing-name ing-link" href="${esc(linkHref(base, it.link))}">${esc(it.name)}</a>`
        : `<span class="ing-name">${esc(it.name)}</span>`;
      if (it.note) s += `<span class="ing-note">, ${esc(it.note)}</span>`;
      s += `</span></label></li>`;
    }
    s += `</ul></section>`;
  }
  return s;
}

export function renderSegs(segs: Seg[], base: string, label: string): string {
  let s = "";
  for (const g of segs) {
    switch (g.t) {
      case "ing": {
        const inner = (g.q ? `<span class="ref-q">${esc(g.q)}</span> ` : "") + esc(g.v);
        s += g.link
          ? `<a class="ref ref-link" href="${esc(linkHref(base, g.link))}">${inner}</a>`
          : `<span class="ref" data-ing="${esc(g.id)}">${inner}</span>`;
        break;
      }
      case "eq":
        s += `<span class="ref-eq">${esc(g.v)}</span>`;
        break;
      case "timer":
        s += `<button type="button" class="timer-chip" data-min="${formatNumber(g.min)}" data-label="${esc(label)}"><svg class="icon" aria-hidden="true"><use href="#i-timer"></use></svg>${esc(g.v)}</button>`;
        break;
      default:
        s += esc(g.v);
    }
  }
  return s;
}

export function renderSteps(m: Model, base: string): string {
  let s = "";
  for (const sec of m.steps) {
    s += `<section class="step-section">`;
    if (sec.title) s += `<h3 class="step-title">${esc(sec.title)}</h3>`;
    s += `<ol class="step-list">`;
    for (const st of sec.items) {
      const label = `Step ${st.n}`;
      s += `<li class="step" data-n="${st.n}"><button type="button" class="step-num" aria-label="Mark step ${st.n} done">${st.n}</button><div class="step-body"><p class="step-text">${renderSegs(st.segs, base, label)}</p>`;
      if (st.note) s += `<p class="step-note">${esc(st.note)}</p>`;
      if (st.image) s += `<img class="step-img" src="${esc(st.image)}" alt="" loading="lazy">`;
      s += `</div></li>`;
    }
    s += `</ol></section>`;
  }
  return s;
}

const ICONS: Record<string, string> = { warning: "i-alert", tip: "i-sparkle", info: "i-info" };

export function renderNotes(m: Model, warnings: boolean): string {
  return m.notes
    .filter((n) => (n.kind === "warning") === warnings)
    .map(
      (n) =>
        `<aside class="note note-${esc(n.kind)}"><svg class="icon" aria-hidden="true"><use href="#${ICONS[n.kind] ?? ""}"></use></svg><p>${esc(n.text)}</p></aside>`,
    )
    .join("");
}
