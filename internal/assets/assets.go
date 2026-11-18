// Package assets embeds the compiled frontend bundle (built from web/ by Vite).
package assets

import (
	"embed"
	"io/fs"
	"os"
)

//go:embed all:dist
var embedded embed.FS

// FS returns the frontend bundle. Setting GET_COOKED_ASSETS to a directory
// serves assets from disk instead (useful with `pnpm --filter web watch`).
func FS() fs.FS {
	if dir := os.Getenv("GET_COOKED_ASSETS"); dir != "" {
		return os.DirFS(dir)
	}
	sub, err := fs.Sub(embedded, "dist")
	if err != nil {
		panic(err)
	}
	return sub
}

// Built reports whether the bundle contains the app (i.e. `pnpm build` ran).
func Built() bool {
	_, err := fs.Stat(FS(), "app.js")
	return err == nil
}
