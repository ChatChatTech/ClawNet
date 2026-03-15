package geo

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/ip2location/ip2location-go/v9"
)

// GeoInfo holds geo information for an IP address.
type GeoInfo struct {
	Country   string  `json:"country"`
	Region    string  `json:"region"`
	City      string  `json:"city"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Timezone  string  `json:"timezone"`
}

// Locator resolves IPs to geographic info.
type Locator struct {
	db     *ip2location.DB
	dbType string
}

// newLocator is the shared constructor used by DB-specific init files.
// It prefers a downloaded DB11 on disk over the embedded database.
func newLocator(dataDir string, dbZip []byte, dbFileName, dbType string) (*Locator, error) {
	tmpDir := filepath.Join(dataDir, "data")
	os.MkdirAll(tmpDir, 0700)

	// Prefer DB5.IPV6 on disk (downloaded via `clawnet geo-upgrade`)
	db5Path := filepath.Join(tmpDir, "IP2LOCATION-LITE-DB5.IPV6.BIN")
	if _, err := os.Stat(db5Path); err == nil {
		db, err := ip2location.OpenDB(db5Path)
		if err == nil {
			return &Locator{db: db, dbType: "DB5"}, nil
		}
	}

	// Also check legacy DB11 on disk
	db11Path := filepath.Join(tmpDir, "IP2LOCATION-LITE-DB11.BIN")
	if _, err := os.Stat(db11Path); err == nil {
		db, err := ip2location.OpenDB(db11Path)
		if err == nil {
			return &Locator{db: db, dbType: "DB11"}, nil
		}
	}

	// Fall back to embedded database
	dbPath := filepath.Join(tmpDir, dbFileName)
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		if err := extractBIN(dbPath, dbZip); err != nil {
			return nil, fmt.Errorf("extract embedded db: %w", err)
		}
	}

	db, err := ip2location.OpenDB(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open ip2location: %w", err)
	}
	return &Locator{db: db, dbType: dbType}, nil
}

func extractBIN(destPath string, zipData []byte) error {
	r, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		return err
	}
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
	return fmt.Errorf("no .BIN file found in embedded zip")
}

// DBType returns the current database type.
func (l *Locator) DBType() string {
	return l.dbType
}

// Close closes the database.
func (l *Locator) Close() {
	if l.db != nil {
		l.db.Close()
	}
}

// Lookup returns geo info for an IP string.
func (l *Locator) Lookup(ipStr string) *GeoInfo {
	if l == nil || l.db == nil {
		return nil
	}
	results, err := l.db.Get_all(ipStr)
	if err != nil {
		return nil
	}
	gi := &GeoInfo{
		Country:   cleanField(results.Country_short),
		Region:    cleanField(results.Region),
		City:      cleanField(results.City),
		Latitude:  float64(results.Latitude),
		Longitude: float64(results.Longitude),
		Timezone:  cleanField(results.Timezone),
	}
	// Fallback: if coordinates are 0,0, use country centroid
	if gi.Latitude == 0 && gi.Longitude == 0 && gi.Country != "" {
		if lat, lon, ok := CountryCentroid(gi.Country); ok {
			gi.Latitude = lat
			gi.Longitude = lon
		}
	}
	return gi
}

// LookupMultiaddr extracts an IP from a libp2p multiaddr string and looks it up.
func (l *Locator) LookupMultiaddr(maddr string) *GeoInfo {
	ip := ExtractIP(maddr)
	if ip == "" {
		return nil
	}
	return l.Lookup(ip)
}

// ExtractIP extracts an IP address from a multiaddr string like /ip4/1.2.3.4/tcp/4001.
func ExtractIP(maddr string) string {
	parts := strings.Split(maddr, "/")
	for i, p := range parts {
		if (p == "ip4" || p == "ip6") && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}

// IsPublicIP checks if an IP string is a public (non-private, non-loopback) address.
func IsPublicIP(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return false
	}
	// Also filter CGNAT (100.64.0.0/10)
	cgnat := net.IPNet{IP: net.ParseIP("100.64.0.0"), Mask: net.CIDRMask(10, 32)}
	if cgnat.Contains(ip) {
		return false
	}
	return true
}

// GeoLabel returns a human-readable label for a GeoInfo.
func (gi *GeoInfo) Label() string {
	if gi == nil {
		return "Unknown"
	}
	parts := []string{}
	if gi.City != "" && gi.City != "-" {
		parts = append(parts, gi.City)
	}
	if gi.Region != "" && gi.Region != "-" {
		parts = append(parts, gi.Region)
	}
	if gi.Country != "" && gi.Country != "-" {
		parts = append(parts, gi.Country)
	}
	if len(parts) == 0 {
		return "Unknown"
	}
	return strings.Join(parts, ", ")
}

func cleanField(s string) string {
	s = strings.TrimSpace(s)
	switch {
	case s == "This parameter is unavailable for selected data file. Please upgrade the data file.":
		return ""
	case strings.Contains(s, "missing in IPv4 BIN"):
		return ""
	}
	return s
}
