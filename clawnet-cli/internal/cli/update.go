package cli

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/ChatChatTech/ClawNet/clawnet-cli/internal/daemon"
	"github.com/ChatChatTech/ClawNet/clawnet-cli/internal/i18n"
)

const (
	updateOwner = "ChatChatTech"
	updateRepo  = "ClawNet"
	npmScope    = "@cctech2077"
	maxBinSize  = 200 << 20 // 200 MB safety cap
	httpTimeout = 15 * time.Second
	dlTimeout   = 5 * time.Minute
)

// npm registries tried in order (npmmirror first for China users)
var npmRegistries = []string{
	"https://registry.npmmirror.com",
	"https://registry.npmjs.org",
}

type ghRelease struct {
	TagName    string    `json:"tag_name"`
	Name       string    `json:"name"`
	Prerelease bool      `json:"prerelease"`
	Assets     []ghAsset `json:"assets"`
}

type ghAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

func cmdUpdate() error {
	current := "v" + daemon.Version
	fmt.Println(i18n.Tf("update.current", current))

	// Parse --source flag: auto (default), npm, github
	source := "auto"
	for _, a := range os.Args[2:] {
		switch a {
		case "--npm":
			source = "npm"
		case "--github":
			source = "github"
		}
	}

	fmt.Println(i18n.T("update.checking"))

	release, err := fetchLatestRelease()
	if err != nil {
		return fmt.Errorf("check update: %w", err)
	}

	if release.TagName == current || release.TagName == daemon.Version {
		fmt.Println(i18n.T("update.up_to_date"))
		return nil
	}

	fmt.Println(i18n.Tf("update.available", release.TagName))

	ver := strings.TrimPrefix(release.TagName, "v")

	binPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("find current binary: %w", err)
	}
	tmpPath := binPath + ".update"
	defer os.Remove(tmpPath)

	var dlErr error
	switch source {
	case "npm":
		dlErr = downloadFromNpm(ver, tmpPath)
	case "github":
		dlErr = downloadFromGitHub(release, tmpPath)
	default: // auto: npm first, then GitHub
		dlErr = downloadFromNpm(ver, tmpPath)
		if dlErr != nil {
			fmt.Println(i18n.T("update.npm_failed_trying_github"))
			dlErr = downloadFromGitHub(release, tmpPath)
		}
	}
	if dlErr != nil {
		return fmt.Errorf("download: %w", dlErr)
	}

	if err := os.Chmod(tmpPath, 0755); err != nil {
		return fmt.Errorf("chmod: %w", err)
	}

	// Atomic replace
	if err := os.Rename(tmpPath, binPath); err != nil {
		return fmt.Errorf("replace binary: %w", err)
	}

	fmt.Println(i18n.Tf("update.success", release.TagName))
	fmt.Println(i18n.T("update.restart_hint"))
	return nil
}

// ── GitHub Releases source ──────────────────────────────────────

// fetchLatestRelease fetches the newest release (including pre-releases).
func fetchLatestRelease() (*ghRelease, error) {
	// /releases?per_page=1 includes pre-releases; /releases/latest does NOT.
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases?per_page=1",
		updateOwner, updateRepo)

	client := &http.Client{Timeout: httpTimeout}
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "clawnet/"+daemon.Version)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("GitHub API %d: %s", resp.StatusCode, string(body))
	}

	var releases []ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, err
	}
	if len(releases) == 0 {
		return nil, fmt.Errorf("no releases found")
	}
	return &releases[0], nil
}

// downloadFromGitHub downloads the platform archive from GitHub Releases
// and extracts the binary. Supports .tar.gz and .zip assets.
func downloadFromGitHub(release *ghRelease, dest string) error {
	// Actual asset naming: clawnet-v{tag}-{os}-{arch}.tar.gz / .zip
	osName := runtime.GOOS
	archName := runtime.GOARCH
	tag := release.TagName

	// Candidate patterns ordered by specificity
	candidates := []string{
		fmt.Sprintf("clawnet-%s-%s-%s", tag, osName, archName),      // clawnet-v1.0.0-beta.9-linux-amd64
		fmt.Sprintf("clawnet-%s-%s", osName, archName),              // clawnet-linux-amd64
	}

	var asset *ghAsset
	for _, cand := range candidates {
		for i := range release.Assets {
			if strings.Contains(release.Assets[i].Name, cand) {
				asset = &release.Assets[i]
				break
			}
		}
		if asset != nil {
			break
		}
	}
	if asset == nil {
		return fmt.Errorf("no asset for %s/%s in release %s (assets: %s)",
			osName, archName, tag, assetNames(release.Assets))
	}

	fmt.Println(i18n.Tf("update.downloading_github", asset.Name, asset.Size))

	data, err := downloadToMemory(asset.BrowserDownloadURL)
	if err != nil {
		return err
	}

	// Determine archive format and extract
	switch {
	case strings.HasSuffix(asset.Name, ".tar.gz") || strings.HasSuffix(asset.Name, ".tgz"):
		return extractBinaryFromTgz(bytes.NewReader(data), binaryNameForOS(), dest)
	case strings.HasSuffix(asset.Name, ".zip"):
		return extractBinaryFromZip(data, binaryNameForOS(), dest)
	default:
		// Raw binary (legacy format)
		return os.WriteFile(dest, data, 0755)
	}
}

// ── npm registry source ─────────────────────────────────────────

// downloadFromNpm downloads the binary from npm registry (npmmirror → npmjs).
func downloadFromNpm(version, dest string) error {
	npmOS := runtime.GOOS
	if npmOS == "windows" {
		npmOS = "win32"
	}
	npmArch := runtime.GOARCH
	if npmArch == "amd64" {
		npmArch = "x64"
	}

	pkgBase := fmt.Sprintf("clawnet-%s-%s", npmOS, npmArch)
	pkgName := fmt.Sprintf("%s/%s", npmScope, pkgBase)
	binName := binaryNameForOS()

	client := &http.Client{Timeout: dlTimeout}

	for _, registry := range npmRegistries {
		tarballURL := fmt.Sprintf("%s/%s/-/%s-%s.tgz", registry, pkgName, pkgBase, version)
		fmt.Println(i18n.Tf("update.trying_npm", registry))

		req, err := http.NewRequest("GET", tarballURL, nil)
		if err != nil {
			continue
		}
		req.Header.Set("User-Agent", "clawnet/"+daemon.Version)

		resp, err := client.Do(req)
		if err != nil {
			continue
		}

		if resp.StatusCode != 200 {
			resp.Body.Close()
			continue
		}

		err = extractBinaryFromTgz(resp.Body, binName, dest)
		resp.Body.Close()
		if err != nil {
			continue
		}

		info, _ := os.Stat(dest)
		if info != nil && info.Size() > 0 {
			fmt.Println(i18n.Tf("update.downloaded_npm", info.Size()))
			return nil
		}
	}
	return fmt.Errorf("npm download failed for all registries")
}

// ── Archive extraction helpers ──────────────────────────────────

// extractBinaryFromTgz extracts the clawnet binary from a .tar.gz stream.
// Matches by filename suffix since paths vary between npm tarballs and
// GitHub release archives.
func extractBinaryFromTgz(r io.Reader, binName, dest string) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		base := filepath.Base(hdr.Name)
		// Match the binary whether it's at top level or nested (package/bin/clawnet)
		if base == binName || base == "clawnet" || base == "clawnet.exe" {
			out, err := os.Create(dest)
			if err != nil {
				return err
			}
			_, err = io.Copy(out, io.LimitReader(tr, maxBinSize))
			out.Close()
			return err
		}
	}
	return fmt.Errorf("binary %q not found in tar.gz", binName)
}

// extractBinaryFromZip extracts the clawnet binary from a .zip buffer (Windows).
func extractBinaryFromZip(data []byte, binName, dest string) error {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return err
	}
	for _, f := range zr.File {
		base := filepath.Base(f.Name)
		if base == binName || base == "clawnet" || base == "clawnet.exe" {
			rc, err := f.Open()
			if err != nil {
				return err
			}
			out, err := os.Create(dest)
			if err != nil {
				rc.Close()
				return err
			}
			_, err = io.Copy(out, io.LimitReader(rc, maxBinSize))
			rc.Close()
			out.Close()
			return err
		}
	}
	return fmt.Errorf("binary %q not found in zip", binName)
}

// ── Shared helpers ──────────────────────────────────────────────

func downloadToMemory(url string) ([]byte, error) {
	client := &http.Client{Timeout: dlTimeout}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("download returned HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxBinSize))
}

func binaryNameForOS() string {
	if runtime.GOOS == "windows" {
		return "clawnet.exe"
	}
	return "clawnet"
}

func assetNames(assets []ghAsset) string {
	names := make([]string, len(assets))
	for i, a := range assets {
		names[i] = a.Name
	}
	return strings.Join(names, ", ")
}

// downloadAsset downloads a URL to a local file (used by nutshell installer too).
func downloadAsset(url, dest string) error {
	data, err := downloadToMemory(url)
	if err != nil {
		return err
	}
	return os.WriteFile(dest, data, 0644)
}
