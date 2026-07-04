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

// StoryboardBackfill fills storyboards for episodes ingested before the
// storyboard pass existed. Deliberately slow and yielding: this is one of the
// lowest-priority workloads on the host (owner directive 2026-07-04). It
// processes ONE episode per cycle, sleeps `pause` between episodes, sleeps
// 10× `pause` when the list is empty or errored, and drops the whole cycle on
// any disk-guard risk (disallow OR error). No per-episode failure ever aborts
// the loop — a broken episode simply resurfaces on the next full pass.
type StoryboardBackfill struct {
	repo       BackfillEpisodeRepo
	store      StoryboardStore
	trans      StoryboardMaker
	guard      DiskAllower
	minFreePct int
	log        *logger.Logger
	pause      time.Duration
	tmpdir     string
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
		eps, err := b.repo.ListWithoutStoryboard(ctx, 1)
		if err != nil || len(eps) == 0 {
			if err != nil && b.log != nil {
				b.log.Warnw("storyboard backfill: list failed, backing off", "error", err)
			}
			sleepCtx(ctx, 10*b.pause) // idle: nothing to do (or a transient DB error)
			continue
		}
		b.processOne(ctx, eps[0]) // errors are logged inside; never aborts the loop
		sleepCtx(ctx, b.pause)
	}
}

// processOne generates + uploads the storyboard for a single episode and flips
// its flag on success. Every step is best-effort: on any error it warns and
// returns WITHOUT setting the flag, so the episode is retried on the next full
// pass. The per-episode temp dir is always cleaned (defer os.RemoveAll), as is
// the transcoder's own storyboard output dir.
func (b *StoryboardBackfill) processOne(ctx context.Context, ep domain.Episode) {
	// The transcoder's own MkdirTemp needs an existing parent; mirror
	// Storyboard()'s guard so a configured-but-absent tmpdir doesn't fail
	// every episode.
	if b.tmpdir != "" {
		if err := os.MkdirAll(b.tmpdir, 0o755); err != nil {
			if b.log != nil {
				b.log.Warnw("storyboard backfill: mkdir tmpdir failed", "episode_id", ep.ID, "tmpdir", b.tmpdir, "error", err)
			}
			return
		}
	}
	dir, err := os.MkdirTemp(b.tmpdir, "sb-backfill-")
	if err != nil {
		if b.log != nil {
			b.log.Warnw("storyboard backfill: mkdir temp failed", "episode_id", ep.ID, "error", err)
		}
		return
	}
	defer func() { _ = os.RemoveAll(dir) }()

	// 1. Pull the HLS payload (playlist.m3u8 + segments) down so ffmpeg has a
	//    local input; segments are referenced relatively, so the co-located
	//    dir is a valid -i source.
	if err := b.store.DownloadPrefix(ctx, ep.MinioPath, dir); err != nil {
		if b.log != nil {
			b.log.Warnw("storyboard backfill: download prefix failed",
				"episode_id", ep.ID, "prefix", ep.MinioPath, "error", err)
		}
		return
	}

	// 2. Generate the sprite sheets + VTT. Pass the stored duration (0 when
	//    nil → Storyboard self-probes the local playlist).
	duration := 0
	if ep.DurationSec != nil {
		duration = *ep.DurationSec
	}
	sb, err := b.trans.Storyboard(ctx, filepath.Join(dir, "playlist.m3u8"), duration)
	if err != nil {
		if b.log != nil {
			b.log.Warnw("storyboard backfill: storyboard generation failed",
				"episode_id", ep.ID, "prefix", ep.MinioPath, "error", err)
		}
		return
	}
	// The transcoder owns its own MkdirTemp subdir (filepath.Dir(VTTPath));
	// clean it once we're done regardless of the upload outcome.
	defer func() { _ = os.RemoveAll(filepath.Dir(sb.VTTPath)) }()

	// 3. Push the sprites back under the same episode prefix.
	if err := b.store.UploadStoryboard(ctx, ep.MinioPath, sb.SheetPaths, sb.VTTPath); err != nil {
		if b.log != nil {
			b.log.Warnw("storyboard backfill: upload failed",
				"episode_id", ep.ID, "prefix", ep.MinioPath, "error", err)
		}
		return
	}

	// 4. Flip the flag ONLY after the sprites are durably uploaded.
	if err := b.repo.SetHasStoryboard(ctx, ep.ID); err != nil {
		if b.log != nil {
			b.log.Warnw("storyboard backfill: set flag failed (sprites uploaded; will retry next pass)",
				"episode_id", ep.ID, "error", err)
		}
		return
	}
	if b.log != nil {
		b.log.Infow("storyboard backfill: episode processed",
			"episode_id", ep.ID, "shikimori_id", ep.ShikimoriID,
			"episode", ep.EpisodeNumber, "prefix", ep.MinioPath, "duration_sec", duration)
	}
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
