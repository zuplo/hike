package git

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"sync"
)

type Result struct {
	Repo   string
	Output string
	Err    error
}

func run(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %s: %w", strings.Join(args, " "), strings.TrimSpace(stderr.String()), err)
	}
	return strings.TrimSpace(stdout.String()), nil
}

// Clone clones a repo to the given directory.
func Clone(url, dest, branch string) error {
	_, err := run(".", "clone", "--branch", branch, url, dest)
	return err
}

// Fetch fetches all remotes in the given repo directory.
func Fetch(repoDir string) error {
	_, err := run(repoDir, "fetch", "--all", "--prune")
	return err
}

// ResetToOriginHead resets the repo to origin's HEAD for the given branch.
func ResetToOriginHead(repoDir, branch string) error {
	_, err := run(repoDir, "reset", "--hard", "origin/"+branch)
	return err
}

// WorktreeAdd creates a new worktree at the given path with a new branch.
func WorktreeAdd(repoDir, worktreePath, branchName string) error {
	_, err := run(repoDir, "worktree", "add", "-b", branchName, worktreePath)
	return err
}

// WorktreeRemove removes a worktree.
func WorktreeRemove(repoDir, worktreePath string) error {
	_, err := run(repoDir, "worktree", "remove", "--force", worktreePath)
	return err
}

// WorktreeList lists all worktrees for a repo.
func WorktreeList(repoDir string) (string, error) {
	return run(repoDir, "worktree", "list")
}

// Status returns the short status of a repo.
func Status(repoDir string) (string, error) {
	return run(repoDir, "status", "--short")
}

// CurrentBranch returns the current branch name.
func CurrentBranch(repoDir string) (string, error) {
	return run(repoDir, "rev-parse", "--abbrev-ref", "HEAD")
}

// AheadBehind returns how many commits ahead/behind the current branch is from origin.
func AheadBehind(repoDir, branch string) (string, error) {
	return run(repoDir, "rev-list", "--left-right", "--count", "HEAD...origin/"+branch)
}

// PullFF performs a fast-forward only pull.
func PullFF(repoDir string) error {
	_, err := run(repoDir, "pull", "--ff-only")
	return err
}

// Push pushes the current branch to origin.
func Push(repoDir string) error {
	_, err := run(repoDir, "push")
	return err
}

// DeleteBranch deletes a local branch.
func DeleteBranch(repoDir, branch string) error {
	_, err := run(repoDir, "branch", "-D", branch)
	return err
}

// RunParallel runs a function for each item in parallel and collects results.
func RunParallel[T any](items []T, fn func(T) Result) []Result {
	results := make([]Result, len(items))
	var wg sync.WaitGroup
	for i, item := range items {
		wg.Add(1)
		go func(idx int, it T) {
			defer wg.Done()
			results[idx] = fn(it)
		}(i, item)
	}
	wg.Wait()
	return results
}
