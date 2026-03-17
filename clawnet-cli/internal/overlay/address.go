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

// SubnetKeyTransform is the bloom filter key transform function matching
// yggdrasil-go's address.SubnetForKey(key).GetKey(). It derives the /64 subnet
// from an Ed25519 public key, then reverse-derives a partial key from the
// subnet bytes — exactly as yggdrasil-go's Subnet.GetKey() does.
// This MUST match the transform used by public Yggdrasil mesh peers.
func SubnetKeyTransform(key ed25519.PublicKey) ed25519.PublicKey {
	subnet := OverlaySubnet(key)
	// Treat the 8-byte subnet as a 16-byte Address (zero-padded) and
	// call the same reverse-derivation that yggdrasil uses in GetKey().
	var addr [16]byte
	copy(addr[:], subnet[:])
	return PartialKeyForAddr(addr)
}

// PartialKeyForAddr reverse-derives a partial Ed25519 public key from an
// overlay IPv6 address. Matches yggdrasil-go's Address.GetKey().
// The recovered key preserves enough bits for bloom-filter lookup via
// SubnetKeyTransform to work correctly.
func PartialKeyForAddr(addr [16]byte) ed25519.PublicKey {
	var key [ed25519.PublicKeySize]byte
	ones := int(addr[1])
	for idx := 0; idx < ones; idx++ {
		key[idx/8] |= 0x80 >> byte(idx%8)
	}
	keyOffset := ones + 1
	addrOffset := 8 + 8 // 8 bits prefix + 8 bits ones
	for idx := addrOffset; idx < 8*len(addr); idx++ {
		bits := addr[idx/8] & (0x80 >> byte(idx%8))
		bits <<= byte(idx % 8)
		keyIdx := keyOffset + (idx - addrOffset)
		bits >>= byte(keyIdx % 8)
		ki := keyIdx / 8
		if ki >= len(key) {
			break
		}
		key[ki] |= bits
	}
	for i := range key {
		key[i] = ^key[i]
	}
	return ed25519.PublicKey(key[:])
}
