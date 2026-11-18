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
- Shopping list, saved recipes, personal notes, scale-to-what-you-have,
  cook-mode step navigation, Atom feed, ranges, `validate --strict`.
- Export: browser print/PDF layout, Markdown and text; CLI `export` to
  md/txt/typ/pdf (PDF via the typst CLI).
- CI (lint, race tests, golden drift, fuzzing, example validation,
  Playwright e2e, action smoke test), GoReleaser release with the skills
  asset, composite GitHub Action, Dependabot.

## Before the first release

- Push to GitHub and tag `v0.1.0`. The release workflow publishes binaries,
  checksums and `get-cooked-skills.tar.gz`, and moves the `v0` tag.
- Choose a license (none is included yet).

## Ideas

- Static PDFs per recipe at build time (needs typst in the build).
- Share the shopping list as a link (encode it in the URL).

- Nutrition per serving from the ingredient database.
- WebP image variants.
- Native wrapper app using the `window.GetCookedNative` bridge.
- `i18n sync` to add newly introduced UI keys to existing catalogs.
