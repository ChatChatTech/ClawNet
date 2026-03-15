package crypto

import (
	"crypto/ed25519"
	"fmt"

	libcrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"golang.org/x/crypto/curve25519"
)

// Ed25519PrivToCurve25519 converts a libp2p Ed25519 private key to a
// Curve25519 private key suitable for X25519 key exchange.
// Uses the standard RFC 8032 clamping on the first 32 bytes (seed → scalar).
func Ed25519PrivToCurve25519(priv libcrypto.PrivKey) ([32]byte, error) {
	raw, err := priv.Raw()
	if err != nil {
		return [32]byte{}, fmt.Errorf("extract raw key: %w", err)
	}
	if len(raw) != ed25519.PrivateKeySize {
		return [32]byte{}, fmt.Errorf("unexpected key length %d", len(raw))
	}
	// The first 32 bytes of an Ed25519 private key are the seed.
	// We hash with SHA-512 and clamp to get the Curve25519 scalar,
	// but Go's x/crypto/curve25519 accepts the seed directly via
	// the standard X25519 function. Use crypto/sha512 for the
	// standard Ed25519→X25519 conversion.
	var scalar [32]byte
	h := sha512Sum(raw[:32])
	copy(scalar[:], h[:32])
	scalar[0] &= 248
	scalar[31] &= 127
	scalar[31] |= 64
	return scalar, nil
}

// Ed25519PubToCurve25519 converts an Ed25519 public key to a Curve25519 public
// key for X25519 key exchange. This uses the birational map from the
// Edwards curve to the Montgomery curve.
func Ed25519PubToCurve25519(pub ed25519.PublicKey) ([32]byte, error) {
	if len(pub) != ed25519.PublicKeySize {
		return [32]byte{}, fmt.Errorf("invalid public key length %d", len(pub))
	}
	return edwardsToMontgomery(pub), nil
}

// PeerIDToCurve25519 extracts the Curve25519 public key from a libp2p peer ID.
func PeerIDToCurve25519(pid peer.ID) ([32]byte, error) {
	pub, err := pid.ExtractPublicKey()
	if err != nil {
		return [32]byte{}, fmt.Errorf("extract public key: %w", err)
	}
	raw, err := pub.Raw()
	if err != nil {
		return [32]byte{}, fmt.Errorf("extract raw pub: %w", err)
	}
	return Ed25519PubToCurve25519(raw)
}

// SharedSecret computes the X25519 shared secret between our private key
// and the peer's Curve25519 public key.
func SharedSecret(privScalar [32]byte, peerPub [32]byte) ([32]byte, error) {
	shared, err := curve25519.X25519(privScalar[:], peerPub[:])
	if err != nil {
		return [32]byte{}, fmt.Errorf("x25519: %w", err)
	}
	var result [32]byte
	copy(result[:], shared)
	return result, nil
}
