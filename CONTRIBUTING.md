# Contributing

Thanks for your interest! This is a personal project that I maintain in my
spare time. Forks are welcome, and pull requests are welcome too: I'll review
and merge them as I can, but I can't promise a timeline or that every change
will fit the project.

## Before you start

- For anything larger than a small fix, open an issue first to discuss it.
- Bug reports are most useful with a minimal recipe YAML that reproduces the
  problem and the output of `get-cooked validate` or `get-cooked render`.

## Development setup

You need Go (see `go.mod`), Node 24+ and pnpm.

```sh
make deps     # install frontend dependencies
make build    # build the frontend bundle and bin/get-cooked
make test     # Go tests (race) + TypeScript unit and conformance tests
make lint     # golangci-lint + biome
make e2e      # Playwright tests against the real binary
make serve    # preview the example site with live reload
```

## How the code fits together

Read [docs/design.md](docs/design.md) first. The most important rule: recipes
are evaluated by **two runtimes**, Go (`internal/runtime`, used for server
rendering, validation and the CLI) and TypeScript (`web/src/runtime`, used in
the browser), and the HTML blocks and Markdown export also exist in both
languages. Any change to evaluation, units, rounding, markup or export must be
made on both sides.

Golden files keep them honest:

```sh
make golden   # regenerate testdata/conformance after an intentional change
make test     # the TypeScript tests replay the goldens and must match exactly
```

CI fails if the goldens are out of date.

The built frontend bundle in `internal/assets/dist` is committed so that
`go install github.com/RashKash103/get-cooked/cmd/get-cooked@latest` works.
After changing anything under `web/`, run `make web` and commit the result;
CI checks that it's up to date.

## Format changes need a migration

Any change that makes existing site files invalid (renamed or removed fields,
new required structure) must ship with a migration in `internal/migrate` so
users can run `get-cooked migrate` instead of editing files by hand:

- Add an entry to `migrate.All` with an id, the version it ships in, a
  description and an `Apply` function that edits text in place (keep
  comments and formatting) and is safe to run repeatedly.
- Make the validation error for the old form say "run `get-cooked migrate`"
  (see `removedFields` in `internal/recipe/schema.go`).
- New settings that `get-cooked i18n init` writes into catalogs get a
  migration too (`migrate.AddTemplateKeys`), adding them with their default
  values only where a catalog doesn't define them.
- Add a test in `internal/migrate` that migrates an old-format site and
  checks that it then validates and that a second run changes nothing.

## Example recipes

The recipes in `examples/site` and the starter recipe written by
`get-cooked init` are untested examples for demonstrating features. Keep the
"Example only" warning note on every example recipe, and don't present them
as real, tested recipes.

## Pull requests

- Keep changes focused; include tests (Go unit tests, a conformance case, or
  an e2e test for UI behaviour).
- Update [docs/recipe-format.md](docs/recipe-format.md) when you change the
  recipe language, and the JSON Schemas in `schema/` when you change a file
  format.
- New UI strings go in `internal/i18n/en.yaml` (and the Spanish example in
  `examples/site/i18n/es.yaml` if you can).
- Make sure `make lint test e2e` passes.

By contributing you agree that your contributions are licensed under the
[MIT License](LICENSE).
