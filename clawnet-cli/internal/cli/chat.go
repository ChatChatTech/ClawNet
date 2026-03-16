package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/ChatChatTech/ClawNet/clawnet-cli/internal/config"
)

// cmdChat dispatches: async mail-list by default, --interactive for real-time.
//
//	clawnet chat                         — inbox (list conversations)
//	clawnet chat <peer_id>               — read thread with a peer
//	clawnet chat <peer_id> <message...>  — send a message
//	clawnet chat --interactive / -i      — legacy real-time random chat
func cmdChat() error {
	args := os.Args[2:] // after "clawnet chat"

	// Check for --interactive / -i
	for i, a := range args {
		if a == "--interactive" || a == "-i" {
			// Remove the flag from args
			args = append(args[:i], args[i+1:]...)
			_ = args
			return cmdChatInteractive()
		}
	}

	// Async mail-list mode
	cfg, _ := config.Load()
	base := fmt.Sprintf("http://127.0.0.1:%d", cfg.WebUIPort)

	switch len(args) {
	case 0:
		return chatInbox(base)
	case 1:
		return chatThread(base, args[0])
	default:
		peerID := args[0]
		body := strings.Join(args[1:], " ")
		return chatSend(base, peerID, body)
	}
}

// ── Async mail-list commands ────────────────────────────────────────

func chatInbox(base string) error {
	red := "\033[38;2;230;57;70m"
	coral := "\033[38;2;247;127;0m"
	tidal := "\033[38;2;69;123;157m"
	dim := "\033[2m"
	rst := "\033[0m"
	cyan := "\033[96m"
	green := "\033[92m"

	resp, err := http.Get(base + "/api/dm/inbox")
	if err != nil {
		return fmt.Errorf("daemon not running? %w", err)
	}
	defer resp.Body.Close()

	var msgs []struct {
		PeerID    string `json:"peer_id"`
		Direction string `json:"direction"`
		Body      string `json:"body"`
		CreatedAt string `json:"created_at"`
		Read      bool   `json:"read"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&msgs); err != nil {
		return err
	}

	fmt.Println(red + "  ClawNet Mail" + rst)
	fmt.Println()

	if len(msgs) == 0 {
		fmt.Println(dim + "  No conversations yet." + rst)
		fmt.Println(dim + "  Send a message: clawnet chat <peer_id> <message>" + rst)
		fmt.Println(dim + "  Random chat:    clawnet chat --interactive" + rst)
		return nil
	}

	for _, m := range msgs {
		short := m.PeerID
		if len(short) > 16 {
			short = short[:16] + "..."
		}
		unread := ""
		if !m.Read && m.Direction == "received" {
			unread = green + " [new]" + rst
		}
		// Truncate body preview
		preview := m.Body
		if len(preview) > 60 {
			preview = preview[:60] + "..."
		}
		arrow := coral + "→" + rst // sent
		if m.Direction == "received" {
			arrow = tidal + "←" + rst // received
		}
		ts := formatTimeShort(m.CreatedAt)
		fmt.Printf("  %s %s%s%s %s %s%s%s%s\n",
			arrow, cyan, short, rst, dim+ts+rst, rst, preview, rst, unread)
	}

	fmt.Println()
	fmt.Println(dim + "  View thread: clawnet chat <peer_id>" + rst)
	fmt.Println(dim + "  Send reply:  clawnet chat <peer_id> <message>" + rst)
	return nil
}

func chatThread(base, peerID string) error {
	red := "\033[38;2;230;57;70m"
	coral := "\033[38;2;247;127;0m"
	tidal := "\033[38;2;69;123;157m"
	dim := "\033[2m"
	rst := "\033[0m"

	resp, err := http.Get(fmt.Sprintf("%s/api/dm/thread/%s?limit=30", base, peerID))
	if err != nil {
		return fmt.Errorf("daemon not running? %w", err)
	}
	defer resp.Body.Close()

	var msgs []struct {
		Direction string `json:"direction"`
		Body      string `json:"body"`
		CreatedAt string `json:"created_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&msgs); err != nil {
		return err
	}

	short := peerID
	if len(short) > 16 {
		short = short[:16] + "..."
	}
	fmt.Printf("%s  Thread with %s%s\n\n", red, short, rst)

	if len(msgs) == 0 {
		fmt.Println(dim + "  No messages yet. Send one:" + rst)
		fmt.Printf(dim+"  clawnet chat %s \"hello!\"\n"+rst, peerID)
		return nil
	}

	// Show oldest-first so it reads like a thread
	for i := len(msgs) - 1; i >= 0; i-- {
		m := msgs[i]
		ts := formatTimeShort(m.CreatedAt)
		if m.Direction == "sent" {
			fmt.Printf("  %s you %s %s\n", dim+ts+rst, coral+"→"+rst, m.Body)
		} else {
			fmt.Printf("  %s peer %s %s\n", dim+ts+rst, tidal+"←"+rst, m.Body)
		}
	}

	fmt.Println()
	fmt.Printf(dim+"  Reply: clawnet chat %s <message>\n"+rst, peerID)
	return nil
}

func chatSend(base, peerID, body string) error {
	green := "\033[92m"
	red := "\033[91m"
	rst := "\033[0m"
	dim := "\033[2m"

	if err := sendChatMsg(base, peerID, body); err != nil {
		fmt.Fprintf(os.Stderr, "%s✗ Send error: %v%s\n", red, err, rst)
		return err
	}

	short := peerID
	if len(short) > 16 {
		short = short[:16] + "..."
	}
	fmt.Printf("%s✓%s Sent to %s\n", green, rst, short)
	fmt.Printf("%s  View thread: clawnet chat %s%s\n", dim, peerID, rst)
	return nil
}

func formatTimeShort(ts string) string {
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		// Try without timezone
		t, err = time.Parse("2006-01-02T15:04:05", ts)
		if err != nil {
			return ts
		}
	}
	now := time.Now()
	diff := now.Sub(t)

	switch {
	case diff < time.Minute:
		return "now"
	case diff < time.Hour:
		return fmt.Sprintf("%dm ago", int(diff.Minutes()))
	case diff < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(diff.Hours()))
	default:
		return t.Format("Jan 02")
	}
}

// ── Interactive chat (legacy) ────────────────────────────────────────

func cmdChatInteractive() error {
	red := "\033[38;2;230;57;70m"
	coral := "\033[38;2;247;127;0m"
	tidal := "\033[38;2;69;123;157m"
	dim := "\033[2m"
	rst := "\033[0m"

	cfg, _ := config.Load()
	base := fmt.Sprintf("http://127.0.0.1:%d", cfg.WebUIPort)

	fmt.Println(red + "🦞 ClawNet Chat" + rst)
	fmt.Println(dim + "Random peer matching — Ctrl+C to quit" + rst)
	fmt.Println()

	// Match with a random peer
	fmt.Println(tidal + "Looking for someone to chat with..." + rst)
	resp, err := http.Get(base + "/api/chat/match")
	if err != nil {
		return fmt.Errorf("daemon not running? %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("match failed: %s", string(body))
	}

	var match struct {
		PeerID string `json:"peer_id"`
		Name   string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&match); err != nil {
		return fmt.Errorf("decode match: %w", err)
	}

	fmt.Printf(coral+"Matched with: %s"+rst+" %s(%s)%s\n", match.Name, dim, match.PeerID[:16], rst)
	fmt.Println(dim + "Type a message and press Enter to send. Ctrl+C to exit." + rst)
	fmt.Println()

	// Track last known message count so we only print new ones
	lastCount := 0

	// Signal handling
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Start polling for incoming messages in background
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(1500 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				msgs := fetchThread(base, match.PeerID, 20)
				if len(msgs) > lastCount {
					for _, m := range msgs[lastCount:] {
						if m.Direction == "received" {
							fmt.Printf("\r%s%s%s: %s\n> ", tidal, match.Name, rst, m.Body)
						}
					}
					lastCount = len(msgs)
				}
			}
		}
	}()

	// Interactive input loop
	scanner := bufio.NewScanner(os.Stdin)
	fmt.Print("> ")
	for {
		select {
		case <-sigCh:
			close(done)
			fmt.Println("\n" + dim + "Chat ended." + rst)
			return nil
		default:
		}

		if !scanner.Scan() {
			break
		}
		text := strings.TrimSpace(scanner.Text())
		if text == "" {
			fmt.Print("> ")
			continue
		}

		if err := sendChatMsg(base, match.PeerID, text); err != nil {
			fmt.Printf(red+"Send error: %v"+rst+"\n", err)
		}
		lastCount++
		fmt.Print("> ")
	}

	close(done)
	fmt.Println("\n" + dim + "Chat ended." + rst)
	return nil
}

type chatMsg struct {
	Direction string `json:"direction"`
	Body      string `json:"body"`
}

func fetchThread(base, peerID string, limit int) []chatMsg {
	resp, err := http.Get(fmt.Sprintf("%s/api/dm/thread/%s?limit=%d", base, peerID, limit))
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	var msgs []chatMsg
	json.NewDecoder(resp.Body).Decode(&msgs)
	return msgs
}

func sendChatMsg(base, peerID, body string) error {
	payload := fmt.Sprintf(`{"peer_id":%q,"body":%q}`, peerID, body)
	resp, err := http.Post(base+"/api/dm/send", "application/json", strings.NewReader(payload))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%s", string(b))
	}
	return nil
}
