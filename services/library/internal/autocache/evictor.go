package autocache

import (
	"context"
	"sync"
	"time"

	"github.com/ILITA-hub/animeenigma/services/library/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/library/internal/metrics"
)

// evictor.go is the Phase-10 budget arithmetic + ordered Stale eviction + periodic
// sweep + Accountant gauge publishing (EVICT-01..04). It is a sibling of the
// Phase-09 Planner in the same package: it copies the Planner's local-interface-seam
// idiom (so the package imports no repo/service/minio package — main.go owns the
// concrete wiring) and its config-gated ticker lifecycle, and composes the Plan-01
// repo/minio/metrics primitives.
//
// One place owns the budget logic (EnsureRoom) beside the freshness rules (Classify)
// so both the admin upload path and the Planner gate the SAME helper (PATTERNS flag 1).
// The pre-admit wiring into the Planner/handler/main.go is Plan 03.
//
// configGetter + plannerLogger are REUSED from planner.go (same package) — they are
// NOT redeclared here.

// --- interface seams (satisfied by the existing repos/minio writer; main.go owns
// the concrete wiring so this package imports no repo/minio package) ---

// poolAccountant reads pool bytes + the ordered Stale candidate list and deletes a
// row by id (the slice of *repo.EpisodeRepository the Evictor needs — Plan 01).
type poolAccountant interface {
	SumPoolBytes(ctx context.Context) (int64, error)
	ListStaleEvictionCandidates(ctx context.Context, cfg *domain.AutocacheConfig, now time.Time) ([]domain.Episode, error)
	DeleteByID(ctx context.Context, id string) error
	// ListPool returns every aeProvider/ pool row so the Accountant sweep can bucket
	// bytes_used + episode count per (source, freshness) via Classify (Plan 02).
	ListPool(ctx context.Context) ([]domain.Episode, error)
}

// jobAccountant reads the in-flight admitted-but-not-yet-materialized autocache job
// bytes (*repo.JobRepository.SumInflightJobBytes — WR-01). main.go injects the concrete
// JobRepository; nil is allowed (an Evictor built with no jobs seam enforces the budget
// against materialized pool rows only, the pre-WR-01 behavior).
type jobAccountant interface {
	SumInflightJobBytes(ctx context.Context) (int64, error)
}

// objectDeleter is the storage seam (storagegw.Gateway.DeletePrefix). It deletes
// every object under a row's minio_path prefix on the given backend, hard-failing
// on the first error so the Evictor can leave the row intact and retry rather than
// orphan a serving pointer. The Evictor only ever evicts storage='minio' rows
// (its candidate queries scope to minio), so the backend is effectively always
// minio — but passing the row's Storage keeps it correct if that ever widens.
type objectDeleter interface {
	DeletePrefix(ctx context.Context, storage, prefix string) error
}

// Freshness is the gauge-label string for an episode's Fresh/Stale classification.
// The Accountant Sets the bytes_used / episodes GaugeVecs keyed by (source, freshness).
type Freshness string

const (
	FreshnessFresh Freshness = "fresh"
	FreshnessStale Freshness = "stale"
)

// daysAgo subtracts d days from now. Used by Classify to compute the Fresh cutoff.
func daysAgo(now time.Time, d int) time.Time {
	return now.AddDate(0, 0, -d)
}

// Classify returns Fresh or Stale for a single episode row, evaluated at `now`
// against the live source-specific freshness windows (EVICT-02). It is a pure
// function (no Postgres) so the freshness rule is unit-testable even though the SQL
// ListStaleEvictionCandidates query does the actual filtering for eviction.
//
// Rules (CONTEXT lines 30-34):
//   - autocache Fresh ⟺ (downloaded_at != nil AND downloaded_at > now-auto_fresh_download_days)
//     OR (last_fetch_at != nil AND last_fetch_at > now-auto_fresh_fetch_days)
//   - admin Fresh ⟺ (downloaded_at != nil AND downloaded_at > now-admin_fresh_days)
//     OR (last_fetch_at != nil AND last_fetch_at > now-admin_fresh_days)
//   - else Stale.
//
// NULL handling: a nil downloaded_at contributes NOTHING to rule 1 ("very old" —
// classifies by last_fetch_at only); a nil last_fetch_at = never fetched, contributing
// nothing to rule 2.
func Classify(ep domain.Episode, cfg *domain.AutocacheConfig, now time.Time) Freshness {
	var downloadWindowDays, fetchWindowDays int
	if ep.Source == domain.EpisodeSourceAutocache {
		downloadWindowDays = cfg.AutoFreshDownloadDays
		fetchWindowDays = cfg.AutoFreshFetchDays
	} else {
		// admin (and any unknown source defaults to the admin window — safest, longest).
		downloadWindowDays = cfg.AdminFreshDays
		fetchWindowDays = cfg.AdminFreshDays
	}

	// Rule 1: recently downloaded.
	if ep.DownloadedAt != nil && ep.DownloadedAt.After(daysAgo(now, downloadWindowDays)) {
		return FreshnessFresh
	}
	// Rule 2: recently fetched (viewed).
	if ep.LastFetchAt != nil && ep.LastFetchAt.After(daysAgo(now, fetchWindowDays)) {
		return FreshnessFresh
	}
	return FreshnessStale
}

// Evictor owns the budget arithmetic (EnsureRoom), the freshness rules (Classify),
// the ordered Stale eviction (evictOne), and the periodic Accountant sweep (Sweep).
// One mutex serializes EnsureRoom (pre-admit) against Sweep so freed headroom is
// never double-spent (T-10-04).
type Evictor struct {
	config  configGetter
	pool    poolAccountant
	jobs    jobAccountant // in-flight reservation (WR-01); nil → materialized-only budget.
	objects objectDeleter
	metrics *metrics.LibraryMetrics
	log     plannerLogger

	// mu serializes the WHOLE read-budget→evict→delete cycle in BOTH EnsureRoom
	// (pre-admit) and Sweep (the ticker) so the admin handler, the Planner, and the
	// sweep can't race-double-spend the same freed headroom (CONTEXT line 99).
	mu sync.Mutex

	stop chan struct{}
	done chan struct{}
}

// NewEvictor wires the Evictor from its seams. jobs (the WR-01 in-flight reservation),
// metrics, and log may be nil (the Evictor is nil-guarded throughout via the metrics
// methods, the log nil-checks, and the jobs nil-check in ensureRoomLocked).
func NewEvictor(
	config configGetter,
	pool poolAccountant,
	jobs jobAccountant,
	objects objectDeleter,
	libMetrics *metrics.LibraryMetrics,
	log plannerLogger,
) *Evictor {
	return &Evictor{
		config:  config,
		pool:    pool,
		jobs:    jobs,
		objects: objects,
		metrics: libMetrics,
		log:     log,
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
	}
}

// EnsureRoom evicts Stale rows in the locked 4-tier order until estBytes fits under
// budget, or returns admitted=false when the entire Stale queue is exhausted and still
// short (EVICT-04). It holds e.mu for the whole read→evict cycle so a concurrent
// sweep/pre-admit can't double-spend the same headroom (T-10-04).
//
// estBytes is the pre-admit estimate of the incoming download (0 means "is the pool
// currently within budget?" — the semantics Sweep reuses for proactive reclaim).
func (e *Evictor) EnsureRoom(ctx context.Context, estBytes int64) (admitted bool, err error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.ensureRoomLocked(ctx, estBytes)
}

// ensureRoomLocked is the reclaim core shared by EnsureRoom (which takes e.mu) and
// Sweep's reclaim half (which takes e.mu around just this call — WR-02). It reads cfg +
// the used-bytes total live (materialized pool + in-flight reservation — WR-01),
// short-circuits when the estimate already fits, then evicts Stale candidates in the
// SQL-locked 4-tier order until the estimate fits or the queue is exhausted, and
// re-measures the used total once more before deciding admission (WR-04). The caller
// MUST hold e.mu for the WHOLE call.
func (e *Evictor) ensureRoomLocked(ctx context.Context, estBytes int64) (admitted bool, err error) {
	cfg, err := e.config.Get(ctx) // live budget + freshness windows (no boot-cache)
	if err != nil {
		return false, err
	}

	used, err := e.usedBytesLocked(ctx) // materialized pool + in-flight reservation
	if err != nil {
		return false, err
	}

	if used+estBytes <= cfg.BudgetBytes {
		return true, nil // fits already — no eviction
	}

	cands, err := e.pool.ListStaleEvictionCandidates(ctx, cfg, time.Now())
	if err != nil {
		return false, err
	}

	for i := range cands {
		if used+estBytes <= cfg.BudgetBytes {
			break
		}
		if evErr := e.evictOne(ctx, cands[i]); evErr != nil {
			// object-delete failed → leave the row, skip this candidate, keep going
			// (never orphan a serving pointer). Do NOT count the byte reclaim.
			if e.log != nil {
				e.log.Warnw("autocache evictor: evict candidate failed, skipping",
					"id", cands[i].ID, "path", cands[i].MinioPath, "error", evErr)
			}
			continue
		}
		if cands[i].SizeBytes != nil {
			used -= *cands[i].SizeBytes
		}
		e.metrics.IncEvictedTotal(string(cands[i].Source))
	}

	// WR-04: the loop above decremented `used` from a base measured BEFORE the evict
	// loop ran, so the admit decision would mix `used` from one instant with the
	// candidate list from a later instant (a concurrent insert/delete between the two
	// reads is not reflected). Re-measure under the still-held mutex so the boundary
	// decision (used exactly at budget) is made off a single consistent snapshot
	// rather than the decremented projection. We stay inside e.mu for the whole
	// read→evict→re-measure cycle, so this introduces no new TOCTOU.
	final, err := e.usedBytesLocked(ctx)
	if err != nil {
		return false, err
	}
	return final+estBytes <= cfg.BudgetBytes, nil
}

// usedBytesLocked returns the budget numerator: Σ materialized aeProvider pool bytes
// PLUS the in-flight reservation (Σ size_bytes of non-terminal autocache jobs admitted
// but not yet materialized into a pool row — WR-01). The caller MUST hold e.mu. The
// in-flight term closes the admit→materialize gap so concurrent / sequential admits
// against a stale snapshot cannot over-admit by ΣestBytes. The jobs seam is nil-guarded
// (a nil seam = materialized-only budget, the pre-WR-01 behavior).
func (e *Evictor) usedBytesLocked(ctx context.Context) (int64, error) {
	used, err := e.pool.SumPoolBytes(ctx) // Σ size_bytes over the aeProvider pool
	if err != nil {
		return 0, err
	}
	if e.jobs != nil {
		inflight, err := e.jobs.SumInflightJobBytes(ctx)
		if err != nil {
			return 0, err
		}
		used += inflight
	}
	return used, nil
}

// evictOne deletes one episode's MinIO objects FIRST, then its row only on success
// (T-10-05). DeletePrefix hard-fails on the first RemoveObject error; on that error
// evictOne returns WITHOUT deleting the row so the caller skips this candidate (never
// an orphaned row pointing at deleted objects — tolerate orphaned objects over an
// orphaned serving pointer). On DeletePrefix success it deletes the row; a row-delete
// error is returned (the objects are already gone — the lesser evil, and a re-list
// won't re-find live objects).
func (e *Evictor) evictOne(ctx context.Context, ep domain.Episode) error {
	if err := e.objects.DeletePrefix(ctx, ep.Storage, ep.MinioPath); err != nil {
		return err // objects (partially) survive → leave the row, caller continues
	}
	if err := e.pool.DeleteByID(ctx, ep.ID); err != nil {
		if e.log != nil {
			e.log.Errorw("autocache evictor: row delete failed after object delete",
				"id", ep.ID, "path", ep.MinioPath, "error", err)
		}
		return err
	}
	return nil
}

// --- ticker lifecycle (mirrors planner.go:138-215, 394-407) ---

// Start launches the sweep-loop goroutine. It returns immediately; the loop runs
// until Stop() is called or ctx is cancelled. Mirrors the Planner lifecycle.
func (e *Evictor) Start(ctx context.Context) {
	go e.loop(ctx)
}

// Stop signals the loop to exit and waits for it (bounded by the in-flight sleep).
// Idempotent — safe to call once or more.
func (e *Evictor) Stop() {
	select {
	case <-e.stop:
		// already stopped
	default:
		close(e.stop)
	}
	<-e.done
}

// loop is the ctx-aware sweep loop. Each iteration re-reads config live, sweeps if
// enabled, and re-reads the cadence (sweep_interval_min) every tick so an admin PATCH
// takes effect without a redeploy. Mirrors planner.go's loop.
func (e *Evictor) loop(ctx context.Context) {
	defer close(e.done)
	for {
		cadence := e.runOnce(ctx)
		if !e.sleep(ctx, cadence) {
			return
		}
	}
}

// runOnce performs one sweep and returns the cadence to sleep before the next. The
// unit drives runOnce directly so it never has to wait on a ticker. When the master
// switch is off it evicts nothing and publishes nothing (POOL-05 / T-10-06 — the
// disabled switch halts eviction too, matching the Planner's guard).
func (e *Evictor) runOnce(ctx context.Context) time.Duration {
	cfg, err := e.config.Get(ctx)
	if err != nil {
		if e.log != nil {
			e.log.Warnw("autocache evictor: config read failed, skipping sweep", "error", err)
		}
		return minSweepInterval
	}
	cadence := time.Duration(cfg.SweepIntervalMin) * time.Minute
	if cadence < minSweepInterval {
		cadence = minSweepInterval
	}
	if !cfg.Enabled {
		return cadence
	}
	e.Sweep(ctx)
	return cadence
}

// Sweep is the periodic Accountant + proactive reclaim. Two halves with DIFFERENT
// locking (WR-02): (a) the proactive reclaim mutates the pool and runs under e.mu (the
// SAME lock EnsureRoom takes) so pre-admit and sweep cannot double-spend freed headroom
// (T-10-04); (b) the gauge refresh is read-only and runs with NO lock held, so a slow
// reclaim of many/large prefixes cannot head-of-line-block the synchronous admin upload
// handler. (a) reuses the EnsureRoom reclaim core with estBytes=0 ("is the pool
// currently within budget? if not, evict Stale until it is"); (b) refreshes the
// bytes_used / budget_bytes / episodes gauges by listing the pool once and
// Classify-bucketing each row. Sweep is the gauge source even when no download flows,
// so Phase 11 always has live series.
func (e *Evictor) Sweep(ctx context.Context) {
	// (a) Proactive reclaim: bring the pool to/under budget (estBytes=0). This is the
	// ONLY half that mutates the pool, so it is the ONLY half that needs e.mu (the
	// double-spend guard vs. a concurrent pre-admit). WR-02: we hold e.mu across the
	// reclaim ONLY — NOT across the read-only gauge refresh below — so a slow reclaim
	// of many/large prefixes no longer head-of-line-blocks the synchronous admin
	// upload handler (which blocks on the same e.mu inside the HTTP request, toward
	// the 120s WriteTimeout). The reclaim itself stays fully serialized, so no
	// double-spend / TOCTOU is introduced; only the lock's HOLD TIME shrinks.
	e.mu.Lock()
	_, reclaimErr := e.ensureRoomLocked(ctx, 0)
	e.mu.Unlock()
	if reclaimErr != nil && e.log != nil {
		e.log.Warnw("autocache evictor: sweep reclaim failed", "error", reclaimErr)
		// Fall through — still publish the gauges from whatever state we can read.
	}

	// (b) Accountant: refresh the gauges with NO lock held. This half is read-only
	// (ListPool + Set on the gauges) and needs no double-spend protection — a gauge
	// snapshot that races a concurrent admit is self-correcting on the next sweep, and
	// is far cheaper than pinning the lock across the pool list. Read cfg live for the
	// budget gauge.
	cfg, err := e.config.Get(ctx)
	if err != nil {
		if e.log != nil {
			e.log.Warnw("autocache evictor: sweep config read failed, skipping gauge refresh", "error", err)
		}
		return
	}
	e.metrics.SetBudgetBytes(cfg.BudgetBytes)

	pool, err := e.pool.ListPool(ctx)
	if err != nil {
		if e.log != nil {
			e.log.Warnw("autocache evictor: sweep pool list failed, skipping gauge refresh", "error", err)
		}
		return
	}
	e.publishAccountantGauges(pool, cfg)
}

// publishAccountantGauges buckets the pool by (source, freshness) and Sets the
// bytes_used + episodes GaugeVecs. It always writes ALL four (source × freshness)
// buckets — including the zero buckets — so a bucket that emptied since the last sweep
// resets to 0 rather than retaining a stale value. now is read once so every row is
// classified against the same instant.
func (e *Evictor) publishAccountantGauges(pool []domain.Episode, cfg *domain.AutocacheConfig) {
	now := time.Now()
	type bucket struct{ bytes, count int64 }
	buckets := map[domain.EpisodeSource]map[Freshness]*bucket{
		domain.EpisodeSourceAdmin:     {FreshnessFresh: {}, FreshnessStale: {}},
		domain.EpisodeSourceAutocache: {FreshnessFresh: {}, FreshnessStale: {}},
	}
	for i := range pool {
		ep := pool[i]
		src := ep.Source
		if _, ok := buckets[src]; !ok {
			// Unknown source defaults to the admin bucket (Classify already treats it
			// as admin) so its bytes/count are still published.
			src = domain.EpisodeSourceAdmin
		}
		fr := Classify(ep, cfg, now)
		b := buckets[src][fr]
		if ep.SizeBytes != nil {
			b.bytes += *ep.SizeBytes
		}
		b.count++
	}
	for src, byFresh := range buckets {
		for fr, b := range byFresh {
			e.metrics.SetBytesUsed(string(src), string(fr), b.bytes)
			e.metrics.SetEpisodes(string(src), string(fr), b.count)
		}
	}
}

// sleep is a ctx-aware + stop-aware sleep. Returns false when the loop must exit (ctx
// cancelled or Stop signalled). Mirrors planner.go's sleep.
func (e *Evictor) sleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-e.stop:
		return false
	case <-t.C:
		return true
	}
}
