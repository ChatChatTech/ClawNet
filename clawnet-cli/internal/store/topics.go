package store

// TopicRoom represents a topic room.
type TopicRoom struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	CreatorID   string `json:"creator_id"`
	CreatedAt   string `json:"created_at"`
	Joined      bool   `json:"joined"`
}

// TopicMessage represents a message in a topic room.
type TopicMessage struct {
	ID         string `json:"id"`
	TopicName  string `json:"topic_name"`
	AuthorID   string `json:"author_id"`
	AuthorName string `json:"author_name"`
	Body       string `json:"body"`
	CreatedAt  string `json:"created_at"`
}

// InsertTopic creates or updates a topic room.
func (s *Store) InsertTopic(t *TopicRoom) error {
	joined := 0
	if t.Joined {
		joined = 1
	}
	_, err := s.DB.Exec(
		`INSERT INTO topics (name, description, creator_id, created_at, joined)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(name) DO UPDATE SET description = excluded.description`,
		t.Name, t.Description, t.CreatorID, t.CreatedAt, joined,
	)
	return err
}

// GetTopic retrieves a topic by name.
func (s *Store) GetTopic(name string) (*TopicRoom, error) {
	row := s.DB.QueryRow(
		`SELECT name, description, creator_id, created_at, joined FROM topics WHERE name = ?`, name,
	)
	t := &TopicRoom{}
	var joined int
	if err := row.Scan(&t.Name, &t.Description, &t.CreatorID, &t.CreatedAt, &joined); err != nil {
		return nil, err
	}
	t.Joined = joined != 0
	return t, nil
}

// ListTopics returns all known topic rooms.
func (s *Store) ListTopics() ([]*TopicRoom, error) {
	rows, err := s.DB.Query(
		`SELECT name, description, creator_id, created_at, joined FROM topics ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var topics []*TopicRoom
	for rows.Next() {
		t := &TopicRoom{}
		var joined int
		if err := rows.Scan(&t.Name, &t.Description, &t.CreatorID, &t.CreatedAt, &joined); err != nil {
			return nil, err
		}
		t.Joined = joined != 0
		topics = append(topics, t)
	}
	return topics, rows.Err()
}

// SetTopicJoined updates the joined status of a topic.
func (s *Store) SetTopicJoined(name string, joined bool) error {
	j := 0
	if joined {
		j = 1
	}
	_, err := s.DB.Exec(`UPDATE topics SET joined = ? WHERE name = ?`, j, name)
	return err
}

// InsertTopicMessage stores a message in a topic room.
func (s *Store) InsertTopicMessage(m *TopicMessage) error {
	_, err := s.DB.Exec(
		`INSERT OR IGNORE INTO topic_messages (id, topic_name, author_id, author_name, body, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		m.ID, m.TopicName, m.AuthorID, m.AuthorName, m.Body, m.CreatedAt,
	)
	return err
}

// ListTopicMessages returns messages for a topic, newest first.
func (s *Store) ListTopicMessages(topicName string, limit, offset int) ([]*TopicMessage, error) {
	rows, err := s.DB.Query(
		`SELECT id, topic_name, author_id, author_name, body, created_at
		 FROM topic_messages WHERE topic_name = ? ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		topicName, limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var msgs []*TopicMessage
	for rows.Next() {
		m := &TopicMessage{}
		if err := rows.Scan(&m.ID, &m.TopicName, &m.AuthorID, &m.AuthorName, &m.Body, &m.CreatedAt); err != nil {
			return nil, err
		}
		msgs = append(msgs, m)
	}
	return msgs, rows.Err()
}
