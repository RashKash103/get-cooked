# Status (2026-11-18)

Repository: https://github.com/RashKash103/get-cooked (MIT).

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

## Releases

- v0.1.0 published 2026-11-18 (binaries for linux/darwin/windows × amd64/arm64,
  checksums, `get-cooked-skills.tar.gz`; floating `v0` tag for the Action).
- v0.1.1: installable with `go install …@latest` (bundle committed).
- v0.2.0: per-language digits.
- v0.3.0 (2026-11-19): structured densities (breaking: run `get-cooked migrate`),
  native/diagonal fractions, fix for the display order of non-Latin numbers,
  `get-cooked migrate`.
- v0.4.0 (2026-11-19): inline ingredient overrides, `measure` as a display
  preference in every unit system, "All recipes" control fix.
- v0.5.0 (2026-11-19): "Aa" display menu (theme + text size), migration that
  adds missing i18n strings.
- v0.5.1 (2026-11-19): full PWA (PNG/maskable/Apple icons, complete manifest,
  `icon:` option, offline page), nginx/Docker docs.
- v0.6.0 (2026-11-19): time ranges `~{4, 5}` with a picker, migration
  `0.6.0-timer-ranges`; Start over; In progress badge + filter; swipe in cook
  mode; collapsed timer stack + tap a timer to go to its step; tap a step
  ingredient to tick it; Share list links (#l= deflate-raw base64url);
  conversions sheet on tapping an amount (replaces the scale prompt; accepts
  localized digits); `subs:` substitutes incl. combinations (compile-time
  expansion of @id into {if}, extra parts get ids like dairy_lemon_juice);
  fixed 150% text overflow from `.step` negative margins.
- To release: tag `vX.Y.Z` and push the tag; the Release workflow does the rest.

## Ideas

- Static PDFs per recipe at build time (needs typst in the build).
- Share the shopping list as a link (encode it in the URL).
- Nutrition per serving from the ingredient database.
- WebP image variants.
- Native wrapper app using the `window.GetCookedNative` bridge.
