package store

import "database/sql"

// Task represents a task in the Task Bazaar.
type Task struct {
	ID          string  `json:"id"`
	AuthorID    string  `json:"author_id"`
	AuthorName  string  `json:"author_name"`
	Title       string  `json:"title"`
	Description string  `json:"description"`
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
		`INSERT INTO tasks (id, author_id, author_name, title, description, reward, status)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   title = excluded.title, description = excluded.description,
		   reward = excluded.reward, status = excluded.status,
		   updated_at = datetime('now')`,
		t.ID, t.AuthorID, t.AuthorName, t.Title, t.Description, t.Reward, t.Status,
	)
	return err
}

// GetTask returns a single task by ID.
func (s *Store) GetTask(id string) (*Task, error) {
	row := s.DB.QueryRow(
		`SELECT id, author_id, author_name, title, description, reward, status,
		        assigned_to, result, created_at, updated_at
		 FROM tasks WHERE id = ?`, id,
	)
	t := &Task{}
	err := row.Scan(&t.ID, &t.AuthorID, &t.AuthorName, &t.Title, &t.Description,
		&t.Reward, &t.Status, &t.AssignedTo, &t.Result, &t.CreatedAt, &t.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return t, err
}

// ListTasks returns tasks with optional status filter.
func (s *Store) ListTasks(status string, limit, offset int) ([]*Task, error) {
	var rows *sql.Rows
	var err error
	if status != "" {
		rows, err = s.DB.Query(
			`SELECT id, author_id, author_name, title, description, reward, status,
			        assigned_to, result, created_at, updated_at
			 FROM tasks WHERE status = ?
			 ORDER BY created_at DESC LIMIT ? OFFSET ?`,
			status, limit, offset,
		)
	} else {
		rows, err = s.DB.Query(
			`SELECT id, author_id, author_name, title, description, reward, status,
			        assigned_to, result, created_at, updated_at
			 FROM tasks ORDER BY created_at DESC LIMIT ? OFFSET ?`,
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
			&t.Reward, &t.Status, &t.AssignedTo, &t.Result, &t.CreatedAt, &t.UpdatedAt); err != nil {
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
