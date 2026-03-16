package cli

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/ChatChatTech/ClawNet/clawnet-cli/internal/config"
)

func cmdMolt() error {
	cfg, _ := config.Load()
	base := fmt.Sprintf("http://127.0.0.1:%d", cfg.WebUIPort)
	resp, err := http.Post(base+"/api/overlay/molt", "application/json", nil)
	if err != nil {
		return fmt.Errorf("daemon not running or unreachable: %w", err)
	}
	defer resp.Body.Close()

	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)

	if resp.StatusCode != http.StatusOK {
		if e, ok := result["error"].(string); ok {
			return fmt.Errorf("molt failed: %s", e)
		}
		return fmt.Errorf("molt failed: HTTP %d", resp.StatusCode)
	}

	fmt.Println("\033[38;2;230;57;70m� Molted!\033[0m full overlay mesh interop enabled")
	return nil
}

func cmdUnmolt() error {
	cfg, _ := config.Load()
	base := fmt.Sprintf("http://127.0.0.1:%d", cfg.WebUIPort)
	resp, err := http.Post(base+"/api/overlay/unmolt", "application/json", nil)
	if err != nil {
		return fmt.Errorf("daemon not running or unreachable: %w", err)
	}
	defer resp.Body.Close()

	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)

	if resp.StatusCode != http.StatusOK {
		if e, ok := result["error"].(string); ok {
			return fmt.Errorf("unmolt failed: %s", e)
		}
		return fmt.Errorf("unmolt failed: HTTP %d", resp.StatusCode)
	}

	fmt.Println("\033[38;2;42;157;143m🦞 Unmolted!\033[0m ClawNet-only mode — external mesh peers blocked")
	return nil
}
