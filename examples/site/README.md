# Example site

> [!CAUTION]
> These recipes are **examples only**. They exist to demonstrate the
> get-cooked recipe format and features, and they have not been tested in a
> kitchen. Quantities, times and temperatures may be wrong. Please don't
> follow them or cook from them.

This folder is a complete get-cooked site used by the documentation, the
tests and the screenshots:

- `get-cooked.yaml`: site config (English and Spanish)
- `ingredients.yaml`: the ingredient database (also the starter database
  written by `get-cooked init`)
- `recipes/`: one recipe per feature area (scaling, options, variants, ranges,
  divided ingredients, recipe references, image variants)
- `recipes/breakfast/cranberry-baked-oatmeal.es.yaml` and `i18n/es.yaml`:
  Spanish translations

Preview it with `make serve`, or `get-cooked serve -s examples/site`.
