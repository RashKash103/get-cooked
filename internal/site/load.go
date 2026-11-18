// Package site loads a get-cooked site and builds it into static files.
package site

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/RashKash103/get-cooked/internal/config"
	"github.com/RashKash103/get-cooked/internal/recipe"
	"github.com/RashKash103/get-cooked/internal/runtime"
	"github.com/RashKash103/get-cooked/internal/units"
)

// Site is a loaded site.
type Site struct {
	Config  *config.Config
	Units   *units.Table
	DB      *recipe.DB
	Recipes []*recipe.Recipe
	Diags   recipe.Diags
}

// LoadOptions controls loading.
type LoadOptions struct {
	Drafts bool // include draft recipes
	Sweep  bool // run the runtime validation sweep
}

// IsRecipeFile reports whether path looks like a recipe file.
func IsRecipeFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".yaml" || ext == ".yml"
}

// Load loads config, ingredient database and recipes. Problems are reported
// in Site.Diags; the returned error is only for unrecoverable failures.
func Load(dir string, opts LoadOptions) (*Site, error) {
	s := &Site{}
	cfgPath := filepath.Join(dir, config.FileName)
	if b, err := os.ReadFile(cfgPath); err == nil {
		var root yaml.Node
		if err := yaml.Unmarshal(b, &root); err != nil {
			return nil, err
		}
		if ds := recipe.ValidateSchema("config", cfgPath, &root); ds.HasErrors() {
			return nil, ds
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	cfg, err := config.Load(dir)
	if err != nil {
		return nil, err
	}
	s.Config = cfg
	tb, err := cfg.UnitTable()
	if err != nil {
		return nil, err
	}
	s.Units = tb
	paths := make([]string, len(cfg.Ingredients))
	for i, p := range cfg.Ingredients {
		paths[i] = cfg.Path(p)
	}
	db, ds := recipe.LoadDB(paths, tb)
	s.DB = db
	s.Diags = append(s.Diags, ds...)

	root := cfg.Path(cfg.Recipes)
	var files []string
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && strings.HasPrefix(d.Name(), ".") && p != root {
			return filepath.SkipDir
		}
		if !d.IsDir() && IsRecipeFile(p) && !strings.HasPrefix(d.Name(), "_") {
			files = append(files, p)
		}
		return nil
	})
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	sort.Strings(files)

	comp := &recipe.Compiler{Units: tb, DB: db, System: cfg.Units.DefaultSystem}
	type result struct {
		r     *recipe.Recipe
		diags recipe.Diags
	}
	results := make([]result, len(files))
	parallel(len(files), func(i int) {
		r, ds := comp.CompileFile(files[i])
		if r != nil && !ds.HasErrors() && r.Category == "" {
			if rel, err := filepath.Rel(root, filepath.Dir(files[i])); err == nil && rel != "." {
				r.Category = titleCase(strings.Split(rel, string(filepath.Separator))[0])
			}
		}
		results[i] = result{r, ds}
	})
	slugs := map[string]string{}
	var all []*recipe.Recipe
	for _, res := range results {
		s.Diags = append(s.Diags, res.diags...)
		if res.r == nil || res.diags.HasErrors() {
			continue
		}
		if prev, ok := slugs[res.r.Slug]; ok {
			s.Diags = append(s.Diags, recipe.Diag{File: res.r.Path, Msg: "duplicate slug " + res.r.Slug + " (also used by " + prev + ")"})
			continue
		}
		slugs[res.r.Slug] = res.r.Path
		all = append(all, res.r)
	}
	linkDiags := recipe.Link(all)
	s.Diags = append(s.Diags, linkDiags...)
	broken := map[string]bool{}
	for _, d := range linkDiags {
		broken[d.File] = true
	}
	checks := make([]recipe.Diags, len(all))
	parallel(len(all), func(i int) {
		r := all[i]
		if broken[r.Path] {
			return
		}
		if opts.Sweep {
			checks[i] = runtime.Sweep(r)
			return
		}
		if _, err := runtime.Render(r.C, runtime.State{}); err != nil {
			d := recipe.Diag{File: r.Path, Msg: err.Error()}
			if re, ok := err.(*runtime.Error); ok {
				d.Line, d.Col = re.Loc.Line, re.Loc.Col
			}
			checks[i] = recipe.Diags{d}
		}
	})
	for i, r := range all {
		s.Diags = append(s.Diags, checks[i]...)
		if checks[i].HasErrors() || broken[r.Path] || r.Draft && !opts.Drafts {
			continue
		}
		s.Recipes = append(s.Recipes, r)
	}
	sort.SliceStable(s.Recipes, func(i, j int) bool {
		a, b := s.Recipes[i], s.Recipes[j]
		if a.Date != b.Date {
			return a.Date > b.Date
		}
		return strings.ToLower(a.Title) < strings.ToLower(b.Title)
	})
	return s, nil
}

func parallel(n int, fn func(i int)) {
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)
	for i := 0; i < n; i++ {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			fn(i)
		}()
	}
	wg.Wait()
}

func titleCase(s string) string {
	words := strings.Fields(strings.NewReplacer("-", " ", "_", " ").Replace(s))
	for i, w := range words {
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}
