// Package site loads a get-cooked site and builds it into static files.
package site

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/RashKash103/get-cooked/internal/config"
	"github.com/RashKash103/get-cooked/internal/i18n"
	"github.com/RashKash103/get-cooked/internal/recipe"
	"github.com/RashKash103/get-cooked/internal/runtime"
	"github.com/RashKash103/get-cooked/internal/units"
)

// Site is a loaded site.
type Site struct {
	Config *config.Config
	Units  *units.Table
	DB     *recipe.DB
	// Recipes in the source (first) language.
	Recipes []*recipe.Recipe
	// Langs holds every configured language, the source language first.
	Langs []*Lang
	Diags recipe.Diags
}

// Lang is the site compiled for one language.
type Lang struct {
	Code    string
	Catalog *i18n.Catalog
	Units   *units.Table
	Recipes []*recipe.Recipe
	// Translated reports which recipe slugs have a translation file
	// (always true for the source language).
	Translated map[string]bool
	// Coverage per recipe slug: translated and total source strings.
	Coverage map[string][2]int
}

// Prefix is the URL path segment for the language ("" for the source language).
func (l *Lang) Prefix(source string) string {
	if l.Code == source {
		return ""
	}
	return l.Code + "/"
}

// LoadOptions controls loading.
type LoadOptions struct {
	Drafts bool // include draft recipes
	Sweep  bool // run the runtime validation sweep
}

// IsRecipeFile reports whether path looks like a recipe file (and not a
// recipe translation such as oatmeal.es.yaml).
func IsRecipeFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return (ext == ".yaml" || ext == ".yml") && i18n.TranslationLang(path) == ""
}

// Load loads config, ingredient database and recipes in every language.
// Problems are reported in Site.Diags; the returned error is only for
// unrecoverable failures.
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
	var orphans []string
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && strings.HasPrefix(d.Name(), ".") && p != root {
			return filepath.SkipDir
		}
		if d.IsDir() || strings.HasPrefix(d.Name(), "_") {
			return nil
		}
		if IsRecipeFile(p) {
			files = append(files, p)
		} else if lang := i18n.TranslationLang(p); lang != "" {
			orphans = append(orphans, p)
		}
		return nil
	})
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	sort.Strings(files)
	known := map[string]bool{}
	for _, f := range files {
		for _, l := range cfg.Languages {
			known[i18n.TranslationPath(f, l)] = true
		}
	}
	for _, o := range orphans {
		if !known[o] {
			lang := i18n.TranslationLang(o)
			msg := "translation file has no matching recipe"
			if !contains(cfg.Languages, lang) {
				msg = fmt.Sprintf("translation for %q, which is not in `languages:` in %s", lang, config.FileName)
			}
			s.Diags = append(s.Diags, recipe.Diag{File: o, Msg: msg, Warn: true})
		}
	}

	for i, code := range cfg.Languages {
		cat, err := i18n.Load(dir, code)
		if err != nil {
			return nil, err
		}
		l := &Lang{Code: code, Catalog: cat, Units: cat.Localize(tb), Translated: map[string]bool{}, Coverage: map[string][2]int{}}
		ds := s.loadLang(l, files, root, i == 0, opts)
		s.Diags = append(s.Diags, ds...)
		s.Langs = append(s.Langs, l)
	}
	s.Recipes = s.Langs[0].Recipes
	return s, nil
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func (s *Site) loadLang(l *Lang, files []string, root string, source bool, opts LoadOptions) recipe.Diags {
	cfg := s.Config
	var diags recipe.Diags
	type result struct {
		r     *recipe.Recipe
		diags recipe.Diags
		rec   *i18n.Recorder
	}
	results := make([]result, len(files))
	parallel(len(files), func(i int) {
		f := files[i]
		var tr *i18n.RecipeTranslation
		var ds recipe.Diags
		if !source {
			t, err := i18n.LoadRecipeTranslation(i18n.TranslationPath(f, l.Code), l.Code)
			if err != nil {
				ds = append(ds, recipe.Diag{File: i18n.TranslationPath(f, l.Code), Msg: err.Error()})
			}
			tr = t
		}
		rec := i18n.NewRecorder(tr)
		system := cfg.Units.DefaultSystem
		if l.Catalog.System != "" {
			system = l.Catalog.System
		}
		comp := &recipe.Compiler{Units: l.Units, DB: s.DB, System: system, Catalog: l.Catalog, Tr: rec.Tr}
		r, cds := comp.CompileFile(f)
		if tr != nil {
			// Errors in a translation point at the translation file.
			for j := range cds {
				cds[j].Msg = fmt.Sprintf("[%s translation] %s", l.Code, cds[j].Msg)
			}
		}
		ds = append(ds, cds...)
		if r != nil && !ds.HasErrors() && r.Category == "" {
			if rel, err := filepath.Rel(root, filepath.Dir(f)); err == nil && rel != "." {
				r.Category = rec.Tr(titleCase(strings.Split(rel, string(filepath.Separator))[0]))
			}
		}
		if source && r != nil && r.Image != "" && !strings.Contains(r.Image, "://") {
			if _, err := os.Stat(filepath.Join(filepath.Dir(f), r.Image)); err != nil {
				ds = append(ds, recipe.Diag{File: f, Msg: "image " + r.Image + " not found next to the recipe"})
			}
		}
		if !source && tr != nil {
			tp := tr.Path
			if miss := rec.Missing(); len(miss) > 0 {
				ds = append(ds, recipe.Diag{File: tp, Warn: true, Msg: fmt.Sprintf("%d of %d strings untranslated (first: %q); they fall back to %s", len(miss), len(rec.Sources), trunc(miss[0]), cfg.Languages[0])})
			}
			if st := rec.Stale(); len(st) > 0 {
				ds = append(ds, recipe.Diag{File: tp, Warn: true, Msg: fmt.Sprintf("%d translated strings no longer appear in the recipe (first: %q); run `get-cooked i18n extract --prune`", len(st), trunc(st[0]))})
			}
		}
		results[i] = result{r, ds, rec}
	})
	slugs := map[string]string{}
	var all []*recipe.Recipe
	for _, res := range results {
		diags = append(diags, res.diags...)
		if res.r == nil || res.diags.HasErrors() {
			continue
		}
		if prev, ok := slugs[res.r.Slug]; ok {
			if source {
				diags = append(diags, recipe.Diag{File: res.r.Path, Msg: "duplicate slug " + res.r.Slug + " (also used by " + prev + ")"})
			}
			continue
		}
		slugs[res.r.Slug] = res.r.Path
		all = append(all, res.r)
		translated := source || res.rec.T != nil
		l.Translated[res.r.Slug] = translated
		if translated {
			l.Coverage[res.r.Slug] = [2]int{len(res.rec.Sources) - len(res.rec.Missing()), len(res.rec.Sources)}
		}
	}
	linkDiags := recipe.Link(all)
	if source {
		diags = append(diags, linkDiags...)
	}
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
		// Untranslated recipes are identical to the source; skip re-checking.
		if !source && !l.Translated[r.Slug] {
			return
		}
		if opts.Sweep {
			checks[i] = runtime.Sweep(r)
		} else if _, err := runtime.Render(r.C, runtime.State{}); err != nil {
			d := recipe.Diag{File: r.Path, Msg: err.Error()}
			if re, ok := err.(*runtime.Error); ok {
				d.Line, d.Col = re.Loc.Line, re.Loc.Col
			}
			checks[i] = recipe.Diags{d}
		}
		if !source {
			for j := range checks[i] {
				checks[i][j].Msg = fmt.Sprintf("[%s translation] %s", l.Code, checks[i][j].Msg)
			}
		}
	})
	for i, r := range all {
		diags = append(diags, checks[i]...)
		if checks[i].HasErrors() || broken[r.Path] || r.Draft && !opts.Drafts {
			continue
		}
		l.Recipes = append(l.Recipes, r)
	}
	sort.SliceStable(l.Recipes, func(i, j int) bool {
		a, b := l.Recipes[i], l.Recipes[j]
		if a.Date != b.Date {
			return a.Date > b.Date
		}
		return strings.ToLower(a.Title) < strings.ToLower(b.Title)
	})
	return diags
}

func trunc(s string) string {
	if len([]rune(s)) > 40 {
		return string([]rune(s)[:40]) + "…"
	}
	return s
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
