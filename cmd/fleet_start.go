package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/zuplo/hike/internal/fleet"
)

var (
	fleetStartNoManager bool
	fleetStartTimeout   time.Duration
)

var fleetStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the fleet daemon and launch the manager Claude",
	Long: `Spawns the hike-fleetd daemon if it isn't running for this root,
generates a manager MCP config pointing at the daemon, and execs an
interactive Claude Code session in your terminal as the fleet manager.

The daemon runs detached and survives manager / terminal exit. Use
'hike fleet status' to see it, 'hike fleet stop' to shut it down.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireConfig(); err != nil {
			return err
		}
		p := fleet.New(rootDir)

		// Pre-flight: hike-fleetd binary present? (Don't check claude unless
		// we're going to launch the manager.)
		if _, err := fleet.LocateFleetd(); err != nil {
			return fmt.Errorf("%w\n\n%s", err, fleet.InstallHint())
		}

		// Pre-flight: claude binary if we're going to launch the manager.
		if !fleetStartNoManager {
			if _, err := fleet.AttemptCheckClaudeBinary(); err != nil {
				return fmt.Errorf("'claude' binary not found on PATH. Install Claude Code first: https://docs.claude.com/")
			}
		}

		// Start (or join) the daemon.
		ctx, cancel := context.WithTimeout(cmd.Context(), fleetStartTimeout)
		defer cancel()

		fmt.Fprintf(os.Stderr, "Starting fleet daemon for %s...\n", rootDir)
		pid, port, err := fleet.Start(ctx, fleet.StartOptions{
			Root:         rootDir,
			StartTimeout: fleetStartTimeout,
		})
		if err != nil {
			return fmt.Errorf("starting daemon: %w", err)
		}
		fmt.Fprintf(os.Stderr, "Fleet daemon running (pid=%d, mcp=http://127.0.0.1:%d)\n", pid, port)

		// Ensure the canonical PROJECTS.md exists at the root so the user
		// has something to edit immediately.
		if err := ensureProjectsMD(p.ProjectsMD()); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not create PROJECTS.md: %v\n", err)
		}

		if fleetStartNoManager {
			fmt.Fprintf(os.Stderr, "\nDaemon started in background. Launch the manager later with:\n  hike fleet attach\n")
			return nil
		}

		// Write the manager MCP config.
		mcpCfgPath := p.ManagerMCPConfig()
		if err := writeManagerMCPConfig(mcpCfgPath, port); err != nil {
			return fmt.Errorf("writing manager MCP config: %w", err)
		}

		// Exec claude in the foreground. We use exec.Command because
		// syscall.Exec would replace this Go process and we want
		// PersistentPostRun hooks (update check) to fire after claude exits.
		appendPrompt := buildManagerAppendPrompt(rootDir, port)
		claudeArgs := []string{
			"--mcp-config", mcpCfgPath,
			"--append-system-prompt", appendPrompt,
		}

		claudePath, err := fleet.AttemptCheckClaudeBinary()
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "Launching manager claude (Ctrl-D to exit; daemon keeps running)...\n\n")

		claudeCmd := exec.Command(claudePath, claudeArgs...)
		claudeCmd.Stdin = os.Stdin
		claudeCmd.Stdout = os.Stdout
		claudeCmd.Stderr = os.Stderr
		claudeCmd.Dir = rootDir

		if err := claudeCmd.Run(); err != nil {
			// Propagate the claude exit code if available; otherwise generic err.
			if ee, ok := err.(*exec.ExitError); ok {
				if ws, ok := ee.Sys().(syscall.WaitStatus); ok && ws.ExitStatus() != 0 {
					os.Exit(ws.ExitStatus())
				}
			}
			return fmt.Errorf("running manager claude: %w", err)
		}
		return nil
	},
}

func init() {
	fleetStartCmd.Flags().BoolVar(&fleetStartNoManager, "no-manager", false,
		"only start the daemon, don't launch the manager Claude")
	fleetStartCmd.Flags().DurationVar(&fleetStartTimeout, "timeout", 30*time.Second,
		"how long to wait for the daemon to become ready")
	fleetCmd.AddCommand(fleetStartCmd)
}

// ensureProjectsMD writes a starter PROJECTS.md if one doesn't exist.
func ensureProjectsMD(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.WriteFile(path, []byte(starterProjectsMD), 0o644)
}

const starterProjectsMD = `# Fleet roster

Each line under a section is one project. Statuses:

- ` + "`[ ]`" + ` idle  · ` + "`[~]`" + ` running  · ` + "`[!]`" + ` blocked  · ` + "`[x]`" + ` archived

Edit this file (or ask the manager to). The daemon watches it.

## Active

<!-- Add projects like:
- [ ] **platform-billing-api** (group: platform) — Add usage-based billing
-->

## Archived

`

// writeManagerMCPConfig writes the JSON file passed via --mcp-config to claude.
func writeManagerMCPConfig(path string, port int) error {
	cfg := managerMCPConfig{
		MCPServers: map[string]mcpServerEntry{
			"hike-fleet": {
				Type: "http",
				URL:  fmt.Sprintf("http://127.0.0.1:%d/manager/mcp", port),
			},
		},
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

type managerMCPConfig struct {
	MCPServers map[string]mcpServerEntry `json:"mcpServers"`
}

type mcpServerEntry struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

// buildManagerAppendPrompt produces the snippet appended to the manager
// claude's system prompt so it understands its role.
func buildManagerAppendPrompt(root string, port int) string {
	return fmt.Sprintf(`You are the manager of a hike fleet — a set of background Claude Code agents working on isolated projects in this workspace.

Workspace root: %s
Fleet MCP: http://127.0.0.1:%d/manager/mcp (already wired into your tools)

Your job is to be the single thread of interaction for the user. You can:
- Read PROJECTS.md at the workspace root to see the project roster
- Call fleet_list_projects, fleet_project_info, fleet_list_workers, fleet_worker_status, fleet_worker_logs, fleet_read_status to inspect state
- Call fleet_add_project to create new fleet projects (calls into hike's worktree provisioning)
- Write PLAN.md files into project directories ($root/{group}-{name}/PLAN.md) describing what a worker should do
- Call fleet_dispatch_worker to spawn a background worker on a project (reads PLAN.md by default)
- Drain fleet_pending_signals periodically to see urgent messages from workers

Background workers will use their own filtered MCP surface (fleet_post_status, fleet_post_finding) to report progress back. You do NOT edit code in repos directly — you dispatch workers and review their work via STATUS.md / JOURNAL.md per-project.`, root, port)
}
