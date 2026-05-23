package fleet

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ErrFleetdNotFound is returned when no hike-fleetd binary can be located.
var ErrFleetdNotFound = errors.New("hike-fleetd binary not found")

// FleetdBinaryName is the executable name we look for.
const FleetdBinaryName = "hike-fleetd"

// LocateFleetd returns the absolute path to the hike-fleetd binary by
// checking, in order:
//  1. The HIKE_FLEETD_BIN environment variable. May be a single path OR a
//     whitespace-separated command (first token is the binary, rest are
//     prefix args) — useful for local dev: `bun run /path/to/main.ts`.
//  2. The user's PATH.
//  3. ~/.hike/bin/hike-fleetd (the install location used by install.sh).
//
// Returns ErrFleetdNotFound if all three fail.
func LocateFleetd() (string, error) {
	if env := strings.TrimSpace(os.Getenv("HIKE_FLEETD_BIN")); env != "" {
		// Split on whitespace; first token is the binary.
		fields := strings.Fields(env)
		first := fields[0]
		if filepath.IsAbs(first) {
			if _, err := os.Stat(first); err == nil {
				return first, nil
			}
		}
		if found, err := exec.LookPath(first); err == nil {
			return found, nil
		}
		return "", fmt.Errorf("HIKE_FLEETD_BIN points at %q which is not found on PATH or filesystem", first)
	}
	if found, err := exec.LookPath(FleetdBinaryName); err == nil {
		return found, nil
	}
	home, err := os.UserHomeDir()
	if err == nil {
		candidate := filepath.Join(home, ".hike", "bin", FleetdBinaryName)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", ErrFleetdNotFound
}

// InstallHint returns a human-readable explanation of how to install hike-fleetd.
func InstallHint() string {
	return fmt.Sprintf(`%s not found.

To install:
  1. Build from source: cd daemon && bun install && bun run scripts/build.ts
     then copy dist/hike-fleetd-<target> to ~/.hike/bin/hike-fleetd
  2. Or set HIKE_FLEETD_BIN=/path/to/hike-fleetd
  3. Or for local dev: HIKE_FLEETD_BIN="bun run /path/to/daemon/src/main.ts"`,
		FleetdBinaryName)
}
