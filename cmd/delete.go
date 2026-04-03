package cmd

import (
	"fmt"
	"os"

	"github.com/ntotten/zproj/internal/project"
	"github.com/spf13/cobra"
)

var deleteCmd = &cobra.Command{
	Use:   "delete [project-name]",
	Short: "Delete a project and its worktrees",
	Long:  "Delete a project. If no name given and you're inside a project, deletes the current one.",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireConfig(); err != nil {
			return err
		}

		var projectName string
		if len(args) == 1 {
			projectName = args[0]
		} else {
			// Try to detect from cwd
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			_, name, err := project.DetectProject(cwd, rootDir)
			if err != nil {
				return fmt.Errorf("no project name given and not inside a project\n\nUsage: zproj delete <project-name>")
			}
			projectName = name
		}

		fmt.Printf("Deleting project %q...\n", projectName)
		if err := project.Delete(rootDir, cfg, projectName); err != nil {
			return err
		}
		fmt.Printf("Project %q deleted.\n", projectName)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(deleteCmd)
}
