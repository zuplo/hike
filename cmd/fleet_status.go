package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/zuplo/hike/internal/fleet"
)

var (
	fleetStatusJSON  bool
	fleetStatusWatch bool
)

var fleetStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show the fleet daemon and worker state",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireConfig(); err != nil {
			return err
		}
		p := fleet.New(rootDir)

		// One-shot or watch loop.
		for {
			if err := printStatus(p); err != nil {
				if !fleetStatusWatch {
					return err
				}
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			}
			if !fleetStatusWatch {
				return nil
			}
			time.Sleep(2 * time.Second)
		}
	},
}

func init() {
	fleetStatusCmd.Flags().BoolVar(&fleetStatusJSON, "json", false, "emit JSON instead of a table")
	fleetStatusCmd.Flags().BoolVar(&fleetStatusWatch, "watch", false, "refresh every 2 seconds")
	fleetCmd.AddCommand(fleetStatusCmd)
}

func printStatus(p fleet.Paths) error {
	running, pid := fleet.IsRunning(p)
	if !running {
		if fleetStatusJSON {
			data, _ := json.MarshalIndent(map[string]any{"running": false}, "", "  ")
			fmt.Println(string(data))
			return nil
		}
		fmt.Println("Fleet daemon: not running")
		fmt.Println("Start with: hike fleet start")
		return nil
	}

	client, err := fleet.DialDaemon(p)
	if err != nil {
		// Daemon pid exists but socket dial failed — odd state.
		fmt.Fprintf(os.Stderr, "Daemon pid=%d but ctl socket unreachable: %v\n", pid, err)
		return err
	}
	defer client.Close()

	var snap fleet.SnapshotResult
	if err := client.Call(nil, fleet.MethodStatusSnapshot, nil, &snap); err != nil {
		return fmt.Errorf("status.snapshot: %w", err)
	}

	if fleetStatusJSON {
		data, _ := json.MarshalIndent(snap, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	fmt.Printf("Fleet daemon: running (pid=%d, uptime=%ds)\n", snap.DaemonPID, snap.UptimeSec)
	fmt.Printf("MCP URL:      %s\n", snap.MCPURL)
	fmt.Printf("Workers:      %d active\n", snap.NumWorkers)

	if len(snap.Projects) > 0 {
		fmt.Println("\nProjects:")
		for _, pr := range snap.Projects {
			run := ""
			if pr.CurrentRun != "" {
				run = fmt.Sprintf(" run=%s", pr.CurrentRun)
			}
			fmt.Printf("  %-32s %s%s\n", pr.Name, pr.State, run)
		}
	}
	if len(snap.Workers) > 0 {
		fmt.Println("\nWorkers:")
		for _, w := range snap.Workers {
			fmt.Printf("  %-20s %-32s %s\n", w.ID, w.ProjectName, w.State)
		}
	}
	return nil
}
