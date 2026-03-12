package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"

	"github.com/ChatChatTech/letschat/letschat-cli/internal/config"
)

// StartAPI starts the HTTP API server for the daemon.
func (d *Daemon) StartAPI(ctx context.Context) *http.Server {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/status", d.handleStatus)
	mux.HandleFunc("GET /api/peers", d.handlePeers)
	mux.HandleFunc("GET /api/profile", d.handleGetProfile)
	mux.HandleFunc("PUT /api/profile", d.handleUpdateProfile)

	addr := fmt.Sprintf("127.0.0.1:%d", d.Config.WebUIPort)
	server := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		fmt.Printf("warning: could not start API server on %s: %v\n", addr, err)
		return server
	}

	go server.Serve(ln)
	return server
}

func (d *Daemon) handleStatus(w http.ResponseWriter, r *http.Request) {
	status := map[string]any{
		"peer_id":    d.Node.PeerID().String(),
		"version":    Version,
		"peers":      len(d.Node.ConnectedPeers()),
		"topics":     d.topicNames(),
		"addrs":      d.addrStrings(),
		"data_dir":   d.DataDir,
	}
	writeJSON(w, status)
}

func (d *Daemon) handlePeers(w http.ResponseWriter, r *http.Request) {
	peers := d.Node.ConnectedPeers()
	result := make([]map[string]string, 0, len(peers))
	for _, p := range peers {
		addrs := d.Node.Host.Peerstore().Addrs(p)
		addrStrs := make([]string, 0, len(addrs))
		for _, a := range addrs {
			addrStrs = append(addrStrs, a.String())
		}
		result = append(result, map[string]string{
			"peer_id": p.String(),
			"addrs":   fmt.Sprintf("%v", addrStrs),
		})
	}
	writeJSON(w, result)
}

func (d *Daemon) handleGetProfile(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, d.Profile)
}

func (d *Daemon) handleUpdateProfile(w http.ResponseWriter, r *http.Request) {
	var p config.Profile
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	d.Profile = &p
	writeJSON(w, map[string]string{"status": "updated"})
}

func (d *Daemon) topicNames() []string {
	names := make([]string, 0, len(d.Node.Topics))
	for name := range d.Node.Topics {
		names = append(names, name)
	}
	return names
}

func (d *Daemon) addrStrings() []string {
	addrs := d.Node.Addrs()
	strs := make([]string, 0, len(addrs))
	for _, a := range addrs {
		strs = append(strs, a.String())
	}
	return strs
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
