package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"
)

// ═══════════════════════════════════════════════════════════
// Silent Auto-Updater
// ═══════════════════════════════════════════════════════════
//
// Checks GitHub Releases every 30 minutes. If a newer version exists,
// downloads the matching binary, replaces the running executable,
// and restarts via exec. Fully silent — no output unless an actual
// update happens. Cannot be disabled by the user.

const (
	updateCheckInterval = 30 * time.Minute
	updateOwner         = "ChatChatTech"
	updateRepo          = "ClawNet"
	updateTimeout       = 2 * time.Minute
)

type ghRelease struct {
	TagName string    `json:"tag_name"`
	Assets  []ghAsset `json:"assets"`
}

type ghAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// startAutoUpdater launches the silent auto-update loop.
func (d *Daemon) startAutoUpdater(ctx context.Context) {
	go func() {
		// Initial delay — let the daemon fully start
		select {
		case <-time.After(2 * time.Minute):
		case <-ctx.Done():
			return
		}

		d.checkAndUpdate()

		ticker := time.NewTicker(updateCheckInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				d.checkAndUpdate()
			}
		}
	}()
}

// checkAndUpdate checks for a newer release and performs the update silently.
func (d *Daemon) checkAndUpdate() {
	latest, err := fetchLatestRelease()
	if err != nil {
		return // silent fail
	}

	latestVer := strings.TrimPrefix(latest.TagName, "v")
	if !isNewer(latestVer, Version) {
		return
	}

	// Find matching binary asset
	assetName := fmt.Sprintf("clawnet-%s-%s", runtime.GOOS, runtime.GOARCH)
	var downloadURL string
	for _, a := range latest.Assets {
		if a.Name == assetName {
			downloadURL = a.BrowserDownloadURL
			break
		}
	}
	if downloadURL == "" {
		return // no matching asset for this platform
	}

	// Download to temp file
	exePath, err := os.Executable()
	if err != nil {
		return
	}

	tmpPath := exePath + ".update"
	if err := downloadBinary(downloadURL, tmpPath); err != nil {
		os.Remove(tmpPath)
		return
	}

	// Atomic replace: rename old → .old, new → current
	oldPath := exePath + ".old"
	os.Remove(oldPath) // remove any previous .old

	if err := os.Rename(exePath, oldPath); err != nil {
		os.Remove(tmpPath)
		return
	}
	if err := os.Rename(tmpPath, exePath); err != nil {
		// Try to restore
		os.Rename(oldPath, exePath)
		return
	}

	// Make executable
	os.Chmod(exePath, 0755)

	// Clean up old binary (best effort)
	os.Remove(oldPath)

	fmt.Printf("[update] updated to v%s\n", latestVer)

	// Restart via exec to load the new binary
	restartSelf(exePath)
}

// fetchLatestRelease queries GitHub API for the latest release.
func fetchLatestRelease() (*ghRelease, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", updateOwner, updateRepo)

	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("github api: %s", resp.Status)
	}

	var rel ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, err
	}
	return &rel, nil
}

// downloadBinary downloads a file from url to dst.
func downloadBinary(url, dst string) error {
	client := &http.Client{Timeout: updateTimeout}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("download: %s", resp.Status)
	}

	f, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, resp.Body)
	return err
}

// isNewer returns true if latest > current using simple semver comparison.
func isNewer(latest, current string) bool {
	lp := parseSemver(latest)
	cp := parseSemver(current)
	for i := 0; i < 3; i++ {
		if lp[i] > cp[i] {
			return true
		}
		if lp[i] < cp[i] {
			return false
		}
	}
	return false
}

// parseSemver extracts [major, minor, patch] from a version string.
func parseSemver(v string) [3]int {
	v = strings.TrimPrefix(v, "v")
	var parts [3]int
	n := 0
	for _, ch := range v {
		if ch >= '0' && ch <= '9' {
			parts[n] = parts[n]*10 + int(ch-'0')
		} else if ch == '.' {
			n++
			if n >= 3 {
				break
			}
		} else {
			break // stop at pre-release suffix
		}
	}
	return parts
}
