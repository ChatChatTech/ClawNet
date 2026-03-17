package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	nutshellOwner   = "ChatChatTech"
	nutshellRepo    = "nutshell"
	nutshellBinName = "nutshell"
)

// cmdNutshell handles `clawnet nutshell <subcommand>`.
func cmdNutshell() error {
	if len(os.Args) < 3 {
		return nutshellUsage()
	}

	sub := os.Args[2]
	switch sub {
	case "install":
		return nutshellInstall(false)
	case "upgrade":
		return nutshellInstall(true)
	case "uninstall":
		return nutshellUninstall()
	case "version":
		return nutshellVersion()
	case "status":
		return nutshellStatus()
	default:
		fmt.Fprintf(os.Stderr, "unknown nutshell subcommand: %s\n", sub)
		return nutshellUsage()
	}
}

func nutshellUsage() error {
	fmt.Println("Usage: clawnet nutshell <subcommand>")
	fmt.Println()
	fmt.Println("Subcommands:")
	fmt.Println("  install    Download and install the Nutshell CLI")
	fmt.Println("  upgrade    Upgrade Nutshell CLI to the latest version")
	fmt.Println("  uninstall  Remove the Nutshell CLI binary")
	fmt.Println("  version    Show installed Nutshell version")
	fmt.Println("  status     Check if Nutshell is installed and show info")
	return nil
}

// nutshellBinPath returns the path where nutshell should be installed.
func nutshellBinPath() string {
	// Prefer placing it next to the clawnet binary
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		candidate := filepath.Join(dir, nutshellBinName)
		// Check if directory is writable
		testFile := filepath.Join(dir, ".nutshell-write-test")
		if f, err := os.Create(testFile); err == nil {
			f.Close()
			os.Remove(testFile)
			return candidate
		}
	}
	// Fallback to /usr/local/bin
	return filepath.Join("/usr/local/bin", nutshellBinName)
}

// nutshellInstall downloads and installs the nutshell binary.
// If upgrade=true, it replaces the existing binary.
func nutshellInstall(upgrade bool) error {
	binPath := nutshellBinPath()

	if !upgrade {
		if _, err := exec.LookPath(nutshellBinName); err == nil {
			fmt.Println("Nutshell is already installed. Use 'clawnet nutshell upgrade' to update.")
			return nutshellVersion()
		}
	}

	fmt.Println("Fetching latest Nutshell release...")
	release, err := fetchNutshellRelease()
	if err != nil {
		return fmt.Errorf("fetch nutshell release: %w", err)
	}

	// Compare versions before downloading
	if upgrade {
		if installed := installedNutshellVersion(); installed != "" {
			latestVer := strings.TrimPrefix(release.TagName, "v")
			if installed == latestVer {
				fmt.Printf("Nutshell is already up to date (v%s).\n", installed)
				return nil
			}
			fmt.Printf("Upgrading Nutshell v%s → %s...\n", installed, release.TagName)
		} else {
			fmt.Printf("Upgrading Nutshell to %s...\n", release.TagName)
		}
	} else {
		fmt.Printf("Installing Nutshell %s...\n", release.TagName)
	}

	// Find matching asset
	osName := runtime.GOOS
	archName := runtime.GOARCH
	assetPattern := fmt.Sprintf("nutshell-%s-%s", osName, archName)

	var asset *ghAsset
	for i := range release.Assets {
		if strings.Contains(release.Assets[i].Name, assetPattern) {
			asset = &release.Assets[i]
			break
		}
	}
	if asset == nil {
		// Try generic name
		for i := range release.Assets {
			if release.Assets[i].Name == nutshellBinName {
				asset = &release.Assets[i]
				break
			}
		}
	}
	if asset == nil {
		return fmt.Errorf("no nutshell binary found for %s/%s in release %s", osName, archName, release.TagName)
	}
	fmt.Printf("Downloading %s (%d bytes)...\n", asset.Name, asset.Size)

	tmpPath := binPath + ".download"
	if err := downloadAsset(asset.BrowserDownloadURL, tmpPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("download nutshell: %w", err)
	}

	if err := os.Chmod(tmpPath, 0755); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("chmod: %w", err)
	}

	if err := os.Rename(tmpPath, binPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("install nutshell to %s: %w", binPath, err)
	}

	fmt.Printf("Nutshell installed at %s\n", binPath)
	return nutshellVersion()
}

// nutshellUninstall removes the nutshell binary.
func nutshellUninstall() error {
	path, err := exec.LookPath(nutshellBinName)
	if err != nil {
		fmt.Println("Nutshell is not installed.")
		return nil
	}

	fmt.Printf("Removing %s...\n", path)
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("remove nutshell: %w", err)
	}
	fmt.Println("Nutshell has been uninstalled.")
	return nil
}

// installedNutshellVersion returns the installed nutshell version string (e.g. "0.2.4"), or empty if unavailable.
func installedNutshellVersion() string {
	path, err := exec.LookPath(nutshellBinName)
	if err != nil {
		return ""
	}
	out, err := exec.Command(path, "version").CombinedOutput()
	if err != nil {
		return ""
	}
	// Expected output: "nutshell v0.2.4\n"
	s := strings.TrimSpace(string(out))
	s = strings.TrimPrefix(s, "nutshell v")
	s = strings.TrimPrefix(s, "nutshell ")
	return s
}

// nutshellVersion prints the installed nutshell version.
func nutshellVersion() error {
	path, err := exec.LookPath(nutshellBinName)
	if err != nil {
		fmt.Println("Nutshell is not installed. Run 'clawnet nutshell install' to install.")
		return nil
	}

	out, err := exec.Command(path, "version").CombinedOutput()
	if err != nil {
		return fmt.Errorf("nutshell version: %w", err)
	}
	fmt.Print(string(out))
	return nil
}

// nutshellStatus shows whether nutshell is installed and basic info.
func nutshellStatus() error {
	path, err := exec.LookPath(nutshellBinName)
	if err != nil {
		fmt.Println("Status: NOT INSTALLED")
		fmt.Println("Run 'clawnet nutshell install' to install.")
		return nil
	}

	fmt.Printf("Status: installed\n")
	fmt.Printf("Path:   %s\n", path)

	if info, err := os.Stat(path); err == nil {
		fmt.Printf("Size:   %.1f MB\n", float64(info.Size())/(1024*1024))
	}

	out, err := exec.Command(path, "version").CombinedOutput()
	if err == nil {
		fmt.Printf("Version: %s", string(out))
	}
	return nil
}

func fetchNutshellRelease() (*ghRelease, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", nutshellOwner, nutshellRepo)
	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "clawnet-nutshell-manager")

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
