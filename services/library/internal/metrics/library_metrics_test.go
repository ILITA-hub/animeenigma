package metrics

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// TestLibraryMetrics_ExposesAllCollectors confirms every SPEC-locked
// collector is registered under the exact name listed in the
// 03-SPEC.md acceptance section. A future rename would silently break
// the dashboard JSON; this test pins the contract.
func TestLibraryMetrics_ExposesAllCollectors(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewLibraryMetricsWithRegisterer(reg)

	// Touch every collector so it appears in the Gather() output
	// even if the value is zero (CounterVec needs a labelset to
	// render).
	m.IncJobsTotal("queued")
	m.AddDownloadBytes(1024)
	m.SetActiveTorrents(2)
	m.SetDiskFreeBytes(123456)
	m.IncEnqueueRejected("disk_full")
	m.SetSeedCount(1)
	// Phase 04 additions:
	m.ObserveEncodeDuration(10)
	m.AddUploadBytes(2048)
	m.IncFilenameDetectFallback("Ohys-Raws")
	m.IncEncodeFailures("ffmpeg_error")
	// Phase 10 additions — touch so the Vec collectors render a labelset:
	m.IncEvictedTotal("autocache")
	m.IncRejectedTotal("budget_full")
	m.SetBytesUsed("autocache", "stale", 1)
	m.SetBudgetBytes(107374182400)
	m.SetEpisodes("autocache", "stale", 1)

	expected := []string{
		"library_jobs_total",
		"library_download_bytes_total",
		"library_active_torrents",
		"library_disk_free_bytes",
		"library_enqueue_rejected_total",
		"library_torrent_seed_count",
		"library_encode_duration_seconds",
		"library_upload_bytes_total",
		"library_filename_detect_fallback_total",
		"library_encode_failures_total",
		"library_autocache_evicted_total",
		"library_autocache_rejected_total",
		"library_autocache_bytes_used",
		"library_autocache_budget_bytes",
		"library_autocache_episodes",
	}

	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	got := map[string]bool{}
	for _, f := range families {
		got[f.GetName()] = true
	}
	for _, name := range expected {
		if !got[name] {
			var have []string
			for n := range got {
				have = append(have, n)
			}
			t.Fatalf("missing collector %q; have: %s", name, strings.Join(have, ","))
		}
	}
}

// TestLibraryMetrics_IncJobsTotal — label value flows through to the
// counter so the Grafana panel sees per-status bars.
func TestLibraryMetrics_IncJobsTotal(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewLibraryMetricsWithRegisterer(reg)

	m.IncJobsTotal("downloading")
	m.IncJobsTotal("downloading")
	m.IncJobsTotal("failed")

	if v := testutil.ToFloat64(m.jobsTotal.WithLabelValues("downloading")); v != 2 {
		t.Fatalf("library_jobs_total{status=downloading} = %v, want 2", v)
	}
	if v := testutil.ToFloat64(m.jobsTotal.WithLabelValues("failed")); v != 1 {
		t.Fatalf("library_jobs_total{status=failed} = %v, want 1", v)
	}
}

// TestLibraryMetrics_AddDownloadBytes — counter only goes up; zero
// or negative deltas are silently ignored so a misbehaving caller
// can't corrupt the cumulative total.
func TestLibraryMetrics_AddDownloadBytes(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewLibraryMetricsWithRegisterer(reg)

	m.AddDownloadBytes(100)
	m.AddDownloadBytes(0)   // no-op
	m.AddDownloadBytes(-50) // no-op (counter must be monotonic)
	m.AddDownloadBytes(25)

	if v := testutil.ToFloat64(m.downloadBytesTotal); v != 125 {
		t.Fatalf("library_download_bytes_total = %v, want 125", v)
	}
}

// TestLibraryMetrics_Gauges — Active / Seed / DiskFree are last-write-wins.
func TestLibraryMetrics_Gauges(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewLibraryMetricsWithRegisterer(reg)

	m.SetActiveTorrents(3)
	m.SetActiveTorrents(5) // overwrite, not accumulate
	if v := testutil.ToFloat64(m.activeTorrents); v != 5 {
		t.Fatalf("library_active_torrents = %v, want 5", v)
	}

	m.SetDiskFreeBytes(2_000_000_000)
	if v := testutil.ToFloat64(m.diskFreeBytes); v != 2_000_000_000 {
		t.Fatalf("library_disk_free_bytes = %v, want 2e9", v)
	}

	m.SetSeedCount(7)
	if v := testutil.ToFloat64(m.torrentSeedCount); v != 7 {
		t.Fatalf("library_torrent_seed_count = %v, want 7", v)
	}
}

// TestLibraryMetrics_IncEnqueueRejected — distinct reasons stay in
// distinct labelsets.
func TestLibraryMetrics_IncEnqueueRejected(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewLibraryMetricsWithRegisterer(reg)

	m.IncEnqueueRejected("invalid_magnet")
	m.IncEnqueueRejected("disk_full")
	m.IncEnqueueRejected("disk_full")

	if v := testutil.ToFloat64(m.enqueueRejectedTotal.WithLabelValues("invalid_magnet")); v != 1 {
		t.Fatalf("invalid_magnet count = %v, want 1", v)
	}
	if v := testutil.ToFloat64(m.enqueueRejectedTotal.WithLabelValues("disk_full")); v != 2 {
		t.Fatalf("disk_full count = %v, want 2", v)
	}
}

// TestLibraryMetrics_AddUploadBytes — same monotonic guard as
// AddDownloadBytes (zero / negative are no-ops).
func TestLibraryMetrics_AddUploadBytes(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewLibraryMetricsWithRegisterer(reg)
	m.AddUploadBytes(500)
	m.AddUploadBytes(0)
	m.AddUploadBytes(-99)
	m.AddUploadBytes(250)
	if v := testutil.ToFloat64(m.GetUploadBytesForTest()); v != 750 {
		t.Fatalf("library_upload_bytes_total = %v, want 750", v)
	}
}

// TestLibraryMetrics_IncFilenameDetectFallback — empty uploader is
// normalised to "unknown" so prometheus rejects nothing.
func TestLibraryMetrics_IncFilenameDetectFallback(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewLibraryMetricsWithRegisterer(reg)
	m.IncFilenameDetectFallback("")
	m.IncFilenameDetectFallback("Ohys-Raws")
	m.IncFilenameDetectFallback("Ohys-Raws")

	if v := testutil.ToFloat64(m.GetFilenameDetectFallbackForTest("unknown")); v != 1 {
		t.Fatalf("fallback {unknown} = %v, want 1", v)
	}
	if v := testutil.ToFloat64(m.GetFilenameDetectFallbackForTest("Ohys-Raws")); v != 2 {
		t.Fatalf("fallback {Ohys-Raws} = %v, want 2", v)
	}
}

// TestLibraryMetrics_IncEncodeFailures — per-reason labels stay distinct.
func TestLibraryMetrics_IncEncodeFailures(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewLibraryMetricsWithRegisterer(reg)
	m.IncEncodeFailures("ffmpeg_error")
	m.IncEncodeFailures("ffmpeg_error")
	m.IncEncodeFailures("upload_error")
	if v := testutil.ToFloat64(m.GetEncodeFailuresForTest("ffmpeg_error")); v != 2 {
		t.Fatalf("ffmpeg_error = %v, want 2", v)
	}
	if v := testutil.ToFloat64(m.GetEncodeFailuresForTest("upload_error")); v != 1 {
		t.Fatalf("upload_error = %v, want 1", v)
	}
}

// TestLibraryMetrics_ObserveEncodeDuration — histogram observes one
// sample correctly (we only verify count via Gather since
// testutil.ToFloat64 doesn't work on a Histogram).
func TestLibraryMetrics_ObserveEncodeDuration(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewLibraryMetricsWithRegisterer(reg)
	m.ObserveEncodeDuration(15.0)
	m.ObserveEncodeDuration(42.0)

	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	var sampleCount uint64
	for _, f := range families {
		if f.GetName() == "library_encode_duration_seconds" {
			for _, m := range f.GetMetric() {
				if h := m.GetHistogram(); h != nil {
					sampleCount += h.GetSampleCount()
				}
			}
		}
	}
	if sampleCount != 2 {
		t.Fatalf("encode_duration sample count = %d, want 2", sampleCount)
	}
}

// TestLibraryMetrics_IncServeTotal — Phase 08 ae serve-path counter: hit / miss
// labels stay in distinct labelsets, and the nil-guard makes Inc safe on a nil
// receiver (so a serve path with metrics disabled never panics).
func TestLibraryMetrics_IncServeTotal(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewLibraryMetricsWithRegisterer(reg)

	m.IncServeTotal("hit")
	m.IncServeTotal("hit")
	m.IncServeTotal("miss")

	if v := testutil.ToFloat64(m.GetServeTotalForTest("hit")); v != 2 {
		t.Fatalf("library_autocache_serve_total{result=hit} = %v, want 2", v)
	}
	if v := testutil.ToFloat64(m.GetServeTotalForTest("miss")); v != 1 {
		t.Fatalf("library_autocache_serve_total{result=miss} = %v, want 1", v)
	}

	// nil-receiver guard — must not panic.
	var nilM *LibraryMetrics
	nilM.IncServeTotal("hit")
}

// TestLibraryMetrics_ServeTotalRegistered — the new counter appears under its
// exact SPEC name so the Phase-11 serve-hit-rate panel finds the series.
func TestLibraryMetrics_ServeTotalRegistered(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewLibraryMetricsWithRegisterer(reg)
	m.IncServeTotal("hit") // touch so the CounterVec renders

	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	for _, f := range families {
		if f.GetName() == "library_autocache_serve_total" {
			return
		}
	}
	t.Fatal("library_autocache_serve_total not registered")
}

// TestLibraryMetrics_IncDownloadsTotal — Phase 09 Planner counter: distinct
// (trigger,result) pairs stay in distinct labelsets, and the nil-guard makes Inc
// safe on a nil receiver (so a Planner with metrics disabled never panics).
func TestLibraryMetrics_IncDownloadsTotal(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewLibraryMetricsWithRegisterer(reg)

	m.IncDownloadsTotal("A", "enqueued")
	m.IncDownloadsTotal("A", "enqueued")
	m.IncDownloadsTotal("B", "present")
	m.IncDownloadsTotal("backfill", "no_release")

	if v := testutil.ToFloat64(m.GetDownloadsTotalForTest("A", "enqueued")); v != 2 {
		t.Fatalf("downloads_total{trigger=A,result=enqueued} = %v, want 2", v)
	}
	if v := testutil.ToFloat64(m.GetDownloadsTotalForTest("B", "present")); v != 1 {
		t.Fatalf("downloads_total{trigger=B,result=present} = %v, want 1", v)
	}
	if v := testutil.ToFloat64(m.GetDownloadsTotalForTest("backfill", "no_release")); v != 1 {
		t.Fatalf("downloads_total{trigger=backfill,result=no_release} = %v, want 1", v)
	}

	// nil-receiver guard — must not panic.
	var nilM *LibraryMetrics
	nilM.IncDownloadsTotal("A", "enqueued")
}

// TestLibraryMetrics_DownloadsTotalRegistered — the new counter appears under its
// exact SPEC name with the {trigger,result} labelset so the Phase-11 download
// panel finds the series.
func TestLibraryMetrics_DownloadsTotalRegistered(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewLibraryMetricsWithRegisterer(reg)
	m.IncDownloadsTotal("A", "enqueued") // touch so the CounterVec renders

	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	for _, f := range families {
		if f.GetName() == "library_autocache_downloads_total" {
			return
		}
	}
	t.Fatal("library_autocache_downloads_total not registered")
}

// TestLibraryMetrics_IncEvictedTotal — Phase 10 eviction counter: per-source
// labels stay distinct, and the nil-guard makes Inc safe on a nil receiver.
func TestLibraryMetrics_IncEvictedTotal(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewLibraryMetricsWithRegisterer(reg)

	m.IncEvictedTotal("autocache")
	m.IncEvictedTotal("autocache")
	m.IncEvictedTotal("admin")

	if v := testutil.ToFloat64(m.GetEvictedTotalForTest("autocache")); v != 2 {
		t.Fatalf("library_autocache_evicted_total{source=autocache} = %v, want 2", v)
	}
	if v := testutil.ToFloat64(m.GetEvictedTotalForTest("admin")); v != 1 {
		t.Fatalf("library_autocache_evicted_total{source=admin} = %v, want 1", v)
	}

	var nilM *LibraryMetrics
	nilM.IncEvictedTotal("autocache") // must not panic
}

// TestLibraryMetrics_IncRejectedTotal — Phase 10 pre-admit reject counter
// (EVICT-04): per-reason labels stay distinct; nil-guarded.
func TestLibraryMetrics_IncRejectedTotal(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewLibraryMetricsWithRegisterer(reg)

	m.IncRejectedTotal("budget_full")
	m.IncRejectedTotal("budget_full")

	if v := testutil.ToFloat64(m.GetRejectedTotalForTest("budget_full")); v != 2 {
		t.Fatalf("library_autocache_rejected_total{reason=budget_full} = %v, want 2", v)
	}

	var nilM *LibraryMetrics
	nilM.IncRejectedTotal("budget_full") // must not panic
}

// TestLibraryMetrics_SetBytesUsedAndEpisodes — Phase 10 Accountant GaugeVecs are
// last-write-wins and keep {source,freshness} labelsets distinct; nil-guarded.
func TestLibraryMetrics_SetBytesUsedAndEpisodes(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewLibraryMetricsWithRegisterer(reg)

	m.SetBytesUsed("autocache", "fresh", 1000)
	m.SetBytesUsed("autocache", "fresh", 2000) // overwrite
	m.SetBytesUsed("admin", "stale", 50)
	if v := testutil.ToFloat64(m.bytesUsed.WithLabelValues("autocache", "fresh")); v != 2000 {
		t.Fatalf("bytes_used{autocache,fresh} = %v, want 2000", v)
	}
	if v := testutil.ToFloat64(m.bytesUsed.WithLabelValues("admin", "stale")); v != 50 {
		t.Fatalf("bytes_used{admin,stale} = %v, want 50", v)
	}

	m.SetEpisodes("autocache", "stale", 12)
	m.SetEpisodes("autocache", "stale", 7) // overwrite
	if v := testutil.ToFloat64(m.episodes.WithLabelValues("autocache", "stale")); v != 7 {
		t.Fatalf("episodes{autocache,stale} = %v, want 7", v)
	}

	var nilM *LibraryMetrics
	nilM.SetBytesUsed("autocache", "fresh", 1) // must not panic
	nilM.SetEpisodes("autocache", "fresh", 1)  // must not panic
}

// TestLibraryMetrics_SetBudgetBytes — Phase 10 budget gauge is last-write-wins;
// nil-guarded.
func TestLibraryMetrics_SetBudgetBytes(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewLibraryMetricsWithRegisterer(reg)

	m.SetBudgetBytes(100)
	m.SetBudgetBytes(107374182400) // overwrite
	if v := testutil.ToFloat64(m.budgetBytes); v != 107374182400 {
		t.Fatalf("library_autocache_budget_bytes = %v, want 107374182400", v)
	}

	var nilM *LibraryMetrics
	nilM.SetBudgetBytes(1) // must not panic
}
