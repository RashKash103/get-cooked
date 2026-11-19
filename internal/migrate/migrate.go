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
	"github.com/RashKash103/get-cooked/internal/i18n"
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

func init() {
	All = append(All, Migration{
		ID:    "0.3.0-i18n-numerals",
		Since: "0.3.0",
		Description: "i18n/<lang>.yaml: add digits, fractions, fraction_glyphs and fraction_display " +
			"(with the defaults `i18n init` writes, so nothing changes) where they're missing",
		Kinds: []Kind{Catalogs},
		Apply: AddTemplateKeys(i18n.EnglishYAML, "digits", "fractions", "fraction_glyphs", "fraction_display"),
	})
}

func init() {
	All = append(All, Migration{
		ID:    "0.5.0-i18n-strings",
		Since: "0.5.0",
		Description: "i18n/<lang>.yaml: add UI strings, time labels, duration words, servings and shopping " +
			"categories that `i18n init` now writes, in English and marked for translation",
		Kinds: []Kind{Catalogs},
		Apply: AddTemplateSectionKeys(i18n.EnglishYAML, "ui", "times", "duration", "servings", "categories"),
	})
}

func init() {
	All = append(All, Migration{
		ID:          "0.6.0-timer-ranges",
		Since:       "0.6.0",
		Description: "{4}–~{5} → ~{4, 5}: a time range becomes one timer with a picker (recipes and their translations)",
		Kinds:       []Kind{Recipes},
		Apply: func(src []byte) ([]byte, int) {
			n := 0
			out := timerRangeRe.ReplaceAllFunc(src, func(m []byte) []byte {
				g := timerRangeRe.FindSubmatch(m)
				lo := strings.TrimSpace(string(g[1]))
				for _, kw := range []string{"if ", "elif ", "else", "end"} {
					if strings.HasPrefix(lo, kw) {
						return m
					}
				}
				n++
				return []byte("~{" + lo + ", " + strings.TrimSpace(string(g[2])) + "}")
			})
			return out, n
		},
	})
}

// {4}–~{5}, {4} - ~{5}, {4} to ~{5} (but not formatted values like {x | int}).
var timerRangeRe = regexp.MustCompile(`\{([^{}|]+)\}\s*(?:–|—|-|to)\s*~\{([^{}]+)\}`)

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

// AddTemplateKeys returns an Apply function that adds top-level keys from a
// template file (with the template's comment lines and default value) to a
// file that doesn't define them. Each key goes where the template has it:
// after the closest preceding template key the file defines, otherwise
// before the next one (above its comments), otherwise at the end.
func AddTemplateKeys(template []byte, keys ...string) func([]byte) ([]byte, int) {
	order, blocks := templateBlocks(template)
	return func(src []byte) ([]byte, int) {
		n := 0
		for _, key := range keys {
			block, ok := blocks[key]
			if !ok {
				continue
			}
			present, err := topLevelKeys(src)
			if err != nil || present == nil {
				return src, n // not a mapping (or invalid YAML): leave it alone
			}
			if _, ok := present[key]; ok {
				continue
			}
			lines := strings.Split(string(src), "\n")
			at := -1
			idx := indexOf(order, key)
			// After the closest preceding key that fits on one line.
			for i := idx - 1; i >= 0 && at < 0; i-- {
				if kl, ok := present[order[i]]; ok && kl.single {
					at = kl.line // insert after this (1-based) line
				}
			}
			// Otherwise before the next key the file defines, above its comments.
			if at < 0 {
				for i := idx + 1; i < len(order) && at < 0; i++ {
					if kl, ok := present[order[i]]; ok {
						j := kl.line - 1 // 0-based index of the key line
						for j > 0 && strings.HasPrefix(strings.TrimSpace(lines[j-1]), "#") {
							j--
						}
						at = j
					}
				}
			}
			if at < 0 {
				at = len(lines)
				for at > 0 && strings.TrimSpace(lines[at-1]) == "" {
					at--
				}
			}
			out := append([]string{}, lines[:at]...)
			out = append(out, block...)
			out = append(out, lines[at:]...)
			src = []byte(strings.Join(out, "\n"))
			n++
		}
		return src, n
	}
}

type keyLine struct {
	line   int  // 1-based line of the key
	single bool // the whole entry is on that line
}

func topLevelKeys(src []byte) (map[string]keyLine, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(src, &root); err != nil {
		return nil, err
	}
	if len(root.Content) == 0 || root.Content[0].Kind != yaml.MappingNode {
		return nil, nil
	}
	m := root.Content[0]
	out := map[string]keyLine{}
	for i := 0; i+1 < len(m.Content); i += 2 {
		k, v := m.Content[i], m.Content[i+1]
		single := v.Line == k.Line && (v.Kind == yaml.ScalarNode && v.Style != yaml.LiteralStyle && v.Style != yaml.FoldedStyle ||
			v.Style&yaml.FlowStyle != 0)
		out[k.Value] = keyLine{line: k.Line, single: single}
	}
	return out, nil
}

// templateBlocks returns the template's top-level keys in order and, for
// single-line entries, the entry's line with the comment lines right above it.
func templateBlocks(template []byte) ([]string, map[string][]string) {
	lines := strings.Split(string(template), "\n")
	present, _ := topLevelKeys(template)
	type kv struct {
		key  string
		line int
	}
	var list []kv
	for k, l := range present {
		list = append(list, kv{k, l.line})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].line < list[j].line })
	order := make([]string, len(list))
	blocks := map[string][]string{}
	for i, e := range list {
		order[i] = e.key
		if !present[e.key].single {
			continue
		}
		start := e.line - 1
		for start > 0 && strings.HasPrefix(strings.TrimSpace(lines[start-1]), "#") {
			start--
		}
		blocks[e.key] = append([]string{}, lines[start:e.line]...)
	}
	return order, blocks
}

func indexOf(list []string, s string) int {
	for i, v := range list {
		if v == s {
			return i
		}
	}
	return -1
}

// untranslatedNote marks keys a migration copied from the English template.
const untranslatedNote = "# Added by `get-cooked migrate` (English defaults; translate as needed):"

// AddTemplateSectionKeys returns an Apply function that, for each named
// top-level mapping section of the template, adds the keys a file's section
// is missing (with the template's values, after the section's last entry,
// under a "translate these" comment), and adds whole sections the file
// lacks. Sections written in flow style ({…}) are left alone.
func AddTemplateSectionKeys(template []byte, sections ...string) func([]byte) ([]byte, int) {
	tLines := strings.Split(string(template), "\n")
	var tRoot yaml.Node
	_ = yaml.Unmarshal(template, &tRoot)
	tMap := mappingOf(&tRoot)
	return func(src []byte) ([]byte, int) {
		n := 0
		for _, name := range sections {
			tKey, tVal := lookup(tMap, name)
			if tVal == nil || tVal.Kind != yaml.MappingNode {
				continue
			}
			var root yaml.Node
			if err := yaml.Unmarshal(src, &root); err != nil {
				return src, n
			}
			m := mappingOf(&root)
			if m == nil {
				return src, n
			}
			lines := strings.Split(string(src), "\n")
			_, val := lookup(m, name)
			if val == nil {
				// Whole section missing: append the template's block (with its comments).
				start := tKey.Line - 1
				for start > 0 && strings.HasPrefix(strings.TrimSpace(tLines[start-1]), "#") {
					start--
				}
				end := lastLine(tVal)
				block := append([]string{""}, tLines[start:end]...)
				at := len(lines)
				for at > 0 && strings.TrimSpace(lines[at-1]) == "" {
					at--
				}
				out := append(append(append([]string{}, lines[:at]...), block...), "")
				src = []byte(strings.Join(out, "\n"))
				n++
				continue
			}
			if val.Kind != yaml.MappingNode || val.Style&yaml.FlowStyle != 0 || len(val.Content) == 0 {
				continue
			}
			have := map[string]bool{}
			for i := 0; i+1 < len(val.Content); i += 2 {
				have[val.Content[i].Value] = true
			}
			indent := strings.Repeat(" ", val.Content[0].Column-1)
			var add []string
			for i := 0; i+1 < len(tVal.Content); i += 2 {
				k := tVal.Content[i]
				if have[k.Value] || tVal.Content[i+1].Line != k.Line {
					continue
				}
				add = append(add, indent+strings.TrimSpace(tLines[k.Line-1]))
			}
			if len(add) == 0 {
				continue
			}
			at := lastLine(val) // insert after the section's last line
			block := append([]string{indent + untranslatedNote}, add...)
			out := append(append(append([]string{}, lines[:at]...), block...), lines[at:]...)
			src = []byte(strings.Join(out, "\n"))
			n += len(add)
		}
		return src, n
	}
}

func mappingOf(root *yaml.Node) *yaml.Node {
	if len(root.Content) == 0 || root.Content[0].Kind != yaml.MappingNode {
		return nil
	}
	return root.Content[0]
}

func lookup(m *yaml.Node, key string) (*yaml.Node, *yaml.Node) {
	if m == nil {
		return nil, nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i], m.Content[i+1]
		}
	}
	return nil, nil
}

// lastLine returns the last (1-based) line used by a node's entries.
func lastLine(n *yaml.Node) int {
	last := n.Line
	for _, c := range n.Content {
		if l := lastLine(c); l > last {
			last = l
		}
	}
	return last
}
