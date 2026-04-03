package cmd

import (
	"fmt"

	"github.com/ntotten/zproj/internal/project"
	"github.com/spf13/cobra"
)

var createCmd = &cobra.Command{
	Use:   "create [name]",
	Short: "Create a new project with worktrees",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runCreate(args[0])
	},
}

func init() {
	rootCmd.AddCommand(createCmd)
}

func runCreate(name string) error {
	if err := requireConfig(); err != nil {
		return err
	}
	group, err := resolveGroup()
	if err != nil {
		return err
	}
	fmt.Printf("Creating project %q in group %q...\n", name, group)
	if err := project.Create(rootDir, cfg, name, group, colorArg); err != nil {
		return err
	}
	fmt.Printf("Project %q created successfully.\n", name)
	return nil
}
