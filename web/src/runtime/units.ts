// Port of internal/units (Go). Keep the two in lock-step; the conformance
// tests replay Go's golden output through this file.
import { formatNumber, round, roundTo } from "./expr";
import type { Ing, Qty, Rule, Rung, Table, Unit } from "./types";

export function roundRule(r: Rule, x: number): number {
  if (r.tiers && r.tiers.length > 0) {
    let step = (r.tiers[r.tiers.length - 1] as { step: number }).step;
    for (const t of r.tiers) {
      if (!t.below || Math.abs(x) < t.below) {
        step = t.step;
        break;
      }
    }
    return roundTo(round(x / step) * step, 6);
  }
  const denoms = r.denoms && r.denoms.length > 0 ? r.denoms : [1];
  let best = 0;
  let bestErr = Number.POSITIVE_INFINITY;
  let bestD = 0;
  for (const d of denoms) {
    const c = round(x * d) / d;
    const e = Math.abs(c - x);
    if (e < bestErr - 1e-12 || (Math.abs(e - bestErr) <= 1e-12 && d < bestD)) {
      best = c;
      bestErr = e;
      bestD = d;
    }
  }
  return roundTo(best, 6);
}

function relErr(r: number, x: number): number {
  return x === 0 ? 0 : Math.abs(r - x) / Math.abs(x);
}

const UNICODE: Record<string, string> = {
  "1/2": "½",
  "1/3": "⅓",
  "2/3": "⅔",
  "1/4": "¼",
  "3/4": "¾",
  "1/8": "⅛",
  "3/8": "⅜",
  "5/8": "⅝",
  "7/8": "⅞",
  "1/5": "⅕",
  "2/5": "⅖",
  "3/5": "⅗",
  "4/5": "⅘",
  "1/6": "⅙",
  "5/6": "⅚",
};

function gcd(a: number, b: number): number {
  while (b !== 0) [a, b] = [b, a % b];
  return a;
}

function decimal(s: string, dec: string): string {
  return dec && dec !== "." ? s.replace(".", dec) : s;
}

/** Replaces ASCII digits with the given ten digits (no-op when empty). */
export function localDigits(s: string, digits?: string): string {
  if (!digits) return s;
  const d = [...digits];
  if (d.length !== 10) return s;
  // A left-to-right mark after each run of digits keeps "١ ١⁄٢" and ranges
  // in logical order (see Table.L in internal/units/units.go).
  return s
    .replace(/[0-9]+(?:[.,٫٬][0-9]+)*/g, (run) => `${run}\u200e`)
    .replace(/[0-9]/g, (c) => d[c.charCodeAt(0) - 48] as string);
}

export function formatFraction(
  x: number,
  style = "unicode",
  dec = ".",
  digits = "",
  glyphs: Record<string, string> = {},
): string {
  const fmt = (v: number) => localDigits(decimal(formatNumber(v), dec), digits);
  if (style === "decimal") return fmt(x);
  const neg = x < 0;
  const ax = Math.abs(x);
  const whole = Math.floor(ax + 1e-9);
  const frac = ax - whole;
  if (frac < 1e-6) return fmt(x);
  for (let d = 2; d <= 8; d++) {
    const n = round(frac * d);
    if (n === 0 || n === d || Math.abs(frac * d - n) > 1e-4) continue;
    const g = gcd(n, d);
    const num = n / g;
    const den = d / g;
    let f: string;
    const custom = glyphs[`${num}/${den}`];
    if (custom !== undefined) f = custom;
    else if (style === "ascii") f = localDigits(`${num}/${den}`, digits);
    else if (style === "native") f = localDigits(`${num}\u2044${den}`, digits);
    else {
      const u = UNICODE[`${num}/${den}`];
      if (!u) return fmt(x);
      f = u;
    }
    const s = whole > 0 ? `${fmt(whole)} ${f}` : f;
    return neg ? `-${s}` : s;
  }
  return fmt(x);
}

export function qtyString(q: Qty): string {
  if (q.t === "") return q.u;
  if (q.u === "") return q.t;
  return `${q.t} ${q.u}`;
}

function label(x: Unit, v: number): string {
  return v > 1 ? x.other : x.one;
}

function smallest(r: Rule): number {
  if (r.tiers && r.tiers.length > 0) return (r.tiers[0] as { step: number }).step;
  let m = 1;
  for (const d of r.denoms ?? []) if (d > m) m = d;
  return 1 / m;
}

function floorTo(r: Rule, x: number): number {
  let best = 0;
  for (const d of r.denoms ?? []) {
    const c = Math.floor(x * d + 1e-9) / d;
    if (c > best) best = c;
  }
  return roundTo(best, 6);
}

export class Units {
  constructor(readonly t: Table) {}

  frac(x: number): string {
    return formatFraction(x, this.t.fractions, this.t.dec ?? ".", this.t.digits ?? "", this.t.glyphs ?? {});
  }

  num(x: number): string {
    return this.l(decimal(formatNumber(x), this.t.dec ?? "."));
  }

  l(s: string): string {
    return localDigits(s, this.t.digits);
  }

  join(q: Qty | null): string {
    if (!q || !this.t.of || q.u === "" || q.id === "each") return " ";
    return ` ${this.t.of} `;
  }

  duration(min: number): string {
    return this.l(duration(min, this.t.words));
  }

  private text(r: Rule, v: number): string {
    return r.tiers && r.tiers.length > 0 ? this.num(v) : this.frac(v);
  }

  private unit(id: string): Unit {
    return this.t.units[id] as Unit;
  }

  private inLadder(sys: string, dim: string, id: string): boolean {
    return (this.t.ladders[`${sys}.${dim}`] ?? []).some((r) => r.unit === id);
  }

  private ruleFor(sys: string, dim: string, id: string): Rule {
    return (this.t.ladders[`${sys}.${dim}`] ?? []).find((r) => r.unit === id) ?? this.t.fallback;
  }

  format(amount: number, unitID: string, system: string, ing: Ing | undefined, fixed: boolean): Qty {
    const src = this.t.units[unitID];
    if (!src) {
      return { v: amount, t: this.frac(roundRule(this.t.fallback, amount)), u: unitID, id: unitID };
    }
    if (src.dim === "count") return this.formatCount(amount, src, system, ing);
    let dim = src.dim;
    let sys = src.sys ?? "";
    let base = amount * src.f;
    if (!fixed) {
      if (system === "metric") {
        sys = "metric";
        if (dim === "volume" && ing?.gpml && ing.measure !== "volume") {
          dim = "mass";
          base *= ing.gpml;
        }
      } else if (system === "us") {
        sys = "us";
        if (dim === "mass" && ing?.gpml && ing.measure === "volume") {
          dim = "volume";
          base /= ing.gpml;
        }
      }
    }
    if (fixed || (sys === (src.sys ?? "") && dim === src.dim && !this.inLadder(sys, dim, src.id))) {
      const r = this.ruleFor(sys, dim, src.id);
      const v = roundRule(r, amount);
      if (fixed || (v > 0 && relErr(v, amount) <= this.t.tol)) {
        return { v, t: this.text(r, v), u: label(src, v), id: src.id };
      }
    }
    return this.ladder(base, sys, dim);
  }

  formatRange(lo: number, hi: number, unitID: string, system: string, ing: Ing | undefined, fixed: boolean): Qty {
    const qh = this.format(hi, unitID, system, ing, fixed);
    const ql = this.format(lo, unitID, system, ing, fixed);
    if (qtyString(ql) === qtyString(qh)) return qh;
    if (ql.id === qh.id && ql.t !== "" && qh.t !== "" && !qh.t.includes("+") && !ql.t.includes("+")) {
      return { v: ql.v, h: qh.v, t: `${ql.t}–${qh.t}`, u: qh.u, id: qh.id };
    }
    return { v: ql.v, h: qh.v, t: `${qtyString(ql)} – ${qtyString(qh)}`, u: "", id: qh.id };
  }

  private ladder(base: number, sys: string, dim: string): Qty {
    const rungs = this.t.ladders[`${sys}.${dim}`] ?? [];
    if (rungs.length === 0) {
      const id = dim === "volume" ? "ml" : "g";
      const x = this.unit(id);
      const v = roundRule(this.t.fallback, base / x.f);
      return { v, t: this.frac(v), u: label(x, v), id };
    }
    for (let i = rungs.length - 1; i >= 0; i--) {
      const r = rungs[i] as Rung;
      const x = this.unit(r.unit);
      const val = base / x.f;
      const v = roundRule(r, val);
      if (v <= 0 || v < (r.min ?? 0) || relErr(v, val) > this.t.tol) continue;
      if (r.max && v > r.max && i + 1 < rungs.length) {
        const q = this.compound(base, r, rungs[i + 1] as Rung);
        if (q) return q;
      }
      return { v, t: this.text(r, v), u: label(x, v), id: x.id };
    }
    const r = rungs[0] as Rung;
    const x = this.unit(r.unit);
    const val = base / x.f;
    let v = roundRule(r, val);
    if (v === 0 && val > 0) {
      if (r.tiny) return { v: 0, t: "", u: r.tiny, id: x.id };
      v = smallest(r);
    }
    return { v, t: this.text(r, v), u: label(x, v), id: x.id };
  }

  private compound(base: number, small: Rung, big: Rung): Qty | null {
    const x = this.unit(small.unit);
    const bx = this.unit(big.unit);
    const whole = floorTo(big, base / bx.f);
    if (whole <= 0 || whole < (big.min ?? 0)) return null;
    const rest = roundRule(small, (base - whole * bx.f) / x.f);
    if (rest <= 0) return { v: whole, t: this.text(big, whole), u: label(bx, whole), id: bx.id };
    const text = `${this.text(big, whole)} ${label(bx, whole)} + ${this.text(small, rest)}`;
    return { v: roundTo(whole + (rest * x.f) / bx.f, 6), t: text, u: label(x, rest), id: bx.id };
  }

  private formatCount(amount: number, src: Unit, system: string, ing: Ing | undefined): Qty {
    const g = ing?.ug?.[src.id];
    if (system === "metric" && g && g > 0) return this.ladder(amount * g, "metric", "mass");
    const r = ing?.round ?? this.t.count;
    let v = roundRule(r, amount);
    if (v < (r.min ?? 0)) v = r.min ?? 0;
    if (v === 0 && amount > 0) v = smallest(r);
    return { v, t: this.text(r, v), u: label(src, v), id: src.id };
  }
}

export function temp(v: number, fromC: boolean, system: string): string {
  const toC = system === "metric" || (system === "original" && fromC);
  if (toC === fromC) return `${formatNumber(round(v))}°${toC ? "C" : "F"}`;
  if (toC) return `${formatNumber(round(((v - 32) * 5) / 9 / 5) * 5)}°C`;
  const f = (v * 9) / 5 + 32;
  const step = f > 250 ? 25 : 5;
  return `${formatNumber(round(f / step) * step)}°F`;
}

export function length(v: number, fromCM: boolean, system: string): string {
  const toCM = system === "metric" || (system === "original" && fromCM);
  if (toCM && fromCM) return `${formatNumber(v)} cm`;
  if (!toCM && !fromCM) return `${formatFraction(round(v * 2) / 2)} in`;
  if (toCM) return `${formatNumber(round(v * 2.54))} cm`;
  return `${formatFraction(round((v / 2.54) * 2) / 2)} in`;
}

export function duration(min: number, words?: Record<string, string>): string {
  const w = (k: string) => words?.[k] || k;
  if (min <= 0) return `0 ${w("min")}`;
  if (min < 1) return `${formatNumber(Math.max(5, round((min * 60) / 5) * 5))} ${w("sec")}`;
  const secs = round(min * 60);
  if (min < 10 && secs % 60 !== 0) {
    let sec = round((secs % 60) / 5) * 5;
    let m = Math.floor(secs / 60);
    if (sec === 60) {
      m += 1;
      sec = 0;
    }
    return sec === 0 ? `${m} ${w("min")}` : `${m} ${w("min")} ${sec} ${w("sec")}`;
  }
  const total = round(min);
  const h = Math.floor(total / 60);
  const m = total % 60;
  if (h === 0) return `${m} ${w("min")}`;
  if (m === 0) return `${h} ${w("hr")}`;
  return `${h} ${w("hr")} ${m} ${w("min")}`;
}
