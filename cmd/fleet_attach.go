package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/zuplo/hike/internal/fleet"
)

var fleetAttachCmd = &cobra.Command{
	Use:   "attach",
	Short: "Launch a new manager Claude attached to an already-running daemon",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireConfig(); err != nil {
			return err
		}
		p := fleet.New(rootDir)

		running, _ := fleet.IsRunning(p)
		if !running {
			return fmt.Errorf("fleet daemon is not running. Start it with: hike fleet start")
		}
		port, err := fleet.ReadPort(p.PortFile())
		if err != nil {
			return fmt.Errorf("reading daemon port: %w", err)
		}

		mcpCfgPath := p.ManagerMCPConfig()
		if err := writeManagerMCPConfig(mcpCfgPath, port); err != nil {
			return fmt.Errorf("writing manager MCP config: %w", err)
		}

		claudePath, err := fleet.AttemptCheckClaudeBinary()
		if err != nil {
			return fmt.Errorf("'claude' binary not found on PATH")
		}

		fmt.Fprintf(os.Stderr, "Launching manager claude (pid file: %s, mcp port: %d)...\n\n", p.PidFile(), port)
		claudeCmd := exec.Command(claudePath,
			"--mcp-config", mcpCfgPath,
			"--append-system-prompt", buildManagerAppendPrompt(rootDir, port),
		)
		claudeCmd.Stdin = os.Stdin
		claudeCmd.Stdout = os.Stdout
		claudeCmd.Stderr = os.Stderr
		claudeCmd.Dir = rootDir

		if err := claudeCmd.Run(); err != nil {
			if ee, ok := err.(*exec.ExitError); ok {
				if ws, ok := ee.Sys().(syscall.WaitStatus); ok && ws.ExitStatus() != 0 {
					os.Exit(ws.ExitStatus())
				}
			}
			return err
		}
		return nil
	},
}

func init() {
	fleetCmd.AddCommand(fleetAttachCmd)
}
