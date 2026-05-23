package cmd

import (
	"github.com/spf13/cobra"
)

// fleetCmd is the parent of all `hike fleet ...` subcommands. It has no
// runtime behavior of its own — each subcommand registers itself in its own
// init().
var fleetCmd = &cobra.Command{
	Use:   "fleet",
	Short: "Run a manager Claude that supervises background agent teams",
	Long: `Run a long-lived "fleet" of autonomous Claude Code agents.

A manager Claude in your terminal coordinates one or more background workers,
each running in its own hike project (worktrees + plan files). The manager
and workers communicate via a daemon process — workers keep running even
when you close the manager or the terminal.

Common flow:
  hike fleet start       # spawn daemon + manager Claude
  hike fleet status      # see daemon + workers from another terminal
  hike fleet logs <project> -f   # tail a worker's event stream
  hike fleet stop        # graceful drain + daemon shutdown
`,
}

func init() {
	rootCmd.AddCommand(fleetCmd)
}
