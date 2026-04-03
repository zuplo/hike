package cmd

import (
	"fmt"

	"github.com/ntotten/zproj/internal/project"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status [name]",
	Short: "Show status of a project's repos",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireConfig(); err != nil {
			return err
		}

		name := args[0]
		group, err := resolveGroup()
		if err != nil {
			return err
		}
		statuses, err := project.GetStatus(rootDir, cfg, name, group)
		if err != nil {
			return err
		}

		fmt.Printf("Project: %s (group: %s)\n\n", name, group)
		for _, s := range statuses {
			dirty := ""
			if s.Dirty {
				dirty = " [dirty]"
			}
			ab := ""
			if s.AheadBehind != "" && s.AheadBehind != "0\t0" {
				ab = fmt.Sprintf(" (%s)", s.AheadBehind)
			}
			fmt.Printf("  %-20s branch: %-20s%s%s\n", s.Repo, s.Branch, dirty, ab)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
