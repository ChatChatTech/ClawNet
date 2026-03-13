package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ChatChatTech/ClawNet/clawnet-cli/internal/store"
	"github.com/google/uuid"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
)

const (
	KnowledgeTopic = "/clawnet/knowledge"
	TopicPrefix    = "/clawnet/topic/"
	MottoTopic     = "/clawnet/motto"
)

// GossipKnowledgeMsg is the wire format for knowledge messages on GossipSub.
type GossipKnowledgeMsg struct {
	Type  string               `json:"type"` // "knowledge", "react", "reply"
	Entry *store.KnowledgeEntry `json:"entry,omitempty"`
	React *GossipReact          `json:"react,omitempty"`
	Reply *store.KnowledgeReply `json:"reply,omitempty"`
}

type GossipReact struct {
	KnowledgeID string `json:"knowledge_id"`
	PeerID      string `json:"peer_id"`
	Reaction    string `json:"reaction"`
}

// GossipTopicMsg is the wire format for topic room messages on GossipSub.
type GossipTopicMsg struct {
	Type    string              `json:"type"` // "message", "create"
	Room    *store.TopicRoom    `json:"room,omitempty"`
	Message *store.TopicMessage `json:"message,omitempty"`
}

// GossipMottoMsg is the wire format for motto/proclamation announcements.
type GossipMottoMsg struct {
	Type      string `json:"type"`       // "motto"
	PeerID    string `json:"peer_id"`
	AgentName string `json:"agent_name"`
	Motto     string `json:"motto"`
}

// startGossipHandlers subscribes to knowledge and topic GossipSub topics and processes incoming messages.
func (d *Daemon) startGossipHandlers(ctx context.Context) {
	// Join and listen on /clawnet/knowledge
	sub, err := d.Node.JoinTopic(KnowledgeTopic)
	if err != nil {
		fmt.Printf("warning: could not join knowledge topic: %v\n", err)
		return
	}
	go d.handleKnowledgeSub(ctx, sub)

	// Join and listen on /clawnet/motto
	mottoSub, err := d.Node.JoinTopic(MottoTopic)
	if err != nil {
		fmt.Printf("warning: could not join motto topic: %v\n", err)
	} else {
		go d.handleMottoSub(ctx, mottoSub)
	}

	// Publish own motto on start if set
	if d.Profile != nil && d.Profile.Motto != "" {
		go func() {
			time.Sleep(3 * time.Second) // wait for peer connections
			d.publishMotto(ctx, d.Profile.Motto)
		}()
	}

	// Start traffic byte counter
	go d.trackTraffic(ctx)
}

func (d *Daemon) handleMottoSub(ctx context.Context, sub *pubsub.Subscription) {
	for {
		msg, err := sub.Next(ctx)
		if err != nil {
			return
		}
		if msg.ReceivedFrom == d.Node.PeerID() {
			continue
		}
		var gm GossipMottoMsg
		if err := json.Unmarshal(msg.Data, &gm); err != nil {
			continue
		}
		if gm.Type == "motto" && gm.PeerID != "" {
			d.PeerMottos.Store(gm.PeerID, gm.Motto)
		}
	}
}

func (d *Daemon) publishMotto(ctx context.Context, motto string) {
	gm := GossipMottoMsg{
		Type:      "motto",
		PeerID:    d.Node.PeerID().String(),
		AgentName: d.Profile.AgentName,
		Motto:     motto,
	}
	data, err := json.Marshal(gm)
	if err != nil {
		return
	}
	d.Node.Publish(ctx, MottoTopic, data)
}

// trackTraffic periodically samples libp2p bandwidth stats.
func (d *Daemon) trackTraffic(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if bw := d.Node.BwCounter; bw != nil {
				stats := bw.GetBandwidthTotals()
				d.rxBytes.Store(uint64(stats.TotalIn))
				d.txBytes.Store(uint64(stats.TotalOut))
			}
		}
	}
}

func (d *Daemon) handleKnowledgeSub(ctx context.Context, sub *pubsub.Subscription) {
	for {
		msg, err := sub.Next(ctx)
		if err != nil {
			return
		}
		// Skip messages from ourselves
		if msg.ReceivedFrom == d.Node.PeerID() {
			continue
		}

		var gm GossipKnowledgeMsg
		if err := json.Unmarshal(msg.Data, &gm); err != nil {
			continue
		}

		switch gm.Type {
		case "knowledge":
			if gm.Entry != nil {
				d.Store.InsertKnowledge(gm.Entry)
			}
		case "react":
			if gm.React != nil {
				d.Store.ReactKnowledge(gm.React.KnowledgeID, gm.React.PeerID, gm.React.Reaction)
			}
		case "reply":
			if gm.Reply != nil {
				d.Store.InsertReply(gm.Reply)
			}
		}
	}
}

// publishKnowledge publishes a new knowledge entry to the network.
func (d *Daemon) publishKnowledge(ctx context.Context, e *store.KnowledgeEntry) error {
	if e.ID == "" {
		e.ID = uuid.New().String()
	}
	if e.CreatedAt == "" {
		e.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	e.AuthorID = d.Node.PeerID().String()
	e.AuthorName = d.Profile.AgentName

	// Store locally
	if err := d.Store.InsertKnowledge(e); err != nil {
		return fmt.Errorf("store knowledge: %w", err)
	}

	// Publish to network
	msg := GossipKnowledgeMsg{Type: "knowledge", Entry: e}
	data, _ := json.Marshal(msg)
	return d.Node.Publish(ctx, KnowledgeTopic, data)
}

// publishReact publishes a reaction to the network.
func (d *Daemon) publishReact(ctx context.Context, knowledgeID, reaction string) error {
	peerID := d.Node.PeerID().String()
	if err := d.Store.ReactKnowledge(knowledgeID, peerID, reaction); err != nil {
		return err
	}
	msg := GossipKnowledgeMsg{
		Type: "react",
		React: &GossipReact{
			KnowledgeID: knowledgeID,
			PeerID:      peerID,
			Reaction:    reaction,
		},
	}
	data, _ := json.Marshal(msg)
	return d.Node.Publish(ctx, KnowledgeTopic, data)
}

// publishReply publishes a reply to the network.
func (d *Daemon) publishReply(ctx context.Context, knowledgeID, body string) error {
	reply := &store.KnowledgeReply{
		ID:          uuid.New().String(),
		KnowledgeID: knowledgeID,
		AuthorID:    d.Node.PeerID().String(),
		AuthorName:  d.Profile.AgentName,
		Body:        body,
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
	}
	if err := d.Store.InsertReply(reply); err != nil {
		return err
	}
	msg := GossipKnowledgeMsg{Type: "reply", Reply: reply}
	data, _ := json.Marshal(msg)
	return d.Node.Publish(ctx, KnowledgeTopic, data)
}

// joinTopicRoom creates/joins a topic room and starts listening on GossipSub.
func (d *Daemon) joinTopicRoom(ctx context.Context, room *store.TopicRoom) error {
	room.Joined = true
	if err := d.Store.InsertTopic(room); err != nil {
		return err
	}

	gsTopicName := TopicPrefix + room.Name
	sub, err := d.Node.JoinTopic(gsTopicName)
	if err != nil {
		return fmt.Errorf("join gossipsub topic: %w", err)
	}

	// Listen for messages from the network
	go d.handleTopicMessages(ctx, room.Name, sub)
	// Broadcast creation so other nodes discover it
	msg := GossipTopicMsg{Type: "create", Room: room}
	data, _ := json.Marshal(msg)
	d.Node.Publish(ctx, gsTopicName, data)

	return nil
}

func (d *Daemon) handleTopicMessages(ctx context.Context, roomName string, sub *pubsub.Subscription) {
	for {
		msg, err := sub.Next(ctx)
		if err != nil {
			return
		}
		if msg.ReceivedFrom == d.Node.PeerID() {
			continue
		}

		var gm GossipTopicMsg
		if err := json.Unmarshal(msg.Data, &gm); err != nil {
			continue
		}

		switch gm.Type {
		case "message":
			if gm.Message != nil {
				d.Store.InsertTopicMessage(gm.Message)
			}
		case "create":
			if gm.Room != nil {
				gm.Room.Joined = false // we haven't explicitly joined
				d.Store.InsertTopic(gm.Room)
			}
		}
	}
}

// publishTopicMessage sends a message to a topic room.
func (d *Daemon) publishTopicMessage(ctx context.Context, topicName, body string) error {
	m := &store.TopicMessage{
		ID:         uuid.New().String(),
		TopicName:  topicName,
		AuthorID:   d.Node.PeerID().String(),
		AuthorName: d.Profile.AgentName,
		Body:       body,
		CreatedAt:  time.Now().UTC().Format(time.RFC3339),
	}
	if err := d.Store.InsertTopicMessage(m); err != nil {
		return err
	}

	gsTopicName := TopicPrefix + topicName
	msg := GossipTopicMsg{Type: "message", Message: m}
	data, _ := json.Marshal(msg)
	return d.Node.Publish(ctx, gsTopicName, data)
}
