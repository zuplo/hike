package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ntotten/zproj/internal/config"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Create a new zproj.yaml configuration file",
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}

		cfgPath := filepath.Join(cwd, config.ConfigFile)

		// Prompt only if file already exists
		if _, err := os.Stat(cfgPath); err == nil {
			if !promptYesNo(fmt.Sprintf("%s already exists. Overwrite?", config.ConfigFile)) {
				fmt.Println("Aborted.")
				return nil
			}
		}

		if err := os.WriteFile(cfgPath, []byte(exampleConfig), 0644); err != nil {
			return fmt.Errorf("writing config: %w", err)
		}

		fmt.Printf("Created %s\n", cfgPath)
		fmt.Println("Edit the file to add your repos, then run 'zproj sync' to clone them.")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
}

const exampleConfig = `# Git provider defaults — repos can use just the name (e.g. "my-app")
# instead of full URLs when org is set.
git:
  # provider: github  # github (default), gitlab, bitbucket, or any host
  # host: github.com  # auto-detected from provider, or set for self-hosted
  org: your-org
  # ssh: true         # use SSH URLs (default: false, uses HTTPS)

# Each group contains a set of repos that are cloned and managed together.
# Every group gets its own [groupname]/ directory.
# Set 'default: true' on a group to use it when --group is not specified.
groups:
  platform:
    default: true
    # aliases: [plat]  # Optional: use 'plat' as shorthand for 'platform'
    repos:
      # With git.org set, just use the repo name:
      - my-app
      - shared-lib

      # Or use a full URL when needed:
      # - git@github.com:other-org/special-repo.git

      # Object form lets you override name or branch:
      # - repo: api
      #   name: api
      #   branch: develop

  # Add more groups to organize repos separately:
  # backend:
  #   aliases: [be]
  #   repos:
  #     - api-service
  #     - worker

# Lifecycle hooks — run after creating a project.
# Repo-level overrides group-level, which overrides global.
# hooks:
#   onCreate: npm install

# Optional: variables available in .template/ files
# templates:
#   variables:
#     ORG: your-org
`

func promptYesNo(question string) bool {
	reader := bufio.NewReader(os.Stdin)
	fmt.Printf("%s [y/N] ", question)
	answer, _ := reader.ReadString('\n')
	answer = strings.TrimSpace(strings.ToLower(answer))
	return answer == "y" || answer == "yes"
}
