// Package scaffold contains the starter files written by `get-cooked init`.
package scaffold

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

//go:embed all:files
var files embed.FS

// renames maps embedded names to their names on disk (dotfiles can't be
// embedded without care, and a .gitignore here would ignore our own files).
var renames = map[string]string{"gitignore": ".gitignore"}

// Write creates the starter site in dir. Existing files are never
// overwritten; it returns the paths written.
func Write(dir, title string) ([]string, error) {
	var written []string
	err := fs.WalkDir(files, "files", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel := strings.TrimPrefix(p, "files/")
		if r, ok := renames[rel]; ok {
			rel = r
		}
		dst := filepath.Join(dir, filepath.FromSlash(rel))
		if _, err := os.Stat(dst); err == nil {
			return nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		b, err := files.ReadFile(p)
		if err != nil {
			return err
		}
		b = []byte(strings.ReplaceAll(string(b), "{{TITLE}}", title))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dst, b, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", dst, err)
		}
		written = append(written, dst)
		return nil
	})
	return written, err
}

// File returns an embedded starter file (for tests).
func File(name string) ([]byte, error) { return files.ReadFile("files/" + name) }
