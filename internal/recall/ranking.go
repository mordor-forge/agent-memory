package recall

import (
	"math"
	"sort"
	"time"

	"github.com/mordor-forge/agent-memory/pkg/memory"
)

const (
	defaultRecallLimit  = 100
	candidateMultiplier = 4
	maxCandidateLimit   = 200
	recencyHalfLife     = 30 * 24 * time.Hour
)

func expandedCandidateLimit(limit int) int {
	base := effectiveRecallLimit(limit)
	candidateLimit := base * candidateMultiplier
	if candidateLimit < base {
		candidateLimit = base
	}
	if candidateLimit > maxCandidateLimit {
		candidateLimit = maxCandidateLimit
	}
	return candidateLimit
}

func effectiveRecallLimit(limit int) int {
	if limit <= 0 {
		return defaultRecallLimit
	}
	return limit
}

func rerankHits(hits []memory.RecallHit, now time.Time) []memory.RecallHit {
	ranked := make([]memory.RecallHit, len(hits))
	copy(ranked, hits)

	for i := range ranked {
		ranked[i].Score = scoreHit(ranked[i], now)
	}

	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].Score != ranked[j].Score {
			return ranked[i].Score > ranked[j].Score
		}
		if ranked[i].Distance != ranked[j].Distance {
			return ranked[i].Distance < ranked[j].Distance
		}
		left := referenceTime(ranked[i].Memory)
		right := referenceTime(ranked[j].Memory)
		if !left.Equal(right) {
			return left.After(right)
		}
		return ranked[i].Memory.ID.String() < ranked[j].Memory.ID.String()
	})

	return ranked
}

func scoreHit(hit memory.RecallHit, now time.Time) float64 {
	similarity := clamp01(1 - hit.Distance)
	importance := clamp01(hit.Memory.Importance)
	confidence := clamp01(hit.Memory.Confidence)
	recency := recencyScore(referenceTime(hit.Memory), now)

	base := similarity*0.60 + importance*0.15 + confidence*0.15 + recency*0.10
	return base * kindMultiplier(hit.Memory.Kind) * statusMultiplier(hit.Memory.Status)
}

func referenceTime(mem memory.Memory) time.Time {
	if mem.LastObservedAt != nil {
		return *mem.LastObservedAt
	}
	if !mem.UpdatedAt.IsZero() {
		return mem.UpdatedAt
	}
	return mem.CreatedAt
}

func recencyScore(reference, now time.Time) float64 {
	if reference.IsZero() {
		return 0
	}
	if !now.After(reference) {
		return 1
	}
	age := now.Sub(reference)
	if age <= 0 {
		return 1
	}
	return math.Exp(-float64(age) / float64(recencyHalfLife))
}

func kindMultiplier(kind string) float64 {
	switch kind {
	case "fact_merge":
		return 1.08
	case "state_snapshot":
		return 1.05
	case "episode_digest":
		return 1.00
	default:
		return 1.00
	}
}

func statusMultiplier(status string) float64 {
	switch status {
	case "active":
		return 1.00
	case "superseded":
		return 0.60
	case "archived":
		return 0.30
	default:
		return 0.90
	}
}

func clamp01(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}
