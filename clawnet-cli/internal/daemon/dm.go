package daemon

import (
	"bufio"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"io"
	"time"

	cryptoe "github.com/ChatChatTech/ClawNet/clawnet-cli/internal/crypto"
	"github.com/ChatChatTech/ClawNet/clawnet-cli/internal/overlay"
	"github.com/ChatChatTech/ClawNet/clawnet-cli/internal/store"
	"github.com/google/uuid"
	libcrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
)

const DMProtocol = protocol.ID("/clawnet/dm/1.0.0")

// DMWireMsg is the wire format for direct messages.
type DMWireMsg struct {
	ID         string `json:"id"`
	Body       string `json:"body"`
	CreatedAt  string `json:"created_at"`
	SenderName string `json:"sender_name"`
}

// registerDMHandler sets up the libp2p stream handler for incoming DMs.
func (d *Daemon) registerDMHandler() {
	d.Node.Host.SetStreamHandler(DMProtocol, func(s network.Stream) {
		defer s.Close()
		remotePeerID := s.Conn().RemotePeer()
		remotePeer := remotePeerID.String()
		reader := bufio.NewReader(s)

		for {
			line, err := reader.ReadBytes('\n')
			if err != nil {
				if err != io.EOF {
					fmt.Printf("DM stream error from %s: %v\n", remotePeer[:16], err)
				}
				return
			}

			// Try to decrypt if E2E is available and the message is encrypted
			body := ""
			if d.Crypto != nil && cryptoe.IsEncrypted(line) {
				plaintext, err := d.Crypto.Decrypt(remotePeerID, line)
				if err != nil {
					fmt.Printf("DM decrypt error from %s: %v\n", remotePeer[:16], err)
					continue
				}
				// plaintext is the original DMWireMsg JSON
				var wm DMWireMsg
				if err := json.Unmarshal(plaintext, &wm); err != nil {
					continue
				}
				body = wm.Body
				dm := &store.DirectMessage{
					ID:        wm.ID,
					PeerID:    remotePeer,
					Direction: "received",
					Body:      body,
					CreatedAt: wm.CreatedAt,
				}
				d.Store.InsertDM(dm)
			} else {
				// Unencrypted (backward compatible)
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
		}
	})
}

// sendDM sends a direct message to a peer via a libp2p stream.
// If libp2p fails and the overlay transport is enabled, falls back
// to sending via the Ironwood overlay network.
func (d *Daemon) sendDM(ctx context.Context, peerIDStr, body string) error {
	pid, err := peer.Decode(peerIDStr)
	if err != nil {
		return fmt.Errorf("invalid peer ID: %w", err)
	}

	wm := DMWireMsg{
		ID:         uuid.New().String(),
		Body:       body,
		CreatedAt:  time.Now().UTC().Format(time.RFC3339),
		SenderName: d.Profile.AgentName,
	}
	wmData, _ := json.Marshal(wm)

	// Try libp2p first
	libp2pErr := d.sendDMViaLibp2p(ctx, pid, wmData)
	if libp2pErr == nil {
		dm := &store.DirectMessage{
			ID: wm.ID, PeerID: peerIDStr, Direction: "sent",
			Body: body, CreatedAt: wm.CreatedAt, Read: true,
		}
		return d.Store.InsertDM(dm)
	}

	// Fallback to overlay if available
	if d.Overlay != nil {
		fmt.Printf("[dm] libp2p failed (%v), trying overlay fallback\n", libp2pErr)
		if err := d.sendDMViaOverlay(ctx, pid, wmData); err != nil {
			return fmt.Errorf("both libp2p and overlay failed: libp2p=%v, overlay=%w", libp2pErr, err)
		}
		dm := &store.DirectMessage{
			ID: wm.ID, PeerID: peerIDStr, Direction: "sent",
			Body: body, CreatedAt: wm.CreatedAt, Read: true,
		}
		return d.Store.InsertDM(dm)
	}

	return fmt.Errorf("open stream to %s: %w", peerIDStr[:16], libp2pErr)
}

// sendDMViaLibp2p sends the DM payload over a libp2p stream.
func (d *Daemon) sendDMViaLibp2p(ctx context.Context, pid peer.ID, wmData []byte) error {
	s, err := d.Node.Host.NewStream(ctx, pid, DMProtocol)
	if err != nil {
		return err
	}
	defer s.Close()

	var data []byte
	if d.Crypto != nil {
		encrypted, err := d.Crypto.Encrypt(pid, wmData)
		if err != nil {
			fmt.Printf("[crypto] encrypt failed, sending plaintext: %v\n", err)
			data = wmData
		} else {
			data = encrypted
		}
	} else {
		data = wmData
	}
	data = append(data, '\n')

	_, err = s.Write(data)
	return err
}

// sendDMViaOverlay sends the DM payload through the Ironwood overlay network.
func (d *Daemon) sendDMViaOverlay(ctx context.Context, pid peer.ID, wmData []byte) error {
	// Prepend MsgTypeDM byte so the receiver can distinguish DM from other overlay traffic
	payload := make([]byte, 1+len(wmData))
	payload[0] = overlay.MsgTypeDM
	copy(payload[1:], wmData)
	return d.Overlay.Send(ctx, pid, payload)
}

// registerOverlayDMHandler sets up the handler for DMs received via overlay.
func (d *Daemon) registerOverlayDMHandler() {
	if d.Overlay == nil {
		return
	}
	d.Overlay.SetMessageHandler(func(from ed25519.PublicKey, data []byte) {
		if len(data) < 2 || data[0] != overlay.MsgTypeDM {
			return // Not a DM message
		}
		dmData := data[1:]

		// Convert Ed25519 pubkey to peer ID
		libPub, err := libcrypto.UnmarshalEd25519PublicKey(from)
		if err != nil {
			return
		}
		remotePeerID, err := peer.IDFromPublicKey(libPub)
		if err != nil {
			return
		}
		remotePeer := remotePeerID.String()

		var wm DMWireMsg
		if err := json.Unmarshal(dmData, &wm); err != nil {
			fmt.Printf("[dm/overlay] unmarshal error from %s: %v\n", remotePeer[:16], err)
			return
		}

		dm := &store.DirectMessage{
			ID:        wm.ID,
			PeerID:    remotePeer,
			Direction: "received",
			Body:      wm.Body,
			CreatedAt: wm.CreatedAt,
		}
		d.Store.InsertDM(dm)
		fmt.Printf("[dm/overlay] received DM from %s via overlay\n", remotePeer[:16])
	})
}
