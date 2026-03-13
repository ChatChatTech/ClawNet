package config

import (
	"encoding/json"
	"os"
	"path/filepath"
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
	BootstrapPeers []string        `json:"bootstrap_peers"`
	Visibility     string          `json:"visibility"`
	GeoFuzzy       bool            `json:"geo_fuzzy"`
	MaxConnections int             `json:"max_connections"`
	RelayEnabled   bool            `json:"relay_enabled"`
	WebUIPort      int             `json:"web_ui_port"`
	TopicsAutoJoin []string        `json:"topics_auto_join"`
	WireGuard      WireGuardConfig `json:"wireguard"`
	BTDHT          BTDHTConfig     `json:"bt_dht"`
	HTTPBootstrap  bool            `json:"http_bootstrap"`
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

// Profile represents the public node profile broadcast to the network.
type Profile struct {
	AgentName    string   `json:"agent_name"`
	Visibility   string   `json:"visibility"`
	Domains      []string `json:"domains"`
	Capabilities []string `json:"capabilities"`
	Bio          string   `json:"bio"`
	Motto        string   `json:"motto,omitempty"`
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
		},
		BootstrapPeers: []string{},
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
func Load() (*Config, error) {
	path := ConfigPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultConfig(), nil
		}
		return nil, err
	}
	cfg := DefaultConfig()
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	// Migrate old default port to new default.
	if cfg.WebUIPort == 3847 {
		cfg.WebUIPort = DefaultAPIPort
	}
	return cfg, nil
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
