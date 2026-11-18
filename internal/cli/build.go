package cli

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/RashKash103/get-cooked/internal/site"
)

func buildCmd() *cobra.Command {
	var dir, out, baseURL, format string
	var drafts, clean, noSweep bool
	cmd := &cobra.Command{
		Use:   "build",
		Short: "Build the static site",
		Example: "  get-cooked build\n" +
			"  get-cooked build -s my-recipes -o public --base-url https://me.github.io/recipes/",
		RunE: func(cmd *cobra.Command, _ []string) error {
			start := time.Now()
			s, err := site.Load(dir, site.LoadOptions{Drafts: drafts, Sweep: !noSweep})
			if err != nil {
				return printLoadError(cmd, err, format)
			}
			printDiags(cmd.ErrOrStderr(), s.Diags, format)
			if s.Diags.HasErrors() {
				return exitError{}
			}
			if out == "" {
				out = s.Config.Path("public")
			}
			if clean {
				if err := os.RemoveAll(out); err != nil {
					return err
				}
			}
			if err := site.Build(s, site.BuildOptions{Out: out, BaseURL: baseURL}); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Built %d recipes into %s in %s\n", len(s.Recipes), out, time.Since(start).Round(time.Millisecond))
			return nil
		},
	}
	cmd.Flags().StringVarP(&dir, "source", "s", ".", "site directory")
	cmd.Flags().StringVarP(&out, "output", "o", "", "output directory (default: <source>/public)")
	cmd.Flags().StringVar(&baseURL, "base-url", "", "override base_url from get-cooked.yaml")
	cmd.Flags().BoolVar(&drafts, "drafts", false, "include draft recipes")
	cmd.Flags().BoolVar(&clean, "clean", true, "empty the output directory first")
	cmd.Flags().BoolVar(&noSweep, "fast", false, "skip the full validation sweep")
	cmd.Flags().StringVar(&format, "format", "text", "diagnostics format: text, github or json")
	return cmd
}
