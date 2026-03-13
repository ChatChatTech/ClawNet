package store

import "database/sql"

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
	Status      string  `json:"status"` // open, assigned, submitted, approved, rejected, cancelled
	AssignedTo  string  `json:"assigned_to"`
	Result      string  `json:"result"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
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
		`INSERT INTO tasks (id, author_id, author_name, title, description, tags, deadline, reward, status)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   title = excluded.title, description = excluded.description,
		   tags = excluded.tags, deadline = excluded.deadline,
		   reward = excluded.reward, status = excluded.status,
		   updated_at = datetime('now')`,
		t.ID, t.AuthorID, t.AuthorName, t.Title, t.Description, t.Tags, t.Deadline, t.Reward, t.Status,
	)
	return err
}

// GetTask returns a single task by ID.
func (s *Store) GetTask(id string) (*Task, error) {
	row := s.DB.QueryRow(
		`SELECT id, author_id, author_name, title, description, tags, deadline, reward, status,
		        assigned_to, result, created_at, updated_at
		 FROM tasks WHERE id = ?`, id,
	)
	t := &Task{}
	err := row.Scan(&t.ID, &t.AuthorID, &t.AuthorName, &t.Title, &t.Description,
		&t.Tags, &t.Deadline, &t.Reward, &t.Status, &t.AssignedTo, &t.Result, &t.CreatedAt, &t.UpdatedAt)
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
			        t.assigned_to, t.result, t.created_at, t.updated_at
			 FROM tasks t
			 LEFT JOIN credit_accounts c ON t.author_id = c.peer_id
			 WHERE t.status = ?
			 ORDER BY COALESCE(c.balance, 0) DESC, t.created_at DESC LIMIT ? OFFSET ?`,
			status, limit, offset,
		)
	} else {
		rows, err = s.DB.Query(
			`SELECT t.id, t.author_id, t.author_name, t.title, t.description, t.tags, t.deadline, t.reward, t.status,
			        t.assigned_to, t.result, t.created_at, t.updated_at
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
			&t.Tags, &t.Deadline, &t.Reward, &t.Status, &t.AssignedTo, &t.Result, &t.CreatedAt, &t.UpdatedAt); err != nil {
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
	_, err := s.DB.Exec(
		`UPDATE tasks SET status = 'assigned', assigned_to = ?, updated_at = datetime('now')
		 WHERE id = ? AND status = 'open'`,
		assigneeID, taskID,
	)
	return err
}

// SubmitTask marks a task as submitted with a result.
func (s *Store) SubmitTask(taskID, result string) error {
	_, err := s.DB.Exec(
		`UPDATE tasks SET status = 'submitted', result = ?, updated_at = datetime('now')
		 WHERE id = ? AND status = 'assigned'`,
		result, taskID,
	)
	return err
}

// ApproveTask marks a task as approved.
func (s *Store) ApproveTask(taskID string) error {
	_, err := s.DB.Exec(
		`UPDATE tasks SET status = 'approved', updated_at = datetime('now')
		 WHERE id = ? AND status = 'submitted'`,
		taskID,
	)
	return err
}

// RejectTask marks a task as rejected.
func (s *Store) RejectTask(taskID string) error {
	_, err := s.DB.Exec(
		`UPDATE tasks SET status = 'rejected', updated_at = datetime('now')
		 WHERE id = ? AND status = 'submitted'`,
		taskID,
	)
	return err
}

// CancelTask cancels an open or assigned task (only the author should call this).
func (s *Store) CancelTask(taskID string) error {
	_, err := s.DB.Exec(
		`UPDATE tasks SET status = 'cancelled', updated_at = datetime('now')
		 WHERE id = ? AND status IN ('open', 'assigned')`,
		taskID,
	)
	return err
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
