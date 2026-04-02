# zproj

A fast CLI tool for managing multi-repo development workspaces using git worktrees.

Create isolated workspaces per feature or task, with all your repos available in each workspace. Each workspace gets its own VS Code `.code-workspace` file and git branch across all repos.

## Install

Requires the [GitHub CLI](https://cli.github.com/) (`gh`) to be installed and authenticated.

```sh
# Download and install the latest release
sh -c "$(curl -fsSL https://raw.githubusercontent.com/zuplo/zproj/main/install.sh)"

# Or with gh directly
gh release download --repo zuplo/zproj --pattern '*darwin_arm64*' --dir /tmp
tar -xzf /tmp/zproj_*_darwin_arm64.tar.gz -C /usr/local/bin
```

## Quick Start

```sh
# Create a new project directory and initialize
mkdir my-projects && cd my-projects
zproj init

# Edit zproj.config.json to add your repos, then run init again
zproj init

# Create a workspace (this is the default command)
zproj my-feature

# Open in VS Code
code my-feature/my-feature.code-workspace
```

## Configuration

`zproj.config.json` defines your repos and groups:

```jsonc
{
  "groups": {
    "default": {
      "repos": [
        // Just a URL — name and branch are inferred
        "git@github.com:your-org/my-app.git",
        "git@github.com:your-org/shared-lib.git",

        // Or use an object to override name/branch
        { "url": "git@github.com:your-org/api.git", "branch": "develop" }
      ]
    },
    "backend": {
      "repos": [
        "git@github.com:your-org/api-service.git",
        "git@github.com:your-org/worker.git"
      ]
    }
  },
  "templates": {
    "variables": {
      "ORG": "your-org"
    }
  }
}
```

- **Repo URL string**: name is derived from the URL (`your-org/my-app.git` -> `my-app`), branch defaults to `main`
- **Repo object**: `url` (required), `name` (optional), `branch` (optional, defaults to `main`)
- **Groups**: `default` group lives at the root. Named groups get a `[group-name]/` directory.

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

## Directory Structure

```
my-projects/
├── zproj.config.json
├── .main/                     # Main repos (always on default branch)
│   ├── my-app/
│   └── shared-lib/
├── my-feature/                # A project
│   ├── my-feature.code-workspace
│   ├── my-app/                # git worktree on branch "my-feature"
│   └── shared-lib/            # git worktree on branch "my-feature"
├── [backend]/                 # A group
│   ├── .main/
│   │   ├── api-service/
│   │   └── worker/
│   └── fix-auth/
│       ├── fix-auth.code-workspace
│       ├── api-service/
│       └── worker/
└── .template/                 # Optional: template files
```

## Templates

Place files in `.template/` (root level) or `[group]/.template/` (group level). They are processed with Go's `text/template` and copied into each new project.

Available variables:
- `{{.ProjectName}}` — the project name
- `{{.Group}}` — the group name
- Any custom variables from `templates.variables` in the config

## Updating

The CLI checks for updates once per day and will notify you if a newer version is available. Run `zproj update` to upgrade.
