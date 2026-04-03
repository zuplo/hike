# zproj

A fast CLI tool for managing multi-repo development workspaces using git worktrees.

Create isolated workspaces per feature or task, with all your repos available in each workspace. Each workspace gets its own VS Code `.code-workspace` file and git branch across all repos.

## Install

Requires the [GitHub CLI](https://cli.github.com/) (`gh`) to be installed and authenticated (`gh auth login`).

```sh
d=$(mktemp -d) && gh release download --repo zuplo/zproj --pattern "*$(uname -s | tr A-Z a-z)_$(uname -m)*.tar.gz" --dir "$d" && tar -xzf "$d"/*.tar.gz -C "$d" && mkdir -p ~/.zproj/bin && mv "$d/zproj" ~/.zproj/bin/ && sudo ln -sf ~/.zproj/bin/zproj /usr/local/bin/zproj && rm -rf "$d" && echo "zproj installed ✓"
```

After the initial install, update with `zproj update` (no sudo needed).

## Quick Start

```sh
# Create a new project directory and initialize
mkdir my-projects && cd my-projects
zproj init

# Edit zproj.yaml to add your repos, then run init again
zproj init

# Create a workspace (this is the default command)
zproj my-feature

# Open in VS Code
code my-feature/my-feature.code-workspace
```

## Configuration

`zproj.yaml` defines your repos and groups:

```yaml
# Git provider defaults — repos can use just the name instead of full URLs
git:
  org: your-org
  # provider: github    # github (default), gitlab, bitbucket
  # host: github.com    # auto-detected from provider, set for self-hosted
  # ssh: true           # use SSH URLs (default: false, uses HTTPS)

groups:
  platform:
    default: true       # Used when --group is not specified
    repos:
      - my-app          # Expands to https://github.com/your-org/my-app.git
      - shared-lib

      # Or use an object to override name/branch
      - url: api
        branch: develop

      # Full URLs still work
      - git@github.com:other-org/special-repo.git

  marketing:
    aliases: [mktg]     # Use 'mktg' as shorthand
    repos:
      - website
      - cms

# Optional: variables available in .template/ files
templates:
  variables:
    ORG: your-org
```

- **`git` config**: set `org` to use short repo names. Supports GitHub, GitLab, Bitbucket, or any self-hosted provider via `host`.
- **Repo short name**: when `git.org` is set, just use `my-repo` instead of the full URL
- **Repo full URL**: SSH (`git@github.com:org/repo.git`) or HTTPS (`https://...`) still works
- **Repo object**: `url` (required), `name` (optional), `branch` (optional, defaults to `main`)
- **Groups**: every group gets a `[group-name]/` directory. Set `default: true` on one group to use it when `--group` is omitted. If only one group exists, it's the default automatically.
- **Aliases**: set `aliases: [short]` on a group to use either name in commands (e.g. `--group mktg`)

## Commands

### `zproj <name> [--group <g>] [--color <color>]`

Create a new project. This is the default command.

```sh
zproj my-feature
zproj my-feature --group backend
zproj my-feature -c purple
```

Creates a directory with git worktrees for each repo and a VS Code workspace file.

Available colors for `--color`: `blue`, `cyan`, `green`, `indigo`, `lime`, `orange`, `pink`, `purple`, `red`, `rose`, `sky`, `slate`, `teal`, `yellow`.

### `zproj init`

Initialize the project. If no config file exists, interactively creates one. If a config exists, clones all repos into `.main/` directories.

```sh
zproj init
```

### `zproj sync [--group <g>]`

Fetch and reset all `.main` repos to the latest `origin/HEAD`.

```sh
zproj sync
zproj sync --group backend
```

### `zproj delete <name> [--group <g>]`

Remove a project and its worktrees.

```sh
zproj delete my-feature
```

### `zproj list`

List all projects.

```sh
zproj list
zproj list --group backend
```

### `zproj status <name> [--group <g>]`

Show the status of each repo in a project (branch, dirty state, ahead/behind).

```sh
zproj status my-feature
```

### `zproj update`

Self-update to the latest release.

```sh
zproj update
```

### `zproj alias [name]`

Create a shorter alias for the `zproj` command. Prompts for a name if not provided.

```sh
zproj alias z
# Now you can use 'z' instead of 'zproj'
z my-feature
z sync
```

## Directory Structure

```
my-projects/
├── zproj.yaml
├── [platform]/                # A group (the default)
│   ├── .main/                 # Main repos (always on default branch)
│   │   ├── my-app/
│   │   └── shared-lib/
│   └── my-feature/            # A project
│       ├── my-feature.code-workspace
│       ├── my-app/            # git worktree on branch "my-feature"
│       └── shared-lib/
├── [marketing]/               # Another group (alias: mktg)
│   ├── .main/
│   │   ├── website/
│   │   └── cms/
│   └── redesign/
│       ├── redesign.code-workspace
│       ├── website/
│       └── cms/
└── .template/                 # Optional: template files
```

## Templates

Place files in `.template/` (root level) or `[group]/.template/` (group level). They are processed with Go's `text/template` and copied into each new project.

Available variables:
- `{{.ProjectName}}` — the project name
- `{{.Group}}` — the group name
- Any custom variables from `templates.variables` in the config

## MCP Server

zproj includes a built-in MCP (Model Context Protocol) server so you can manage projects from Claude or other AI assistants.

### Claude Code

Add to your Claude Code MCP settings (`~/.claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "zproj": {
      "command": "zproj",
      "args": ["mcp"],
      "cwd": "/path/to/your/projects"
    }
  }
}
```

### Available MCP tools

- `create_project` — Create a new project with worktrees
- `delete_project` — Delete a project and its worktrees
- `list_projects` — List all projects
- `sync_repos` — Sync .main repos to latest
- `project_status` — Show git status of repos in a project

## Updating

The CLI checks for updates once per day and will notify you if a newer version is available. Run `zproj update` to upgrade.
