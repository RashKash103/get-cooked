import { describe, expect, it } from "vitest";
import type { Ing, Table } from "../runtime/types";
import { decodeList, encodeList, type ListRecipe, mergeLines, mergeShared } from "./list";

// A small table: tsp, tbsp, cup ladder plus grams and sticks.
const table: Table = {
  units: {
    tsp: { id: "tsp", dim: "volume", sys: "us", f: 4.92892159375, one: "tsp", other: "tsp" },
    tbsp: { id: "tbsp", dim: "volume", sys: "us", f: 14.78676478125, one: "tbsp", other: "tbsp" },
    cup: { id: "cup", dim: "volume", sys: "us", f: 236.5882365, one: "cup", other: "cups" },
    ml: { id: "ml", dim: "volume", sys: "metric", f: 1, one: "ml", other: "ml" },
    g: { id: "g", dim: "mass", sys: "metric", f: 1, one: "g", other: "g" },
    each: { id: "each", dim: "count", f: 1, one: "", other: "" },
    stick: { id: "stick", dim: "count", f: 1, one: "stick", other: "sticks" },
  },
  ladders: {
    "us.volume": [
      { unit: "tsp", denoms: [2, 4, 8], tiny: "pinch" },
      { unit: "tbsp", denoms: [2], min: 1, max: 8 },
      { unit: "cup", denoms: [2, 3, 4], min: 0.25 },
    ],
    "metric.mass": [{ unit: "g", tiers: [{ below: 10, step: 0.5 }, { below: 1000, step: 1 }, { step: 5 }] }],
    "metric.volume": [{ unit: "ml", tiers: [{ below: 10, step: 0.5 }, { below: 1000, step: 1 }, { step: 5 }] }],
  },
  tol: 0.1,
  fractions: "unicode",
  count: { denoms: [2, 4] },
  fallback: { denoms: [2, 3, 4, 8] },
};

const butter: Ing = { name: "butter", gpml: 0.959, ug: { stick: 113 } };
const flour: Ing = { name: "flour", gpml: 125 / 236.5882365 };

const recipes: ListRecipe[] = [
  {
    slug: "a",
    title: "A",
    url: "/a/",
    yield: "4",
    items: [
      { k: "butter", n: "butter", c: "dairy", u: "tbsp", v: 2, i: butter },
      { k: "flour", n: "flour", c: "baking", u: "cup", v: 1, i: flour },
      { k: "egg", n: "egg", p: "eggs", c: "dairy", u: "each", v: 2 },
      { k: "n:salt", n: "salt", c: "", u: "", v: 0 },
    ],
  },
  {
    slug: "b",
    title: "B",
    url: "/b/",
    yield: "2",
    items: [
      { k: "butter", n: "butter", c: "dairy", u: "stick", v: 1, i: butter },
      { k: "flour", n: "flour", c: "baking", u: "g", v: 125, i: flour },
      { k: "egg", n: "egg", p: "eggs", c: "dairy", u: "each", v: 1 },
      { k: "n:salt", n: "salt", c: "", u: "", v: 0, t: "pinch" },
    ],
  },
];

describe("shopping list merge", () => {
  const lines = Object.fromEntries(mergeLines(recipes, table, "original").map((l) => [l.k, l]));
  it("merges tablespoons of butter into the larger stick amount", () => {
    expect(lines.butter?.text).toBe("1 ¼ sticks");
    const metric = Object.fromEntries(mergeLines(recipes, table, "metric").map((l) => [l.k, l]));
    expect(metric.butter?.text).toBe("141 g");
  });
  it("merges grams into cups when cups dominate (or ties) using density", () => {
    expect(lines.flour?.text).toBe("2 cups");
  });
  it("sums counts and pluralizes", () => {
    expect(lines.egg?.text).toBe("3");
    expect(lines.egg?.name).toBe("eggs");
  });
  it("keeps unmeasured items", () => {
    expect(lines["n:salt"]?.text).toBe("pinch");
    expect(lines["n:salt"]?.cat).toBe("other");
  });
  it("converts to metric when asked", () => {
    const m = Object.fromEntries(mergeLines(recipes, table, "metric").map((l) => [l.k, l]));
    expect(m.flour?.text).toBe("250 g");
  });
});

describe("sharing a list", () => {
  const state = {
    recipes: [
      {
        slug: "toast",
        title: "Toast",
        url: "/recipes/toast/",
        yield: "2 servings",
        items: [{ k: "milk", n: "milk", c: "dairy", u: "cup", v: 1 }],
      },
    ],
    checked: ["milk"],
  };
  it("round-trips through a compact link", async () => {
    const code = await encodeList(state);
    expect(code).toMatch(/^z[\w-]+$/);
    expect(await decodeList(code)).toEqual(state);
  });
  it("merges a shared list, replacing recipes already on it", () => {
    const mine = {
      recipes: [
        { ...state.recipes[0], yield: "4 servings" } as ListRecipe,
        { ...state.recipes[0], slug: "soup" } as ListRecipe,
      ],
      checked: ["eggs"],
    };
    const merged = mergeShared(mine, state);
    expect(merged.recipes.map((r) => `${r.slug} ${r.yield}`)).toEqual(["soup 2 servings", "toast 2 servings"]);
    expect(merged.checked).toEqual(["eggs", "milk"]);
  });
  it("rejects links that aren't lists", async () => {
    await expect(decodeList(`j${btoa("{}")}`)).rejects.toThrow();
  });
});
