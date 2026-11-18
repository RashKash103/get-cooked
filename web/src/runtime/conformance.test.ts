// Replays golden output produced by the Go runtime (go test ./internal/... -update)
// through the TypeScript runtime. Any divergence between the two is a bug.
import { readdirSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { evaluate } from "./expr";
import { render } from "./render";
import type { Compiled, IR, Model, State, Value } from "./types";

const dir = join(import.meta.dirname, "../../../testdata/conformance");

describe("expression vectors", () => {
  const { env, vectors } = JSON.parse(readFileSync(join(dir, "expr-vectors.json"), "utf8")) as {
    env: Record<string, Value>;
    vectors: { src: string; ir: IR; want?: Value; error?: string }[];
  };
  for (const v of vectors) {
    it(v.src, () => {
      if (v.error) {
        expect(() => evaluate(v.ir, env)).toThrow(v.error);
      } else {
        expect(evaluate(v.ir, env)).toEqual(v.want);
      }
    });
  }
});

const files = readdirSync(dir).filter((f) => f.endsWith(".json") && f !== "expr-vectors.json");

describe.each(files)("recipe %s", (file) => {
  const golden = JSON.parse(readFileSync(join(dir, file), "utf8")) as {
    compiled: Compiled;
    cases: { state: State; model: Model }[];
  };
  it.each(golden.cases.map((c, i) => [i, c] as const))("case %i matches Go", (_i, c) => {
    expect(render(golden.compiled, c.state)).toEqual(c.model);
  });
});
