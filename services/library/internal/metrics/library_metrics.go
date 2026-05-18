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

// LibraryMetrics bundles the six library-specific collectors locked
// by 03-SPEC. Methods are safe to call from any goroutine.
type LibraryMetrics struct {
	jobsTotal            *prometheus.CounterVec
	downloadBytesTotal   prometheus.Counter
	activeTorrents       prometheus.Gauge
	diskFreeBytes        prometheus.Gauge
	enqueueRejectedTotal *prometheus.CounterVec
	torrentSeedCount     prometheus.Gauge
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
