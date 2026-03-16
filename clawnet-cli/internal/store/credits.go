package store

import (
	"database/sql"
	"math"
)

// CreditAccount represents a peer's Shell (贝壳) account.
type CreditAccount struct {
	PeerID       string `json:"peer_id"`
	Balance      int64  `json:"balance"`
	Frozen       int64  `json:"frozen"`
	TotalEarned  int64  `json:"total_earned"`
	TotalSpent   int64  `json:"total_spent"`
	UpdatedAt    string `json:"updated_at"`
}

// CreditTransaction represents a Shell transfer record.
type CreditTransaction struct {
	ID        string `json:"id"`
	FromPeer  string `json:"from_peer"`
	ToPeer    string `json:"to_peer"`
	Amount    int64  `json:"amount"`
	Reason    string `json:"reason"` // "transfer", "task_payment", "task_reward", "initial", "reputation_bonus", "swarm_reward"
	RefID     string `json:"ref_id"` // optional reference (task_id, etc.)
	CreatedAt string `json:"created_at"`
}

// EnsureCreditAccount creates an account with initial balance if it doesn't exist.
func (s *Store) EnsureCreditAccount(peerID string, initialBalance int64) error {
	_, err := s.DB.Exec(
		`INSERT OR IGNORE INTO credit_accounts (peer_id, balance, total_earned)
		 VALUES (?, ?, ?)`,
		peerID, initialBalance, initialBalance,
	)
	return err
}

// GetCreditBalance returns the credit account for a peer.
func (s *Store) GetCreditBalance(peerID string) (*CreditAccount, error) {
	row := s.DB.QueryRow(
		`SELECT peer_id, CAST(balance AS INTEGER), CAST(frozen AS INTEGER),
		        CAST(total_earned AS INTEGER), CAST(total_spent AS INTEGER), updated_at
		 FROM credit_accounts WHERE peer_id = ?`, peerID,
	)
	a := &CreditAccount{}
	err := row.Scan(&a.PeerID, &a.Balance, &a.Frozen, &a.TotalEarned, &a.TotalSpent, &a.UpdatedAt)
	if err == sql.ErrNoRows {
		return &CreditAccount{PeerID: peerID}, nil
	}
	return a, err
}

// TransferCredits moves credits from one peer to another within a transaction.
func (s *Store) TransferCredits(txnID, fromPeer, toPeer string, amount int64, reason, refID string) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Check sender balance
	var balance int64
	err = tx.QueryRow(`SELECT CAST(balance AS INTEGER) FROM credit_accounts WHERE peer_id = ?`, fromPeer).Scan(&balance)
	if err != nil {
		return err
	}
	if balance < amount {
		return ErrInsufficientCredits
	}

	// Debit sender
	_, err = tx.Exec(
		`UPDATE credit_accounts SET balance = balance - ?, total_spent = total_spent + ?, updated_at = datetime('now')
		 WHERE peer_id = ?`, amount, amount, fromPeer,
	)
	if err != nil {
		return err
	}

	// Credit receiver (ensure account exists)
	_, err = tx.Exec(
		`INSERT INTO credit_accounts (peer_id, balance, total_earned)
		 VALUES (?, ?, ?)
		 ON CONFLICT(peer_id) DO UPDATE SET
		   balance = balance + ?, total_earned = total_earned + ?, updated_at = datetime('now')`,
		toPeer, amount, amount, amount, amount,
	)
	if err != nil {
		return err
	}

	// Record transaction
	_, err = tx.Exec(
		`INSERT INTO credit_transactions (id, from_peer, to_peer, amount, reason, ref_id)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		txnID, fromPeer, toPeer, amount, reason, refID,
	)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// FreezeCredits freezes an amount from available balance.
func (s *Store) FreezeCredits(peerID string, amount int64) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var balance int64
	err = tx.QueryRow(`SELECT CAST(balance AS INTEGER) FROM credit_accounts WHERE peer_id = ?`, peerID).Scan(&balance)
	if err != nil {
		return err
	}
	if balance < amount {
		return ErrInsufficientCredits
	}

	_, err = tx.Exec(
		`UPDATE credit_accounts SET balance = balance - ?, frozen = frozen + ?, updated_at = datetime('now')
		 WHERE peer_id = ?`, amount, amount, peerID,
	)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// UnfreezeCredits returns frozen credits back to available balance.
func (s *Store) UnfreezeCredits(peerID string, amount int64) error {
	_, err := s.DB.Exec(
		`UPDATE credit_accounts SET balance = balance + ?, frozen = frozen - ?, updated_at = datetime('now')
		 WHERE peer_id = ? AND frozen >= ?`,
		amount, amount, peerID, amount,
	)
	return err
}

// ListCreditTransactions returns recent transactions for a peer.
func (s *Store) ListCreditTransactions(peerID string, limit, offset int) ([]*CreditTransaction, error) {
	rows, err := s.DB.Query(
		`SELECT id, from_peer, to_peer, amount, reason, ref_id, created_at
		 FROM credit_transactions
		 WHERE from_peer = ? OR to_peer = ?
		 ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		peerID, peerID, limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var txns []*CreditTransaction
	for rows.Next() {
		t := &CreditTransaction{}
		if err := rows.Scan(&t.ID, &t.FromPeer, &t.ToPeer, &t.Amount, &t.Reason, &t.RefID, &t.CreatedAt); err != nil {
			return nil, err
		}
		txns = append(txns, t)
	}
	return txns, rows.Err()
}

// AddCredits adds credits to a peer (for initial grant, reputation bonus, etc.)
func (s *Store) AddCredits(txnID, peerID string, amount int64, reason string) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(
		`INSERT INTO credit_accounts (peer_id, balance, total_earned)
		 VALUES (?, ?, ?)
		 ON CONFLICT(peer_id) DO UPDATE SET
		   balance = balance + ?, total_earned = total_earned + ?, updated_at = datetime('now')`,
		peerID, amount, amount, amount, amount,
	)
	if err != nil {
		return err
	}

	_, err = tx.Exec(
		`INSERT INTO credit_transactions (id, from_peer, to_peer, amount, reason, ref_id)
		 VALUES (?, 'system', ?, ?, ?, '')`,
		txnID, peerID, amount, reason,
	)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// LogCreditAudit stores a credit audit record received from peers for supervision.
func (s *Store) LogCreditAudit(txnID, taskID, from, to string, amount int64, reason, eventTime string) error {
	_, err := s.DB.Exec(
		`INSERT OR IGNORE INTO credit_audit_log (txn_id, task_id, from_peer, to_peer, amount, reason, event_time)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		txnID, taskID, from, to, amount, reason, eventTime,
	)
	return err
}

// CreditAuditRecord represents a peer-broadcast credit audit entry.
type CreditAuditRecord struct {
	TxnID      string `json:"txn_id"`
	TaskID     string `json:"task_id"`
	FromPeer   string `json:"from_peer"`
	ToPeer     string `json:"to_peer"`
	Amount     int64  `json:"amount"`
	Reason     string `json:"reason"`
	EventTime  string `json:"event_time"`
	ReceivedAt string `json:"received_at"`
}

// ListCreditAudit returns recent credit audit records.
func (s *Store) ListCreditAudit(limit, offset int) ([]*CreditAuditRecord, error) {
	rows, err := s.DB.Query(
		`SELECT txn_id, task_id, from_peer, to_peer, amount, reason, event_time, received_at
		 FROM credit_audit_log ORDER BY received_at DESC LIMIT ? OFFSET ?`, limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*CreditAuditRecord
	for rows.Next() {
		r := &CreditAuditRecord{}
		if err := rows.Scan(&r.TxnID, &r.TaskID, &r.FromPeer, &r.ToPeer, &r.Amount, &r.Reason, &r.EventTime, &r.ReceivedAt); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	if result == nil {
		result = []*CreditAuditRecord{}
	}
	return result, nil
}

// ══════════════════════════════════════════════════════════
// Lobster Tier System — Social Energy Model v2.0
// ══════════════════════════════════════════════════════════
//
// 20 tiers named after real crustacean species and rare colour morphs,
// ordered by biological rarity — from the most abundant invasive crayfish
// to the near-mythical translucent Ghost Lobster.

// LobsterTier represents a node's rank in the network, themed after lobster rarity.
type LobsterTier struct {
	Level     int    `json:"level"`
	Name      string `json:"name"`
	NameEN    string `json:"name_en"`
	Emoji     string `json:"emoji"`
	MinEnergy int64  `json:"min_energy"`
}

// All tiers ordered by rarity (energy threshold).
// Named after real-world crustacean species and rare colour morphs.
//
//   Lv 1–4:   Common species (farmed / invasive / ubiquitous)
//   Lv 5–8:   Commercial species (restaurant-grade)
//   Lv 9–12:  Prized species (high-value / restricted range)
//   Lv 13–16: Rare & protected species (endangered / limited habitat)
//   Lv 17–20: Legendary colour morphs (genetic mutations, 1-in-millions)
var LobsterTiers = []LobsterTier{
	// ── Common (farmed / invasive) ──
	{Level: 1, Name: "克氏原螯虾", NameEN: "Red Swamp Crayfish", Emoji: "🦐", MinEnergy: 0},            // Procambarus clarkii — 全球入侵种，最常见的小龙虾
	{Level: 2, Name: "大理石纹螯虾", NameEN: "Marbled Crayfish", Emoji: "🦐", MinEnergy: 2},             // Procambarus virginalis — 孤雌生殖，自我克隆
	{Level: 3, Name: "信号小龙虾", NameEN: "Signal Crayfish", Emoji: "🦐", MinEnergy: 5},                // Pacifastacus leniusculus — 北美溪流常见种
	{Level: 4, Name: "红螯螯虾", NameEN: "Red Claw Crayfish", Emoji: "🦐", MinEnergy: 10},               // Cherax quadricarinatus — 澳洲养殖种

	// ── Commercial (restaurant-grade) ──
	{Level: 5, Name: "波士顿龙虾", NameEN: "American Lobster", Emoji: "🦞", MinEnergy: 18},              // Homarus americanus — 经典餐厅龙虾
	{Level: 6, Name: "欧洲龙虾", NameEN: "European Lobster", Emoji: "🦞", MinEnergy: 30},                // Homarus gammarus — 大西洋东岸，比波龙稀有
	{Level: 7, Name: "加州刺龙虾", NameEN: "California Spiny Lobster", Emoji: "🦞", MinEnergy: 50},      // Panulirus interruptus — 太平洋东岸
	{Level: 8, Name: "日本伊势龙虾", NameEN: "Japanese Spiny Lobster", Emoji: "🦞", MinEnergy: 80},      // Panulirus japonicus — 日本国宝级食材

	// ── Prized (high-value / restricted range) ──
	{Level: 9, Name: "锦绣龙虾", NameEN: "Ornate Spiny Lobster", Emoji: "🦞", MinEnergy: 120},           // Panulirus ornatus — 印太海域，花纹华丽
	{Level: 10, Name: "澳洲岩龙虾", NameEN: "Southern Rock Lobster", Emoji: "🦞", MinEnergy: 180},       // Jasus edwardsii — 澳洲高端出口
	{Level: 11, Name: "拖鞋龙虾", NameEN: "Mediterranean Slipper Lobster", Emoji: "🦞", MinEnergy: 260}, // Scyllarides latus — 地中海，外形奇特，种群下降
	{Level: 12, Name: "吉普斯兰刺螯虾", NameEN: "Gippsland Spiny Crayfish", Emoji: "🦞", MinEnergy: 380}, // Euastacus kershawi — 澳洲维州特有种，分布极窄

	// ── Rare & protected (endangered / limited habitat) ──
	{Level: 13, Name: "默里河螯虾", NameEN: "Murray Crayfish", Emoji: "🦞", MinEnergy: 550},             // Euastacus armatus — 澳洲第二大淡水螯虾，易危种
	{Level: 14, Name: "胡安费尔南德斯岩龙虾", NameEN: "Juan Fernández Rock Lobster", Emoji: "🦞", MinEnergy: 800}, // Jasus frontalis — 仅分布于智利胡安费尔南德斯群岛
	{Level: 15, Name: "塔斯马尼亚巨螯虾", NameEN: "Tasmanian Giant Crayfish", Emoji: "🦞", MinEnergy: 1200},      // Astacopsis gouldi — 世界最大淡水无脊椎动物，濒危
	{Level: 16, Name: "毛伊龙虾", NameEN: "Banded Spiny Lobster", Emoji: "🦞", MinEnergy: 1800},         // Panulirus marginatus — 夏威夷特有种，极度限域

	// ── Legendary colour morphs (genetic mutations, 1-in-millions) ──
	{Level: 17, Name: "蓝龙虾", NameEN: "Blue Lobster", Emoji: "💎", MinEnergy: 3000},                   // 基因突变，约 200 万分之一
	{Level: 18, Name: "双色龙虾", NameEN: "Split-Colored Lobster", Emoji: "🌗", MinEnergy: 5000},        // 雌雄嵌合体 (Gynandromorph)，约 5000 万分之一
	{Level: 19, Name: "白化龙虾", NameEN: "Albino Lobster", Emoji: "🤍", MinEnergy: 10000},              // 完全白化，约 1 亿分之一
	{Level: 20, Name: "幽灵龙虾", NameEN: "Ghost Lobster", Emoji: "👻", MinEnergy: 20000},               // 通体半透明，有记录以来仅数例
}

// GetTier returns the lobster tier for a given energy balance.
func GetTier(energy int64) LobsterTier {
	tier := LobsterTiers[0]
	for _, t := range LobsterTiers {
		if energy >= t.MinEnergy {
			tier = t
		}
	}
	return tier
}

// EnergyRegenRate returns 0 in Phase 0 (passive income disabled).
func EnergyRegenRate(prestige float64) float64 {
	return 0
}

// EnergyProfile is the full account view including tier and prestige info.
type EnergyProfile struct {
	PeerID      string      `json:"peer_id"`
	Energy      int64       `json:"energy"`
	Frozen      int64       `json:"frozen"`
	Prestige    float64     `json:"prestige"`
	Tier        LobsterTier `json:"tier"`
	RegenRate   float64     `json:"regen_rate"`
	TotalEarned int64       `json:"total_earned"`
	TotalSpent  int64       `json:"total_spent"`
	UpdatedAt   string      `json:"updated_at"`
}

// GetEnergyProfile returns the full energy profile for a peer.
func (s *Store) GetEnergyProfile(peerID string) (*EnergyProfile, error) {
	row := s.DB.QueryRow(
		`SELECT peer_id, CAST(balance AS INTEGER), CAST(frozen AS INTEGER),
		        prestige, CAST(total_earned AS INTEGER), CAST(total_spent AS INTEGER), updated_at
		 FROM credit_accounts WHERE peer_id = ?`, peerID,
	)
	var p EnergyProfile
	err := row.Scan(&p.PeerID, &p.Energy, &p.Frozen, &p.Prestige, &p.TotalEarned, &p.TotalSpent, &p.UpdatedAt)
	if err == sql.ErrNoRows {
		p = EnergyProfile{PeerID: peerID, Tier: LobsterTiers[0], RegenRate: 0}
		return &p, nil
	}
	if err != nil {
		return nil, err
	}
	p.Tier = GetTier(p.Energy)
	p.RegenRate = 0 // Phase 0: regen disabled
	return &p, nil
}

// RegenAllEnergy is disabled in Phase 0 (Shell system: no passive income).
// Returns 0 accounts updated.
func (s *Store) RegenAllEnergy() (int, error) {
	return 0, nil
}

// DecayAllPrestige applies daily prestige decay (factor 0.998) to all accounts.
// Returns number of accounts updated.
func (s *Store) DecayAllPrestige() (int, error) {
	res, err := s.DB.Exec(
		`UPDATE credit_accounts SET prestige = prestige * 0.998, updated_at = datetime('now')
		 WHERE prestige > 0.01`)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// AddPrestige adds prestige to a peer, weighted by the evaluator's prestige.
func (s *Store) AddPrestige(peerID string, amount float64, evaluatorPrestige float64) error {
	// Weight function: W(P) = 0.1 + 0.9 * (1 - e^(-P/50))
	weight := 0.1 + 0.9*(1.0-math.Exp(-evaluatorPrestige/50.0))
	gain := amount * weight
	if gain < 0.001 {
		return nil
	}
	_, err := s.DB.Exec(
		`UPDATE credit_accounts SET prestige = prestige + ?, updated_at = datetime('now')
		 WHERE peer_id = ?`, gain, peerID)
	return err
}

// BurnEnergy permanently removes energy from the system (deflationary).
func (s *Store) BurnEnergy(peerID string, amount int64) error {
	_, err := s.DB.Exec(
		`UPDATE credit_accounts SET balance = balance - ?, total_spent = total_spent + ?, updated_at = datetime('now')
		 WHERE peer_id = ? AND balance >= ?`,
		amount, amount, peerID, amount)
	return err
}

// LeaderboardEntry represents one row in the wealth leaderboard.
type LeaderboardEntry struct {
	Rank           int         `json:"rank"`
	PeerID         string      `json:"peer_id"`
	Energy         int64       `json:"energy"`
	Prestige       float64     `json:"prestige"`
	Tier           LobsterTier `json:"tier"`
	TasksCompleted int         `json:"tasks_completed"`
	Contributions  int         `json:"contributions"`
	TotalEarned    int64       `json:"total_earned"`
}

// GetWealthLeaderboard returns peers ranked by energy (descending).
func (s *Store) GetWealthLeaderboard(limit int) ([]*LeaderboardEntry, error) {
	rows, err := s.DB.Query(
		`SELECT c.peer_id, CAST(c.balance AS INTEGER), c.prestige, CAST(c.total_earned AS INTEGER),
		        COALESCE(r.tasks_completed, 0), COALESCE(r.contributions, 0)
		 FROM credit_accounts c
		 LEFT JOIN reputation r ON c.peer_id = r.peer_id
		 ORDER BY c.balance DESC
		 LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []*LeaderboardEntry
	rank := 0
	for rows.Next() {
		rank++
		e := &LeaderboardEntry{Rank: rank}
		if err := rows.Scan(&e.PeerID, &e.Energy, &e.Prestige, &e.TotalEarned,
			&e.TasksCompleted, &e.Contributions); err != nil {
			return nil, err
		}
		e.Tier = GetTier(e.Energy)
		entries = append(entries, e)
	}
	if entries == nil {
		entries = []*LeaderboardEntry{}
	}
	return entries, rows.Err()
}
