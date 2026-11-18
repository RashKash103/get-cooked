// Package docs embeds reference documentation printed by `get-cooked docs`.
package docs

import _ "embed"

// RecipeFormat is the recipe format reference (recipe-format.md).
//
//go:embed recipe-format.md
var RecipeFormat string
