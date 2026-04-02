package config

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const ConfigFile = "zproj.config.json"

type Config struct {
	Groups    map[string]Group `json:"groups"`
	Templates *Templates       `json:"templates,omitempty"`
}

type Group struct {
	Repos []Repo `json:"-"`

	RawRepos []json.RawMessage `json:"repos"`
}

type Repo struct {
	URL    string `json:"url"`
	Name   string `json:"name,omitempty"`
	Branch string `json:"branch,omitempty"`
}

type Templates struct {
	Variables map[string]string `json:"variables,omitempty"`
}

// RepoName returns the resolved name for a repo.
// If Name is set, returns it. Otherwise derives from the URL.
func (r Repo) RepoName() string {
	if r.Name != "" {
		return r.Name
	}
	return repoNameFromURL(r.URL)
}

// RepoBranch returns the resolved branch for a repo.
// Defaults to "main" if not set.
func (r Repo) RepoBranch() string {
	if r.Branch != "" {
		return r.Branch
	}
	return "main"
}

func repoNameFromURL(url string) string {
	// Handle both SSH and HTTPS URLs
	// git@github.com:org/repo.git -> repo
	// https://github.com/org/repo.git -> repo
	base := filepath.Base(url)
	return strings.TrimSuffix(base, ".git")
}

func (g *Group) UnmarshalJSON(data []byte) error {
	type rawGroup struct {
		Repos []json.RawMessage `json:"repos"`
	}
	var rg rawGroup
	if err := json.Unmarshal(data, &rg); err != nil {
		return err
	}

	for _, raw := range rg.Repos {
		var repo Repo
		// Try string first (plain URL)
		var s string
		if err := json.Unmarshal(raw, &s); err == nil {
			repo = Repo{URL: s}
		} else {
			// Try object
			if err := json.Unmarshal(raw, &repo); err != nil {
				return fmt.Errorf("invalid repo entry: %s", string(raw))
			}
		}
		if repo.URL == "" {
			return fmt.Errorf("repo entry missing url: %s", string(raw))
		}
		g.Repos = append(g.Repos, repo)
	}
	return nil
}

// Load reads and parses the config from the given path.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}
	// Strip // comments (JSONC support for the example config)
	data = stripJSONComments(data)

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// Validate checks the config for common errors and returns helpful messages.
func (c *Config) Validate() error {
	if c.Groups == nil || len(c.Groups) == 0 {
		return fmt.Errorf(`config error: no groups defined

Your %s must have at least one group with repos. Example:

  {
    "groups": {
      "default": {
        "repos": ["git@github.com:org/repo.git"]
      }
    }
  }`, ConfigFile)
	}

	for groupName, group := range c.Groups {
		if err := validateGroupName(groupName); err != nil {
			return fmt.Errorf("config error in group %q: %w", groupName, err)
		}
		if len(group.Repos) == 0 {
			return fmt.Errorf("config error: group %q has no repos\n\nAdd at least one repo URL to the group's \"repos\" array.", groupName)
		}

		seen := make(map[string]bool)
		for i, repo := range group.Repos {
			if err := validateRepo(repo, i, groupName); err != nil {
				return err
			}
			name := repo.RepoName()
			if seen[name] {
				return fmt.Errorf("config error: duplicate repo name %q in group %q\n\nUse the \"name\" field to give one of them a unique name:\n  { \"url\": \"%s\", \"name\": \"%s-2\" }", name, groupName, repo.URL, name)
			}
			seen[name] = true
		}
	}

	return nil
}

var validGroupNameRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)

func validateGroupName(name string) error {
	if !validGroupNameRe.MatchString(name) {
		return fmt.Errorf("invalid group name %q\n\nGroup names must start with a letter or number and contain only letters, numbers, hyphens, and underscores.", name)
	}
	return nil
}

var sshURLRe = regexp.MustCompile(`^[\w.-]+@[\w.-]+:[\w./-]+$`)

func validateRepo(repo Repo, index int, group string) error {
	repoURL := repo.URL

	if repoURL == "" {
		return fmt.Errorf("config error: repo #%d in group %q is missing a URL", index+1, group)
	}

	// Check for common URL issues
	if strings.Contains(repoURL, " ") {
		return fmt.Errorf("config error: repo URL contains spaces in group %q: %q\n\nRepo URLs should not contain spaces.", group, repoURL)
	}

	isSSH := sshURLRe.MatchString(repoURL)
	isHTTPS := false
	if !isSSH {
		if u, err := url.Parse(repoURL); err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != "" {
			isHTTPS = true
		}
	}

	if !isSSH && !isHTTPS {
		return fmt.Errorf(`config error: invalid repo URL in group %q: %q

Repo URLs should be either:
  SSH:   git@github.com:org/repo.git
  HTTPS: https://github.com/org/repo.git`, group, repoURL)
	}

	// Warn about missing .git suffix (non-fatal, just derive name correctly)
	name := repo.RepoName()
	if name == "" {
		return fmt.Errorf("config error: could not derive repo name from URL %q in group %q\n\nSet an explicit \"name\" field for this repo.", repoURL, group)
	}

	return nil
}

var lineCommentRe = regexp.MustCompile(`(?m)^\s*//.*$|//.*$`)

func stripJSONComments(data []byte) []byte {
	return lineCommentRe.ReplaceAll(data, nil)
}

// FindRoot walks up from startDir looking for zproj.config.json.
// Returns the directory containing it.
func FindRoot(startDir string) (string, error) {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, ConfigFile)); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("could not find %s in any parent directory", ConfigFile)
		}
		dir = parent
	}
}

// GroupDir returns the directory path for a group within the root.
// "default" group uses the root directly; named groups use [groupname]/.
func GroupDir(root, group string) string {
	if group == "default" {
		return root
	}
	return filepath.Join(root, "["+group+"]")
}

// MainDir returns the .main directory for a group.
func MainDir(root, group string) string {
	return filepath.Join(GroupDir(root, group), ".main")
}
