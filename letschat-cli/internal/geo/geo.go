package geo

import (
	"embed"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/ip2location/ip2location-go/v9"
)

//go:embed data/IP2LOCATION-LITE-DB1.BIN
var embeddedDB embed.FS

// GeoInfo holds geo information for an IP address.
type GeoInfo struct {
	Country   string  `json:"country"`
	Region    string  `json:"region"`
	City      string  `json:"city"`
	Latitude  float32 `json:"latitude"`
	Longitude float32 `json:"longitude"`
	Timezone  string  `json:"timezone"`
}

// Locator resolves IPs to geographic info.
type Locator struct {
	db     *ip2location.DB
	dbType string // "DB1" or "DB11"
}

// NewLocator creates a Locator. It first tries a user-provided DB11 file in dataDir,
// then falls back to the embedded DB1 (country only).
func NewLocator(dataDir string) (*Locator, error) {
	// Try user-upgraded DB11 first
	db11Path := filepath.Join(dataDir, "data", "IP2LOCATION-LITE-DB11.BIN")
	if _, err := os.Stat(db11Path); err == nil {
		db, err := ip2location.OpenDB(db11Path)
		if err == nil {
			return &Locator{db: db, dbType: "DB11"}, nil
		}
	}

	// Fall back to embedded DB1
	tmpDir := filepath.Join(dataDir, "data")
	os.MkdirAll(tmpDir, 0700)
	embeddedPath := filepath.Join(tmpDir, "IP2LOCATION-LITE-DB1.BIN")

	// Write embedded DB only if not already existing
	if _, err := os.Stat(embeddedPath); os.IsNotExist(err) {
		data, err := embeddedDB.ReadFile("data/IP2LOCATION-LITE-DB1.BIN")
		if err != nil {
			return nil, fmt.Errorf("read embedded db: %w", err)
		}
		if err := os.WriteFile(embeddedPath, data, 0644); err != nil {
			return nil, fmt.Errorf("write db file: %w", err)
		}
	}

	db, err := ip2location.OpenDB(embeddedPath)
	if err != nil {
		return nil, fmt.Errorf("open ip2location: %w", err)
	}
	return &Locator{db: db, dbType: "DB1"}, nil
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
		Latitude:  results.Latitude,
		Longitude: results.Longitude,
	}
	if l.dbType == "DB11" {
		gi.Region = cleanField(results.Region)
		gi.City = cleanField(results.City)
		gi.Timezone = cleanField(results.Timezone)
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
	if s == "This parameter is unavailable for selected data file. Please upgrade the data file." {
		return ""
	}
	return s
}
