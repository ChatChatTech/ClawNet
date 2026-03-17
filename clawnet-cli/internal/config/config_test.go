package config

import "testing"

func TestLayerEnabled_NoDevLayers(t *testing.T) {
	cfg := &Config{}
	for _, layer := range []string{"stun", "mdns", "dht", "bt-dht", "bootstrap", "relay", "overlay", "k8s"} {
		if !cfg.LayerEnabled(layer) {
			t.Errorf("expected %q enabled when DevLayers is empty", layer)
		}
	}
}

func TestLayerEnabled_Whitelist(t *testing.T) {
	cfg := &Config{DevLayers: []string{"dht", "overlay"}}
	if !cfg.LayerEnabled("dht") {
		t.Error("dht should be enabled")
	}
	if !cfg.LayerEnabled("overlay") {
		t.Error("overlay should be enabled")
	}
	if cfg.LayerEnabled("mdns") {
		t.Error("mdns should be disabled")
	}
	if cfg.LayerEnabled("relay") {
		t.Error("relay should be disabled")
	}
}
