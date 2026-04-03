package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

const ConfigFile = "zproj.yaml"

type Config struct {
	Git       *GitConfig       `yaml:"git,omitempty"`
	Hooks     *Hooks           `yaml:"hooks,omitempty"`
	Groups    map[string]Group `yaml:"groups"`
	Templates *Templates       `yaml:"templates,omitempty"`

	// built during validation
	aliasMap     map[string]string // alias -> canonical group name
	defaultGroup string
}

type GitConfig struct {
	Provider string `yaml:"provider,omitempty"` // "github", "gitlab", "bitbucket", etc.
	Host     string `yaml:"host,omitempty"`     // e.g. "github.com", "gitlab.mycompany.com"
	Org      string `yaml:"org,omitempty"`      // default org/owner
	SSH      bool   `yaml:"ssh,omitempty"`      // true for SSH URLs, false for HTTPS
}

type Hooks struct {
	OnCreate string `yaml:"onCreate,omitempty"`
}

type Group struct {
	Repos   []Repo   `yaml:"-"`
	Default bool     `yaml:"default,omitempty"`
	Aliases []string `yaml:"aliases,omitempty"`
	Hooks   *Hooks   `yaml:"hooks,omitempty"`
}

type Repo struct {
	URL    string `yaml:"repo"`
	Name   string `yaml:"name,omitempty"`
	Branch string `yaml:"branch,omitempty"`
	Hooks  *Hooks `yaml:"hooks,omitempty"`
}

// ResolveOnCreateHook returns the most specific onCreate hook for a repo.
// Priority: repo > group > global. Returns empty string if none set.
func (c *Config) ResolveOnCreateHook(groupName string, repo Repo) string {
	if repo.Hooks != nil && repo.Hooks.OnCreate != "" {
		return repo.Hooks.OnCreate
	}
	if grp, ok := c.Groups[groupName]; ok && grp.Hooks != nil && grp.Hooks.OnCreate != "" {
		return grp.Hooks.OnCreate
	}
	if c.Hooks != nil && c.Hooks.OnCreate != "" {
		return c.Hooks.OnCreate
	}
	return ""
}

type Templates struct {
	Variables map[string]string `yaml:"variables,omitempty"`
}

// RepoName returns the resolved name for a repo.
func (r Repo) RepoName() string {
	if r.Name != "" {
		return r.Name
	}
	return repoNameFromURL(r.URL)
}

// RepoBranch returns the resolved branch for a repo.
func (r Repo) RepoBranch() string {
	if r.Branch != "" {
		return r.Branch
	}
	return "main"
}

func repoNameFromURL(u string) string {
	base := filepath.Base(u)
	return strings.TrimSuffix(base, ".git")
}

// DefaultGroup returns the name of the default group, or empty if none set.
func (c *Config) DefaultGroup() string {
	return c.defaultGroup
}

// ResolveGroup resolves a group name or alias to the canonical group name.
// Returns the canonical name and true if found, or the input and false if not.
func (c *Config) ResolveGroup(nameOrAlias string) (string, bool) {
	if _, ok := c.Groups[nameOrAlias]; ok {
		return nameOrAlias, true
	}
	if canonical, ok := c.aliasMap[nameOrAlias]; ok {
		return canonical, true
	}
	return nameOrAlias, false
}

// UnmarshalYAML supports repos as either plain strings or objects.
func (g *Group) UnmarshalYAML(value *yaml.Node) error {
	// Decode the known fields first
	type groupFields struct {
		Default bool     `yaml:"default,omitempty"`
		Aliases []string `yaml:"aliases,omitempty"`
		Repos   []yaml.Node `yaml:"repos"`
	}
	var raw groupFields
	if err := value.Decode(&raw); err != nil {
		return err
	}

	g.Default = raw.Default
	g.Aliases = raw.Aliases

	for _, node := range raw.Repos {
		var repo Repo
		switch node.Kind {
		case yaml.ScalarNode:
			repo = Repo{URL: node.Value}
		case yaml.MappingNode:
			if err := node.Decode(&repo); err != nil {
				return fmt.Errorf("invalid repo entry at line %d: %w", node.Line, err)
			}
		default:
			return fmt.Errorf("invalid repo entry at line %d: expected string or mapping", node.Line)
		}
		if repo.URL == "" {
			return fmt.Errorf("repo entry missing 'repo' field at line %d", node.Line)
		}
		g.Repos = append(g.Repos, repo)
	}
	return nil
}

// ExpandRepoURL expands a short repo name (e.g. "my-repo") to a full URL
// using the git config. If it's already a full URL, returns it unchanged.
func (c *Config) ExpandRepoURL(repoURL string) string {
	// Already a full URL (SSH or HTTPS)
	if strings.Contains(repoURL, ":") || strings.Contains(repoURL, "//") {
		return repoURL
	}

	if c.Git == nil || c.Git.Org == "" {
		return repoURL
	}

	host := c.Git.Host
	if host == "" {
		switch strings.ToLower(c.Git.Provider) {
		case "gitlab":
			host = "gitlab.com"
		case "bitbucket":
			host = "bitbucket.org"
		default:
			host = "github.com"
		}
	}

	org := c.Git.Org
	name := repoURL

	if c.Git.SSH {
		return fmt.Sprintf("git@%s:%s/%s.git", host, org, name)
	}
	return fmt.Sprintf("https://%s/%s/%s.git", host, org, name)
}

// Load reads and parses the config from the given path.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", filepath.Base(path), err)
	}
	// Expand short repo names to full URLs
	cfg.expandRepoURLs()

	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *Config) expandRepoURLs() {
	for name, group := range c.Groups {
		for i, repo := range group.Repos {
			group.Repos[i].URL = c.ExpandRepoURL(repo.URL)
		}
		c.Groups[name] = group
	}
}

// Validate checks the config for common errors and returns helpful messages.
func (c *Config) Validate() error {
	if c.Groups == nil || len(c.Groups) == 0 {
		return fmt.Errorf(`config error: no groups defined

Your %s must have at least one group with repos. Example:

  groups:
    mygroup:
      default: true
      repos:
        - git@github.com:org/repo.git`, ConfigFile)
	}

	c.aliasMap = make(map[string]string)
	defaultCount := 0

	for groupName, group := range c.Groups {
		if err := validateGroupName(groupName); err != nil {
			return fmt.Errorf("config error in group %q: %w", groupName, err)
		}
		if len(group.Repos) == 0 {
			return fmt.Errorf("config error: group %q has no repos\n\nAdd at least one repo URL to the group's repos list.", groupName)
		}

		if group.Default {
			defaultCount++
			c.defaultGroup = groupName
			if defaultCount > 1 {
				return fmt.Errorf("config error: multiple groups marked as default\n\nOnly one group can have 'default: true'.")
			}
		}

		for _, alias := range group.Aliases {
			if err := validateGroupName(alias); err != nil {
				return fmt.Errorf("config error: invalid alias %q in group %q: %w", alias, groupName, err)
			}
			if _, exists := c.Groups[alias]; exists {
				return fmt.Errorf("config error: alias %q in group %q conflicts with an existing group name", alias, groupName)
			}
			if existing, exists := c.aliasMap[alias]; exists {
				return fmt.Errorf("config error: alias %q is used by both group %q and %q", alias, existing, groupName)
			}
			c.aliasMap[alias] = groupName
		}

		seen := make(map[string]bool)
		for i, repo := range group.Repos {
			if err := validateRepo(repo, i, groupName); err != nil {
				return err
			}
			name := repo.RepoName()
			if seen[name] {
				return fmt.Errorf("config error: duplicate repo name %q in group %q\n\nUse the \"name\" field to give one a unique name:\n  - repo: %s\n    name: %s-2", name, groupName, repo.URL, name)
			}
			seen[name] = true
		}
	}

	// If only one group, make it the default automatically
	if defaultCount == 0 && len(c.Groups) == 1 {
		for name := range c.Groups {
			c.defaultGroup = name
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

	name := repo.RepoName()
	if name == "" {
		return fmt.Errorf("config error: could not derive repo name from URL %q in group %q\n\nSet an explicit \"name\" field for this repo.", repoURL, group)
	}

	return nil
}

// FindConfigFile returns the config file path within a directory.
func FindConfigFile(dir string) (string, bool) {
	path := filepath.Join(dir, ConfigFile)
	if _, err := os.Stat(path); err == nil {
		return path, true
	}
	return "", false
}

// FindRoot walks up from startDir looking for zproj.yaml.
func FindRoot(startDir string) (string, error) {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return "", err
	}
	for {
		if _, found := FindConfigFile(dir); found {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("could not find %s in any parent directory", ConfigFile)
		}
		dir = parent
	}
}

// MainDir returns the .zproj/{group} directory where bare repos live.
func MainDir(root, group string) string {
	return filepath.Join(root, ".zproj", group)
}

// ProjectDir returns the directory for a project.
// Projects live at root/{projectName}.
func ProjectDir(root, projectName string) string {
	return filepath.Join(root, projectName)
}

// ProjectName builds a project directory name from group and name.
func ProjectName(group, name string) string {
	return group + "-" + name
}
