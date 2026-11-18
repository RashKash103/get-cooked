# get-cooked Claude skills

Skills for working on a get-cooked recipe site with Claude Code. Install them
into your recipe repository with:

```sh
get-cooked skills install            # all skills into ./.claude/skills
get-cooked skills install import-recipe --dir ~/.claude/skills
```

| Skill | What it does |
|---|---|
| `import-recipe` | Turns a recipe from a URL, text or photo into a get-cooked YAML file, adding scaling logic, options and ingredient database entries. |
| `recipe-illustration` | Draws a flat, top-down SVG illustration for a recipe in the house style. |
| `add-site-language` | Adds a language to the site: UI strings, units and ingredient names. |
| `translate-recipe` | Translates one recipe (or all) into a configured language. |

The skills call the `get-cooked` CLI (`validate`, `render`, `docs`, `i18n`),
so it must be on your PATH.
