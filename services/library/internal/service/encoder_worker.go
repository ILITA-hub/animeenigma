package service

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/library/internal/autocache"
	"github.com/ILITA-hub/animeenigma/services/library/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/library/internal/ffmpeg"
	"github.com/anacrolix/torrent/metainfo"
)

// EncoderJobStore is the slice of *repo.JobRepository the encoder
// worker needs. Phase 03's WorkerPool uses a similar pattern; we
// declare a distinct interface here so the worker's tests don't have
// to provide UpdateProgress / Cancel methods.
type EncoderJobStore interface {
	// ClaimForEncoding atomically claims the oldest status='encoding' row
	// and flips it to the non-claimable 'transcoding' state, returning it
	// with Status='transcoding' (or (nil, nil) when none is available).
	ClaimForEncoding(ctx context.Context) (*domain.Job, error)
	GetByID(ctx context.Context, id string) (*domain.Job, error)
	UpdateStatus(ctx context.Context, id string, newStatus domain.JobStatus, errorText string) error
	// UpdateStorage writes the RESOLVED storage backend (returned by the upload)
	// back onto the job row after a successful upload, mirroring how
	// library_episodes.audio_lang/quality record the actual output rather than
	// the request. The Link handler later reads it to know which backend to
	// list/move the pending objects from.
	UpdateStorage(ctx context.Context, id, storage string) error
}

// EpisodeStore is the slice of *repo.EpisodeRepository the encoder
// worker needs.
type EpisodeStore interface {
	Create(ctx context.Context, ep *domain.Episode) error
}

// Transcoder is the surface the worker consumes from
// internal/ffmpeg.Transcoder.
type Transcoder interface {
	Transcode(ctx context.Context, sourcePath string) (*ffmpeg.Result, error)
	// Storyboard generates the scrub-preview sprite sheets + VTT for the
	// already-transcoded source. Consumed best-effort by the worker — a
	// failure here never fails the job.
	Storyboard(ctx context.Context, sourcePath string, durationSec int) (*ffmpeg.StoryboardResult, error)
}

// Uploader is the surface the worker consumes from storagegw.Gateway (the
// adapter over libs/storageclient). Upload routes the storage service by
// content class + per-job override and returns the RESOLVED backend id the
// files landed on; the worker records that on the job + episode rows.
type Uploader interface {
	Upload(ctx context.Context, class, override, prefix string, filePaths []string) (storage string, err error)
	// UploadStoryboard puts the storyboard sprite sheets + VTT under the episode
	// prefix on the SAME resolved backend the HLS landed on. Consumed
	// best-effort by the worker.
	UploadStoryboard(ctx context.Context, storage, prefix string, sheetPaths []string, vttPath string) error
}

// EpisodeDetector is the surface the worker consumes from
// internal/parser/filename.Detector.
type EpisodeDetector interface {
	DetectEpisode(filename, uploader string) (int, bool)
}

// SourcePathResolver resolves a job's downloaded payload to an
// on-disk source path. The default implementation walks
// {downloadDir}/{infohash}/ for the largest video file.
type SourcePathResolver interface {
	Resolve(ctx context.Context, job *domain.Job, infohash string) (string, error)
}

// EncodeMetrics is the encoder-relevant slice of LibraryMetrics.
type EncodeMetrics interface {
	IncJobsTotal(status string)
	ObserveEncodeDuration(seconds float64)
	AddUploadBytes(n int64)
	IncEncodeFailures(reason string)
	// SetEncodeActiveWorkers publishes the live concurrent-transcode count the
	// degradation-aware graded limiter admits (AUTO-575).
	SetEncodeActiveWorkers(n int)
}

// EncoderPool is N goroutines that race for status='encoding' jobs
// via JobStore.ClaimForEncoding(...). Each goroutine drives one job at a
// time:
//
//	ClaimForEncoding() [encoding → transcoding, atomic + non-claimable] →
//	  resolve source → detect episode → Transcode → status=uploading →
//	  Upload → status=done (insert library_episodes row when
//	  shikimori_id != "").
//
// The atomic flip to 'transcoding' (a state nobody claims) is what stops a
// second idle worker from re-claiming a row mid-ffmpeg — the row drops out
// of the claimable 'encoding' set the instant it is claimed.
//
// Cancellation: ffmpeg / minio respect ctx.Done() via
// exec.CommandContext + the SDK's ctx-aware client. Worker exits
// gracefully on ctx cancellation.
type EncoderPool struct {
	workers     int
	jobRepo     EncoderJobStore
	episodeRepo EpisodeStore
	transcoder  Transcoder
	uploader    Uploader
	detector    EpisodeDetector
	resolver    SourcePathResolver
	metrics     EncodeMetrics
	log         *logger.Logger
	// Phase 06 (workstream raw-jp / v0.2). nil-safe: when nil, the
	// post-done webhook fire is skipped — the encoder's correctness
	// is unaffected (catalog 1h TTL handles eventual consistency).
	invalidator CatalogInvalidator

	pollInterval time.Duration

	// limiter is the degradation-aware GRADED concurrency limiter (AUTO-575).
	// It caps the number of concurrent transcodes by the live platform
	// degradation level — full at level 0, 1 at level 1, 0 at level 2+ — so
	// heavy ffmpeg work backs off under host pressure instead of stacking CPU
	// (the pattern that tripped the host-pressure governor). A running transcode
	// always finishes; only admission of NEW work is gated, and gated jobs stay
	// queued in the DB (status='encoding', unclaimed) rather than being dropped.
	limiter *encodeLimiter

	wg sync.WaitGroup
}

// NewEncoderPool constructs an EncoderPool. workers >= 1, pollInterval
// defaults to 2s.
//
// invalidator is nil-safe (Phase 06, workstream raw-jp / v0.2): pass
// nil to disable the post-done catalog webhook entirely; production
// passes a CatalogInvalidator built via NewCatalogInvalidator. The
// nil case mirrors the no-op invalidator's behavior — the catalog's
// 1h TTL preserves correctness, only the fast-path is skipped.
func NewEncoderPool(
	workers int,
	jobRepo EncoderJobStore,
	episodeRepo EpisodeStore,
	transcoder Transcoder,
	uploader Uploader,
	detector EpisodeDetector,
	resolver SourcePathResolver,
	metrics EncodeMetrics,
	log *logger.Logger,
	invalidator CatalogInvalidator,
) *EncoderPool {
	if workers < 1 {
		workers = 1
	}
	// The limiter's max cap (level-0 throughput) is the goroutine count itself,
	// so at level 0 all workers may transcode concurrently. The active-workers
	// gauge setter is bound from metrics when present (nil-guarded inside the
	// limiter), keeping the single-emitter gauge in the library metrics package.
	var setActive func(int)
	if metrics != nil {
		setActive = metrics.SetEncodeActiveWorkers
	}
	return &EncoderPool{
		workers:      workers,
		jobRepo:      jobRepo,
		episodeRepo:  episodeRepo,
		transcoder:   transcoder,
		uploader:     uploader,
		detector:     detector,
		resolver:     resolver,
		metrics:      metrics,
		log:          log,
		invalidator:  invalidator,
		pollInterval: 2 * time.Second,
		limiter:      newEncodeLimiter(workers, setActive, log),
	}
}

// SetShedChecker wires the degradation watcher into the graded limiter
// (nil-safe; call before Start). Name kept for parity with the download +
// storyboard pools' wiring in main.go.
func (p *EncoderPool) SetShedChecker(c ShedChecker) { p.limiter.set(c) }

// Start launches worker goroutines; returns immediately. Goroutines
// exit on <-ctx.Done().
func (p *EncoderPool) Start(ctx context.Context) {
	for i := 0; i < p.workers; i++ {
		p.wg.Add(1)
		go p.runWorker(ctx, i)
	}
}

// Stop waits up to timeout for goroutines to exit.
func (p *EncoderPool) Stop(timeout time.Duration) error {
	doneCh := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(doneCh)
	}()
	select {
	case <-doneCh:
		return nil
	case <-time.After(timeout):
		if p.log != nil {
			p.log.Warnw("encoder pool stop timed out", "timeout", timeout)
		}
		return errors.New("encoder pool stop timed out")
	}
}

// runWorker claims an encoding row (atomically flipping it to the
// non-claimable 'transcoding' state); on empty queue it sleeps
// pollInterval and retries.
func (p *EncoderPool) runWorker(ctx context.Context, idx int) {
	defer p.wg.Done()
	for {
		if ctx.Err() != nil {
			return
		}
		// Degradation-aware graded admission (AUTO-575): reserve a transcode slot
		// at the current cap BEFORE claiming, so the cap actually bounds how many
		// ffmpeg runs stack concurrently. A cap of 0 (Critical) or an already-
		// saturated cap (this goroutine is a surplus worker beyond the graded
		// limit) → wait; the job stays queued in the DB, nothing is dropped.
		if !p.limiter.tryAcquire() {
			if !p.sleep(ctx, p.pollInterval) {
				return
			}
			continue
		}
		job, err := p.jobRepo.ClaimForEncoding(ctx)
		if err != nil {
			p.limiter.release()
			if p.log != nil {
				p.log.Warnw("encoder claim failed", "worker", idx, "error", err)
			}
			if !p.sleep(ctx, p.pollInterval) {
				return
			}
			continue
		}
		if job == nil {
			// Empty queue — free the slot immediately so it doesn't count
			// against the cap while we idle.
			p.limiter.release()
			if !p.sleep(ctx, p.pollInterval) {
				return
			}
			continue
		}
		p.processJob(ctx, job)
		p.limiter.release()
	}
}

// sleep is a ctx-aware time.Sleep. Returns false when the context fires.
// Delegates to the package-level sleepCtx primitive shared with WorkerPool
// and the storyboard backfill loop.
func (p *EncoderPool) sleep(ctx context.Context, d time.Duration) bool {
	return sleepCtx(ctx, d)
}

// failJob writes status=failed with errorText + bumps the
// per-reason failure counter.
func (p *EncoderPool) failJob(ctx context.Context, job *domain.Job, reason, errText string) {
	_ = p.jobRepo.UpdateStatus(ctx, job.ID, domain.JobStatusFailed, errText)
	if p.metrics != nil {
		p.metrics.IncJobsTotal(string(domain.JobStatusFailed))
		p.metrics.IncEncodeFailures(reason)
	}
	if p.log != nil {
		p.log.Errorw("encoder job failed",
			"job_id", job.ID,
			"reason", reason,
			"error", errText,
		)
	}
}

// processJob drives a single claimed job through transcoding → uploading
// → done|failed. The row arrives already in 'transcoding' (ClaimForEncoding
// flipped it atomically) — a non-claimable state — so no second worker can
// re-claim it while ffmpeg runs. We do NOT touch the status here at the
// top; the next persisted transition is 'uploading' once the transcode
// completes.
func (p *EncoderPool) processJob(ctx context.Context, job *domain.Job) {
	// 1. The row is already 'transcoding' (claimed). Record the metric so
	//    dashboards still see one "encode started" per job.
	if p.metrics != nil {
		p.metrics.IncJobsTotal(string(domain.JobStatusTranscoding))
	}

	// 2. Derive infohash from the magnet.
	m, err := metainfo.ParseMagnetUri(job.Magnet)
	if err != nil {
		p.failJob(ctx, job, "invalid_magnet", fmt.Sprintf("magnet parse failed at encode time: %v", err))
		return
	}
	infohash := strings.ToLower(m.InfoHash.HexString())

	// 3. Resolve the on-disk source path.
	source, err := p.resolver.Resolve(ctx, job, infohash)
	if err != nil {
		p.failJob(ctx, job, "source_missing", fmt.Sprintf("resolve source: %v", err))
		return
	}

	// 4. Determine episode number. Autocache (and admin-with-known-episode) jobs
	// persist the INTENDED episode at enqueue time (migration 009) — trust it
	// directly. Filename detection is a fragile fallback (the generic detector
	// only matches "- NN (" / "- NN [" styles and fails on common release names
	// like "...S04E10 VOSTFR ...-Tsundere-Raws (CR).mkv"), used only when the
	// episode is unknown (e.g. a manual folder ingest).
	var episode int
	if job.Episode != nil && *job.Episode > 0 {
		episode = *job.Episode
	} else {
		var ok bool
		episode, ok = p.detector.DetectEpisode(filepath.Base(source), job.Uploader)
		if !ok {
			p.failJob(ctx, job, "episode_detect_failed",
				fmt.Sprintf("could not detect episode number from filename: %s", filepath.Base(source)))
			return
		}
	}

	// 5. Transcode.
	start := time.Now()
	result, err := p.transcoder.Transcode(ctx, source)
	if err != nil {
		p.failJob(ctx, job, "ffmpeg_error", err.Error())
		return
	}
	if p.metrics != nil {
		p.metrics.ObserveEncodeDuration(time.Since(start).Seconds())
	}

	// Cancellation observation (post-Transcode).
	if fresh, err := p.jobRepo.GetByID(ctx, job.ID); err == nil && fresh != nil && fresh.Status == domain.JobStatusCancelled {
		if p.metrics != nil {
			p.metrics.IncJobsTotal(string(domain.JobStatusCancelled))
		}
		return
	}

	// 6. Status → uploading.
	if err := p.jobRepo.UpdateStatus(ctx, job.ID, domain.JobStatusUploading, ""); err != nil {
		p.failJob(ctx, job, "upload_error", fmt.Sprintf("status uploading: %v", err))
		return
	}
	if p.metrics != nil {
		p.metrics.IncJobsTotal(string(domain.JobStatusUploading))
	}

	// 7. Compute MinIO prefix. Resolved (shikimori_id) uploads land under the
	// unified autocache pool layout (aeProvider/<mal>/RAW/<ep>/) so NEW admin
	// content is already migrated; the pending/ branch is unchanged.
	var prefix string
	if job.ShikimoriID != "" {
		prefix = autocache.RawPrefix(job.ShikimoriID, episode)
	} else {
		prefix = fmt.Sprintf("pending/%s/%d/", job.ID, episode)
	}

	// 8. Upload (segments concurrently, playlist last). Content class routes the
	// storage service: autocache (Planner-driven) content is library-auto, every
	// other ingest is library-manual (which alone honors the per-job override).
	// The upload returns the RESOLVED backend id — recorded on the job + episode.
	class := domain.ClassLibraryManual
	if job.Source == domain.JobSourceAutocache {
		class = domain.ClassLibraryAuto
	}
	files := append([]string{}, result.SegmentPaths...)
	files = append(files, result.PlaylistPath)
	storage, err := p.uploader.Upload(ctx, class, job.Storage, prefix, files)
	if err != nil {
		p.failJob(ctx, job, "upload_error", err.Error())
		return
	}
	uploadBytes := SumFileSizes(files)
	if p.metrics != nil {
		p.metrics.AddUploadBytes(uploadBytes)
	}
	// Persist the resolved backend back onto the job row (best-effort: the
	// episode row below is the authoritative serving pointer, so a write-back
	// blip only affects a later Link of an unresolved job — logged, never fatal).
	if err := p.jobRepo.UpdateStorage(ctx, job.ID, storage); err != nil && p.log != nil {
		p.log.Warnw("encoder: job storage write-back failed", "job_id", job.ID, "storage", storage, "error", err)
	}

	// 8b. Storyboard pass (scrub-preview sprites) — strictly best-effort: any
	// failure is logged and the job proceeds without a storyboard. Skipped for
	// pending/ jobs (no ShikimoriID → no episode row exists to flag; sprites
	// would be orphans nothing ever references). Runs AFTER the HLS upload
	// succeeds so a storyboard failure never blocks playable output, and
	// BEFORE episodeRepo.Create so hasStoryboard is known at insert time
	// (avoiding a second UPDATE).
	hasStoryboard := false
	if job.ShikimoriID == "" {
		// nothing — pending/ uploads have no episode row.
	} else if sb, sbErr := p.transcoder.Storyboard(ctx, source, result.DurationSec); sbErr != nil {
		if p.log != nil {
			p.log.Warnw("storyboard generation failed; episode ships without preview sprites",
				"job_id", job.ID, "error", sbErr)
		}
	} else {
		if upErr := p.uploader.UploadStoryboard(ctx, storage, prefix, sb.SheetPaths, sb.VTTPath); upErr != nil {
			if p.log != nil {
				p.log.Warnw("storyboard upload failed", "job_id", job.ID, "error", upErr)
			}
		} else {
			hasStoryboard = true
		}
		_ = os.RemoveAll(filepath.Dir(sb.VTTPath))
	}

	// 9. Persist library_episodes row (only when shikimori_id known).
	if job.ShikimoriID != "" {
		jobIDCopy := job.ID
		duration := result.DurationSec
		size := result.SizeBytes
		downloadedAt := time.Now()
		ep := &domain.Episode{
			ShikimoriID:   job.ShikimoriID,
			EpisodeNumber: episode,
			JobID:         &jobIDCopy,
			MinioPath:     prefix,
			// Storage is the backend the upload actually resolved to (minio for
			// library-manual, s3 for library-auto in prod). The evictor's LOCAL-disk
			// queries scope to storage='minio', so an s3 row is correctly excluded.
			Storage:     storage,
			DurationSec: &duration,
			SizeBytes:   &size,
			// Storage class drives the Accountant byte-accounting and the
			// Evictor's order (autocache is evicted before admin) + freshness
			// windows. Without this the column defaulted to 'admin', so every
			// auto-downloaded episode was mislabelled as protected admin content.
			Source: episodeSourceFor(job.Source),
			Track:  domain.EpisodeTrackRaw,
			// downloaded_at is the Fresh-rule-1 basis; set it to now (the
			// download finished moments ago) so eviction freshness works.
			DownloadedAt: &downloadedAt,
			// HasStoryboard reflects the best-effort pass above — false when
			// ShikimoriID is empty (branch never runs), the ffmpeg pass fails,
			// or the MinIO upload fails.
			HasStoryboard: hasStoryboard,
		}
		if err := p.episodeRepo.Create(ctx, ep); err != nil {
			// Duplicate (re-encode of an existing episode) → log + continue.
			if strings.Contains(strings.ToLower(err.Error()), "already exists") {
				if p.log != nil {
					p.log.Warnw("episode already exists; MinIO files replaced",
						"shikimori_id", job.ShikimoriID, "episode", episode)
				}
			} else {
				p.failJob(ctx, job, "episode_insert_failed", err.Error())
				return
			}
		}
	}

	// 10. Clean up local temp dir (best-effort).
	if result.PlaylistPath != "" {
		_ = os.RemoveAll(filepath.Dir(result.PlaylistPath))
	}

	// 11. status=done.
	if err := p.jobRepo.UpdateStatus(ctx, job.ID, domain.JobStatusDone, ""); err != nil {
		if p.log != nil {
			p.log.Errorw("final UpdateStatus(done) failed", "job_id", job.ID, "error", err)
		}
		return
	}
	if p.metrics != nil {
		p.metrics.IncJobsTotal(string(domain.JobStatusDone))
	}
	if p.log != nil {
		p.log.Infow("encoder job done",
			"job_id", job.ID,
			"shikimori_id", job.ShikimoriID,
			"episode", episode,
			"minio_prefix", prefix,
			"duration_sec", result.DurationSec,
			"size_bytes", result.SizeBytes,
			"storage", storage,
			"upload_bytes", uploadBytes,
		)
	}

	// Phase 06 (workstream raw-jp / v0.2). Best-effort cache-bust
	// fire. nil-safe + empty-shikimori-safe; never fails the job.
	// The invalidator owns its own per-request timeout (default 3s)
	// and never returns an error to this caller.
	if p.invalidator != nil && job.ShikimoriID != "" {
		p.invalidator.Invalidate(ctx, job.ShikimoriID)
	}
}

// SumFileSizes totals the on-disk size of every path, skipping any that can't be
// stat'd. The storage service upload path returns only the resolved backend id
// (not a byte count), so the encoder recomputes the uploaded volume here for the
// library_upload_bytes_total metric — the files still exist on disk at this point
// (temp-dir cleanup runs later, in step 10).
func SumFileSizes(paths []string) int64 {
	var total int64
	for _, p := range paths {
		if st, err := os.Stat(p); err == nil {
			total += st.Size()
		}
	}
	return total
}

// episodeSourceFor maps a job's source to the episode storage class. Only the
// Planner-driven autocache path produces 'autocache' content; every other path
// (manual / nyaa / animetosho / jackett admin ingest) is 'admin' — longer
// freshness, lower eviction priority.
func episodeSourceFor(js domain.JobSource) domain.EpisodeSource {
	if js == domain.JobSourceAutocache {
		return domain.EpisodeSourceAutocache
	}
	return domain.EpisodeSourceAdmin
}

// ---- DefaultSourceResolver ----

// videoExtensions is the recognised set; lowercased.
var videoExtensions = map[string]bool{
	".mp4":  true,
	".mkv":  true,
	".avi":  true,
	".mov":  true,
	".m4v":  true,
	".webm": true,
	".ts":   true,
}

// DefaultSourceResolver implements SourcePathResolver by walking
// {downloadDir}/{infohash}/ for the largest video file.
type DefaultSourceResolver struct {
	downloadDir string
	maxDepth    int
}

// NewDefaultSourceResolver constructs the resolver. maxDepth defaults
// to 10 to bound walking on pathological torrents.
func NewDefaultSourceResolver(downloadDir string) *DefaultSourceResolver {
	return &DefaultSourceResolver{downloadDir: downloadDir, maxDepth: 10}
}

// Resolve walks the {downloadDir}/{infohash}/ tree for the LARGEST
// video file (any of .mp4/.mkv/.avi/.mov/.m4v/.webm/.ts case-insensitive).
// Returns an error if the dir is missing or contains no video files.
func (r *DefaultSourceResolver) Resolve(ctx context.Context, _ *domain.Job, infohash string) (string, error) {
	if infohash == "" {
		return "", errors.New("infohash is empty")
	}
	root := filepath.Join(r.downloadDir, infohash)
	st, err := os.Stat(root)
	if err != nil {
		return "", fmt.Errorf("stat %s: %w", root, err)
	}
	if !st.IsDir() {
		return "", fmt.Errorf("%s is not a directory", root)
	}

	var bestPath string
	var bestSize int64

	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		// Bound depth.
		rel, _ := filepath.Rel(root, path)
		depth := strings.Count(rel, string(filepath.Separator))
		if depth > r.maxDepth {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if !videoExtensions[ext] {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil // ignore unreadable entries
		}
		if info.Size() > bestSize {
			bestSize = info.Size()
			bestPath = path
		}
		return nil
	})
	if walkErr != nil {
		return "", fmt.Errorf("walk %s: %w", root, walkErr)
	}
	if bestPath == "" {
		return "", fmt.Errorf("no video file under %s", root)
	}
	return bestPath, nil
}
