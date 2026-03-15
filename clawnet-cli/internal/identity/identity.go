package identity

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
)

const keyFileName = "identity.key"

// LoadOrGenerate loads an Ed25519 key from disk, or generates a new one.
// It enforces 0600 permissions on the key file.
func LoadOrGenerate(dataDir string) (crypto.PrivKey, error) {
	keyPath := filepath.Join(dataDir, keyFileName)

	info, err := os.Stat(keyPath)
	if err == nil {
		// File exists — enforce permissions (owner-only).
		if perm := info.Mode().Perm(); perm&0077 != 0 {
			// Attempt to fix permissions before erroring.
			if fixErr := os.Chmod(keyPath, 0600); fixErr != nil {
				return nil, fmt.Errorf("identity key %s has insecure permissions %o and could not be fixed: %w", keyPath, perm, fixErr)
			}
		}
		data, readErr := os.ReadFile(keyPath)
		if readErr != nil {
			return nil, readErr
		}
		return crypto.UnmarshalEd25519PrivateKey(data)
	}
	if !os.IsNotExist(err) {
		return nil, err
	}

	// Generate new Ed25519 key pair
	priv, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		return nil, err
	}

	raw, err := priv.Raw()
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return nil, err
	}
	if err := os.WriteFile(keyPath, raw, 0600); err != nil {
		return nil, err
	}

	return priv, nil
}

// PeerIDFromKey derives the libp2p Peer ID from a private key.
func PeerIDFromKey(priv crypto.PrivKey) (peer.ID, error) {
	return peer.IDFromPrivateKey(priv)
}
