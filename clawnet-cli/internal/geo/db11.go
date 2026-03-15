//go:build db11

package geo

import _ "embed"

//go:embed data/IP2LOCATION-LITE-DB11.BIN.zip
var embeddedDBZip []byte

const embeddedDBFileName = "IP2LOCATION-LITE-DB11.BIN"
const embeddedDBType = "DB11"

// NewLocator creates a Locator using the embedded DB11 database (city-level).
func NewLocator(dataDir string) (*Locator, error) {
	return newLocator(dataDir, embeddedDBZip, embeddedDBFileName, embeddedDBType)
}
