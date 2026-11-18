# get-cooked — Design & Architecture

`get-cooked` is a single-binary static site generator (Go) that turns a folder of
YAML recipes into a fast, mobile-first recipe website. Recipes are *programs*:
quantities, times, equipment, steps and notes can all depend on the selected
servings and on reader-selected options, and the site recomputes everything live
in the browser.

## Goals

1. **Powerful recipe language** — arbitrary math, conditionals, variables,
   options (toggles / choices / numbers) that add, remove or rewrite
   ingredients, equipment, steps and notes.
2. **Correct, friendly quantities** — unit conversion (incl. mass⇄volume via
   per-ingredient density), unit systems (original / US / metric), and rounding
   that never prints `15/64 tsp`.
3. **One binary** — `get-cooked build` needs no Node, no Python. Frontend assets
   are built when get-cooked itself is built and are embedded with `go:embed`.
4. **Mobile-first, uncrowded UI** — light/dark, cook mode (screen wake lock),
   tap-to-start timers, works offline once visited.
5. **CI-friendly** — downloadable release binaries + a GitHub Action, strict
   `validate` command with GitHub annotations.

## Non-goals (for now)

Nutrition calculation, user accounts, server-side anything, a recipe editor UI.

---

## Why YAML

YAML is readable for humans *and* very easy for an LLM to produce (important for
the import skill), supports comments and multi-line strings, and has editor
schema support via `yaml-language-server`. TOML gets awkward with deeply nested
lists of objects (steps, sections). Expressions live inside YAML strings, so the
format itself stays dumb and the power lives in a small, well-defined expression
language.

Every recipe file can start with:

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/RashKash103/get-cooked/main/schema/recipe.schema.json
```

## Site layout (user repo)

```
my-recipes/
  get-cooked.yaml          # site config
  ingredients.yaml         # shared ingredient database (densities, plurals, rounding)
  recipes/
    desserts/
      cranberry-oat-bars.yaml
      cranberry-oat-bars.jpg
  static/                  # copied verbatim (favicon, CNAME, …)
  .claude/skills/import-recipe/   # optional, installed by `get-cooked init`
```

Slug = file name without extension. Category defaults to the parent directory
name unless set in the recipe.

---

## Recipe format (summary — full reference in `docs/recipe-format.md`)

```yaml
title: Cranberry Oat Bars
description: Chewy, buttery bars. Great for lunchboxes.
image: cranberry-oat-bars.jpg
category: Desserts
tags: [baking, make-ahead]
source: { name: Grandma, url: https://example.com }

servings: { default: 9, min: 1, max: 24, unit: bar }

options:
  - id: cranberries            # also auto-created by `optional: true` ingredients
    type: toggle
    label: Add dried cranberries
  - id: sweetness
    type: choice
    label: Sweetness
    default: regular
    choices: [{ id: less, label: Less sweet }, { id: regular, label: Regular }]

vars:
  small: servings <= 4
  pan: "small ? '8×8 inch' : '13×9 inch'"
  bake: "small ? 22 : 25 + servings * 0.5"

times: { prep: 15, cook: bake }   # numbers or expressions, minutes

equipment:
  - { id: pan, name: "{pan} baking pan" }

ingredients:
  - section: Base
    items:
      - { id: oats, ingredient: rolled-oats, amount: 2, unit: cup }
      - id: sugar
        ingredient: brown-sugar
        amount: "0.75 * scale * (sweetness == 'less' ? 0.7 : 1)"   # string ⇒ raw expression
        unit: cup
      - { id: cranberries, ingredient: dried-cranberries, amount: 0.75, unit: cup, optional: true }

steps:
  - Preheat the oven to {350 | temp}. Line a &pan with parchment.
  - text: Stir together @oats and @sugar{1/2}{if cranberries}, then fold in @cranberries{end}.
  - when: cranberries
    text: Press a few extra cranberries on top.
  - Bake for ~{bake} until golden.

notes:
  - when: servings == 1
    kind: warning
    text: A single bar bakes in a very small dish and may come out softer.
```

### Core rules

* **Everything can have `when:`** — ingredients, equipment, steps, notes,
  sections. Items may share an `id` if their `when` conditions are mutually
  exclusive (variants). `validate` proves it by sweeping states.
* **Amounts**: a *number* is the amount at default servings and scales linearly
  with `scale` (`scale: false` pins it). A *string* is a raw expression and is
  not auto-scaled — use `scale` / `servings` yourself (e.g. eggs:
  `"max(1, round(2 * scale))"`).
* **`scale`** defaults to `servings / servings.default` and can be overridden
  per recipe with `scaling: <expr>` (e.g. pan-size choices).
* **`optional: true`** on an ingredient auto-creates a toggle option with the
  same id (label "Add <name>"), so `when: cranberries` just works.

### Text templates (steps, notes, names, labels)

| Syntax | Meaning |
|---|---|
| `{expr}` | evaluate and print |
| `{expr \| fmt}` / `{expr \| fmt:arg}` | formatters: `frac`, `int`, `duration`, `temp` (°F in), `temp_c`, `len` (inches in), `len_cm`, `qty:<unit>` |
| `{if expr}…{elif expr}…{else}…{end}` | inline conditional, nestable |
| `@id` | ingredient reference: current quantity + name |
| `@id{expr}` | portion of the ingredient (`@sugar{1/2}`) |
| `@id{-}` | name only |
| `&id` | equipment reference (current variant name) |
| `~{expr}` | tappable timer (minutes) |
| `\{ \} \@ \& \~ \\` | escapes |

References are checked at compile time (unknown id ⇒ error with suggestion).

### Expression language

Small, safe, deterministic, side-effect free. Values: number, bool, string.

* literals: `3`, `2.5`, `'text'`/`"text"`, `true`, `false`
* operators (low→high): `?:`, `||`, `&&`, `== !=`, `< <= > >=`, `+ -`,
  `* / %`, `^` (right-assoc power), unary `- !`
* functions: `min max abs round(x[,step]) floor ceil clamp(x,lo,hi) sqrt pow`
  `lerp(a,b,t) if(c,a,b) plural(n, one[, other]) str(x)`
* identifiers: `servings`, `base_servings`, `scale`, `metric` (bool: reader
  chose metric), every option id, every `vars` entry.
* `when` must evaluate to a bool (type errors are reported, not coerced).
* Static checks at compile time: unknown identifiers, unknown functions,
  arity, var cycles. Dynamic checks during `validate` sweep: type errors,
  division by zero, NaN/∞, negative amounts, overlapping variants.

---

## Quantities, units and rounding

* Built-in unit table (data, not code): volume (tsp, tbsp, cup, fl oz, pint,
  quart, gallon, ml, l), mass (g, kg, oz, lb), count-like units (clove, can,
  slice, pinch…), plus aliases (`tablespoons`, `T`, `grams`…).
* `ingredients.yaml` gives per-ingredient `density` (grams per cup), `each`
  (grams per piece), `plural`, `aliases`, `measure` preference and rounding.
* **Unit systems**: `original` (keep authored system & dimension), `us`,
  `metric` (prefers grams when density is known — bakers' default).
* **Display ladders** (data-driven, overridable in `get-cooked.yaml`): e.g. US
  volume `tsp → tbsp → cup`, each with min value and allowed fraction
  denominators. The algorithm picks the largest unit whose rounded value is
  ≥ its minimum and within tolerance, then rounds to the nearest allowed
  fraction. 15/64 tsp ⇒ ¼ tsp; 0.125 cup ⇒ 2 tbsp; 1 tbsp × ⅓ ⇒ 1 tsp.
  Metric uses step rounding (0.5 g under 10 g, 1 g, 5 g, …; kg ≥ 1000 g).
  Count ingredients round per ingredient (`round: 1`, `min: 1`).

---

## Architecture

```
 YAML files ──► Go: load + JSON-Schema validate ──► compile (expr/tmpl → IR)
                                                     │
                     ┌───────────────────────────────┼─────────────────────┐
                     ▼                               ▼                     ▼
          Go runtime: render @ default     validate sweep (all       compiled IR JSON
          → SSR HTML + JSON-LD (SEO)       servings × options)       embedded in page
                                                                           │
                                                     browser: TS runtime ◄─┘
                                                     (same algorithm, live)
```

* **Go owns parsing.** Expressions and templates are parsed once in Go with
  precise error positions (file:line:col) and serialised to a compact JSON IR.
  The browser never parses source text — it only evaluates IR, which keeps the
  JS tiny.
* **Two runtimes, one contract.** The *runtime* (evaluate IR, scale, convert,
  round, format, produce a "rendered model") exists in Go (for SSR, JSON-LD,
  `render` CLI, validation sweep) and TypeScript (live UI). Parity is enforced
  by a **conformance suite**: Go writes golden `(compiled IR, state) → rendered
  model` fixtures; Vitest replays them through the TS runtime and must match
  byte-for-byte. Rounding uses round-half-away-from-zero in both languages.
* **Units table + rounding config** come from Go and are embedded in each
  page's JSON, so there is a single source of truth.
* **SSR + hydrate.** Every page is fully rendered HTML at default state (works
  without JS, fast first paint, printable). JS re-renders dynamic regions from
  the model on interaction.

### Go packages

| Package | Responsibility |
|---|---|
| `cmd/get-cooked` | entrypoint |
| `internal/cli` | commands: `init build serve validate render new import schema version` |
| `internal/expr` | lexer, parser, AST, static checks, evaluator, IR encode |
| `internal/tmpl` | text template parser → segment IR |
| `internal/units` | unit table, conversion, ladders, rounding, number/fraction formatting |
| `internal/recipe` | YAML model, decoding with positions, ingredient DB, compile → `Compiled` |
| `internal/runtime` | render `Compiled` for a `State` → `Model`; sweep validation |
| `internal/site` | site config, build pipeline, HTML templates, search index, sitemap, JSON-LD, image resizing |
| `internal/assets` | embedded frontend bundle (Vite output) |
| `internal/importer` | URL → schema.org Recipe JSON-LD → draft YAML (ingredient-line parser) |
| `internal/devserver` | `serve`: static server + rebuild on change + live reload (SSE) |
| `schema` | embedded JSON Schemas (recipe, ingredients, config) |
| `skills` | embedded Claude skill(s), installed by `init` |

### Frontend (`web/`)

Vite + TypeScript + Tailwind v4, **no framework** (vanilla TS modules, a few
KB). Output goes to `internal/assets/dist` and is embedded.

* `src/runtime/` — TS port of the runtime (IR eval, units, render model).
* `src/ui/` — controllers: servings stepper, options sheet, unit switch,
  ingredient checklist, step progress, timers, cook mode, theme, search,
  share/state-in-URL.

### Pages

* **Index** — search box, category chips, recipe cards (image, title, time).
  Client-side search over a small JSON index (title, tags, ingredients).
* **Recipe** — hero image, title, one-line description, time + servings.
  Compact control bar: `− 9 bars +`, *Customize* (bottom sheet: options, units),
  *Cook mode*. Ingredients (tap to check off) and numbered steps; two columns on
  wide screens. Conditional notes as small callouts. Minimal chrome.
* `404.html`, `sitemap.xml`, `robots.txt`, `manifest.webmanifest`, `sw.js`.

---

## Timers, notifications and the "kitchen" QoL layer

Research summary (platform reality, Nov 2026):

* **No web API creates OS clock timers**, installed PWA or not.
* **Android**: Chrome can open `intent:` URLs, but only for activities in the
  BROWSABLE category; Clock apps' `android.intent.action.SET_TIMER` generally
  isn't, so it can't be relied on. Offered as an opt-in, experimental "Send to
  Clock app" action with in-page fallback.
* **iOS**: no Clock URL scheme. A `shortcuts://run-shortcut?name=…&input=…`
  link can drive a user-made Shortcut that starts a timer — documented as an
  opt-in site setting (`timers.ios_shortcut`).
* **Notifications**: Notification API works on desktop/Android browsers and on
  iOS only for *installed* PWAs (16.4+). Scheduled ("trigger") notifications
  never shipped, so a timer fires its notification from the page (via the
  service worker's `showNotification`) when the page is alive; timers are
  stored as absolute end-times so they survive reloads/backgrounding and catch
  up immediately on return.

What we build:

1. **Tap `~{…}` chips** to start a timer (label = step summary). Timers live in
   a floating tray: pause/resume, +1 min, cancel. Persisted in `localStorage`.
2. **When done**: notification (if permitted), sound (Web Audio, works after
   the first tap), vibration, title-bar blink, and a dismissable banner.
3. **PWA**: manifest + service worker (offline cache of visited recipes and
   assets) so it installs to the home screen, works offline in the kitchen and
   gets notification support on iOS.
4. **Native bridge** for a future Android/iOS wrapper app: if
   `window.GetCookedNative` exists, timers/wake-lock are delegated to it:

   ```ts
   interface GetCookedNative {
     setTimer(seconds: number, label: string): void   // e.g. AlarmClock.ACTION_SET_TIMER
     keepScreenOn?(on: boolean): void
   }
   ```

Other QoL features:

* **Cook mode**: Screen Wake Lock (re-acquired on `visibilitychange`), larger
  type, current-step highlight, tap-to-complete steps, "next step" button.
* Ingredient checklist + step progress, remembered per recipe.
* State in URL (`?s=6&o.cranberries=1&u=metric`) → shareable customised recipe;
  also remembered in `localStorage`.
* Unit system + theme preferences remembered site-wide.
* Ingredient hover/tap on `@refs` in steps shows the scaled amount inline.
* Print stylesheet (just ingredients + steps).
* `Copy ingredients` (as a shopping list text).
* Keyboard: `+`/`-` servings, `c` cook mode, `/` search.
* Respect `prefers-reduced-motion`.

---

## CLI

```
get-cooked init [dir]                 scaffold a site (+ Claude import skill)
get-cooked build [-s dir] [-o dist] [--base-url URL] [--drafts]
get-cooked serve [-s dir] [-p 8080]   dev server, rebuild + live reload
get-cooked validate [-s dir] [--format text|github|json]
get-cooked render <file> [--servings N] [--set id=value]... [--units us|metric|original] [--json]
get-cooked new "<title>" [--category X]
get-cooked import <url>               JSON-LD → draft YAML on stdout
get-cooked schema [recipe|ingredients|config]
get-cooked version
```

## Distribution & CI

* **GoReleaser**: linux/darwin/windows × amd64/arm64 archives + checksums on tag
  `v*`; the before-hook builds the frontend.
* **`action.yml`** (composite): downloads the release binary for the runner and
  runs `get-cooked build`. Consumers:

  ```yaml
  - uses: RashKash103/get-cooked@v0
    with: { source: ., output: dist, base-url: https://me.github.io/recipes/ }
  - uses: actions/upload-pages-artifact@v3
    with: { path: dist }
  ```

* **CI** (`.github/workflows/ci.yml`): Go lint (golangci-lint) + test (race,
  coverage); web: biome, tsc, vitest (incl. Go↔TS conformance); validate the
  example site; e2e Playwright (mobile + desktop, light + dark, axe a11y) against
  the real binary serving the example site. Dependabot for Go, npm, actions.

## Testing strategy

| Layer | Tests |
|---|---|
| expr | table tests (precedence, errors with positions), fuzz test (parser never panics) |
| tmpl | table tests incl. nesting, escapes, unknown refs |
| units | conversion + rounding tables (`15/64 tsp → ¼ tsp`, ladders, metric steps) |
| recipe | decode/compile errors with file:line, optional→option, variants |
| runtime | golden models for fixture recipes across states; sweep errors |
| conformance | Go goldens replayed by Vitest against TS runtime |
| site | build example site to temp dir; assert pages, JSON-LD, sitemap, assets |
| cli | command tests (exit codes, output) |
| importer | httptest server with saved recipe pages |
| e2e | Playwright: search, servings, pan swap, 1-serving note, cranberry step, units, URL state, timers, cook mode (mocked wake lock), dark mode, no horizontal overflow, axe |

## Roadmap after v0.1

Sub-recipe references (`recipe: pie-crust` with scaled link), nutrition from
ingredient DB, multi-recipe shopping list, responsive WebP images, i18n,
quantity ranges (`1–2 cloves`), native wrapper app using the bridge above.
