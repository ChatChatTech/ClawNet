package geo

import _ "embed"

//go:embed data/IP2LOCATION-LITE-DB1.IPV6.BIN.zip
var embeddedDBZip []byte

const embeddedDBFileName = "IP2LOCATION-LITE-DB1.IPV6.BIN"
const embeddedDBType = "DB1"

// NewLocator creates a Locator. It prefers a DB5.IPV6 file on disk (via geo-upgrade)
// and falls back to the embedded DB1.IPV6 database (country-level, IPv4+IPv6).
func NewLocator(dataDir string) (*Locator, error) {
	return newLocator(dataDir, embeddedDBZip, embeddedDBFileName, embeddedDBType)
}
