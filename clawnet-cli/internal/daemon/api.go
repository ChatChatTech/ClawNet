package daemon

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/protocol/ping"
	"github.com/multiformats/go-multiaddr"

	"github.com/ChatChatTech/ClawNet/clawnet-cli/internal/config"
	"github.com/ChatChatTech/ClawNet/clawnet-cli/internal/geo"
	"github.com/ChatChatTech/ClawNet/clawnet-cli/internal/store"
)

// StartAPI starts the HTTP API server for the daemon.
func (d *Daemon) StartAPI(ctx context.Context) *http.Server {
	mux := http.NewServeMux()

	// Phase 0 endpoints
	mux.HandleFunc("GET /api/status", d.handleStatus)
	mux.HandleFunc("GET /api/heartbeat", d.handleHeartbeat)
	mux.HandleFunc("GET /api/peers", d.handlePeers)
	mux.HandleFunc("GET /api/peers/geo", d.handlePeersGeo)
	mux.HandleFunc("GET /api/profile", d.handleGetProfile)
	mux.HandleFunc("PUT /api/profile", d.handleUpdateProfile)
	mux.HandleFunc("PUT /api/motto", d.handleSetMotto)
	mux.HandleFunc("GET /api/traffic", d.handleTraffic)
	mux.HandleFunc("GET /api/peers/{id}/profile", d.handleLookupPeerProfile)
	mux.HandleFunc("GET /api/peers/{id}/ping", d.handlePeerPing)

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
	mux.HandleFunc("DELETE /api/dm/thread/{peer_id}", d.handleDMDelete)

	// Chat — random peer matching
	mux.HandleFunc("GET /api/chat/match", d.handleChatMatch)

	// Phase 1 — Topology SSE stream
	mux.HandleFunc("GET /api/topology", d.handleTopologyWS)

	// Diagnostics
	mux.HandleFunc("GET /api/diagnostics", d.handleDiagnostics)

	// Overlay (Ironwood) transport status
	mux.HandleFunc("GET /api/overlay/status", d.handleOverlayStatus)
	mux.HandleFunc("GET /api/overlay/tree", d.handleOverlayTree)
	mux.HandleFunc("GET /api/overlay/peers/geo", d.handleOverlayPeersGeo)
	mux.HandleFunc("GET /api/overlay/bloom-test", d.handleOverlayBloomTest)

	// Overlay peer management
	mux.HandleFunc("GET /api/overlay/peers", d.handleOverlayPeersList)
	mux.HandleFunc("POST /api/overlay/peers/add", d.handleOverlayPeerAdd)
	mux.HandleFunc("POST /api/overlay/peers/remove", d.handleOverlayPeerRemove)
	mux.HandleFunc("POST /api/overlay/peers/retry", d.handleOverlayPeersRetry)

	// Overlay molt / unmolt (TUN IPv6 access control)
	mux.HandleFunc("POST /api/overlay/molt", d.handleOverlayMolt)
	mux.HandleFunc("POST /api/overlay/unmolt", d.handleOverlayUnmolt)
	mux.HandleFunc("GET /api/overlay/molt/status", d.handleOverlayMoltStatus)

	// E2E crypto status
	mux.HandleFunc("GET /api/crypto/sessions", d.handleCryptoSessions)

	// Phase 2 routes
	d.RegisterPhase2Routes(mux)

	// Intuitive design routes (milestones, achievements, watch, endpoints)
	d.RegisterIntuitiveRoutes(mux)

	// Wrap mux with localhost access guard.
	handler := localhostGuard(mux)

	addr := fmt.Sprintf("0.0.0.0:%d", d.Config.WebUIPort)
	server := &http.Server{
		Addr:    addr,
		Handler: handler,
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
	if d.Overlay != nil {
		status["overlay_peers"] = d.Overlay.PeerCount()
		status["overlay_ipv6"] = d.Overlay.OverlayAddress()
		status["overlay_subnet"] = d.Overlay.OverlaySubnet()
		status["overlay_molted"] = d.Overlay.IsMolted()
		status["overlay_tun"] = d.Overlay.TUNName()
	}

	// P0: Next Action hint
	status["next_action"] = d.nextAction()
	// P1: Milestone progress
	status["milestones"] = d.milestoneProgress()
	// P1: Achievement count
	peerID := d.Node.PeerID().String()
	status["achievements"] = d.Store.AchievementCount(peerID)
	// P2: Pending offline operations
	if pc := d.Store.PendingOpCount(); pc > 0 {
		status["pending_ops"] = pc
	}
	// P3: Role template
	if d.Profile.Role != "" {
		status["role"] = d.Profile.Role
	}
	// P3: Credit balance for zero-balance hints
	peerIDStr := d.Node.PeerID().String()
	if ep, err := d.Store.GetEnergyProfile(peerIDStr); err == nil && ep != nil {
		status["balance"] = ep.Energy
	}

	writeJSON(w, status)
}

func (d *Daemon) handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	if d.hbState == nil {
		writeJSON(w, map[string]string{"status": "initializing"})
		return
	}
	writeJSON(w, d.hbState)
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
		Role           string       `json:"role,omitempty"`
		Location       string       `json:"location"`
		Geo            *geo.GeoInfo `json:"geo,omitempty"`
		IsSelf         bool         `json:"is_self"`
		LatencyMs      int64        `json:"latency_ms"`
		ConnectedSince int64        `json:"connected_since"`
		Motto          string       `json:"motto,omitempty"`
		BwIn           int64        `json:"bw_in"`
		BwOut          int64        `json:"bw_out"`
		Reputation     float64      `json:"reputation"`
	}
	result := make([]peerGeo, 0, len(peers)+1)

	// Add self first
	selfID := d.Node.PeerID().String()
	selfEntry := peerGeo{PeerID: selfID, ShortID: shortID(selfID), Location: "Unknown", IsSelf: true}
	if d.Profile != nil {
		selfEntry.Motto = d.Profile.Motto
		selfEntry.AgentName = d.Profile.AgentName
		selfEntry.Role = d.Profile.Role
	}
	if rep, err := d.Store.GetReputation(selfID); err == nil {
		selfEntry.Reputation = rep.Score
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
		if rl, ok := d.PeerRoles.Load(pid); ok {
			entry.Role = rl.(string)
		}
		if bw := d.Node.BwCounter; bw != nil {
			st := bw.GetBandwidthForPeer(p)
			entry.BwIn = int64(st.TotalIn)
			entry.BwOut = int64(st.TotalOut)
		}
		if rep, err := d.Store.GetReputation(pid); err == nil {
			entry.Reputation = rep.Score
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

// selfCurrency returns the CurrencyInfo for this node's geo location.
func (d *Daemon) selfCurrency() *geo.CurrencyInfo {
	if d.Geo == nil {
		return geo.CurrencyForCountry("")
	}
	for _, a := range d.Node.Addrs() {
		ip := geo.ExtractIP(a.String())
		if ip != "" && geo.IsPublicIP(ip) {
			if gi := d.Geo.Lookup(ip); gi != nil {
				return geo.CurrencyForCountry(gi.Country)
			}
		}
	}
	return geo.CurrencyForCountry("")
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
	// Persist to disk
	if err := d.saveProfile(); err != nil {
		fmt.Printf("warning: failed to save profile: %v\n", err)
	}
	// Re-publish to DHT
	go func() {
		if err := d.Node.PublishProfile(d.ctx, d.Profile); err != nil {
			fmt.Printf("dht-profile: re-publish failed: %v\n", err)
		}
	}()
	writeJSON(w, map[string]string{"status": "updated"})
}

func (d *Daemon) handleLookupPeerProfile(w http.ResponseWriter, r *http.Request) {
	pidStr := r.PathValue("id")
	pid, err := peer.Decode(pidStr)
	if err != nil {
		http.Error(w, `{"error":"invalid peer ID"}`, http.StatusBadRequest)
		return
	}
	rec, err := d.Node.LookupProfile(r.Context(), pid)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusNotFound)
		return
	}
	writeJSON(w, rec)
}

func (d *Daemon) handlePeerPing(w http.ResponseWriter, r *http.Request) {
	pidStr := r.PathValue("id")
	pid, err := peer.Decode(pidStr)
	if err != nil {
		http.Error(w, `{"error":"invalid peer ID"}`, http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	ch := ping.Ping(ctx, d.Node.Host, pid)
	select {
	case res := <-ch:
		if res.Error != nil {
			http.Error(w, fmt.Sprintf(`{"error":"ping failed: %s"}`, res.Error.Error()), http.StatusBadGateway)
			return
		}
		d.Node.Host.Peerstore().RecordLatency(pid, res.RTT)
		writeJSON(w, map[string]any{
			"peer_id":    pidStr,
			"rtt_ms":     res.RTT.Milliseconds(),
			"rtt_string": res.RTT.String(),
		})
	case <-ctx.Done():
		http.Error(w, `{"error":"ping timeout"}`, http.StatusGatewayTimeout)
	}
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

	// Milestone: first knowledge entry
	reward := d.CheckAndCompleteMilestone("first_knowledge")
	d.RecordEvent("knowledge_published", d.Node.PeerID().String(), entry.ID, entry.Title)
	d.BroadcastEcho(r.Context(), "knowledge_published", entry.Title, fmt.Sprintf("%s shared: %s", d.Profile.AgentName, entry.Title))

	resp := map[string]any{"entry": entry}
	if reward > 0 {
		resp["milestone_completed"] = "first_knowledge"
		resp["milestone_reward"] = reward
	}
	writeJSON(w, resp)
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

	// Milestone: first topic message
	reward := d.CheckAndCompleteMilestone("first_topic")
	d.RecordEvent("topic_message", d.Node.PeerID().String(), name, body.Body)

	resp := map[string]any{"status": "sent"}
	if reward > 0 {
		resp["milestone_completed"] = "first_topic"
		resp["milestone_reward"] = reward
	}
	writeJSON(w, resp)
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

func (d *Daemon) handleDMDelete(w http.ResponseWriter, r *http.Request) {
	peerID := r.PathValue("peer_id")
	if err := d.Store.DeleteDMThread(peerID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]string{"status": "ok"})
}

// ── Chat matching ──

func (d *Daemon) handleChatMatch(w http.ResponseWriter, r *http.Request) {
	peers := d.Node.ConnectedPeers()
	if len(peers) == 0 {
		http.Error(w, `{"error":"no peers online"}`, http.StatusServiceUnavailable)
		return
	}
	// Pick a random peer
	idx := time.Now().UnixNano() % int64(len(peers))
	chosen := peers[idx]
	name := chosen.String()[:16]
	if n, ok := d.PeerAgentNames.Load(chosen); ok {
		name = n.(string)
	}
	info := map[string]string{
		"peer_id": chosen.String(),
		"name":    name,
	}
	writeJSON(w, info)
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

// handleDiagnostics returns a full network diagnostics report.
func (d *Daemon) handleDiagnostics(w http.ResponseWriter, r *http.Request) {
	h := d.Node.Host
	nw := h.Network()

	// Collect listen addresses
	listenAddrs := make([]string, 0, len(h.Addrs()))
	for _, a := range h.Addrs() {
		listenAddrs = append(listenAddrs, a.String())
	}

	// Announce addresses from config
	announceAddrs := d.Config.AnnounceAddrs

	// Classify connections
	var directCount, relayCount int
	for _, c := range nw.Conns() {
		addr := c.RemoteMultiaddr().String()
		if strings.Contains(addr, "/p2p-circuit/") {
			relayCount++
		} else {
			directCount++
		}
	}

	// DHT routing table size
	dhtSize := 0
	if d.Node.DHT != nil {
		dhtSize = d.Node.DHT.RoutingTable().Size()
	}

	// BT DHT status
	btdhtStatus := "disabled"
	if d.Node.BTDHT != nil {
		btdhtStatus = "running"
	}

	// NAT status from config
	natMode := "auto"
	if d.Config.ForcePrivate {
		natMode = "force_private"
	}

	// Relay config
	relayEnabled := d.Config.RelayEnabled

	// Bootstrap peers
	bootstrapPeers := d.Config.BootstrapPeers

	// Check bootstrap connectivity
	bootstrapReachable := make(map[string]bool)
	for _, bp := range bootstrapPeers {
		ma, err := multiaddr.NewMultiaddr(bp)
		if err != nil {
			continue
		}
		pi, err := peer.AddrInfoFromP2pAddr(ma)
		if err != nil {
			continue
		}
		connected := nw.Connectedness(pi.ID) == network.Connected
		bootstrapReachable[pi.ID.String()[:16]] = connected
	}

	// Uptime
	uptime := time.Since(d.StartedAt).Truncate(time.Second).String()

	// Bandwidth totals
	bwStats := d.Node.BwCounter.GetBandwidthTotals()

	diag := map[string]any{
		"peer_id":           h.ID().String(),
		"version":           Version,
		"uptime":            uptime,
		"listen_addrs":      listenAddrs,
		"announce_addrs":    announceAddrs,
		"nat_mode":          natMode,
		"relay_enabled":     relayEnabled,
		"peers_total":       len(nw.Peers()),
		"connections_direct": directCount,
		"connections_relay":  relayCount,
		"dht_routing_table": dhtSize,
		"btdht_status":      btdhtStatus,
		"bootstrap_peers":   bootstrapReachable,
		"topics":            d.topicNames(),
		"bandwidth_in":      bwStats.TotalIn,
		"bandwidth_out":     bwStats.TotalOut,
	}

	// Overlay (Ironwood) transport status
	if d.Overlay != nil {
		diag["overlay_peers"] = d.Overlay.PeerCount()
	} else {
		diag["overlay_peers"] = 0
	}

	// E2E crypto status
	if d.Crypto != nil {
		diag["crypto_sessions"] = d.Crypto.SessionCount()
	} else {
		diag["crypto_sessions"] = 0
	}

	writeJSON(w, diag)
}

// handleOverlayStatus returns Ironwood overlay transport status with full debug info.
func (d *Daemon) handleOverlayStatus(w http.ResponseWriter, r *http.Request) {
	status := map[string]any{
		"enabled": d.Config.Overlay.Enabled,
	}
	if d.Overlay != nil {
		status["peer_count"] = d.Overlay.PeerCount()
		status["public_key"] = fmt.Sprintf("%x", d.Overlay.PublicKey())
		status["overlay_ipv6"] = d.Overlay.OverlayAddress()
		status["overlay_subnet"] = d.Overlay.OverlaySubnet()
		status["molted"] = d.Overlay.IsMolted()
		status["tun_device"] = d.Overlay.TUNName()
		if debugInfo := d.Overlay.GetDebugInfo(); debugInfo != nil {
			status["routing_entries"] = debugInfo.Self.RoutingEntries
			status["peers"] = debugInfo.Peers
			status["tree_size"] = len(debugInfo.Tree)
			status["paths"] = debugInfo.Paths
			status["sessions"] = debugInfo.Sessions
		}
	} else {
		status["peer_count"] = 0
	}
	writeJSON(w, status)
}

// handleOverlayTree returns the full Ironwood spanning tree for visualization.
func (d *Daemon) handleOverlayTree(w http.ResponseWriter, r *http.Request) {
	if d.Overlay == nil {
		writeJSON(w, map[string]any{"error": "overlay not enabled"})
		return
	}
	debugInfo := d.Overlay.GetDebugInfo()
	if debugInfo == nil {
		writeJSON(w, map[string]any{"error": "debug info unavailable"})
		return
	}
	writeJSON(w, map[string]any{
		"self": debugInfo.Self,
		"tree": debugInfo.Tree,
	})
}

// handleOverlayBloomTest tests if a destination key is present in any peer's
// bloom filter. GET /api/overlay/bloom-test?key=<hex-encoded-ed25519-pubkey>
func (d *Daemon) handleOverlayBloomTest(w http.ResponseWriter, r *http.Request) {
	if d.Overlay == nil {
		writeJSON(w, map[string]any{"error": "overlay not enabled"})
		return
	}
	keyHex := r.URL.Query().Get("key")
	if keyHex == "" {
		writeJSON(w, map[string]any{"error": "missing key parameter"})
		return
	}
	keyBytes, err := hex.DecodeString(keyHex)
	if err != nil || len(keyBytes) != 32 {
		writeJSON(w, map[string]any{"error": "invalid key (need 64 hex chars)"})
		return
	}
	result := d.Overlay.TestBloomFor(keyBytes)
	matches := []string{}
	for k, v := range result {
		if v {
			matches = append(matches, k)
		}
	}
	writeJSON(w, map[string]any{
		"target_key":     keyHex[:16],
		"bloom_matches":  matches,
		"total_peers":    len(result),
		"matching_peers": len(matches),
	})
}

// handleOverlayPeersGeo returns overlay peers with geo data.
// Geo resolution is done incrementally by a background goroutine;
// this handler only reads from the cache and never blocks on lookups.
func (d *Daemon) handleOverlayPeersGeo(w http.ResponseWriter, r *http.Request) {
	if d.geoCache == nil {
		writeJSON(w, []any{})
		return
	}
	writeJSON(w, d.geoCache.snapshot())
}

// handleOverlayPeersList returns full peer info for all overlay peers.
func (d *Daemon) handleOverlayPeersList(w http.ResponseWriter, r *http.Request) {
	if d.Overlay == nil {
		writeJSON(w, []any{})
		return
	}
	writeJSON(w, d.Overlay.GetPeers())
}

// handleOverlayPeerAdd adds a new overlay peer by URI.
func (d *Daemon) handleOverlayPeerAdd(w http.ResponseWriter, r *http.Request) {
	if d.Overlay == nil {
		http.Error(w, `{"error":"overlay not enabled"}`, http.StatusServiceUnavailable)
		return
	}
	var req struct {
		URI string `json:"uri"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.URI == "" {
		http.Error(w, `{"error":"missing or invalid uri"}`, http.StatusBadRequest)
		return
	}
	if err := d.Overlay.AddPeer(req.URI); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "uri": req.URI})
}

// handleOverlayPeerRemove removes an overlay peer by URI.
func (d *Daemon) handleOverlayPeerRemove(w http.ResponseWriter, r *http.Request) {
	if d.Overlay == nil {
		http.Error(w, `{"error":"overlay not enabled"}`, http.StatusServiceUnavailable)
		return
	}
	var req struct {
		URI string `json:"uri"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.URI == "" {
		http.Error(w, `{"error":"missing or invalid uri"}`, http.StatusBadRequest)
		return
	}
	if err := d.Overlay.RemovePeer(req.URI); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "uri": req.URI})
}

// handleOverlayPeersRetry forces all overlay links to retry immediately.
func (d *Daemon) handleOverlayPeersRetry(w http.ResponseWriter, r *http.Request) {
	if d.Overlay == nil {
		http.Error(w, `{"error":"overlay not enabled"}`, http.StatusServiceUnavailable)
		return
	}
	d.Overlay.RetryPeersNow()
	writeJSON(w, map[string]any{"ok": true})
}

// handleOverlayMolt enables full mesh interop (molt = shed shell, open to all).
func (d *Daemon) handleOverlayMolt(w http.ResponseWriter, r *http.Request) {
	if d.Overlay == nil {
		http.Error(w, `{"error":"overlay not enabled"}`, http.StatusServiceUnavailable)
		return
	}
	d.Overlay.Molt()
	d.Config.Overlay.Molted = true
	_ = d.Config.Save()
	writeJSON(w, map[string]any{"ok": true, "molted": true})
}

// handleOverlayUnmolt returns to ClawNet-only mode (grow new shell).
func (d *Daemon) handleOverlayUnmolt(w http.ResponseWriter, r *http.Request) {
	if d.Overlay == nil {
		http.Error(w, `{"error":"overlay not enabled"}`, http.StatusServiceUnavailable)
		return
	}
	d.Overlay.Unmolt()
	d.Config.Overlay.Molted = false
	_ = d.Config.Save()
	writeJSON(w, map[string]any{"ok": true, "molted": false})
}

// handleOverlayMoltStatus returns current molt state.
func (d *Daemon) handleOverlayMoltStatus(w http.ResponseWriter, r *http.Request) {
	if d.Overlay == nil {
		writeJSON(w, map[string]any{"molted": false, "enabled": false, "tun": ""})
		return
	}
	writeJSON(w, map[string]any{
		"molted":  d.Overlay.IsMolted(),
		"enabled": true,
		"tun":     d.Overlay.TUNName(),
	})
}

// handleCryptoSessions returns E2E encryption session info.
func (d *Daemon) handleCryptoSessions(w http.ResponseWriter, r *http.Request) {
	status := map[string]any{
		"enabled": d.Crypto != nil,
	}
	if d.Crypto != nil {
		status["session_count"] = d.Crypto.SessionCount()
		status["sessions"] = d.Crypto.Sessions()
	} else {
		status["session_count"] = 0
		status["sessions"] = []any{}
	}
	writeJSON(w, status)
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

// localhostGuard rejects requests that do not originate from localhost.
// It inspects the connecting IP (RemoteAddr) rather than trusting headers.
func localhostGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			host = r.RemoteAddr
		}
		ip := net.ParseIP(host)
		if ip != nil && (ip.IsLoopback() || ip.Equal(net.IPv4zero) || ip.Equal(net.IPv6zero)) {
			next.ServeHTTP(w, r)
			return
		}
		http.Error(w, `{"error":"access denied: API is localhost-only"}`, http.StatusForbidden)
	})
}
