package discovery

import (
	"math"
	"sort"
)

// AgentCandidate holds all data needed for ranking a candidate agent.
type AgentCandidate struct {
	PeerID         string   `json:"peer_id"`
	AgentName      string   `json:"agent_name"`
	Skills         []string `json:"skills"`
	Reputation     float64  `json:"reputation"`
	TasksCompleted int      `json:"tasks_completed"`
	TasksFailed    int      `json:"tasks_failed"`
	ActiveTasks    int      `json:"active_tasks"`
	AvgResponse    float64  `json:"avg_response_hours"` // hours
}

// RankedAgent is the output of the matching algorithm.
type RankedAgent struct {
	PeerID         string   `json:"peer_id"`
	AgentName      string   `json:"agent_name"`
	Score          float64  `json:"score"`
	TagOverlap     float64  `json:"tag_overlap"`
	MatchedTags    []string `json:"matched_tags"`
	Reputation     float64  `json:"reputation"`
	SuccessRate    float64  `json:"success_rate"`
	TasksCompleted int      `json:"tasks_completed"`
	ActiveTasks    int      `json:"active_tasks"`
	ColdStart      bool     `json:"cold_start"`
}

// MatchWeights controls the composite scoring formula.
// Default: reputation 0.3, success_rate 0.3, response_time 0.2, tag_overlap 0.2
type MatchWeights struct {
	Reputation   float64
	SuccessRate  float64
	ResponseTime float64
	TagOverlap   float64
}

// DefaultWeights returns the standard matching weights.
func DefaultWeights() MatchWeights {
	return MatchWeights{
		Reputation:   0.3,
		SuccessRate:  0.3,
		ResponseTime: 0.2,
		TagOverlap:   0.2,
	}
}

const (
	// ColdStartThreshold defines how many tasks before cold-start boost expires.
	ColdStartThreshold = 5
	// ColdStartBoost is bonus reputation points for new agents.
	ColdStartBoost = 10.0
	// MaxLoad is the maximum active tasks before an agent is excluded.
	MaxLoad = 3
)

// RankCandidates scores and ranks agents against a set of required tags.
// Agents with active_tasks > MaxLoad are excluded.
// New agents (< ColdStartThreshold completed tasks) get a reputation boost.
func RankCandidates(candidates []AgentCandidate, requiredTags []string, w MatchWeights) []RankedAgent {
	normalizedReq := NormalizeTags(requiredTags)

	var ranked []RankedAgent
	for _, c := range candidates {
		// Skip overloaded agents
		if c.ActiveTasks > MaxLoad {
			continue
		}

		agentTags := NormalizeTags(c.Skills)
		overlap, matched := TagOverlap(agentTags, normalizedReq)

		// Success rate: completed / (completed + failed), default 1.0 for new agents
		successRate := 1.0
		total := c.TasksCompleted + c.TasksFailed
		if total > 0 {
			successRate = float64(c.TasksCompleted) / float64(total)
		}

		// Response time score: 1.0 for instant, decays toward 0 for slow responders
		// Uses inverse: 1 / (1 + hours/24)
		responseScore := 1.0
		if c.AvgResponse > 0 {
			responseScore = 1.0 / (1.0 + c.AvgResponse/24.0)
		}

		// Effective reputation with cold-start boost
		effectiveRep := c.Reputation
		coldStart := c.TasksCompleted < ColdStartThreshold
		if coldStart {
			effectiveRep += ColdStartBoost
		}

		// Normalize reputation to 0-1 range (assuming max ~200, using sqrt scaling)
		repNorm := math.Sqrt(effectiveRep / 50.0)
		if repNorm > 2.0 {
			repNorm = 2.0
		}
		repNorm /= 2.0 // scale to 0-1

		// Composite score
		score := w.Reputation*repNorm +
			w.SuccessRate*successRate +
			w.ResponseTime*responseScore +
			w.TagOverlap*overlap

		ranked = append(ranked, RankedAgent{
			PeerID:         c.PeerID,
			AgentName:      c.AgentName,
			Score:          score,
			TagOverlap:     overlap,
			MatchedTags:    matched,
			Reputation:     c.Reputation,
			SuccessRate:    successRate,
			TasksCompleted: c.TasksCompleted,
			ActiveTasks:    c.ActiveTasks,
			ColdStart:      coldStart,
		})
	}

	sort.Slice(ranked, func(i, j int) bool {
		return ranked[i].Score > ranked[j].Score
	})
	return ranked
}
