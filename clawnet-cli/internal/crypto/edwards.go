package crypto

import (
	"crypto/sha512"
	"math/big"
)

func sha512Sum(data []byte) [64]byte {
	return sha512.Sum512(data)
}

// p is the field prime for Curve25519: 2^255 - 19
var fieldPrime = new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 255), big.NewInt(19))

// edwardsToMontgomery converts an Ed25519 public key (compressed Edwards point)
// to the corresponding Curve25519/X25519 public key (Montgomery u-coordinate).
//
// The birational map is: u = (1 + y) / (1 - y) mod p
func edwardsToMontgomery(edPub []byte) [32]byte {
	if len(edPub) != 32 {
		return [32]byte{}
	}
	// Ed25519 public key encoding: the y-coordinate in little-endian with
	// the high bit of the last byte being the sign of x.
	yBytes := make([]byte, 32)
	copy(yBytes, edPub)
	yBytes[31] &= 0x7f // clear sign bit

	// Decode y as little-endian integer
	y := new(big.Int).SetBytes(reverse32(yBytes))

	// u = (1 + y) * modInverse(1 - y, p)  mod p
	one := big.NewInt(1)
	num := new(big.Int).Add(one, y)           // 1 + y
	den := new(big.Int).Sub(one, y)            // 1 - y
	den.Mod(den, fieldPrime)                   // ensure positive
	denInv := new(big.Int).ModInverse(den, fieldPrime)
	if denInv == nil {
		return [32]byte{} // degenerate case: y == 1
	}
	u := num.Mul(num, denInv)
	u.Mod(u, fieldPrime)

	// Encode u as 32-byte little-endian
	uBytes := u.Bytes() // big-endian
	var result [32]byte
	for i, b := range uBytes {
		result[len(uBytes)-1-i] = b
	}
	return result
}

// reverse32 reverses a 32-byte slice (little-endian ↔ big-endian).
func reverse32(b []byte) []byte {
	out := make([]byte, len(b))
	for i := range b {
		out[len(b)-1-i] = b[i]
	}
	return out
}
