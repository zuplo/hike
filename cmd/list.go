package cmd

import (
	"fmt"

	"github.com/ntotten/zproj/internal/project"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:     "list",
	Short:   "List all projects",
	Aliases: []string{"ls"},
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireConfig(); err != nil {
			return err
		}

		groups := make(map[string]struct{})
		if groupArgIsExplicit() {
			g, err := resolveGroup()
			if err != nil {
				return err
			}
			groups[g] = struct{}{}
		} else {
			for g := range cfg.Groups {
				groups[g] = struct{}{}
			}
		}

		found := false
		for g := range groups {
			projects, err := project.List(rootDir, g)
			if err != nil {
				return err
			}
			if len(projects) == 0 {
				continue
			}
			found = true
			if len(cfg.Groups) > 1 {
				fmt.Printf("[%s]\n", g)
			}
			for _, p := range projects {
				fmt.Printf("  %s\n", p)
			}
		}

		if !found {
			fmt.Println("No projects found.")
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(listCmd)
}
