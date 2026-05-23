// Package fleet provides the Go-side glue for the `hike fleet` command:
// spawning the TS-on-Bun daemon (hike-fleetd), connecting to its control
// socket, and shared types for the JSON-RPC protocol between them.
package fleet

import "path/filepath"

// Paths derives every fleet-related path from a hike root directory.
// One Paths value per root; cheap to construct.
type Paths struct {
	Root string
}

// New returns a Paths rooted at the given hike root (the dir containing
// hike.yaml). Use this everywhere instead of hand-building paths.
func New(root string) Paths {
	return Paths{Root: root}
}

// FleetDir returns ${root}/.hike/fleet — the parent of all daemon state.
func (p Paths) FleetDir() string {
	return filepath.Join(p.Root, ".hike", "fleet")
}

// PidFile returns ${root}/.hike/fleet/daemon.pid.
func (p Paths) PidFile() string {
	return filepath.Join(p.FleetDir(), "daemon.pid")
}

// PortFile returns ${root}/.hike/fleet/daemon.port (single-line decimal).
func (p Paths) PortFile() string {
	return filepath.Join(p.FleetDir(), "daemon.port")
}

// CtlSock returns ${root}/.hike/fleet/ctl.sock — the JSON-RPC unix socket
// between the Go CLI and the TS daemon.
func (p Paths) CtlSock() string {
	return filepath.Join(p.FleetDir(), "ctl.sock")
}

// DaemonLog returns ${root}/.hike/fleet/logs/daemon.log.
func (p Paths) DaemonLog() string {
	return filepath.Join(p.FleetDir(), "logs", "daemon.log")
}

// WorkerLogDir returns ${root}/.hike/fleet/logs/workers — one .jsonl per worker.
func (p Paths) WorkerLogDir() string {
	return filepath.Join(p.FleetDir(), "logs", "workers")
}

// WorkerLog returns ${root}/.hike/fleet/logs/workers/{workerID}.jsonl.
func (p Paths) WorkerLog(workerID string) string {
	return filepath.Join(p.WorkerLogDir(), workerID+".jsonl")
}

// StateDB returns ${root}/.hike/fleet/state.db (SQLite).
func (p Paths) StateDB() string {
	return filepath.Join(p.FleetDir(), "state.db")
}

// ManagerMCPConfig returns the generated --mcp-config file path passed to
// the manager `claude` invocation.
func (p Paths) ManagerMCPConfig() string {
	return filepath.Join(p.FleetDir(), "manager-mcp.json")
}

// ManagerPromptFile returns the editable manager system prompt path.
func (p Paths) ManagerPromptFile() string {
	return filepath.Join(p.FleetDir(), "MANAGER_PROMPT.md")
}

// WorkerPromptFile returns the editable worker system prompt path.
func (p Paths) WorkerPromptFile() string {
	return filepath.Join(p.FleetDir(), "WORKER_PROMPT.md")
}

// ProjectsMD returns ${root}/PROJECTS.md (or the configured override).
// Defaults are baked into config.ResolvedFleet; callers passing a custom
// filename should join it against root themselves.
func (p Paths) ProjectsMD() string {
	return filepath.Join(p.Root, "PROJECTS.md")
}
