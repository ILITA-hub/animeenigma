// Package metrics holds the library service's Prometheus collectors.
//
// We use a service-local package (rather than extending libs/metrics)
// because every other service does its own thing — the shared libs
// package only owns the HTTP middleware / DB pool collectors that
// every service uses identically. Library-specific counters belong
// next to the code that emits them.
//
// All six SPEC-locked collectors register against the default
// prometheus registerer via promauto so they auto-appear on the
// existing /metrics endpoint wired by transport/router.go.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// LibraryMetrics bundles the SPEC-locked collectors. Phase 03 added
// six (jobs/download/torrents/disk/enqueue/seed); Phase 04 adds four
// more for the encoder + uploader lanes:
//
//   - encodeDurationSeconds   Histogram — wall time per ffmpeg encode
//   - uploadBytesTotal        Counter   — bytes PUT to MinIO across jobs
//   - filenameDetectFallback  CounterVec{uploader} — generic-fallback hits
//   - encodeFailuresTotal     CounterVec{reason}   — encoder failure attribution
//
// Methods are safe to call from any goroutine.
type LibraryMetrics struct {
	jobsTotal            *prometheus.CounterVec
	downloadBytesTotal   prometheus.Counter
	activeTorrents       prometheus.Gauge
	diskFreeBytes        prometheus.Gauge
	enqueueRejectedTotal *prometheus.CounterVec
	torrentSeedCount     prometheus.Gauge

	// Phase 04 additions:
	encodeDurationSeconds  prometheus.Histogram
	uploadBytesTotal       prometheus.Counter
	filenameDetectFallback *prometheus.CounterVec
	encodeFailuresTotal    *prometheus.CounterVec

	// Phase 06 (workstream raw-jp / v0.2) addition:
	//   cacheInvalidationTotal{result="ok"|"fail"} — incremented
	//   once per webhook fire from the library encoder to the
	//   catalog's /internal/cache/invalidate/raw endpoint.
	cacheInvalidationTotal *prometheus.CounterVec

	// Phase 08 (workstream auto-torrent-population / v4.1) addition:
	//   autocacheServeTotal{result="hit"|"miss"} — incremented once per
	//   ae serve-path resolution (HIT = served from pool, MISS = absent →
	//   failover + backfill demand). Phase 11 charts the serve-hit-rate.
	autocacheServeTotal *prometheus.CounterVec

	// Phase 09 (workstream auto-torrent-population / v4.1) addition:
	//   autocacheDownloadsTotal{trigger,result} — incremented once per Planner
	//   drain decision (OBS-04). trigger ∈ {A (ongoing), B (next_ep), backfill}
	//   derived from the demand reason; result ∈ {enqueued, present, no_release,
	//   dedup, error}. Low cardinality — no mal_id/episode, so /metrics leaks no
	//   per-title data. Phase 11 charts download volume by trigger.
	autocacheDownloadsTotal *prometheus.CounterVec
}

// NewLibraryMetrics registers the collectors against the default
// prometheus registry. Use NewLibraryMetricsWithRegisterer to bind
// against a custom registry — primarily for tests.
func NewLibraryMetrics() *LibraryMetrics {
	return NewLibraryMetricsWithRegisterer(prometheus.DefaultRegisterer)
}

// NewLibraryMetricsWithRegisterer is the test seam. Passing a fresh
// prometheus.NewRegistry() lets each test register cleanly without
// the "duplicate collector" panic promauto raises when the same
// metric name is registered twice against DefaultRegisterer.
func NewLibraryMetricsWithRegisterer(reg prometheus.Registerer) *LibraryMetrics {
	factory := promauto.With(reg)
	return &LibraryMetrics{
		jobsTotal: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "library_jobs_total",
				Help: "Total number of library_jobs status transitions, labeled by the new status.",
			},
			[]string{"status"},
		),
		downloadBytesTotal: factory.NewCounter(
			prometheus.CounterOpts{
				Name: "library_download_bytes_total",
				Help: "Total bytes pulled by the torrent client across all jobs since process start.",
			},
		),
		activeTorrents: factory.NewGauge(
			prometheus.GaugeOpts{
				Name: "library_active_torrents",
				Help: "Number of torrents currently being downloaded (in-memory DownloadHandle count).",
			},
		),
		diskFreeBytes: factory.NewGauge(
			prometheus.GaugeOpts{
				Name: "library_disk_free_bytes",
				Help: "Bytes free under LIBRARY_TORRENT_DOWNLOAD_DIR, refreshed by the disk guard goroutine.",
			},
		),
		enqueueRejectedTotal: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "library_enqueue_rejected_total",
				Help: "Total enqueue requests rejected, labeled by reason (invalid_magnet, disk_full, ...).",
			},
			[]string{"reason"},
		),
		torrentSeedCount: factory.NewGauge(
			prometheus.GaugeOpts{
				Name: "library_torrent_seed_count",
				Help: "Number of completed torrents currently seeding (post-download, pre-drop).",
			},
		),

		// Phase 04 — encoder + uploader lane.
		encodeDurationSeconds: factory.NewHistogram(
			prometheus.HistogramOpts{
				Name:    "library_encode_duration_seconds",
				Help:    "Wall time taken by ffmpeg to transcode one source file.",
				Buckets: prometheus.ExponentialBuckets(10, 2, 8), // 10s .. 1280s
			},
		),
		uploadBytesTotal: factory.NewCounter(
			prometheus.CounterOpts{
				Name: "library_upload_bytes_total",
				Help: "Total bytes uploaded to MinIO across all completed jobs.",
			},
		),
		filenameDetectFallback: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "library_filename_detect_fallback_total",
				Help: "Generic-fallback regex hits in the filename detector, labeled by uploader (empty → \"unknown\").",
			},
			[]string{"uploader"},
		),
		encodeFailuresTotal: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "library_encode_failures_total",
				Help: "Encoder-worker failures, labeled by reason (source_missing, episode_detect_failed, ffmpeg_error, upload_error, episode_insert_failed).",
			},
			[]string{"reason"},
		),

		// Phase 06 (workstream raw-jp / v0.2).
		cacheInvalidationTotal: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "library_cache_invalidation_total",
				Help: "Cache-invalidation webhook fires from library to catalog, labeled by result (ok|fail).",
			},
			[]string{"result"},
		),

		// Phase 08 (workstream auto-torrent-population / v4.1).
		autocacheServeTotal: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "library_autocache_serve_total",
				Help: "ae serve-path resolutions, labeled by result (hit|miss).",
			},
			[]string{"result"},
		),

		// Phase 09 (workstream auto-torrent-population / v4.1).
		autocacheDownloadsTotal: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "library_autocache_downloads_total",
				Help: "Planner download decisions, labeled by trigger (A=ongoing|B=next_ep|backfill) and result (enqueued|present|no_release|dedup|error).",
			},
			[]string{"trigger", "result"},
		),
	}
}

// IncJobsTotal increments library_jobs_total{status=newStatus}.
// Callers should invoke this on every status transition so the panel
// "Job status counts (24h)" reflects the actual workflow.
func (m *LibraryMetrics) IncJobsTotal(status string) {
	m.jobsTotal.WithLabelValues(status).Inc()
}

// AddDownloadBytes adds n bytes to the total. Callers pass a delta
// since the previous tick — NOT the cumulative byte count for the
// torrent — so the counter stays monotonic across torrent additions.
func (m *LibraryMetrics) AddDownloadBytes(n int64) {
	if n <= 0 {
		return
	}
	m.downloadBytesTotal.Add(float64(n))
}

// SetActiveTorrents reflects the in-memory handle map size. The
// worker pool publishes this on a 5s tick.
func (m *LibraryMetrics) SetActiveTorrents(n int) {
	m.activeTorrents.Set(float64(n))
}

// SetDiskFreeBytes updates the gauge from the disk guard's Statfs
// reading.
func (m *LibraryMetrics) SetDiskFreeBytes(n uint64) {
	m.diskFreeBytes.Set(float64(n))
}

// IncEnqueueRejected increments library_enqueue_rejected_total with
// the provided reason label. Reasons used by the handler:
//   - "invalid_magnet" — metainfo.ParseMagnetUri returned an error
//   - "disk_full"      — diskGuard.Allow returned false
func (m *LibraryMetrics) IncEnqueueRejected(reason string) {
	m.enqueueRejectedTotal.WithLabelValues(reason).Inc()
}

// SetSeedCount reflects the number of torrents in the seeding window
// (post-Complete, pre-Drop). The worker pool publishes this alongside
// SetActiveTorrents on the same tick.
func (m *LibraryMetrics) SetSeedCount(n int) {
	m.torrentSeedCount.Set(float64(n))
}

// GetJobsTotalForTest returns the underlying Counter for the given
// status label so tests can read its value via testutil.ToFloat64
// without exporting the whole CounterVec. Production code MUST NOT
// call this — use IncJobsTotal.
func (m *LibraryMetrics) GetJobsTotalForTest(status string) prometheus.Counter {
	return m.jobsTotal.WithLabelValues(status)
}

// GetEnqueueRejectedForTest is the test-seam analogue for
// library_enqueue_rejected_total.
func (m *LibraryMetrics) GetEnqueueRejectedForTest(reason string) prometheus.Counter {
	return m.enqueueRejectedTotal.WithLabelValues(reason)
}

// ObserveEncodeDuration records one ffmpeg wall-time sample.
func (m *LibraryMetrics) ObserveEncodeDuration(seconds float64) {
	if m == nil {
		return
	}
	m.encodeDurationSeconds.Observe(seconds)
}

// AddUploadBytes adds n bytes to library_upload_bytes_total. Guards
// against n <= 0 (mirrors AddDownloadBytes) so a buggy uploader can
// never make the counter non-monotonic.
func (m *LibraryMetrics) AddUploadBytes(n int64) {
	if m == nil || n <= 0 {
		return
	}
	m.uploadBytesTotal.Add(float64(n))
}

// IncFilenameDetectFallback increments
// library_filename_detect_fallback_total{uploader}. An empty uploader
// label is normalised to "unknown" so prometheus never sees an empty
// label value.
func (m *LibraryMetrics) IncFilenameDetectFallback(uploader string) {
	if m == nil {
		return
	}
	if uploader == "" {
		uploader = "unknown"
	}
	m.filenameDetectFallback.WithLabelValues(uploader).Inc()
}

// IncEncodeFailures increments library_encode_failures_total{reason}.
// Reasons used by the encoder worker:
//   - "source_missing"        — SourcePathResolver returned an error
//   - "episode_detect_failed" — detector returned (0, false)
//   - "ffmpeg_error"          — Transcode returned non-nil error
//   - "upload_error"          — MinIO writer returned non-nil error
//   - "episode_insert_failed" — episode row insertion failed
//   - "invalid_magnet"        — magnet failed to re-parse at encode time
func (m *LibraryMetrics) IncEncodeFailures(reason string) {
	if m == nil {
		return
	}
	m.encodeFailuresTotal.WithLabelValues(reason).Inc()
}

// GetEncodeFailuresForTest is the test-seam analogue for
// library_encode_failures_total.
func (m *LibraryMetrics) GetEncodeFailuresForTest(reason string) prometheus.Counter {
	return m.encodeFailuresTotal.WithLabelValues(reason)
}

// GetFilenameDetectFallbackForTest is the test-seam analogue for
// library_filename_detect_fallback_total.
func (m *LibraryMetrics) GetFilenameDetectFallbackForTest(uploader string) prometheus.Counter {
	if uploader == "" {
		uploader = "unknown"
	}
	return m.filenameDetectFallback.WithLabelValues(uploader)
}

// GetUploadBytesForTest exposes the upload-bytes counter for tests.
func (m *LibraryMetrics) GetUploadBytesForTest() prometheus.Counter {
	return m.uploadBytesTotal
}

// IncCacheInvalidation increments library_cache_invalidation_total
// with result label "ok" (HTTP 2xx response) or "fail" (everything
// else — non-2xx, transport error, timeout). Phase 06 (workstream
// raw-jp / v0.2).
func (m *LibraryMetrics) IncCacheInvalidation(result string) {
	if m == nil {
		return
	}
	m.cacheInvalidationTotal.WithLabelValues(result).Inc()
}

// GetCacheInvalidationForTest is the test-seam analogue for
// library_cache_invalidation_total.
func (m *LibraryMetrics) GetCacheInvalidationForTest(result string) prometheus.Counter {
	return m.cacheInvalidationTotal.WithLabelValues(result)
}

// IncServeTotal increments library_autocache_serve_total with result label
// "hit" (episode served from the pool) or "miss" (episode absent → failover +
// backfill demand). Only the low-cardinality result label — no mal_id/episode,
// so /metrics leaks no per-title viewing data. Phase 08 (SERVE-01/03); Phase 11
// charts this series.
func (m *LibraryMetrics) IncServeTotal(result string) {
	if m == nil {
		return
	}
	m.autocacheServeTotal.WithLabelValues(result).Inc()
}

// GetServeTotalForTest is the test-seam analogue for
// library_autocache_serve_total.
func (m *LibraryMetrics) GetServeTotalForTest(result string) prometheus.Counter {
	return m.autocacheServeTotal.WithLabelValues(result)
}

// IncDownloadsTotal increments library_autocache_downloads_total{trigger,result}.
// The Phase-09 Planner calls this once per drain decision: trigger derived from
// the demand reason (ongoing→"A", next_ep→"B", backfill→"backfill") and result in
// {enqueued, present, no_release, dedup, error}. Only low-cardinality labels — no
// mal_id/episode, so /metrics leaks no per-title data. Nil-guarded so a Planner
// with metrics disabled never panics.
func (m *LibraryMetrics) IncDownloadsTotal(trigger, result string) {
	if m == nil {
		return
	}
	m.autocacheDownloadsTotal.WithLabelValues(trigger, result).Inc()
}

// GetDownloadsTotalForTest is the test-seam analogue for
// library_autocache_downloads_total.
func (m *LibraryMetrics) GetDownloadsTotalForTest(trigger, result string) prometheus.Counter {
	return m.autocacheDownloadsTotal.WithLabelValues(trigger, result)
}
