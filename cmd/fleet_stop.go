package cmd

import (
	"context"
	"fmt"
	"os"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/zuplo/hike/internal/fleet"
)

var (
	fleetStopGrace time.Duration
	fleetStopKill  bool
)

var fleetStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Gracefully stop the fleet daemon and its workers",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireConfig(); err != nil {
			return err
		}
		p := fleet.New(rootDir)

		running, pid := fleet.IsRunning(p)
		if !running {
			fmt.Fprintln(os.Stderr, "Fleet daemon is not running.")
			return nil
		}

		// Ask daemon to shut down gracefully.
		client, err := fleet.DialDaemon(p)
		if err == nil {
			ctx, cancel := context.WithTimeout(cmd.Context(), fleetStopGrace+5*time.Second)
			defer cancel()
			rpcErr := client.Call(ctx, fleet.MethodDaemonStop, fleet.DaemonStopParams{
				GraceSeconds: int(fleetStopGrace.Seconds()),
				Kill:         fleetStopKill,
			}, nil)
			_ = client.Close()
			if rpcErr != nil {
				fmt.Fprintf(os.Stderr, "daemon.stop RPC failed: %v\n", rpcErr)
			}
		} else {
			fmt.Fprintf(os.Stderr, "Could not reach daemon socket; falling back to signal: %v\n", err)
		}

		// Wait for the process to exit. If grace elapses, SIGKILL.
		deadline := time.Now().Add(fleetStopGrace + 2*time.Second)
		for time.Now().Before(deadline) {
			alive, _ := fleet.IsRunning(p)
			if !alive {
				fmt.Fprintln(os.Stderr, "Fleet daemon stopped.")
				return nil
			}
			time.Sleep(200 * time.Millisecond)
		}

		// Still alive — escalate.
		fmt.Fprintf(os.Stderr, "Daemon (pid=%d) still running after %s; sending SIGKILL.\n", pid, fleetStopGrace)
		proc, err := os.FindProcess(pid)
		if err == nil {
			_ = proc.Signal(syscall.SIGKILL)
		}
		// Best-effort cleanup.
		_ = os.Remove(p.PidFile())
		_ = os.Remove(p.PortFile())
		_ = os.Remove(p.CtlSock())
		return nil
	},
}

func init() {
	fleetStopCmd.Flags().DurationVar(&fleetStopGrace, "grace", 30*time.Second, "how long to wait for workers to drain")
	fleetStopCmd.Flags().BoolVar(&fleetStopKill, "kill", false, "skip graceful drain; SIGKILL workers immediately")
	fleetCmd.AddCommand(fleetStopCmd)
}
