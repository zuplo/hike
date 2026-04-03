package cmd

import (
	"fmt"
	"os"

	"github.com/zuplo/hike/internal/project"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status [project-name]",
	Short: "Show status of a project's repos",
	Long:  "Show git status. If no name given and you're inside a project, uses the current one.",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireConfig(); err != nil {
			return err
		}

		projectName, err := resolveProjectName(args)
		if err != nil {
			return err
		}

		statuses, err := project.GetStatus(rootDir, cfg, projectName)
		if err != nil {
			return err
		}

		fmt.Printf("Project: %s\n\n", projectName)
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

// resolveProjectName gets project name from args or detects from cwd.
func resolveProjectName(args []string) (string, error) {
	if len(args) >= 1 {
		return args[0], nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	_, name, err := project.DetectProject(cwd, rootDir)
	if err != nil {
		return "", fmt.Errorf("no project name given and not inside a project")
	}
	return name, nil
}
