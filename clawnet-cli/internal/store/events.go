package store

// Event represents a network activity event for the watch stream.
type Event struct {
	ID        int64  `json:"id"`
	Type      string `json:"type"`
	Actor     string `json:"actor"`
	Target    string `json:"target"`
	Detail    string `json:"detail"`
	CreatedAt string `json:"created_at"`
}

// InsertEvent records a new event.
func (s *Store) InsertEvent(typ, actor, target, detail string) error {
	_, err := s.DB.Exec(`INSERT INTO events (type, actor, target, detail) VALUES (?, ?, ?, ?)`,
		typ, actor, target, detail)
	return err
}

// ListEventsSince returns events after the given ID (for polling).
func (s *Store) ListEventsSince(afterID int64, limit int) ([]*Event, error) {
	rows, err := s.DB.Query(`SELECT id, type, actor, target, detail, created_at FROM events WHERE id > ? ORDER BY id ASC LIMIT ?`, afterID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var events []*Event
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.ID, &e.Type, &e.Actor, &e.Target, &e.Detail, &e.CreatedAt); err != nil {
			return nil, err
		}
		events = append(events, &e)
	}
	return events, nil
}

// ListRecentEvents returns the most recent events.
func (s *Store) ListRecentEvents(limit int) ([]*Event, error) {
	rows, err := s.DB.Query(`SELECT id, type, actor, target, detail, created_at FROM events ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var events []*Event
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.ID, &e.Type, &e.Actor, &e.Target, &e.Detail, &e.CreatedAt); err != nil {
			return nil, err
		}
		events = append(events, &e)
	}
	// Reverse to chronological order
	for i, j := 0, len(events)-1; i < j; i, j = i+1, j-1 {
		events[i], events[j] = events[j], events[i]
	}
	return events, nil
}

// PruneOldEvents removes events older than 24 hours.
func (s *Store) PruneOldEvents() error {
	_, err := s.DB.Exec(`DELETE FROM events WHERE created_at < datetime('now', '-24 hours')`)
	return err
}
