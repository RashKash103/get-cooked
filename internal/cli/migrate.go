package cli

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/RashKash103/get-cooked/internal/config"
	"github.com/RashKash103/get-cooked/internal/migrate"
)

func migrateCmd() *cobra.Command {
	var dir string
	var dryRun, list bool
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Upgrade a site's files to the current format",
		Long: "Applies format migrations to the site's ingredient databases, recipes, config and\n" +
			"translations, editing files in place (comments and formatting are kept). Safe to run\n" +
			"repeatedly: files that are already up to date are left alone.",
		Example: "  get-cooked migrate --dry-run\n  get-cooked migrate",
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			if list {
				for _, m := range migrate.All {
					fmt.Fprintf(out, "%s %s %s\n", render(out, sTitle, m.ID), render(out, sMuted, "(since "+m.Since+")"), m.Description)
				}
				return nil
			}
			cfg, err := config.Load(dir)
			if err != nil {
				return err
			}
			changes, err := migrate.Run(cfg, dryRun)
			if err != nil {
				return err
			}
			if len(changes) == 0 {
				fmt.Fprintln(out, render(out, sOK, "✓")+" everything is up to date")
				return nil
			}
			verb := "migrated"
			if dryRun {
				verb = "would migrate"
			}
			for _, c := range changes {
				rel, err := filepath.Rel(cfg.Dir, c.File)
				if err != nil {
					rel = c.File
				}
				fmt.Fprintf(out, "%s %s %s\n", render(out, sOK, verb), render(out, sPath, rel),
					render(out, sMuted, fmt.Sprintf("(%s, %d change(s))", c.Migration, c.Count)))
			}
			if dryRun {
				fmt.Fprintln(out, render(out, sMuted, "dry run: nothing was written"))
			} else {
				fmt.Fprintln(out, render(out, sMuted, "next: get-cooked validate"))
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&dir, "source", "s", ".", "site directory")
	cmd.Flags().BoolVarP(&dryRun, "dry-run", "n", false, "show what would change without writing")
	cmd.Flags().BoolVar(&list, "list", false, "list all migrations")
	return cmd
}
