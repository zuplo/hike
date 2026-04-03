package cmd

import (
	"fmt"

	"github.com/zuplo/hike/internal/config"
	"github.com/zuplo/hike/internal/names"
	"github.com/zuplo/hike/internal/project"
	"github.com/spf13/cobra"
)

var createCmd = &cobra.Command{
	Use:   "create [group] [name]",
	Short: "Create a new project with worktrees",
	Args:  cobra.MaximumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		group, name := resolveCreateArgs(args)
		return runCreateWithArgs(group, name)
	},
}

func init() {
	rootCmd.AddCommand(createCmd)
}

func runCreateWithArgs(groupInput, name string) error {
	if err := requireConfig(); err != nil {
		return err
	}

	// Resolve group
	var group string
	if groupInput != "" {
		resolved, ok := cfg.ResolveGroup(groupInput)
		if !ok {
			return fmt.Errorf("group %q not found in config", groupInput)
		}
		group = resolved
	} else if cfg.DefaultGroup() != "" {
		group = cfg.DefaultGroup()
	} else {
		return fmt.Errorf("no group specified and no default group set in config\n\nSet a default group in %s:\n  groups:\n    mygroup:\n      default: true", "hike.yaml")
	}

	// Generate name if not provided
	if name == "" {
		name = names.Generate()
	}

	projectName := config.ProjectName(group, name)

	color := colorArg
	if color == "random" {
		color = project.RandomColor()
	}

	fmt.Printf("Creating project %q in group %q...\n", projectName, group)
	if err := project.Create(rootDir, cfg, projectName, group, color); err != nil {
		return err
	}
	fmt.Printf("Project %q created successfully.\n", projectName)
	return nil
}
