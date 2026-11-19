// Package migrate upgrades a site's files when get-cooked changes a file
// format. Every breaking format change must add a Migration here so users
// can run `get-cooked migrate` instead of editing files by hand.
//
// Migrations edit the text in place (keeping comments, ordering and
// formatting), must be idempotent, and their output is re-parsed as YAML.
package migrate

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/RashKash103/get-cooked/internal/config"
)

// Kind is the kind of site file a migration applies to.
type Kind string

// File kinds.
const (
	Ingredients Kind = "ingredients" // ingredient database files
	Recipes     Kind = "recipes"     // recipe files (and their ingredient_db sections)
	Config      Kind = "config"      // get-cooked.yaml
	Catalogs    Kind = "catalogs"    // i18n/<lang>.yaml
)

// Migration is one format upgrade.
type Migration struct {
	ID          string // stable identifier, e.g. "0.3.0-density"
	Since       string // first version with the new format
	Description string
	Kinds       []Kind
	// Apply returns the migrated text and how many changes were made.
	Apply func(src []byte) ([]byte, int)
}

// All lists migrations, oldest first.
var All = []Migration{
	{
		ID:          "0.3.0-density",
		Since:       "0.3.0",
		Description: "grams_per_cup: N → density: { value: N, unit: g/cup }",
		Kinds:       []Kind{Ingredients, Recipes},
		Apply: func(src []byte) ([]byte, int) {
			n := 0
			out := gramsPerCupRe.ReplaceAllFunc(src, func(m []byte) []byte {
				n++
				v := gramsPerCupRe.FindSubmatch(m)[1]
				return []byte(fmt.Sprintf("density: { value: %s, unit: g/cup }", v))
			})
			return out, n
		},
	},
}

var gramsPerCupRe = regexp.MustCompile(`\bgrams_per_cup:[ \t]*([0-9]*\.?[0-9]+)`)

// Change is a file a migration changed (or would change).
type Change struct {
	File      string
	Migration string
	Count     int
}

// Files returns the site's files by kind.
func Files(cfg *config.Config) (map[Kind][]string, error) {
	files := map[Kind][]string{Config: {cfg.Path(config.FileName)}}
	for _, p := range cfg.Ingredients {
		files[Ingredients] = append(files[Ingredients], cfg.Path(p))
	}
	matches, _ := filepath.Glob(filepath.Join(cfg.Dir, "i18n", "*.yaml"))
	files[Catalogs] = matches
	root := cfg.Path(cfg.Recipes)
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && strings.HasPrefix(d.Name(), ".") && p != root {
			return filepath.SkipDir
		}
		ext := strings.ToLower(filepath.Ext(p))
		if !d.IsDir() && (ext == ".yaml" || ext == ".yml") {
			files[Recipes] = append(files[Recipes], p)
		}
		return nil
	})
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	for k := range files {
		sort.Strings(files[k])
	}
	return files, nil
}

// Run applies every migration to the site's files. With dryRun nothing is
// written. It returns the changes made (or that would be made).
func Run(cfg *config.Config, dryRun bool) ([]Change, error) {
	files, err := Files(cfg)
	if err != nil {
		return nil, err
	}
	var changes []Change
	for _, m := range All {
		seen := map[string]bool{}
		for _, k := range m.Kinds {
			for _, f := range files[k] {
				if seen[f] {
					continue
				}
				seen[f] = true
				src, err := os.ReadFile(f)
				if errors.Is(err, os.ErrNotExist) {
					continue
				}
				if err != nil {
					return changes, err
				}
				out, n := m.Apply(src)
				if n == 0 || bytes.Equal(out, src) {
					continue
				}
				var check yaml.Node
				if err := yaml.Unmarshal(out, &check); err != nil {
					return changes, fmt.Errorf("%s: migration %s produced invalid YAML (file left unchanged): %w", f, m.ID, err)
				}
				changes = append(changes, Change{File: f, Migration: m.ID, Count: n})
				if !dryRun {
					info, err := os.Stat(f)
					if err != nil {
						return changes, err
					}
					if err := os.WriteFile(f, out, info.Mode().Perm()); err != nil {
						return changes, err
					}
				}
			}
		}
	}
	return changes, nil
}
