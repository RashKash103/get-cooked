---
name: translate-recipe
description: Translate one or more get-cooked recipes into another configured site language by filling in the recipe's <name>.<lang>.yaml translation file, keeping expressions and references intact. Use when the user asks to translate a recipe, or to update translations after a recipe changed.
---

# Translate a get-cooked recipe

Recipe translations live next to the recipe as `<recipe>.<lang>.yaml`. They
map each original text to its translation, so the recipe's logic (amounts,
options, scaling) stays in one place:

```yaml
strings:
  Cranberry Baked Oatmeal: Avena horneada con arándanos
  "Heat the oven to {375 | temp} and butter the &dish.": "Precalienta el horno a {375 | temp} y engrasa la &dish."
```

Empty values fall back to the original language, so partial work is safe.

## Steps

1. Check `get-cooked.yaml` lists the language under `languages:`. If not, use
   the `add-site-language` skill first (UI strings, units and ingredient names
   come from `i18n/<lang>.yaml`, not from recipe files).
2. Run `get-cooked i18n extract --lang <lang> [recipes/…/recipe.yaml]`. It
   creates or updates the translation file(s) with every string of the
   recipe, keeping existing translations.
3. Read the original recipe YAML to understand context (what `@id` refers to,
   which option a conditional belongs to), then translate each empty value:
   - Keep everything inside `{…}` exactly: expressions, formatters
     (`{375 | temp}`), and conditionals (`{if cranberries}…{end}`). You may
     translate the text *between* the tags and move whole tags to fit the
     grammar, but each `{if …}` needs its matching `{else}`/`{end}`.
   - Keep references exactly: `@id`, `@id{1/2}`, `@id.part`, `@id{-}`, `&id`,
     `~{minutes}` (or `~{lo, hi}` for a range). They render the ingredient amount and name, equipment or a
     timer, already in the target language, so don't add the ingredient name
     next to them. If the language puts a connector between amount and name,
     the site config handles it.
   - Translate tags and the category too (they appear in the index).
   - Use natural kitchen language for the target audience, short imperative
     steps, the same tone as the original. Translate meaning, not word order.
   - Leave a value empty only if it should stay in the original language.
4. Run `get-cooked validate`. It reports broken references or expressions in
   translations, untranslated strings and stale entries.
5. Render it to read it as a cook would:
   `get-cooked render <recipe.yaml> --lang <lang> --servings <n>` (try the
   extremes and an option or two).
6. When the original recipe changes later, run `extract` again: new strings
   appear empty and stale ones are listed at the end. Remove them with
   `--prune` once handled.
