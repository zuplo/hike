package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
	"github.com/zuplo/hike/internal/fleet"
)

var fleetDoctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Diagnose fleet prerequisites: claude binary, hike-fleetd, ports, disk",
	RunE: func(cmd *cobra.Command, args []string) error {
		ok := true

		// hike root resolved?
		if rootDir == "" {
			fmt.Println("  ✗ hike root: not found (no hike.yaml in this dir or any parent)")
			ok = false
		} else {
			fmt.Printf("  ✓ hike root: %s\n", rootDir)
		}

		// claude binary on PATH?
		if path, err := exec.LookPath("claude"); err == nil {
			ver := tryRun(path, "--version")
			if ver == "" {
				ver = "version unknown"
			}
			fmt.Printf("  ✓ claude binary: %s (%s)\n", path, ver)
		} else {
			fmt.Println("  ✗ claude binary: not found on PATH (install Claude Code)")
			ok = false
		}

		// hike-fleetd binary?
		if path, err := fleet.LocateFleetd(); err == nil {
			fmt.Printf("  ✓ hike-fleetd:   %s\n", path)
		} else {
			fmt.Printf("  ✗ hike-fleetd:   %v\n", err)
			fmt.Println()
			fmt.Println(fleet.InstallHint())
			ok = false
		}

		// fleet dir writable?
		if rootDir != "" {
			p := fleet.New(rootDir)
			if err := fleet.EnsureFleetDir(p); err != nil {
				fmt.Printf("  ✗ fleet dir:     %v\n", err)
				ok = false
			} else {
				fmt.Printf("  ✓ fleet dir:     %s\n", p.FleetDir())
			}
		}

		// Daemon already up?
		if rootDir != "" {
			p := fleet.New(rootDir)
			if running, pid := fleet.IsRunning(p); running {
				port, _ := fleet.ReadPort(p.PortFile())
				fmt.Printf("  i daemon:        running (pid=%d, mcp port=%d)\n", pid, port)
			} else {
				fmt.Println("  i daemon:        not running (start with 'hike fleet start')")
			}
		}

		if !ok {
			fmt.Println()
			os.Exit(1)
		}
		return nil
	},
}

func init() {
	fleetCmd.AddCommand(fleetDoctorCmd)
}

func tryRun(name string, args ...string) string {
	out, err := exec.Command(name, args...).Output()
	if err != nil {
		return ""
	}
	return string(trimRightBytes(out, "\n\r\t "))
}

func trimRightBytes(b []byte, cutset string) []byte {
	for len(b) > 0 {
		last := b[len(b)-1]
		found := false
		for i := 0; i < len(cutset); i++ {
			if last == cutset[i] {
				found = true
				break
			}
		}
		if !found {
			break
		}
		b = b[:len(b)-1]
	}
	return b
}
