package p2p_test

import (
	"os"
	"testing"

	"github.com/ChatChatTech/ClawNet/clawnet-cli/internal/store"
)

func TestStoreKnowledgeCRUD(t *testing.T) {
	dir, err := os.MkdirTemp("", "clawnet-test-*")
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
	dir, err := os.MkdirTemp("", "clawnet-test-*")
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
	dir, err := os.MkdirTemp("", "clawnet-test-*")
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

func TestResumeAndTaskMatching(t *testing.T) {
	dir, err := os.MkdirTemp("", "clawnet-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	db, err := store.Open(dir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()

	// --- Resume CRUD ---

	// Insert resume for Agent A (data analyst)
	resumeA := &store.AgentResume{
		PeerID:      "peer-A",
		AgentName:   "DataBot",
		Skills:      `["data-analysis","python","statistics"]`,
		DataSources: `["market-data","financial-reports"]`,
		Description: "Specialized in quantitative data analysis",
	}
	if err := db.UpsertResume(resumeA); err != nil {
		t.Fatalf("upsert resume A: %v", err)
	}

	// Insert resume for Agent B (web developer)
	resumeB := &store.AgentResume{
		PeerID:      "peer-B",
		AgentName:   "WebDev",
		Skills:      `["web-scraping","javascript","python"]`,
		DataSources: `["web-pages"]`,
		Description: "Full-stack web development agent",
	}
	if err := db.UpsertResume(resumeB); err != nil {
		t.Fatalf("upsert resume B: %v", err)
	}

	// Insert resume for Agent C (no overlap)
	resumeC := &store.AgentResume{
		PeerID:      "peer-C",
		AgentName:   "ArtBot",
		Skills:      `["image-generation","creative-writing"]`,
		DataSources: `[]`,
		Description: "Creative content generation",
	}
	if err := db.UpsertResume(resumeC); err != nil {
		t.Fatalf("upsert resume C: %v", err)
	}

	// Get resume
	got, err := db.GetResume("peer-A")
	if err != nil {
		t.Fatalf("get resume: %v", err)
	}
	if got == nil || got.AgentName != "DataBot" {
		t.Fatalf("resume A: got %v", got)
	}

	// List resumes
	all, err := db.ListResumes(50)
	if err != nil {
		t.Fatalf("list resumes: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("expected 3 resumes, got %d", len(all))
	}

	// Update resume
	resumeA.Skills = `["data-analysis","python","statistics","machine-learning"]`
	if err := db.UpsertResume(resumeA); err != nil {
		t.Fatalf("update resume A: %v", err)
	}
	got, _ = db.GetResume("peer-A")
	if got.Skills != `["data-analysis","python","statistics","machine-learning"]` {
		t.Errorf("updated skills = %q", got.Skills)
	}

	t.Log("OK: Resume CRUD working")

	// --- Task with tags ---

	// Ensure credit accounts exist
	db.EnsureCreditAccount("peer-X", 100)

	// Create a task requiring data-analysis + python
	task1 := &store.Task{
		ID:       "task-1",
		AuthorID: "peer-X",
		Title:    "Analyze Q1 sales data",
		Tags:     `["data-analysis","python"]`,
		Deadline: "2026-03-20T00:00:00Z",
		Reward:   15,
		Status:   "open",
	}
	if err := db.InsertTask(task1); err != nil {
		t.Fatalf("insert task: %v", err)
	}

	// Verify tags stored
	fetched, err := db.GetTask("task-1")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if fetched.Tags != `["data-analysis","python"]` {
		t.Errorf("task tags = %q", fetched.Tags)
	}
	if fetched.Deadline != "2026-03-20T00:00:00Z" {
		t.Errorf("task deadline = %q", fetched.Deadline)
	}

	t.Log("OK: Task template with tags + deadline working")

	// --- Matching: agents for task ---

	// Set up reputation for peer-A to be higher
	db.RecalcReputation("peer-A")
	db.RecalcReputation("peer-B")
	db.RecalcReputation("peer-C")

	matches, err := db.MatchAgentsForTask("task-1")
	if err != nil {
		t.Fatalf("match agents: %v", err)
	}
	if len(matches) == 0 {
		t.Fatal("expected at least one match")
	}

	// peer-A has data-analysis + python (2/2 match = 1.0)
	// peer-B has python (1/2 match = 0.5)
	// peer-C has no overlap (should not appear)
	foundA, foundB, foundC := false, false, false
	for _, m := range matches {
		switch m.PeerID {
		case "peer-A":
			foundA = true
			if m.MatchScore < 0.99 {
				t.Errorf("peer-A match score = %.2f, want 1.0", m.MatchScore)
			}
		case "peer-B":
			foundB = true
			if m.MatchScore < 0.49 || m.MatchScore > 0.51 {
				t.Errorf("peer-B match score = %.2f, want 0.5", m.MatchScore)
			}
		case "peer-C":
			foundC = true
		}
	}
	if !foundA {
		t.Error("peer-A should match (data-analysis + python)")
	}
	if !foundB {
		t.Error("peer-B should match (python)")
	}
	if foundC {
		t.Error("peer-C should NOT match (no overlap)")
	}

	// peer-A should rank higher than peer-B (better match score)
	if len(matches) >= 2 && matches[0].PeerID != "peer-A" {
		t.Errorf("expected peer-A to rank first, got %s", matches[0].PeerID)
	}

	t.Log("OK: Agent-for-task matching working (ranked by skill overlap)")

	// --- Matching: tasks for agent ---

	// Create another task peer-A can't do
	task2 := &store.Task{
		ID:       "task-2",
		AuthorID: "peer-X",
		Title:    "Generate product images",
		Tags:     `["image-generation"]`,
		Reward:   10,
		Status:   "open",
	}
	db.InsertTask(task2)

	// peer-A should see task-1 (matching) and task-2 should rank lower or not appear
	tasksForA, err := db.MatchTasksForAgent("peer-A")
	if err != nil {
		t.Fatalf("match tasks: %v", err)
	}
	if len(tasksForA) == 0 {
		t.Fatal("expected at least one task for peer-A")
	}
	if tasksForA[0].ID != "task-1" {
		t.Errorf("expected task-1 to rank first for peer-A, got %s", tasksForA[0].ID)
	}

	// peer-C should see task-2 (image-generation matches)
	tasksForC, err := db.MatchTasksForAgent("peer-C")
	if err != nil {
		t.Fatalf("match tasks for C: %v", err)
	}
	foundTask2 := false
	for _, task := range tasksForC {
		if task.ID == "task-2" {
			foundTask2 = true
			break
		}
	}
	if !foundTask2 {
		t.Error("peer-C should see task-2 (image-generation)")
	}

	t.Log("OK: Tasks-for-agent matching working")
	t.Log("SUCCESS: Resume + Task Template + Supply-Demand Matching all verified")
}
