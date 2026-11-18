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
category: Breakfast                     # default: the parent folder name
tags: [make-ahead, vegetarian]
cuisine: American
author: Sam
source: { name: Grandma, url: https://example.com }
date: 2026-10-02                        # newest first on the index
draft: false                            # drafts build only with --drafts

servings: { default: 4, min: 1, max: 12, step: 1, unit: serving, plural: servings }
# or simply: servings: 4

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

Generate or refresh it with `get-cooked i18n extract --lang <lang>`. Empty
values fall back to the original. Keep `{…}`, `@…`, `&…` and `~{…}` intact.
Put display text in `{if}` blocks rather than inside expression strings
(`'8×8 pan'`), because text inside expressions isn't translated.

## Ingredient database (ingredients.yaml)

```yaml
all-purpose-flour:
  name: all-purpose flour
  aliases: [flour, ap flour]     # auto-matching for shorthand lines
  grams_per_cup: 125             # density: enables cups ⇄ grams
  category: baking
egg:
  name: egg
  plural: eggs
  grams_each: 50
  round: { step: 1, min: 1 }     # whole eggs, at least one
butter:
  grams_per_cup: 227
  unit_grams: { stick: 113 }     # weight of count units
milk:
  grams_per_cup: 245
  measure: volume                # liquids stay in ml when converting to metric
```
