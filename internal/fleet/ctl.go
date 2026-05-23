package fleet

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strconv"
	"sync/atomic"
	"time"
)

// Client is a one-shot JSON-RPC 2.0 client over the daemon's ctl.sock.
// Open with Dial, Call as many methods as you need, Close.
type Client struct {
	conn   net.Conn
	reader *bufio.Reader
	nextID atomic.Int64
}

// Dial opens a unix-socket connection to the daemon. Returns an error if the
// daemon is not running or the socket file is missing.
func Dial(socketPath string) (*Client, error) {
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("connecting to daemon at %s: %w", socketPath, err)
	}
	return &Client{conn: conn, reader: bufio.NewReader(conn)}, nil
}

// DialWithTimeout is Dial with a connect timeout.
func DialWithTimeout(socketPath string, timeout time.Duration) (*Client, error) {
	d := net.Dialer{Timeout: timeout}
	conn, err := d.Dial("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("connecting to daemon at %s: %w", socketPath, err)
	}
	return &Client{conn: conn, reader: bufio.NewReader(conn)}, nil
}

// Close closes the underlying socket. Safe to call multiple times.
func (c *Client) Close() error {
	if c.conn == nil {
		return nil
	}
	err := c.conn.Close()
	c.conn = nil
	return err
}

// Call issues a JSON-RPC request and decodes the result into out.
// Pass out=nil if you don't care about the result body. ctx may be nil for
// "no deadline".
func (c *Client) Call(ctx context.Context, method string, params any, out any) error {
	if ctx == nil {
		ctx = context.Background()
	}
	id := c.nextID.Add(1)
	idBytes, _ := json.Marshal(id)
	req := Request{
		JSONRPC: "2.0",
		ID:      idBytes,
		Method:  method,
		Params:  params,
	}
	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshaling rpc request: %w", err)
	}

	// Apply context deadline if any.
	if dl, ok := ctx.Deadline(); ok {
		_ = c.conn.SetDeadline(dl)
		defer func() { _ = c.conn.SetDeadline(time.Time{}) }()
	}

	if _, err := c.conn.Write(append(body, '\n')); err != nil {
		return fmt.Errorf("writing rpc request: %w", err)
	}

	line, err := c.reader.ReadBytes('\n')
	if err != nil {
		return fmt.Errorf("reading rpc response: %w", err)
	}
	var resp Response
	if err := json.Unmarshal(line, &resp); err != nil {
		return fmt.Errorf("decoding rpc response: %w (raw: %s)", err, string(line))
	}
	if resp.Error != nil {
		return resp.Error
	}
	if out != nil && len(resp.Result) > 0 {
		if err := json.Unmarshal(resp.Result, out); err != nil {
			return fmt.Errorf("decoding rpc result: %w", err)
		}
	}
	return nil
}

// ReadPort reads the daemon.port file and returns the integer port.
// Returns 0 + error if the file is missing or malformed.
func ReadPort(portFile string) (int, error) {
	data, err := os.ReadFile(portFile)
	if err != nil {
		return 0, err
	}
	port, err := strconv.Atoi(string(bytesTrim(data)))
	if err != nil {
		return 0, fmt.Errorf("parsing port file %s: %w", portFile, err)
	}
	return port, nil
}

// ReadPID reads the daemon.pid file.
func ReadPID(pidFile string) (int, error) {
	data, err := os.ReadFile(pidFile)
	if err != nil {
		return 0, err
	}
	pid, err := strconv.Atoi(string(bytesTrim(data)))
	if err != nil {
		return 0, fmt.Errorf("parsing pid file %s: %w", pidFile, err)
	}
	return pid, nil
}

// bytesTrim trims ASCII whitespace from both ends without importing bytes.
func bytesTrim(b []byte) []byte {
	start := 0
	end := len(b)
	for start < end && (b[start] == ' ' || b[start] == '\t' || b[start] == '\n' || b[start] == '\r') {
		start++
	}
	for end > start && (b[end-1] == ' ' || b[end-1] == '\t' || b[end-1] == '\n' || b[end-1] == '\r') {
		end--
	}
	return b[start:end]
}
