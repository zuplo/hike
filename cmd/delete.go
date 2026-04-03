package cmd

import (
	"fmt"

	"github.com/ntotten/zproj/internal/project"
	"github.com/spf13/cobra"
)

var deleteCmd = &cobra.Command{
	Use:   "delete [name]",
	Short: "Delete a project and its worktrees",
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
		fmt.Printf("Deleting project %q from group %q...\n", name, group)
		if err := project.Delete(rootDir, cfg, name, group); err != nil {
			return err
		}
		fmt.Printf("Project %q deleted.\n", name)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(deleteCmd)
}
