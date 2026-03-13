package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	// PrimaryBootstrapURL is the main bootstrap endpoint (GitHub Pages at chatchat.space).
	PrimaryBootstrapURL = "https://chatchat.space/bootstrap.json"
	// FallbackBootstrapURL is the raw GitHub fallback.
	FallbackBootstrapURL = "https://raw.githubusercontent.com/ChatChatTech/ClawNet/main/bootstrap.json"

	fetchTimeout = 10 * time.Second
	maxBodySize  = 1 << 20 // 1 MB
)

// List is the bootstrap peer list fetched from HTTP.
type List struct {
	Version    int      `json:"version"`
	UpdatedAt  string   `json:"updated_at"`
	MinVersion string   `json:"min_cli_version,omitempty"`
	Nodes      []string `json:"nodes"`
}

// FetchPeers fetches the bootstrap peer list from HTTP endpoints,
// trying primary first then fallback.
func FetchPeers(ctx context.Context) (*List, error) {
	urls := []string{PrimaryBootstrapURL, FallbackBootstrapURL}

	var lastErr error
	for _, u := range urls {
		list, err := fetchFrom(ctx, u)
		if err != nil {
			lastErr = err
			fmt.Printf("bootstrap: fetch from %s failed: %v\n", u, err)
			continue
		}
		return list, nil
	}

	return nil, fmt.Errorf("all bootstrap URLs failed: %w", lastErr)
}

func fetchFrom(ctx context.Context, url string) (*List, error) {
	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "ClawNet/0.5.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodySize))
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	var list List
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, fmt.Errorf("decode json: %w", err)
	}

	return &list, nil
}
