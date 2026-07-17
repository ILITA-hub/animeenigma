package queue

import (
	"context"
	"strconv"
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

	// enumCacheTTL bounds how often EnumerateAll's HTTP fan-out (capabilities
	// + per-provider translations/episodes) re-runs for one anime. It's an
	// ~90s call on a timeout-y title; at a 10s claim cadence with multiple
	// workers, an uncached enumeration would hammer catalog on every tick.
	// Staleness is safe: done-ness is decided against fresh DB rows
	// (PendingUnits/NextSkipTask read e.store every claim), never against
	// the enumeration itself — a stale enum can only under- or over-offer
	// candidate units, not misreport what's already verified.
	enumCacheTTL = 5 * time.Minute
)

// enumEntry is one cached EnumerateAll result.
type enumEntry struct {
	enum Enumeration
	at   time.Time
}

type Engine struct {
	cat         *catalogclient.Client
	sig         *signals.Signals
	store       *repo.Store
	reprobeTTL  time.Duration
	skipEnabled bool
	log         *logger.Logger

	// mu guards memb/membAt, enumCache, inflightUnits, and inflightProv: Claim
	// (potentially several worker goroutines) and Snapshot (HTTP handler)
	// share one Engine and can race on this state.
	mu   sync.Mutex
	memb *catalogclient.Membership

	membAt time.Time
	now    func() time.Time

	enumCache map[string]enumEntry // animeID → cached Enumeration

	// inflightUnits/inflightProv are the in-process claim leases that let
	// several workers Claim concurrently without double-probing the same
	// unit or hitting the same upstream provider at once. Claim keys: verify
	// unit → AnimeID+"|"+Provider+"|"+Key.String(); skip task → "skip|" +
	// primary-unit AnimeID+"|"+Provider+"|"+Team+"|"+Episode (prefixed so it
	// can never collide with a verify key).
	inflightUnits map[string]struct{}
	inflightProv  map[string]struct{}
}

func NewEngine(cat *catalogclient.Client, sig *signals.Signals, store *repo.Store, reprobeTTL time.Duration, skipEnabled bool, log *logger.Logger) *Engine {
	return &Engine{
		cat: cat, sig: sig, store: store, reprobeTTL: reprobeTTL, skipEnabled: skipEnabled, log: log, now: time.Now,
		enumCache:     map[string]enumEntry{},
		inflightUnits: map[string]struct{}{},
		inflightProv:  map[string]struct{}{},
	}
}

// membership caches the catalog membership fetch for membershipTTL. Like
// enumerate, the (up to 30s) HTTP call runs WITHOUT holding mu — mu also
// guards the lease/release hot path, and holding it across a slow fetch
// would stall every other worker's Claim and release. Two workers
// cold-missing concurrently both fetch; the second write overwrites the
// first with an equally-fresh result.
func (e *Engine) membership(ctx context.Context) *catalogclient.Membership {
	e.mu.Lock()
	if e.memb != nil && e.now().Sub(e.membAt) < membershipTTL {
		m := e.memb
		e.mu.Unlock()
		return m
	}
	e.mu.Unlock()

	m, err := e.cat.Membership(ctx)
	if err != nil {
		if e.log != nil {
			e.log.Warnw("membership fetch failed; reusing stale", "error", err)
		}
		e.mu.Lock()
		defer e.mu.Unlock()
		return e.memb // possibly nil — BuildCandidates tolerates it
	}
	e.mu.Lock()
	e.memb, e.membAt = m, e.now()
	e.mu.Unlock()
	return m
}

func (e *Engine) ranked(ctx context.Context) []Candidate {
	m := e.membership(ctx)
	visited := e.sig.VisitedAnime(ctx)
	cs := Rank(BuildCandidates(m, visited, func(id string) int { return e.sig.UniqueVisitors(ctx, id) }))
	cvmetrics.QueueDepth.Set(float64(len(cs)))
	return cs
}

// enumerate is EnumerateAll behind a per-anime TTL cache (see enumCacheTTL).
// The cache is only checked/populated under mu; the (possibly slow) HTTP
// fan-out itself runs WITHOUT holding mu — two workers cold-missing the same
// anime concurrently both fetch, and the second write just overwrites the
// first with an equally-fresh result. Only successful fetches are cached; an
// error always falls through to the caller's own (cooldown) handling.
func (e *Engine) enumerate(ctx context.Context, animeID string) (Enumeration, error) {
	e.mu.Lock()
	if entry, ok := e.enumCache[animeID]; ok && e.now().Sub(entry.at) < enumCacheTTL {
		e.mu.Unlock()
		return entry.enum, nil
	}
	e.mu.Unlock()

	enum, err := EnumerateAll(ctx, e.cat, animeID, e.log)
	if err != nil {
		return Enumeration{}, err
	}
	e.mu.Lock()
	e.enumCache[animeID] = enumEntry{enum: enum, at: e.now()}
	e.mu.Unlock()
	return enum, nil
}

// verifyClaimKey/skipClaimKey are the in-flight lease keys — see the
// inflightUnits doc comment on Engine.
func verifyClaimKey(u Unit) string { return u.AnimeID + "|" + u.Provider + "|" + u.Key.String() }

func skipClaimKey(u SkipUnit) string {
	return "skip|" + u.AnimeID + "|" + u.Provider + "|" + u.Team + "|" + strconv.Itoa(u.Episode)
}

// lease marks (provider, key) in-flight under mu and returns an idempotent
// release closure. ok is false — and release nil — when either provider or
// key is already held by another in-flight claim.
func (e *Engine) lease(provider, key string) (release func(), ok bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, blocked := e.inflightProv[provider]; blocked {
		return nil, false
	}
	if _, blocked := e.inflightUnits[key]; blocked {
		return nil, false
	}
	e.inflightProv[provider] = struct{}{}
	e.inflightUnits[key] = struct{}{}
	var once sync.Once
	return func() {
		once.Do(func() {
			e.mu.Lock()
			delete(e.inflightProv, provider)
			delete(e.inflightUnits, key)
			e.mu.Unlock()
		})
	}, true
}

// claimVerifyUnit walks pending in probe-priority order and leases the FIRST
// unit whose provider AND claim key are both free. Returns ok=false when
// every pending unit is currently leased by another in-flight claim.
func (e *Engine) claimVerifyUnit(pending []Unit) (unit *Unit, release func(), ok bool) {
	for _, u := range pending {
		if release, leased := e.lease(u.Provider, verifyClaimKey(u)); leased {
			uu := u
			return &uu, release, true
		}
	}
	return nil, nil, false
}

// Claim returns the single highest-priority pending verify unit or, once the
// verify lane is settled for a candidate and the skip lane is enabled, the
// next skip-probe task. Idle → (nil, nil, nil, nil). Error → (nil, nil, nil,
// err). The returned release func MUST be called when the probe (and its
// persist) completes; it frees the unit's in-flight lease. It is idempotent
// and never nil when a unit or task is returned.
//
// With CV_WORKERS>1 several goroutines call Claim concurrently: a candidate
// whose only pending work is already leased by another in-flight claim is
// skipped WITHOUT a cooldown (it still has real work, another worker just
// has it) and the loop moves to the next candidate instead of falling
// through to that candidate's skip lane.
func (e *Engine) Claim(ctx context.Context) (*Unit, *SkipTask, func(), error) {
	scanned := 0
	for _, cand := range e.ranked(ctx) {
		if scanned >= maxScan {
			break
		}
		if e.sig.InCooldown(ctx, cand.AnimeID) {
			continue
		}
		scanned++
		enum, err := e.enumerate(ctx, cand.AnimeID)
		if err != nil {
			if e.log != nil {
				e.log.Warnw("enumerate failed", "anime_id", cand.AnimeID, "error", err)
			}
			e.sig.SetCooldown(ctx, cand.AnimeID, time.Hour) // don't hammer a broken title
			continue
		}
		rows, err := e.store.ByAnime(ctx, cand.AnimeID)
		if err != nil {
			return nil, nil, nil, err
		}
		pending := PendingUnits(enum.Verify, rows, e.now(), e.reprobeTTL)
		if len(pending) > 0 {
			if u, release, ok := e.claimVerifyUnit(pending); ok {
				return u, nil, release, nil
			}
			// Every pending unit is leased by another in-flight claim — the
			// title isn't settled, so it must not be cooled down, and its
			// skip lane isn't due yet either (skip only starts once verify
			// settles). Move on to the next candidate.
			continue
		}
		if e.skipEnabled {
			skipRows, err := e.store.SkipByAnime(ctx, cand.AnimeID)
			if err != nil {
				return nil, nil, nil, err
			}
			fps, err := e.store.Fingerprints(ctx, cand.AnimeID)
			if err != nil {
				return nil, nil, nil, err
			}
			if task := NextSkipTask(enum.Skip, skipRows, fps, e.now()); task != nil {
				release, ok := e.lease(task.Unit.Provider, skipClaimKey(task.Unit))
				if !ok {
					// Blocked by another in-flight claim — same "don't cool a
					// title with real pending work" reasoning as the verify
					// lane above.
					continue
				}
				return nil, task, release, nil
			}
		}
		e.sig.SetCooldown(ctx, cand.AnimeID, CooldownTTL(cand.Ongoing))
	}
	return nil, nil, nil, nil
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
