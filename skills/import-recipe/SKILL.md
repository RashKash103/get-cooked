---
name: import-recipe
description: Convert a recipe from a URL, pasted text or a photo into a get-cooked YAML recipe file, with servings scaling, smart options and ingredient database entries. Use when the user wants to add, import, port or convert a recipe into their get-cooked site.
---

# Import a recipe into get-cooked

You are adding a recipe to a get-cooked site (a folder with `get-cooked.yaml`,
`ingredients.yaml` and `recipes/`). The result must pass `get-cooked validate`
and read like a carefully written recipe, not a scraped one.

## 1. Learn the format

Run `get-cooked docs format` and read it fully before writing YAML. It covers
expressions, options, variants, divided ingredients, text templates and
references. Re-check it whenever unsure. Never invent syntax.

## 2. Get the source

- **URL**: run `get-cooked import <url> > /tmp/draft.yaml`. It extracts the
  schema.org Recipe data (title, times, servings, ingredient lines, steps)
  into a rough draft. If it fails (no structured data), fetch the page and
  read the recipe yourself.
- **Text or photo**: transcribe it faithfully first.
- **PDF in the site** (e.g. a scanned cookbook in `books/`): read the page,
  transcribe it, and point the source at it with `pdf:` and `page:` so the
  recipe links to that page in the built-in viewer.

Keep the author's quantities and method. Don't "improve" the recipe. Record
the origin in `source:` (`name`, `url`, or `pdf` + `page`). If you changed
anything, say so with `credit:`, e.g. `credit: "Adapted from {source}"`.

## 3. Write the recipe

Create `recipes/<category>/<slug>.yaml` (lowercase-hyphenated slug from the
title, category folder matching existing ones; look at `recipes/`).

1. **Metadata**: title, one or two sentence description in the site's voice,
   `category`, a few `tags`, `date` (today), `source`.
2. **Servings**: the recipe's yield as `default`. Pick a sensible `min`/`max`/
   `step` (for example cookies: step 6 or 12; a loaf: step 1, unit `loaf`,
   plural `loaves`). Set `unit` when the yield isn't "servings".
3. **Ingredients**: one item per ingredient with an `id` short enough to read
   in steps (`flour`, `sugar`, `butter`). Prefer `ingredient:` keys from
   `ingredients.yaml`. If an ingredient is missing there, add an entry with
   `name`, `aliases`, `category`, and a realistic density (`density: { value: 125, unit: g/cup }`;
   other units like `g/ml`, `g/cm3` or `oz/cup` are listed in the schema) (or
   `grams_each`, `unit_grams`, `measure: volume` for liquids, `round` for
   counted items like eggs). Use well-known densities; don't guess wildly.
   Use sections when the source groups ingredients.
4. **Steps**: rewrite into clear, short steps that reference ingredients with
   `@id` (amount + name) the first time each is used, `@id{-}` for later
   mentions, equipment with `&id`, temperatures with `{350 | temp}` (or
   `temp_c` if written in Celsius), and every time with a timer `~{minutes}`. Write a range like "4 to 5 minutes"
   as one timer, `~{4, 5}`, not `{4}–~{5}`.
   If an ingredient is used in parts across steps, use `split:` with
   `@id.part`, or portions like `@sugar{1/2}`.
5. **Think about scaling**: this is what makes get-cooked worth it. Look for:
   - Pan or pot size that should change with the batch (equipment variants or
     `{if}` text).
   - Bake/cook times that grow with batch depth (`vars:` with an expression).
   - Counted items that must stay whole (eggs: use a DB entry with `round`).
   - Things that don't scale linearly (a pinch of salt, one bay leaf: use
     `scale: false` or an expression).
   - Warnings at the extremes (`notes:` with `when: servings == 1`).
6. **Options**: add 1–3 genuinely useful options the source suggests
   (substitutions, optional add-ins with `optional: true`, method choices like
   oven vs. air fryer, spice level). Each option must change ingredients
   and/or steps. Don't add options for their own sake.
   When the source says "or use X" for an ingredient, write it as `subs:` on
   that ingredient (`subs: [kefir]`, `{ ingredient: x, ratio: 0.75 }`, or a
   combination like `[{ ingredient: milk, ratio: 0.875 }, 2 tbsp lemon juice]`)
   instead of a separate option; see the recipe format's "Substitutes".
7. **Notes**: storage, make-ahead or serving tips from the source, kept short.
8. Put any text that readers see inside templates (`{if}…{end}`), not inside
   expression strings, so it can be translated.

## 4. Verify

```sh
get-cooked validate
get-cooked render recipes/<category>/<slug>.yaml
get-cooked render recipes/<category>/<slug>.yaml --servings <min> --units metric
get-cooked render recipes/<category>/<slug>.yaml --servings <max> --set <option>=<value>
```

Read the rendered output as a cook would. Check that amounts round sensibly,
steps make sense at the extremes, and every option visibly changes something.
Fix and re-run until `validate` is clean (warnings included).

## 5. Finish

- If the site has several `languages:`, mention that `translate-recipe` can
  translate the new recipe.
- Offer to draw an illustration with the `recipe-illustration` skill if the
  recipe has no image.
- Summarize what you added: the file, any new ingredient database entries,
  and the scaling logic and options you chose.
