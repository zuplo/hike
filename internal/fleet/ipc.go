package fleet

import "encoding/json"

// JSON-RPC 2.0 over newline-delimited frames on the ctl.sock unix socket.
// The Go CLI is always the client; the TS daemon is always the server.
// One connection per CLI invocation; multiple sequential calls allowed.

// Request is a JSON-RPC 2.0 request frame.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  any             `json:"params,omitempty"`
}

// Response is a JSON-RPC 2.0 response frame.
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

// RPCError is a JSON-RPC 2.0 error object.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func (e *RPCError) Error() string { return e.Message }

// Method names. Centralized so both sides reference the same strings.
const (
	MethodPing           = "ping"
	MethodStatusSnapshot = "status.snapshot"
	MethodWorkerLogsTail = "worker.logs.tail"
	MethodWorkerDispatch = "worker.dispatch"
	MethodWorkerStop     = "worker.stop"
	MethodProjectsList   = "projects.list"
	MethodProjectAdd     = "projects.add"
	MethodDaemonStop     = "daemon.stop"
)

// SnapshotResult is what status.snapshot returns.
type SnapshotResult struct {
	DaemonPID  int            `json:"daemonPid"`
	UptimeSec  int64          `json:"uptimeSec"`
	MCPURL     string         `json:"mcpUrl"`
	NumWorkers int            `json:"numWorkers"`
	Projects   []ProjectState `json:"projects"`
	Workers    []WorkerState  `json:"workers"`
}

// ProjectState mirrors one row of the daemon's projects table for CLI display.
type ProjectState struct {
	Name       string `json:"name"`
	Group      string `json:"group"`
	Dir        string `json:"dir"`
	State      string `json:"state"`
	CurrentRun string `json:"currentRun,omitempty"`
}

// WorkerState mirrors one row of the daemon's workers table.
type WorkerState struct {
	ID            string `json:"id"`
	ProjectName   string `json:"projectName"`
	State         string `json:"state"`
	Model         string `json:"model"`
	StartedAt     int64  `json:"startedAt"`
	EndedAt       int64  `json:"endedAt,omitempty"`
	LastHeartbeat int64  `json:"lastHeartbeat,omitempty"`
}

// LogsTailParams asks the daemon to stream lines from a worker's .jsonl.
type LogsTailParams struct {
	Project  string `json:"project,omitempty"`
	WorkerID string `json:"workerId,omitempty"`
	Follow   bool   `json:"follow"`
	Lines    int    `json:"lines,omitempty"` // last N lines before tail
}

// DaemonStopParams controls graceful daemon shutdown.
type DaemonStopParams struct {
	GraceSeconds int  `json:"graceSeconds"`
	Kill         bool `json:"kill"`
}
