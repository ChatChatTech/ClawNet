package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
)

const EchoTopic = "/clawnet/echo"

// EchoMsg is a lightweight event broadcast over gossip.
// Kept small (<200 bytes) — does not enter FTS or permanent storage.
type EchoMsg struct {
	Type      string `json:"type"`       // "task_claimed", "knowledge_published", etc.
	Actor     string `json:"actor"`      // short peer ID (first 16 chars)
	ActorName string `json:"actor_name"` // agent name
	Target    string `json:"target"`     // task/knowledge ID or title
	Timestamp int64  `json:"ts"`         // unix seconds
	Message   string `json:"message"`    // human-readable summary
}

// echoBuf is a ring buffer of recent echo messages received from the network.
type echoBuf struct {
	mu   sync.Mutex
	ring []EchoMsg
	cap  int
	idx  int
}

func newEchoBuf(cap int) *echoBuf {
	return &echoBuf{ring: make([]EchoMsg, 0, cap), cap: cap}
}

func (b *echoBuf) push(m EchoMsg) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.ring) < b.cap {
		b.ring = append(b.ring, m)
	} else {
		b.ring[b.idx] = m
	}
	b.idx = (b.idx + 1) % b.cap
}

// Recent returns up to n most recent echoes in chronological order.
func (b *echoBuf) Recent(n int) []EchoMsg {
	b.mu.Lock()
	defer b.mu.Unlock()
	total := len(b.ring)
	if n > total {
		n = total
	}
	if total < b.cap {
		// ring not yet full — just take the last n
		return append([]EchoMsg{}, b.ring[total-n:]...)
	}
	// ring is full — read from idx backwards
	result := make([]EchoMsg, n)
	for i := 0; i < n; i++ {
		pos := (b.idx - n + i + b.cap) % b.cap
		result[i] = b.ring[pos]
	}
	return result
}

// startEchoHandler joins the echo gossip topic and processes incoming echoes.
func (d *Daemon) startEchoHandler(ctx context.Context) {
	sub, err := d.Node.JoinTopic(EchoTopic)
	if err != nil {
		fmt.Printf("warning: could not join echo topic: %v\n", err)
		return
	}
	d.EchoBuf = newEchoBuf(100)
	go d.handleEchoSub(ctx, sub)
}

func (d *Daemon) handleEchoSub(ctx context.Context, sub *pubsub.Subscription) {
	for {
		msg, err := sub.Next(ctx)
		if err != nil {
			return
		}
		if msg.ReceivedFrom == d.Node.PeerID() {
			continue // skip own echoes
		}
		var echo EchoMsg
		if err := json.Unmarshal(msg.Data, &echo); err != nil {
			continue
		}
		// Discard echoes older than 24 hours
		if time.Now().Unix()-echo.Timestamp > 86400 {
			continue
		}
		d.EchoBuf.push(echo)
		// Also record as local event for watch stream
		d.Store.InsertEvent("echo_"+echo.Type, echo.Actor, echo.Target, echo.Message)
		notifyWatchListeners()
	}
}

// BroadcastEcho publishes an echo message to the gossip network.
func (d *Daemon) BroadcastEcho(ctx context.Context, typ, target, message string) {
	peerID := d.Node.PeerID().String()
	name := ""
	if d.Profile != nil {
		name = d.Profile.AgentName
	}
	echo := EchoMsg{
		Type:      typ,
		Actor:     peerID[:16],
		ActorName: name,
		Target:    target,
		Timestamp: time.Now().Unix(),
		Message:   message,
	}
	data, err := json.Marshal(echo)
	if err != nil {
		return
	}
	d.Node.Publish(ctx, EchoTopic, data)
	// Store locally too
	if d.EchoBuf != nil {
		d.EchoBuf.push(echo)
	}
}
