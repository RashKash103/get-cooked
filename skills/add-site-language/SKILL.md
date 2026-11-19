---
name: add-site-language
description: Add a new language to a get-cooked recipe site, translating the UI strings, unit names, duration words and ingredient names, and enabling the language picker. Use when the user wants the site available in another language (e.g. "add Spanish", "support French").
---

# Add a language to a get-cooked site

A get-cooked site lists its languages in `get-cooked.yaml`:

```yaml
languages: [en, es]   # the first is the language recipes are written in
```

Each language has UI strings in `i18n/<lang>.yaml`. Anything missing falls
back to English. Recipes are translated separately (see the
`translate-recipe` skill). A recipe without a translation is shown in the
original language with the UI translated.

## Steps

1. Pick the language tag (BCP 47: `es`, `fr`, `de`, `pt-BR`, `zh-Hans`). Ask
   the user if the regional variant matters (e.g. `es` vs `es-MX`) and which
   unit system its readers expect.
2. Run `get-cooked i18n init <lang>`. It creates `i18n/<lang>.yaml` with every
   UI string, all units and every ingredient-database name pre-filled in
   English.
3. Translate the file in place:
   - `name`: the language's own name ("Español", "Français", "Deutsch").
   - `dir`: `rtl` for Arabic, Hebrew, Persian or Urdu; otherwise `ltr`.
   - `decimal`: `","` where that is the convention (most of Europe and Latin
     America).
   - `digits`: the numbering system readers expect, if not 0–9 (`arab` for
     Arabic, `arabext` for Persian/Urdu, `deva` for Hindi/Marathi, `beng`,
     `thai`, …; ask the user, since many Arabic-speaking regions use 0–9).
     With non-Latin digits, set `fractions: native` so fractions use the same
     numerals ("١⁄٢" rather than "½"), and `fraction_display: diagonal` to
     typeset them with a small raised numerator and lowered denominator. If the language has conventional
     characters for some fractions, list them in `fraction_glyphs`
     (`{ "1/2": "…" }`); they win over the style.
   - `of`: the word that joins an amount and an ingredient if the language
     needs one ("de" in Spanish and French: "2 tazas de harina"), else `""`.
   - `system`: `metric` for languages whose readers use metric, else leave
     empty.
   - `ui`: translate every value. Keep `{placeholders}` exactly as they are.
     Keep it short and natural for buttons; match the tone of the English.
   - `times`, `servings`, `duration`: short, conventional kitchen terms
     (`duration` uses abbreviations, e.g. `h`, `min`, `s`).
   - `units`: conventional abbreviations or words for the language (Spanish:
     `cdta`/`cda`/`taza`). Keep metric symbols (`g`, `ml`, `kg`, `l`) unchanged.
   - `ingredients`: natural shopping-list names; add `plural` for counted
     items (eggs, onions, lemons).
4. Add the tag to `languages:` in `get-cooked.yaml` (never move the first entry;
   that's the source language).
5. Run `get-cooked validate` and `get-cooked i18n status`. With the dev
   server (`get-cooked serve`), check `/<lang>/` in a browser: the picker,
   a recipe page, the Customize sheet and a timer.
6. Offer to translate recipes with the `translate-recipe` skill.

Don't edit the built-in English strings; to change English wording, edit
`i18n/en.yaml` (create it with `get-cooked i18n init en`).
