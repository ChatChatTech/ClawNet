package overlay

// Overlay wire handshake protocol.
// ClawNet overlay nodes use this to peer with the global mesh (~4000 nodes).
//
// Wire format:
//   "meta" (4 bytes) + uint16(remaining length) + TLV fields + ed25519 signature
//   TLV fields: metaVersionMajor(0), metaVersionMinor(5), metaPublicKey(32), metaPriority(1)
//   Signature covers blake2b-512(password || publicKey)

import (
	"bytes"
	"crypto/ed25519"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"time"

	"golang.org/x/crypto/blake2b"
)

const (
	wireProtoMajor uint16 = 0
	wireProtoMinor uint16 = 5
	handshakeDeadline     = 6 * time.Second
)

// TLV type tags (wire protocol; order is immutable).
const (
	tlvVersionMajor uint16 = iota // uint16
	tlvVersionMinor               // uint16
	tlvPublicKey                  // [32]byte
	tlvPriority                   // uint8
)

var metaPreamble = [4]byte{'m', 'e', 't', 'a'}

// overlayHandshake performs a wire-compatible handshake on conn.
// Returns the remote's ed25519 public key and negotiated priority.
// password should be nil for open peering (standard for public peers).
func overlayHandshake(conn net.Conn, privKey ed25519.PrivateKey, priority uint8, password []byte) (ed25519.PublicKey, uint8, error) {
	localPub := privKey.Public().(ed25519.PublicKey)

	// --- Encode our metadata ---
	bs := make([]byte, 0, 128)
	bs = append(bs, metaPreamble[:]...)
	bs = append(bs, 0, 0) // placeholder for remaining length

	// TLV: version major
	bs = binary.BigEndian.AppendUint16(bs, tlvVersionMajor)
	bs = binary.BigEndian.AppendUint16(bs, 2)
	bs = binary.BigEndian.AppendUint16(bs, wireProtoMajor)

	// TLV: version minor
	bs = binary.BigEndian.AppendUint16(bs, tlvVersionMinor)
	bs = binary.BigEndian.AppendUint16(bs, 2)
	bs = binary.BigEndian.AppendUint16(bs, wireProtoMinor)

	// TLV: public key
	bs = binary.BigEndian.AppendUint16(bs, tlvPublicKey)
	bs = binary.BigEndian.AppendUint16(bs, ed25519.PublicKeySize)
	bs = append(bs, localPub...)

	// TLV: priority
	bs = binary.BigEndian.AppendUint16(bs, tlvPriority)
	bs = binary.BigEndian.AppendUint16(bs, 1)
	bs = append(bs, priority)

	// blake2b-512(password, publicKey) + ed25519 sign
	hasher, err := blake2b.New512(password)
	if err != nil {
		return nil, 0, fmt.Errorf("blake2b init: %w", err)
	}
	if _, err := hasher.Write(localPub); err != nil {
		return nil, 0, fmt.Errorf("blake2b write: %w", err)
	}
	hash := hasher.Sum(nil)
	sig := ed25519.Sign(privKey, hash)
	bs = append(bs, sig...)

	// Fill in remaining length (everything after the 6-byte header)
	binary.BigEndian.PutUint16(bs[4:6], uint16(len(bs)-6))

	// --- Send ---
	if err := conn.SetDeadline(time.Now().Add(handshakeDeadline)); err != nil {
		return nil, 0, fmt.Errorf("set deadline: %w", err)
	}
	if _, err := conn.Write(bs); err != nil {
		return nil, 0, fmt.Errorf("write handshake: %w", err)
	}

	// --- Decode remote metadata ---
	var hdr [6]byte
	if _, err := io.ReadFull(conn, hdr[:]); err != nil {
		return nil, 0, fmt.Errorf("read header: %w", err)
	}
	if !bytes.Equal(hdr[:4], metaPreamble[:]) {
		return nil, 0, fmt.Errorf("invalid preamble: not overlay-compatible")
	}
	remLen := binary.BigEndian.Uint16(hdr[4:6])
	if remLen < ed25519.SignatureSize {
		return nil, 0, fmt.Errorf("handshake too short (%d bytes)", remLen)
	}
	payload := make([]byte, remLen)
	if _, err := io.ReadFull(conn, payload); err != nil {
		return nil, 0, fmt.Errorf("read payload: %w", err)
	}

	remoteSig := payload[len(payload)-ed25519.SignatureSize:]
	tlvData := payload[:len(payload)-ed25519.SignatureSize]

	var remoteMajor, remoteMinor uint16
	var remotePub ed25519.PublicKey
	var remotePrio uint8

	for len(tlvData) >= 4 {
		tag := binary.BigEndian.Uint16(tlvData[:2])
		tlen := binary.BigEndian.Uint16(tlvData[2:4])
		tlvData = tlvData[4:]
		if len(tlvData) < int(tlen) {
			break
		}
		switch tag {
		case tlvVersionMajor:
			remoteMajor = binary.BigEndian.Uint16(tlvData[:2])
		case tlvVersionMinor:
			remoteMinor = binary.BigEndian.Uint16(tlvData[:2])
		case tlvPublicKey:
			remotePub = make(ed25519.PublicKey, ed25519.PublicKeySize)
			copy(remotePub, tlvData[:ed25519.PublicKeySize])
		case tlvPriority:
			remotePrio = tlvData[0]
		}
		tlvData = tlvData[tlen:]
	}

	// Verify version
	if remoteMajor != wireProtoMajor || remoteMinor != wireProtoMinor {
		return nil, 0, fmt.Errorf("version mismatch: remote %d.%d, expected %d.%d",
			remoteMajor, remoteMinor, wireProtoMajor, wireProtoMinor)
	}
	if len(remotePub) != ed25519.PublicKeySize {
		return nil, 0, fmt.Errorf("missing or invalid remote public key")
	}

	// Verify signature: blake2b-512(password, remotePub), then ed25519 verify
	verifier, err := blake2b.New512(password)
	if err != nil {
		return nil, 0, fmt.Errorf("blake2b verify init: %w", err)
	}
	if _, err := verifier.Write(remotePub); err != nil {
		return nil, 0, fmt.Errorf("blake2b verify write: %w", err)
	}
	remoteHash := verifier.Sum(nil)
	if !ed25519.Verify(remotePub, remoteHash, remoteSig) {
		return nil, 0, fmt.Errorf("handshake signature verification failed")
	}

	// Clear deadline
	_ = conn.SetDeadline(time.Time{})

	// Use the higher priority of the two
	if remotePrio > priority {
		priority = remotePrio
	}
	return remotePub, priority, nil
}
