package daemon

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
)

// BundleProtocol is the libp2p stream protocol for P2P bundle transfer.
const BundleProtocol = protocol.ID("/clawnet/bundle/1.0.0")

// Bundle wire format:
//
//   Request:  [1 byte cmd] [2 byte LE task-id-len] [task-id bytes]
//   Response: [1 byte status] [4 byte LE bundle-len] [bundle bytes] [32 byte sha256]
//
// Commands: 0x01 = fetch-by-task
// Status:   0x00 = OK, 0x01 = not-found, 0x02 = error

const (
	bundleCmdFetch    = 0x01
	bundleStatusOK    = 0x00
	bundleStatusNoHit = 0x01
	bundleStatusErr   = 0x02
	bundleMaxSize     = 50 << 20 // 50 MB
)

// registerBundleHandler sets up the libp2p stream handler for incoming
// bundle fetch requests from other peers.
func (d *Daemon) registerBundleHandler() {
	d.Node.Host.SetStreamHandler(BundleProtocol, func(s network.Stream) {
		defer s.Close()
		_ = s.SetDeadline(time.Now().Add(60 * time.Second))

		reader := bufio.NewReader(s)

		cmd, err := reader.ReadByte()
		if err != nil || cmd != bundleCmdFetch {
			s.Write([]byte{bundleStatusErr})
			return
		}

		// Read task ID length (2 bytes LE)
		var idLen uint16
		if err := binary.Read(reader, binary.LittleEndian, &idLen); err != nil || idLen == 0 || idLen > 512 {
			s.Write([]byte{bundleStatusErr})
			return
		}

		idBuf := make([]byte, idLen)
		if _, err := io.ReadFull(reader, idBuf); err != nil {
			s.Write([]byte{bundleStatusErr})
			return
		}
		taskID := string(idBuf)

		bundle, _, err := d.Store.GetTaskBundle(taskID)
		if err != nil || bundle == nil {
			s.Write([]byte{bundleStatusNoHit})
			return
		}

		// Write response: status + length + bundle + sha256
		s.Write([]byte{bundleStatusOK})

		var bLen [4]byte
		binary.LittleEndian.PutUint32(bLen[:], uint32(len(bundle)))
		s.Write(bLen[:])
		s.Write(bundle)

		hash := sha256.Sum256(bundle)
		s.Write(hash[:])
	})
}

// fetchBundleFromPeer attempts to download a task bundle from a specific peer
// via the P2P bundle stream protocol.
func (d *Daemon) fetchBundleFromPeer(ctx context.Context, pid peer.ID, taskID string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	s, err := d.Node.Host.NewStream(ctx, pid, BundleProtocol)
	if err != nil {
		return nil, fmt.Errorf("open bundle stream: %w", err)
	}
	defer s.Close()
	_ = s.SetDeadline(time.Now().Add(30 * time.Second))

	// Send request: cmd + id-len + id
	idBytes := []byte(taskID)
	header := make([]byte, 3+len(idBytes))
	header[0] = bundleCmdFetch
	binary.LittleEndian.PutUint16(header[1:3], uint16(len(idBytes)))
	copy(header[3:], idBytes)
	if _, err := s.Write(header); err != nil {
		return nil, fmt.Errorf("write request: %w", err)
	}

	reader := bufio.NewReader(s)
	status, err := reader.ReadByte()
	if err != nil {
		return nil, fmt.Errorf("read status: %w", err)
	}
	if status == bundleStatusNoHit {
		return nil, nil // peer doesn't have it
	}
	if status != bundleStatusOK {
		return nil, fmt.Errorf("peer returned error status %d", status)
	}

	// Read bundle length
	var bLen uint32
	if err := binary.Read(reader, binary.LittleEndian, &bLen); err != nil {
		return nil, fmt.Errorf("read length: %w", err)
	}
	if bLen > bundleMaxSize {
		return nil, fmt.Errorf("bundle too large: %d bytes", bLen)
	}

	bundle := make([]byte, bLen)
	if _, err := io.ReadFull(reader, bundle); err != nil {
		return nil, fmt.Errorf("read bundle: %w", err)
	}

	// Read and verify SHA-256
	var remoteHash [32]byte
	if _, err := io.ReadFull(reader, remoteHash[:]); err != nil {
		return nil, fmt.Errorf("read hash: %w", err)
	}
	localHash := sha256.Sum256(bundle)
	if localHash != remoteHash {
		return nil, fmt.Errorf("hash mismatch: expected %x got %x", remoteHash, localHash)
	}

	return bundle, nil
}

// fetchBundleFromNetwork tries all connected peers until a bundle is found.
// On success it caches the bundle locally and returns it.
func (d *Daemon) fetchBundleFromNetwork(ctx context.Context, taskID string) ([]byte, string, error) {
	peers := d.Node.ConnectedPeers()
	for _, pid := range peers {
		bundle, err := d.fetchBundleFromPeer(ctx, pid, taskID)
		if err != nil {
			continue
		}
		if bundle == nil {
			continue
		}
		// Cache locally
		hash := fmt.Sprintf("%x", sha256.Sum256(bundle))
		if err := d.Store.InsertTaskBundle(taskID, bundle, hash); err != nil {
			fmt.Printf("bundle-p2p: cache error: %v\n", err)
		}
		return bundle, hash, nil
	}
	return nil, "", fmt.Errorf("bundle not found on any connected peer")
}
