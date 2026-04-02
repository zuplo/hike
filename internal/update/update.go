package update

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	repo         = "zuplo/zproj"
	checkFile    = ".zproj-update-check"
	checkInterval = 24 * time.Hour
)

type ghRelease struct {
	TagName string `json:"tag_name"`
}

// LatestVersion returns the latest release version tag (e.g. "0.1.0").
func LatestVersion() (string, error) {
	if _, err := exec.LookPath("gh"); err != nil {
		return "", fmt.Errorf("gh CLI not found")
	}

	var stdout bytes.Buffer
	cmd := exec.Command("gh", "release", "view", "--repo", repo, "--json", "tagName", "--jq", ".tagName")
	cmd.Stdout = &stdout
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to check latest release: %w", err)
	}

	tag := strings.TrimSpace(stdout.String())
	return strings.TrimPrefix(tag, "v"), nil
}

// SelfUpdate downloads the latest release and replaces the current binary.
func SelfUpdate(currentVersion string) error {
	latest, err := LatestVersion()
	if err != nil {
		return err
	}

	if latest == currentVersion {
		return fmt.Errorf("already at latest version (%s)", currentVersion)
	}

	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("could not determine executable path: %w", err)
	}
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return fmt.Errorf("could not resolve executable path: %w", err)
	}

	archive := fmt.Sprintf("zproj_%s_%s_%s.tar.gz", latest, runtime.GOOS, runtime.GOARCH)

	tmpDir, err := os.MkdirTemp("", "zproj-update-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	// Download using gh CLI (handles private repo auth)
	cmd := exec.Command("gh", "release", "download", "v"+latest,
		"--repo", repo,
		"--pattern", archive,
		"--dir", tmpDir,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("download failed: %w", err)
	}

	// Extract
	tarCmd := exec.Command("tar", "-xzf", filepath.Join(tmpDir, archive), "-C", tmpDir)
	if err := tarCmd.Run(); err != nil {
		return fmt.Errorf("extract failed: %w", err)
	}

	newBinary := filepath.Join(tmpDir, "zproj")

	// Replace current binary: rename old, move new, remove old
	backupPath := execPath + ".old"
	if err := os.Rename(execPath, backupPath); err != nil {
		return fmt.Errorf("could not backup current binary: %w", err)
	}

	if err := copyFile(newBinary, execPath); err != nil {
		// Restore backup on failure
		os.Rename(backupPath, execPath)
		return fmt.Errorf("could not install new binary: %w", err)
	}
	os.Remove(backupPath)

	// Reset the update check timestamp
	writeCheckTimestamp()

	return nil
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0755)
}

// CheckOutdated checks if a newer version is available, throttled to once per day.
// Returns the latest version string if outdated, empty string if current or check skipped.
func CheckOutdated(currentVersion string) string {
	if currentVersion == "dev" {
		return ""
	}

	if !shouldCheck() {
		return ""
	}

	// Run the check in a quick timeout so it doesn't slow down the CLI
	type result struct {
		version string
	}
	ch := make(chan result, 1)
	go func() {
		latest, err := LatestVersion()
		if err != nil {
			ch <- result{}
			return
		}
		writeCheckTimestamp()
		if latest != currentVersion {
			ch <- result{version: latest}
			return
		}
		ch <- result{}
	}()

	select {
	case r := <-ch:
		return r.version
	case <-time.After(2 * time.Second):
		return ""
	}
}

func checkFilePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, checkFile)
}

func shouldCheck() bool {
	path := checkFilePath()
	if path == "" {
		return false
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return true // No file = never checked
	}

	var state checkState
	if err := json.Unmarshal(data, &state); err != nil {
		return true
	}

	return time.Since(state.LastCheck) > checkInterval
}

func writeCheckTimestamp() {
	path := checkFilePath()
	if path == "" {
		return
	}

	data, _ := json.Marshal(checkState{LastCheck: time.Now()})
	os.WriteFile(path, data, 0644)
}

type checkState struct {
	LastCheck time.Time `json:"last_check"`
}
