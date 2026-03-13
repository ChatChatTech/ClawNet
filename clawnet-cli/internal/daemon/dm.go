package daemon

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/ChatChatTech/ClawNet/clawnet-cli/internal/store"
	"github.com/google/uuid"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
)

const DMProtocol = protocol.ID("/clawnet/dm/1.0.0")

// DMWireMsg is the wire format for direct messages.
type DMWireMsg struct {
	ID        string `json:"id"`
	Body      string `json:"body"`
	CreatedAt string `json:"created_at"`
	SenderName string `json:"sender_name"`
}

// registerDMHandler sets up the libp2p stream handler for incoming DMs.
func (d *Daemon) registerDMHandler() {
	d.Node.Host.SetStreamHandler(DMProtocol, func(s network.Stream) {
		defer s.Close()
		remotePeer := s.Conn().RemotePeer().String()
		reader := bufio.NewReader(s)

		for {
			line, err := reader.ReadBytes('\n')
			if err != nil {
				if err != io.EOF {
					fmt.Printf("DM stream error from %s: %v\n", remotePeer[:16], err)
				}
				return
			}
			var wm DMWireMsg
			if err := json.Unmarshal(line, &wm); err != nil {
				continue
			}
			dm := &store.DirectMessage{
				ID:        wm.ID,
				PeerID:    remotePeer,
				Direction: "received",
				Body:      wm.Body,
				CreatedAt: wm.CreatedAt,
			}
			d.Store.InsertDM(dm)
		}
	})
}

// sendDM sends a direct message to a peer via a libp2p stream.
func (d *Daemon) sendDM(ctx context.Context, peerIDStr, body string) error {
	pid, err := peer.Decode(peerIDStr)
	if err != nil {
		return fmt.Errorf("invalid peer ID: %w", err)
	}

	s, err := d.Node.Host.NewStream(ctx, pid, DMProtocol)
	if err != nil {
		return fmt.Errorf("open stream to %s: %w", peerIDStr[:16], err)
	}
	defer s.Close()

	wm := DMWireMsg{
		ID:         uuid.New().String(),
		Body:       body,
		CreatedAt:  time.Now().UTC().Format(time.RFC3339),
		SenderName: d.Profile.AgentName,
	}
	data, _ := json.Marshal(wm)
	data = append(data, '\n')

	if _, err := s.Write(data); err != nil {
		return fmt.Errorf("write DM: %w", err)
	}

	// Store locally as sent
	dm := &store.DirectMessage{
		ID:        wm.ID,
		PeerID:    peerIDStr,
		Direction: "sent",
		Body:      body,
		CreatedAt: wm.CreatedAt,
		Read:      true,
	}
	return d.Store.InsertDM(dm)
}
