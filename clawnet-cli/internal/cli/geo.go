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

const db11AssetName = "IP2LOCATION-LITE-DB11.BIN.zip"

func cmdGeoUpgrade() error {
	dataDir := config.DataDir()
	geoDir := filepath.Join(dataDir, "data")
	os.MkdirAll(geoDir, 0700)

	db11Path := filepath.Join(geoDir, "IP2LOCATION-LITE-DB11.BIN")

	// Check if already installed
	if info, err := os.Stat(db11Path); err == nil {
		fmt.Printf("DB11 already installed (%d bytes, modified %s)\n",
			info.Size(), info.ModTime().Format("2006-01-02"))
		fmt.Println("Reinstalling with latest version...")
	}

	// Find the DB11 asset in the latest release
	fmt.Println("Fetching latest release...")
	release, err := fetchLatestRelease()
	if err != nil {
		return fmt.Errorf("fetch release: %w", err)
	}

	var assetURL string
	var assetSize int64
	for _, a := range release.Assets {
		if strings.Contains(a.Name, "DB11") && strings.HasSuffix(a.Name, ".zip") {
			assetURL = a.BrowserDownloadURL
			assetSize = a.Size
			break
		}
	}
	if assetURL == "" {
		return fmt.Errorf("DB11 asset not found in release %s — please upload %s as a release asset", release.TagName, db11AssetName)
	}

	fmt.Printf("Downloading DB11 (%d bytes)...\n", assetSize)

	// Download zip to temp file
	zipPath := db11Path + ".zip.tmp"
	if err := downloadGeoAsset(assetURL, zipPath); err != nil {
		os.Remove(zipPath)
		return fmt.Errorf("download: %w", err)
	}

	// Extract .BIN from zip
	fmt.Println("Extracting...")
	if err := extractDB11(zipPath, db11Path); err != nil {
		os.Remove(zipPath)
		return fmt.Errorf("extract: %w", err)
	}
	os.Remove(zipPath)

	info, _ := os.Stat(db11Path)
	fmt.Printf("✅ DB11 installed: %s (%d bytes)\n", db11Path, info.Size())
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

func extractDB11(zipPath, destPath string) error {
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
