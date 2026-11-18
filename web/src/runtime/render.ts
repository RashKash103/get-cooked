// Port of internal/runtime/runtime.go: renders a compiled recipe for a state.
import { type Env, ExprError, evaluate, formatNumber, kind, round, roundTo, str } from "./expr";
import type {
  Amount,
  Compiled,
  IngC,
  IngM,
  IR,
  LinkM,
  Model,
  OptionC,
  Qty,
  Seg,
  State,
  Template,
  Value,
} from "./types";
import { length, qtyString, roundRule, temp, Units } from "./units";

export class RenderError extends Error {}

function optionValue(o: OptionC, raw: Value | undefined): Value {
  switch (o.type) {
    case "toggle":
      return typeof raw === "boolean" ? raw : Boolean(o.default);
    case "choice":
      if (typeof raw === "string" && o.choices?.some((c) => c.id === raw)) return raw;
      return String(o.default);
    default: {
      let v = typeof raw === "number" && Number.isFinite(raw) ? raw : Number(o.default);
      if (o.min !== undefined) v = Math.max(v, o.min);
      if (o.max !== undefined) v = Math.min(v, o.max);
      return v;
    }
  }
}

export function environment(c: Compiled, st: State): { env: Env; servings: number; system: string } {
  let servings = st.servings && st.servings > 0 ? st.servings : c.servings.default;
  servings = Math.min(Math.max(servings, c.servings.min), c.servings.max);
  let system = st.system ?? "";
  if (system !== "us" && system !== "metric" && system !== "original") system = c.system;
  const env: Env = {
    servings,
    base_servings: c.servings.default,
    scale: servings / c.servings.default,
    metric: system === "metric",
  };
  for (const o of c.options ?? []) env[o.id] = optionValue(o, st.options?.[o.id]);
  if (c.scale) {
    const s = evaluate(c.scale, env);
    if (typeof s !== "number") throw new RenderError("scaling: expected a number");
    if (s <= 0) throw new RenderError(`scaling: scale must be positive, got ${formatNumber(s)}`);
    env.scale = s;
  }
  for (const v of c.vars ?? []) env[v.id] = evaluate(v.e, env);
  return { env, servings, system };
}

interface Active {
  ic: IngC;
  value: number;
  has: boolean;
  parts?: Map<string, number>;
}

const fracRule = { denoms: [2, 3, 4, 8] };
const recipeRule = { denoms: [2, 3, 4] };

class Renderer {
  readonly units: Units;
  readonly ings = new Map<string, Active>();
  readonly equip = new Map<string, string>();
  readonly uses = new Map<string, { n: number; q: string }[]>();
  step = 0;

  constructor(
    readonly c: Compiled,
    readonly env: Env,
    readonly system: string,
  ) {
    this.units = new Units(c.units);
  }

  cond(e: IR | undefined): boolean {
    if (!e) return true;
    const v = evaluate(e, this.env);
    if (typeof v !== "boolean")
      throw new RenderError(`when: condition must be true/false but got ${kind(v)} "${str(v)}"`);
    return v;
  }

  num(e: IR, what: string): number {
    const v = evaluate(e, this.env);
    if (typeof v !== "number") throw new RenderError(`${what}: expected a number but got ${kind(v)} "${str(v)}"`);
    return v;
  }

  amount(a: Amount, what: string): number {
    let v: number;
    if (a.e) v = this.num(a.e, `amount of "${what}"`);
    else if (a.n === undefined) v = 0;
    else if (a.s) v = a.n * (this.env.scale as number);
    else v = a.n;
    if (v < 0) throw new RenderError(`amount of "${what}" is negative (${formatNumber(v)})`);
    return v;
  }

  qty(ic: IngC, v: number): Qty {
    const ref = ic.recipe;
    if (ref) {
      let q = roundRule(recipeRule, v);
      if (q === 0) q = 0.25;
      return { v: q, t: this.units.frac(q), u: q <= 1 ? ref.one : ref.other, id: "recipe" };
    }
    const info = ic.ref ? this.c.ings?.[ic.ref] : undefined;
    return this.units.format(v, ic.unit || "each", this.system, info, Boolean(ic.fixed));
  }

  link(ic: IngC, v: number): LinkM | null {
    const ref = ic.recipe;
    if (!ref) return null;
    const step = ref.step > 0 ? ref.step : 1;
    let s = Math.ceil(v / step - 1e-9) * step;
    s = roundTo(Math.min(Math.max(s, ref.min), ref.max), 6);
    const options: Record<string, Value> = {};
    for (const w of ref.with ?? []) {
      const val = evaluate(w.e, this.env);
      const want = { toggle: "boolean", choice: "string", number: "number" }[w.type];
      if (w.type && kind(val) !== want) throw new RenderError(`with.${w.id}: option is a ${w.type}`);
      options[w.id] = val;
    }
    return { slug: ref.slug, servings: s, options };
  }

  name(ic: IngC, q: Qty | null): string {
    if (q && q.id === "each" && q.v > 1 && ic.plural) return ic.plural;
    return ic.name;
  }

  format(v: Value, f: string | undefined, args: string[] | undefined): string {
    if (!f) return str(v);
    if (typeof v !== "number") throw new RenderError(`formatter "${f}" needs a number, got ${kind(v)} "${str(v)}"`);
    switch (f) {
      case "frac":
        return this.units.frac(roundRule(fracRule, v));
      case "int":
        return formatNumber(round(v));
      case "duration":
        return this.units.duration(v);
      case "temp":
        return temp(v, false, this.system);
      case "temp_c":
        return temp(v, true, this.system);
      case "len":
        return length(v, false, this.system);
      case "len_cm":
        return length(v, true, this.system);
      case "qty":
        return qtyString(this.units.format(v, (args?.[0] ?? "") as string, this.system, undefined, false));
    }
    throw new RenderError(`unknown formatter "${f}"`);
  }

  segs(t: Template | undefined, out: Seg[] = []): Seg[] {
    for (const s of t ?? []) {
      if (typeof s === "string") {
        out.push(seg("text", s));
      } else if ("e" in s) {
        out.push(seg("text", this.format(evaluate(s.e, this.env), s.f, s.a)));
      } else if ("ing" in s) {
        const a = this.ings.get(s.ing);
        if (!a) throw new RenderError(`@${s.ing} is referenced but not included for this selection`);
        const g = seg("ing", "");
        g.id = s.ing;
        let q: Qty | null = null;
        if (a.has && !s.n) {
          let v = a.value;
          if (s.pt) {
            const pv = a.parts?.get(s.pt);
            if (pv === undefined) throw new RenderError(`@${s.ing}.${s.pt}: no split named "${s.pt}"`);
            v = pv;
          }
          if (s.p) {
            const p = this.num(s.p, `@${s.ing} portion`);
            if (p <= 0) throw new RenderError(`@${s.ing} portion must be positive`);
            v *= p;
          }
          q = this.qty(a.ic, v);
          g.q = qtyString(q);
          g.j = this.units.join(q);
          if (this.step > 0 && (s.pt || s.p)) {
            const list = this.uses.get(s.ing) ?? [];
            list.push({ n: this.step, q: g.q });
            this.uses.set(s.ing, list);
          }
        }
        if (a.has) g.link = this.link(a.ic, a.value);
        g.v = this.name(a.ic, q);
        out.push(g);
      } else if ("eq" in s) {
        const name = this.equip.get(s.eq);
        if (name === undefined) throw new RenderError(`&${s.eq} is referenced but not included for this selection`);
        const g = seg("eq", name);
        g.id = s.eq;
        out.push(g);
      } else if ("timer" in s) {
        const v = this.num(s.timer, "timer");
        if (v <= 0) throw new RenderError(`timer must be positive, got ${formatNumber(v)}`);
        const g = seg("timer", this.units.duration(v));
        g.min = roundTo(v, 2);
        out.push(g);
      } else if ("if" in s) {
        let matched = false;
        for (const b of s.if) {
          const c = evaluate(b.c, this.env);
          if (typeof c !== "boolean") throw new RenderError("{if}: condition must be true/false");
          if (c) {
            this.segs(b.t, out);
            matched = true;
            break;
          }
        }
        if (!matched) this.segs(s.else, out);
      }
    }
    return out;
  }

  flat(t: Template | undefined): string {
    if (!t || t.length === 0) return "";
    let s = "";
    for (const g of this.segs(t)) {
      if (g.t === "ing" && g.q !== "") s += g.q + g.j;
      s += g.v;
    }
    return s;
  }
}

function seg(t: Seg["t"], v: string): Seg {
  return { t, v, id: "", q: "", j: "", min: 0, link: null };
}

function merge(segs: Seg[]): Seg[] {
  const out: Seg[] = [];
  for (const s of segs) {
    if (s.t === "text") {
      if (s.v === "") continue;
      const last = out[out.length - 1];
      if (last && last.t === "text") {
        last.v += s.v;
        continue;
      }
    }
    out.push(s);
  }
  return out;
}

/** Renders a compiled recipe for a state. Throws RenderError/ExprError. */
export function render(c: Compiled, st: State): Model {
  const { env, servings, system } = environment(c, st);
  const r = new Renderer(c, env, system);
  const m: Model = {
    servings,
    yield: `${r.units.frac(servings)} ${servings === 1 ? c.servings.one : c.servings.other}`,
    scale: roundTo(env.scale as number, 4),
    system,
    options: [],
    times: [],
    equipment: [],
    ingredients: [],
    steps: [],
    notes: [],
  };
  for (const o of c.options ?? []) if (r.cond(o.when)) m.options.push(o.id);
  for (const t of c.times ?? []) {
    const v = r.num(t.e, `times.${t.id}`);
    if (v < 0) throw new RenderError(`times.${t.id} is negative`);
    m.times.push({ id: t.id, label: t.label, minutes: roundTo(v, 2), text: r.units.duration(v) });
  }
  const pending: { title: string; items: Active[] }[] = [];
  for (const s of c.ingredients) {
    if (!r.cond(s.when)) continue;
    const ps = { title: s.title ?? "", items: [] as Active[] };
    for (const ic of s.items) {
      if (!r.cond(ic.when)) continue;
      const a: Active = { ic, value: 0, has: false };
      if (ic.amount) {
        const v = r.amount(ic.amount, ic.id);
        if (v === 0) continue;
        a.value = v;
        a.has = true;
      }
      if (ic.split && ic.split.length > 0) {
        a.parts = new Map();
        for (const sp of ic.split) {
          const v = r.amount(sp.a, `${ic.id}.${sp.id}`);
          a.parts.set(sp.id, v);
          a.value += v;
        }
        if (a.value === 0) continue;
        a.has = true;
      }
      if (r.ings.has(ic.id)) throw new RenderError(`ingredient "${ic.id}" is included twice`);
      r.ings.set(ic.id, a);
      ps.items.push(a);
    }
    pending.push(ps);
  }
  for (const e of c.equipment ?? []) {
    if (!r.cond(e.when)) continue;
    if (r.equip.has(e.id)) throw new RenderError(`equipment "${e.id}" is included twice`);
    r.equip.set(e.id, "");
    const name = r.flat(e.name);
    r.equip.set(e.id, name);
    m.equipment.push({ id: e.id, name, note: r.flat(e.note) });
  }
  for (const ps of pending) {
    const items: IngM[] = [];
    for (const a of ps.items) {
      const im: IngM = {
        id: a.ic.id,
        q: null,
        j: "",
        name: "",
        note: "",
        optional: Boolean(a.ic.optional),
        link: null,
        uses: [],
      };
      if (a.has) {
        im.q = r.qty(a.ic, a.value);
        im.link = r.link(a.ic, a.value);
      }
      im.name = r.name(a.ic, im.q);
      im.j = r.units.join(im.q);
      im.note = r.flat(a.ic.note);
      items.push(im);
    }
    if (items.length > 0) m.ingredients.push({ title: ps.title, items });
  }
  let n = 0;
  for (const s of c.steps) {
    if (!r.cond(s.when)) continue;
    const items: Model["steps"][number]["items"] = [];
    for (const st of s.items) {
      if (!r.cond(st.when)) continue;
      n++;
      r.step = n;
      const segs = r.segs(st.text);
      r.step = 0;
      const note = r.flat(st.note);
      items.push({ n, segs: merge(segs), note, image: st.image ?? "" });
    }
    if (items.length > 0) m.steps.push({ title: s.title ?? "", items });
  }
  for (const sec of m.ingredients) {
    for (const it of sec.items) {
      const u = r.uses.get(it.id);
      if (u && u.length >= 2) it.uses = u;
    }
  }
  for (const nt of c.notes ?? []) {
    if (!r.cond(nt.when)) continue;
    m.notes.push({ kind: nt.kind, text: r.flat(nt.text) });
  }
  return m;
}

export { ExprError };
