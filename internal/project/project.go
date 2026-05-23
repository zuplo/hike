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
	"time"

	"github.com/zuplo/hike/internal/config"
	"github.com/zuplo/hike/internal/git"
)

// Metadata stored in each project directory as .hike-project.json.
type Metadata struct {
	Group string `json:"group"`
}

const metadataFile = ".hike-project.json"

// FleetMarker stored in each project directory as .hike-fleet.json when the
// project is under fleet management. Presence alone signals managed=true;
// the struct holds future-proofing fields.
type FleetMarker struct {
	Managed   bool   `json:"managed"`
	CreatedAt string `json:"createdAt,omitempty"` // RFC3339
}

const fleetMarkerFile = ".hike-fleet.json"

// TemplateData is the data passed to all templates during project creation.
type TemplateData struct {
	ProjectName string
	Group       string
	Color       string // hex color value, empty if not set
	ColorName   string // color name (e.g. "purple"), empty if not set
	Repos       []RepoInfo
	Variables   map[string]string
}

// RepoInfo provides repo details to templates.
type RepoInfo struct {
	Name   string
	Branch string
	URL    string
}

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

	if err := processTemplates(root, group, projectDir, projectName, color, grp.Repos, cfg); err != nil {
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

// MarkFleetManaged writes .hike-fleet.json into projectDir, claiming the
// project for fleet supervision. Idempotent: rewrites if already present.
func MarkFleetManaged(projectDir string) error {
	marker := FleetMarker{
		Managed:   true,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	data, err := json.MarshalIndent(marker, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(projectDir, fleetMarkerFile), append(data, '\n'), 0644)
}

// IsFleetManaged reports whether the project directory has a .hike-fleet.json
// marker. Returns (false, nil) when the file does not exist; (false, err) on
// any other error.
func IsFleetManaged(projectDir string) (bool, error) {
	_, err := os.Stat(filepath.Join(projectDir, fleetMarkerFile))
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// UnmarkFleetManaged removes the .hike-fleet.json marker. Idempotent: returns
// nil if the file is already absent.
func UnmarkFleetManaged(projectDir string) error {
	err := os.Remove(filepath.Join(projectDir, fleetMarkerFile))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
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
		return fmt.Errorf("reading project metadata: %w\n\nIs %q a hike project?", err, projectName)
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

func processTemplates(root, group, projectDir, name, color string, repos []config.Repo, cfg *config.Config) error {
	// Build template data
	data := TemplateData{
		ProjectName: name,
		Group:       group,
		Variables:   make(map[string]string),
	}

	// Resolve color
	if color != "" {
		hex, ok := ResolveColor(color)
		if !ok {
			return fmt.Errorf("unknown color %q, valid colors: %s", color, strings.Join(ColorNames(), ", "))
		}
		data.Color = hex
		data.ColorName = color
	}

	// Build repo list
	for _, repo := range repos {
		data.Repos = append(data.Repos, RepoInfo{
			Name:   repo.RepoName(),
			Branch: repo.RepoBranch(),
			URL:    repo.URL,
		})
	}

	// Custom variables
	if cfg.Templates != nil {
		for k, v := range cfg.Templates.Variables {
			data.Variables[k] = v
		}
	}

	tmplDir := filepath.Join(root, ".template")
	if _, err := os.Stat(tmplDir); os.IsNotExist(err) {
		return nil
	}

	funcMap := template.FuncMap{
		"json": func(v any) string {
			b, _ := json.MarshalIndent(v, "", "  ")
			return string(b)
		},
	}

	return filepath.Walk(tmplDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}

		relPath, _ := filepath.Rel(tmplDir, path)

		// Template the filename itself (e.g. "{{.ProjectName}}.code-workspace")
		nameTmpl, err := template.New("filename").Parse(relPath)
		if err != nil {
			return fmt.Errorf("parsing filename template %s: %w", relPath, err)
		}
		var nameBuf strings.Builder
		if err := nameTmpl.Execute(&nameBuf, data); err != nil {
			return fmt.Errorf("executing filename template %s: %w", relPath, err)
		}
		destPath := filepath.Join(projectDir, nameBuf.String())

		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		tmpl, err := template.New(filepath.Base(path)).Funcs(funcMap).Parse(string(content))
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

		return tmpl.Execute(f, data)
	})
}
