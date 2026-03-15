package daemon

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"time"

	"github.com/ChatChatTech/ClawNet/clawnet-cli/internal/store"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
)

const (
	SyncProtocol = protocol.ID("/clawnet/knowledge-sync/1.0.0")
	syncMaxItems = 500
)

// syncRequest is sent by the requesting peer to indicate what it already has.
type syncRequest struct {
	KnowledgeSince     string `json:"knowledge_since"`
	TopicMessagesSince string `json:"topic_messages_since"`
}

// syncEntry is a union type for streamed sync items.
type syncEntry struct {
	Type         string               `json:"type"` // "knowledge", "topic_room", "topic_message"
	Knowledge    *store.KnowledgeEntry `json:"knowledge,omitempty"`
	TopicRoom    *store.TopicRoom      `json:"topic_room,omitempty"`
	TopicMessage *store.TopicMessage   `json:"topic_message,omitempty"`
}

// registerSyncHandler sets up the libp2p stream handler for history sync requests.
func (d *Daemon) registerSyncHandler() {
	d.Node.Host.SetStreamHandler(SyncProtocol, func(s network.Stream) {
		defer s.Close()
		s.SetDeadline(time.Now().Add(30 * time.Second))
		remotePeer := s.Conn().RemotePeer().String()

		// Read request
		reader := bufio.NewReader(s)
		line, err := reader.ReadBytes('\n')
		if err != nil {
			fmt.Printf("sync-handler: read request error from %s: %v\n", remotePeer[:16], err)
			return
		}
		var req syncRequest
		if err := json.Unmarshal(line, &req); err != nil {
			fmt.Printf("sync-handler: unmarshal error from %s: %v\n", remotePeer[:16], err)
			return
		}

		writer := bufio.NewWriter(s)
		defer writer.Flush()

		count := 0

		// Stream knowledge entries
		entries, err := d.Store.ListKnowledgeSince(req.KnowledgeSince, syncMaxItems)
		if err == nil {
			for _, e := range entries {
				data, _ := json.Marshal(syncEntry{Type: "knowledge", Knowledge: e})
				writer.Write(data)
				writer.WriteByte('\n')
				count++
			}
		}

		// Stream topic rooms (send all rooms so receiver can create them before messages)
		rooms, err := d.Store.ListTopics()
		if err == nil {
			for _, r := range rooms {
				data, _ := json.Marshal(syncEntry{Type: "topic_room", TopicRoom: r})
				writer.Write(data)
				writer.WriteByte('\n')
				count++
			}
		}

		// Stream topic messages
		msgs, err := d.Store.ListTopicMessagesSince(req.TopicMessagesSince, syncMaxItems)
		if err == nil {
			for _, m := range msgs {
				data, _ := json.Marshal(syncEntry{Type: "topic_message", TopicMessage: m})
				writer.Write(data)
				writer.WriteByte('\n')
				count++
			}
		}
		fmt.Printf("sync-handler: sent %d entries to %s (since k=%q t=%q)\n", count, remotePeer[:16], req.KnowledgeSince, req.TopicMessagesSince)
	})
}

// requestHistorySync connects to a random peer and requests missing history.
func (d *Daemon) requestHistorySync(ctx context.Context) {
	peers := d.Node.ConnectedPeers()
	if len(peers) == 0 {
		return
	}

	// Shuffle peers and try those that support the protocol
	rand.Shuffle(len(peers), func(i, j int) { peers[i], peers[j] = peers[j], peers[i] })
	for _, target := range peers {
		count := d.syncFromPeer(ctx, target)
		if count >= 0 {
			if count > 0 {
				return // got data, done
			}
		}
	}
}

// syncFromPeer attempts sync with one peer. Returns -1 if protocol unsupported, 0 if no new data, >0 for entries synced.
func (d *Daemon) syncFromPeer(ctx context.Context, target peer.ID) int {
	s, err := d.Node.Host.NewStream(ctx, target, SyncProtocol)
	if err != nil {
		return -1 // peer doesn't support sync protocol
	}
	defer s.Close()
	s.SetDeadline(time.Now().Add(30 * time.Second))

	// Build request with our latest timestamps
	req := syncRequest{
		KnowledgeSince:     d.Store.LatestKnowledgeTime(),
		TopicMessagesSince: d.Store.LatestTopicMessageTime(),
	}
	data, _ := json.Marshal(req)
	data = append(data, '\n')
	if _, err := s.Write(data); err != nil {
		return -1
	}
	// Signal we're done writing the request
	s.CloseWrite()

	// Read streamed entries
	reader := bufio.NewReader(s)
	count := 0
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if err != io.EOF {
				fmt.Printf("sync: read error: %v\n", err)
			}
			break
		}
		var entry syncEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}
		switch entry.Type {
		case "knowledge":
			if entry.Knowledge != nil {
				d.Store.InsertKnowledge(entry.Knowledge)
				count++
			}
		case "topic_room":
			if entry.TopicRoom != nil {
				entry.TopicRoom.Joined = false // don't auto-join synced rooms
				d.Store.InsertTopic(entry.TopicRoom)
			}
		case "topic_message":
			if entry.TopicMessage != nil {
				d.Store.InsertTopicMessage(entry.TopicMessage)
				count++
			}
		}
	}
	if count > 0 {
		fmt.Printf("sync: fetched %d entries from %s\n", count, target.String()[:16])
	}
	return count
}

// startHistorySync triggers a history sync shortly after startup, then periodically.
func (d *Daemon) startHistorySync(ctx context.Context) {
	go func() {
		// Wait for peer connections to establish
		time.Sleep(20 * time.Second)
		fmt.Println("sync: starting initial history sync...")
		d.requestHistorySync(ctx)
		fmt.Println("sync: initial sync complete")

		// Periodic re-sync every 5 minutes
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				d.requestHistorySync(ctx)
			}
		}
	}()
}
