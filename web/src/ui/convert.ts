// "Other units": the same amount of an ingredient in US and metric, by
// volume and by weight when the ingredient's density (or weight each) is known.
import type { Ing, Qty, Table } from "../runtime/types";
import { qtyString, Units } from "../runtime/units";

export interface Conversion {
  sys: string; // "us" | "metric"
  dim: string; // "volume" | "mass" | "count" | …
  text: string;
}

/** Lists the amount q in other units, skipping duplicates and the current text. */
export function conversions(q: Qty, ing: Ing | undefined, table: Table): Conversion[] {
  const u = new Units(table);
  const src = table.units[q.id];
  if (!src || q.v <= 0) return [];
  const current = qtyString(q);
  const out: Conversion[] = [];
  const seen = new Set<string>([current]);
  const add = (qq: Qty, sys: string) => {
    const text = qtyString(qq);
    const dim = table.units[qq.id]?.dim ?? src.dim;
    if (!text || seen.has(text) || out.some((c) => c.sys === sys && c.dim === dim)) return;
    seen.add(text);
    out.push({ sys, dim, text });
  };
  const fmt = (v: number, unit: string, sys: string, i: Ing | undefined): Qty =>
    q.h ? u.formatRange(v, (q.h * v) / q.v, unit, sys, i, false) : u.format(v, unit, sys, i, false);
  for (const sys of ["us", "metric"]) {
    if (src.dim === "count") {
      // Whole items: show their weight when it's known.
      if (ing?.each) add(fmt(q.v * ing.each, "g", sys, { ...ing, measure: "mass" }), sys);
      continue;
    }
    for (const measure of ["volume", "mass"]) {
      if (!ing?.gpml && measure !== (src.dim === "mass" ? "mass" : "volume")) continue;
      add(fmt(q.v, q.id, sys, ing ? { ...ing, measure } : undefined), sys);
    }
  }
  return out;
}
