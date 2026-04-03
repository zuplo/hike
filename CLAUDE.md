# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

**hike** (`hk`) is a Go CLI tool for managing multi-repo development workspaces using git worktrees. It lets you create isolated project directories where each repo gets a worktree with its own branch, VS Code workspace file, and lifecycle hooks.

## Build & Run

```sh
go build -o hike .                    # Build binary
go build -ldflags "-X github.com/zuplo/hike/cmd.version=dev" -o hike .  # With version
```

No tests exist yet. No Makefile — just `go build`.

## Release

Tag-based via GoReleaser + GitHub Actions. To release:
```sh
git tag v0.X.0 && git push origin v0.X.0
```

Version is injected via ldflags: `-X github.com/zuplo/hike/cmd.version={{.Version}}`.

## Architecture

### Shared state in `cmd/root.go`

All commands share package-level vars set during `cobra.OnInitialize`:
- `rootDir` — path to the directory containing `hike.yaml`
- `cfg` — parsed config (nil if not found)
- `cfgLoadErr` — deferred config error (so commands like `update` work without config)

Commands that need config call `requireConfig()` which surfaces `cfgLoadErr` or missing-config errors. Commands like `update`, `init`, `mcp` work without a valid config.

### Config resolution (`internal/config`)

- Walks up from cwd to find `hike.yaml`
- YAML supports repos as plain strings (`my-app`) or objects (`repo: my-app, branch: develop`)
- Short repo names expanded to full URLs using `git.org`/`git.provider`/`git.host` config
- Groups have `default: true` flag and `aliases` for shorthand
- `ResolveGroup()` checks canonical names then aliases
- `ResolveOnCreateHook()` implements hook precedence: repo > group > global

### Project detection (`internal/project`)

- Each project stores `.hike-project.json` with `{"group": "..."}` metadata
- `DetectProject()` walks up from cwd to find this file — used by `pull`, `push`, `status`, `delete` for auto-detection
- `List()` scans root dir for directories containing `.hike-project.json`

### Directory layout

```
root/
├── hike.yaml
├── .hike/{group}/{repo}/          # Main repo clones (hidden)
├── {group}-{name}/                # Project directories (flat)
│   ├── .hike-project.json
│   ├── {name}.code-workspace
│   └── {repo}/                    # Git worktrees
```

### Default command resolution (`resolveCreateArgs` in `root.go`)

`hike <arg1> [arg2]` — first arg is checked against known groups/aliases. If it matches a group, it's the group and second arg (or random name) is the project name. If it doesn't match a group, it's treated as the project name with the default group.

### Parallel operations (`internal/git`)

`git.RunParallel[T]()` is a generic concurrent runner used for clones, syncs, worktree creation, hooks, pulls, and pushes. All git operations shell out to `git` (not go-git).

### Self-update (`internal/update`)

- `LatestVersion()` hits GitHub API directly via `net/http` (no `gh` CLI needed)
- `SelfUpdate()` downloads release tarball and replaces `~/.hike/bin/hike`
- `CheckOutdated()` runs in background with 2s timeout, throttled to once/24h via `~/.hike/update-check.json`

### MCP server (`cmd/mcp.go`)

`hike mcp` starts a stdio MCP server using `mark3labs/mcp-go`. Exposes tools for create, delete, list, pull, push, status, sync.
