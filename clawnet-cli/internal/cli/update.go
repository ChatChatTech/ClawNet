package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/ChatChatTech/ClawNet/clawnet-cli/internal/daemon"
)

const (
	updateOwner = "ChatChatTech"
	updateRepo  = "ClawNet"
)

type ghRelease struct {
	TagName string    `json:"tag_name"`
	Name    string    `json:"name"`
	Assets  []ghAsset `json:"assets"`
}

type ghAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

func cmdUpdate() error {
	current := "v" + daemon.Version
	fmt.Printf("Current version: %s\n", current)
	fmt.Println("Checking for updates...")

	release, err := fetchLatestRelease()
	if err != nil {
		return fmt.Errorf("check update: %w", err)
	}

	if release.TagName == current || release.TagName == daemon.Version {
		fmt.Println("Already up to date.")
		return nil
	}

	fmt.Printf("New version available: %s\n", release.TagName)

	// Find matching asset for current OS/arch
	assetName := fmt.Sprintf("clawnet-%s-%s", runtime.GOOS, runtime.GOARCH)
	var asset *ghAsset
	for i := range release.Assets {
		if strings.Contains(release.Assets[i].Name, assetName) {
			asset = &release.Assets[i]
			break
		}
	}
	if asset == nil {
		// Try the generic name
		for i := range release.Assets {
			if release.Assets[i].Name == "clawnet" {
				asset = &release.Assets[i]
				break
			}
		}
	}
	if asset == nil {
		return fmt.Errorf("no binary found for %s/%s in release %s", runtime.GOOS, runtime.GOARCH, release.TagName)
	}

	fmt.Printf("Downloading %s (%d bytes)...\n", asset.Name, asset.Size)

	// Download to temp file
	binPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("find current binary: %w", err)
	}

	tmpPath := binPath + ".update"
	if err := downloadAsset(asset.BrowserDownloadURL, tmpPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("download: %w", err)
	}

	// Make executable
	if err := os.Chmod(tmpPath, 0755); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("chmod: %w", err)
	}

	// Atomic replace: rename over the current binary
	if err := os.Rename(tmpPath, binPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("replace binary: %w", err)
	}

	fmt.Printf("Updated to %s successfully.\n", release.TagName)
	fmt.Println("Restart the daemon to use the new version.")
	return nil
}

func fetchLatestRelease() (*ghRelease, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", updateOwner, updateRepo)
	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "clawnet/"+daemon.Version)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("GitHub API returned %d: %s", resp.StatusCode, string(body))
	}

	var rel ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, err
	}
	return &rel, nil
}

func downloadAsset(url, dest string) error {
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("download returned %d", resp.StatusCode)
	}

	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()

	// Limit read to 200 MB to prevent abuse
	_, err = io.Copy(out, io.LimitReader(resp.Body, 200<<20))
	return err
}
