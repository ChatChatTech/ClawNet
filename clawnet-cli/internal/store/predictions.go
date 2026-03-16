package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
)

// Prediction represents a prediction market event.
type Prediction struct {
	ID               string  `json:"id"`
	CreatorID        string  `json:"creator_id"`
	CreatorName      string  `json:"creator_name"`
	Question         string  `json:"question"`
	Options          string  `json:"options"`           // JSON array: ["Cut ≥25bp","No change","Hike"]
	Category         string  `json:"category"`          // macro-economics, tech, ai, sports, etc.
	ResolutionDate   string  `json:"resolution_date"`   // RFC3339
	ResolutionSource string  `json:"resolution_source"` // description of ground truth
	Status           string  `json:"status"`            // open, resolved, cancelled
	Result           string  `json:"result"`            // winning option after resolution
	TotalStake       int64  `json:"total_stake"`
	CreatedAt        string  `json:"created_at"`
	UpdatedAt        string  `json:"updated_at"`
}

// PredictionBet represents a bet on a prediction event.
type PredictionBet struct {
	ID           string  `json:"id"`
	PredictionID string  `json:"prediction_id"`
	BettorID     string  `json:"bettor_id"`
	BettorName   string  `json:"bettor_name"`
	Option       string  `json:"option"`    // selected option
	Stake        int64   `json:"stake"`     // Shell amount
	Reasoning    string  `json:"reasoning"` // optional explanation
	CreatedAt    string  `json:"created_at"`
}

// PredictionResolution is a proposed resolution from a peer.
type PredictionResolution struct {
	ID           string `json:"id"`
	PredictionID string `json:"prediction_id"`
	ResolverID   string `json:"resolver_id"`
	Result       string `json:"result"`       // the winning option
	EvidenceURL  string `json:"evidence_url"` // proof
	CreatedAt    string `json:"created_at"`
}

// PredictionLeaderEntry summarizes a peer's prediction accuracy.
type PredictionLeaderEntry struct {
	PeerID    string  `json:"peer_id"`
	TotalBets int     `json:"total_bets"`
	Wins      int     `json:"wins"`
	Losses    int     `json:"losses"`
	Profit    int64   `json:"profit"`
	Accuracy  float64 `json:"accuracy"` // wins / total_bets
}

// InsertPrediction upserts a prediction event.
func (s *Store) InsertPrediction(p *Prediction) error {
	_, err := s.DB.Exec(
		`INSERT INTO predictions (id, creator_id, creator_name, question, options,
		  category, resolution_date, resolution_source, status, total_stake)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   status = excluded.status, result = CASE WHEN excluded.result != '' THEN excluded.result ELSE predictions.result END,
		   total_stake = excluded.total_stake,
		   updated_at = datetime('now')`,
		p.ID, p.CreatorID, p.CreatorName, p.Question, p.Options,
		p.Category, p.ResolutionDate, p.ResolutionSource, p.Status, p.TotalStake,
	)
	return err
}

// GetPrediction returns a prediction by ID with computed option stakes.
func (s *Store) GetPrediction(id string) (*Prediction, error) {
	row := s.DB.QueryRow(
		`SELECT id, creator_id, creator_name, question, options, category,
		        resolution_date, resolution_source, status, result, total_stake,
		        created_at, updated_at
		 FROM predictions WHERE id = ?`, id,
	)
	p := &Prediction{}
	err := row.Scan(&p.ID, &p.CreatorID, &p.CreatorName, &p.Question, &p.Options,
		&p.Category, &p.ResolutionDate, &p.ResolutionSource, &p.Status, &p.Result,
		&p.TotalStake, &p.CreatedAt, &p.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return p, err
}

// ListPredictions lists predictions with optional status/category filters.
func (s *Store) ListPredictions(status, category string, limit, offset int) ([]*Prediction, error) {
	q := `SELECT id, creator_id, creator_name, question, options, category,
	             resolution_date, resolution_source, status, result, total_stake,
	             created_at, updated_at
	      FROM predictions WHERE 1=1`
	args := []any{}
	if status != "" {
		q += " AND status = ?"
		args = append(args, status)
	}
	if category != "" {
		q += " AND category = ?"
		args = append(args, category)
	}
	q += " ORDER BY total_stake DESC, created_at DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := s.DB.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var preds []*Prediction
	for rows.Next() {
		p := &Prediction{}
		if err := rows.Scan(&p.ID, &p.CreatorID, &p.CreatorName, &p.Question, &p.Options,
			&p.Category, &p.ResolutionDate, &p.ResolutionSource, &p.Status, &p.Result,
			&p.TotalStake, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		preds = append(preds, p)
	}
	return preds, rows.Err()
}

// InsertPredictionBet inserts a bet and updates the prediction's total stake.
func (s *Store) InsertPredictionBet(b *PredictionBet) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(
		`INSERT INTO prediction_bets (id, prediction_id, bettor_id, bettor_name, option, stake, reasoning)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO NOTHING`,
		b.ID, b.PredictionID, b.BettorID, b.BettorName, b.Option, b.Stake, b.Reasoning,
	)
	if err != nil {
		return err
	}

	_, err = tx.Exec(
		`UPDATE predictions SET total_stake = total_stake + ?, updated_at = datetime('now')
		 WHERE id = ?`,
		b.Stake, b.PredictionID,
	)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// ListPredictionBets returns all bets for a prediction event.
func (s *Store) ListPredictionBets(predictionID string) ([]*PredictionBet, error) {
	rows, err := s.DB.Query(
		`SELECT id, prediction_id, bettor_id, bettor_name, option, stake, reasoning, created_at
		 FROM prediction_bets WHERE prediction_id = ?
		 ORDER BY created_at ASC`, predictionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var bets []*PredictionBet
	for rows.Next() {
		b := &PredictionBet{}
		if err := rows.Scan(&b.ID, &b.PredictionID, &b.BettorID, &b.BettorName,
			&b.Option, &b.Stake, &b.Reasoning, &b.CreatedAt); err != nil {
			return nil, err
		}
		bets = append(bets, b)
	}
	return bets, rows.Err()
}

// InsertPredictionResolution records a resolution proposal.
func (s *Store) InsertPredictionResolution(r *PredictionResolution) error {
	_, err := s.DB.Exec(
		`INSERT INTO prediction_resolutions (id, prediction_id, resolver_id, result, evidence_url)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO NOTHING`,
		r.ID, r.PredictionID, r.ResolverID, r.Result, r.EvidenceURL,
	)
	return err
}

// CountResolutions counts how many unique resolvers submitted the same result.
func (s *Store) CountResolutions(predictionID, result string) (int, error) {
	var count int
	err := s.DB.QueryRow(
		`SELECT COUNT(DISTINCT resolver_id) FROM prediction_resolutions
		 WHERE prediction_id = ? AND result = ?`,
		predictionID, result,
	).Scan(&count)
	return count, err
}

// ResolvePrediction sets the winning result and marks the prediction resolved.
func (s *Store) ResolvePrediction(predictionID, result string) error {
	_, err := s.DB.Exec(
		`UPDATE predictions SET status = 'resolved', result = ?, updated_at = datetime('now')
		 WHERE id = ? AND status = 'open'`,
		result, predictionID,
	)
	return err
}

// SettlePrediction distributes credits to winners from losers.
// Returns the list of (bettor_id, profit) for each winner.
func (s *Store) SettlePrediction(predictionID, result string) ([]struct{ PeerID string; Profit int64 }, error) {
	bets, err := s.ListPredictionBets(predictionID)
	if err != nil {
		return nil, err
	}

	var totalPool, winnerPool int64
	for _, b := range bets {
		totalPool += b.Stake
		if b.Option == result {
			winnerPool += b.Stake
		}
	}

	if winnerPool == 0 || totalPool == 0 {
		return nil, nil // no winners or no bets
	}

	var settlements []struct{ PeerID string; Profit int64 }
	for _, b := range bets {
		if b.Option == result {
			// Winner: gets back stake + proportional share of loser pool
			payout := b.Stake * totalPool / winnerPool
			profit := payout - b.Stake
			settlements = append(settlements, struct{ PeerID string; Profit int64 }{b.BettorID, profit})
		}
	}
	return settlements, nil
}

// GetOptionStakes returns a map of option -> total stake for a prediction.
func (s *Store) GetOptionStakes(predictionID string) (map[string]int64, error) {
	rows, err := s.DB.Query(
		`SELECT option, SUM(stake) FROM prediction_bets
		 WHERE prediction_id = ? GROUP BY option`,
		predictionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	stakes := map[string]int64{}
	for rows.Next() {
		var opt string
		var total int64
		if err := rows.Scan(&opt, &total); err != nil {
			return nil, err
		}
		stakes[opt] = total
	}
	return stakes, rows.Err()
}

// GetPredictionLeaderboard returns top predictors ranked by profit.
func (s *Store) GetPredictionLeaderboard(limit int) ([]*PredictionLeaderEntry, error) {
	rows, err := s.DB.Query(
		`SELECT b.bettor_id,
		        COUNT(*) as total_bets,
		        SUM(CASE WHEN b.option = p.result AND p.status = 'resolved' THEN 1 ELSE 0 END) as wins,
		        SUM(CASE WHEN b.option != p.result AND p.status = 'resolved' THEN 1 ELSE 0 END) as losses,
		        COALESCE(SUM(CASE WHEN p.status = 'resolved' THEN
		          CASE WHEN b.option = p.result THEN b.stake ELSE -b.stake END
		        ELSE 0 END), 0) as profit
		 FROM prediction_bets b
		 JOIN predictions p ON b.prediction_id = p.id
		 GROUP BY b.bettor_id
		 ORDER BY profit DESC
		 LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []*PredictionLeaderEntry
	for rows.Next() {
		e := &PredictionLeaderEntry{}
		if err := rows.Scan(&e.PeerID, &e.TotalBets, &e.Wins, &e.Losses, &e.Profit); err != nil {
			return nil, err
		}
		if e.TotalBets > 0 {
			e.Accuracy = float64(e.Wins) / float64(e.TotalBets)
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// PredictionOptionDetail is used for API response with per-option stats.
type PredictionOptionDetail struct {
	Name       string `json:"name"`
	TotalStake int64  `json:"total_stake"`
	Bettors    int    `json:"bettors"`
}

// GetPredictionDetails returns the prediction with per-option breakdown.
func (s *Store) GetPredictionDetails(id string) (*Prediction, []PredictionOptionDetail, error) {
	p, err := s.GetPrediction(id)
	if err != nil || p == nil {
		return p, nil, err
	}

	var options []string
	if err := json.Unmarshal([]byte(p.Options), &options); err != nil {
		return p, nil, nil
	}

	rows, err := s.DB.Query(
		`SELECT option, SUM(stake), COUNT(DISTINCT bettor_id)
		 FROM prediction_bets WHERE prediction_id = ? GROUP BY option`,
		id,
	)
	if err != nil {
		return p, nil, err
	}
	defer rows.Close()

	stakeMap := map[string]PredictionOptionDetail{}
	for rows.Next() {
		var opt string
		var stake int64
		var bettors int
		if err := rows.Scan(&opt, &stake, &bettors); err != nil {
			continue
		}
		stakeMap[opt] = PredictionOptionDetail{Name: opt, TotalStake: stake, Bettors: bettors}
	}

	// Ensure all options appear in the response
	var details []PredictionOptionDetail
	for _, opt := range options {
		if d, ok := stakeMap[opt]; ok {
			details = append(details, d)
		} else {
			details = append(details, PredictionOptionDetail{Name: opt})
		}
	}

	return p, details, nil
}

// ValidatePredictionOption checks if the given option is valid for the prediction.
func ValidatePredictionOption(optionsJSON, option string) error {
	var options []string
	if err := json.Unmarshal([]byte(optionsJSON), &options); err != nil {
		return fmt.Errorf("invalid options JSON: %w", err)
	}
	for _, o := range options {
		if o == option {
			return nil
		}
	}
	return fmt.Errorf("invalid option %q", option)
}

// ── Appeal mechanism ──

// PredictionAppeal represents a bettor's challenge during the appeal window.
type PredictionAppeal struct {
	ID           string `json:"id"`
	PredictionID string `json:"prediction_id"`
	AppellantID  string `json:"appellant_id"`
	Reason       string `json:"reason"`
	EvidenceURL  string `json:"evidence_url"`
	CreatedAt    string `json:"created_at"`
}

// SetPendingWithAppeal transitions a prediction to "pending" with an appeal deadline.
func (s *Store) SetPendingWithAppeal(predictionID, result string, deadline string) error {
	_, err := s.DB.Exec(
		`UPDATE predictions SET status = 'pending', result = ?, appeal_deadline = ?, updated_at = datetime('now')
		 WHERE id = ? AND status = 'open'`,
		result, deadline, predictionID,
	)
	return err
}

// InsertPredictionAppeal records an appeal against a pending prediction.
func (s *Store) InsertPredictionAppeal(a *PredictionAppeal) error {
	_, err := s.DB.Exec(
		`INSERT INTO prediction_appeals (id, prediction_id, appellant_id, reason, evidence_url)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO NOTHING`,
		a.ID, a.PredictionID, a.AppellantID, a.Reason, a.EvidenceURL,
	)
	return err
}

// CountAppeals returns the number of unique appellants for a prediction.
func (s *Store) CountAppeals(predictionID string) (int, error) {
	var count int
	err := s.DB.QueryRow(
		`SELECT COUNT(DISTINCT appellant_id) FROM prediction_appeals WHERE prediction_id = ?`,
		predictionID,
	).Scan(&count)
	return count, err
}

// ListPredictionAppeals returns all appeals for a prediction.
func (s *Store) ListPredictionAppeals(predictionID string) ([]*PredictionAppeal, error) {
	rows, err := s.DB.Query(
		`SELECT id, prediction_id, appellant_id, reason, evidence_url, created_at
		 FROM prediction_appeals WHERE prediction_id = ?
		 ORDER BY created_at ASC`, predictionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var appeals []*PredictionAppeal
	for rows.Next() {
		a := &PredictionAppeal{}
		if err := rows.Scan(&a.ID, &a.PredictionID, &a.AppellantID, &a.Reason, &a.EvidenceURL, &a.CreatedAt); err != nil {
			return nil, err
		}
		appeals = append(appeals, a)
	}
	return appeals, rows.Err()
}

// RevertPredictionToOpen reverts a pending prediction back to open, clearing result and appeals.
func (s *Store) RevertPredictionToOpen(predictionID string) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(
		`UPDATE predictions SET status = 'open', result = '', appeal_deadline = '', updated_at = datetime('now')
		 WHERE id = ? AND status = 'pending'`, predictionID,
	)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`DELETE FROM prediction_resolutions WHERE prediction_id = ?`, predictionID)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`DELETE FROM prediction_appeals WHERE prediction_id = ?`, predictionID)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// ListExpiredPendingPredictions returns predictions in "pending" status whose appeal_deadline has passed.
func (s *Store) ListExpiredPendingPredictions() ([]*Prediction, error) {
	rows, err := s.DB.Query(
		`SELECT id, creator_id, creator_name, question, options, category,
		        resolution_date, resolution_source, status, result, total_stake,
		        created_at, updated_at
		 FROM predictions
		 WHERE status = 'pending' AND appeal_deadline != '' AND appeal_deadline <= datetime('now')`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var preds []*Prediction
	for rows.Next() {
		p := &Prediction{}
		if err := rows.Scan(&p.ID, &p.CreatorID, &p.CreatorName, &p.Question, &p.Options,
			&p.Category, &p.ResolutionDate, &p.ResolutionSource, &p.Status, &p.Result,
			&p.TotalStake, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		preds = append(preds, p)
	}
	return preds, rows.Err()
}
