import { afterEach, describe, expect, it } from "vitest";
import { asciiDigits, setDict } from "./i18n";

describe("asciiDigits", () => {
  afterEach(() => setDict({}));
  it("reads amounts typed in the page language's digits", () => {
    setDict({ _digits: "০১২৩৪৫৬৭৮৯" });
    expect(asciiDigits("১ ১/২")).toBe("1 1/2");
    expect(asciiDigits("৯০০")).toBe("900");
  });
  it("leaves Latin digits alone", () => {
    setDict({});
    expect(asciiDigits("1.5")).toBe("1.5");
  });
});
