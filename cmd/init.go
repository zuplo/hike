package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ntotten/zproj/internal/config"
	"github.com/ntotten/zproj/internal/git"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize: create config and clone repos into .main directories",
	RunE: func(cmd *cobra.Command, args []string) error {
		// If no config exists, offer to create one interactively
		if cfg == nil {
			return runInitInteractive()
		}
		return runInit()
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
}

const exampleConfig = `# Each group contains a set of repos that are cloned and managed together.
# Every group gets its own [groupname]/ directory.
# Set 'default: true' on a group to use it when --group is not specified.
groups:
  platform:
    default: true
    # aliases: [plat]  # Optional: use 'plat' as shorthand for 'platform'
    repos:
      # Simplest form: just a git URL.
      # Name is derived from the URL (e.g. "my-app"), branch defaults to "main".
      - git@github.com:your-org/my-app.git
      - git@github.com:your-org/shared-lib.git

      # Object form lets you override name or branch:
      # - url: git@github.com:your-org/api.git
      #   name: api
      #   branch: develop

  # Add more groups to organize repos separately:
  # backend:
  #   aliases: [be]
  #   repos:
  #     - git@github.com:your-org/api-service.git
  #     - git@github.com:your-org/worker.git

# Optional: variables available in .template/ files
# templates:
#   variables:
#     ORG: your-org
`

func runInitInteractive() error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	cfgPath := filepath.Join(cwd, config.ConfigFile)
	fmt.Printf("No %s found in current directory.\n\n", config.ConfigFile)

	if !promptYesNo("Create a new configuration file here?") {
		fmt.Println("Aborted.")
		return nil
	}

	if err := os.WriteFile(cfgPath, []byte(exampleConfig), 0644); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}

	fmt.Printf("\nCreated %s\n", cfgPath)
	fmt.Println("Edit the file to add your repos, then run 'zproj init' again to clone them.")
	return nil
}

func promptYesNo(question string) bool {
	reader := bufio.NewReader(os.Stdin)
	fmt.Printf("%s [Y/n] ", question)
	answer, _ := reader.ReadString('\n')
	answer = strings.TrimSpace(strings.ToLower(answer))
	return answer == "" || answer == "y" || answer == "yes"
}

func runInit() error {
	for groupName, group := range cfg.Groups {
		mainDir := config.MainDir(rootDir, groupName)
		if err := os.MkdirAll(mainDir, 0755); err != nil {
			return fmt.Errorf("creating .main for group %q: %w", groupName, err)
		}

		fmt.Printf("Initializing group %q (%d repos)...\n", groupName, len(group.Repos))

		results := git.RunParallel(group.Repos, func(repo config.Repo) git.Result {
			repoName := repo.RepoName()
			dest := filepath.Join(mainDir, repoName)

			if _, err := os.Stat(dest); err == nil {
				return git.Result{Repo: repoName, Output: "already exists, skipping"}
			}

			fmt.Printf("  Cloning %s...\n", repoName)
			if err := git.Clone(repo.URL, dest, repo.RepoBranch()); err != nil {
				return git.Result{Repo: repoName, Err: err}
			}
			return git.Result{Repo: repoName, Output: "cloned"}
		})

		var errs []string
		for _, r := range results {
			if r.Err != nil {
				errs = append(errs, fmt.Sprintf("  %s: %v", r.Repo, r.Err))
			} else {
				fmt.Printf("  %s: %s\n", r.Repo, r.Output)
			}
		}
		if len(errs) > 0 {
			fmt.Fprintf(os.Stderr, "Errors in group %q:\n%s\n", groupName, strings.Join(errs, "\n"))
		}
	}

	fmt.Println("Init complete.")
	return nil
}
