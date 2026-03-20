package daemon

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/ChatChatTech/ClawNet/clawnet-cli/internal/store"
	"github.com/google/uuid"
)

// A2A Agent Card — Google Agent-to-Agent protocol compatible
// See: https://google.github.io/A2A/

type A2AAgentCard struct {
	Name         string          `json:"name"`
	Description  string          `json:"description"`
	URL          string          `json:"url"`
	Version      string          `json:"version"`
	Protocol     string          `json:"protocol"`
	Provider     *A2AProvider    `json:"provider,omitempty"`
	Capabilities A2ACapabilities `json:"capabilities"`
	Skills       []A2ASkill      `json:"skills"`
	Auth         *A2AAuth        `json:"authentication,omitempty"`
}

type A2AProvider struct {
	Organization string `json:"organization"`
	URL          string `json:"url"`
}

type A2ACapabilities struct {
	Streaming              bool `json:"streaming"`
	PushNotifications      bool `json:"pushNotifications"`
	StateTransitionHistory bool `json:"stateTransitionHistory"`
}

type A2ASkill struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	Examples    []string `json:"examples,omitempty"`
}

type A2AAuth struct {
	Schemes []string `json:"schemes"`
}

func (d *Daemon) handleA2AAgentCard(w http.ResponseWriter, r *http.Request) {
	peerID := d.Node.PeerID().String()
	resume, _ := d.Store.GetResume(peerID)

	agentName := "ClawNet Agent"
	agentDesc := "A ClawNet P2P network agent"
	var skills []A2ASkill

	if d.Profile != nil && d.Profile.AgentName != "" {
		agentName = d.Profile.AgentName
	}

	if resume != nil {
		if resume.AgentName != "" {
			agentName = resume.AgentName
		}
		if resume.Description != "" {
			agentDesc = resume.Description
		}
		// Parse resume skills JSON array into A2A skills
		var skillList []string
		if json.Unmarshal([]byte(resume.Skills), &skillList) == nil {
			for i, s := range skillList {
				skills = append(skills, A2ASkill{
					ID:          fmt.Sprintf("skill-%d", i),
					Name:        s,
					Description: fmt.Sprintf("Capable of %s", s),
					Tags:        []string{s},
				})
			}
		}
	}

	// Always include ClawNet native capabilities as skills
	nativeSkills := []A2ASkill{
		{ID: "clawnet-knowledge-search", Name: "Knowledge Search", Description: "Search the ClawNet Knowledge Mesh for information", Tags: []string{"knowledge", "search", "rag"}},
		{ID: "clawnet-task-create", Name: "Task Creation", Description: "Create and publish tasks to the ClawNet Auction House", Tags: []string{"task", "auction", "delegation"}},
		{ID: "clawnet-task-execute", Name: "Task Execution", Description: "Claim and execute tasks from the network", Tags: []string{"task", "execution", "work"}},
		{ID: "clawnet-p2p-messaging", Name: "P2P Messaging", Description: "Send encrypted direct messages to other agents", Tags: []string{"messaging", "dm", "encrypted"}},
		{ID: "clawnet-knowledge-publish", Name: "Knowledge Publishing", Description: "Publish knowledge to the decentralized Knowledge Mesh", Tags: []string{"knowledge", "publish", "share"}},
	}
	skills = append(nativeSkills, skills...)

	card := A2AAgentCard{
		Name:        agentName,
		Description: agentDesc,
		URL:         "http://localhost:3998",
		Version:     Version,
		Protocol:    "a2a/0.3.0",
		Provider: &A2AProvider{
			Organization: "ClawNet",
			URL:          "https://clawnet.cc",
		},
		Capabilities: A2ACapabilities{
			Streaming:              false,
			PushNotifications:      false,
			StateTransitionHistory: true,
		},
		Skills: skills,
		Auth:   nil, // Local-only, no auth needed
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(card)
}

// A2A Task Bridge — receive A2A-format task requests and map to ClawNet tasks.

type A2ATaskRequest struct {
	ID      string     `json:"id"`
	Message A2AMessage `json:"message"`
}

type A2AMessage struct {
	Role  string    `json:"role"`
	Parts []A2APart `json:"parts"`
}

type A2APart struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

func (d *Daemon) handleA2ATaskSend(w http.ResponseWriter, r *http.Request) {
	var req A2ATaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid A2A request"}`, http.StatusBadRequest)
		return
	}

	// Extract text from message parts
	var parts []string
	for _, part := range req.Message.Parts {
		if part.Type == "text" && part.Text != "" {
			parts = append(parts, part.Text)
		}
	}
	taskDesc := strings.Join(parts, "\n")

	if taskDesc == "" {
		http.Error(w, `{"error":"empty task description"}`, http.StatusBadRequest)
		return
	}

	// Use request ID or generate one
	taskID := req.ID
	if taskID == "" {
		taskID = uuid.New().String()
	}

	// Build a ClawNet task from the A2A request
	t := &store.Task{
		ID:          taskID,
		Title:       truncate(taskDesc, 120),
		Description: taskDesc,
		Tags:        `["a2a"]`,
		Reward:      0, // A2A tasks are zero-reward help-wanted by default
		Mode:        "simple",
	}

	if err := d.publishTask(d.ctx, t); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	d.RecordEvent("a2a_task_received", d.Node.PeerID().String(), t.ID, fmt.Sprintf("A2A task received: %s", t.Title))

	resp := map[string]any{
		"id": t.ID,
		"status": map[string]string{
			"state":   "submitted",
			"message": "Task submitted to ClawNet Auction House",
		},
	}
	writeJSON(w, resp)
}

// truncate returns s truncated to maxLen characters.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
