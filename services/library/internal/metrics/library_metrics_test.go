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
