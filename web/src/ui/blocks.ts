// Mirror of internal/site/blocks.go. The markup must stay identical so the
// server-rendered page and live updates look the same.
import { formatNumber } from "../runtime/expr";
import type { LinkM, Model, Seg, Value } from "../runtime/types";
import { qtyString } from "../runtime/units";
import { num, t } from "./i18n";

/**
 * Sets fractions written with the fraction slash ("١⁄٢") with a raised
 * numerator and lowered denominator when the language asks for it
 * (fraction_display: diagonal). Mirrors diagonalFractions in blocks.go.
 */
export function diagonalFractions(html: string): string {
  return html.replace(
    /(\p{Nd}+)\u200e?\u2044(\p{Nd}+)(\u200e?)/gu,
    '<span class="frac"><sup>$1</sup>&#8260;<sub>$2</sub></span>$3',
  );
}

/** Applies diagonalFractions when the page's language enables it. */
export function fr(html: string): string {
  return t("_fracs") === "diagonal" ? diagonalFractions(html) : html;
}

/** Escapes like Go's html.EscapeString. */
export function esc(s: string): string {
  return s.replace(
    /[&'<>"]/g,
    (c) => ({ "&": "&amp;", "'": "&#39;", "<": "&lt;", ">": "&gt;", '"': "&#34;" })[c] as string,
  );
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
    .map(
      (e) => `<li class="equip">${esc(e.name)}${e.note ? ` <span class="equip-note">${esc(e.note)}</span>` : ""}</li>`,
    )
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
      if (it.q) s += `<span class="qty">${esc(qtyString(it.q))}</span>${esc(it.j)}`;
      s += it.link
        ? `<a class="ing-name ing-link" href="${esc(linkHref(base, it.link))}">${esc(it.name)}</a>`
        : `<span class="ing-name">${esc(it.name)}</span>`;
      if (it.note) s += `<span class="ing-note">, ${esc(it.note)}</span>`;
      if (it.uses.length > 0) {
        s += `<span class="ing-uses">${it.uses.map((u) => esc(t("uses", { q: u.q, n: num(u.n) }))).join(" · ")}</span>`;
      }
      s += `</span></label>`;
      if (it.swap) {
        s += `<button type="button" class="swap-btn" data-swap="${esc(it.swap)}" aria-haspopup="dialog"><svg class="icon" aria-hidden="true"><use href="#i-swap"></use></svg>${esc(t("swap"))}</button>`;
      }
      s += `</li>`;
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
        const inner = (g.q ? `<span class="ref-q">${esc(g.q)}</span>${esc(g.j)}` : "") + esc(g.v);
        s += g.link
          ? `<a class="ref ref-link" href="${esc(linkHref(base, g.link))}">${inner}</a>`
          : `<span class="ref" data-ing="${esc(g.id)}">${inner}</span>`;
        break;
      }
      case "eq":
        s += `<span class="ref-eq">${esc(g.v)}</span>`;
        break;
      case "timer":
        s += `<button type="button" class="timer-chip" data-min="${formatNumber(g.min)}"${g.max > 0 ? ` data-max="${formatNumber(g.max)}"` : ""} data-label="${esc(label)}"><svg class="icon" aria-hidden="true"><use href="#i-timer"></use></svg>${esc(g.v)}</button>`;
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
      const n = num(st.n);
      const label = t("step", { n });
      s += `<li class="step" data-n="${st.n}"><button type="button" class="step-num" aria-label="${esc(t("mark_done", { n }))}">${n}</button><div class="step-body"><p class="step-text">${renderSegs(st.segs, base, label)}</p>`;
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
