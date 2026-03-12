package p2p_test

import (
	"os"
	"testing"

	"github.com/ChatChatTech/letschat/letschat-cli/internal/store"
)

func TestStoreKnowledgeCRUD(t *testing.T) {
	dir, err := os.MkdirTemp("", "letchat-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	db, err := store.Open(dir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()

	// Insert knowledge
	entry := &store.KnowledgeEntry{
		ID:         "k1",
		AuthorID:   "peer-A",
		AuthorName: "Alice",
		Title:      "Go Concurrency Patterns",
		Body:       "Goroutines and channels are the building blocks of concurrent Go programs.",
		Domains:    []string{"go", "concurrency"},
		CreatedAt:  "2026-03-13T00:00:00Z",
	}
	if err := db.InsertKnowledge(entry); err != nil {
		t.Fatalf("insert: %v", err)
	}

	// Get by ID
	got, err := db.GetKnowledge("k1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Title != "Go Concurrency Patterns" {
		t.Errorf("title = %q, want %q", got.Title, "Go Concurrency Patterns")
	}
	if len(got.Domains) != 2 || got.Domains[0] != "go" {
		t.Errorf("domains = %v, want [go concurrency]", got.Domains)
	}

	// List
	entries, err := db.ListKnowledge("", 10, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("list count = %d, want 1", len(entries))
	}

	// Filter by domain
	entries, err = db.ListKnowledge("go", 10, 0)
	if err != nil {
		t.Fatalf("list filter: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("filtered count = %d, want 1", len(entries))
	}

	// Full-text search
	results, err := db.SearchKnowledge(store.EscapeFTS5("goroutines channels"), 10)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("search count = %d, want 1", len(results))
	}

	// React
	if err := db.ReactKnowledge("k1", "peer-B", "upvote"); err != nil {
		t.Fatalf("react: %v", err)
	}
	got, _ = db.GetKnowledge("k1")
	if got.Upvotes != 1 {
		t.Errorf("upvotes = %d, want 1", got.Upvotes)
	}

	// Reply
	reply := &store.KnowledgeReply{
		ID:          "r1",
		KnowledgeID: "k1",
		AuthorID:    "peer-B",
		AuthorName:  "Bob",
		Body:        "Great article!",
		CreatedAt:   "2026-03-13T01:00:00Z",
	}
	if err := db.InsertReply(reply); err != nil {
		t.Fatalf("reply: %v", err)
	}
	replies, err := db.ListReplies("k1", 10)
	if err != nil {
		t.Fatalf("list replies: %v", err)
	}
	if len(replies) != 1 || replies[0].Body != "Great article!" {
		t.Errorf("replies = %v", replies)
	}

	t.Log("SUCCESS: knowledge CRUD + FTS5 + reactions + replies all working")
}

func TestStoreTopicsCRUD(t *testing.T) {
	dir, err := os.MkdirTemp("", "letchat-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	db, err := store.Open(dir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()

	// Create topic
	room := &store.TopicRoom{
		Name:        "ml-papers",
		Description: "Machine learning papers discussion",
		CreatorID:   "peer-A",
		CreatedAt:   "2026-03-13T00:00:00Z",
		Joined:      true,
	}
	if err := db.InsertTopic(room); err != nil {
		t.Fatalf("insert topic: %v", err)
	}

	// List
	topics, err := db.ListTopics()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(topics) != 1 || topics[0].Name != "ml-papers" {
		t.Fatalf("topics = %v", topics)
	}

	// Post message
	msg := &store.TopicMessage{
		ID:         "m1",
		TopicName:  "ml-papers",
		AuthorID:   "peer-A",
		AuthorName: "Alice",
		Body:       "Has anyone read the new transformer paper?",
		CreatedAt:  "2026-03-13T00:01:00Z",
	}
	if err := db.InsertTopicMessage(msg); err != nil {
		t.Fatalf("post: %v", err)
	}

	msgs, err := db.ListTopicMessages("ml-papers", 10, 0)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(msgs) != 1 || msgs[0].Body != "Has anyone read the new transformer paper?" {
		t.Fatalf("msgs = %v", msgs)
	}

	// Leave
	if err := db.SetTopicJoined("ml-papers", false); err != nil {
		t.Fatalf("leave: %v", err)
	}
	topic, _ := db.GetTopic("ml-papers")
	if topic.Joined {
		t.Error("topic should not be joined")
	}

	t.Log("SUCCESS: topic rooms CRUD + messages working")
}

func TestStoreDMCRUD(t *testing.T) {
	dir, err := os.MkdirTemp("", "letchat-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	db, err := store.Open(dir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()

	// Send DM
	dm1 := &store.DirectMessage{
		ID:        "dm1",
		PeerID:    "peer-B",
		Direction: "sent",
		Body:      "Hello Bob!",
		CreatedAt: "2026-03-13T00:00:00Z",
		Read:      true,
	}
	if err := db.InsertDM(dm1); err != nil {
		t.Fatalf("send: %v", err)
	}

	// Receive DM
	dm2 := &store.DirectMessage{
		ID:        "dm2",
		PeerID:    "peer-B",
		Direction: "received",
		Body:      "Hi Alice!",
		CreatedAt: "2026-03-13T00:01:00Z",
		Read:      false,
	}
	if err := db.InsertDM(dm2); err != nil {
		t.Fatalf("receive: %v", err)
	}

	// Inbox
	inbox, err := db.ListDMInbox()
	if err != nil {
		t.Fatalf("inbox: %v", err)
	}
	if len(inbox) != 1 || inbox[0].Body != "Hi Alice!" {
		t.Fatalf("inbox = %v", inbox)
	}

	// Thread
	thread, err := db.ListDMThread("peer-B", 10, 0)
	if err != nil {
		t.Fatalf("thread: %v", err)
	}
	if len(thread) != 2 {
		t.Fatalf("thread count = %d, want 2", len(thread))
	}

	// Unread count
	count, err := db.UnreadDMCount()
	if err != nil {
		t.Fatalf("unread: %v", err)
	}
	if count != 1 {
		t.Errorf("unread = %d, want 1", count)
	}

	// Mark read
	if err := db.MarkDMRead("peer-B"); err != nil {
		t.Fatalf("mark read: %v", err)
	}
	count, _ = db.UnreadDMCount()
	if count != 0 {
		t.Errorf("unread after mark = %d, want 0", count)
	}

	t.Log("SUCCESS: DM CRUD + inbox + thread + read status working")
}
