//go:build !db1

package geo

import _ "embed"

//go:embed data/IP2LOCATION-LITE-DB11.BIN.zip
var embeddedDBZip []byte

// NewLocator creates a Locator using the embedded DB11 database (city-level).
func NewLocator(dataDir string) (*Locator, error) {
	return newLocator(dataDir, embeddedDBZip, "IP2LOCATION-LITE-DB11.BIN", "DB11")
}
