package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/zuplo/hike/internal/config"
	"github.com/zuplo/hike/internal/git"
	"github.com/spf13/cobra"
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Clone missing repos and sync all .main repos to latest origin/HEAD",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireConfig(); err != nil {
			return err
		}
		return runSync()
	},
}

func init() {
	rootCmd.AddCommand(syncCmd)
}

func runSync() error {
	groups := cfg.Groups
	if groupArgIsExplicit() {
		g, err := resolveGroup()
		if err != nil {
			return err
		}
		grp, ok := cfg.Groups[g]
		if !ok {
			return fmt.Errorf("group %q not found", g)
		}
		groups = map[string]config.Group{g: grp}
	}

	for groupName, group := range groups {
		mainDir := config.MainDir(rootDir, groupName)
		if err := os.MkdirAll(mainDir, 0755); err != nil {
			return fmt.Errorf("creating .main for group %q: %w", groupName, err)
		}

		fmt.Printf("Syncing group %q (%d repos)...\n", groupName, len(group.Repos))

		results := git.RunParallel(group.Repos, func(repo config.Repo) git.Result {
			repoName := repo.RepoName()
			repoDir := filepath.Join(mainDir, repoName)

			// Clone if not present
			if _, err := os.Stat(repoDir); os.IsNotExist(err) {
				fmt.Printf("  Cloning %s...\n", repoName)
				if err := git.Clone(repo.URL, repoDir, repo.RepoBranch()); err != nil {
					return git.Result{Repo: repoName, Err: fmt.Errorf("clone: %w", err)}
				}
				return git.Result{Repo: repoName, Output: "cloned"}
			}

			// Already cloned — fetch and reset
			if err := git.Fetch(repoDir); err != nil {
				return git.Result{Repo: repoName, Err: fmt.Errorf("fetch: %w", err)}
			}
			if err := git.ResetToOriginHead(repoDir, repo.RepoBranch()); err != nil {
				return git.Result{Repo: repoName, Err: fmt.Errorf("reset: %w", err)}
			}
			return git.Result{Repo: repoName, Output: "synced"}
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
			fmt.Fprintf(os.Stderr, "Errors:\n%s\n", strings.Join(errs, "\n"))
		}
	}

	fmt.Println("Sync complete.")
	return nil
}

func groupArgIsExplicit() bool {
	return rootCmd.PersistentFlags().Changed("group")
}
