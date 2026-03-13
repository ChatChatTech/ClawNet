package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"sync"

	"github.com/ChatChatTech/ClawNet/clawnet-cli/internal/config"
	"github.com/ChatChatTech/ClawNet/clawnet-cli/internal/geo"
	"github.com/ChatChatTech/ClawNet/clawnet-cli/internal/store"
)

// StartAPI starts the HTTP API server for the daemon.
func (d *Daemon) StartAPI(ctx context.Context) *http.Server {
	mux := http.NewServeMux()

	// Phase 0 endpoints
	mux.HandleFunc("GET /api/status", d.handleStatus)
	mux.HandleFunc("GET /api/peers", d.handlePeers)
	mux.HandleFunc("GET /api/peers/geo", d.handlePeersGeo)
	mux.HandleFunc("GET /api/profile", d.handleGetProfile)
	mux.HandleFunc("PUT /api/profile", d.handleUpdateProfile)
	mux.HandleFunc("PUT /api/motto", d.handleSetMotto)
	mux.HandleFunc("GET /api/traffic", d.handleTraffic)

	// Phase 1 — Knowledge Mesh
	mux.HandleFunc("POST /api/knowledge", d.handlePostKnowledge)
	mux.HandleFunc("GET /api/knowledge/feed", d.handleKnowledgeFeed)
	mux.HandleFunc("GET /api/knowledge/search", d.handleKnowledgeSearch)
	mux.HandleFunc("POST /api/knowledge/{id}/react", d.handleKnowledgeReact)
	mux.HandleFunc("POST /api/knowledge/{id}/reply", d.handleKnowledgeReply)
	mux.HandleFunc("GET /api/knowledge/{id}/replies", d.handleKnowledgeReplies)

	// Phase 1 — Topic Rooms
	mux.HandleFunc("POST /api/topics", d.handleCreateTopic)
	mux.HandleFunc("GET /api/topics", d.handleListTopics)
	mux.HandleFunc("POST /api/topics/{name}/join", d.handleJoinTopic)
	mux.HandleFunc("POST /api/topics/{name}/leave", d.handleLeaveTopic)
	mux.HandleFunc("POST /api/topics/{name}/messages", d.handlePostTopicMessage)
	mux.HandleFunc("GET /api/topics/{name}/messages", d.handleGetTopicMessages)

	// Phase 1 — Direct Messages
	mux.HandleFunc("POST /api/dm/send", d.handleDMSend)
	mux.HandleFunc("GET /api/dm/inbox", d.handleDMInbox)
	mux.HandleFunc("GET /api/dm/thread/{peer_id}", d.handleDMThread)

	// Phase 1 — Topology SSE stream
	mux.HandleFunc("GET /api/topology", d.handleTopologyWS)

	// Phase 2 routes
	d.RegisterPhase2Routes(mux)

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

// ── Phase 0 handlers ──

func (d *Daemon) handleStatus(w http.ResponseWriter, r *http.Request) {
	unread, _ := d.Store.UnreadDMCount()

	// Resolve own geo location (pick first public addr)
	var selfGeo *geo.GeoInfo
	if d.Geo != nil {
		for _, a := range d.Node.Addrs() {
			ip := geo.ExtractIP(a.String())
			if ip != "" && geo.IsPublicIP(ip) {
				selfGeo = d.Geo.Lookup(ip)
				break
			}
		}
	}

	status := map[string]any{
		"peer_id":    d.Node.PeerID().String(),
		"version":    Version,
		"peers":      len(d.Node.ConnectedPeers()),
		"topics":     d.topicNames(),
		"data_dir":   d.DataDir,
		"unread_dm":  unread,
		"geo_db":     d.geoDBType(),
		"started_at": d.StartedAt.Unix(),
	}
	if selfGeo != nil {
		status["location"] = selfGeo.Label()
	}
	writeJSON(w, status)
}

func (d *Daemon) handlePeers(w http.ResponseWriter, r *http.Request) {
	peers := d.Node.ConnectedPeers()
	result := make([]map[string]any, 0, len(peers))
	for _, p := range peers {
		addrs := d.Node.Host.Peerstore().Addrs(p)
		pid := p.String()
		entry := map[string]any{
			"peer_id": pid,
		}
		// Resolve geo from first public addr; never expose raw IPs
		if d.Geo != nil {
			for _, a := range addrs {
				ip := geo.ExtractIP(a.String())
				if ip != "" && geo.IsPublicIP(ip) {
					if gi := d.Geo.Lookup(ip); gi != nil {
						entry["location"] = gi.Label()
						entry["geo"] = gi
					}
					break
				}
			}
		}
		if m, ok := d.PeerMottos.Load(pid); ok {
			entry["motto"] = m.(string)
		}
		if n, ok := d.PeerAgentNames.Load(pid); ok {
			entry["agent_name"] = n.(string)
		}
		result = append(result, entry)
	}
	writeJSON(w, result)
}

func (d *Daemon) handlePeersGeo(w http.ResponseWriter, r *http.Request) {
	peers := d.Node.ConnectedPeers()
	type peerGeo struct {
		PeerID         string       `json:"peer_id"`
		ShortID        string       `json:"short_id"`
		AgentName      string       `json:"agent_name,omitempty"`
		Location       string       `json:"location"`
		Geo            *geo.GeoInfo `json:"geo,omitempty"`
		IsSelf         bool         `json:"is_self"`
		LatencyMs      int64        `json:"latency_ms"`
		ConnectedSince int64        `json:"connected_since"`
		Motto          string       `json:"motto,omitempty"`
	}
	result := make([]peerGeo, 0, len(peers)+1)

	// Add self first
	selfID := d.Node.PeerID().String()
	selfEntry := peerGeo{PeerID: selfID, ShortID: shortID(selfID), Location: "Unknown", IsSelf: true}
	if d.Profile != nil {
		selfEntry.Motto = d.Profile.Motto
		selfEntry.AgentName = d.Profile.AgentName
	}
	if d.Geo != nil {
		for _, a := range d.Node.Addrs() {
			ip := geo.ExtractIP(a.String())
			if ip != "" && geo.IsPublicIP(ip) {
				if gi := d.Geo.Lookup(ip); gi != nil {
					selfEntry.Location = gi.Label()
					selfEntry.Geo = gi
				}
				break
			}
		}
	}
	result = append(result, selfEntry)

	// Add peers
	for _, p := range peers {
		addrs := d.Node.Host.Peerstore().Addrs(p)
		pid := p.String()
		entry := peerGeo{PeerID: pid, ShortID: shortID(pid), Location: "Unknown"}

		// Get latency from peerstore
		lat := d.Node.Host.Peerstore().LatencyEWMA(p)
		if lat > 0 {
			entry.LatencyMs = lat.Milliseconds()
		}

		// Get connection open time
		conns := d.Node.Host.Network().ConnsToPeer(p)
		if len(conns) > 0 {
			entry.ConnectedSince = conns[0].Stat().Opened.Unix()
		}

		if d.Geo != nil {
			for _, a := range addrs {
				ip := geo.ExtractIP(a.String())
				if ip != "" && geo.IsPublicIP(ip) {
					if gi := d.Geo.Lookup(ip); gi != nil {
						entry.Location = gi.Label()
						entry.Geo = gi
					}
					break
				}
			}
		}
		if m, ok := d.PeerMottos.Load(pid); ok {
			entry.Motto = m.(string)
		}
		if n, ok := d.PeerAgentNames.Load(pid); ok {
			entry.AgentName = n.(string)
		}
		result = append(result, entry)
	}
	writeJSON(w, result)
}

// handleSetMotto sets the node's motto/proclamation and gossips it.
func (d *Daemon) handleSetMotto(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Motto string `json:"motto"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if d.Profile != nil {
		d.Profile.Motto = body.Motto
	}
	// Gossip the motto to the network
	d.publishMotto(d.ctx, body.Motto)
	writeJSON(w, map[string]string{"status": "ok", "motto": body.Motto})
}

// handleTraffic returns cumulative network traffic bytes (NIC + P2P).
func (d *Daemon) handleTraffic(w http.ResponseWriter, r *http.Request) {
	nicRx, nicTx := d.getTrafficBytes()
	var p2pRx, p2pTx uint64
	if bw := d.Node.BwCounter; bw != nil {
		stats := bw.GetBandwidthTotals()
		if stats.TotalIn > 0 {
			p2pRx = uint64(stats.TotalIn)
		}
		if stats.TotalOut > 0 {
			p2pTx = uint64(stats.TotalOut)
		}
	}
	writeJSON(w, map[string]any{
		"nic_name": d.nicName,
		"nic_rx":   nicRx,
		"nic_tx":   nicTx,
		"p2p_rx":   p2pRx,
		"p2p_tx":   p2pTx,
	})
}

func shortID(id string) string {
	if len(id) > 16 {
		return id[:16]
	}
	return id
}

func (d *Daemon) geoDBType() string {
	if d.Geo != nil {
		return d.Geo.DBType()
	}
	return "none"
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
	if d.Profile == nil {
		d.Profile = &p
	} else {
		if p.AgentName != "" {
			d.Profile.AgentName = p.AgentName
		}
		if p.Visibility != "" {
			d.Profile.Visibility = p.Visibility
		}
		if p.Domains != nil {
			d.Profile.Domains = p.Domains
		}
		if p.Capabilities != nil {
			d.Profile.Capabilities = p.Capabilities
		}
		if p.Bio != "" {
			d.Profile.Bio = p.Bio
		}
		if p.Motto != "" {
			d.Profile.Motto = p.Motto
		}
		if p.Version != "" {
			d.Profile.Version = p.Version
		}
	}
	writeJSON(w, map[string]string{"status": "updated"})
}

// ── Knowledge handlers ──

func (d *Daemon) handlePostKnowledge(w http.ResponseWriter, r *http.Request) {
	var entry store.KnowledgeEntry
	if err := json.NewDecoder(r.Body).Decode(&entry); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if entry.Title == "" || entry.Body == "" {
		http.Error(w, `{"error":"title and body are required"}`, http.StatusBadRequest)
		return
	}
	if err := d.publishKnowledge(d.ctx, &entry); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, entry)
}

func (d *Daemon) handleKnowledgeFeed(w http.ResponseWriter, r *http.Request) {
	domain := r.URL.Query().Get("domain")
	limit := queryInt(r, "limit", 50)
	offset := queryInt(r, "offset", 0)

	entries, err := d.Store.ListKnowledge(domain, limit, offset)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if entries == nil {
		entries = []*store.KnowledgeEntry{}
	}
	writeJSON(w, entries)
}

func (d *Daemon) handleKnowledgeSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		http.Error(w, `{"error":"q parameter required"}`, http.StatusBadRequest)
		return
	}
	limit := queryInt(r, "limit", 20)
	escaped := store.EscapeFTS5(q)
	entries, err := d.Store.SearchKnowledge(escaped, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if entries == nil {
		entries = []*store.KnowledgeEntry{}
	}
	writeJSON(w, entries)
}

func (d *Daemon) handleKnowledgeReact(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		Reaction string `json:"reaction"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if body.Reaction != "upvote" && body.Reaction != "flag" {
		http.Error(w, `{"error":"reaction must be upvote or flag"}`, http.StatusBadRequest)
		return
	}
	if err := d.publishReact(d.ctx, id, body.Reaction); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]string{"status": "ok"})
}

func (d *Daemon) handleKnowledgeReply(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		Body string `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if body.Body == "" {
		http.Error(w, `{"error":"body is required"}`, http.StatusBadRequest)
		return
	}
	if err := d.publishReply(d.ctx, id, body.Body); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]string{"status": "ok"})
}

func (d *Daemon) handleKnowledgeReplies(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	limit := queryInt(r, "limit", 50)
	replies, err := d.Store.ListReplies(id, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if replies == nil {
		replies = []*store.KnowledgeReply{}
	}
	writeJSON(w, replies)
}

// ── Topic Room handlers ──

func (d *Daemon) handleCreateTopic(w http.ResponseWriter, r *http.Request) {
	var room store.TopicRoom
	if err := json.NewDecoder(r.Body).Decode(&room); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if room.Name == "" {
		http.Error(w, `{"error":"name is required"}`, http.StatusBadRequest)
		return
	}
	room.CreatorID = d.Node.PeerID().String()
	if room.CreatedAt == "" {
		room.CreatedAt = "now"
	}
	if err := d.joinTopicRoom(d.ctx, &room); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, room)
}

func (d *Daemon) handleListTopics(w http.ResponseWriter, r *http.Request) {
	topics, err := d.Store.ListTopics()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if topics == nil {
		topics = []*store.TopicRoom{}
	}
	writeJSON(w, topics)
}

func (d *Daemon) handleJoinTopic(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	room := &store.TopicRoom{
		Name:      name,
		CreatorID: d.Node.PeerID().String(),
		CreatedAt: "now",
	}
	if err := d.joinTopicRoom(d.ctx, room); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]string{"status": "joined", "topic": name})
}

func (d *Daemon) handleLeaveTopic(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := d.Store.SetTopicJoined(name, false); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// Note: we don't unsubscribe from GossipSub to still receive broadcasts
	writeJSON(w, map[string]string{"status": "left", "topic": name})
}

func (d *Daemon) handlePostTopicMessage(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var body struct {
		Body string `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if body.Body == "" {
		http.Error(w, `{"error":"body is required"}`, http.StatusBadRequest)
		return
	}
	if err := d.publishTopicMessage(d.ctx, name, body.Body); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]string{"status": "sent"})
}

func (d *Daemon) handleGetTopicMessages(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	limit := queryInt(r, "limit", 50)
	offset := queryInt(r, "offset", 0)
	msgs, err := d.Store.ListTopicMessages(name, limit, offset)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if msgs == nil {
		msgs = []*store.TopicMessage{}
	}
	writeJSON(w, msgs)
}

// ── DM handlers ──

func (d *Daemon) handleDMSend(w http.ResponseWriter, r *http.Request) {
	var body struct {
		PeerID string `json:"peer_id"`
		Body   string `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if body.PeerID == "" || body.Body == "" {
		http.Error(w, `{"error":"peer_id and body are required"}`, http.StatusBadRequest)
		return
	}
	if err := d.sendDM(d.ctx, body.PeerID, body.Body); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]string{"status": "sent"})
}

func (d *Daemon) handleDMInbox(w http.ResponseWriter, r *http.Request) {
	msgs, err := d.Store.ListDMInbox()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if msgs == nil {
		msgs = []*store.DirectMessage{}
	}
	writeJSON(w, msgs)
}

func (d *Daemon) handleDMThread(w http.ResponseWriter, r *http.Request) {
	peerID := r.PathValue("peer_id")
	limit := queryInt(r, "limit", 50)
	offset := queryInt(r, "offset", 0)
	msgs, err := d.Store.ListDMThread(peerID, limit, offset)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if msgs == nil {
		msgs = []*store.DirectMessage{}
	}
	// Mark as read
	d.Store.MarkDMRead(peerID)
	writeJSON(w, msgs)
}

// ── Topology WebSocket ──

var wsUpgrader = &wsUpgradeHelper{}

type wsUpgradeHelper struct{}

type wsConn struct {
	w   http.ResponseWriter
	f   http.Flusher
	ctx context.Context
}

// handleTopologyWS streams topology updates as Server-Sent Events (simpler than WebSocket, no extra deps).
func (d *Daemon) handleTopologyWS(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Send initial topology
	d.sendTopologyEvent(w, flusher)

	// Register for updates
	ch := d.registerTopologyListener()
	defer d.unregisterTopologyListener(ch)

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ch:
			d.sendTopologyEvent(w, flusher)
		}
	}
}

func (d *Daemon) sendTopologyEvent(w http.ResponseWriter, f http.Flusher) {
	peers := d.Node.ConnectedPeers()
	selfID := d.Node.PeerID().String()

	selfNode := map[string]any{
		"id": selfID, "name": d.Profile.AgentName, "self": true,
	}
	if d.Geo != nil {
		for _, a := range d.Node.Addrs() {
			ip := geo.ExtractIP(a.String())
			if ip != "" && geo.IsPublicIP(ip) {
				if gi := d.Geo.Lookup(ip); gi != nil {
					selfNode["location"] = gi.Label()
					selfNode["geo"] = gi
				}
				break
			}
		}
	}

	nodes := []map[string]any{selfNode}
	links := []map[string]string{}

	for _, p := range peers {
		pid := p.String()
		node := map[string]any{
			"id":   pid,
			"name": pid[:16],
			"self": false,
		}
		if d.Geo != nil {
			addrs := d.Node.Host.Peerstore().Addrs(p)
			for _, a := range addrs {
				ip := geo.ExtractIP(a.String())
				if ip != "" && geo.IsPublicIP(ip) {
					if gi := d.Geo.Lookup(ip); gi != nil {
						node["location"] = gi.Label()
						node["geo"] = gi
					}
					break
				}
			}
		}
		nodes = append(nodes, node)
		links = append(links, map[string]string{
			"source": selfID,
			"target": pid,
		})
	}

	data := map[string]any{"nodes": nodes, "links": links}
	jsonData, _ := json.Marshal(data)
	fmt.Fprintf(w, "data: %s\n\n", jsonData)
	f.Flush()
}

var (
	topologyListenersMu sync.Mutex
	topologyListeners   = make(map[chan struct{}]struct{})
)

func (d *Daemon) registerTopologyListener() chan struct{} {
	ch := make(chan struct{}, 1)
	topologyListenersMu.Lock()
	topologyListeners[ch] = struct{}{}
	topologyListenersMu.Unlock()
	return ch
}

func (d *Daemon) unregisterTopologyListener(ch chan struct{}) {
	topologyListenersMu.Lock()
	delete(topologyListeners, ch)
	topologyListenersMu.Unlock()
}

// NotifyTopologyChange alerts all SSE listeners of a topology change.
func NotifyTopologyChange() {
	topologyListenersMu.Lock()
	for ch := range topologyListeners {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
	topologyListenersMu.Unlock()
}

// ── helpers ──

func (d *Daemon) topicNames() []string {
	names := make([]string, 0, len(d.Node.Topics))
	for name := range d.Node.Topics {
		names = append(names, name)
	}
	return names
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func queryInt(r *http.Request, key string, defaultVal int) int {
	s := r.URL.Query().Get(key)
	if s == "" {
		return defaultVal
	}
	v, err := strconv.Atoi(s)
	if err != nil || v < 0 {
		return defaultVal
	}
	return v
}
