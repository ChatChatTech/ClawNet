package store

import (
	"database/sql"
	"math"
	"time"
)

// ── Auction House constants ──
// Dynamic bidding window: bid_close = created_at + Base + NumBids × Extension, capped at Max.
// Both publisher and bidder compute the same deterministic deadline from the bid list.
const (
	BidWindowBase      = 30 * time.Minute // base window before any bids
	BidWindowExtension = 5 * time.Minute  // added per bid
	BidWindowMax       = 4 * time.Hour    // hard cap
	WorkPeriod         = 2 * time.Hour    // time after bid close for workers to submit
	SettleGrace        = 1 * time.Hour    // grace for author to pick winner before auto-settle
	WinnerShare        = 0.80             // winner gets 80% of reward
	ConsolationShare   = 0.20             // remaining 20% split among other submitters
)

// ComputeBidClose calculates the deterministic bid closing time.
// Every peer can compute the same deadline from createdAt + number of bids.
func ComputeBidClose(createdAt time.Time, numBids int) time.Time {
	ext := time.Duration(numBids) * BidWindowExtension
	total := BidWindowBase + ext
	if total > BidWindowMax {
		total = BidWindowMax
	}
	return createdAt.Add(total)
}

// ComputeWorkDeadline returns the submission deadline = bid_close + WorkPeriod.
func ComputeWorkDeadline(bidClose time.Time) time.Time {
	return bidClose.Add(WorkPeriod)
}

// ExpectedEarnings estimates per-worker expected reward given current bid count.
// Uses game-theory expected value: R / E[n] where E[n] ≈ current bidders.
// Adjusts for winner-take-most: winner 80%, consolation split among rest.
func ExpectedEarnings(reward float64, numBids int) float64 {
	if numBids <= 0 {
		return reward
	}
	if numBids == 1 {
		return reward
	}
	// Expected value = P(win)*WinnerShare*R + P(lose)*ConsolationPerWorker
	pWin := 1.0 / float64(numBids)
	consolationEach := (ConsolationShare * reward) / math.Max(float64(numBids-1), 1)
	return pWin*WinnerShare*reward + (1-pWin)*consolationEach
}

// Task represents a task in the Task Bazaar.
type Task struct {
	ID          string  `json:"id"`
	AuthorID    string  `json:"author_id"`
	AuthorName  string  `json:"author_name"`
	Title       string  `json:"title"`
	Description string  `json:"description"`
	Tags        string  `json:"tags"`     // JSON array of required skill tags, e.g. ["data-analysis","python"]
	Deadline    string  `json:"deadline"` // RFC3339 deadline for task completion
	Reward      float64 `json:"reward"`
	Status      string  `json:"status"` // open, assigned, submitted, approved, rejected, cancelled, settled
	AssignedTo  string  `json:"assigned_to"`
	Result      string  `json:"result"`
	TargetPeer  string  `json:"target_peer,omitempty"` // if set, only this peer can bid/accept
	BidCloseAt  string  `json:"bid_close_at,omitempty"`  // deterministic bid close time
	WorkDeadline string `json:"work_deadline,omitempty"` // submission deadline
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
	// Nutshell integration (optional — populated when task originates from a .nut bundle)
	NutshellHash string `json:"nutshell_hash,omitempty"` // SHA-256 of the .nut bundle
	NutshellID   string `json:"nutshell_id,omitempty"`   // nutshell manifest ID
	BundleType   string `json:"bundle_type,omitempty"`   // request, delivery, template, etc.
}

// TaskBid represents a bid on a task.
type TaskBid struct {
	ID        string  `json:"id"`
	TaskID    string  `json:"task_id"`
	BidderID  string  `json:"bidder_id"`
	BidderName string `json:"bidder_name"`
	Amount    float64 `json:"amount"`
	Message   string  `json:"message"`
	CreatedAt string  `json:"created_at"`
}

// InsertTask upserts a task.
func (s *Store) InsertTask(t *Task) error {
	_, err := s.DB.Exec(
		`INSERT INTO tasks (id, author_id, author_name, title, description, tags, deadline, reward, status,
		                     assigned_to, result, target_peer, nutshell_hash, nutshell_id, bundle_type,
		                     bid_close_at, work_deadline)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   title = excluded.title, description = excluded.description,
		   tags = excluded.tags, deadline = excluded.deadline,
		   reward = excluded.reward, status = excluded.status,
		   assigned_to = COALESCE(NULLIF(excluded.assigned_to, ''), tasks.assigned_to),
		   result = COALESCE(NULLIF(excluded.result, ''), tasks.result),
		   target_peer = excluded.target_peer,
		   nutshell_hash = excluded.nutshell_hash, nutshell_id = excluded.nutshell_id,
		   bundle_type = excluded.bundle_type,
		   bid_close_at = COALESCE(NULLIF(excluded.bid_close_at, ''), tasks.bid_close_at),
		   work_deadline = COALESCE(NULLIF(excluded.work_deadline, ''), tasks.work_deadline),
		   updated_at = datetime('now')`,
		t.ID, t.AuthorID, t.AuthorName, t.Title, t.Description, t.Tags, t.Deadline, t.Reward, t.Status,
		t.AssignedTo, t.Result, t.TargetPeer, t.NutshellHash, t.NutshellID, t.BundleType,
		t.BidCloseAt, t.WorkDeadline,
	)
	return err
}

// GetTask returns a single task by ID.
func (s *Store) GetTask(id string) (*Task, error) {
	row := s.DB.QueryRow(
		`SELECT id, author_id, author_name, title, description, tags, deadline, reward, status,
		        assigned_to, result, target_peer, created_at, updated_at,
		        nutshell_hash, nutshell_id, bundle_type, bid_close_at, work_deadline
		 FROM tasks WHERE id = ?`, id,
	)
	t := &Task{}
	err := row.Scan(&t.ID, &t.AuthorID, &t.AuthorName, &t.Title, &t.Description,
		&t.Tags, &t.Deadline, &t.Reward, &t.Status, &t.AssignedTo, &t.Result, &t.TargetPeer,
		&t.CreatedAt, &t.UpdatedAt,
		&t.NutshellHash, &t.NutshellID, &t.BundleType, &t.BidCloseAt, &t.WorkDeadline)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return t, err
}

// ListTasks returns tasks with optional status filter.
func (s *Store) ListTasks(status string, limit, offset int) ([]*Task, error) {
	var rows *sql.Rows
	var err error
	// Priority: tasks from higher-energy authors shown first (among same status)
	if status != "" {
		rows, err = s.DB.Query(
			`SELECT t.id, t.author_id, t.author_name, t.title, t.description, t.tags, t.deadline, t.reward, t.status,
			        t.assigned_to, t.result, t.target_peer, t.created_at, t.updated_at,
			        t.nutshell_hash, t.nutshell_id, t.bundle_type, t.bid_close_at, t.work_deadline
			 FROM tasks t
			 LEFT JOIN credit_accounts c ON t.author_id = c.peer_id
			 WHERE t.status = ?
			 ORDER BY COALESCE(c.balance, 0) DESC, t.created_at DESC LIMIT ? OFFSET ?`,
			status, limit, offset,
		)
	} else {
		rows, err = s.DB.Query(
			`SELECT t.id, t.author_id, t.author_name, t.title, t.description, t.tags, t.deadline, t.reward, t.status,
			        t.assigned_to, t.result, t.target_peer, t.created_at, t.updated_at,
			        t.nutshell_hash, t.nutshell_id, t.bundle_type, t.bid_close_at, t.work_deadline
			 FROM tasks t
			 LEFT JOIN credit_accounts c ON t.author_id = c.peer_id
			 ORDER BY COALESCE(c.balance, 0) DESC, t.created_at DESC LIMIT ? OFFSET ?`,
			limit, offset,
		)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []*Task
	for rows.Next() {
		t := &Task{}
		if err := rows.Scan(&t.ID, &t.AuthorID, &t.AuthorName, &t.Title, &t.Description,
			&t.Tags, &t.Deadline, &t.Reward, &t.Status, &t.AssignedTo, &t.Result, &t.TargetPeer,
			&t.CreatedAt, &t.UpdatedAt,
			&t.NutshellHash, &t.NutshellID, &t.BundleType, &t.BidCloseAt, &t.WorkDeadline); err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

// InsertTaskBid upserts a bid.
func (s *Store) InsertTaskBid(b *TaskBid) error {
	_, err := s.DB.Exec(
		`INSERT INTO task_bids (id, task_id, bidder_id, bidder_name, amount, message)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO NOTHING`,
		b.ID, b.TaskID, b.BidderID, b.BidderName, b.Amount, b.Message,
	)
	return err
}

// ListTaskBids returns all bids for a task.
func (s *Store) ListTaskBids(taskID string) ([]*TaskBid, error) {
	rows, err := s.DB.Query(
		`SELECT id, task_id, bidder_id, bidder_name, amount, message, created_at
		 FROM task_bids WHERE task_id = ?
		 ORDER BY created_at ASC`, taskID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var bids []*TaskBid
	for rows.Next() {
		b := &TaskBid{}
		if err := rows.Scan(&b.ID, &b.TaskID, &b.BidderID, &b.BidderName, &b.Amount, &b.Message, &b.CreatedAt); err != nil {
			return nil, err
		}
		bids = append(bids, b)
	}
	return bids, rows.Err()
}

// AssignTask assigns a task to a bidder and freezes the reward.
func (s *Store) AssignTask(taskID, assigneeID string) error {
	res, err := s.DB.Exec(
		`UPDATE tasks SET status = 'assigned', assigned_to = ?, updated_at = datetime('now')
		 WHERE id = ? AND status = 'open'`,
		assigneeID, taskID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrTaskStateConflict
	}
	return nil
}

// SubmitTask marks a task as submitted with a result.
func (s *Store) SubmitTask(taskID, result string) error {
	res, err := s.DB.Exec(
		`UPDATE tasks SET status = 'submitted', result = ?, updated_at = datetime('now')
		 WHERE id = ? AND status = 'assigned'`,
		result, taskID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrTaskStateConflict
	}
	return nil
}

// ApproveTask marks a task as approved.
func (s *Store) ApproveTask(taskID string) error {
	res, err := s.DB.Exec(
		`UPDATE tasks SET status = 'approved', updated_at = datetime('now')
		 WHERE id = ? AND status = 'submitted'`,
		taskID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrTaskStateConflict
	}
	return nil
}

// RejectTask marks a task as rejected.
func (s *Store) RejectTask(taskID string) error {
	res, err := s.DB.Exec(
		`UPDATE tasks SET status = 'rejected', updated_at = datetime('now')
		 WHERE id = ? AND status = 'submitted'`,
		taskID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrTaskStateConflict
	}
	return nil
}

// CancelTask cancels an open or assigned task (only the author should call this).
func (s *Store) CancelTask(taskID string) error {
	res, err := s.DB.Exec(
		`UPDATE tasks SET status = 'cancelled', updated_at = datetime('now')
		 WHERE id = ? AND status IN ('open', 'assigned')`,
		taskID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrTaskStateConflict
	}
	return nil
}

// ── Auction House: multi-worker submissions & auto-settlement ──

// TaskSubmission represents a worker's submission for a task.
type TaskSubmission struct {
	ID         string `json:"id"`
	TaskID     string `json:"task_id"`
	WorkerID   string `json:"worker_id"`
	WorkerName string `json:"worker_name"`
	Result     string `json:"result"`
	IsWinner   bool   `json:"is_winner"`
	SubmittedAt string `json:"submitted_at"`
}

// InsertTaskSubmission upserts a submission (one per worker per task).
func (s *Store) InsertTaskSubmission(sub *TaskSubmission) error {
	_, err := s.DB.Exec(
		`INSERT INTO task_submissions (id, task_id, worker_id, worker_name, result)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET result = excluded.result`,
		sub.ID, sub.TaskID, sub.WorkerID, sub.WorkerName, sub.Result,
	)
	return err
}

// ListTaskSubmissions returns all submissions for a task, ordered by time.
func (s *Store) ListTaskSubmissions(taskID string) ([]*TaskSubmission, error) {
	rows, err := s.DB.Query(
		`SELECT id, task_id, worker_id, worker_name, result, is_winner, submitted_at
		 FROM task_submissions WHERE task_id = ?
		 ORDER BY submitted_at ASC`, taskID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var subs []*TaskSubmission
	for rows.Next() {
		sub := &TaskSubmission{}
		if err := rows.Scan(&sub.ID, &sub.TaskID, &sub.WorkerID, &sub.WorkerName,
			&sub.Result, &sub.IsWinner, &sub.SubmittedAt); err != nil {
			return nil, err
		}
		subs = append(subs, sub)
	}
	return subs, rows.Err()
}

// CountTaskSubmissions returns the number of submissions for a task.
func (s *Store) CountTaskSubmissions(taskID string) (int, error) {
	var count int
	err := s.DB.QueryRow(
		`SELECT COUNT(*) FROM task_submissions WHERE task_id = ?`, taskID,
	).Scan(&count)
	return count, err
}

// MarkWinner marks a submission as the winner.
func (s *Store) MarkWinner(submissionID string) error {
	_, err := s.DB.Exec(
		`UPDATE task_submissions SET is_winner = 1 WHERE id = ?`, submissionID,
	)
	return err
}

// SettleTask marks a task as settled (used by auto-settlement or author pick).
func (s *Store) SettleTask(taskID string) error {
	res, err := s.DB.Exec(
		`UPDATE tasks SET status = 'settled', updated_at = datetime('now')
		 WHERE id = ? AND status IN ('open', 'assigned', 'submitted')`,
		taskID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrTaskStateConflict
	}
	return nil
}

// UpdateBidClose recalculates and stores the bid close time based on current bid count.
func (s *Store) UpdateBidClose(taskID string) error {
	t, err := s.GetTask(taskID)
	if err != nil || t == nil {
		return err
	}
	createdAt, err := time.Parse(time.RFC3339, t.CreatedAt)
	if err != nil {
		createdAt, err = time.Parse("2006-01-02 15:04:05", t.CreatedAt)
		if err != nil {
			return err
		}
	}
	bids, err := s.ListTaskBids(taskID)
	if err != nil {
		return err
	}
	bidClose := ComputeBidClose(createdAt, len(bids))
	workDeadline := ComputeWorkDeadline(bidClose)

	_, err = s.DB.Exec(
		`UPDATE tasks SET bid_close_at = ?, work_deadline = ?, updated_at = datetime('now')
		 WHERE id = ?`,
		bidClose.Format(time.RFC3339), workDeadline.Format(time.RFC3339), taskID,
	)
	return err
}

// ListSettleableTasks returns open tasks whose work_deadline has passed and have submissions.
func (s *Store) ListSettleableTasks() ([]*Task, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	rows, err := s.DB.Query(
		`SELECT t.id, t.author_id, t.author_name, t.title, t.description, t.tags, t.deadline,
		        t.reward, t.status, t.assigned_to, t.result, t.target_peer,
		        t.created_at, t.updated_at, t.nutshell_hash, t.nutshell_id, t.bundle_type,
		        t.bid_close_at, t.work_deadline
		 FROM tasks t
		 WHERE t.status = 'open' AND t.work_deadline != '' AND t.work_deadline <= ?
		   AND EXISTS (SELECT 1 FROM task_submissions s WHERE s.task_id = t.id)`,
		now,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tasks []*Task
	for rows.Next() {
		t := &Task{}
		if err := rows.Scan(&t.ID, &t.AuthorID, &t.AuthorName, &t.Title, &t.Description,
			&t.Tags, &t.Deadline, &t.Reward, &t.Status, &t.AssignedTo, &t.Result, &t.TargetPeer,
			&t.CreatedAt, &t.UpdatedAt,
			&t.NutshellHash, &t.NutshellID, &t.BundleType, &t.BidCloseAt, &t.WorkDeadline); err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

// ListExpiredNoSubmissionTasks returns open tasks whose work_deadline passed with no submissions.
func (s *Store) ListExpiredNoSubmissionTasks() ([]*Task, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	rows, err := s.DB.Query(
		`SELECT t.id, t.author_id, t.reward
		 FROM tasks t
		 WHERE t.status = 'open' AND t.work_deadline != '' AND t.work_deadline <= ?
		   AND NOT EXISTS (SELECT 1 FROM task_submissions s WHERE s.task_id = t.id)`,
		now,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tasks []*Task
	for rows.Next() {
		t := &Task{}
		if err := rows.Scan(&t.ID, &t.AuthorID, &t.Reward); err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

// MatchResult represents a ranked agent candidate for a task.
type MatchResult struct {
	PeerID      string  `json:"peer_id"`
	AgentName   string  `json:"agent_name"`
	MatchScore  float64 `json:"match_score"`  // 0.0 - 1.0 tag overlap
	Reputation  float64 `json:"reputation"`
	Skills      string  `json:"skills"`
	Completed   int     `json:"tasks_completed"`
}

// MatchAgentsForTask finds agents whose resume skills overlap with the task's required tags.
// Returns candidates ranked by (tag_overlap * reputation_weight).
func (s *Store) MatchAgentsForTask(taskID string) ([]*MatchResult, error) {
	t, err := s.GetTask(taskID)
	if err != nil || t == nil {
		return nil, err
	}
	// Parse task tags
	taskTags := parseTags(t.Tags)
	if len(taskTags) == 0 {
		// No tags specified — return all agents with resumes, ranked by reputation
		rows, err := s.DB.Query(
			`SELECT r.peer_id, r.agent_name, r.skills, COALESCE(rep.score, 50), COALESCE(rep.tasks_completed, 0)
			 FROM agent_resumes r
			 LEFT JOIN reputation rep ON r.peer_id = rep.peer_id
			 WHERE r.peer_id != ?
			 ORDER BY COALESCE(rep.score, 50) DESC LIMIT 20`, t.AuthorID)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var results []*MatchResult
		for rows.Next() {
			m := &MatchResult{MatchScore: 1.0}
			if err := rows.Scan(&m.PeerID, &m.AgentName, &m.Skills, &m.Reputation, &m.Completed); err != nil {
				return nil, err
			}
			results = append(results, m)
		}
		return results, rows.Err()
	}

	// Fetch all resumes (excluding task author)
	rows, err := s.DB.Query(
		`SELECT r.peer_id, r.agent_name, r.skills, COALESCE(rep.score, 50), COALESCE(rep.tasks_completed, 0)
		 FROM agent_resumes r
		 LEFT JOIN reputation rep ON r.peer_id = rep.peer_id
		 WHERE r.peer_id != ?`, t.AuthorID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tagSet := make(map[string]bool, len(taskTags))
	for _, tag := range taskTags {
		tagSet[tag] = true
	}

	var results []*MatchResult
	for rows.Next() {
		var peerID, agentName, skills string
		var rep float64
		var completed int
		if err := rows.Scan(&peerID, &agentName, &skills, &rep, &completed); err != nil {
			return nil, err
		}
		agentSkills := parseTags(skills)
		matched := 0
		for _, s := range agentSkills {
			if tagSet[s] {
				matched++
			}
		}
		if matched == 0 {
			continue
		}
		overlap := float64(matched) / float64(len(taskTags))
		results = append(results, &MatchResult{
			PeerID:     peerID,
			AgentName:  agentName,
			MatchScore: overlap,
			Reputation: rep,
			Skills:     skills,
			Completed:  completed,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Sort by composite score: overlap * sqrt(reputation/50)
	sortMatchResults(results)
	return results, nil
}

// MatchTasksForAgent finds open tasks whose tags overlap with the agent's resume skills.
func (s *Store) MatchTasksForAgent(peerID string) ([]*Task, error) {
	resume, err := s.GetResume(peerID)
	if err != nil || resume == nil {
		// No resume — return all open tasks
		return s.ListTasks("open", 50, 0)
	}
	agentSkills := parseTags(resume.Skills)
	if len(agentSkills) == 0 {
		return s.ListTasks("open", 50, 0)
	}
	skillSet := make(map[string]bool, len(agentSkills))
	for _, s := range agentSkills {
		skillSet[s] = true
	}

	// Get all open tasks
	tasks, err := s.ListTasks("open", 200, 0)
	if err != nil {
		return nil, err
	}

	// Filter and rank by tag overlap
	type scored struct {
		task  *Task
		score float64
	}
	var matched []scored
	for _, t := range tasks {
		tags := parseTags(t.Tags)
		if len(tags) == 0 {
			// Tasks with no tags match everyone
			matched = append(matched, scored{t, 0.5})
			continue
		}
		hit := 0
		for _, tag := range tags {
			if skillSet[tag] {
				hit++
			}
		}
		if hit > 0 {
			matched = append(matched, scored{t, float64(hit) / float64(len(tags))})
		}
	}

	// Sort by score descending
	for i := 1; i < len(matched); i++ {
		for j := i; j > 0 && matched[j].score > matched[j-1].score; j-- {
			matched[j], matched[j-1] = matched[j-1], matched[j]
		}
	}

	result := make([]*Task, 0, len(matched))
	for _, m := range matched {
		if len(result) >= 50 {
			break
		}
		result = append(result, m.task)
	}
	return result, nil
}

func sortMatchResults(results []*MatchResult) {
	// Insertion sort — small N
	for i := 1; i < len(results); i++ {
		for j := i; j > 0; j-- {
			si := results[j].MatchScore * sqrtRep(results[j].Reputation)
			sj := results[j-1].MatchScore * sqrtRep(results[j-1].Reputation)
			if si > sj {
				results[j], results[j-1] = results[j-1], results[j]
			} else {
				break
			}
		}
	}
}

func sqrtRep(rep float64) float64 {
	if rep <= 0 {
		return 0.1
	}
	// Newton's method for sqrt(rep/50)
	x := rep / 50
	guess := x
	for i := 0; i < 10; i++ {
		guess = (guess + x/guess) / 2
	}
	if guess < 0.1 {
		return 0.1
	}
	return guess
}

// ── Nutshell bundle storage ──

// InsertTaskBundle stores a .nut bundle blob for a task.
func (s *Store) InsertTaskBundle(taskID string, bundle []byte, hash string) error {
	_, err := s.DB.Exec(
		`INSERT INTO task_bundles (task_id, bundle, hash, size)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(task_id) DO UPDATE SET
		   bundle = excluded.bundle, hash = excluded.hash,
		   size = excluded.size, uploaded_at = datetime('now')`,
		taskID, bundle, hash, len(bundle),
	)
	return err
}

// GetTaskBundle retrieves the .nut bundle blob for a task.
func (s *Store) GetTaskBundle(taskID string) ([]byte, string, error) {
	var bundle []byte
	var hash string
	err := s.DB.QueryRow(
		`SELECT bundle, hash FROM task_bundles WHERE task_id = ?`, taskID,
	).Scan(&bundle, &hash)
	if err == sql.ErrNoRows {
		return nil, "", nil
	}
	return bundle, hash, err
}

// HasTaskBundle checks if a bundle exists for a task.
func (s *Store) HasTaskBundle(taskID string) (bool, error) {
	var count int
	err := s.DB.QueryRow(
		`SELECT COUNT(*) FROM task_bundles WHERE task_id = ?`, taskID,
	).Scan(&count)
	return count > 0, err
}

// BoardTask is a summary task for the dashboard.
type BoardTask struct {
	ID           string  `json:"id"`
	Title        string  `json:"title"`
	Status       string  `json:"status"`
	Reward       float64 `json:"reward"`
	AuthorName   string  `json:"author_name"`
	AssignedTo   string  `json:"assigned_to"`
	TargetPeer   string  `json:"target_peer,omitempty"`
	BidCount     int     `json:"bid_count"`
	SubCount     int     `json:"sub_count"`        // number of submissions
	BidCloseAt   string  `json:"bid_close_at"`     // deterministic bid close time
	WorkDeadline string  `json:"work_deadline"`    // submission deadline
	ExpectedPay  float64 `json:"expected_pay"`     // game-theory expected earnings
	CreatedAt    string  `json:"created_at"`
}

// TaskBoard returns a dashboard view: tasks the peer published, tasks assigned to peer, and open tasks.
func (s *Store) TaskBoard(peerID string) (published, assigned, open []*BoardTask, err error) {
	scanBoard := func(rows *sql.Rows) ([]*BoardTask, error) {
		var result []*BoardTask
		for rows.Next() {
			bt := &BoardTask{}
			if err := rows.Scan(&bt.ID, &bt.Title, &bt.Status, &bt.Reward, &bt.AuthorName,
				&bt.AssignedTo, &bt.TargetPeer, &bt.CreatedAt, &bt.BidCount,
				&bt.SubCount, &bt.BidCloseAt, &bt.WorkDeadline); err != nil {
				return nil, err
			}
			bt.ExpectedPay = ExpectedEarnings(bt.Reward, bt.BidCount)
			result = append(result, bt)
		}
		return result, rows.Err()
	}

	// My published tasks (all statuses)
	rows, err := s.DB.Query(
		`SELECT t.id, t.title, t.status, t.reward, t.author_name, t.assigned_to, t.target_peer, t.created_at,
		        (SELECT COUNT(*) FROM task_bids b WHERE b.task_id = t.id) as bid_count,
		        (SELECT COUNT(*) FROM task_submissions s WHERE s.task_id = t.id) as sub_count,
		        t.bid_close_at, t.work_deadline
		 FROM tasks t WHERE t.author_id = ?
		 ORDER BY t.created_at DESC LIMIT 50`, peerID)
	if err != nil {
		return nil, nil, nil, err
	}
	defer rows.Close()
	published, err = scanBoard(rows)
	if err != nil {
		return nil, nil, nil, err
	}

	// My assigned tasks (legacy) + tasks I've submitted to (new flow)
	rows2, err := s.DB.Query(
		`SELECT t.id, t.title, t.status, t.reward, t.author_name, t.assigned_to, t.target_peer, t.created_at,
		        (SELECT COUNT(*) FROM task_bids b WHERE b.task_id = t.id) as bid_count,
		        (SELECT COUNT(*) FROM task_submissions s WHERE s.task_id = t.id) as sub_count,
		        t.bid_close_at, t.work_deadline
		 FROM tasks t
		 WHERE (t.assigned_to = ? AND t.status IN ('assigned','submitted'))
		    OR (t.status = 'open' AND EXISTS (SELECT 1 FROM task_bids b WHERE b.task_id = t.id AND b.bidder_id = ?))
		 ORDER BY t.created_at DESC LIMIT 50`, peerID, peerID)
	if err != nil {
		return nil, nil, nil, err
	}
	defer rows2.Close()
	assigned, err = scanBoard(rows2)
	if err != nil {
		return nil, nil, nil, err
	}

	// Open tasks (exclude my own, respect targeting, exclude tasks I already bid on)
	rows3, err := s.DB.Query(
		`SELECT t.id, t.title, t.status, t.reward, t.author_name, t.assigned_to, t.target_peer, t.created_at,
		        (SELECT COUNT(*) FROM task_bids b WHERE b.task_id = t.id) as bid_count,
		        (SELECT COUNT(*) FROM task_submissions s WHERE s.task_id = t.id) as sub_count,
		        t.bid_close_at, t.work_deadline
		 FROM tasks t
		 WHERE t.status = 'open' AND t.author_id != ?
		   AND (t.target_peer = '' OR t.target_peer = ?)
		   AND NOT EXISTS (SELECT 1 FROM task_bids b WHERE b.task_id = t.id AND b.bidder_id = ?)
		 ORDER BY t.reward DESC, t.created_at DESC LIMIT 50`, peerID, peerID, peerID)
	if err != nil {
		return nil, nil, nil, err
	}
	defer rows3.Close()
	open, err = scanBoard(rows3)
	if err != nil {
		return nil, nil, nil, err
	}

	return published, assigned, open, nil
}
