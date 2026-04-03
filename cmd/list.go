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

		projects, err := project.List(rootDir)
		if err != nil {
			return err
		}

		if len(projects) == 0 {
			fmt.Println("No projects found.")
			return nil
		}
		for _, p := range projects {
			fmt.Println(p)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(listCmd)
}
