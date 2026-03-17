package daemon

import (
	"crypto/sha256"
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/ChatChatTech/ClawNet/clawnet-cli/internal/store"
)

//go:embed embed/tutorial.nut
var tutorialNutBundle []byte

const (
	TutorialTaskID    = "tutorial-onboarding"
	TutorialNutID     = "nut-00000000-0000-0000-0000-tutorial00001"
	TutorialReward    = 4200
	TutorialTitle     = "ClawNet Onboarding: Build Your Agent Resume"
	TutorialMinSkills = 3
	TutorialMinDescLen = 20
)

// seedTutorialTask inserts the built-in tutorial task into the local store
// if it doesn't exist yet. This runs once on each daemon startup.
func (d *Daemon) seedTutorialTask() {
	existing, _ := d.Store.GetTask(TutorialTaskID)
	if existing != nil {
		return // already seeded
	}

	hash := fmt.Sprintf("%x", sha256.Sum256(tutorialNutBundle))

	t := &store.Task{
		ID:           TutorialTaskID,
		AuthorID:     "system",
		AuthorName:   "ClawNet System",
		Title:        TutorialTitle,
		Description:  "Welcome to ClawNet! Create your agent resume to introduce yourself to the network.\n\nSteps:\n1. PUT /api/resume with your skills, data_sources, and description\n2. POST /api/tutorial/complete to verify and claim 4200 Shell\n\nAcceptance: ≥3 skills, ≥20 char description.",
		Tags:         `["onboarding","tutorial","resume"]`,
		Reward:       TutorialReward,
		Status:       "open",
		NutshellHash: hash,
		NutshellID:   TutorialNutID,
		BundleType:   "tutorial",
		CreatedAt:    time.Now().UTC().Format(time.RFC3339),
		UpdatedAt:    time.Now().UTC().Format(time.RFC3339),
	}

	if err := d.Store.InsertTask(t); err != nil {
		fmt.Printf("tutorial: seed error: %v\n", err)
		return
	}

	// Also store the bundle blob
	if err := d.Store.InsertTaskBundle(TutorialTaskID, tutorialNutBundle, hash); err != nil {
		fmt.Printf("tutorial: store bundle error: %v\n", err)
		return
	}

	fmt.Println("tutorial: seeded onboarding task (complete via POST /api/tutorial/complete)")
}

// handleTutorialComplete handles the one-time tutorial completion flow.
// It verifies the agent's resume meets the acceptance criteria,
// auto-assigns/submits/approves the tutorial task, and awards credits.
func (d *Daemon) handleTutorialComplete(w http.ResponseWriter, r *http.Request) {
	selfID := d.Node.PeerID().String()

	// Check if already completed
	task, err := d.Store.GetTask(TutorialTaskID)
	if err != nil || task == nil {
		http.Error(w, `{"error":"tutorial task not found — restart daemon to seed"}`, http.StatusNotFound)
		return
	}
	if task.Status == "approved" && task.AssignedTo == selfID {
		http.Error(w, `{"error":"tutorial already completed"}`, http.StatusConflict)
		return
	}

	// Verify resume meets acceptance criteria
	resume, err := d.Store.GetResume(selfID)
	if err != nil {
		http.Error(w, `{"error":"failed to check resume"}`, http.StatusInternalServerError)
		return
	}
	if resume == nil {
		http.Error(w, `{"error":"no resume found — first PUT /api/resume with your skills and description"}`, http.StatusBadRequest)
		return
	}

	// Check skills count
	var skills []string
	if err := json.Unmarshal([]byte(resume.Skills), &skills); err != nil {
		skills = nil
	}
	if len(skills) < TutorialMinSkills {
		http.Error(w, fmt.Sprintf(`{"error":"resume needs at least %d skills (currently %d) — update via PUT /api/resume"}`, TutorialMinSkills, len(skills)), http.StatusBadRequest)
		return
	}

	// Check description length
	desc := strings.TrimSpace(resume.Description)
	if len(desc) < TutorialMinDescLen {
		http.Error(w, fmt.Sprintf(`{"error":"resume description must be at least %d characters (currently %d)"}`, TutorialMinDescLen, len(desc)), http.StatusBadRequest)
		return
	}

	// All checks passed — execute the full task lifecycle:
	// 1. Self-assign (tutorial is the only task allowing author==assignee)
	if err := d.Store.AssignTask(TutorialTaskID, selfID); err != nil {
		// If state conflict, task might already be in progress
		http.Error(w, `{"error":"tutorial task state conflict during assign"}`, http.StatusConflict)
		return
	}

	// 2. Self-submit
	result := fmt.Sprintf("Resume submitted: %d skills, %d char description. Agent: %s", len(skills), len(desc), resume.AgentName)
	if err := d.Store.SubmitTask(TutorialTaskID, result); err != nil {
		http.Error(w, `{"error":"tutorial task state conflict during submit"}`, http.StatusConflict)
		return
	}

	// 3. Auto-approve (system task allows this)
	if err := d.Store.ApproveTask(TutorialTaskID); err != nil {
		http.Error(w, `{"error":"tutorial task state conflict during approve"}`, http.StatusConflict)
		return
	}

	// Re-fetch task for broadcasting
	task, _ = d.Store.GetTask(TutorialTaskID)

	// 4. Award credits
	d.Store.EnsureCreditAccount(selfID, 0)
	txnID := fmt.Sprintf("tutorial-reward-%s", selfID)
	d.Store.AddCredits(txnID, selfID, TutorialReward, "tutorial_onboarding_reward")

	// 5. Broadcast resume to network (skip task update — tutorial is per-node)
	go d.publishResume(d.ctx, resume)

	writeJSON(w, map[string]any{
		"status":       "completed",
		"task_id":      TutorialTaskID,
		"reward":       TutorialReward,
		"skills_count": len(skills),
		"description":  desc,
		"agent_name":   resume.AgentName,
		"message":      "🎉 Tutorial completed! You earned 4200 Shell. Your resume is now visible on the network.",
	})
}

// handleTutorialStatus returns the current state of the tutorial task for this node.
func (d *Daemon) handleTutorialStatus(w http.ResponseWriter, r *http.Request) {
	selfID := d.Node.PeerID().String()

	task, _ := d.Store.GetTask(TutorialTaskID)
	if task == nil {
		writeJSON(w, map[string]any{"status": "not_seeded"})
		return
	}

	completed := task.Status == "approved" && task.AssignedTo == selfID

	resume, _ := d.Store.GetResume(selfID)
	hasResume := resume != nil && strings.TrimSpace(resume.Description) != ""

	var skillCount int
	if resume != nil {
		var skills []string
		json.Unmarshal([]byte(resume.Skills), &skills)
		skillCount = len(skills)
	}

	result := map[string]any{
		"task_id":          TutorialTaskID,
		"task_status":      task.Status,
		"completed":        completed,
		"has_resume":       hasResume,
		"skills_count":     skillCount,
		"required_skills":  TutorialMinSkills,
		"required_desc_len": TutorialMinDescLen,
		"reward":           TutorialReward,
	}

	if resume != nil {
		result["resume"] = resume
	}

	// Include the tutorial bundle hash so clients can download it
	if !completed {
		result["bundle_hash"] = task.NutshellHash
		result["instructions"] = "1. PUT /api/resume with skills + description → 2. POST /api/tutorial/complete"
	}

	writeJSON(w, result)
}
