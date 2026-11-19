import { describe, expect, it } from "vitest";
import { nextTextSize, TEXT_SIZES } from "./theme";

describe("text size steps", () => {
  it("steps up and down through the sizes and stops at the ends", () => {
    expect(nextTextSize(100, 1)).toBe(112.5);
    expect(nextTextSize(112.5, -1)).toBe(100);
    expect(nextTextSize(150, 1)).toBe(150);
    expect(nextTextSize(87.5, -1)).toBe(87.5);
    expect(nextTextSize(125, 0)).toBe(100);
  });
  it("snaps an unusual stored size to the next step", () => {
    expect(nextTextSize(105, 1)).toBe(112.5);
    expect(nextTextSize(105, -1)).toBe(100);
  });
  it("covers the documented range", () => {
    expect(TEXT_SIZES[0]).toBe(87.5);
    expect(TEXT_SIZES.at(-1)).toBe(150);
  });
});
