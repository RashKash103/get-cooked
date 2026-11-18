// Package cli implements the get-cooked command line interface.
package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/RashKash103/get-cooked/internal/config"
	"github.com/RashKash103/get-cooked/internal/recipe"
	"github.com/RashKash103/get-cooked/internal/runtime"
	"github.com/RashKash103/get-cooked/internal/site"
)

// Build information, set via -ldflags.
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

func init() {
	// `go install …@v1.2.3` builds have no -ldflags; take the version from
	// the module build info so `version` and `skills install` still match
	// the release.
	if Version != "dev" {
		return
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return
	}
	if v := info.Main.Version; v != "" && v != "(devel)" {
		Version = strings.TrimPrefix(v, "v")
	}
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			if Commit == "none" && len(s.Value) >= 7 {
				Commit = s.Value[:7]
			}
		case "vcs.time":
			if Date == "unknown" {
				Date = s.Value
			}
		}
	}
}

// Execute runs the CLI and returns the process exit code.
func Execute(args []string, stdout, stderr io.Writer) int {
	root := NewRoot(stdout, stderr)
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		if _, ok := err.(exitError); !ok {
			fmt.Fprintln(stderr, "error:", err)
		}
		return 1
	}
	return 0
}

// exitError signals failure after diagnostics were already printed.
type exitError struct{}

func (exitError) Error() string { return "failed" }

// NewRoot builds the root command.
func NewRoot(stdout, stderr io.Writer) *cobra.Command {
	root := &cobra.Command{
		Use:           "get-cooked",
		Short:         "Build a recipe website from programmable YAML recipes",
		Long:          "get-cooked turns a folder of YAML recipes into a fast, mobile-first static recipe website.\nRecipes can scale with servings, swap equipment, and react to reader-selected options.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.AddCommand(
		buildCmd(), serveCmd(), validateCmd(), renderCmd(), initCmd(), newCmd(),
		importCmd(), schemaCmd(), i18nCmd(), skillsCmd(), docsCmd(), exportCmd(), versionCmd(),
	)
	return root
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, _ []string) {
			fmt.Fprintf(cmd.OutOrStdout(), "get-cooked %s (commit %s, built %s)\n", Version, Commit, Date)
		},
	}
}

func validateCmd() *cobra.Command {
	var dir, format string
	var drafts, strict bool
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Check recipes, the ingredient database and config for errors",
		Long: "Validates every recipe against the schema, checks expressions and references, and renders each\n" +
			"recipe across servings, options and unit systems to catch problems that only appear at runtime.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := site.Load(dir, site.LoadOptions{Drafts: drafts, Sweep: true, Strict: strict})
			if err != nil {
				return printLoadError(cmd, err, format)
			}
			printDiags(cmd.OutOrStdout(), s.Diags, format)
			if s.Diags.HasErrors() {
				return exitError{}
			}
			if format == "text" {
				fmt.Fprintf(cmd.OutOrStdout(), "✓ %d recipes OK\n", len(s.Recipes))
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&dir, "source", "s", ".", "site directory")
	cmd.Flags().StringVar(&format, "format", "text", "output format: text, github or json")
	cmd.Flags().BoolVar(&drafts, "drafts", true, "also validate drafts")
	cmd.Flags().BoolVar(&strict, "strict", false, "also warn about ingredients that can't convert units")
	return cmd
}

func printLoadError(cmd *cobra.Command, err error, format string) error {
	if ds, ok := err.(recipe.Diags); ok {
		printDiags(cmd.OutOrStdout(), ds, format)
		return exitError{}
	}
	return err
}

func printDiags(w io.Writer, ds recipe.Diags, format string) {
	switch format {
	case "json":
		if ds == nil {
			ds = recipe.Diags{}
		}
		b, _ := json.MarshalIndent(ds, "", "  ")
		fmt.Fprintln(w, string(b))
	case "github":
		for _, d := range ds {
			level := "error"
			if d.Warn {
				level = "warning"
			}
			fmt.Fprintf(w, "::%s file=%s,line=%d,col=%d::%s\n", level, d.File, max(d.Line, 1), max(d.Col, 1), escapeGH(d.Msg))
		}
	default:
		for _, d := range ds {
			fmt.Fprintln(w, d.Error())
		}
	}
}

func escapeGH(s string) string {
	return strings.NewReplacer("%", "%25", "\r", "%0D", "\n", "%0A").Replace(s)
}

func renderCmd() *cobra.Command {
	var servings float64
	var sets []string
	var system, dir, lang string
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "render <recipe.yaml>",
		Short: "Render one recipe to the terminal for a given state",
		Example: "  get-cooked render recipes/breakfast/oatmeal.yaml --servings 2 --set cranberries=false --units metric\n" +
			"  get-cooked render recipes/cookies.yaml --json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if dir == "" {
				dir = findSiteRoot(args[0])
			}
			s, err := site.Load(dir, site.LoadOptions{Drafts: true})
			if err != nil {
				return printLoadError(cmd, err, "text")
			}
			abs, _ := filepath.Abs(args[0])
			recipes := s.Recipes
			if lang != "" {
				found := false
				for _, l := range s.Langs {
					if l.Code == lang {
						recipes, found = l.Recipes, true
					}
				}
				if !found {
					return fmt.Errorf("language %q is not in `languages:` (%s)", lang, strings.Join(s.Config.Languages, ", "))
				}
			}
			var r *recipe.Recipe
			for _, x := range recipes {
				if p, _ := filepath.Abs(x.Path); p == abs {
					r = x
				}
			}
			if r == nil {
				var mine recipe.Diags
				for _, d := range s.Diags {
					if p, _ := filepath.Abs(d.File); p == abs {
						mine = append(mine, d)
					}
				}
				if len(mine) == 0 {
					return fmt.Errorf("%s is not a recipe under %s", args[0], s.Config.Dir)
				}
				printDiags(cmd.ErrOrStderr(), mine, "text")
				return exitError{}
			}
			st := runtime.State{Servings: servings, System: system, Options: map[string]any{}}
			for _, kv := range sets {
				k, v, ok := strings.Cut(kv, "=")
				if !ok {
					return fmt.Errorf("--set expects id=value, got %q", kv)
				}
				st.Options[k] = parseValue(v)
			}
			m, err := runtime.Render(r.C, st)
			if err != nil {
				return err
			}
			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(m)
			}
			writeText(cmd.OutOrStdout(), r, m)
			return nil
		},
	}
	cmd.Flags().Float64Var(&servings, "servings", 0, "servings (default: the recipe's default)")
	cmd.Flags().StringArrayVar(&sets, "set", nil, "set an option, e.g. --set cranberries=false (repeatable)")
	cmd.Flags().StringVar(&system, "units", "", "unit system: original, us or metric")
	cmd.Flags().StringVarP(&dir, "source", "s", "", "site directory (default: nearest parent with get-cooked.yaml)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "print the rendered model as JSON")
	cmd.Flags().StringVar(&lang, "lang", "", "language (default: the source language)")
	return cmd
}

func parseValue(v string) any {
	switch v {
	case "true", "yes", "on":
		return true
	case "false", "no", "off":
		return false
	}
	if f, err := strconv.ParseFloat(v, 64); err == nil {
		return f
	}
	return v
}

// findSiteRoot walks up from a recipe file to the directory containing
// get-cooked.yaml (or ingredients.yaml), defaulting to ".".
func findSiteRoot(file string) string {
	dir := file
	for i := 0; i < 10; i++ {
		next := parentDir(dir)
		if next == dir {
			break
		}
		dir = next
		for _, marker := range []string{config.FileName, "ingredients.yaml"} {
			if _, err := os.Stat(dir + string(os.PathSeparator) + marker); err == nil {
				return dir
			}
		}
	}
	return "."
}

func parentDir(p string) string {
	i := strings.LastIndexAny(p, `/\`)
	switch {
	case i < 0:
		if p == "." {
			return p
		}
		return "."
	case i == 0:
		return p[:1]
	}
	return p[:i]
}

func writeText(w io.Writer, r *recipe.Recipe, m *runtime.Model) {
	fmt.Fprintf(w, "%s — %s (units: %s)\n", r.Title, m.Yield, m.System)
	var times []string
	for _, t := range m.Times {
		times = append(times, t.Label+" "+t.Text)
	}
	if len(times) > 0 {
		fmt.Fprintln(w, strings.Join(times, " · "))
	}
	if len(m.Equipment) > 0 {
		fmt.Fprintln(w, "\nEquipment")
		for _, e := range m.Equipment {
			fmt.Fprintf(w, "  • %s\n", e.Name)
		}
	}
	fmt.Fprintln(w, "\nIngredients")
	for _, s := range m.Ingredients {
		indent := "  "
		if s.Title != "" {
			fmt.Fprintf(w, "  %s\n", s.Title)
			indent = "    "
		}
		for _, it := range s.Items {
			line := it.Name
			if it.Q != nil {
				line = it.Q.String() + it.J + it.Name
			}
			if it.Note != "" {
				line += ", " + it.Note
			}
			fmt.Fprintf(w, "%s• %s\n", indent, line)
		}
	}
	fmt.Fprintln(w, "\nSteps")
	for _, s := range m.Steps {
		if s.Title != "" {
			fmt.Fprintf(w, "  %s\n", s.Title)
		}
		for _, st := range s.Items {
			var sb strings.Builder
			for _, seg := range st.Segs {
				switch {
				case seg.T == "ing" && seg.Q != "":
					sb.WriteString(seg.Q + seg.J + seg.V)
				case seg.T == "timer":
					sb.WriteString("⏲ " + seg.V)
				default:
					sb.WriteString(seg.V)
				}
			}
			fmt.Fprintf(w, "  %d. %s\n", st.N, sb.String())
			if st.Note != "" {
				fmt.Fprintf(w, "     ↳ %s\n", st.Note)
			}
		}
	}
	if len(m.Notes) > 0 {
		fmt.Fprintln(w, "\nNotes")
		icons := map[string]string{"warning": "⚠", "tip": "★", "info": "ℹ"}
		for _, n := range m.Notes {
			fmt.Fprintf(w, "  %s %s\n", icons[n.Kind], n.Text)
		}
	}
}
