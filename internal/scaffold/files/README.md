# {{TITLE}}

Recipes built with [get-cooked](https://github.com/RashKash103/get-cooked).

```sh
get-cooked serve              # preview with live reload at http://localhost:8080
get-cooked new "Lemon Bars"   # start a recipe
get-cooked validate           # check everything
get-cooked build              # write the site to ./public
get-cooked skills install     # Claude Code skills: import, translate, illustrate
```

Recipes live in `recipes/`, the ingredient database in `ingredients.yaml`.
Pushing to `main` deploys to GitHub Pages (see `.github/workflows/deploy.yml`;
enable Pages with "GitHub Actions" as the source in the repo settings).
