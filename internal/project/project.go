package project

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	"github.com/ntotten/zproj/internal/config"
	"github.com/ntotten/zproj/internal/git"
)

// Create creates a new project with worktrees for all repos in the group.
func Create(root string, cfg *config.Config, name, group, color string) error {
	grp, ok := cfg.Groups[group]
	if !ok {
		return fmt.Errorf("group %q not found in config", group)
	}

	projectDir := filepath.Join(config.GroupDir(root, group), name)
	if _, err := os.Stat(projectDir); err == nil {
		return fmt.Errorf("project %q already exists at %s", name, projectDir)
	}

	if err := os.MkdirAll(projectDir, 0755); err != nil {
		return fmt.Errorf("creating project directory: %w", err)
	}

	mainDir := config.MainDir(root, group)
	results := git.RunParallel(grp.Repos, func(repo config.Repo) git.Result {
		repoName := repo.RepoName()
		repoMainDir := filepath.Join(mainDir, repoName)
		worktreePath := filepath.Join(projectDir, repoName)
		branchName := name

		if err := git.WorktreeAdd(repoMainDir, worktreePath, branchName); err != nil {
			return git.Result{Repo: repoName, Err: fmt.Errorf("creating worktree: %w", err)}
		}
		return git.Result{Repo: repoName, Output: "created"}
	})

	var errs []string
	for _, r := range results {
		if r.Err != nil {
			errs = append(errs, fmt.Sprintf("  %s: %v", r.Repo, r.Err))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("errors creating worktrees:\n%s", strings.Join(errs, "\n"))
	}

	if err := generateWorkspace(projectDir, name, grp.Repos, color); err != nil {
		return fmt.Errorf("generating workspace: %w", err)
	}

	if err := processTemplates(root, group, projectDir, name, cfg); err != nil {
		return fmt.Errorf("processing templates: %w", err)
	}

	// Run onCreate hooks
	if err := runOnCreateHooks(cfg, group, grp, projectDir); err != nil {
		return fmt.Errorf("running hooks: %w", err)
	}

	return nil
}

func runOnCreateHooks(cfg *config.Config, groupName string, grp config.Group, projectDir string) error {
	// Filter repos that have hooks
	type repoHook struct {
		repo config.Repo
		hook string
	}
	var hooks []repoHook
	for _, repo := range grp.Repos {
		if hook := cfg.ResolveOnCreateHook(groupName, repo); hook != "" {
			hooks = append(hooks, repoHook{repo: repo, hook: hook})
		}
	}
	if len(hooks) == 0 {
		return nil
	}

	fmt.Printf("Running onCreate hooks...\n")
	results := git.RunParallel(hooks, func(rh repoHook) git.Result {
		repoName := rh.repo.RepoName()
		repoDir := filepath.Join(projectDir, repoName)

		cmd := exec.Command("sh", "-c", rh.hook)
		cmd.Dir = repoDir
		out, err := cmd.CombinedOutput()
		if err != nil {
			return git.Result{Repo: repoName, Err: fmt.Errorf("%s: %w\n%s", rh.hook, err, string(out))}
		}
		return git.Result{Repo: repoName, Output: "done"}
	})

	var errs []string
	for _, r := range results {
		if r.Err != nil {
			errs = append(errs, fmt.Sprintf("  %s: %v", r.Repo, r.Err))
		} else {
			fmt.Printf("  %s: hook %s\n", r.Repo, r.Output)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("hook errors:\n%s", strings.Join(errs, "\n"))
	}
	return nil
}

// Delete removes a project and its worktrees.
func Delete(root string, cfg *config.Config, name, group string) error {
	grp, ok := cfg.Groups[group]
	if !ok {
		return fmt.Errorf("group %q not found in config", group)
	}

	projectDir := filepath.Join(config.GroupDir(root, group), name)
	if _, err := os.Stat(projectDir); os.IsNotExist(err) {
		return fmt.Errorf("project %q does not exist", name)
	}

	mainDir := config.MainDir(root, group)
	results := git.RunParallel(grp.Repos, func(repo config.Repo) git.Result {
		repoName := repo.RepoName()
		repoMainDir := filepath.Join(mainDir, repoName)
		worktreePath := filepath.Join(projectDir, repoName)

		if err := git.WorktreeRemove(repoMainDir, worktreePath); err != nil {
			return git.Result{Repo: repoName, Err: fmt.Errorf("removing worktree: %w", err)}
		}
		// Clean up the branch
		if err := git.DeleteBranch(repoMainDir, name); err != nil {
			// Non-fatal: branch might not exist or might be on a remote
			return git.Result{Repo: repoName, Output: "worktree removed (branch cleanup skipped)"}
		}
		return git.Result{Repo: repoName, Output: "removed"}
	})

	var errs []string
	for _, r := range results {
		if r.Err != nil {
			errs = append(errs, fmt.Sprintf("  %s: %v", r.Repo, r.Err))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("errors removing worktrees:\n%s", strings.Join(errs, "\n"))
	}

	// Remove project directory
	if err := os.RemoveAll(projectDir); err != nil {
		return fmt.Errorf("removing project directory: %w", err)
	}

	return nil
}

type workspaceFile struct {
	Folders  []workspaceFolder  `json:"folders"`
	Settings map[string]any     `json:"settings,omitempty"`
}

type workspaceFolder struct {
	Path string `json:"path"`
}

// ColorMap maps color names to hex values for VS Code title bar.
var ColorMap = map[string]string{
	"red":     "#b91c1c",
	"orange":  "#c2410c",
	"yellow":  "#a16207",
	"green":   "#15803d",
	"teal":    "#0f766e",
	"blue":    "#1d4ed8",
	"indigo":  "#4338ca",
	"purple":  "#7e22ce",
	"pink":    "#be185d",
	"rose":    "#e11d48",
	"sky":     "#0369a1",
	"lime":    "#4d7c0f",
	"cyan":    "#0e7490",
	"slate":   "#475569",
}

// ResolveColor maps a color name to its hex value.
// Returns empty string if not found.
func ResolveColor(name string) (string, bool) {
	hex, ok := ColorMap[name]
	return hex, ok
}

// ColorNames returns all valid color names sorted.
func ColorNames() []string {
	names := make([]string, 0, len(ColorMap))
	for k := range ColorMap {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

func generateWorkspace(projectDir, name string, repos []config.Repo, color string) error {
	ws := workspaceFile{
		Folders: make([]workspaceFolder, len(repos)),
	}
	for i, repo := range repos {
		ws.Folders[i] = workspaceFolder{Path: repo.RepoName()}
	}
	if color != "" {
		hex, ok := ResolveColor(color)
		if !ok {
			return fmt.Errorf("unknown color %q, valid colors: %s", color, strings.Join(ColorNames(), ", "))
		}
		ws.Settings = map[string]any{
			"workbench.colorCustomizations": map[string]string{
				"titleBar.activeBackground":   hex,
				"titleBar.activeForeground":   "#ffffff",
				"titleBar.inactiveBackground": hex,
				"titleBar.inactiveForeground": "#cccccc",
			},
		}
	}

	data, err := json.MarshalIndent(ws, "", "  ")
	if err != nil {
		return err
	}

	wsPath := filepath.Join(projectDir, name+".code-workspace")
	return os.WriteFile(wsPath, append(data, '\n'), 0644)
}

func processTemplates(root, group, projectDir, name string, cfg *config.Config) error {
	vars := map[string]string{
		"ProjectName": name,
		"Group":       group,
	}
	if cfg.Templates != nil {
		for k, v := range cfg.Templates.Variables {
			vars[k] = v
		}
	}

	// Check group-level templates first, then global
	templateDirs := []string{
		filepath.Join(config.GroupDir(root, group), ".template"),
		filepath.Join(root, ".template"),
	}

	for _, tmplDir := range templateDirs {
		if _, err := os.Stat(tmplDir); os.IsNotExist(err) {
			continue
		}

		err := filepath.Walk(tmplDir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return err
			}

			relPath, _ := filepath.Rel(tmplDir, path)
			destPath := filepath.Join(projectDir, relPath)

			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}

			tmpl, err := template.New(filepath.Base(path)).Parse(string(content))
			if err != nil {
				return fmt.Errorf("parsing template %s: %w", relPath, err)
			}

			if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
				return err
			}

			f, err := os.Create(destPath)
			if err != nil {
				return err
			}
			defer f.Close()

			return tmpl.Execute(f, vars)
		})
		if err != nil {
			return err
		}
	}

	return nil
}

// List returns all project names for a group.
func List(root, group string) ([]string, error) {
	groupDir := config.GroupDir(root, group)
	entries, err := os.ReadDir(groupDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var projects []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		// Skip .main, .template, and [group] dirs
		if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "[") {
			continue
		}
		// Verify it's a project by checking for workspace file
		wsPath := filepath.Join(groupDir, name, name+".code-workspace")
		if _, err := os.Stat(wsPath); err == nil {
			projects = append(projects, name)
		}
	}
	return projects, nil
}

// ProjectStatus holds status info for a single repo in a project.
type ProjectStatus struct {
	Repo         string
	Branch       string
	Dirty        bool
	AheadBehind  string
}

// GetStatus returns the status of all repos in a project.
func GetStatus(root string, cfg *config.Config, name, group string) ([]ProjectStatus, error) {
	grp, ok := cfg.Groups[group]
	if !ok {
		return nil, fmt.Errorf("group %q not found in config", group)
	}

	projectDir := filepath.Join(config.GroupDir(root, group), name)
	var statuses []ProjectStatus

	for _, repo := range grp.Repos {
		repoName := repo.RepoName()
		repoDir := filepath.Join(projectDir, repoName)

		branch, _ := git.CurrentBranch(repoDir)
		statusOut, _ := git.Status(repoDir)
		ab, _ := git.AheadBehind(repoDir, repo.RepoBranch())

		statuses = append(statuses, ProjectStatus{
			Repo:        repoName,
			Branch:      branch,
			Dirty:       statusOut != "",
			AheadBehind: ab,
		})
	}
	return statuses, nil
}
