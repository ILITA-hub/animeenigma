package service

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/library/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/library/internal/ffmpeg"
)

// ---- Narrow local seams (satisfied by the concrete repo / minio.Writer /
// ffmpeg.Transcoder / DiskGuard; main.go owns the wiring, same idiom as
// encoder_worker.go). Names are distinct from the encoder worker's
// Transcoder/Uploader/EpisodeStore so the two workers' interfaces don't
// collide in the package. ----

// BackfillEpisodeRepo is the slice of *repo.EpisodeRepository the backfill
// worker needs: the oldest-first list of storyboard-less episodes and the
// per-id flag flip.
type BackfillEpisodeRepo interface {
	ListWithoutStoryboard(ctx context.Context, limit int) ([]domain.Episode, error)
	SetHasStoryboard(ctx context.Context, id string) error
}

// StoryboardStore is the slice of *minio.Writer the backfill worker needs:
// pull the episode's HLS payload down for a local ffmpeg input, and push the
// generated sprite sheets + VTT back under the same prefix.
type StoryboardStore interface {
	DownloadPrefix(ctx context.Context, prefix, destDir string) error
	UploadStoryboard(ctx context.Context, prefix string, sheetPaths []string, vttPath string) error
}

// StoryboardMaker is the slice of *ffmpeg.Transcoder the backfill worker needs.
// Storyboard self-probes the duration when passed <= 0 and errors if it stays
// unknown (that error is just the per-episode warn path here).
type StoryboardMaker interface {
	Storyboard(ctx context.Context, sourcePath string, durationSec int) (*ffmpeg.StoryboardResult, error)
}

// DiskAllower is the slice of *DiskGuard the backfill worker needs — matches
// DiskGuard.Allow (disk_guard.go:67).
type DiskAllower interface {
	Allow(minFreePct int) (allowed bool, freePct int, err error)
}

// backfillBatchSize is how many storyboard-less rows the worker pulls per cycle.
// Pulling a batch (rather than limit=1) is what lets the per-session backoff step
// OVER a recently-failed oldest row and make progress on the ones behind it — the
// starvation guard. It is a soft cap on how far ahead the loop can look, not a
// batch it processes at once (it still processes one eligible episode per cycle).
const backfillBatchSize = 50

// backfillCooldown is how long a failed episode is skipped before it becomes
// eligible again. A broken row (bad source, ffmpeg reject, upload failure) waits
// this long between attempts instead of being re-downloaded (multi-GB) and
// re-ffmpeg'd every single cycle. Lowest-priority tier ⇒ a long window is fine.
const backfillCooldown = 6 * time.Hour

// StoryboardBackfill fills storyboards for episodes ingested before the
// storyboard pass existed. Deliberately slow and yielding: this is one of the
// lowest-priority workloads on the host (owner directive 2026-07-04). Each cycle
// it pulls a batch of storyboard-less rows (oldest first), processes the FIRST
// one not currently in failure-backoff, sleeps `pause` between episodes, sleeps
// 10× `pause` when nothing is eligible (empty list, all cooling down, or a
// transient list error), and drops the whole cycle on any disk-guard risk
// (disallow OR error).
//
// A per-episode failure never aborts the loop AND never wedges the queue: the
// failed id is recorded in `failedAt` and skipped for `cooldown`, so the next
// cycle advances to the next eligible row instead of re-selecting the same
// broken oldest row forever (the starvation fix). On success the row drops out
// via its has_storyboard flag.
type StoryboardBackfill struct {
	repo       BackfillEpisodeRepo
	store      StoryboardStore
	trans      StoryboardMaker
	guard      DiskAllower
	minFreePct int
	log        *logger.Logger
	pause      time.Duration
	tmpdir     string

	// failedAt is a per-session, in-memory backoff: episode id → time of its
	// last processing failure. An episode is eligible again once
	// now-failedAt > cooldown. No migration, no persistence.
	//
	// Bound: this map can ONLY ever hold ids of episodes that both (a) still lack
	// a storyboard and (b) have failed at least once — a finite, small subset of
	// the already-small storyboard-less backlog. A row drops out of the source
	// query the instant it succeeds, so a succeeded id is simply never looked up
	// again; no eviction is needed.
	//
	// Restart semantics: the map is lost on process restart, so each still-broken
	// episode gets exactly one immediate retry per process lifetime before it
	// re-enters cooldown. That is acceptable for this lowest-priority tier.
	failedAt map[string]time.Time
	// cooldown + clock are struct fields (not consts/time.Now calls) purely so a
	// test can shrink the window and drive a fake clock — mirroring how `pause`
	// is injectable. Production uses backfillCooldown + time.Now.
	cooldown time.Duration
	clock    func() time.Time
}

// NewStoryboardBackfill constructs the worker. pause defaults to 60s when
// non-positive. minFreePct reuses the same disk-guard threshold the download /
// encode admit path enforces (cfg.Disk.MinFreePct). log may be nil (all log
// calls are nil-guarded, matching the encoder worker's test wiring).
func NewStoryboardBackfill(
	repo BackfillEpisodeRepo,
	store StoryboardStore,
	trans StoryboardMaker,
	guard DiskAllower,
	minFreePct int,
	pause time.Duration,
	tmpdir string,
	log *logger.Logger,
) *StoryboardBackfill {
	if pause <= 0 {
		pause = 60 * time.Second
	}
	return &StoryboardBackfill{
		repo:       repo,
		store:      store,
		trans:      trans,
		guard:      guard,
		minFreePct: minFreePct,
		log:        log,
		pause:      pause,
		tmpdir:     tmpdir,
		failedAt:   make(map[string]time.Time),
		cooldown:   backfillCooldown,
		clock:      time.Now,
	}
}

// Run is the ctx-aware background loop. It exits promptly on ctx cancellation
// (the sleepCtx select observes ctx.Done). Wire it as a goroutine in main.go
// behind cfg.Storyboard.BackfillEnabled.
func (b *StoryboardBackfill) Run(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}
		// Drop the cycle on any disk risk — a guard error counts as "no".
		if ok, freePct, err := b.guard.Allow(b.minFreePct); err != nil || !ok {
			if b.log != nil {
				b.log.Debugw("storyboard backfill: disk guard disallows, waiting",
					"free_pct", freePct, "min_free_pct", b.minFreePct, "error", err)
			}
			sleepCtx(ctx, b.pause)
			continue
		}
		// Pull a batch (not just the single oldest row) so a recently-failed
		// oldest episode can be stepped over — the starvation guard.
		eps, err := b.repo.ListWithoutStoryboard(ctx, backfillBatchSize)
		if err != nil {
			if ctx.Err() == nil && b.log != nil {
				b.log.Warnw("storyboard backfill: list failed, backing off", "error", err)
			}
			sleepCtx(ctx, 10*b.pause) // transient DB error
			continue
		}
		ep, ok := b.nextEligible(eps)
		if !ok {
			// Nothing pending, or every pending row is still cooling down after a
			// recent failure → idle, same long sleep as an empty list.
			sleepCtx(ctx, 10*b.pause)
			continue
		}
		if !b.processOne(ctx, ep) {
			// Record the failure so this row is skipped for `cooldown` and the
			// queue advances to the next eligible episode next cycle, instead of
			// re-selecting the same broken oldest row forever (Finding 1). The
			// per-step warn was already emitted inside processOne.
			b.failedAt[ep.ID] = b.clock()
		}
		sleepCtx(ctx, b.pause)
	}
}

// nextEligible returns the first batch episode not currently in failure-backoff
// (absent from failedAt, or its cooldown has elapsed). The batch arrives
// created_at ASC, so "first eligible" preserves oldest-first processing while
// stepping over recently-failed rows. Returns ok=false when the batch is empty
// or every row is still cooling down.
func (b *StoryboardBackfill) nextEligible(eps []domain.Episode) (domain.Episode, bool) {
	now := b.clock()
	for _, ep := range eps {
		if failedAt, seen := b.failedAt[ep.ID]; seen && now.Sub(failedAt) <= b.cooldown {
			continue // still cooling down
		}
		return ep, true
	}
	return domain.Episode{}, false
}

// processOne generates + uploads the storyboard for a single episode and flips
// its flag on success. Every step is best-effort: on any error it warns and
// returns false WITHOUT setting the flag; the caller then records the failure in
// the per-session backoff. Returns true only when the flag was flipped. The
// per-episode temp dir is always cleaned (defer os.RemoveAll), as is the
// transcoder's own storyboard output dir.
func (b *StoryboardBackfill) processOne(ctx context.Context, ep domain.Episode) bool {
	// The transcoder's own MkdirTemp needs an existing parent; mirror
	// Storyboard()'s guard so a configured-but-absent tmpdir doesn't fail
	// every episode.
	if b.tmpdir != "" {
		if err := os.MkdirAll(b.tmpdir, 0o755); err != nil {
			b.warn(ctx, "storyboard backfill: mkdir tmpdir failed", "episode_id", ep.ID, "tmpdir", b.tmpdir, "error", err)
			return false
		}
	}
	dir, err := os.MkdirTemp(b.tmpdir, "sb-backfill-")
	if err != nil {
		b.warn(ctx, "storyboard backfill: mkdir temp failed", "episode_id", ep.ID, "error", err)
		return false
	}
	defer func() { _ = os.RemoveAll(dir) }()

	// 1. Pull the HLS payload (playlist.m3u8 + segments) down so ffmpeg has a
	//    local input; segments are referenced relatively, so the co-located
	//    dir is a valid -i source.
	if err := b.store.DownloadPrefix(ctx, ep.MinioPath, dir); err != nil {
		b.warn(ctx, "storyboard backfill: download prefix failed",
			"episode_id", ep.ID, "prefix", ep.MinioPath, "error", err)
		return false
	}

	// 2. Generate the sprite sheets + VTT. Pass the stored duration (0 when
	//    nil → Storyboard self-probes the local playlist).
	duration := 0
	if ep.DurationSec != nil {
		duration = *ep.DurationSec
	}
	sb, err := b.trans.Storyboard(ctx, filepath.Join(dir, "playlist.m3u8"), duration)
	if err != nil {
		b.warn(ctx, "storyboard backfill: storyboard generation failed",
			"episode_id", ep.ID, "prefix", ep.MinioPath, "error", err)
		return false
	}
	// The transcoder owns its own MkdirTemp subdir (filepath.Dir(VTTPath));
	// clean it once we're done regardless of the upload outcome.
	defer func() { _ = os.RemoveAll(filepath.Dir(sb.VTTPath)) }()

	// 3. Push the sprites back under the same episode prefix.
	if err := b.store.UploadStoryboard(ctx, ep.MinioPath, sb.SheetPaths, sb.VTTPath); err != nil {
		b.warn(ctx, "storyboard backfill: upload failed",
			"episode_id", ep.ID, "prefix", ep.MinioPath, "error", err)
		return false
	}

	// 4. Flip the flag ONLY after the sprites are durably uploaded.
	if err := b.repo.SetHasStoryboard(ctx, ep.ID); err != nil {
		b.warn(ctx, "storyboard backfill: set flag failed (sprites uploaded; will retry next pass)",
			"episode_id", ep.ID, "error", err)
		return false
	}
	if b.log != nil {
		b.log.Infow("storyboard backfill: episode processed",
			"episode_id", ep.ID, "shikimori_id", ep.ShikimoriID,
			"episode", ep.EpisodeNumber, "prefix", ep.MinioPath, "duration_sec", duration)
	}
	return true
}

// warn emits a per-step failure warning — UNLESS the context is already
// cancelled. A SIGTERM mid-cycle turns every in-flight step (download, ffmpeg,
// upload) into an error; logging those would be pure shutdown noise (Finding 4).
func (b *StoryboardBackfill) warn(ctx context.Context, msg string, kv ...any) {
	if ctx.Err() != nil || b.log == nil {
		return
	}
	b.log.Warnw(msg, kv...)
}

// sleepCtx sleeps d, returning early if ctx is cancelled — the yielding
// primitive that keeps the backfill loop responsive to SIGTERM between its
// deliberately long pauses.
func sleepCtx(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}
