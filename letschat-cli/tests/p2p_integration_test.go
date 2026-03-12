package p2p_test

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/crypto"
	"letchat-cli/internal/config"
	"letchat-cli/internal/p2p"
)

func makeTestConfig(tcpPort, quicPort, apiPort int) *config.Config {
	cfg := config.DefaultConfig()
	cfg.ListenAddrs = []string{
		fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", tcpPort),
		fmt.Sprintf("/ip4/127.0.0.1/udp/%d/quic-v1", quicPort),
	}
	cfg.WebUIPort = apiPort
	cfg.RelayEnabled = true
	cfg.BootstrapPeers = []string{}
	return cfg
}

func generateKey(t *testing.T) crypto.PrivKey {
	t.Helper()
	priv, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	return priv
}

// TestTwoNodesConnect tests that two nodes on localhost can discover each other
// via mDNS and establish a connection.
func TestTwoNodesConnect(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	key1 := generateKey(t)
	key2 := generateKey(t)

	cfg1 := makeTestConfig(14001, 14001, 13847)
	cfg2 := makeTestConfig(14002, 14002, 13848)

	// Start node 1
	node1, err := p2p.NewNode(ctx, key1, cfg1)
	if err != nil {
		t.Fatalf("start node1: %v", err)
	}
	defer node1.Close()

	t.Logf("Node 1 Peer ID: %s", node1.PeerID())
	for _, addr := range node1.Addrs() {
		t.Logf("Node 1 addr: %s", addr)
	}

	// Start node 2, with node 1 as bootstrap peer
	addr1 := fmt.Sprintf("%s/p2p/%s", node1.Addrs()[0], node1.PeerID())
	cfg2.BootstrapPeers = []string{addr1}

	node2, err := p2p.NewNode(ctx, key2, cfg2)
	if err != nil {
		t.Fatalf("start node2: %v", err)
	}
	defer node2.Close()

	t.Logf("Node 2 Peer ID: %s", node2.PeerID())

	// Wait for nodes to discover each other
	connected := false
	for i := 0; i < 30; i++ {
		peers1 := node1.ConnectedPeers()
		peers2 := node2.ConnectedPeers()
		t.Logf("tick %d: node1 peers=%d, node2 peers=%d", i, len(peers1), len(peers2))
		if len(peers1) > 0 && len(peers2) > 0 {
			connected = true
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	if !connected {
		t.Fatal("nodes did not connect within timeout")
	}

	// Verify mutual connection
	peers1 := node1.ConnectedPeers()
	peers2 := node2.ConnectedPeers()

	found1to2 := false
	for _, p := range peers1 {
		if p == node2.PeerID() {
			found1to2 = true
			break
		}
	}
	found2to1 := false
	for _, p := range peers2 {
		if p == node1.PeerID() {
			found2to1 = true
			break
		}
	}

	if !found1to2 {
		t.Error("node1 does not see node2 in peers")
	}
	if !found2to1 {
		t.Error("node2 does not see node1 in peers")
	}

	t.Logf("SUCCESS: node1 peers=%d, node2 peers=%d", len(peers1), len(peers2))
}

// TestGossipSubMessaging tests that two connected nodes can exchange messages
// via GossipSub topics.
func TestGossipSubMessaging(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	key1 := generateKey(t)
	key2 := generateKey(t)

	cfg1 := makeTestConfig(15001, 15001, 15847)
	cfg2 := makeTestConfig(15002, 15002, 15848)

	// Start both nodes
	node1, err := p2p.NewNode(ctx, key1, cfg1)
	if err != nil {
		t.Fatalf("start node1: %v", err)
	}
	defer node1.Close()

	addr1 := fmt.Sprintf("%s/p2p/%s", node1.Addrs()[0], node1.PeerID())
	cfg2.BootstrapPeers = []string{addr1}

	node2, err := p2p.NewNode(ctx, key2, cfg2)
	if err != nil {
		t.Fatalf("start node2: %v", err)
	}
	defer node2.Close()

	// Wait for connection
	for i := 0; i < 20; i++ {
		if len(node1.ConnectedPeers()) > 0 && len(node2.ConnectedPeers()) > 0 {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if len(node1.ConnectedPeers()) == 0 {
		t.Fatal("nodes did not connect")
	}

	// Both subscribe to the test topic
	topicName := "/letchat/test-gossip"
	sub2, err := node2.JoinTopic(topicName)
	if err != nil {
		t.Fatalf("node2 join topic: %v", err)
	}

	_, err = node1.JoinTopic(topicName)
	if err != nil {
		t.Fatalf("node1 join topic: %v", err)
	}

	// Wait for GossipSub mesh to form
	time.Sleep(2 * time.Second)

	// Node 1 publishes a message
	testMsg := map[string]string{
		"type": "test",
		"body": "hello from node1",
	}
	data, _ := json.Marshal(testMsg)

	if err := node1.Publish(ctx, topicName, data); err != nil {
		t.Fatalf("node1 publish: %v", err)
	}

	// Node 2 should receive the message
	received := make(chan bool, 1)
	go func() {
		msg, err := sub2.Next(ctx)
		if err != nil {
			return
		}
		var decoded map[string]string
		if err := json.Unmarshal(msg.Data, &decoded); err != nil {
			return
		}
		if decoded["body"] == "hello from node1" {
			received <- true
		}
	}()

	select {
	case <-received:
		t.Log("SUCCESS: node2 received message from node1 via GossipSub")
	case <-time.After(10 * time.Second):
		t.Fatal("node2 did not receive message within timeout")
	}
}
