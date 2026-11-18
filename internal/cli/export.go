package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/RashKash103/get-cooked/internal/export"
	"github.com/RashKash103/get-cooked/internal/recipe"
	"github.com/RashKash103/get-cooked/internal/runtime"
	"github.com/RashKash103/get-cooked/internal/site"
)

func exportCmd() *cobra.Command {
	var servings float64
	var sets []string
	var system, dir, lang, format, out, paper string
	cmd := &cobra.Command{
		Use:   "export <recipe.yaml>",
		Short: "Export a recipe as Markdown, text, Typst or PDF for given servings and options",
		Example: "  get-cooked export recipes/breakfast/oatmeal.yaml -f pdf --servings 2 --set cranberries=false\n" +
			"  get-cooked export recipes/cookies.yaml -f md --units metric --lang es -o cookies.md",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if dir == "" {
				dir = findSiteRoot(args[0])
			}
			s, err := site.Load(dir, site.LoadOptions{Drafts: true})
			if err != nil {
				return printLoadError(cmd, err, "text")
			}
			l := s.Langs[0]
			if lang != "" {
				l = nil
				for _, x := range s.Langs {
					if x.Code == lang {
						l = x
					}
				}
				if l == nil {
					return fmt.Errorf("language %q is not in `languages:` (%s)", lang, strings.Join(s.Config.Languages, ", "))
				}
			}
			abs, _ := filepath.Abs(args[0])
			var r *recipe.Recipe
			for _, x := range l.Recipes {
				if p, _ := filepath.Abs(x.Path); p == abs {
					r = x
				}
			}
			if r == nil {
				printDiags(cmd.ErrOrStderr(), s.Diags, "text")
				return fmt.Errorf("%s is not a valid recipe in %s", args[0], s.Config.Dir)
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
			opts := map[string]any{}
			for _, o := range r.C.Options {
				opts[o.ID] = o.Default
			}
			for k, v := range st.Options {
				opts[k] = v
			}
			d := export.Doc{R: r, M: m, Cat: l.Catalog, Opts: opts}
			if format == "" {
				format = strings.TrimPrefix(filepath.Ext(out), ".")
			}
			var data []byte
			switch format {
			case "", "md", "markdown":
				format = "md"
				data = []byte(export.Markdown(d))
			case "txt", "text":
				data = []byte(export.Text(d))
			case "typ", "typst":
				data = []byte(export.Typst(d, paper))
			case "pdf":
				data, err = export.PDF(cmd.Context(), export.Typst(d, paper))
				if err != nil {
					return err
				}
				if out == "" {
					out = r.Slug + ".pdf"
				}
			default:
				return fmt.Errorf("unknown format %q (md, txt, typ, pdf)", format)
			}
			if out == "" || out == "-" {
				_, err = cmd.OutOrStdout().Write(data)
				return err
			}
			if err := os.WriteFile(out, data, 0o644); err != nil {
				return err
			}
			w := cmd.ErrOrStderr()
			fmt.Fprintf(w, "%s %s\n", render(w, sOK, "wrote"), render(w, sPath, out))
			return nil
		},
	}
	cmd.Flags().StringVarP(&format, "format", "f", "", "md, txt, typ or pdf (default: from --output, else md)")
	cmd.Flags().StringVarP(&out, "output", "o", "", "output file (default: stdout; PDF defaults to <slug>.pdf)")
	cmd.Flags().Float64Var(&servings, "servings", 0, "servings")
	cmd.Flags().StringArrayVar(&sets, "set", nil, "set an option, e.g. --set vegan=true")
	cmd.Flags().StringVar(&system, "units", "", "original, us or metric")
	cmd.Flags().StringVar(&lang, "lang", "", "language")
	cmd.Flags().StringVar(&paper, "paper", "a4", "PDF paper: a4, us-letter or a5")
	cmd.Flags().StringVarP(&dir, "source", "s", "", "site directory (default: nearest parent with get-cooked.yaml)")
	return cmd
}
