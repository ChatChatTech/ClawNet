//go:build db1

package geo

import _ "embed"

//go:embed data/IP2LOCATION-LITE-DB1.BIN.zip
var embeddedDBZip []byte

// NewLocator creates a Locator using the embedded DB1 database (country-level, smaller).
func NewLocator(dataDir string) (*Locator, error) {
	return newLocator(dataDir, embeddedDBZip, "IP2LOCATION-LITE-DB1.BIN", "DB1")
}
