package overlay

// ClawNet overlay IPv6 address derivation from Ed25519 public keys.
// Pure cryptographic function, no external dependencies.
//
// The 200::/7 address space is used by the overlay mesh network.
// The address encodes the public key so that any node can verify
// address ownership.

import (
	"crypto/ed25519"
	"fmt"
	"net"
)

// OverlayAddress derives the ClawNet 200::/7 IPv6 address from an
// Ed25519 public key. Returns the address as a 16-byte array.
// Algorithm: bitwise-invert the key, count leading ones → ones byte,
// then pack remaining bits after the first 0 into the address.
func OverlayAddress(publicKey ed25519.PublicKey) [16]byte {
	if len(publicKey) != ed25519.PublicKeySize {
		return [16]byte{}
	}

	var buf [ed25519.PublicKeySize]byte
	copy(buf[:], publicKey)
	for i := range buf {
		buf[i] = ^buf[i]
	}

	var addr [16]byte
	var temp []byte
	done := false
	ones := byte(0)
	bits := byte(0)
	nBits := 0

	for idx := 0; idx < 8*len(buf); idx++ {
		bit := (buf[idx/8] & (0x80 >> byte(idx%8))) >> byte(7-(idx%8))
		if !done && bit != 0 {
			ones++
			continue
		}
		if !done && bit == 0 {
			done = true
			continue
		}
		bits = (bits << 1) | bit
		nBits++
		if nBits == 8 {
			nBits = 0
			temp = append(temp, bits)
			bits = 0
		}
	}

	// Prefix: 0x02 (overlay address range 200::/7, node bit = 0)
	addr[0] = 0x02
	addr[1] = ones
	copy(addr[2:], temp)
	return addr
}

// OverlaySubnet derives the overlay /64 subnet prefix from an
// Ed25519 public key. Returns the first 8 bytes.
func OverlaySubnet(publicKey ed25519.PublicKey) [8]byte {
	addr := OverlayAddress(publicKey)
	var subnet [8]byte
	copy(subnet[:], addr[:8])
	// Set the subnet bit (last bit of prefix byte)
	subnet[0] |= 0x01 // 0x02 | 0x01 = 0x03
	return subnet
}

// FormatOverlayAddress formats the derived IPv6 address as a string.
func FormatOverlayAddress(publicKey ed25519.PublicKey) string {
	addr := OverlayAddress(publicKey)
	ip := net.IP(addr[:])
	return ip.String()
}

// FormatOverlaySubnet formats the derived /64 subnet prefix as a string.
func FormatOverlaySubnet(publicKey ed25519.PublicKey) string {
	subnet := OverlaySubnet(publicKey)
	ip := make(net.IP, 16)
	copy(ip[:8], subnet[:])
	return fmt.Sprintf("%s/64", ip.String())
}
