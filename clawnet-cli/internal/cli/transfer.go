package cli

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ChatChatTech/ClawNet/clawnet-cli/internal/config"
	"github.com/ChatChatTech/ClawNet/clawnet-cli/internal/i18n"
	"github.com/ChatChatTech/ClawNet/clawnet-cli/internal/identity"
	"github.com/ChatChatTech/ClawNet/clawnet-cli/internal/store"
)

// ── Export file format ──────────────────────────────────────────────
//
// Layout: [ 32-byte HMAC-SHA256 ] [ gzip(JSON payload) ]
//
// The HMAC key is SHA256(identity.key raw bytes), so only the original
// key holder can produce a valid signature.  Changing any byte in the
// payload section invalidates the HMAC → prevents credit tampering.

const exportMagic = "CLAW" // first 4 bytes inside the JSON, sanity check

// Colors for transfer commands (reuse lobster theme)
const (
	cWarn = "\033[1;38;2;247;127;0m" // Bold Coral Orange — warnings
	cOK   = "\033[1;38;2;69;123;157m" // Bold Tidal Blue — success
)

type exportPayload struct {
	Magic      string  `json:"magic"`
	Version    int     `json:"version"`
	ExportedAt string  `json:"exported_at"`
	PeerID     string  `json:"peer_id"`
	IdentityKey []byte `json:"identity_key"` // raw Ed25519 private key
	Balance    int64   `json:"balance"`
	Frozen     int64   `json:"frozen"`
	TotalEarned int64  `json:"total_earned"`
	TotalSpent int64   `json:"total_spent"`
	Prestige   float64 `json:"prestige"`
	Reputation *store.ReputationRecord `json:"reputation,omitempty"`
}

// ── cmdExport ───────────────────────────────────────────────────────

func cmdExport() error {
	dataDir := config.DataDir()
	keyPath := filepath.Join(dataDir, "identity.key")

	// Load identity key
	priv, err := identity.LoadOrGenerate(dataDir)
	if err != nil {
		return fmt.Errorf("cannot load identity: %w", err)
	}
	peerID, err := identity.PeerIDFromKey(priv)
	if err != nil {
		return fmt.Errorf("cannot derive peer ID: %w", err)
	}
	keyRaw, err := priv.Raw()
	if err != nil {
		return fmt.Errorf("cannot marshal key: %w", err)
	}

	// Check identity.key exists (not just generated)
	if _, err := os.Stat(keyPath); os.IsNotExist(err) {
		return fmt.Errorf("%s", i18n.T("export.no_identity"))
	}

	// Build payload
	payload := exportPayload{
		Magic:       exportMagic,
		Version:     1,
		ExportedAt:  time.Now().UTC().Format(time.RFC3339),
		PeerID:      peerID.String(),
		IdentityKey: keyRaw,
	}

	// Try to read credit/reputation from DB
	db, dbErr := store.Open(dataDir)
	if dbErr == nil {
		defer db.Close()
		if acct, err := db.GetCreditBalance(peerID.String()); err == nil {
			payload.Balance = acct.Balance
			payload.Frozen = acct.Frozen
			payload.TotalEarned = acct.TotalEarned
			payload.TotalSpent = acct.TotalSpent
		}
		// Read prestige from credit_accounts
		var prestige float64
		row := db.DB.QueryRow(`SELECT COALESCE(prestige, 0) FROM credit_accounts WHERE peer_id = ?`, peerID.String())
		if row.Scan(&prestige) == nil {
			payload.Prestige = prestige
		}
		if rep, err := db.GetReputation(peerID.String()); err == nil {
			payload.Reputation = rep
		}
	}

	// Show summary
	fmt.Println()
	fmt.Println(cBanner + "  ┌─────────────────────────────────────────┐" + cReset)
	fmt.Println(cBanner + "  │       " + i18n.T("export.title") + "           │" + cReset)
	fmt.Println(cBanner + "  └─────────────────────────────────────────┘" + cReset)
	fmt.Println()
	fmt.Printf("  %-12s %s\n", i18n.T("export.field.peer_id"), payload.PeerID)
	fmt.Printf("  %-12s %d Shell\n", i18n.T("export.field.balance"), payload.Balance)
	fmt.Printf("  %-12s %d Shell\n", i18n.T("export.field.frozen"), payload.Frozen)
	fmt.Printf("  %-12s %.2f\n", i18n.T("export.field.prestige"), payload.Prestige)
	fmt.Println()

	// ── Double confirmation ──
	fmt.Println(cWarn + "  " + i18n.T("export.warning1") + cReset)
	fmt.Println(cWarn + "  " + i18n.T("export.warning2") + cReset)
	fmt.Println()

	reader := bufio.NewReader(os.Stdin)
	fmt.Print("  " + i18n.T("export.confirm"))
	ans1, _ := reader.ReadString('\n')
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(ans1)), "y") {
		fmt.Println("  " + i18n.T("common.cancelled"))
		return nil
	}

	fmt.Print("  " + i18n.T("export.confirm_peerid"))
	ans2, _ := reader.ReadString('\n')
	ans2 = strings.TrimSpace(ans2)
	prefix := peerID.String()
	if len(prefix) > 12 {
		prefix = prefix[:12]
	}
	if ans2 != prefix {
		fmt.Println("  " + i18n.T("export.mismatch"))
		return nil
	}

	// ── Determine output path ──
	outFile := fmt.Sprintf("clawnet-export-%s.claw", time.Now().Format("20060102-150405"))
	if len(os.Args) > 2 && os.Args[2] != "" {
		outFile = os.Args[2]
	}

	// ── Serialize payload ──
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	// ── Compress ──
	compressedData, err := compressGzip(jsonData)
	if err != nil {
		return fmt.Errorf("compress: %w", err)
	}

	// ── HMAC ──
	hmacKey := sha256.Sum256(keyRaw)
	mac := hmac.New(sha256.New, hmacKey[:])
	mac.Write(compressedData)
	signature := mac.Sum(nil) // 32 bytes

	// ── Write file: [32-byte HMAC] [gzip data] ──
	f, err := os.OpenFile(outFile, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("create export file: %w", err)
	}
	if _, err := f.Write(signature); err != nil {
		f.Close()
		os.Remove(outFile)
		return fmt.Errorf("write signature: %w", err)
	}
	if _, err := f.Write(compressedData); err != nil {
		f.Close()
		os.Remove(outFile)
		return fmt.Errorf("write data: %w", err)
	}
	f.Close()

	absPath, _ := filepath.Abs(outFile)
	fmt.Println()
	fmt.Println(cOK + "  " + i18n.T("export.saved") + absPath + cReset)
	fmt.Println()
	fmt.Println(cWarn + "  ╔═══════════════════════════════════════════════════════════╗" + cReset)
	fmt.Println(cWarn + "  ║  " + i18n.T("export.backup1") + "                          ║" + cReset)
	fmt.Println(cWarn + "  ║  " + i18n.T("export.backup2") + "   ║" + cReset)
	fmt.Println(cWarn + "  ║  " + i18n.T("export.backup3") + "         ║" + cReset)
	fmt.Println(cWarn + "  ╚═══════════════════════════════════════════════════════════╝" + cReset)
	fmt.Println()

	// ── Wipe local data ──
	fmt.Println("  " + i18n.T("export.wiping"))
	if err := nukeDataDir(dataDir, false); err != nil {
		return fmt.Errorf("wipe failed: %w", err)
	}
	fmt.Println(cOK + "  " + i18n.T("export.wiped") + cReset)
	fmt.Println()
	fmt.Println("  " + i18n.T("export.restore_hint"))
	fmt.Printf("    clawnet import %s\n", filepath.Base(outFile))
	fmt.Println()

	return nil
}

// ── cmdImport ───────────────────────────────────────────────────────

func cmdImport() error {
	if len(os.Args) < 3 {
		return fmt.Errorf("usage: clawnet import <file.claw>")
	}
	importPath := os.Args[2]

	dataDir := config.DataDir()
	keyPath := filepath.Join(dataDir, "identity.key")

	// Refuse if identity already exists
	if _, err := os.Stat(keyPath); err == nil {
		return fmt.Errorf("%s", i18n.Tf("import.already_exists", keyPath))
	}

	// Read file
	data, err := os.ReadFile(importPath)
	if err != nil {
		return fmt.Errorf("read export file: %w", err)
	}
	if len(data) < 33 {
		return fmt.Errorf("invalid export file (too small)")
	}

	signature := data[:32]
	compressedData := data[32:]

	// Decompress
	jsonData, err := decompressGzip(compressedData)
	if err != nil {
		return fmt.Errorf("decompress: %w", err)
	}

	// Parse payload
	var payload exportPayload
	if err := json.Unmarshal(jsonData, &payload); err != nil {
		return fmt.Errorf("parse payload: %w", err)
	}
	if payload.Magic != exportMagic {
		return fmt.Errorf("%s", i18n.T("import.invalid_file"))
	}

	// Verify HMAC
	hmacKey := sha256.Sum256(payload.IdentityKey)
	mac := hmac.New(sha256.New, hmacKey[:])
	mac.Write(compressedData)
	expected := mac.Sum(nil)
	if !hmac.Equal(signature, expected) {
		return fmt.Errorf("HMAC verification failed — file has been tampered with")
	}

	// Show what we're importing
	fmt.Println()
	fmt.Println(cBanner + "  ┌─────────────────────────────────────────┐" + cReset)
	fmt.Println(cBanner + "  │       " + i18n.T("import.title") + "           │" + cReset)
	fmt.Println(cBanner + "  └─────────────────────────────────────────┘" + cReset)
	fmt.Println()
	fmt.Printf("  %-14s %s\n", i18n.T("import.field.peer_id"), payload.PeerID)
	fmt.Printf("  %-14s %d Shell\n", i18n.T("import.field.balance"), payload.Balance)
	fmt.Printf("  %-14s %s\n", i18n.T("import.field.exported_at"), payload.ExportedAt)
	fmt.Println()

	reader := bufio.NewReader(os.Stdin)
	fmt.Print("  " + i18n.T("import.confirm"))
	ans, _ := reader.ReadString('\n')
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(ans)), "y") {
		fmt.Println("  " + i18n.T("common.cancelled"))
		return nil
	}

	// Create data dir and write identity key
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}
	if err := os.WriteFile(keyPath, payload.IdentityKey, 0600); err != nil {
		return fmt.Errorf("write identity key: %w", err)
	}

	// Write default config if missing
	cfgPath := config.ConfigPath()
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		cfg := config.DefaultConfig()
		cfg.Save()
	}

	// Seed credit account in DB
	db, err := store.Open(dataDir)
	if err == nil {
		defer db.Close()
		db.EnsureCreditAccount(payload.PeerID, payload.Balance)
		// Restore reputation if present
		if payload.Reputation != nil {
			db.UpsertReputation(payload.Reputation)
		}
		// Restore prestige
		if payload.Prestige > 0 {
			db.DB.Exec(`UPDATE credit_accounts SET prestige = ? WHERE peer_id = ?`,
				payload.Prestige, payload.PeerID)
		}
	}

	fmt.Println()
	fmt.Println(cOK + "  " + i18n.T("import.success") + cReset)
	fmt.Printf("  Peer ID: %s\n", payload.PeerID)
	fmt.Println()
	fmt.Println("  " + i18n.T("import.next_steps"))
	fmt.Println("    clawnet init    — finish directory setup")
	fmt.Println("    clawnet start   — start your node")
	fmt.Println()

	return nil
}

// ── cmdNuke ─────────────────────────────────────────────────────────

func cmdNuke() error {
	dataDir := config.DataDir()

	// Check if anything exists
	if _, err := os.Stat(dataDir); os.IsNotExist(err) {
		fmt.Println("  " + i18n.T("nuke.nothing"))
		return nil
	}

	// Try to stop daemon first
	pidPath := filepath.Join(dataDir, "daemon.pid")
	if pidData, err := os.ReadFile(pidPath); err == nil {
		if pid, err := strconv.Atoi(strings.TrimSpace(string(pidData))); err == nil {
			if proc, err := os.FindProcess(pid); err == nil {
				proc.Signal(os.Interrupt)
				fmt.Println("  " + i18n.T("nuke.stopped_daemon"))
				time.Sleep(500 * time.Millisecond)
			}
		}
	}

	// Show what will be removed
	keyPath := filepath.Join(dataDir, "identity.key")
	hasKey := false
	if _, err := os.Stat(keyPath); err == nil {
		hasKey = true
	}

	fmt.Println()
	fmt.Println(cWarn + "  ╔═══════════════════════════════════════════════════════════╗" + cReset)
	fmt.Println(cWarn + "  ║               " + i18n.T("nuke.title") + "                          ║" + cReset)
	fmt.Println(cWarn + "  ╚═══════════════════════════════════════════════════════════╝" + cReset)
	fmt.Println()
	fmt.Printf("  Data directory: %s\n", dataDir)
	if hasKey {
		fmt.Println("  Identity key:   " + cWarn + i18n.T("nuke.key_found") + cReset)
	}
	fmt.Println()

	reader := bufio.NewReader(os.Stdin)

	keepKey := false
	if hasKey {
		fmt.Print("  " + i18n.T("nuke.keep_key"))
		ans, _ := reader.ReadString('\n')
		keepKey = strings.HasPrefix(strings.ToLower(strings.TrimSpace(ans)), "y")
	}

	fmt.Println()
	if keepKey {
		fmt.Println(cWarn + "  " + i18n.T("nuke.warn_except_key") + cReset)
	} else {
		fmt.Println(cWarn + "  " + i18n.T("nuke.warn_permanent") + cReset)
		fmt.Println(cWarn + "  " + i18n.T("nuke.warn_export_first") + cReset)
	}
	fmt.Print("  " + i18n.T("nuke.confirm"))
	ans, _ := reader.ReadString('\n')
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(ans)), "y") {
		fmt.Println("  " + i18n.T("common.cancelled"))
		return nil
	}

	// If keeping key, back it up temporarily
	var keyBackup []byte
	if keepKey {
		keyBackup, _ = os.ReadFile(keyPath)
	}

	// Remove data directory
	if err := nukeDataDir(dataDir, false); err != nil {
		return fmt.Errorf("remove data directory: %w", err)
	}

	// Restore key if keeping
	if keepKey && keyBackup != nil {
		os.MkdirAll(dataDir, 0700)
		os.WriteFile(keyPath, keyBackup, 0600)
		fmt.Println(cOK + "  " + i18n.T("nuke.preserved") + cReset)
	} else {
		fmt.Println(cOK + "  " + i18n.T("nuke.removed") + cReset)
	}

	// Offer to remove binary
	binaryPath, _ := os.Executable()
	if binaryPath != "" {
		fmt.Println()
		fmt.Printf("  %s [y/N] ", i18n.Tf("nuke.remove_binary", binaryPath))
		ans, _ := reader.ReadString('\n')
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(ans)), "y") {
			if err := os.Remove(binaryPath); err != nil {
				fmt.Printf("  %s\n", i18n.Tf("nuke.binary_error", err))
				fmt.Printf("  %s\n", i18n.Tf("nuke.binary_remove_hint", binaryPath))
			} else {
				fmt.Println(cOK + "  " + i18n.T("nuke.binary_removed") + cReset)
			}
		}
	}

	fmt.Println()
	fmt.Println("  " + i18n.T("nuke.goodbye"))
	return nil
}

// ── Helpers ─────────────────────────────────────────────────────────

// nukeDataDir removes the data directory entirely.
func nukeDataDir(dataDir string, keepDir bool) error {
	if keepDir {
		// Remove contents but keep the directory
		entries, err := os.ReadDir(dataDir)
		if err != nil {
			return err
		}
		for _, e := range entries {
			os.RemoveAll(filepath.Join(dataDir, e.Name()))
		}
		return nil
	}
	return os.RemoveAll(dataDir)
}

func compressGzip(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	w, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if err != nil {
		return nil, err
	}
	if _, err := w.Write(data); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func decompressGzip(data []byte) ([]byte, error) {
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}
