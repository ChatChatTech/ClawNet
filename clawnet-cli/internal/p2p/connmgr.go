package p2p

import (
	connmgr "github.com/libp2p/go-libp2p/p2p/net/connmgr"
)

// NewConnManager creates a connection manager with reasonable bounds.
func NewConnManager(maxConns int) *connmgr.BasicConnMgr {
	low := maxConns / 2
	if low < 10 {
		low = 10
	}
	cm, _ := connmgr.NewConnManager(low, maxConns)
	return cm
}
