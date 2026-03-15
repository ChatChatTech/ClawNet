package crypto

import (
	"crypto/rand"
	"database/sql"
	"os"
	"testing"

	libcrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	_ "github.com/mattn/go-sqlite3"
)

func TestEdwardsToCurve25519Roundtrip(t *testing.T) {
	// Generate an Ed25519 key pair
	priv, _, err := libcrypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	// Convert private key
	scalar, err := Ed25519PrivToCurve25519(priv)
	if err != nil {
		t.Fatal(err)
	}
	if scalar == [32]byte{} {
		t.Fatal("scalar is zero")
	}

	// Convert public key
	pubRaw, _ := priv.GetPublic().Raw()
	pubCurve, err := Ed25519PubToCurve25519(pubRaw)
	if err != nil {
		t.Fatal(err)
	}
	if pubCurve == [32]byte{} {
		t.Fatal("public key is zero")
	}
}

func TestEncryptDecrypt(t *testing.T) {
	// Create two key pairs (Alice and Bob)
	alicePriv, _, _ := libcrypto.GenerateEd25519Key(rand.Reader)
	bobPriv, _, _ := libcrypto.GenerateEd25519Key(rand.Reader)

	alicePeerID, _ := peer.IDFromPrivateKey(alicePriv)
	bobPeerID, _ := peer.IDFromPrivateKey(bobPriv)

	// Create temp SQLite databases
	aliceDB := openTestDB(t)
	defer aliceDB.Close()
	bobDB := openTestDB(t)
	defer bobDB.Close()

	aliceEngine, err := NewEngine(alicePriv, aliceDB)
	if err != nil {
		t.Fatal("alice engine:", err)
	}
	bobEngine, err := NewEngine(bobPriv, bobDB)
	if err != nil {
		t.Fatal("bob engine:", err)
	}

	// Alice encrypts a message for Bob
	plaintext := []byte(`{"id":"test","body":"hello world","created_at":"2025-01-01T00:00:00Z","sender_name":"alice"}`)
	ciphertext, err := aliceEngine.Encrypt(bobPeerID, plaintext)
	if err != nil {
		t.Fatal("encrypt:", err)
	}

	// Verify it's marked as encrypted
	if !IsEncrypted(ciphertext) {
		t.Fatal("ciphertext not detected as encrypted")
	}

	// Bob decrypts the message
	decrypted, err := bobEngine.Decrypt(alicePeerID, ciphertext)
	if err != nil {
		t.Fatal("decrypt:", err)
	}

	if string(decrypted) != string(plaintext) {
		t.Fatalf("decrypted != plaintext\ngot:  %s\nwant: %s", decrypted, plaintext)
	}

	// Verify session count
	if bobEngine.SessionCount() != 1 {
		t.Fatalf("expected 1 session, got %d", bobEngine.SessionCount())
	}
}

func TestIsEncryptedFalseForPlaintext(t *testing.T) {
	plain := []byte(`{"id":"test","body":"hello","created_at":"2025-01-01T00:00:00Z","sender_name":"alice"}`)
	if IsEncrypted(plain) {
		t.Fatal("plaintext detected as encrypted")
	}
}

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	f, err := os.CreateTemp("", "clawnet-crypto-test-*.db")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	t.Cleanup(func() { os.Remove(f.Name()) })

	db, err := sql.Open("sqlite3", f.Name())
	if err != nil {
		t.Fatal(err)
	}
	return db
}
