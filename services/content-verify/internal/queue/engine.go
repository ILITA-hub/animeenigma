package queue

import (
	"context"
	"math/rand"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"

	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/catalogclient"
	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/cvmetrics"
	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/domain"
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

	// aniskipCoverageTTL bounds how often the AniSkip probe gate re-checks
	// one anime's per-episode coverage through the catalog proxy. AniSkip
	// data only grows (crowdsourced submissions), so staleness here means at
	// worst one redundant probe of an episode that just got covered. Kept
	// well above the claim cadence because uncovered episodes bypass the
	// catalog's positive cache and hit aniskip.com upstream every sweep.
	aniskipCoverageTTL = 6 * time.Hour
)

// enumEntry is one cached EnumerateAll result.
type enumEntry struct {
	enum Enumeration
	at   time.Time
}

// aniskipEntry is one anime's cached AniSkip coverage. Episodes are fetched
// lazily (only ones with probeable work), so the map can grow within the
// entry's TTL — `at` is set on first fetch and the whole entry expires
// together.
type aniskipEntry struct {
	cov AniskipCoverage
	at  time.Time
}

type Engine struct {
	cat         *catalogclient.Client
	sig         *signals.Signals
	store       *repo.Store
	reprobeTTL  time.Duration
	skipEnabled bool
	// pins is the parsed CV_PIN_ANIME operator directive: animeID → preferred
	// provider ("" = whole-title pin). Pinned titles rank above everything,
	// bypass cooldowns, and plan the preferred provider's skip family first.
	pins map[string]string
	log  *logger.Logger

	// weights/freshWindow/idleCooldown/idleWindow are the banded-prioritization
	// knobs (spec §3): weights is the per-claim [Band1,Band2,Band3] lottery,
	// freshWindow floats a just-aired ongoing to the front of Band 1,
	// idleCooldown is the settled cooldown for Band 3, idleWindow pages the
	// idle-sweep tail. rng is injectable so tests can force a deterministic
	// primary band; production uses rand.Float64.
	weights      [3]int
	freshWindow  time.Duration
	idleCooldown time.Duration
	idleWindow   int
	rng          func() float64

	// mu guards interestCache/interestAt, enumCache, aniskipCache, malIDs,
	// inflightUnits, and inflightProv: Claim (potentially several worker
	// goroutines) and Snapshot (HTTP handler) share one Engine and can race
	// on this state.
	mu            sync.Mutex
	interestCache *catalogclient.Interest
	interestAt    time.Time

	now func() time.Time

	enumCache    map[string]enumEntry    // animeID → cached Enumeration
	aniskipCache map[string]aniskipEntry // animeID → cached AniSkip coverage
	malIDs       map[string]string       // animeID → MAL id (immutable, cached forever)

	// inflightUnits/inflightProv are the in-process claim leases that let
	// several workers Claim concurrently without double-probing the same
	// unit or hitting the same upstream provider at once. Claim keys: verify
	// unit → AnimeID+"|"+Provider+"|"+Key.String(); skip task → "skip|" +
	// primary-unit AnimeID+"|"+Provider+"|"+Team+"|"+Episode (prefixed so it
	// can never collide with a verify key).
	inflightUnits map[string]struct{}
	inflightProv  map[string]struct{}
}

func NewEngine(cat *catalogclient.Client, sig *signals.Signals, store *repo.Store, reprobeTTL time.Duration, skipEnabled bool, pins map[string]string, weights [3]int, freshWindow, idleCooldown time.Duration, idleWindow int, log *logger.Logger) *Engine {
	if pins == nil {
		pins = map[string]string{}
	}
	return &Engine{
		cat: cat, sig: sig, store: store, reprobeTTL: reprobeTTL, skipEnabled: skipEnabled, pins: pins,
		weights: weights, freshWindow: freshWindow, idleCooldown: idleCooldown, idleWindow: idleWindow,
		rng: rand.Float64, log: log, now: time.Now,
		enumCache:     map[string]enumEntry{},
		aniskipCache:  map[string]aniskipEntry{},
		malIDs:        map[string]string{},
		inflightUnits: map[string]struct{}{},
		inflightProv:  map[string]struct{}{},
	}
}

// interest fetches the banded interest snapshot behind membershipTTL. On a
// fresh fetch it advances the idle sweep cursor by idleWindow (wrapping at
// idle_total) so successive refreshes walk the catalog tail. The HTTP call
// runs WITHOUT holding mu (same reasoning as the old membership()).
func (e *Engine) interest(ctx context.Context) *catalogclient.Interest {
	e.mu.Lock()
	if e.interestCache != nil && e.now().Sub(e.interestAt) < membershipTTL {
		it := e.interestCache
		e.mu.Unlock()
		return it
	}
	e.mu.Unlock()

	offset := e.sig.IdleCursor(ctx)
	it, err := e.cat.InterestBands(ctx, offset, e.idleWindow)
	if err != nil {
		if e.log != nil {
			e.log.Warnw("interest fetch failed; reusing stale", "error", err)
		}
		e.mu.Lock()
		defer e.mu.Unlock()
		return e.interestCache // possibly nil — BuildCandidates tolerates it
	}
	e.sig.AdvanceIdleCursor(ctx, e.idleWindow, it.IdleTotal)
	e.mu.Lock()
	e.interestCache, e.interestAt = it, e.now()
	e.mu.Unlock()
	return it
}

// bandedCandidates returns candidates concatenated in this claim's band
// try-order (pins, lottery-primary band, then the rest), each band's slice
// intra-sorted. The lottery gives the weighting; fall-through means an empty
// higher band never wastes the tick.
func (e *Engine) bandedCandidates(ctx context.Context) []Candidate {
	it := e.interest(ctx)
	visited := e.sig.VisitedAnime(ctx)
	all := BuildCandidates(it, visited, e.pins, func(id string) int { return e.sig.UniqueVisitors(ctx, id) })
	cvmetrics.QueueDepth.Set(float64(len(all)))

	groups := map[Band][]Candidate{}
	for _, c := range all {
		b := BandOf(c)
		groups[b] = append(groups[b], c)
	}
	now := e.now()
	for b := range groups {
		g := groups[b]
		sort.SliceStable(g, func(i, j int) bool { return IntraLess(g[i], g[j], now, e.freshWindow) })
		groups[b] = g
	}
	order := bandOrder(e.weights, e.rng())
	out := make([]Candidate, 0, len(all))
	for _, b := range order {
		out = append(out, groups[b]...)
	}
	return out
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

// malID resolves an anime's MAL id via the catalog, behind a forever cache
// (the mapping is immutable; an empty id from a successful response means
// "no MAL mapping" and is cached too). "" on fetch error — not cached, so
// the next sweep retries.
func (e *Engine) malID(ctx context.Context, animeID string) string {
	e.mu.Lock()
	if id, ok := e.malIDs[animeID]; ok {
		e.mu.Unlock()
		return id
	}
	e.mu.Unlock()

	id, err := e.cat.AnimeMalID(ctx, animeID)
	if err != nil {
		if e.log != nil {
			e.log.Warnw("mal id fetch failed", "anime_id", animeID, "error", err)
		}
		return ""
	}
	e.mu.Lock()
	e.malIDs[animeID] = id
	e.mu.Unlock()
	return id
}

// aniskipSweepCap bounds per-episode coverage fetches per Claim so a
// 1000-episode title can't stall one worker's tick on a cold cache — the
// remainder stays "missing" and the next Claims finish the sweep.
const aniskipSweepCap = 50

// aniskipCoverage returns the AniSkip probe-gate coverage for animeID (see
// aniskipgate.go), fetching — through the catalog's pure-AniSkip proxy —
// only episodes that still have probeable work. The cached coverage map is
// copy-on-write: entries returned to callers are never mutated afterwards,
// so reads outside mu are safe. nil (gate off, probe everything) when there
// is nothing due, no MAL mapping, or nothing could be fetched.
func (e *Engine) aniskipCoverage(ctx context.Context, animeID string, units []SkipUnit, rows []domain.SkipTiming) AniskipCoverage {
	rowByKey := skipRowIndex(rows)
	needed := map[int]bool{}
	for _, u := range units {
		// Episodes with only terminal rows need no coverage: the planner
		// won't probe them regardless.
		if rowDue(rowFor(rowByKey, u), e.now()) {
			needed[u.Episode] = true
		}
	}
	if len(needed) == 0 {
		return nil
	}

	e.mu.Lock()
	entry, fresh := e.aniskipCache[animeID]
	if fresh && e.now().Sub(entry.at) >= aniskipCoverageTTL {
		fresh = false
	}
	known := AniskipCoverage(nil)
	if fresh {
		known = entry.cov
	}
	var missing []int
	for ep := range needed {
		if _, fetched := known[ep]; !fetched {
			missing = append(missing, ep)
		}
	}
	e.mu.Unlock()

	if len(missing) == 0 {
		return known
	}
	sort.Ints(missing)
	if len(missing) > aniskipSweepCap {
		missing = missing[:aniskipSweepCap]
	}

	malID := e.malID(ctx, animeID)
	if malID == "" {
		return known
	}

	fetched := map[int][]string{}
	for _, ep := range missing {
		kinds, err := e.cat.AniskipKinds(ctx, malID, ep)
		if err != nil {
			// Not recorded → retried next sweep. ctx errors abort the rest.
			if e.log != nil {
				e.log.Warnw("aniskip coverage fetch failed",
					"anime_id", animeID, "mal_id", malID, "episode", ep, "error", err)
			}
			if ctx.Err() != nil {
				break
			}
			continue
		}
		if kinds == nil {
			kinds = []string{} // "checked, uncovered" — see AniskipCoverage
		}
		fetched[ep] = kinds
	}
	if len(fetched) == 0 {
		return known
	}

	e.mu.Lock()
	cur, ok := e.aniskipCache[animeID]
	if !ok || e.now().Sub(cur.at) >= aniskipCoverageTTL {
		cur = aniskipEntry{cov: AniskipCoverage{}, at: e.now()}
	}
	merged := make(AniskipCoverage, len(cur.cov)+len(fetched))
	for k, v := range cur.cov {
		merged[k] = v
	}
	for k, v := range fetched {
		merged[k] = v
	}
	e.aniskipCache[animeID] = aniskipEntry{cov: merged, at: cur.at}
	e.mu.Unlock()
	return merged
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
	for _, cand := range e.bandedCandidates(ctx) {
		if scanned >= maxScan {
			break
		}
		// Pinned titles bypass cooldowns — an operator pin means "look at
		// this NOW", and a stale settled-cooldown must not mute it.
		if !cand.Pinned && e.sig.InCooldown(ctx, cand.AnimeID) {
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
			skipUnits := enum.Skip
			if pref := e.pins[cand.AnimeID]; pref != "" {
				skipUnits = PreferProvider(skipUnits, pref)
			}
			// AniSkip probe gate: don't spend probes on sides AniSkip
			// already covers (owner directive 2026-07-18) — see aniskipgate.go.
			cov := e.aniskipCoverage(ctx, cand.AnimeID, skipUnits, skipRows)
			skipUnits = FilterAniskipCovered(skipUnits, cov)
			if task := NextSkipTask(skipUnits, skipRows, fps, e.now()); task != nil {
				if task.Pair == nil {
					task.CoveredKinds = cov.CoveredKinds(task.Unit.Episode)
				}
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
		e.sig.SetCooldown(ctx, cand.AnimeID, CooldownTTL(BandOf(cand), e.idleCooldown))
	}
	return nil, nil, nil, nil
}

type QueueEntry struct {
	AnimeID  string `json:"anime_id"`
	Name     string `json:"name"`
	Band     int    `json:"band"`
	Ongoing  bool   `json:"ongoing"`
	Top      bool   `json:"top"`
	Visitors int    `json:"visitors"`
	Cooling  bool   `json:"cooling"`
}

// Snapshot renders the computed queue for the admin/debug endpoint.
func (e *Engine) Snapshot(ctx context.Context, limit int) []QueueEntry {
	out := []QueueEntry{}
	for i, c := range e.bandedCandidates(ctx) {
		if i >= limit {
			break
		}
		out = append(out, QueueEntry{AnimeID: c.AnimeID, Name: c.Name, Band: int(BandOf(c)),
			Ongoing: c.Ongoing, Top: c.Top, Visitors: c.Visitors,
			Cooling: e.sig.InCooldown(ctx, c.AnimeID)})
	}
	return out
}
