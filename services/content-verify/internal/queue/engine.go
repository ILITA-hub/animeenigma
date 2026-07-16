package queue

import (
	"context"
	"sync"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"

	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/catalogclient"
	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/cvmetrics"
	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/repo"
	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/signals"
)

const (
	membershipTTL = 10 * time.Minute
	maxScan       = 15 // candidates inspected per claim tick
)

type Engine struct {
	cat        *catalogclient.Client
	sig        *signals.Signals
	store      *repo.Store
	reprobeTTL time.Duration
	log        *logger.Logger

	// mu guards memb/membAt: Claim (worker goroutine) and Snapshot (HTTP
	// handler) share one Engine and can race on the membership cache.
	mu     sync.Mutex
	memb   *catalogclient.Membership
	membAt time.Time
	now    func() time.Time
}

func NewEngine(cat *catalogclient.Client, sig *signals.Signals, store *repo.Store, reprobeTTL time.Duration, log *logger.Logger) *Engine {
	return &Engine{cat: cat, sig: sig, store: store, reprobeTTL: reprobeTTL, log: log, now: time.Now}
}

func (e *Engine) membership(ctx context.Context) *catalogclient.Membership {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.memb != nil && e.now().Sub(e.membAt) < membershipTTL {
		return e.memb
	}
	m, err := e.cat.Membership(ctx)
	if err != nil {
		if e.log != nil {
			e.log.Warnw("membership fetch failed; reusing stale", "error", err)
		}
		return e.memb // possibly nil — BuildCandidates tolerates it
	}
	e.memb, e.membAt = m, e.now()
	return m
}

func (e *Engine) ranked(ctx context.Context) []Candidate {
	m := e.membership(ctx)
	visited := e.sig.VisitedAnime(ctx)
	cs := Rank(BuildCandidates(m, visited, func(id string) int { return e.sig.UniqueVisitors(ctx, id) }))
	cvmetrics.QueueDepth.Set(float64(len(cs)))
	return cs
}

// Claim returns the single highest-priority pending unit, or (nil, false,
// nil) when the queue is idle. One unit per tick — hot titles preempt
// naturally between units of a slower title.
func (e *Engine) Claim(ctx context.Context) (*Unit, bool, error) {
	scanned := 0
	for _, cand := range e.ranked(ctx) {
		if scanned >= maxScan {
			break
		}
		if e.sig.InCooldown(ctx, cand.AnimeID) {
			continue
		}
		scanned++
		units, err := EnumerateUnits(ctx, e.cat, cand.AnimeID, e.log)
		if err != nil {
			if e.log != nil {
				e.log.Warnw("enumerate failed", "anime_id", cand.AnimeID, "error", err)
			}
			e.sig.SetCooldown(ctx, cand.AnimeID, time.Hour) // don't hammer a broken title
			continue
		}
		rows, err := e.store.ByAnime(ctx, cand.AnimeID)
		if err != nil {
			return nil, false, err
		}
		pending := PendingUnits(units, rows, e.now(), e.reprobeTTL)
		if len(pending) == 0 {
			e.sig.SetCooldown(ctx, cand.AnimeID, CooldownTTL(cand.Ongoing))
			continue
		}
		u := pending[0]
		return &u, cand.Ongoing, nil
	}
	return nil, false, nil
}

type QueueEntry struct {
	AnimeID  string `json:"anime_id"`
	Name     string `json:"name"`
	Score    int    `json:"score"`
	Ongoing  bool   `json:"ongoing"`
	Top      bool   `json:"top"`
	Visitors int    `json:"visitors"`
	Cooling  bool   `json:"cooling"`
}

// Snapshot renders the computed queue for the admin/debug endpoint.
func (e *Engine) Snapshot(ctx context.Context, limit int) []QueueEntry {
	out := []QueueEntry{}
	for i, c := range e.ranked(ctx) {
		if i >= limit {
			break
		}
		out = append(out, QueueEntry{AnimeID: c.AnimeID, Name: c.Name, Score: c.Score(),
			Ongoing: c.Ongoing, Top: c.Top, Visitors: c.Visitors,
			Cooling: e.sig.InCooldown(ctx, c.AnimeID)})
	}
	return out
}
