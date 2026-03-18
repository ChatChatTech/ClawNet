package cli

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/ChatChatTech/ClawNet/clawnet-cli/internal/config"
	"github.com/ChatChatTech/ClawNet/clawnet-cli/internal/i18n"
)

func cmdMolt() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
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
			return fmt.Errorf("%s", i18n.Tf("molt.failed", e))
		}
		return fmt.Errorf("%s", i18n.Tf("molt.failed", fmt.Sprintf("HTTP %d", resp.StatusCode)))
	}

	fmt.Println(i18n.T("molt.success"))
	return nil
}

func cmdUnmolt() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
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
			return fmt.Errorf("%s", i18n.Tf("unmolt.failed", e))
		}
		return fmt.Errorf("%s", i18n.Tf("unmolt.failed", fmt.Sprintf("HTTP %d", resp.StatusCode)))
	}

	fmt.Println(i18n.T("unmolt.success"))
	return nil
}
