package fleet

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// IsRunning returns true if the daemon for this root is running. Checks both
// that daemon.pid exists and that the PID is alive (signal 0 probe).
func IsRunning(p Paths) (bool, int) {
	pid, err := ReadPID(p.PidFile())
	if err != nil {
		return false, 0
	}
	if pid <= 0 {
		return false, 0
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false, 0
	}
	// On Unix, FindProcess always succeeds. Signal 0 is the standard liveness
	// probe — returns nil if the process exists and we can signal it.
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		if errors.Is(err, os.ErrProcessDone) {
			return false, 0
		}
		// EPERM means the process exists but we don't own it. Treat as alive
		// — we shouldn't blindly start a second daemon in that case.
		if errors.Is(err, syscall.EPERM) {
			return true, pid
		}
		return false, 0
	}
	return true, pid
}

// StartOptions controls how StartDaemon spawns hike-fleetd.
type StartOptions struct {
	Root          string        // hike root dir (the one containing hike.yaml)
	ExtraArgs     []string      // extra args appended after the standard ones
	StartTimeout  time.Duration // how long to wait for daemon.pid + healthz
	ManagerPort   int           // 0 = let daemon pick a free port
	StdoutLogPath string        // empty defaults to ${root}/.hike/fleet/logs/daemon.log
}

// Start launches the hike-fleetd daemon for the given root if it's not
// already running. Returns the PID and the MCP HTTP port. Idempotent: if a
// daemon is already running, returns its PID and port without spawning a
// new one.
func Start(ctx context.Context, opts StartOptions) (pid int, port int, err error) {
	if opts.StartTimeout == 0 {
		opts.StartTimeout = 30 * time.Second
	}
	p := New(opts.Root)

	// Already running? Read its port and return.
	if running, existing := IsRunning(p); running {
		port, perr := ReadPort(p.PortFile())
		if perr == nil {
			return existing, port, nil
		}
		// Stale port file. Continue trying to start; if it succeeds, the new
		// daemon will write a fresh port.
	}

	// Ensure parent directories exist.
	if err := os.MkdirAll(p.WorkerLogDir(), 0o755); err != nil {
		return 0, 0, fmt.Errorf("creating fleet dirs: %w", err)
	}

	// Remove stale socket/port/pid files from a crashed previous run.
	for _, f := range []string{p.PidFile(), p.PortFile(), p.CtlSock()} {
		_ = os.Remove(f)
	}

	// Open the daemon log for append.
	logPath := opts.StdoutLogPath
	if logPath == "" {
		logPath = p.DaemonLog()
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return 0, 0, fmt.Errorf("opening daemon log %s: %w", logPath, err)
	}
	defer logFile.Close()

	// Resolve the fleetd binary or runner command. HIKE_FLEETD_BIN is split
	// on whitespace so "bun run /path/to/main.ts" works for local dev.
	argv, err := resolveFleetdArgv()
	if err != nil {
		return 0, 0, err
	}

	// Build daemon args: argv prefix + standard args + caller extras.
	args := append([]string{}, argv[1:]...)
	args = append(args,
		"--root", opts.Root,
		"--pid-file", p.PidFile(),
		"--port-file", p.PortFile(),
		"--ctl-sock", p.CtlSock(),
		"--state-db", p.StateDB(),
		"--worker-log-dir", p.WorkerLogDir(),
		"--manager-prompt", p.ManagerPromptFile(),
		"--worker-prompt", p.WorkerPromptFile(),
		"--projects-md", p.ProjectsMD(),
	)
	if opts.ManagerPort > 0 {
		args = append(args, "--port", fmt.Sprintf("%d", opts.ManagerPort))
	}
	args = append(args, opts.ExtraArgs...)

	cmd := exec.Command(argv[0], args...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Stdin = nil
	// Detach: new session, new process group. Survives the Go parent exiting.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	// Set cwd to the hike root so any relative paths the daemon emits make sense.
	cmd.Dir = opts.Root

	if err := cmd.Start(); err != nil {
		return 0, 0, fmt.Errorf("spawning %s: %w", argv[0], err)
	}

	// Release the child so the Go process can exit cleanly without leaving
	// a zombie. We don't need the exit status — daemon is now its own thing.
	if err := cmd.Process.Release(); err != nil {
		return 0, 0, fmt.Errorf("releasing daemon process: %w", err)
	}

	// Wait for the daemon to write its pidfile + portfile and become healthy.
	deadline := time.Now().Add(opts.StartTimeout)
	for {
		if time.Now().After(deadline) {
			return 0, 0, fmt.Errorf("daemon did not become ready within %s (see %s for diagnostics)",
				opts.StartTimeout, logPath)
		}
		// Did the daemon write pid + port?
		running, existing := IsRunning(p)
		if running {
			port, perr := ReadPort(p.PortFile())
			if perr == nil && port > 0 {
				// One last gate: healthz reachable.
				if pingHealth(ctx, port, 1*time.Second) == nil {
					return existing, port, nil
				}
			}
		}
		select {
		case <-ctx.Done():
			return 0, 0, ctx.Err()
		case <-time.After(150 * time.Millisecond):
		}
	}
}

// pingHealth GETs http://127.0.0.1:port/healthz and returns nil on 200.
func pingHealth(ctx context.Context, port int, timeout time.Duration) error {
	url := fmt.Sprintf("http://127.0.0.1:%d/healthz", port)
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(cctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	client := http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("healthz returned %d", resp.StatusCode)
	}
	return nil
}

// resolveFleetdArgv finds the daemon binary (or runner) and returns the
// argv0 + any prefix args from HIKE_FLEETD_BIN.
func resolveFleetdArgv() ([]string, error) {
	if env := strings.TrimSpace(os.Getenv("HIKE_FLEETD_BIN")); env != "" {
		fields := strings.Fields(env)
		if len(fields) == 0 {
			return nil, fmt.Errorf("HIKE_FLEETD_BIN is set but empty after whitespace trim")
		}
		// Resolve the first token through PATH if it isn't an abs path.
		first := fields[0]
		if !filepath.IsAbs(first) {
			resolved, err := exec.LookPath(first)
			if err != nil {
				return nil, fmt.Errorf("HIKE_FLEETD_BIN: looking up %q: %w", first, err)
			}
			fields[0] = resolved
		}
		return fields, nil
	}
	bin, err := LocateFleetd()
	if err != nil {
		return nil, err
	}
	return []string{bin}, nil
}

// EnsureFleetDir creates ${root}/.hike/fleet and its subdirs. Idempotent.
func EnsureFleetDir(p Paths) error {
	return os.MkdirAll(p.WorkerLogDir(), 0o755)
}

// DialDaemon is a convenience: connect to the ctl.sock for this root.
// Returns ErrDaemonNotRunning if the daemon isn't up.
func DialDaemon(p Paths) (*Client, error) {
	if running, _ := IsRunning(p); !running {
		return nil, ErrDaemonNotRunning
	}
	return DialWithTimeout(p.CtlSock(), 2*time.Second)
}

// ErrDaemonNotRunning is returned by DialDaemon when no daemon is up.
var ErrDaemonNotRunning = errors.New("hike fleet daemon is not running for this root (run 'hike fleet start')")

// CopyDaemonLog appends recent daemon log content to w. Best-effort; returns
// nil on missing file. Useful for `hike fleet doctor`.
func CopyDaemonLog(p Paths, w io.Writer, tailBytes int64) error {
	f, err := os.Open(p.DaemonLog())
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()
	if tailBytes > 0 {
		fi, err := f.Stat()
		if err == nil && fi.Size() > tailBytes {
			if _, err := f.Seek(fi.Size()-tailBytes, io.SeekStart); err != nil {
				return err
			}
		}
	}
	_, err = io.Copy(w, f)
	return err
}

// AttemptCheckClaudeBinary returns the path to the `claude` binary, or an
// error if it's not on PATH. Used by `hike fleet doctor` and `start`.
func AttemptCheckClaudeBinary() (string, error) {
	return exec.LookPath("claude")
}
