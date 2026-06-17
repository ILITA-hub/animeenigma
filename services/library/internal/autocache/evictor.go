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
}

// objectDeleter is the MinIO seam (*minio.Writer.DeletePrefix — Plan 01). It deletes
// every object under a row's minio_path prefix, hard-failing on the first error so the
// Evictor can leave the row intact and retry rather than orphan a serving pointer.
type objectDeleter interface {
	DeletePrefix(ctx context.Context, prefix string) error
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

// NewEvictor wires the Evictor from its seams. metrics + log may be nil (the Evictor
// is nil-guarded throughout via the metrics methods + the log nil-checks).
func NewEvictor(
	config configGetter,
	pool poolAccountant,
	objects objectDeleter,
	libMetrics *metrics.LibraryMetrics,
	log plannerLogger,
) *Evictor {
	return &Evictor{
		config:  config,
		pool:    pool,
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

// ensureRoomLocked is the lock-free reclaim core shared by EnsureRoom (which takes
// e.mu) and Sweep (which already holds e.mu). It reads cfg + the pool byte-sum live,
// short-circuits when the estimate already fits, then evicts Stale candidates in the
// SQL-locked 4-tier order until the estimate fits or the queue is exhausted. The
// caller MUST hold e.mu.
func (e *Evictor) ensureRoomLocked(ctx context.Context, estBytes int64) (admitted bool, err error) {
	cfg, err := e.config.Get(ctx) // live budget + freshness windows (no boot-cache)
	if err != nil {
		return false, err
	}

	used, err := e.pool.SumPoolBytes(ctx) // Σ size_bytes over the aeProvider pool
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

	return used+estBytes <= cfg.BudgetBytes, nil
}

// evictOne deletes one episode's MinIO objects FIRST, then its row only on success
// (T-10-05). DeletePrefix hard-fails on the first RemoveObject error; on that error
// evictOne returns WITHOUT deleting the row so the caller skips this candidate (never
// an orphaned row pointing at deleted objects — tolerate orphaned objects over an
// orphaned serving pointer). On DeletePrefix success it deletes the row; a row-delete
// error is returned (the objects are already gone — the lesser evil, and a re-list
// won't re-find live objects).
func (e *Evictor) evictOne(ctx context.Context, ep domain.Episode) error {
	if err := e.objects.DeletePrefix(ctx, ep.MinioPath); err != nil {
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
