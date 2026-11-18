package cli

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"

	"github.com/RashKash103/get-cooked/internal/skills"
)

func skillsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skills",
		Short: "Install Claude Code skills for working on a recipe site",
		Long: "Downloads the get-cooked Claude Code skills (import-recipe, recipe-illustration,\n" +
			"add-site-language, translate-recipe). Release builds download the skills published with\n" +
			"their release; development builds use the latest main branch.",
	}
	cmd.AddCommand(skillsInstallCmd(), skillsListCmd(), skillsPackCmd())
	return cmd
}

func fetchSkills(cmd *cobra.Command) ([]*skills.Skill, skills.Source, error) {
	src := skills.SourceFor(Version)
	b, err := skills.Download(cmd.Context(), src.URL)
	if err != nil {
		return nil, src, err
	}
	all, err := skills.Parse(b)
	return all, src, err
}

func skillsListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List available skills",
		RunE: func(cmd *cobra.Command, _ []string) error {
			all, src, err := fetchSkills(cmd)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "%s %s\n\n", render(out, sMuted, "from"), src.Label)
			for _, s := range all {
				fmt.Fprintf(out, "%s\n  %s\n", render(out, sTitle, s.Name), s.Description)
			}
			return nil
		},
	}
}

func skillsInstallCmd() *cobra.Command {
	var dir string
	var yes bool
	cmd := &cobra.Command{
		Use:   "install [skill...]",
		Short: "Install skills into a directory (default ./.claude/skills)",
		Example: "  get-cooked skills install\n" +
			"  get-cooked skills install import-recipe translate-recipe\n" +
			"  get-cooked skills install --dir ~/.claude/skills --yes",
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			target, err := filepath.Abs(expandHome(dir))
			if err != nil {
				return err
			}
			src := skills.SourceFor(Version)
			fmt.Fprintf(out, "%s %s\n", render(out, sMuted, "Downloading skills from"), src.Label+"…")
			all, _, err := fetchSkills(cmd)
			if err != nil {
				return err
			}
			list, err := skills.Select(all, args)
			if err != nil {
				return err
			}
			var lines []string
			for _, s := range list {
				lines = append(lines, render(out, sTitle, s.Name)+" "+render(out, sMuted, s.Description))
			}
			fmt.Fprintln(out, render(out, sBorder, "Install to "+render(out, sPath, target)+"\n\n"+strings.Join(lines, "\n")))
			if ex := skills.Existing(target, list); len(ex) > 0 {
				fmt.Fprintf(out, "%s replaces existing: %s\n", render(out, sWarn, "note:"), strings.Join(ex, ", "))
			}
			if !yes {
				if !term.IsTerminal(os.Stdin.Fd()) {
					return errors.New("not a terminal: pass --yes to confirm the install location")
				}
				ok := false
				err := huh.NewConfirm().
					Title(fmt.Sprintf("Install %d skill(s) into %s?", len(list), target)).
					Affirmative("Install").Negative("Cancel").Value(&ok).Run()
				if err != nil {
					return err
				}
				if !ok {
					fmt.Fprintln(out, "cancelled")
					return nil
				}
			}
			if err := skills.Install(target, list); err != nil {
				return err
			}
			fmt.Fprintf(out, "%s installed %d skill(s) into %s\n", render(out, sOK, "✓"), len(list), render(out, sPath, target))
			return nil
		},
	}
	cmd.Flags().StringVarP(&dir, "dir", "d", filepath.Join(".claude", "skills"), "directory to install into")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "don't ask for confirmation")
	return cmd
}

func skillsPackCmd() *cobra.Command {
	var out string
	cmd := &cobra.Command{
		Use:    "pack <skills-dir>",
		Short:  "Package a skills directory into the release asset (maintainers)",
		Hidden: true,
		Args:   cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var buf bytes.Buffer
			if err := skills.Pack(args[0], &buf); err != nil {
				return err
			}
			if _, err := skills.Parse(buf.Bytes()); err != nil {
				return err
			}
			return os.WriteFile(out, buf.Bytes(), 0o644)
		},
	}
	cmd.Flags().StringVarP(&out, "output", "o", skills.AssetName, "output file")
	return cmd
}

func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if h, err := os.UserHomeDir(); err == nil {
			return filepath.Join(h, strings.TrimPrefix(p, "~"))
		}
	}
	return p
}
