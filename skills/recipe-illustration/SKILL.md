---
name: recipe-illustration
description: Draw a flat, top-down SVG illustration of a get-cooked recipe's finished dish in the site's house style (pastel background, simple plate or pan, a few flat shapes). Use when a recipe needs an image, or the user asks for a recipe picture, icon, cover or illustration.
---

# Recipe illustration (house style)

Every illustration on the site follows the same recipe so the index looks
like a set. Tailor the food, never the style.

## Canvas and structure

- `viewBox="0 0 800 600"`, no `width`/`height`, no `<text>`, no gradients,
  filters, strokes on large shapes, external images or fonts.
- Layer order, back to front:
  1. **Background**: one full-bleed pastel `<rect width="800" height="600">`.
  2. **Shadow**: the vessel's silhouette in `#000` at `opacity=".08"`,
     offset about `translate(12 14)` (or an ellipse slightly offset).
  3. **Vessel**: seen from directly above, centered, filling about 60–70%
     of the height. Pick what the dish is served in:
     - Plate or bowl: `<circle cx="400" cy="300" r="250" fill="#fafaf9"/>`
       (bowls add an inner rim circle `r="200" fill="#e7e5e4"`).
     - Baking dish: rounded rect around `x=170 y=90 w=460 h=420 rx=40`,
       `#f5f5f4` rim with the food inset by about 30px.
     - Sheet pan / board: wide rounded rect around `x=120 y=110 w=560 h=380 rx=30`.
     - Loaf or free-standing item: the item itself plus its shadow, no vessel.
  4. **Food**: 2–4 big flat shapes for the main component, each with a
     slightly darker outline shape behind it (same shape, about 12px larger)
     to read as a crust or edge.
  5. **Details**: small dots or blobs for texture (oats, chips, herbs,
     berries, spices), placed with natural randomness, and 1–3 accent pieces
     (a pat of butter, a lemon slice, a basil leaf). Short rounded strokes
     (`stroke-linecap="round"`, width 8–14) are fine for drizzles or score marks.

## Color

- Background: a light tint that complements the food, not the food color:
  warm food on cool pastel (`#dbeafe`, `#e0f2fe`, `#ecfccb`), cool or red food
  on warm pastel (`#fde7d3`, `#fef3c7`, `#fde2e4`). Avoid reusing the same
  background as neighbouring recipes in the same category.
- Food: 2–3 related hues per component (base, darker edge, lighter
  highlight), taken from the real ingredients: golden browns
  `#d6a35c #c08a45 #e8b46b`, tomato `#c2410c #dc4a1a`, greens `#15803d`,
  berries `#b91c1c #dc2626`, chocolate `#3f2a1d`, butter `#fde68a`.
- Vessels stay neutral: `#fafaf9`, `#f5f5f4`, `#e7e5e4`, dark pans `#57534e`.

## Example (cookies on a pink background)

```svg
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 800 600">
  <rect width="800" height="600" fill="#fde2e4"/>
  <circle cx="250" cy="200" r="110" fill="#000" opacity=".08" transform="translate(6 8)"/>
  <circle cx="250" cy="200" r="110" fill="#c98a43"/>
  <circle cx="250" cy="200" r="98" fill="#d9a05a"/>
  <circle cx="220" cy="180" r="9" fill="#3f2a1d"/>
  <circle cx="275" cy="225" r="8" fill="#2b1a10"/>
  <!-- …more cookies and chips… -->
</svg>
```

Look at the existing `recipes/**/*.svg` files and match them.

## Steps

1. Read the recipe YAML to understand the finished dish: what it's served in,
   its color and texture, and the key visible ingredients or garnish.
2. Write `<slug>.svg` next to the recipe. Keep it under about 12 KB; generate
   repetitive dots with a short script if needed and paste the result.
3. Set `image: <slug>.svg` (and a short `image_alt`) in the recipe.
4. If an option visibly changes the dish (a topping, a different cooking
   vessel), offer a variant: draw `<slug>-<variant>.svg` from the same base
   (same background and composition, only the changed element differs) and
   add it under `images:` with a `when:` condition.
5. Run `get-cooked validate`. If a browser preview is available, look at it
   next to two existing illustrations and adjust until it reads as part of
   the set.
