// Replays testdata/conformance/blocks/blocks.json (written by
// `go test ./internal/site -update`): the TypeScript runtime + blocks must
// produce exactly the HTML the Go site builder serves, in every language.
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { render } from "../runtime/render";
import type { Compiled, State } from "../runtime/types";
import { fr, renderEquipment, renderIngredients, renderNotes, renderSteps, renderTimes } from "./blocks";
import { type Meta, toMarkdown } from "./export";
import { setDict } from "./i18n";

interface Case {
  lang: string;
  slug: string;
  base: string;
  dict: Record<string, string>;
  compiled: Compiled;
  state: State;
  meta: Meta;
  html: Record<string, string>;
}

const cases = JSON.parse(
  readFileSync(join(import.meta.dirname, "../../../testdata/conformance/blocks/blocks.json"), "utf8"),
) as Case[];

describe("blocks match Go", () => {
  it.each(cases.map((c, i) => [`${c.lang} ${c.slug} #${i}`, c] as const))("%s", (_name, c) => {
    setDict(c.dict);
    const m = render(c.compiled, c.state);
    const opts = Object.fromEntries((c.compiled.options ?? []).map((o) => [o.id, o.default]));
    Object.assign(opts, c.state.options ?? {});
    expect({
      times: fr(renderTimes(m)),
      equipment: fr(renderEquipment(m)),
      ingredients: fr(renderIngredients(m, c.base)),
      steps: fr(renderSteps(m, c.base)),
      warnings: fr(renderNotes(m, true)),
      tips: fr(renderNotes(m, false)),
      markdown: toMarkdown(c.meta, c.compiled, m, opts),
    }).toEqual(c.html);
  });
});
