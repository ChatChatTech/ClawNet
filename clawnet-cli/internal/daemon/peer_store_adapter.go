package daemon

import (
	"github.com/ChatChatTech/ClawNet/clawnet-cli/internal/matrix"
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

// matrixTokenAdapter bridges store.Store to matrix.TokenStore interface.
type matrixTokenAdapter struct {
	db *store.Store
}

func (a *matrixTokenAdapter) SaveMatrixTokens(tokens map[string]matrix.TokenEntry) error {
	st := make(map[string]store.MatrixToken, len(tokens))
	for hs, te := range tokens {
		st[hs] = store.MatrixToken{
			Homeserver:  hs,
			AccessToken: te.AccessToken,
			UserID:      te.UserID,
		}
	}
	return a.db.SaveMatrixTokens(st)
}

func (a *matrixTokenAdapter) LoadMatrixTokens() (map[string]matrix.TokenEntry, error) {
	loaded, err := a.db.LoadMatrixTokens()
	if err != nil {
		return nil, err
	}
	tokens := make(map[string]matrix.TokenEntry, len(loaded))
	for hs, mt := range loaded {
		tokens[hs] = matrix.TokenEntry{
			AccessToken: mt.AccessToken,
			UserID:      mt.UserID,
		}
	}
	return tokens, nil
}
