package service

import (
	"context"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/scheduler/internal/domain"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func testLogger(t *testing.T) *logger.Logger {
	t.Helper()
	log, err := logger.New(logger.Config{Level: "error"})
	if err != nil {
		t.Fatalf("logger.New: %v", err)
	}
	return log
}

// SeedLastSuccess must prime the gauge from persisted rows (so the series
// survive a container restart instead of false-paging scheduler-sync-stale,
// AUTO-610/611) while skipping rows for jobs the binary no longer runs
// (a resurrected stale series would age past the 25h threshold forever).
func TestSeedLastSuccessFiltersUnknownJobs(t *testing.T) {
	metrics.SchedulerJobLastSuccess.Reset()
	t.Cleanup(metrics.SchedulerJobLastSuccess.Reset)

	at := time.Now().Add(-2 * time.Hour).Truncate(time.Second)
	n := SeedLastSuccess([]domain.JobSuccess{
		{Job: "shikimori_sync", LastSuccessAt: at},
		{Job: "job_removed_from_binary", LastSuccessAt: at},
	}, testLogger(t))

	if n != 1 {
		t.Fatalf("SeedLastSuccess seeded %d series, want 1", n)
	}
	got := testutil.ToFloat64(metrics.SchedulerJobLastSuccess.WithLabelValues("shikimori_sync"))
	if got != float64(at.Unix()) {
		t.Errorf("seeded gauge = %v, want %v", got, at.Unix())
	}
	// The unknown job must not have created a series: exactly the one series
	// we asserted above plus the WithLabelValues call itself.
	if c := testutil.CollectAndCount(metrics.SchedulerJobLastSuccess); c != 1 {
		t.Errorf("gauge has %d series, want 1 (stale job must not be resurrected)", c)
	}
}

// recordSuccess must both refresh the gauge and persist the timestamp; a nil
// store (not wired) must not panic.
func TestRecordSuccessSetsGaugeAndPersists(t *testing.T) {
	metrics.SchedulerJobLastSuccess.Reset()
	t.Cleanup(metrics.SchedulerJobLastSuccess.Reset)

	st := &fakeSuccessStore{}
	s := &JobService{log: testLogger(t)}
	s.SetSuccessStore(st)

	before := time.Now()
	s.recordSuccess(context.Background(), "cleanup")

	if st.job != "cleanup" {
		t.Errorf("persisted job = %q, want %q", st.job, "cleanup")
	}
	if st.at.Before(before.Truncate(time.Second)) {
		t.Errorf("persisted timestamp %v is before test start %v", st.at, before)
	}
	got := testutil.ToFloat64(metrics.SchedulerJobLastSuccess.WithLabelValues("cleanup"))
	if got < float64(before.Unix()) {
		t.Errorf("gauge = %v, want >= %v", got, before.Unix())
	}

	// nil store: gauge still updates, no panic.
	s2 := &JobService{log: testLogger(t)}
	s2.recordSuccess(context.Background(), "top_anime_sync")
	if testutil.ToFloat64(metrics.SchedulerJobLastSuccess.WithLabelValues("top_anime_sync")) == 0 {
		t.Error("nil-store recordSuccess did not set gauge")
	}
}

// Every job name recorded anywhere in the service must be in KnownJobs, or a
// restart would silently drop its persisted seed.
func TestKnownJobsCoversAllRecordedJobs(t *testing.T) {
	want := []string{
		"shikimori_sync", "cleanup", "top_anime_sync", "calendar_sync",
		"playback_probe", "read_threshold_recompute", "provider_ranking_recompute",
		"subtitle_probe", "autocache_logic_a", "autocache_prediction", "fanfic_daily",
	}
	known := map[string]bool{}
	for _, j := range KnownJobs {
		known[j] = true
	}
	for _, j := range want {
		if !known[j] {
			t.Errorf("KnownJobs is missing %q", j)
		}
	}
}

type fakeSuccessStore struct {
	job string
	at  time.Time
}

func (f *fakeSuccessStore) Upsert(_ context.Context, job string, at time.Time) error {
	f.job = job
	f.at = at
	return nil
}
