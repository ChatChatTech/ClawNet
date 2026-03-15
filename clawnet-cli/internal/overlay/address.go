package overlay

// Yggdrasil-compatible IPv6 address derivation from Ed25519 public keys.
// Ported from yggdrasil-go/src/address/address.go — pure cryptographic
// function, no external dependencies.
//
// The 200::/7 address space is a reserved ULA range used exclusively
// by the Yggdrasil mesh network. The address encodes the public key
// so that any node can verify address ownership.

import (
	"crypto/ed25519"
	"fmt"
	"net"
)

// YggdrasilAddress derives the Yggdrasil 200::/7 IPv6 address from an
// Ed25519 public key. Returns the address as a 16-byte array.
// Algorithm: bitwise-invert the key, count leading ones → ones byte,
// then pack remaining bits after the first 0 into the address.
func YggdrasilAddress(publicKey ed25519.PublicKey) [16]byte {
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

	// Prefix: 0x02 (Yggdrasil address range 200::/7, node bit = 0)
	addr[0] = 0x02
	addr[1] = ones
	copy(addr[2:], temp)
	return addr
}

// YggdrasilSubnet derives the Yggdrasil /64 subnet prefix from an
// Ed25519 public key. Returns the first 8 bytes.
func YggdrasilSubnet(publicKey ed25519.PublicKey) [8]byte {
	addr := YggdrasilAddress(publicKey)
	var subnet [8]byte
	copy(subnet[:], addr[:8])
	// Set the subnet bit (last bit of prefix byte)
	subnet[0] |= 0x01 // 0x02 | 0x01 = 0x03
	return subnet
}

// FormatYggdrasilAddress formats the derived IPv6 address as a string.
func FormatYggdrasilAddress(publicKey ed25519.PublicKey) string {
	addr := YggdrasilAddress(publicKey)
	ip := net.IP(addr[:])
	return ip.String()
}

// FormatYggdrasilSubnet formats the derived /64 subnet prefix as a string.
func FormatYggdrasilSubnet(publicKey ed25519.PublicKey) string {
	subnet := YggdrasilSubnet(publicKey)
	ip := make(net.IP, 16)
	copy(ip[:8], subnet[:])
	return fmt.Sprintf("%s/64", ip.String())
}
