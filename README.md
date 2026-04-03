# hike

A fast CLI tool for managing multi-repo development workspaces using git worktrees.

Create isolated workspaces per feature or task, with all your repos available in each workspace. Each workspace gets its own VS Code `.code-workspace` file and git branch across all repos.

## Install

```sh
curl -fsSL https://zuplo.github.io/hike/install.sh | sh
```

Or as a single command:

```sh
d=$(mktemp -d) && curl -fsSL "https://github.com/zuplo/hike/releases/latest/download/hike_$(curl -sL https://api.github.com/repos/zuplo/hike/releases/latest | grep tag_name | sed -E 's/.*"v([^"]+)".*/\1/')_$(uname -s | tr A-Z a-z)_$(uname -m).tar.gz" | tar -xz -C "$d" && mkdir -p ~/.hike/bin && mv "$d/hike" ~/.hike/bin/ && sudo ln -sf ~/.hike/bin/hike /usr/local/bin/hike && rm -rf "$d" && echo "hike installed ✓"
```

After the initial install, update with `hk update` (no sudo needed). Both `hike` and `hk` are installed automatically.

## Quick Start

```sh
# Create a new project directory and generate a config file
mkdir my-projects && cd my-projects
hk init

# Edit hike.yaml to add your repos, then sync to clone them
hk sync

# Create a workspace
hk platform                 # -> platform-bold-cedar/
hk platform my-feature      # -> platform-my-feature/

# Open in VS Code
code platform-my-feature/platform-my-feature.code-workspace

# Run commands from inside a project (auto-detects project)
cd platform-my-feature
hk pull
hk push
hk status
hk delete
```

## Configuration

`hike.yaml` defines your repos and groups:

```yaml
# Git provider defaults — repos can use just the name instead of full URLs
git:
  org: your-org
  # provider: github    # github (default), gitlab, bitbucket
  # host: github.com    # auto-detected from provider, set for self-hosted
  # ssh: true           # use SSH URLs (default: false, uses HTTPS)

groups:
  platform:
    default: true       # Used when group is not specified
    repos:
      - my-app          # Expands to https://github.com/your-org/my-app.git
      - shared-lib

      # Or use an object to override name/branch
      - repo: api
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
- **Repo object**: `repo` (required), `name` (optional), `branch` (optional, defaults to `main`)
- **Groups**: repos are organized into groups. Set `default: true` on one group to use it when no group is specified. If only one group exists, it's the default automatically.
- **Aliases**: set `aliases: [short]` on a group to use either name in commands (e.g. `hk mktg`)

### Hooks

Lifecycle hooks run after creating a project. The most specific hook wins: **repo overrides group, group overrides global**.

```yaml
# Global default — runs for every repo unless overridden
hooks:
  onCreate: npm install

groups:
  platform:
    # Group-level override — applies to all repos in this group
    hooks:
      onCreate: pnpm install
    repos:
      - my-app
      - repo: legacy-service
        # Repo-level override — only this repo uses yarn
        hooks:
          onCreate: yarn install
```

Hooks run in parallel across repos for speed.

## Commands

### `hk [group] [name] [-c color]`

Create a new project. This is the default command. The project directory is named `{group}-{name}`.

```sh
hk platform my-feature    # Creates platform-my-feature/
hk platform               # Generates random name: platform-bold-cedar/
hk my-feature             # Uses default group: platform-my-feature/
hk platform -c purple     # With a color
hk platform my-feature -c # Random color
```

The first argument is matched against known groups — if it matches, it's treated as the group. Otherwise it's the project name (using the default group).

Available colors for `-c`: `blue`, `cyan`, `green`, `indigo`, `lime`, `orange`, `pink`, `purple`, `red`, `rose`, `sky`, `slate`, `teal`, `yellow`.

### `hk init`

Create a new `hike.yaml` configuration file in the current directory.

```sh
hk init
```

### `hk sync [-g group]`

Clone any missing repos and sync all `.hike/` repos to the latest `origin/HEAD`. This is the command to run after editing your config to add new repos.

> [!WARNING]
> Sync performs a hard reset (`git reset --hard`) on `.hike/` repos to match the remote. Any uncommitted or unpushed changes in `.hike/` directories **will be lost**. This is by design — these repos are meant to be clean mirrors of the remote. Always do your work in project worktrees, never directly in `.hike/`.

```sh
hk sync
hk sync -g backend
```

### `hk pull [project-name]`

Pull latest changes (fast-forward only) in all repos of a project. Auto-detects the project if run from inside one.

```sh
hk pull                   # From inside a project
hk pull platform-my-feat  # By name
```

### `hk push [project-name]`

Push all repos in a project. Auto-detects the project if run from inside one.

```sh
hk push                   # From inside a project
hk push platform-my-feat  # By name
```

### `hk status [project-name]`

Show the status of each repo in a project (branch, dirty state, ahead/behind). Auto-detects the project if run from inside one.

```sh
hk status
```

### `hk delete [project-name]`

Remove a project and its worktrees. Auto-detects the project if run from inside one.

```sh
hk delete                     # From inside a project
hk delete platform-my-feat    # By name
```

### `hk list`

List all projects.

```sh
hk list
```

### `hk update`

Self-update to the latest release.

```sh
hk update
```

## Directory Structure

```
my-projects/
├── hike.yaml
├── .hike/                        # Hidden — main repo clones
│   ├── platform/
│   │   ├── my-app/
│   │   └── shared-lib/
│   └── marketing/
│       ├── website/
│       └── cms/
├── platform-my-feature/           # A project
│   ├── .hike-project.json        # Metadata (group info)
│   ├── platform-my-feature.code-workspace
│   ├── my-app/                    # git worktree
│   └── shared-lib/
├── platform-bold-cedar/           # Another project (random name)
│   └── ...
├── marketing-redesign/
│   ├── website/
│   └── cms/
└── .template/                     # Optional: template files
```

## Templates

Place files in `.template/` at the root level. They are processed with Go's `text/template` and copied into each new project.

Available variables:
- `{{.ProjectName}}` — the project name
- `{{.Group}}` — the group name
- Any custom variables from `templates.variables` in the config

## MCP Server

hike includes a built-in MCP (Model Context Protocol) server so you can manage projects from Claude or other AI assistants.

### Claude Code

Add to your Claude Code MCP settings (`~/.claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "hike": {
      "command": "hike",
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
- `pull_project` — Pull latest in all repos
- `push_project` — Push all repos
- `project_status` — Show git status of repos in a project
- `sync_repos` — Sync .hike repos to latest

## Updating

The CLI checks for updates once per day and will notify you if a newer version is available. Run `hk update` to upgrade.

## Disclaimer

This is not an official [Zuplo](https://zuplo.com) product. It is a free, open-source tool provided as-is under the [MIT License](LICENSE), with no warranty or support guarantees. Use at your own risk.
