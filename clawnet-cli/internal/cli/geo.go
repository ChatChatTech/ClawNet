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
	"github.com/ChatChatTech/ClawNet/clawnet-cli/internal/i18n"
)

const (
	geoUpgradeFileName = "IP2LOCATION-LITE-DB5.IPV6.BIN"
	geoDBURL           = "https://clawnet.cc/IP2LOCATION-LITE-DB5.IPV6.BIN.zip"
)

func cmdGeoUpgrade() error {
	force := len(os.Args) > 2 && (os.Args[2] == "--force" || os.Args[2] == "-f")

	dataDir := config.DataDir()
	geoDir := filepath.Join(dataDir, "data")
	os.MkdirAll(geoDir, 0700)

	destPath := filepath.Join(geoDir, geoUpgradeFileName)

	// Check if already installed
	if info, err := os.Stat(destPath); err == nil {
		if !force {
			fmt.Println(i18n.Tf("geo.already_installed", info.Size(), info.ModTime().Format("2006-01-02")))
			fmt.Println(i18n.T("geo.use_force"))
			return nil
		}
		fmt.Println(i18n.Tf("geo.redownloading", info.Size()))
	}

	fmt.Println(i18n.T("geo.downloading"))
	if err := downloadAndExtractGeoDB(geoDBURL, destPath); err != nil {
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

	fmt.Println(i18n.T("geo.extracting"))
	if err := extractGeoBIN(zipPath, destPath); err != nil {
		os.Remove(zipPath)
		return fmt.Errorf("extract: %w", err)
	}
	os.Remove(zipPath)

	info, _ := os.Stat(destPath)
	fmt.Println(i18n.Tf("geo.installed", filepath.Base(destPath), info.Size()))
	fmt.Println(i18n.T("geo.restart_hint"))
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
	return fmt.Errorf("%s", i18n.T("geo.no_bin_in_zip"))
}
