package cmd

import (
	"fmt"

	"github.com/zuplo/hike/internal/project"
	"github.com/spf13/cobra"
)

var pushCmd = &cobra.Command{
	Use:   "push [project-name]",
	Short: "Push all repos in a project",
	Long:  "Runs git push on all repos. If no name given and you're inside a project, uses the current one.",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireConfig(); err != nil {
			return err
		}

		projectName, err := resolveProjectName(args)
		if err != nil {
			return err
		}

		fmt.Printf("Pushing %s...\n", projectName)
		results, err := project.Push(rootDir, cfg, projectName)
		if err != nil {
			return err
		}

		for _, r := range results {
			if r.Err != nil {
				fmt.Printf("  %s: %v\n", r.Repo, r.Err)
			} else {
				fmt.Printf("  %s: %s\n", r.Repo, r.Output)
			}
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(pushCmd)
}
