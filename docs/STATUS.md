# Status (2026-11-18)

Everything in `docs/design.md` is implemented, tested and committed:

- Recipe language: expressions, options, vars, variants, templates, divided
  ingredients, recipe references, validation sweep.
- Units, conversion, rounding ladders and compound quantities.
- Go and TypeScript runtimes with golden parity tests (models and HTML, in
  English and Spanish).
- Site builder: multi-language, PWA, JSON-LD, sitemap, responsive images,
  live-reload dev server.
- CLI: init, new, serve, validate, build, render, import, i18n
  (init/extract/status), skills (install/list), docs, schema, version.
- Claude skills: import-recipe, recipe-illustration, add-site-language,
  translate-recipe.
- CI (lint, race tests, golden drift, fuzzing, example validation,
  Playwright e2e, action smoke test), GoReleaser release with the skills
  asset, composite GitHub Action, Dependabot.

## Before the first release

- Push to GitHub and tag `v0.1.0`. The release workflow publishes binaries,
  checksums and `get-cooked-skills.tar.gz`, and moves the `v0` tag.
- Choose a license (none is included yet).

## Ideas

- Nutrition per serving from the ingredient database.
- Multi-recipe shopping list (aggregate by ingredient key, convert units).
- WebP image variants.
- Quantity ranges ("1–2 cloves").
- Native wrapper app using the `window.GetCookedNative` bridge.
- `i18n sync` to add newly introduced UI keys to existing catalogs.
