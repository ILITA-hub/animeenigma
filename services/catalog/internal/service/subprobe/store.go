// Package subprobe implements the active subtitle-provider health probe: it
// pings the Jimaku + OpenSubtitles APIs on a fixed interval, classifies each as
// up/degraded/down, stores the latest verdict in an in-memory HealthStore, and
// emits probe_subtitle_* Prometheus gauges. The SubsAggregator reads the store
// to overlay provider_health on the /subtitles/all response.
package subprobe

import (
	"sync"
	"time"
)

// Status is the active-probe verdict for one provider.
type Status string

const (
	StatusUp       Status = "up"
	StatusDegraded Status = "degraded"
	StatusDown     Status = "down"
	StatusUnknown  Status = "unknown"
)

// Health is the latest active-probe verdict for one provider.
type Health struct {
	Status    Status
	LatencyMS int64
	CheckedAt time.Time
}

// Store holds the latest per-provider Health, guarded for concurrent
// probe writes + request-path reads.
type Store struct {
	mu         sync.RWMutex
	health     map[string]Health
	staleAfter time.Duration
	now        func() time.Time
}

// NewStore returns a Store. Entries whose CheckedAt is older than staleAfter are
// reported as StatusUnknown by Snapshot (never a stale "up").
func NewStore(staleAfter time.Duration) *Store {
	return &Store{
		health:     map[string]Health{},
		staleAfter: staleAfter,
		now:        time.Now,
	}
}

// Record stores the latest verdict for a provider.
func (s *Store) Record(provider string, h Health) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.health[provider] = h
}

// Snapshot returns a copy of all known provider health, downgrading any entry
// older than staleAfter to StatusUnknown.
func (s *Store) Snapshot() map[string]Health {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]Health, len(s.health))
	now := s.now()
	for p, h := range s.health {
		if now.Sub(h.CheckedAt) > s.staleAfter {
			h.Status = StatusUnknown
		}
		out[p] = h
	}
	return out
}
