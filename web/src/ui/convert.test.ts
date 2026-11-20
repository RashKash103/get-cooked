import { describe, expect, it } from "vitest";
import type { Ing, Table } from "../runtime/types";
import { Units } from "../runtime/units";
import { conversions } from "./convert";

const table: Table = {
  units: {
    cup: { id: "cup", dim: "volume", sys: "us", f: 236.5882365, one: "cup", other: "cups" },
    ml: { id: "ml", dim: "volume", sys: "metric", f: 1, one: "ml", other: "ml" },
    l: { id: "l", dim: "volume", sys: "metric", f: 1000, one: "l", other: "l" },
    g: { id: "g", dim: "mass", sys: "metric", f: 1, one: "g", other: "g" },
    kg: { id: "kg", dim: "mass", sys: "metric", f: 1000, one: "kg", other: "kg" },
    oz: { id: "oz", dim: "mass", sys: "us", f: 28.349523125, one: "oz", other: "oz" },
    lb: { id: "lb", dim: "mass", sys: "us", f: 453.59237, one: "lb", other: "lb" },
  },
  ladders: {
    "us.volume": [{ unit: "cup", denoms: [2, 3, 4], min: 0.25 }],
    "us.mass": [
      { unit: "oz", denoms: [2, 4] },
      { unit: "lb", denoms: [2, 4], min: 1 },
    ],
    "metric.mass": [
      { unit: "g", tiers: [{ below: 10, step: 0.5 }, { below: 1000, step: 1 }, { step: 5 }] },
      { unit: "kg", tiers: [{ step: 0.05 }], min: 1 },
    ],
    "metric.volume": [
      { unit: "ml", tiers: [{ below: 10, step: 0.5 }, { below: 1000, step: 1 }, { step: 5 }] },
      { unit: "l", tiers: [{ step: 0.05 }], min: 1 },
    ],
  },
  tol: 0.1,
  fractions: "unicode",
  count: { denoms: [2, 4] },
  fallback: { denoms: [2, 3, 4, 8] },
};

const chips: Ing = { name: "chocolate chips", gpml: 170 / 236.5882365 };

describe("ranges in two units", () => {
  it("keep both numbers in the high end's unit", () => {
    const q = new Units(table).formatRange(510, 1000, "g", "metric", chips, false);
    expect(q.t).toBe("510 g – 1 kg");
    expect([q.v, q.h, q.id]).toEqual([0.51, 1, "kg"]);
  });

  it("convert to other units correctly", () => {
    const q = new Units(table).formatRange(510, 1000, "g", "metric", chips, false);
    const texts = conversions(q, chips, table).map((c) => `${c.sys} ${c.dim}: ${c.text}`);
    expect(texts).toContain("us volume: 3–6 cups");
    expect(texts.join("\n")).not.toMatch(/\d{3,}.*cups|510–1 kg/);
  });
});
