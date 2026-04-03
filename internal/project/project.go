package project

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	"github.com/ntotten/zproj/internal/config"
	"github.com/ntotten/zproj/internal/git"
)

// Metadata stored in each project directory as .zproj-project.json.
type Metadata struct {
	Group string `json:"group"`
}

const metadataFile = ".zproj-project.json"

// Create creates a new project with worktrees for all repos in the group.
func Create(root string, cfg *config.Config, projectName, group, color string) error {
	grp, ok := cfg.Groups[group]
	if !ok {
		return fmt.Errorf("group %q not found in config", group)
	}

	projectDir := config.ProjectDir(root, projectName)
	if _, err := os.Stat(projectDir); err == nil {
		return fmt.Errorf("project %q already exists at %s", projectName, projectDir)
	}

	if err := os.MkdirAll(projectDir, 0755); err != nil {
		return fmt.Errorf("creating project directory: %w", err)
	}

	// Write project metadata
	if err := writeMetadata(projectDir, Metadata{Group: group}); err != nil {
		return fmt.Errorf("writing metadata: %w", err)
	}

	mainDir := config.MainDir(root, group)
	results := git.RunParallel(grp.Repos, func(repo config.Repo) git.Result {
		repoName := repo.RepoName()
		repoMainDir := filepath.Join(mainDir, repoName)
		worktreePath := filepath.Join(projectDir, repoName)
		branchName := projectName

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

	if err := generateWorkspace(projectDir, projectName, grp.Repos, color); err != nil {
		return fmt.Errorf("generating workspace: %w", err)
	}

	if err := processTemplates(root, group, projectDir, projectName, cfg); err != nil {
		return fmt.Errorf("processing templates: %w", err)
	}

	if err := runOnCreateHooks(cfg, group, grp, projectDir); err != nil {
		return fmt.Errorf("running hooks: %w", err)
	}

	return nil
}

func writeMetadata(projectDir string, meta Metadata) error {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(projectDir, metadataFile), append(data, '\n'), 0644)
}

// ReadMetadata reads the project metadata from a project directory.
func ReadMetadata(projectDir string) (*Metadata, error) {
	data, err := os.ReadFile(filepath.Join(projectDir, metadataFile))
	if err != nil {
		return nil, err
	}
	var meta Metadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}
	return &meta, nil
}

// DetectProject checks if the given directory is inside a project.
// Returns the project directory and name, or error if not in a project.
func DetectProject(dir, root string) (projectDir, projectName string, err error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", "", err
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", "", err
	}

	// Walk up from dir, but stop at root
	d := abs
	for {
		// Check if this dir has project metadata
		if _, err := os.Stat(filepath.Join(d, metadataFile)); err == nil {
			name := filepath.Base(d)
			return d, name, nil
		}
		parent := filepath.Dir(d)
		if parent == d || parent == absRoot || d == absRoot {
			break
		}
		d = parent
	}

	return "", "", fmt.Errorf("not inside a project directory")
}

func runOnCreateHooks(cfg *config.Config, groupName string, grp config.Group, projectDir string) error {
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
func Delete(root string, cfg *config.Config, projectName string) error {
	projectDir := config.ProjectDir(root, projectName)
	if _, err := os.Stat(projectDir); os.IsNotExist(err) {
		return fmt.Errorf("project %q does not exist", projectName)
	}

	meta, err := ReadMetadata(projectDir)
	if err != nil {
		return fmt.Errorf("reading project metadata: %w\n\nIs %q a zproj project?", err, projectName)
	}

	grp, ok := cfg.Groups[meta.Group]
	if !ok {
		return fmt.Errorf("group %q (from project metadata) not found in config", meta.Group)
	}

	mainDir := config.MainDir(root, meta.Group)
	results := git.RunParallel(grp.Repos, func(repo config.Repo) git.Result {
		repoName := repo.RepoName()
		repoMainDir := filepath.Join(mainDir, repoName)
		worktreePath := filepath.Join(projectDir, repoName)

		if err := git.WorktreeRemove(repoMainDir, worktreePath); err != nil {
			return git.Result{Repo: repoName, Err: fmt.Errorf("removing worktree: %w", err)}
		}
		if err := git.DeleteBranch(repoMainDir, projectName); err != nil {
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

	if err := os.RemoveAll(projectDir); err != nil {
		return fmt.Errorf("removing project directory: %w", err)
	}

	return nil
}

// List returns all project names in the root.
func List(root string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}

	var projects []string
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		// Verify it's a project by checking for metadata
		if _, err := os.Stat(filepath.Join(root, e.Name(), metadataFile)); err == nil {
			projects = append(projects, e.Name())
		}
	}
	return projects, nil
}

// Pull runs git pull on all repos in a project.
func Pull(root string, cfg *config.Config, projectName string) ([]git.Result, error) {
	projectDir := config.ProjectDir(root, projectName)
	meta, err := ReadMetadata(projectDir)
	if err != nil {
		return nil, fmt.Errorf("reading project metadata: %w", err)
	}

	grp, ok := cfg.Groups[meta.Group]
	if !ok {
		return nil, fmt.Errorf("group %q not found in config", meta.Group)
	}

	return git.RunParallel(grp.Repos, func(repo config.Repo) git.Result {
		repoDir := filepath.Join(projectDir, repo.RepoName())
		if err := git.PullFF(repoDir); err != nil {
			return git.Result{Repo: repo.RepoName(), Err: err}
		}
		return git.Result{Repo: repo.RepoName(), Output: "pulled"}
	}), nil
}

// Push runs git push on all repos in a project.
func Push(root string, cfg *config.Config, projectName string) ([]git.Result, error) {
	projectDir := config.ProjectDir(root, projectName)
	meta, err := ReadMetadata(projectDir)
	if err != nil {
		return nil, fmt.Errorf("reading project metadata: %w", err)
	}

	grp, ok := cfg.Groups[meta.Group]
	if !ok {
		return nil, fmt.Errorf("group %q not found in config", meta.Group)
	}

	return git.RunParallel(grp.Repos, func(repo config.Repo) git.Result {
		repoDir := filepath.Join(projectDir, repo.RepoName())
		if err := git.Push(repoDir); err != nil {
			return git.Result{Repo: repo.RepoName(), Err: err}
		}
		return git.Result{Repo: repo.RepoName(), Output: "pushed"}
	}), nil
}

// GetStatus returns the status of all repos in a project.
func GetStatus(root string, cfg *config.Config, projectName string) ([]ProjectStatus, error) {
	projectDir := config.ProjectDir(root, projectName)
	meta, err := ReadMetadata(projectDir)
	if err != nil {
		return nil, fmt.Errorf("reading project metadata: %w", err)
	}

	grp, ok := cfg.Groups[meta.Group]
	if !ok {
		return nil, fmt.Errorf("group %q not found in config", meta.Group)
	}

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

// ProjectStatus holds status info for a single repo in a project.
type ProjectStatus struct {
	Repo        string
	Branch      string
	Dirty       bool
	AheadBehind string
}

// Color helpers

var ColorMap = map[string]string{
	"red":    "#b91c1c",
	"orange": "#c2410c",
	"yellow": "#a16207",
	"green":  "#15803d",
	"teal":   "#0f766e",
	"blue":   "#1d4ed8",
	"indigo": "#4338ca",
	"purple": "#7e22ce",
	"pink":   "#be185d",
	"rose":   "#e11d48",
	"sky":    "#0369a1",
	"lime":   "#4d7c0f",
	"cyan":   "#0e7490",
	"slate":  "#475569",
}

func ResolveColor(name string) (string, bool) {
	hex, ok := ColorMap[name]
	return hex, ok
}

func RandomColor() string {
	names := ColorNames()
	return names[rand.Intn(len(names))]
}

func ColorNames() []string {
	names := make([]string, 0, len(ColorMap))
	for k := range ColorMap {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

type workspaceFile struct {
	Folders  []workspaceFolder `json:"folders"`
	Settings map[string]any    `json:"settings,omitempty"`
}

type workspaceFolder struct {
	Path string `json:"path"`
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

	templateDirs := []string{
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
