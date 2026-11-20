# get-cooked recipe format

A recipe is one YAML file in `recipes/` (subfolders become categories). Start
each file with the schema line so editors autocomplete and check it:

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/RashKash103/get-cooked/main/schema/recipe.schema.json
```

Run `get-cooked validate` after every change. It checks the schema, every
expression and reference, and renders the recipe across all servings,
options and unit systems.

## Skeleton

```yaml
title: Cranberry Baked Oatmeal          # required
description: One or two sentences.      # shown under the title and on cards
image: oatmeal.jpg                      # next to the recipe file, or a URL
image_alt: Golden oatmeal in a square pan
images:                                 # optional variants for the recipe page
  - when: cranberries                   # first match wins; `image:` stays the
    src: oatmeal-cranberries.jpg        # default on the index and in previews
    alt: Oatmeal studded with cranberries
category: Breakfast                     # default: the parent folder name
tags: [make-ahead, vegetarian]
cuisine: American
author: Sam
source: { name: Grandma, url: https://example.com, credit: "Adapted from {source}" }
date: 2026-10-02                        # newest first on the index
draft: false                            # drafts build only with --drafts

servings: { default: 4, min: 1, max: 12, step: 1, unit: serving, plural: servings }
# or simply: servings: 4
makes: { amount: "1/2", unit: cup }     # what it makes, for recipes that use it (optional)

options: [...]      # reader choices (toggles, choices, numbers)
vars: {...}         # computed values
scaling: "..."      # override the scale factor (optional)
times: {...}        # minutes; numbers or expressions
equipment: [...]
ingredients: [...]  # required
steps: [...]        # required
notes: [...]
ingredient_db: {...} # recipe-local ingredient database entries (optional)
```

## Expressions

Used by `amount`, `times`, `vars`, `when`, `scaling`, `with` and inside text.

- Values: numbers, `true`/`false`, `'text'` or `"text"`.
- Operators: `+ - * / % ^`, `== != < <= > >=`, `&& || !`, `cond ? a : b`.
- Functions: `min max abs round(x[,step]) floor(x[,step]) ceil(x[,step]) clamp(x,lo,hi)
  sqrt pow lerp(a,b,t) if(c,a,b) plural(n, one[, other]) str(x)`.
- Names: `servings`, `base_servings` (the default), `scale` (servings ÷ default,
  or `scaling:`), `metric` (true when the reader chose metric), every option id
  and every var.
- `when:` must be true/false. Comparing a number with text is an error.

Quote expressions that start with a special YAML character or contain `: `:
`amount: "servings > 2 ? 1 : 0.5"`.

## Options

```yaml
options:
  - id: cranberries            # lowercase letters, digits, underscores
    type: toggle               # toggle | choice | number
    label: Add cranberries
    help: Optional one-line help
    default: true
  - id: method
    type: choice
    label: Cook in
    default: oven
    choices:
      - { id: oven, label: Oven }
      - { id: air_fryer, label: Air fryer }
    # choices can also be plain strings: [smooth, chunky]
  - id: heat
    type: number
    label: Spice level
    min: 0
    max: 3
    step: 1
    default: 1
    unit: "%"                  # optional suffix in the UI
  - id: fresh
    type: toggle
    label: Use fresh berries
    when: cranberries          # only shown when another option allows it
```

## Vars and times

```yaml
vars:
  small: servings <= 2
  bake: "small ? 25 : 30 + servings * 2"
times:
  prep: 15
  bake: bake
  rest: 5
  # total defaults to the sum of the others; set it to override
```

Common time keys get translated labels: prep, cook, rest, total, bake, chill,
rise, active, cool, marinate.

## Ingredients

```yaml
ingredients:
  - 2 1/4 cups all-purpose flour, sifted    # shorthand line (parsed)
  - id: sugar
    ingredient: brown-sugar     # key in ingredients.yaml (enables conversions)
    amount: 0.75                # number: amount at default servings, scales
    unit: cup
    note: packed
  - id: eggs
    ingredient: egg
    amount: "max(1, round(2 * scale))"   # string: raw expression, NOT auto-scaled
  - { id: garlic, ingredient: garlic, amount: [2, 3], unit: clove }   # range: "2–3 cloves"
  - id: vanilla
    name: vanilla bean paste    # name without a database entry
    amount: 1
    unit: tsp
    scale: false                # numeric amount that never scales
    fixed_unit: true            # never convert or change the unit
  - id: cranberries
    ingredient: dried-cranberries
    amount: 0.5
    unit: cup
    optional: true              # creates a toggle option with the same id
    default: true
    option_label: Add dried cranberries
  - section: Topping            # group heading
    when: "!plain"              # sections can be conditional
    items:
      - { id: oats, ingredient: rolled-oats, amount: 0.5, unit: cup }
```

Rules:

- Ids are lowercase with underscores. They default from the name or
  database key (`all-purpose-flour` → `all_purpose_flour`).
- Two items may share an id when their `when:` conditions never overlap
  (variants): `{id: fat, ingredient: butter, when: "!vegan"}` and
  `{id: fat, ingredient: olive-oil, when: vegan}`. Steps say `@fat` either way.
- An amount that evaluates to 0 hides the ingredient.
- Any ingredient database setting can be overridden inline for one recipe
  (or given to an ingredient without a database entry): `measure`, `density`,
  `grams_each`, `unit_grams`, `round`, `category`, plus `name`/`plural`.
  For example `{ id: flour, ingredient: all-purpose-flour, amount: 2, unit: cup, measure: mass }`
  always shows the flour by weight, and
  `{ id: berries, name: wild blueberries, amount: 1, unit: cup, density: { value: 140, unit: g/cup } }`
  lets an ad-hoc ingredient convert to grams.
- Ranges (`amount: [low, high]`, numbers or expressions) scale and convert
  like single amounts; shorthand lines such as `2-3 cloves garlic` become
  ranges too. The shopping list buys the high end.
- Units: tsp, tbsp, cup, fl oz, pint, quart, gallon, ml, l, g, kg, oz, lb,
  plus count units (clove, can, slice, pinch, dash, stick, sprig, bunch, …).
  Leave the unit out for counted items ("2 eggs"). Declare other count units
  under `units.custom` in get-cooked.yaml.
- Commas inside `{ … }` separate fields, so quote notes that contain commas:
  `note: "melted, plus more"`.

### Divided ingredients

When one ingredient is used in several steps, list its total once and give
each step its share:

```yaml
  - id: salt
    ingredient: kosher-salt
    unit: tsp
    split: { dough: 4, topping: 2 }   # total 6 tsp; each part scales
steps:
  - Mix the flour with @salt.dough.
  - Sprinkle @salt.topping over the top.
```

Portions like `@sugar{1/2}` used in two or more steps show the same breakdown
under the ingredient ("¾ cup for step 2 · ¾ cup for step 4").

### Substitutes

`subs:` lists what the reader can use instead. The ingredient gets a **Swap**
button, and a choice option named `swap_<id>` appears in Customize; steps,
scaling, unit conversion, the shopping list and exports all follow the pick.

```yaml
  - id: dairy
    ingredient: buttermilk
    amount: 2
    unit: cup
    subs:
      - kefir                                   # same amount and unit
      - { ingredient: plain-yogurt, ratio: 0.75, note: thinned with water }
      - 1 3/4 cups whole milk                   # an amount of its own
      # Several ingredients together: a list, or { label:, items: [...] }.
      - [{ ingredient: milk, ratio: 0.875 }, { ingredient: lemon-juice, amount: 2, unit: tbsp }]
steps:
  - Whisk in @dairy{if swap_dairy == 'milk_lemon_juice'}, then rest 5 minutes{end}.
```

- A substitute without an amount uses the original's amount and unit;
  `ratio:` multiplies it. Each one can be shorthand text or a mapping with
  any ingredient field (`note`, `measure`, `density`, …), except `id`,
  `when`, `optional`, `recipe` and `split`.
- A **combination** replaces one ingredient with several. Its first part takes
  the ingredient's id; the others get ids like `dairy_lemon_juice` (or the
  `id:` you give them) and their own lines in the list, and they only inherit
  an amount when they give a `ratio:`. Wherever the text says `@dairy`, every
  part is named: "1 ¾ cups milk and 2 tbsp lemon juice". Portions
  (`@dairy{1/2}`) apply to each part.
- Choice ids come from the ingredient keys or names, joined with `_` for a
  combination (`buttermilk`, `kefir`, `plain_yogurt`, `milk_lemon_juice`), so
  text can depend on the pick with `{if swap_dairy == 'kefir'}…{end}`.
- The swap is offered only while the ingredient is in the recipe (its
  `when:` holds and, for an optional one, it's ticked).

### Recipe references (prerequisites)

```yaml
  - id: crust
    recipe: pie-crust            # slug of another recipe
    amount: "servings / 8"       # in that recipe's servings unit (e.g. crusts)
    note: chilled
    with: { butter_type: "vegan ? 'vegan' : 'dairy'" }   # set its options
```

The ingredient links to the other recipe, opened with enough servings
(rounded up to what it can make) and the `with:` options applied.

### Recipes that make an ingredient

A spice blend, sauce or stock can say what it makes, in a real unit, at its
default servings:

```yaml
# garam-masala.yaml
servings: { default: 1, unit: batch, plural: batches }
makes:
  ingredient: garam-masala         # optional: the ingredient database entry it makes
  amount: "1/2"                    # at the default servings; "1 1/2" works too
  unit: cup
  density: { value: 100, unit: g/cup }   # any ingredient fields: density, measure, …
```

Other recipes can then use it by measure, with conversions and scaling like
any ingredient, and a link to the recipe opened with enough batches:

```yaml
  - { recipe: garam-masala, amount: 2, unit: tsp }   # explicit reference
  - { ingredient: garam-masala, amount: 2, unit: tsp } # or just the ingredient…
  - 2 tsp garam masala                                 # …in any form
```

- With `ingredient:`, every recipe that uses that ingredient links to this
  recipe automatically, and `makes:` starts from its database entry (its
  density, category, …); fields in `makes:` override them. The shopping
  list merges it with the same ingredient from other recipes.
- Without `ingredient:`, use `recipe:` with a `unit:`. The name defaults to
  the recipe's title (or `makes.name`).
- A `recipe:` without `unit:` still counts in the recipe's own servings unit.
- The recipe's page shows "Makes ½ cup", scaled with its servings.
- If the unit can't be converted (teaspoons to grams with no density, say),
  validation says so: add a `density:` to `makes:` or the database entry.

## Source

Credit where a recipe came from. Every field is optional, but give at least a
name, a URL or a PDF.

```yaml
source:
  name: Salt, Fat, Acid, Heat       # what to call it (default: the URL or PDF file name)
  url: https://example.com/recipe   # a web page
  pdf: /books/cookbook.pdf          # a PDF in the site, opened in the built-in viewer
  page: 112                         # the page the recipe is on
  credit: "Modified from {source}"  # the line shown instead of "Source: …"
```

- `pdf:` is a path next to the recipe file, or from the site folder with a
  leading `/` (handy for a `books/` folder that several recipes share). The
  build publishes each PDF once, and the recipe links to the site's own PDF
  viewer, opened at `page:`. It works on phones (where browsers often can't
  show PDFs inline), offline once opened, and scanned books are fine. `page:`
  counts from the first page of the file, which may differ from the number
  printed on it.
- Without `pdf:`, `page:` is just shown: "Grandma's notebook, p. 12".
- `credit:` is a sentence around the linked name: `{source}` marks where it
  goes ("Adapted from {source}, with less sugar."); without it, the name
  follows the text ("Modified from" → "Modified from Salt, Fat, Acid, Heat").
  The page follows the name unless you place it with `{page}`. The credit is
  translated like the rest of the recipe.
- With both `url:` and `pdf:`, the name links to the web page and the page
  ("p. 112", or "PDF") to the viewer.

## Equipment

```yaml
equipment:
  - { id: pan, name: "{if servings <= 2}8×8 inch pan{else}13×9 inch pan{end}" }
  - { id: mixer, name: stand mixer, note: or a hand mixer, when: "!by_hand" }
  - large bowl                   # shorthand; id "large_bowl"
```

Variants with the same id and non-overlapping `when:` work like ingredients.

## Steps, notes and text

Steps, notes, equipment names and ingredient notes are **text templates**:

| Syntax | Meaning |
|---|---|
| `{expr}` | value of an expression |
| `{expr \| fmt}` | formatted: `frac`, `int`, `duration` (minutes), `temp` (°F in), `temp_c` (°C in), `len` (inches in), `len_cm`, `qty:<unit>` (e.g. `{0.5 * scale \| qty:cup}`) |
| `{if expr}…{elif expr}…{else}…{end}` | conditional text (nestable) |
| `@id` | ingredient with its current amount: "2 cups flour" |
| `@id{expr}` | a portion: `@sugar{1/2}` |
| `@id.part` | a named `split:` portion |
| `@id{-}` | the name only |
| `&id` | equipment name (current variant) |
| `~{minutes}` | a tappable timer, e.g. `~{bake}` or `~{2.5}` |
| `~{lo, hi}` | a time range shown as "4–5 min", e.g. `~{4, 5}` or `~{bake, bake + 5}`. Tapping it opens a picker with the start, 25%, 50%, 75% and end times, plus a slider for anything in between |
| `\{ \} \@ \& \~ \\` | literal characters |

```yaml
steps:
  - Heat the oven to {350 | temp}. Grease the &pan.
  - text: Whisk @flour, @sugar{1/2} and @salt{if cranberries}, then fold in @cranberries{end}.
    note: Don't overmix.
    image: step-2.jpg
  - when: cranberries                # whole step added by an option
    text: Scatter the rest of @cranberries{-} on top.
  - section: Frosting                # grouped steps
    steps:
      - Beat @butter{-} with the remaining @sugar{1/2}.
  - Bake ~{bake}.
notes:
  - when: servings == 1
    kind: warning                     # warning (shown near the top) | tip | info
    text: A single serving may come out softer.
  - Store airtight for 5 days.        # plain note
```

Never write ` #` in unquoted YAML text: YAML treats it as a comment and drops the
rest of the line (`validate` warns about this).

## Translations

If `get-cooked.yaml` lists several `languages:`, each recipe can have a
translation file next to it, `<recipe>.<lang>.yaml`, mapping original text to
translated text:

```yaml
strings:
  Cranberry Baked Oatmeal: Avena horneada con arándanos
  "Heat the oven to {375 | temp} and butter the &dish.": "Precalienta el horno a {375 | temp} y engrasa la &dish."
```

Numbers are written in the language's digits when its `i18n/<lang>.yaml`
sets `digits:` (for example `arab` or `deva`), so keep numbers in templates
as expressions or formatters (`{servings}`, `{350 | temp}`) rather than typed
digits when they should be localized.

Generate or refresh it with `get-cooked i18n extract --lang <lang>`. Empty
values fall back to the original. Keep `{…}`, `@…`, `&…` and `~{…}` intact.
Put display text in `{if}` blocks rather than inside expression strings
(`'8×8 pan'`), because text inside expressions isn't translated.

## Ingredient database (ingredients.yaml)

```yaml
all-purpose-flour:
  name: all-purpose flour
  aliases: [flour, ap flour]     # auto-matching for shorthand lines
  density: { value: 125, unit: g/cup }   # enables cups ⇄ grams
  category: baking
egg:
  name: egg
  plural: eggs
  grams_each: 50
  round: { step: 1, min: 1 }     # whole eggs, at least one
butter:
  density: { value: 227, unit: g/cup }
  unit_grams: { stick: 113 }     # weight of count units
milk:
  density: { value: 1.035, unit: g/ml }  # units: g/ml g/cm3 g/l kg/l kg/m3 g/tsp g/tbsp g/cup
                                         # oz/tbsp oz/cup oz/floz oz/pint lb/cup lb/pint lb/quart lb/gal
  measure: volume                # always show by volume (liquids); `mass` always by weight
```
