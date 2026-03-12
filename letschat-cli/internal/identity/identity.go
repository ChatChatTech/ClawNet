package identity

import (
	"crypto/rand"
	"os"
	"path/filepath"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
)

const keyFileName = "identity.key"

// LoadOrGenerate loads an Ed25519 key from disk, or generates a new one.
func LoadOrGenerate(dataDir string) (crypto.PrivKey, error) {
	keyPath := filepath.Join(dataDir, keyFileName)

	data, err := os.ReadFile(keyPath)
	if err == nil {
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
