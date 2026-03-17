package daemon

import (
	"github.com/ChatChatTech/ClawNet/clawnet-cli/internal/overlay"
	"github.com/ChatChatTech/ClawNet/clawnet-cli/internal/store"
)

// peerStoreAdapter bridges store.Store to overlay.PeerStore interface.
type peerStoreAdapter struct {
	db *store.Store
}

func (a *peerStoreAdapter) SaveOverlayPeers(peers map[string]*overlay.PeerState) error {
	sp := make(map[string]*store.OverlayPeer, len(peers))
	for addr, ps := range peers {
		sp[addr] = &store.OverlayPeer{
			Address:     ps.Address,
			Source:      ps.Source,
			Alive:       ps.Alive,
			LastSeen:    ps.LastSeen,
			LastAttempt: ps.LastAttempt,
			ConsecFails: ps.ConsecFails,
			TotalConns:  ps.TotalConns,
		}
	}
	return a.db.SaveOverlayPeers(sp)
}

func (a *peerStoreAdapter) LoadOverlayPeers() (map[string]*overlay.PeerState, error) {
	loaded, err := a.db.LoadOverlayPeers()
	if err != nil {
		return nil, err
	}
	peers := make(map[string]*overlay.PeerState, len(loaded))
	for addr, op := range loaded {
		peers[addr] = &overlay.PeerState{
			Address:     op.Address,
			Source:      op.Source,
			Alive:       op.Alive,
			LastSeen:    op.LastSeen,
			LastAttempt: op.LastAttempt,
			ConsecFails: op.ConsecFails,
			TotalConns:  op.TotalConns,
		}
	}
	return peers, nil
}
