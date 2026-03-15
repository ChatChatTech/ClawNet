package matrix

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	maxResponseBody = 1 << 20 // 1 MB
	httpTimeout     = 15 * time.Second
)

// Client is a minimal Matrix Client-Server API client.
// It only implements the endpoints needed for peer discovery.
type Client struct {
	homeserver  string // e.g. "https://matrix.org"
	accessToken string
	userID      string
	httpClient  *http.Client
	mu          sync.Mutex
}

// NewClient creates a Client for the given homeserver URL.
func NewClient(homeserver string) *Client {
	return &Client{
		homeserver: strings.TrimRight(homeserver, "/"),
		httpClient: &http.Client{Timeout: httpTimeout},
	}
}

// registerRequest is the JSON body for /_matrix/client/v3/register.
type registerRequest struct {
	Username string      `json:"username"`
	Password string      `json:"password"`
	Auth     *authData   `json:"auth,omitempty"`
}

type authData struct {
	Type string `json:"type"`
}

// loginRequest is the JSON body for /_matrix/client/v3/login.
type loginRequest struct {
	Type       string           `json:"type"`
	Identifier loginIdentifier  `json:"identifier"`
	Password   string           `json:"password"`
}

type loginIdentifier struct {
	Type string `json:"type"`
	User string `json:"user"`
}

// authResponse represents the login/register response.
type authResponse struct {
	UserID      string `json:"user_id"`
	AccessToken string `json:"access_token"`
	// Error fields
	ErrCode string `json:"errcode,omitempty"`
	Error   string `json:"error,omitempty"`
}

// Login authenticates with the homeserver using password login.
func (c *Client) Login(ctx context.Context, username, password string) error {
	body := loginRequest{
		Type: "m.login.password",
		Identifier: loginIdentifier{
			Type: "m.id.user",
			User: username,
		},
		Password: password,
	}
	resp, err := c.doJSON(ctx, "POST", "/_matrix/client/v3/login", body)
	if err != nil {
		return fmt.Errorf("login request: %w", err)
	}
	var ar authResponse
	if err := json.Unmarshal(resp, &ar); err != nil {
		return fmt.Errorf("login decode: %w", err)
	}
	if ar.ErrCode != "" {
		return fmt.Errorf("login failed: %s: %s", ar.ErrCode, ar.Error)
	}
	c.mu.Lock()
	c.accessToken = ar.AccessToken
	c.userID = ar.UserID
	c.mu.Unlock()
	return nil
}

// Register creates a new account on the homeserver.
// Falls back to login if the username is already taken.
// Supports m.login.dummy (preferred) and m.login.terms (auto-accept) auth flows.
func (c *Client) Register(ctx context.Context, username, password string) error {
	body := registerRequest{
		Username: username,
		Password: password,
		Auth:     &authData{Type: "m.login.dummy"},
	}
	resp, err := c.doJSON(ctx, "POST", "/_matrix/client/v3/register", body)
	if err != nil {
		return fmt.Errorf("register request: %w", err)
	}
	var ar authResponse
	if err := json.Unmarshal(resp, &ar); err != nil {
		return fmt.Errorf("register decode: %w", err)
	}
	if ar.ErrCode == "M_USER_IN_USE" {
		// Already registered — try login instead
		return c.Login(ctx, username, password)
	}
	// If dummy auth was rejected, check if terms acceptance is required
	if ar.ErrCode == "M_UNAUTHORIZED" || ar.ErrCode == "M_FORBIDDEN" {
		if session, flows := parseInteractiveAuth(resp); len(flows) > 0 {
			for _, flow := range flows {
				if containsStage(flow, "m.login.terms") {
					return c.registerWithTerms(ctx, username, password, session)
				}
			}
		}
	}
	if ar.ErrCode != "" {
		return fmt.Errorf("register failed: %s: %s", ar.ErrCode, ar.Error)
	}
	c.mu.Lock()
	c.accessToken = ar.AccessToken
	c.userID = ar.UserID
	c.mu.Unlock()
	return nil
}

// registerWithTerms re-attempts registration with m.login.terms auth.
func (c *Client) registerWithTerms(ctx context.Context, username, password, session string) error {
	body := registerRequestTerms{
		Username: username,
		Password: password,
		Auth: authDataTerms{
			Type:    "m.login.terms",
			Session: session,
		},
	}
	resp, err := c.doJSON(ctx, "POST", "/_matrix/client/v3/register", body)
	if err != nil {
		return fmt.Errorf("register (terms): %w", err)
	}
	var ar authResponse
	if err := json.Unmarshal(resp, &ar); err != nil {
		return fmt.Errorf("register (terms) decode: %w", err)
	}
	if ar.ErrCode == "M_USER_IN_USE" {
		return c.Login(ctx, username, password)
	}
	if ar.ErrCode != "" {
		return fmt.Errorf("register (terms) failed: %s: %s", ar.ErrCode, ar.Error)
	}
	c.mu.Lock()
	c.accessToken = ar.AccessToken
	c.userID = ar.UserID
	c.mu.Unlock()
	return nil
}

// registerRequestTerms includes session for interactive auth.
type registerRequestTerms struct {
	Username string         `json:"username"`
	Password string         `json:"password"`
	Auth     authDataTerms  `json:"auth,omitempty"`
}

type authDataTerms struct {
	Type    string `json:"type"`
	Session string `json:"session,omitempty"`
}

// parseInteractiveAuth extracts session and flows from a 401 interactive auth response.
func parseInteractiveAuth(data []byte) (session string, flows [][]string) {
	var resp struct {
		Session string `json:"session"`
		Flows   []struct {
			Stages []string `json:"stages"`
		} `json:"flows"`
	}
	if json.Unmarshal(data, &resp) != nil {
		return "", nil
	}
	for _, f := range resp.Flows {
		flows = append(flows, f.Stages)
	}
	return resp.Session, flows
}

func containsStage(stages []string, target string) bool {
	for _, s := range stages {
		if s == target {
			return true
		}
	}
	return false
}

// CheckHealth probes the homeserver's /_matrix/client/versions endpoint.
// Returns the response latency or an error if unreachable.
func (c *Client) CheckHealth(ctx context.Context) (time.Duration, error) {
	start := time.Now()
	url := c.homeserver + "/_matrix/client/versions"
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return 0, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode != 200 {
		return 0, fmt.Errorf("status %d", resp.StatusCode)
	}
	return time.Since(start), nil
}

// joinResponse represents the room join response.
type joinResponse struct {
	RoomID  string `json:"room_id"`
	ErrCode string `json:"errcode,omitempty"`
	Error   string `json:"error,omitempty"`
}

// JoinRoom joins a room by alias or ID.
func (c *Client) JoinRoom(ctx context.Context, roomAliasOrID string) (string, error) {
	// URL-encode the room alias: # → %23, : → %3A
	encoded := strings.NewReplacer(
		"#", "%23",
		":", "%3A",
	).Replace(roomAliasOrID)
	path := "/_matrix/client/v3/join/" + encoded
	resp, err := c.doJSON(ctx, "POST", path, struct{}{})
	if err != nil {
		return "", fmt.Errorf("join room: %w", err)
	}
	var jr joinResponse
	if err := json.Unmarshal(resp, &jr); err != nil {
		return "", fmt.Errorf("join decode: %w", err)
	}
	if jr.ErrCode != "" {
		return "", fmt.Errorf("join failed: %s: %s", jr.ErrCode, jr.Error)
	}
	return jr.RoomID, nil
}

// SendMessage sends a text message to a room.
func (c *Client) SendMessage(ctx context.Context, roomID string, body string) error {
	txnID := hex.EncodeToString(sha256Hash([]byte(fmt.Sprintf("%s-%d", body, time.Now().UnixNano())))[:8])
	path := fmt.Sprintf("/_matrix/client/v3/rooms/%s/send/m.room.message/%s",
		strings.NewReplacer("!", "%21", ":", "%3A").Replace(roomID),
		txnID)
	msg := map[string]string{
		"msgtype": "m.text",
		"body":    body,
	}
	resp, err := c.doJSON(ctx, "PUT", path, msg)
	if err != nil {
		return fmt.Errorf("send message: %w", err)
	}
	// Check for error response
	var errResp struct {
		ErrCode string `json:"errcode,omitempty"`
		Error   string `json:"error,omitempty"`
	}
	if json.Unmarshal(resp, &errResp) == nil && errResp.ErrCode != "" {
		return fmt.Errorf("send failed: %s: %s", errResp.ErrCode, errResp.Error)
	}
	return nil
}

// SyncResponse represents the /sync response (minimal fields).
type SyncResponse struct {
	NextBatch string `json:"next_batch"`
	Rooms     struct {
		Join map[string]JoinedRoom `json:"join"`
	} `json:"rooms"`
}

// JoinedRoom represents a joined room in the sync response.
type JoinedRoom struct {
	Timeline struct {
		Events []RoomEvent `json:"events"`
	} `json:"timeline"`
}

// RoomEvent represents a room event.
type RoomEvent struct {
	Type    string          `json:"type"`
	Sender  string          `json:"sender"`
	Content json.RawMessage `json:"content"`
}

// Sync performs a /sync request. Pass since="" for initial sync.
func (c *Client) Sync(ctx context.Context, since string, timeoutMs int) (*SyncResponse, error) {
	path := fmt.Sprintf("/_matrix/client/v3/sync?timeout=%d", timeoutMs)
	if since != "" {
		path += "&since=" + since
	}
	// Only get timeline events from joined rooms, skip others
	path += "&filter=" + `{"room":{"timeline":{"limit":50},"state":{"lazy_load_members":true},"ephemeral":{"types":[]}},"presence":{"types":[]}}`
	resp, err := c.doAuthed(ctx, "GET", path, nil)
	if err != nil {
		return nil, fmt.Errorf("sync: %w", err)
	}
	var sr SyncResponse
	if err := json.Unmarshal(resp, &sr); err != nil {
		return nil, fmt.Errorf("sync decode: %w", err)
	}
	return &sr, nil
}

// LoggedIn returns true if the client has an access token.
func (c *Client) LoggedIn() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.accessToken != ""
}

// SetToken sets the access token directly (for loading from cache).
func (c *Client) SetToken(token, userID string) {
	c.mu.Lock()
	c.accessToken = token
	c.userID = userID
	c.mu.Unlock()
}

// Token returns the current access token and user ID.
func (c *Client) Token() (string, string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.accessToken, c.userID
}

// Homeserver returns the homeserver URL.
func (c *Client) Homeserver() string {
	return c.homeserver
}

// doJSON performs an authenticated JSON request.
func (c *Client) doJSON(ctx context.Context, method, path string, body interface{}) ([]byte, error) {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		bodyReader = bytes.NewReader(data)
	}
	return c.doAuthed(ctx, method, path, bodyReader)
}

// doAuthed performs an authenticated HTTP request.
func (c *Client) doAuthed(ctx context.Context, method, path string, body io.Reader) ([]byte, error) {
	url := c.homeserver + path
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	c.mu.Lock()
	token := c.accessToken
	c.mu.Unlock()
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody))
	if err != nil {
		return nil, err
	}
	return data, nil
}

func sha256Hash(data []byte) []byte {
	h := sha256.Sum256(data)
	return h[:]
}
