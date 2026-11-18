// Shopping list kept on the device. Recipe pages add a snapshot of their
// current ingredients (servings, options and units included); the list page
// merges the same ingredient across recipes, converting units when it can,
// and groups lines by aisle.
import type { Compiled, Ing, LinkM, Model, Table } from "../runtime/types";
import { qtyString, Units } from "../runtime/units";
import { esc } from "./blocks";
import { t } from "./i18n";
import { load, save } from "./storage";
import { toast } from "./toast";

export interface ListItem {
  k: string; // merge key: ingredient DB key, or the name
  n: string; // name
  p?: string; // plural
  c: string; // category
  u: string; // display unit id ("" = no amount, "recipe" = a prerequisite)
  v: number; // amount in that unit (the high end for ranges)
  t?: string; // display text when it can't be merged (pinch, prerequisites)
  i?: Ing; // conversion data
  r?: string; // link for prerequisites
}

export interface ListRecipe {
  slug: string;
  title: string;
  url: string;
  yield: string;
  items: ListItem[];
}

interface ListState {
  recipes: ListRecipe[];
  checked: string[];
}

const KEY = "gc:list";
const read = (): ListState => load<ListState>(KEY, { recipes: [], checked: [] });
const write = (s: ListState) => {
  save(KEY, s.recipes.length ? s : undefined);
  updateCount();
};

export function updateCount(): void {
  const el = document.getElementById("list-count");
  if (!el) return;
  const n = read().recipes.length;
  el.textContent = String(n);
  el.hidden = n === 0;
}

export function inList(slug: string): boolean {
  return read().recipes.some((r) => r.slug === slug);
}

/** Builds list items from a rendered recipe. */
export function itemsFrom(c: Compiled, m: Model, href: (l: LinkM) => string): ListItem[] {
  const out: ListItem[] = [];
  const all = c.ingredients.flatMap((s) => s.items);
  for (const sec of m.ingredients) {
    for (const it of sec.items) {
      // Variants share an id; the rendered name tells them apart.
      const ic =
        all.find((x) => x.id === it.id && (x.name === it.name || x.plural === it.name)) ??
        all.find((x) => x.id === it.id);
      const info = ic?.ref ? c.ings?.[ic.ref] : undefined;
      const item: ListItem = {
        k: ic?.ref || `n:${it.name.toLowerCase()}`,
        n: ic?.name ?? it.name,
        p: ic?.plural,
        c: info?.cat ?? "",
        u: "",
        v: 0,
        i: info,
      };
      const v = it.q ? Math.max(it.q.v, it.q.h ?? 0) : 0;
      if (it.link) {
        Object.assign(item, { k: `r:${it.link.slug}`, u: "recipe", t: it.q ? qtyString(it.q) : "", r: href(it.link) });
      } else if (it.q && v > 0) {
        // Ranges buy the high end.
        Object.assign(item, { u: it.q.id, v });
      } else if (it.q) {
        item.t = qtyString(it.q); // "pinch"
      }
      out.push(item);
    }
  }
  return out;
}

export function addRecipe(r: ListRecipe): void {
  const s = read();
  const i = s.recipes.findIndex((x) => x.slug === r.slug);
  if (i >= 0) s.recipes[i] = r;
  else s.recipes.push(r);
  write(s);
  toast(t("list_added"));
}

const ORDER = ["produce", "dairy", "meat", "bakery", "baking", "pantry", "frozen", "other"];

interface Line {
  k: string;
  name: string;
  cat: string;
  text: string;
  link?: string;
}

/** Merges items into display lines (exported for tests). */
export function mergeLines(recipes: ListRecipe[], table: Table, system: string): Line[] {
  const units = new Units(table);
  const groups = new Map<string, ListItem[]>();
  for (const r of recipes) {
    for (const it of r.items) {
      const g = groups.get(it.k) ?? [];
      g.push(it);
      groups.set(it.k, g);
    }
  }
  const lines: Line[] = [];
  for (const [k, items] of groups) {
    const first = items[0] as ListItem;
    const info = items.find((x) => x.i)?.i;
    // Buckets by dimension: volume and mass in base units, counts per unit.
    interface Bucket {
      dim: string;
      base: number;
      unit: string;
      sys: string;
    }
    const buckets = new Map<string, Bucket>();
    const extras: string[] = [];
    let link: string | undefined;
    for (const it of items) {
      const u = table.units[it.u];
      if (it.u === "recipe") {
        extras.push(it.t ?? "");
        link = it.r;
        continue;
      }
      if (!u || it.v <= 0) {
        if (it.t) extras.push(it.t);
        continue;
      }
      const key = u.dim === "count" ? `count:${u.id}` : u.dim;
      const b = buckets.get(key) ?? {
        dim: u.dim,
        base: 0,
        unit: u.dim === "count" ? u.id : u.dim === "volume" ? "ml" : "g",
        sys: "",
      };
      b.base += it.v * u.f;
      if (!b.sys && u.sys) b.sys = u.sys;
      buckets.set(key, b);
    }
    // When one ingredient arrives in different units (tbsp and sticks of
    // butter), merge everything convertible into the largest bucket's unit.
    const grams = (b: Bucket): number | undefined => {
      if (b.dim === "mass") return b.base;
      if (b.dim === "volume") return info?.gpml ? b.base * info.gpml : undefined;
      const g = info?.ug?.[b.unit];
      return g ? b.base * g : undefined;
    };
    if (buckets.size > 1) {
      let target: Bucket | undefined;
      for (const b of buckets.values()) {
        const g = grams(b);
        if (g !== undefined && (!target || g > (grams(target) ?? 0))) target = b;
      }
      const tg = target ? grams(target) : undefined;
      if (target && tg && target.base > 0) {
        const gramsPerUnit = tg / target.base;
        for (const [k, b] of buckets) {
          const g = grams(b);
          if (b === target || g === undefined) continue;
          target.base += g / gramsPerUnit;
          buckets.delete(k);
        }
      }
    }
    const parts: string[] = [];
    let count = 0;
    for (const b of buckets.values()) {
      // "As written" keeps each unit's own system; counts stay counts.
      let sys = system;
      if (sys === "original") sys = b.dim === "count" ? "original" : b.sys || "metric";
      const q = units.format(b.base, b.unit, sys, info, false);
      parts.push(qtyString(q));
      if (q.id === "each") count += q.v;
    }
    const uniq = [...new Set(extras.filter(Boolean))];
    const text = [...parts, ...uniq].join(" + ");
    const name = count > 1 && first.p ? first.p : first.n;
    lines.push({ k, name, cat: first.c || "other", text, link });
  }
  return lines;
}

export function initListPage(): void {
  const page = document.getElementById("list-page");
  const dataEl = document.getElementById("list-data");
  if (!page || !dataEl?.textContent) return;
  const data = JSON.parse(dataEl.textContent) as { units: Table; system: string; categories: Record<string, string> };
  const system = load<string>("gc:units", data.system);

  function paint(): void {
    const s = read();
    const empty = s.recipes.length === 0;
    (document.getElementById("list-empty") as HTMLElement).hidden = !empty;
    (document.getElementById("list-recipes-wrap") as HTMLElement).hidden = empty;
    (document.getElementById("list-items-wrap") as HTMLElement).hidden = empty;
    if (empty) return;
    (document.getElementById("list-recipes") as HTMLElement).innerHTML = s.recipes
      .map(
        (r) =>
          `<li class="list-recipe"><a href="${esc(r.url)}">${esc(r.title)}</a> <span class="ing-note">${esc(r.yield)}</span><button type="button" class="icon-btn list-remove" data-slug="${esc(r.slug)}" aria-label="${esc(t("list_remove", { name: r.title }))}"><svg class="icon" aria-hidden="true"><use href="#i-x"></use></svg></button></li>`,
      )
      .join("");
    const lines = mergeLines(s.recipes, data.units, system);
    const cats = [...new Set(lines.map((l) => l.cat))].sort((a, b) => {
      const ia = ORDER.indexOf(a) < 0 ? 99 : ORDER.indexOf(a);
      const ib = ORDER.indexOf(b) < 0 ? 99 : ORDER.indexOf(b);
      return ia - ib || a.localeCompare(b);
    });
    const checked = new Set(s.checked);
    let html = "";
    for (const cat of cats) {
      const label = data.categories[cat] ?? cat.charAt(0).toUpperCase() + cat.slice(1);
      html += `<section class="ing-section"><h3 class="ing-title">${esc(label)}</h3><ul class="ing-list">`;
      for (const l of lines.filter((x) => x.cat === cat).sort((a, b) => a.name.localeCompare(b.name))) {
        const name = l.link
          ? `<a class="ing-name ing-link" href="${esc(l.link)}">${esc(l.name)}</a>`
          : `<span class="ing-name">${esc(l.name)}</span>`;
        html += `<li class="ing" data-k="${esc(l.k)}"><label class="ing-label"><input type="checkbox" class="ing-check list-check" data-k="${esc(l.k)}"${checked.has(l.k) ? " checked" : ""}><span class="ing-text">${name}${l.text ? ` <span class="qty list-qty">${esc(l.text)}</span>` : ""}</span></label></li>`;
      }
      html += "</ul></section>";
    }
    (document.getElementById("list-items") as HTMLElement).innerHTML = html;
  }

  page.addEventListener("change", (e) => {
    const box = e.target as HTMLInputElement;
    if (!box.classList.contains("list-check")) return;
    const s = read();
    const set = new Set(s.checked);
    if (box.checked) set.add(box.dataset.k ?? "");
    else set.delete(box.dataset.k ?? "");
    s.checked = [...set];
    write(s);
  });
  page.addEventListener("click", (e) => {
    const rm = (e.target as Element).closest<HTMLElement>(".list-remove");
    if (rm) {
      const s = read();
      s.recipes = s.recipes.filter((r) => r.slug !== rm.dataset.slug);
      write(s);
      paint();
    }
  });
  document.getElementById("list-clear")?.addEventListener("click", () => {
    if (!confirm(t("list_confirm_clear"))) return;
    write({ recipes: [], checked: [] });
    paint();
  });
  document.getElementById("list-clear-checked")?.addEventListener("click", () => {
    const s = read();
    const checked = new Set(s.checked);
    // Remove checked lines from every recipe snapshot.
    for (const r of s.recipes) r.items = r.items.filter((it) => !checked.has(it.k));
    s.recipes = s.recipes.filter((r) => r.items.length > 0);
    s.checked = [];
    write(s);
    paint();
  });
  document.getElementById("list-copy")?.addEventListener("click", async () => {
    const s = read();
    const lines = mergeLines(s.recipes, data.units, system).filter((l) => !s.checked.includes(l.k));
    const text = `${t("list")}\n\n${lines.map((l) => `- ${l.text ? `${l.text} ` : ""}${l.name}`).join("\n")}`;
    try {
      await navigator.clipboard.writeText(text);
      toast(t("copied"));
    } catch {
      // ignore
    }
  });
  paint();
}
