# Status / resume notes (2026-11-18)

Design: `docs/design.md`. Everything below is relative to that plan.

## Done and tested
- `internal/expr`: parser, checks, evaluator, JSON IR, fuzz test, vectors → `testdata/conformance/expr-vectors.json`
- `internal/tmpl`: text templates. Equipment refs use `&id`, **not** `#id`, because YAML treats ` #` as a comment. Compile warns when a plain scalar has a trailing comment.
- `internal/units`: table, ladders, compound quantities ("¾ cup + 2 tbsp"), metric tiers, temp/length/duration
- `internal/ingline`: free-text ingredient line parser (shorthand ingredients + future importer)
- `internal/config`, `internal/recipe` (schema validation with line numbers, compiler, ingredient DB, `Link` for recipe references), `internal/runtime` (Go renderer + validation sweep)
- Recipe references: ingredient `recipe: <slug>`, `amount` in the target's servings unit, `with: {opt: expr}`; renders `link` on IngM/Seg
- `schema/*.schema.json` (embedded)
- Example site: `examples/site` (7 recipes + ingredients.yaml). `get-cooked validate -s examples/site` passes.
- Conformance goldens: `go test ./internal/runtime -update` writes `testdata/conformance/*.json`; `cd web && npx vitest run` replays them through the TS runtime (all passing)
- TS runtime: `web/src/runtime/{types,expr,units,render}.ts`
- CLI: `render`, `validate`, `version` (others are stubs in `internal/cli/stubs.go`)

## Written, not yet compiled or tested
- `internal/site/{build,blocks,images,jsonld}.go` + `templates/*.html` (Go side compiles; never run end to end)
- `web/src/styles.css`, `web/src/ui/{blocks,storage,toast,theme,native,wakelock,timers,recipe}.ts`

## Next steps (in order)
1. Move `<div id="timers">` from recipe.html into base.html; add `data-timer-intent` / `data-ios-shortcut` attributes to `<html>` from `cfg.Timers`.
2. Write `web/src/ui/index.ts` (search + category chips, `?q=`/`?c=` sync, `/` focuses search), `web/src/main.ts` (imports styles.css, initTheme, initTimers, initRecipe/initIndex, SW registration), `web/src/sw.ts` (uses `__GC_VERSION__`, `__GC_BASE__`, `__GC_PRECACHE__` placeholders; cache-first assets, network-first pages), and `web/public/icon.svg`.
3. `pnpm build` (outputs to `internal/assets/dist`), then implement the `build` CLI command (`site.Load` + `site.Build`) and build `examples/site` → **show the user a preview** (they asked; e.g. publish the built site as an Artifact or serve it locally).
4. Sample images: SVG illustrations for each recipe (`image:` fields point to `*.svg` that don't exist yet).
5. Blocks parity test: Go writes golden HTML of blocks for the conformance cases; vitest compares against `blocks.ts`.
6. Remaining CLI: `serve` (rebuild on change + SSE live reload at `/__livereload`), `init` (scaffold + install Claude skill), `new`, `import <url>` (JSON-LD → YAML via ingline), `schema`.
7. Claude skill `skills/import-recipe/SKILL.md` (embedded; installed by `init`).
8. Docs: `docs/recipe-format.md`, README, consumer workflow example.
9. CI: `.github/workflows/{ci,release}.yml`, `.goreleaser.yaml`, `action.yml`, `.golangci.yml`, dependabot, Makefile.
10. E2E: `e2e/` Playwright (mobile + desktop, dark, axe), plus site/CLI/compile-error Go tests.
