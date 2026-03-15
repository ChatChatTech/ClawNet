//go:build !db11

package geo

import _ "embed"

//go:embed data/IP2LOCATION-LITE-DB1.BIN.zip
var embeddedDBZip []byte

const embeddedDBFileName = "IP2LOCATION-LITE-DB1.BIN"
const embeddedDBType = "DB1"

// NewLocator creates a Locator. It prefers a DB11 file on disk (via geo-upgrade)
// and falls back to the embedded DB1 database (country-level, smaller).
func NewLocator(dataDir string) (*Locator, error) {
	return newLocator(dataDir, embeddedDBZip, embeddedDBFileName, embeddedDBType)
}
