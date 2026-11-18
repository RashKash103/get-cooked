package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/RashKash103/get-cooked/internal/config"
	"github.com/RashKash103/get-cooked/internal/i18n"
	"github.com/RashKash103/get-cooked/internal/recipe"
	"github.com/RashKash103/get-cooked/internal/site"
)

func i18nCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "i18n",
		Short: "Manage translations of the site UI and recipes",
		Long: "Sites list their languages in get-cooked.yaml (`languages: [en, es]`, first = the language recipes\n" +
			"are written in). UI strings live in i18n/<lang>.yaml; recipe translations live next to each recipe as\n" +
			"<recipe>.<lang>.yaml and map original text to translated text.",
	}
	cmd.AddCommand(i18nInitCmd(), i18nExtractCmd(), i18nStatusCmd())
	return cmd
}

func i18nInitCmd() *cobra.Command {
	var dir string
	var force bool
	cmd := &cobra.Command{
		Use:   "init <lang>",
		Short: "Create i18n/<lang>.yaml with every UI string, unit and ingredient name to translate",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			lang := args[0]
			if !i18n.ValidTag(lang) {
				return fmt.Errorf("%q is not a language tag like es, fr or pt-BR", lang)
			}
			cfg, err := config.Load(dir)
			if err != nil {
				return err
			}
			path := i18n.Path(cfg.Dir, lang)
			if _, err := os.Stat(path); err == nil && !force {
				return fmt.Errorf("%s already exists (use --force to overwrite)", path)
			}
			tb, err := cfg.UnitTable()
			if err != nil {
				return err
			}
			paths := make([]string, len(cfg.Ingredients))
			for i, p := range cfg.Ingredients {
				paths[i] = cfg.Path(p)
			}
			db, _ := recipe.LoadDB(paths, tb)
			text := string(i18n.EnglishYAML)
			if lang != i18n.Default {
				text = strings.Replace(text, "name: English", "name: "+lang+" # replace with the language's own name, e.g. Español", 1)
				var units strings.Builder
				units.WriteString("units:\n")
				ids := make([]string, 0, len(tb.Units))
				for id := range tb.Units {
					if id != "each" {
						ids = append(ids, id)
					}
				}
				sort.Strings(ids)
				for _, id := range ids {
					u := tb.Units[id]
					fmt.Fprintf(&units, "  %s: { one: %q, other: %q }\n", id, u.One, u.Other)
				}
				text = strings.Replace(text, "units: {}\n", units.String(), 1)
				var ings strings.Builder
				ings.WriteString("ingredients:\n")
				for _, k := range db.Keys() {
					e := db.Entries[k]
					if e.Plural != "" {
						fmt.Fprintf(&ings, "  %s: { name: %q, plural: %q }\n", k, e.Name, e.Plural)
					} else {
						fmt.Fprintf(&ings, "  %s: { name: %q }\n", k, e.Name)
					}
				}
				if len(db.Keys()) > 0 {
					text = strings.Replace(text, "ingredients: {}\n", ings.String(), 1)
				}
			}
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "%s %s\n", render(out, sOK, "created"), render(out, sPath, path))
			if !contains(cfg.Languages, lang) {
				fmt.Fprintf(out, "%s add %q to `languages:` in %s to publish it\n", render(out, sMuted, "next:"), lang, config.FileName)
			}
			if lang != i18n.Default {
				fmt.Fprintf(out, "%s translate the values, then run `get-cooked i18n extract --lang %s` for recipes\n", render(out, sMuted, "then:"), lang)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&dir, "source", "s", ".", "site directory")
	cmd.Flags().BoolVar(&force, "force", false, "overwrite an existing file")
	return cmd
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func i18nExtractCmd() *cobra.Command {
	var dir string
	var langs []string
	var prune bool
	cmd := &cobra.Command{
		Use:   "extract [recipe.yaml...]",
		Short: "Create or update recipe translation files (<recipe>.<lang>.yaml)",
		Long: "Collects every translatable string of each recipe and writes <recipe>.<lang>.yaml, keeping existing\n" +
			"translations. New strings get empty values (which fall back to the original language).",
		Example: "  get-cooked i18n extract --lang es\n  get-cooked i18n extract recipes/desserts/cookies.yaml --lang es --lang fr",
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := site.Load(dir, site.LoadOptions{Drafts: true})
			if err != nil {
				return printLoadError(cmd, err, "text")
			}
			cfg := s.Config
			if len(langs) == 0 {
				langs = cfg.Languages[1:]
			}
			if len(langs) == 0 {
				return errors.New("no target languages: pass --lang or add more `languages:` to get-cooked.yaml")
			}
			for _, l := range langs {
				if l == cfg.Languages[0] {
					return fmt.Errorf("%s is the source language; recipes are written in it", l)
				}
			}
			targets := map[string]bool{}
			for _, a := range args {
				abs, _ := filepath.Abs(a)
				targets[abs] = true
			}
			src := s.Langs[0]
			out := cmd.OutOrStdout()
			n := 0
			for _, r := range src.Recipes {
				abs, _ := filepath.Abs(r.Path)
				if len(targets) > 0 && !targets[abs] {
					continue
				}
				// Recompile with a recorder to collect the source strings in order.
				rec := i18n.NewRecorder(nil)
				comp := &recipe.Compiler{Units: src.Units, DB: s.DB, System: cfg.Units.DefaultSystem, Catalog: src.Catalog, Tr: rec.Tr}
				if _, ds := comp.CompileFile(r.Path); ds.HasErrors() {
					printDiags(cmd.ErrOrStderr(), ds, "text")
					return exitError{}
				}
				for _, l := range langs {
					path := i18n.TranslationPath(r.Path, l)
					existing, err := i18n.LoadRecipeTranslation(path, l)
					if err != nil {
						return err
					}
					if err := i18n.WriteTranslation(path, l, r.Path, rec.Sources, existing, prune); err != nil {
						return err
					}
					todo := 0
					for _, s := range rec.Sources {
						if existing == nil || existing.Strings[s] == "" {
							todo++
						}
					}
					verb := "updated"
					if existing == nil {
						verb = "created"
					}
					fmt.Fprintf(out, "%s %s %s\n", render(out, sOK, fmt.Sprintf("%-7s", verb)), render(out, sPath, path),
						render(out, sMuted, fmt.Sprintf("(%d of %d to translate)", todo, len(rec.Sources))))
					n++
				}
			}
			if n == 0 {
				return errors.New("no matching recipes")
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&dir, "source", "s", ".", "site directory")
	cmd.Flags().StringArrayVar(&langs, "lang", nil, "target language (repeatable; default: all non-source languages)")
	cmd.Flags().BoolVar(&prune, "prune", false, "remove translations whose original text no longer exists")
	return cmd
}

func i18nStatusCmd() *cobra.Command {
	var dir string
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show translation coverage per language",
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := site.Load(dir, site.LoadOptions{Drafts: true})
			if err != nil {
				return printLoadError(cmd, err, "text")
			}
			out := cmd.OutOrStdout()
			cfg := s.Config
			fmt.Fprintf(out, "%s %s\n\n", render(out, sTitle, "Languages:"), strings.Join(cfg.Languages, ", "))
			for i, l := range s.Langs {
				if i == 0 {
					fmt.Fprintf(out, "%s %s (source language)\n", render(out, sTitle, l.Code), l.Catalog.Name)
					continue
				}
				ui := render(out, sOK, "UI translated")
				if _, err := os.Stat(i18n.Path(cfg.Dir, l.Code)); err != nil {
					ui = render(out, sWarn, "no i18n/"+l.Code+".yaml (UI falls back to English); run: get-cooked i18n init "+l.Code)
				}
				fmt.Fprintf(out, "%s %s · %s\n", render(out, sTitle, l.Code), l.Catalog.Name, ui)
				for _, r := range l.Recipes {
					c, ok := l.Coverage[r.Slug]
					switch {
					case !l.Translated[r.Slug]:
						fmt.Fprintf(out, "  %s %s\n", render(out, sMuted, "  —  "), r.Slug)
					case c[0] == c[1]:
						fmt.Fprintf(out, "  %s %s\n", render(out, sOK, " 100%"), r.Slug)
					case ok:
						fmt.Fprintf(out, "  %s %s\n", render(out, sWarn, fmt.Sprintf("%4d%%", c[0]*100/max(c[1], 1))), r.Slug)
					}
				}
				fmt.Fprintln(out)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&dir, "source", "s", ".", "site directory")
	return cmd
}
