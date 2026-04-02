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
	repo          = "zuplo/zproj"
	binDir        = ".zproj/bin"
	checkInterval = 24 * time.Hour
)

// BinDir returns ~/.zproj/bin
func BinDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, binDir), nil
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

// SelfUpdate downloads the latest release and replaces the binary in ~/.zproj/bin/.
func SelfUpdate(currentVersion string) error {
	latest, err := LatestVersion()
	if err != nil {
		return err
	}

	if latest == currentVersion {
		return fmt.Errorf("already at latest version (%s)", currentVersion)
	}

	installDir, err := BinDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(installDir, 0755); err != nil {
		return fmt.Errorf("creating install dir: %w", err)
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

	// Replace binary in ~/.zproj/bin/ (no sudo needed)
	src := filepath.Join(tmpDir, "zproj")
	dst := filepath.Join(installDir, "zproj")

	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("reading new binary: %w", err)
	}
	if err := os.WriteFile(dst, data, 0755); err != nil {
		return fmt.Errorf("writing new binary: %w", err)
	}

	writeCheckTimestamp()
	return nil
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

func stateDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".zproj")
}

func shouldCheck() bool {
	dir := stateDir()
	if dir == "" {
		return false
	}

	data, err := os.ReadFile(filepath.Join(dir, "update-check.json"))
	if err != nil {
		return true
	}

	var state checkState
	if err := json.Unmarshal(data, &state); err != nil {
		return true
	}

	return time.Since(state.LastCheck) > checkInterval
}

func writeCheckTimestamp() {
	dir := stateDir()
	if dir == "" {
		return
	}
	os.MkdirAll(dir, 0755)

	data, _ := json.Marshal(checkState{LastCheck: time.Now()})
	os.WriteFile(filepath.Join(dir, "update-check.json"), data, 0644)
}

type checkState struct {
	LastCheck time.Time `json:"last_check"`
}
