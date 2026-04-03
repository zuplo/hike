package update

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	repo          = "zuplo/hike"
	binDir        = ".hike/bin"
	checkInterval = 24 * time.Hour
)

// BinDir returns ~/.hike/bin
func BinDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, binDir), nil
}

type ghRelease struct {
	TagName string `json:"tag_name"`
}

// LatestVersion returns the latest release version tag (e.g. "0.1.0").
func LatestVersion() (string, error) {
	resp, err := http.Get(fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo))
	if err != nil {
		return "", fmt.Errorf("failed to check latest release: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}

	var release ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", fmt.Errorf("parsing release: %w", err)
	}

	return strings.TrimPrefix(release.TagName, "v"), nil
}

// SelfUpdate downloads the latest release and replaces the binary in ~/.hike/bin/.
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

	archive := fmt.Sprintf("hike_%s_%s_%s.tar.gz", latest, runtime.GOOS, runtime.GOARCH)
	url := fmt.Sprintf("https://github.com/%s/releases/download/v%s/%s", repo, latest, archive)

	tmpDir, err := os.MkdirTemp("", "hike-update-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	// Download
	archivePath := filepath.Join(tmpDir, archive)
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("download failed: HTTP %d", resp.StatusCode)
	}

	f, err := os.Create(archivePath)
	if err != nil {
		return err
	}
	if _, err := f.ReadFrom(resp.Body); err != nil {
		f.Close()
		return fmt.Errorf("download failed: %w", err)
	}
	f.Close()

	// Extract
	tarCmd := exec.Command("tar", "-xzf", archivePath, "-C", tmpDir)
	if err := tarCmd.Run(); err != nil {
		return fmt.Errorf("extract failed: %w", err)
	}

	// Replace binary in ~/.hike/bin/ (no sudo needed)
	src := filepath.Join(tmpDir, "hike")
	dst := filepath.Join(installDir, "hike")

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
	return filepath.Join(home, ".hike")
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
