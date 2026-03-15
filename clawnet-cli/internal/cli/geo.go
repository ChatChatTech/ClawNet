package cli

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ChatChatTech/ClawNet/clawnet-cli/internal/config"
)

const geoUpgradeFileName = "IP2LOCATION-LITE-DB5.IPV6.BIN"

func cmdGeoUpgrade() error {
	dataDir := config.DataDir()
	geoDir := filepath.Join(dataDir, "data")
	os.MkdirAll(geoDir, 0700)

	destPath := filepath.Join(geoDir, geoUpgradeFileName)

	// Check if already installed
	if info, err := os.Stat(destPath); err == nil {
		fmt.Printf("DB5.IPV6 already installed (%d bytes, modified %s)\n",
			info.Size(), info.ModTime().Format("2006-01-02"))
		fmt.Println("Reinstalling with latest version...")
	}

	// Download from GitHub release
	fmt.Println("Fetching latest release...")
	release, err := fetchLatestRelease()
	if err != nil {
		return fmt.Errorf("fetch release: %w", err)
	}

	var assetURL string
	var assetSize int64
	for _, a := range release.Assets {
		if strings.Contains(a.Name, "DB5") && strings.Contains(a.Name, "IPV6") && strings.HasSuffix(a.Name, ".zip") {
			assetURL = a.BrowserDownloadURL
			assetSize = a.Size
			break
		}
	}
	if assetURL == "" {
		return fmt.Errorf("geo DB asset not found in release %s — please check https://github.com/ChatChatTech/ClawNet/releases",
			release.TagName)
	}

	fmt.Printf("Downloading DB5.IPV6 (%d bytes) from %s...\n", assetSize, release.TagName)
	if err := downloadAndExtractGeoDB(assetURL, destPath); err != nil {
		return err
	}
	return nil
}

func downloadAndExtractGeoDB(url, destPath string) error {
	zipPath := destPath + ".zip.tmp"
	if err := downloadGeoAsset(url, zipPath); err != nil {
		os.Remove(zipPath)
		return fmt.Errorf("download: %w", err)
	}

	fmt.Println("Extracting...")
	if err := extractGeoBIN(zipPath, destPath); err != nil {
		os.Remove(zipPath)
		return fmt.Errorf("extract: %w", err)
	}
	os.Remove(zipPath)

	info, _ := os.Stat(destPath)
	fmt.Printf("✅ Geo DB installed: %s (%d bytes)\n", filepath.Base(destPath), info.Size())
	fmt.Println("Restart the daemon to use city-level geolocation.")
	return nil
}

func downloadGeoAsset(url, destPath string) error {
	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, resp.Body)
	return err
}

func extractGeoBIN(zipPath, destPath string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()
	for _, f := range r.File {
		if strings.HasSuffix(f.Name, ".BIN") {
			rc, err := f.Open()
			if err != nil {
				return err
			}
			defer rc.Close()
			out, err := os.Create(destPath)
			if err != nil {
				return err
			}
			defer out.Close()
			_, err = io.Copy(out, rc)
			return err
		}
	}
	return fmt.Errorf("no .BIN file found in zip")
}
