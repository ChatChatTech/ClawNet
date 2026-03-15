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

func cmdChat() error {
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
