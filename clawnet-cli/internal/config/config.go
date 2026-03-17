package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

const (
	DefaultP2PPort    = 4001
	DefaultAPIPort    = 3998
	DefaultWGPort     = 51820
	DefaultBTDHTPort  = 6881
	DefaultMaxConns   = 200
	DefaultDataDir    = ".openclaw/clawnet"
	DefaultVisibility = "public"
)

// Config represents the node configuration stored in config.json.
type Config struct {
	ListenAddrs    []string        `json:"listen_addrs"`
	AnnounceAddrs  []string        `json:"announce_addrs,omitempty"`
	BootstrapPeers []string        `json:"bootstrap_peers"`
	Visibility     string          `json:"visibility"`
	GeoFuzzy       bool            `json:"geo_fuzzy"`
	MaxConnections int             `json:"max_connections"`
	RelayEnabled   bool            `json:"relay_enabled"`
	ForcePrivate   bool            `json:"force_private"`
	WebUIPort      int             `json:"web_ui_port"`
	TopicsAutoJoin []string        `json:"topics_auto_join"`
	WireGuard      WireGuardConfig `json:"wireguard"`
	BTDHT          BTDHTConfig     `json:"bt_dht"`
	HTTPBootstrap  bool            `json:"http_bootstrap"`
	Overlay        OverlayConfig  `json:"overlay"`
	DevLayers      []string       `json:"-"` // runtime-only: dev mode layer whitelist
}

// LayerEnabled returns true if the named layer should start.
// When DevLayers is empty (normal mode), all layers are enabled.
func (c *Config) LayerEnabled(name string) bool {
	if len(c.DevLayers) == 0 {
		return true
	}
	for _, l := range c.DevLayers {
		if l == name {
			return true
		}
	}
	return false
}

type WireGuardConfig struct {
	Enabled    bool `json:"enabled"`
	ListenPort int  `json:"listen_port"`
	AutoAccept bool `json:"auto_accept"`
}

// BTDHTConfig controls BitTorrent Mainline DHT discovery.
type BTDHTConfig struct {
	Enabled    bool `json:"enabled"`
	ListenPort int  `json:"listen_port"`
}

// OverlayConfig controls the Ironwood overlay transport.
type OverlayConfig struct {
	Enabled        bool     `json:"enabled"`
	ListenPort     int      `json:"listen_port,omitempty"`
	StaticPeers    []string `json:"static_peers,omitempty"`
	BootstrapPeers []string `json:"bootstrap_peers,omitempty"`
	Molted         bool     `json:"molted,omitempty"` // true = full mesh interop, false = ClawNet-only
}

// Profile represents the public node profile broadcast to the network.
type Profile struct {
	AgentName    string   `json:"agent_name"`
	Visibility   string   `json:"visibility"`
	Domains      []string `json:"domains"`
	Capabilities []string `json:"capabilities"`
	Bio          string   `json:"bio"`
	Motto        string   `json:"motto,omitempty"`
	Role         string   `json:"role,omitempty"`
	GeoCity      string   `json:"geo_city,omitempty"`
	GeoLatFuzzy  float64  `json:"geo_lat_fuzzy,omitempty"`
	GeoLonFuzzy  float64  `json:"geo_lon_fuzzy,omitempty"`
	Version      string   `json:"version"`
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		ListenAddrs: []string{
			"/ip4/0.0.0.0/tcp/4001",
			"/ip4/0.0.0.0/udp/4001/quic-v1",
			"/ip4/0.0.0.0/tcp/4002/ws",
		},
		BootstrapPeers: []string{
			"/ip4/210.45.71.67/tcp/4001/p2p/12D3KooWL2PeeDZChvnoERrfNkZa6JENyDiNWnbPwaNxNjETpmYh",
		},
		Visibility:     DefaultVisibility,
		GeoFuzzy:       true,
		MaxConnections: DefaultMaxConns,
		RelayEnabled:   true,
		WebUIPort:      DefaultAPIPort,
		TopicsAutoJoin: []string{
			"/clawnet/global",
			"/clawnet/lobby",
		},
		WireGuard: WireGuardConfig{
			Enabled:    false,
			ListenPort: DefaultWGPort,
			AutoAccept: false,
		},
		BTDHT: BTDHTConfig{
			Enabled:    true,
			ListenPort: DefaultBTDHTPort,
		},
		HTTPBootstrap: true,
		Overlay: OverlayConfig{
			Enabled: false,
		},
	}
}

// DataDir returns the absolute path to the data directory.
func DataDir() string {
	if d := os.Getenv("CLAWNET_DATA_DIR"); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, DefaultDataDir)
}

// ConfigPath returns the path to config.json.
func ConfigPath() string {
	return filepath.Join(DataDir(), "config.json")
}

// Load reads config from disk. Returns default config if file doesn't exist.
// Environment variables override config.json values:
//
//	CLAWNET_ANNOUNCE_ADDRS  - comma-separated multiaddrs to advertise
//	CLAWNET_BOOTSTRAP_PEERS - comma-separated bootstrap peer multiaddrs
//	CLAWNET_FORCE_PRIVATE   - set to "1" or "true" to force private reachability
func Load() (*Config, error) {
	path := ConfigPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			cfg := DefaultConfig()
			cfg.applyEnvOverrides()
			return cfg, nil
		}
		return nil, err
	}
	cfg := DefaultConfig()
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	// Migrate old "pinecone" config key to "overlay".
	cfg.migratePineconeKey(data)
	// Migrate old default port to new default.
	if cfg.WebUIPort == 3847 {
		cfg.WebUIPort = DefaultAPIPort
	}
	// Add WebSocket listen address if missing (upgrade from older config).
	cfg.migrateWSAddr()
	cfg.applyEnvOverrides()
	return cfg, nil
}

// migrateWSAddr ensures a /ws listen address is present for existing configs.
func (c *Config) migrateWSAddr() {
	for _, addr := range c.ListenAddrs {
		if strings.HasSuffix(addr, "/ws") {
			return
		}
	}
	c.ListenAddrs = append(c.ListenAddrs, "/ip4/0.0.0.0/tcp/4002/ws")
}

// migratePineconeKey reads old "pinecone" config key and applies it to Overlay.
func (c *Config) migratePineconeKey(data []byte) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return
	}
	if _, ok := raw["pinecone"]; !ok {
		return
	}
	// Old config has "pinecone" key — parse it into OverlayConfig
	var old OverlayConfig
	if err := json.Unmarshal(raw["pinecone"], &old); err != nil {
		return
	}
	if old.Enabled {
		c.Overlay = old
	}
}

// applyEnvOverrides applies environment variable overrides to config fields.
func (c *Config) applyEnvOverrides() {
	if v := os.Getenv("CLAWNET_ANNOUNCE_ADDRS"); v != "" {
		c.AnnounceAddrs = splitComma(v)
	}
	if v := os.Getenv("CLAWNET_BOOTSTRAP_PEERS"); v != "" {
		c.BootstrapPeers = splitComma(v)
	}
	if v := os.Getenv("CLAWNET_FORCE_PRIVATE"); v == "1" || strings.EqualFold(v, "true") {
		c.ForcePrivate = true
	}
	if v := os.Getenv("CLAWNET_OVERLAY_ENABLED"); v == "1" || strings.EqualFold(v, "true") {
		c.Overlay.Enabled = true
	}
	if v := os.Getenv("CLAWNET_OVERLAY_BOOTSTRAP"); v != "" {
		c.Overlay.BootstrapPeers = splitComma(v)
	}
}

func splitComma(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// Save writes config to disk.
func (c *Config) Save() error {
	path := ConfigPath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}
