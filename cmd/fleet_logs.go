package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/zuplo/hike/internal/fleet"
)

var (
	fleetLogsFollow bool
	fleetLogsTail   int
)

var fleetLogsCmd = &cobra.Command{
	Use:   "logs [project] [worker-id]",
	Short: "Tail the SDK event stream for a worker",
	Long: `Tail the captured stream-JSON from a worker subprocess.

Without a worker-id, picks the most recent worker for the project (or, with
no args, the most recent worker overall). For v0 we read the .jsonl file
directly so this works even if the daemon is down.`,
	Args: cobra.MaximumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireConfig(); err != nil {
			return err
		}
		p := fleet.New(rootDir)

		logPath, err := pickWorkerLog(p, args)
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "Reading %s\n", logPath)

		return tailFile(logPath, fleetLogsTail, fleetLogsFollow, os.Stdout)
	},
}

func init() {
	fleetLogsCmd.Flags().BoolVarP(&fleetLogsFollow, "follow", "f", false, "follow new lines as they're appended")
	fleetLogsCmd.Flags().IntVarP(&fleetLogsTail, "lines", "n", 200, "show last N lines before following")
	fleetCmd.AddCommand(fleetLogsCmd)
}

// pickWorkerLog returns the .jsonl path matching args. With no args, picks
// the most recently modified .jsonl in WorkerLogDir. With one arg, treats it
// as a project name and picks the newest worker log whose name doesn't matter
// (v0 has no DB-backed project↔worker map, so we just pick latest).
func pickWorkerLog(p fleet.Paths, args []string) (string, error) {
	if len(args) >= 2 {
		// Direct: project + worker-id.
		return p.WorkerLog(args[1]), nil
	}
	if len(args) == 1 && strings.HasSuffix(args[0], ".jsonl") {
		// Allow passing a path directly for debug.
		return args[0], nil
	}
	dir := p.WorkerLogDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("reading %s: %w (no workers yet?)", dir, err)
	}
	var newest string
	var newestTime time.Time
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		if fi.ModTime().After(newestTime) {
			newestTime = fi.ModTime()
			newest = filepath.Join(dir, e.Name())
		}
	}
	if newest == "" {
		return "", fmt.Errorf("no worker logs found in %s", dir)
	}
	return newest, nil
}

// tailFile prints the last `tail` lines from path, then follows for new
// content if follow is true. Simple polling implementation — robust enough
// for line-oriented log files.
func tailFile(path string, tail int, follow bool, out io.Writer) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	// Read all, keep last `tail` lines.
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	var buf []string
	for scanner.Scan() {
		buf = append(buf, scanner.Text())
		if tail > 0 && len(buf) > tail {
			buf = buf[len(buf)-tail:]
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	for _, line := range buf {
		fmt.Fprintln(out, line)
	}

	if !follow {
		return nil
	}

	// Follow: re-stat the file periodically and read any new bytes.
	for {
		// Always position at end of file as of last read.
		pos, err := f.Seek(0, io.SeekCurrent)
		if err != nil {
			return err
		}
		fi, err := f.Stat()
		if err != nil {
			return err
		}
		if fi.Size() > pos {
			r := bufio.NewReader(f)
			for {
				line, err := r.ReadString('\n')
				if err != nil {
					if err == io.EOF {
						break
					}
					return err
				}
				fmt.Fprint(out, line)
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
}
