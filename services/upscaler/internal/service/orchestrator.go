package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/upscaler/internal/autocache"
	"github.com/ILITA-hub/animeenigma/services/upscaler/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/upscaler/internal/ffmpeg"
	"github.com/ILITA-hub/animeenigma/services/upscaler/internal/repo"
	"github.com/ILITA-hub/animeenigma/services/upscaler/internal/source"
)

const (
	// orchestratorInterval is how often the orchestrator polls for jobs that
	// need to advance through the lifecycle.
	orchestratorInterval = 10 * time.Second
)

// ── Injected dependency interfaces ────────────────────────────────────────────
//
// Small interfaces are defined here (not imported wholesale) so the orchestrator
// can be unit-tested with handwritten fakes — no real ffmpeg / minio / source
// torrent volume is required.

// orchJobRepo is the slice of JobRepository the orchestrator drives. It uses
// repo.JobFilter directly so the real *repo.JobRepository satisfies it without
// an adapter; fakes construct repo.JobFilter values just the same.
type orchJobRepo interface {
	List(ctx context.Context, f repo.JobFilter) ([]domain.UpscaleJob, error)
	UpdateStatus(ctx context.Context, id string, status domain.JobStatus, errText string) error
	SetSourceMeta(ctx context.Context, id, codec, pixfmt, fps string, height, segCount int) error
	SetOutputPrefix(ctx context.Context, id, prefix string) error
}

// orchSegmentRepo is the slice of SegmentRepository the orchestrator drives.
type orchSegmentRepo interface {
	BulkInsertPending(ctx context.Context, jobID string, n int) error
	Counts(ctx context.Context, jobID string) (pending, leased, done int, err error)
}

// orchResolver stages the original source file for a job.
type orchResolver interface {
	Resolve(ctx context.Context, job *domain.UpscaleJob) (string, error)
}

// orchProber probes a staged video file for codec/pixfmt/fps/dimensions.
type orchProber interface {
	Probe(ctx context.Context, path string) (source.ProbeResult, error)
}

// orchSegmenter splits the source into segments and demuxes sidecars.
type orchSegmenter interface {
	Segment(ctx context.Context, srcVideoPath, outDir string, seconds int) ([]string, error)
	DemuxSidecars(ctx context.Context, srcPath, outDir string) (ffmpeg.Sidecars, error)
}

// orchFinalizer concats upscaled segments + remuxes sidecars into an HLS package.
type orchFinalizer interface {
	Concat(ctx context.Context, upscaledSegDir string, sc ffmpeg.Sidecars, probe source.ProbeResult, out string) error
}

// orchWriter uploads the finalized HLS package to object storage.
type orchWriter interface {
	EnsureBucket(ctx context.Context) error
	Upload(ctx context.Context, prefix string, filePaths []string) (int64, error)
}

// hlsLister enumerates the playlist + segment files produced by Concat. Pulled
// out as an interface so tests don't need real files on disk.
type hlsLister func(hlsDir string) ([]string, error)

// ── Orchestrator ──────────────────────────────────────────────────────────────

// Orchestrator polls the job table and drives each job through the lifecycle:
//
//	queued → segmenting → upscaling → finalizing → done
//
// Two transitions are owned here:
//
//  1. queued→segmenting→upscaling: resolve + probe the source, segment + demux
//     sidecars, seed the segment rows, flip to upscaling. Workers then lease and
//     upscale individual segments out-of-band.
//
//  2. upscaling→finalizing→done: INDEPENDENTLY detect all-done (pending==0 &&
//     leased==0 && done>0) and flip upscaling→finalizing — this closes the I-1
//     liveness gap: the leaser only flips on an incoming lease_req, so if workers
//     stop polling after the last segment the job would otherwise hang. Then for
//     finalizing jobs: concat → upload → record prefix → done.
//
// Per-job work is wrapped in a recover so one bad job never crashes the poller.
type Orchestrator struct {
	jobs      orchJobRepo
	segs      orchSegmentRepo
	resolver  orchResolver
	prober    orchProber
	segmenter orchSegmenter
	finalizer orchFinalizer
	writer    orchWriter
	listHLS   hlsLister
	logBuf    orchLogFlusher // optional; nil = no flush

	stagingDir     string
	segmentSeconds int
	log            *logger.Logger

	stopCh chan struct{}
}

// orchLogFlusher is the minimal interface the Orchestrator needs from LogBuffer
// to flush per-job logs to object storage when a job completes.
type orchLogFlusher interface {
	Flush(ctx context.Context, jobID string) error
}

// OrchestratorDeps bundles the orchestrator's collaborators. All fields are
// required except ListHLS (defaults to listHLSFiles), Log (defaults to the
// package logger), and LogBuffer (optional; log flush is skipped if nil).
type OrchestratorDeps struct {
	Jobs      orchJobRepo
	Segments  orchSegmentRepo
	Resolver  orchResolver
	Prober    orchProber
	Segmenter orchSegmenter
	Finalizer orchFinalizer
	Writer    orchWriter
	ListHLS   hlsLister
	LogBuffer orchLogFlusher

	StagingDir     string
	SegmentSeconds int
	Log            *logger.Logger
}

// NewOrchestrator constructs an Orchestrator from its dependencies.
func NewOrchestrator(d OrchestratorDeps) *Orchestrator {
	log := d.Log
	if log == nil {
		log = logger.Default()
	}
	listHLS := d.ListHLS
	if listHLS == nil {
		listHLS = listHLSFiles
	}
	secs := d.SegmentSeconds
	if secs <= 0 {
		secs = 45
	}
	return &Orchestrator{
		jobs:           d.Jobs,
		segs:           d.Segments,
		resolver:       d.Resolver,
		prober:         d.Prober,
		segmenter:      d.Segmenter,
		finalizer:      d.Finalizer,
		writer:         d.Writer,
		listHLS:        listHLS,
		logBuf:         d.LogBuffer,
		stagingDir:     d.StagingDir,
		segmentSeconds: secs,
		log:            log,
		stopCh:         make(chan struct{}),
	}
}

// Run starts the poller. It blocks until ctx is cancelled or Stop() is called.
// The first tick runs immediately so a freshly-queued job advances without
// waiting a full interval; subsequent ticks fire every orchestratorInterval.
func (o *Orchestrator) Run(ctx context.Context) {
	// Immediate first pass.
	o.tick(ctx)

	ticker := time.NewTicker(orchestratorInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-o.stopCh:
			return
		case <-ticker.C:
			o.tick(ctx)
		}
	}
}

// Stop signals Run() to exit. Safe to call multiple times or before Run().
func (o *Orchestrator) Stop() {
	select {
	case <-o.stopCh:
		// already closed
	default:
		close(o.stopCh)
	}
}

// tick performs one full poll cycle across every lifecycle stage. processSegmenting
// runs first so a job stranded in `segmenting` by an OOM/restart mid-segmentation
// is reclaimed (the first tick fires immediately on startup — see Run — giving
// startup recovery; subsequent ticks give periodic recovery).
func (o *Orchestrator) tick(ctx context.Context) {
	o.processSegmenting(ctx)
	o.processQueued(ctx)
	o.detectAllDone(ctx)
	o.processFinalizing(ctx)
}

// processSegmenting reclaims jobs left stuck in `segmenting` by an OOM/crash/
// restart that interrupted the (otherwise synchronous) segmentJob transition.
// Without this sweep such a job livelocks forever: no other code path re-selects
// a `segmenting` job (processQueued only lists `queued`), so the threat-model's
// "restart mid-segmentation" case would strand the job permanently.
//
// segmentJob is idempotent — it re-runs Resolve/Probe/Segment (which overwrite
// the per-job staging dir) and BulkInsertPending (documented "idempotent — safe
// on retry") — so re-driving a `segmenting` job cleanly re-segments and advances
// it to `upscaling`. This mirrors the library service's ResumeInterruptedEncodes
// startup sweep.
func (o *Orchestrator) processSegmenting(ctx context.Context) {
	jobs, err := o.jobs.List(ctx, repo.JobFilter{Status: domain.JobSegmenting, Limit: 50})
	if err != nil {
		o.log.Warnw("orchestrator: list segmenting jobs failed", "error", err)
		return
	}
	for i := range jobs {
		job := jobs[i] // copy; job is value
		o.log.Infow("orchestrator: reclaiming job stuck in segmenting (restart recovery)", "job_id", job.ID)
		o.runJob(job.ID, func() { o.segmentJob(ctx, &job) })
	}
}

// processQueued advances queued jobs through segmenting → upscaling.
func (o *Orchestrator) processQueued(ctx context.Context) {
	jobs, err := o.jobs.List(ctx, repo.JobFilter{Status: domain.JobQueued, Limit: 50})
	if err != nil {
		o.log.Warnw("orchestrator: list queued jobs failed", "error", err)
		return
	}
	for i := range jobs {
		job := jobs[i] // copy; job is value
		o.runJob(job.ID, func() { o.segmentJob(ctx, &job) })
	}
}

// detectAllDone INDEPENDENTLY flips upscaling jobs to finalizing once every
// segment is done — without relying on a worker lease_req (the I-1 liveness fix).
func (o *Orchestrator) detectAllDone(ctx context.Context) {
	jobs, err := o.jobs.List(ctx, repo.JobFilter{Status: domain.JobUpscaling, Limit: 50})
	if err != nil {
		o.log.Warnw("orchestrator: list upscaling jobs failed", "error", err)
		return
	}
	for i := range jobs {
		job := jobs[i]
		o.runJob(job.ID, func() {
			pending, leased, done, cerr := o.segs.Counts(ctx, job.ID)
			if cerr != nil {
				o.log.Warnw("orchestrator: segment counts failed", "job_id", job.ID, "error", cerr)
				return
			}
			if pending == 0 && leased == 0 && done > 0 {
				// All segments finished. Flip to finalizing. Idempotent with the
				// leaser's flip: if the leaser already flipped, this job won't be
				// in the upscaling list next tick, and a redundant UpdateStatus is
				// harmless.
				if uerr := o.jobs.UpdateStatus(ctx, job.ID, domain.JobFinalizing, ""); uerr != nil {
					o.log.Warnw("orchestrator: flip to finalizing failed", "job_id", job.ID, "error", uerr)
					return
				}
				o.log.Infow("orchestrator: all segments done — finalizing",
					"job_id", job.ID, "done", done)
			}
		})
	}
}

// processFinalizing concats + uploads finalizing jobs and flips them to done.
func (o *Orchestrator) processFinalizing(ctx context.Context) {
	jobs, err := o.jobs.List(ctx, repo.JobFilter{Status: domain.JobFinalizing, Limit: 50})
	if err != nil {
		o.log.Warnw("orchestrator: list finalizing jobs failed", "error", err)
		return
	}
	for i := range jobs {
		job := jobs[i]
		o.runJob(job.ID, func() { o.finalizeJob(ctx, &job) })
	}
}

// segmentJob drives a single queued job: segmenting → upscaling.
// On ANY error (including source.ErrSourceGone) the job is flipped to failed.
func (o *Orchestrator) segmentJob(ctx context.Context, job *domain.UpscaleJob) {
	if err := o.jobs.UpdateStatus(ctx, job.ID, domain.JobSegmenting, ""); err != nil {
		o.log.Warnw("orchestrator: flip to segmenting failed", "job_id", job.ID, "error", err)
		return
	}

	// Resolve the source file into the per-job staging dir.
	srcPath, err := o.resolver.Resolve(ctx, job)
	if err != nil {
		o.failJob(ctx, job.ID, fmt.Sprintf("resolve source: %v", err))
		return
	}

	// Probe for codec/pixfmt/fps/dimensions.
	probe, err := o.prober.Probe(ctx, srcPath)
	if err != nil {
		o.failJob(ctx, job.ID, fmt.Sprintf("probe source: %v", err))
		return
	}

	jobStaging := filepath.Join(o.stagingDir, job.ID)

	// Segment the video-only stream into {staging}/{jobID}/seg_*.mkv.
	segPaths, err := o.segmenter.Segment(ctx, srcPath, jobStaging, o.segmentSeconds)
	if err != nil {
		o.failJob(ctx, job.ID, fmt.Sprintf("segment: %v", err))
		return
	}
	if len(segPaths) == 0 {
		o.failJob(ctx, job.ID, "segment produced no segments")
		return
	}

	// Demux audio/subs/fonts/chapters sidecars to {staging}/{jobID}/.
	if _, err := o.segmenter.DemuxSidecars(ctx, srcPath, jobStaging); err != nil {
		o.failJob(ctx, job.ID, fmt.Sprintf("demux sidecars: %v", err))
		return
	}

	n := len(segPaths)

	// Persist source metadata (codec/pixfmt/fps/HEIGHT) + the ACTUAL segment
	// count. Height is persisted so the finalizer can derive the
	// UPSCALED-{height}p prefix deterministically (height × scale) WITHOUT
	// re-reading the possibly-gone source; the segment count lets the
	// leaser/worker pool know how many segments to expect.
	if err := o.jobs.SetSourceMeta(ctx, job.ID, probe.Codec, probe.PixFmt, probe.FPS, probe.Height, n); err != nil {
		o.failJob(ctx, job.ID, fmt.Sprintf("set source meta: %v", err))
		return
	}

	// Seed the segment rows (idempotent — safe on retry).
	if err := o.segs.BulkInsertPending(ctx, job.ID, n); err != nil {
		o.failJob(ctx, job.ID, fmt.Sprintf("seed segments: %v", err))
		return
	}

	// Hand off to the worker pool.
	if err := o.jobs.UpdateStatus(ctx, job.ID, domain.JobUpscaling, ""); err != nil {
		o.log.Warnw("orchestrator: flip to upscaling failed", "job_id", job.ID, "error", err)
		return
	}

	o.log.Infow("orchestrator: job segmented",
		"job_id", job.ID, "segments", n, "codec", probe.Codec, "pixfmt", probe.PixFmt)
}

// finalizeJob drives a single finalizing job: concat → upload → done.
// On ANY error the job is flipped to failed. Concat + Upload run EXACTLY once per
// finalizing job because the job leaves the finalizing status (→done or →failed)
// at the end of this call, so it is not re-selected on the next tick.
func (o *Orchestrator) finalizeJob(ctx context.Context, job *domain.UpscaleJob) {
	jobStaging := filepath.Join(o.stagingDir, job.ID)
	upscaledDir := filepath.Join(jobStaging, "upscaled")
	hlsDir := filepath.Join(jobStaging, "hls")

	// Re-demux is NOT needed: sidecars were parked under {staging}/{jobID}/ at
	// segmenting time. Reconstruct the Sidecars struct from those known paths,
	// keeping only the ones that actually exist on disk.
	sidecars := reconstructSidecars(jobStaging)

	// Reconstruct the ProbeResult the Finalizer needs from the persisted source
	// meta (Finalizer.Concat only reads PixFmt — for the 8/10-bit pix_fmt
	// decision). This avoids re-reading the original (possibly-gone) source.
	probe := source.ProbeResult{
		Codec:  job.SourceCodec,
		PixFmt: job.SourcePixFmt,
		FPS:    job.SourceFPS,
	}

	// Concat upscaled segments + remux sidecars into an HLS package.
	if err := o.finalizer.Concat(ctx, upscaledDir, sidecars, probe, hlsDir); err != nil {
		o.failJob(ctx, job.ID, fmt.Sprintf("concat/finalize: %v", err))
		return
	}

	// Enumerate the produced HLS files (playlist + .ts segments).
	hlsFiles, err := o.listHLS(hlsDir)
	if err != nil {
		o.failJob(ctx, job.ID, fmt.Sprintf("list hls files: %v", err))
		return
	}
	if len(hlsFiles) == 0 {
		o.failJob(ctx, job.ID, "finalize produced no hls files")
		return
	}

	// Ensure the destination bucket exists.
	if err := o.writer.EnsureBucket(ctx); err != nil {
		o.failJob(ctx, job.ID, fmt.Sprintf("ensure bucket: %v", err))
		return
	}

	// scaleHeight = persisted source height × job.Scale. Both are durable
	// columns set at segmenting time, so the prefix is deterministic and needs
	// NO filesystem access at finalize. When SourceHeight is 0 (legacy/unprobed
	// job) we fall back to job.Scale alone so the prefix stays well-formed; this
	// is logged at warn since the resolution label will be wrong.
	scaleHeight := o.scaleHeight(job)

	prefix := autocache.UpscaledPrefix(job.ShikimoriID, job.Episode, scaleHeight)
	if _, err := o.writer.Upload(ctx, prefix, hlsFiles); err != nil {
		o.failJob(ctx, job.ID, fmt.Sprintf("upload: %v", err))
		return
	}

	if err := o.jobs.SetOutputPrefix(ctx, job.ID, prefix); err != nil {
		o.failJob(ctx, job.ID, fmt.Sprintf("set output prefix: %v", err))
		return
	}

	if err := o.jobs.UpdateStatus(ctx, job.ID, domain.JobDone, ""); err != nil {
		o.log.Warnw("orchestrator: flip to done failed", "job_id", job.ID, "error", err)
		return
	}

	if o.logBuf != nil {
		if err := o.logBuf.Flush(ctx, job.ID); err != nil {
			o.log.Warnw("orchestrator: log flush failed", "job_id", job.ID, "error", err)
		}
	}

	o.log.Infow("orchestrator: job finalized", "job_id", job.ID, "prefix", prefix)
}

// scaleHeight computes the target output height = job.SourceHeight × job.Scale,
// reading ONLY the durable columns persisted at segmenting time — no filesystem
// access. When SourceHeight is 0 (a legacy/unprobed job) it falls back to scale
// alone so the MinIO prefix is still well-formed, logging a warn because the
// resolution label will be inaccurate.
func (o *Orchestrator) scaleHeight(job *domain.UpscaleJob) int {
	scale := job.Scale
	if scale <= 0 {
		scale = 1
	}
	if job.SourceHeight <= 0 {
		o.log.Warnw("orchestrator: SourceHeight not persisted; using scale fallback for prefix",
			"job_id", job.ID, "scale", scale)
		return scale
	}
	return job.SourceHeight * scale
}

// failJob flips a job to failed with the given error text. Logged at warn.
func (o *Orchestrator) failJob(ctx context.Context, id, errText string) {
	o.log.Warnw("orchestrator: job failed", "job_id", id, "error", errText)
	if err := o.jobs.UpdateStatus(ctx, id, domain.JobFailed, errText); err != nil {
		o.log.Warnw("orchestrator: marking job failed errored", "job_id", id, "error", err)
	}
}

// runJob runs fn with a per-job panic recover so one bad job never crashes the
// poller goroutine.
func (o *Orchestrator) runJob(jobID string, fn func()) {
	defer func() {
		if r := recover(); r != nil {
			o.log.Errorw("orchestrator: recovered panic processing job", "job_id", jobID, "panic", r)
		}
	}()
	fn()
}

// ── Filesystem helpers ────────────────────────────────────────────────────────

// reconstructSidecars rebuilds the Sidecars struct from the known sidecar paths
// parked under jobStaging at demux time, including only files that exist on disk.
func reconstructSidecars(jobStaging string) ffmpeg.Sidecars {
	var sc ffmpeg.Sidecars
	if p := filepath.Join(jobStaging, "audio.mka"); nonEmptyFile(p) {
		sc.AudioPath = p
	}
	if p := filepath.Join(jobStaging, "subs.mks"); nonEmptyFile(p) {
		sc.SubPaths = []string{p}
	}
	if fonts := globDir(filepath.Join(jobStaging, "fonts")); len(fonts) > 0 {
		sc.FontPaths = fonts
	}
	if p := filepath.Join(jobStaging, "chapters.ini"); nonEmptyFile(p) {
		sc.ChaptersPath = p
	}
	return sc
}

// listHLSFiles globs playlist + segment files in hlsDir (the production
// hlsLister). Returns the playlist and every .ts segment; ordering is left to
// the uploader (which special-cases playlist.m3u8 to upload last).
func listHLSFiles(hlsDir string) ([]string, error) {
	playlist, _ := filepath.Glob(filepath.Join(hlsDir, "*.m3u8"))
	segments, _ := filepath.Glob(filepath.Join(hlsDir, "*.ts"))
	out := make([]string, 0, len(playlist)+len(segments))
	out = append(out, segments...)
	out = append(out, playlist...)
	return out, nil
}

// nonEmptyFile reports whether path exists and has size > 0.
func nonEmptyFile(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && !fi.IsDir() && fi.Size() > 0
}

// globDir returns the sorted absolute paths of all regular files directly in dir.
func globDir(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() {
			out = append(out, filepath.Join(dir, e.Name()))
		}
	}
	sort.Strings(out)
	return out
}
