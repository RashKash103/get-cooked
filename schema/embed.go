// Package schema embeds the JSON Schemas for recipes, the ingredient database
// and the site config.
package schema

import "embed"

// FS holds the *.schema.json files.
//
//go:embed *.schema.json
var FS embed.FS
