package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/RashKash103/get-cooked/docs"
	"github.com/RashKash103/get-cooked/internal/config"
	"github.com/RashKash103/get-cooked/internal/importer"
	"github.com/RashKash103/get-cooked/internal/recipe"
	"github.com/RashKash103/get-cooked/internal/scaffold"
	"github.com/RashKash103/get-cooked/schema"
)

func docsCmd() *cobra.Command {
	return &cobra.Command{
		Use:       "docs [format]",
		Short:     "Print reference documentation (the recipe format)",
		ValidArgs: []string{"format"},
		Args:      cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 && args[0] != "format" {
				return fmt.Errorf("unknown topic %q (available: format)", args[0])
			}
			fmt.Fprint(cmd.OutOrStdout(), docs.RecipeFormat)
			return nil
		},
	}
}

func schemaCmd() *cobra.Command {
	return &cobra.Command{
		Use:       "schema [recipe|ingredients|config|i18n]",
		Short:     "Print a JSON Schema",
		ValidArgs: []string{"recipe", "ingredients", "config", "i18n"},
		Args:      cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := "recipe"
			if len(args) == 1 {
				name = args[0]
			}
			b, err := schema.FS.ReadFile(name + ".schema.json")
			if err != nil {
				return fmt.Errorf("unknown schema %q (available: recipe, ingredients, config, i18n)", name)
			}
			_, err = cmd.OutOrStdout().Write(b)
			return err
		},
	}
}

func initCmd() *cobra.Command {
	var title string
	cmd := &cobra.Command{
		Use:   "init [dir]",
		Short: "Create a new recipe site",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := "."
			if len(args) == 1 {
				dir = args[0]
			}
			if title == "" {
				abs, _ := filepath.Abs(dir)
				title = titleWords(filepath.Base(abs))
			}
			written, err := scaffold.Write(dir, title)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if len(written) == 0 {
				fmt.Fprintln(out, "nothing to do: all starter files already exist")
				return nil
			}
			for _, p := range written {
				fmt.Fprintf(out, "%s %s\n", render(out, sOK, "created"), render(out, sPath, p))
			}
			fmt.Fprintf(out, "\n%s\n  cd %s\n  get-cooked serve          # preview at http://localhost:8080\n  get-cooked new \"My Recipe\"\n  get-cooked skills install # Claude Code skills\n", render(out, sTitle, "Next:"), dir)
			return nil
		},
	}
	cmd.Flags().StringVar(&title, "title", "", "site title (default: the directory name)")
	return cmd
}

func titleWords(s string) string {
	words := strings.Fields(strings.NewReplacer("-", " ", "_", " ").Replace(s))
	for i, w := range words {
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	if len(words) == 0 {
		return "Recipes"
	}
	return strings.Join(words, " ")
}

const newTemplate = `# yaml-language-server: $schema=https://raw.githubusercontent.com/RashKash103/get-cooked/main/schema/recipe.schema.json
title: %s
description: A short, appetizing description.
date: %s

servings: { default: 4, min: 1, max: 12 }

times:
  prep: 10
  cook: 20

ingredients:
  - { id: example, name: example ingredient, amount: 1, unit: cup }

steps:
  - Use @example here. Cook for ~{20}.
`

func newCmd() *cobra.Command {
	var dir, category string
	cmd := &cobra.Command{
		Use:   "new <title>",
		Short: "Create a new recipe file",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			title := strings.Join(args, " ")
			cfg, err := config.Load(dir)
			if err != nil {
				return err
			}
			slug := recipe.Slugify(title, "-")
			if slug == "" {
				return errors.New("the title needs some letters or digits")
			}
			folder := cfg.Path(cfg.Recipes)
			if category != "" {
				folder = filepath.Join(folder, recipe.Slugify(category, "-"))
			}
			path := filepath.Join(folder, slug+".yaml")
			if _, err := os.Stat(path); err == nil {
				return fmt.Errorf("%s already exists", path)
			}
			if err := os.MkdirAll(folder, 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(path, []byte(fmt.Sprintf(newTemplate, title, time.Now().Format("2006-01-02"))), 0o644); err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "%s %s\n", render(out, sOK, "created"), render(out, sPath, path))
			return nil
		},
	}
	cmd.Flags().StringVarP(&dir, "source", "s", ".", "site directory")
	cmd.Flags().StringVarP(&category, "category", "c", "", "category folder")
	return cmd
}

func importCmd() *cobra.Command {
	var outFile string
	cmd := &cobra.Command{
		Use:   "import <url|file.html>",
		Short: "Draft a recipe from a web page's schema.org data",
		Long: "Reads the schema.org Recipe structured data most recipe sites publish and prints a draft\n" +
			"get-cooked recipe. The draft has the raw ingredient lines and steps; refine it by hand or\n" +
			"with the import-recipe Claude skill.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			src := args[0]
			var page []byte
			var err error
			if strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") {
				page, err = importer.Fetch(cmd.Context(), src)
			} else {
				page, err = os.ReadFile(src)
				src = ""
			}
			if err != nil {
				return err
			}
			r, err := importer.Extract(page)
			if err != nil {
				return err
			}
			y, err := r.YAML(src)
			if err != nil {
				return err
			}
			if outFile == "" {
				_, err = cmd.OutOrStdout().Write(y)
				return err
			}
			return os.WriteFile(outFile, y, 0o644)
		},
	}
	cmd.Flags().StringVarP(&outFile, "output", "o", "", "write to a file instead of stdout")
	return cmd
}
