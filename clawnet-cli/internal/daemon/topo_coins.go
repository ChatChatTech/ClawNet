package daemon

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"sync"
	"time"
)

// ── Topo Coin State Machine ──
//
// All coin logic runs inside the daemon process. No HTTP endpoint can
// create coins or award Shell — the daemon is the sole authority.
//
// The TUI reads visual state via GET /api/topo/coin-state (read-only)
// and sends parameter-less action signals via POST /api/topo/coin-grab
// and POST /api/topo/coin-redeem. These signals only advance the
// internal state machine; the daemon decides whether they are valid.

// coinState is the daemon-internal coin state machine.
type coinState struct {
	mu sync.Mutex

	// Spawn control
	lastSpawnAt      time.Time
	spawnCooldown    time.Duration
	beachExpiry      time.Duration // coins disappear after this
	beachCoins       int           // 0-3 coins currently on beach
	beachSpawnedAt   time.Time     // when current beach coins appeared
	chestCoins       int           // coins collected in chest (0-10)

	// Visual state for TUI
	coinPositions    [][2]int // [row, col] for each beach coin
	lastMessage      string
	messageExpiresAt time.Time
}

func newCoinState() *coinState {
	return &coinState{
		spawnCooldown: 45 * time.Second,
		beachExpiry:   2 * time.Minute,
	}
}

// CoinVisualState is the read-only visual state sent to TUI.
type CoinVisualState struct {
	BeachCoins    int      `json:"beach_coins"`
	ChestCoins    int      `json:"chest_coins"`
	CoinPositions [][2]int `json:"coin_positions,omitempty"` // [row, col] pairs
	Message       string   `json:"message,omitempty"`
}

// trySpawn is called internally by the daemon to potentially spawn coins.
// It uses network metrics to decide probability — no external trigger needed.
func (d *Daemon) coinTrySpawn() {
	d.coins.mu.Lock()
	defer d.coins.mu.Unlock()

	// Already have coins on beach or in chest — no new spawn
	if d.coins.beachCoins > 0 {
		return
	}

	// Rate limit
	if time.Since(d.coins.lastSpawnAt) < d.coins.spawnCooldown {
		return
	}

	// Expire beach coins if too old
	if d.coins.beachCoins > 0 && time.Since(d.coins.beachSpawnedAt) > d.coins.beachExpiry {
		d.coins.beachCoins = 0
		d.coins.coinPositions = nil
		return
	}

	// Calculate probability using real network metrics
	peers := float64(len(d.Node.ConnectedPeers()))
	var balance float64
	selfID := d.Node.PeerID().String()
	if ep, err := d.Store.GetEnergyProfile(selfID); err == nil && ep != nil {
		balance = float64(ep.Energy)
	}
	topics := float64(len(d.topicNames()))
	var activity float64
	if d.EchoBuf != nil {
		activity = float64(len(d.EchoBuf.Recent(50)))
	}

	// Dynamic probability — early network boost
	basePr := 0.50
	if peers < 3 {
		basePr = 0.90
	} else if peers < 10 {
		basePr = 0.75
	}
	pF := math.Max(0.3, math.Min(1.0, 1.0-peers/200.0))
	bF := math.Max(0.2, math.Min(1.0, 1.0-balance/50000.0))
	tF := math.Max(0.5, math.Min(1.0, 1.0-topics/30.0))
	aB := math.Min(0.5, activity/20.0)
	prob := basePr * pF * bF * tF * (1.0 + aB)

	if rand.Float64() >= prob {
		return
	}

	// Spawn 1-3 coins
	n := 1 + rand.Intn(3)
	d.coins.beachCoins = n
	d.coins.beachSpawnedAt = time.Now()
	d.coins.lastSpawnAt = time.Now()
	d.coins.coinPositions = make([][2]int, n)
	for i := 0; i < n; i++ {
		d.coins.coinPositions[i] = [2]int{2 + rand.Intn(4), 5 + rand.Intn(30)}
	}
}

// coinGrab moves beach coins to the chest. Returns true if coins were grabbed.
func (d *Daemon) coinGrab() bool {
	d.coins.mu.Lock()
	defer d.coins.mu.Unlock()

	if d.coins.beachCoins == 0 {
		return false
	}
	if d.coins.chestCoins+d.coins.beachCoins > 10 {
		return false
	}

	d.coins.chestCoins += d.coins.beachCoins
	d.coins.beachCoins = 0
	d.coins.coinPositions = nil
	return true
}

// coinRedeem converts chest coins to Shell. All logic is internal.
// Returns the Shell amount awarded, or 0 if nothing to redeem.
func (d *Daemon) coinRedeem(ctx context.Context) (int64, int) {
	d.coins.mu.Lock()
	count := d.coins.chestCoins
	if count == 0 {
		d.coins.mu.Unlock()
		return 0, 0
	}
	d.coins.chestCoins = 0
	d.coins.mu.Unlock()

	selfID := d.Node.PeerID().String()
	d.Store.EnsureCreditAccount(selfID, 0)

	amount := int64(count) * 100
	txnID := fmt.Sprintf("topo-coin-%s-%d", selfID[:8], time.Now().UnixMilli())
	d.Store.AddCredits(txnID, selfID, amount, "topo_coin_redeem")

	// Broadcast signed audit so other nodes can verify
	d.publishCreditAudit(ctx, txnID, "", "system", selfID, amount, "topo_coin_redeem")
	d.RecordEvent("coin_redeemed", selfID, txnID, fmt.Sprintf("Redeemed %d topo coins for %d Shell", count, amount))

	// Set message
	d.coins.mu.Lock()
	d.coins.lastMessage = fmt.Sprintf("+%d Shell deposited!", amount)
	d.coins.messageExpiresAt = time.Now().Add(6 * time.Second)
	d.coins.mu.Unlock()

	return amount, count
}

// coinVisualState returns the current visual state for the TUI.
func (d *Daemon) coinVisualState() CoinVisualState {
	d.coins.mu.Lock()
	defer d.coins.mu.Unlock()

	// Expire beach coins
	if d.coins.beachCoins > 0 && time.Since(d.coins.beachSpawnedAt) > d.coins.beachExpiry {
		d.coins.beachCoins = 0
		d.coins.coinPositions = nil
	}

	// Expire message
	msg := d.coins.lastMessage
	if msg != "" && time.Now().After(d.coins.messageExpiresAt) {
		d.coins.lastMessage = ""
		msg = ""
	}

	return CoinVisualState{
		BeachCoins:    d.coins.beachCoins,
		ChestCoins:    d.coins.chestCoins,
		CoinPositions: d.coins.coinPositions,
		Message:       msg,
	}
}

// ── HTTP handlers (minimal read-only state + parameter-less signals) ──

// handleCoinState returns read-only visual state for the TUI.
// Also triggers a spawn check (the act of viewing topo = active session).
func (d *Daemon) handleCoinState(w http.ResponseWriter, r *http.Request) {
	// The TUI polling this endpoint is the signal that topo is active.
	// Try to spawn coins on each poll.
	d.coinTrySpawn()
	writeJSON(w, d.coinVisualState())
}

// handleCoinGrab moves beach coins to chest. No parameters — daemon decides.
func (d *Daemon) handleCoinGrab(w http.ResponseWriter, r *http.Request) {
	ok := d.coinGrab()
	if !ok {
		http.Error(w, `{"error":"nothing to grab"}`, http.StatusBadRequest)
		return
	}
	writeJSON(w, d.coinVisualState())
}

// handleCoinRedeem converts chest coins to Shell. No parameters — daemon decides.
func (d *Daemon) handleCoinRedeemV2(w http.ResponseWriter, r *http.Request) {
	amount, count := d.coinRedeem(d.ctx)
	if count == 0 {
		http.Error(w, `{"error":"no coins to redeem"}`, http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]any{
		"status":  "ok",
		"coins":   count,
		"shell":   amount,
		"message": fmt.Sprintf("%d coins → %d Shell deposited!", count, amount),
	})
}
