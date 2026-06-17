package autocache

import (
	"context"
	"testing"
	"time"

	liberrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/services/library/internal/domain"
	libmetrics "github.com/ILITA-hub/animeenigma/services/library/internal/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// promValue reads a counter's current value via testutil.ToFloat64.
func promValue(t *testing.T, c prometheus.Counter) float64 {
	t.Helper()
	return testutil.ToFloat64(c)
}

// fakeDrainer is an in-memory demandDrainer. drained is returned by Drain;
// deletes records each Delete(malID, episode).
type fakeDrainer struct {
	drained []domain.AutocacheDemand
	deletes [][2]any // {malID, episode}
}

func (f *fakeDrainer) Drain(_ context.Context, limit int) ([]domain.AutocacheDemand, error) {
	if limit <= 0 || len(f.drained) == 0 {
		return nil, nil
	}
	if limit < len(f.drained) {
		return f.drained[:limit], nil
	}
	return f.drained, nil
}

func (f *fakeDrainer) Delete(_ context.Context, malID string, episode int) error {
	f.deletes = append(f.deletes, [2]any{malID, episode})
	return nil
}

// fakePresence is an in-memory presenceChecker. present maps "mal:ep" → true
// (returns a row); otherwise returns liberrors.NotFound.
type fakePresence struct {
	present map[string]bool
}

func (f *fakePresence) GetByShikimoriEpisode(_ context.Context, shikimoriID string, ep int) (*domain.Episode, error) {
	if f.present[key(shikimoriID, ep)] {
		return &domain.Episode{ShikimoriID: shikimoriID, EpisodeNumber: ep}, nil
	}
	return nil, liberrors.NotFound("episode")
}

// fakeEnqueuer is an in-memory jobEnqueuer. active maps "mal:ep" → true
// (HasActiveForEpisode); created accumulates every Create call.
type fakeEnqueuer struct {
	active  map[string]bool
	created []*domain.Job
}

func (f *fakeEnqueuer) HasActiveForEpisode(_ context.Context, shikimoriID string, ep int) (bool, error) {
	return f.active[key(shikimoriID, ep)], nil
}

func (f *fakeEnqueuer) Create(_ context.Context, job *domain.Job) error {
	f.created = append(f.created, job)
	return nil
}

// fakeSearcher returns a fixed result for any query.
type fakeSearcher struct {
	releases []domain.Release
}

func (f *fakeSearcher) FetchAll(_ context.Context, _ SearchQuery) (SearchResult, error) {
	return SearchResult{Releases: f.releases}, nil
}

// fakeConfig returns a fixed config.
type fakeConfig struct {
	cfg domain.AutocacheConfig
}

func (f *fakeConfig) Get(_ context.Context) (*domain.AutocacheConfig, error) {
	c := f.cfg
	return &c, nil
}

func key(malID string, ep int) string { return malID + ":" + itoa(ep) }

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

func enabledCfg() domain.AutocacheConfig {
	return domain.AutocacheConfig{Enabled: true, QualityCap: 1080, MinSeeders: 3, SweepIntervalMin: 20}
}

func newPlannerForTest(d *fakeDrainer, p *fakePresence, e *fakeEnqueuer, s *fakeSearcher, c *fakeConfig, m *libmetrics.LibraryMetrics) *Planner {
	return NewPlanner(d, p, e, s, c, m, nil)
}

func winningRelease() []domain.Release {
	return []domain.Release{{Title: "Anime - 05 [1080p]", Magnet: "magnet:?xt=urn:btih:abc", Uploader: "Ohys-Raws", Quality: "1080p", SizeBytes: 123, Seeders: 9}}
}

func TestPlannerDisabledNoOp(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := libmetrics.NewLibraryMetricsWithRegisterer(reg)
	d := &fakeDrainer{drained: []domain.AutocacheDemand{{MALID: "1", Episode: 5, Reason: domain.DemandReasonOngoing}}}
	p := &fakePresence{present: map[string]bool{}}
	e := &fakeEnqueuer{active: map[string]bool{}}
	s := &fakeSearcher{releases: winningRelease()}
	c := &fakeConfig{cfg: domain.AutocacheConfig{Enabled: false, SweepIntervalMin: 20}}

	pl := newPlannerForTest(d, p, e, s, c, m)
	pl.runOnce(context.Background())

	if len(e.created) != 0 {
		t.Fatalf("disabled config must enqueue nothing, got %d jobs", len(e.created))
	}
	if len(d.deletes) != 0 {
		t.Fatalf("disabled config must delete nothing, got %d", len(d.deletes))
	}
}

func TestPlannerPresentDeletesAndCounts(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := libmetrics.NewLibraryMetricsWithRegisterer(reg)
	d := &fakeDrainer{drained: []domain.AutocacheDemand{{MALID: "1", Episode: 5, Reason: domain.DemandReasonBackfill}}}
	p := &fakePresence{present: map[string]bool{"1:5": true}}
	e := &fakeEnqueuer{active: map[string]bool{}}
	s := &fakeSearcher{releases: winningRelease()}
	c := &fakeConfig{cfg: enabledCfg()}

	pl := newPlannerForTest(d, p, e, s, c, m)
	pl.runOnce(context.Background())

	if len(e.created) != 0 {
		t.Fatalf("present episode must enqueue nothing, got %d", len(e.created))
	}
	if len(d.deletes) != 1 {
		t.Fatalf("present episode must delete demand once, got %d", len(d.deletes))
	}
	if got := promValue(t, m.GetDownloadsTotalForTest("backfill", "present")); got != 1 {
		t.Fatalf("present count = %v, want 1", got)
	}
}

func TestPlannerInFlightDedup(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := libmetrics.NewLibraryMetricsWithRegisterer(reg)
	d := &fakeDrainer{drained: []domain.AutocacheDemand{{MALID: "1", Episode: 5, Reason: domain.DemandReasonNextEp}}}
	p := &fakePresence{present: map[string]bool{}}
	e := &fakeEnqueuer{active: map[string]bool{"1:5": true}}
	s := &fakeSearcher{releases: winningRelease()}
	c := &fakeConfig{cfg: enabledCfg()}

	pl := newPlannerForTest(d, p, e, s, c, m)
	pl.runOnce(context.Background())

	if len(e.created) != 0 {
		t.Fatalf("in-flight episode must enqueue no second job, got %d", len(e.created))
	}
	if len(d.deletes) != 0 {
		t.Fatalf("in-flight episode must leave demand row, got %d deletes", len(d.deletes))
	}
	if got := promValue(t, m.GetDownloadsTotalForTest("B", "dedup")); got != 1 {
		t.Fatalf("dedup count = %v, want 1", got)
	}
}

func TestPlannerWinningReleaseEnqueues(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := libmetrics.NewLibraryMetricsWithRegisterer(reg)
	d := &fakeDrainer{drained: []domain.AutocacheDemand{{MALID: "42", Episode: 5, Reason: domain.DemandReasonNextEp}}}
	p := &fakePresence{present: map[string]bool{}}
	e := &fakeEnqueuer{active: map[string]bool{}}
	s := &fakeSearcher{releases: winningRelease()}
	c := &fakeConfig{cfg: enabledCfg()}

	pl := newPlannerForTest(d, p, e, s, c, m)
	pl.runOnce(context.Background())

	if len(e.created) != 1 {
		t.Fatalf("winning release must enqueue exactly one job, got %d", len(e.created))
	}
	job := e.created[0]
	if job.Source != domain.JobSourceAutocache {
		t.Errorf("job source = %q, want autocache", job.Source)
	}
	if job.Episode == nil || *job.Episode != 5 {
		t.Errorf("job episode = %v, want 5", job.Episode)
	}
	if job.ShikimoriID != "42" {
		t.Errorf("job shikimori_id = %q, want 42", job.ShikimoriID)
	}
	if job.Status != domain.JobStatusQueued {
		t.Errorf("job status = %q, want queued", job.Status)
	}
	if len(d.deletes) != 0 {
		t.Fatalf("fresh enqueue must leave demand row, got %d deletes", len(d.deletes))
	}
	if got := promValue(t, m.GetDownloadsTotalForTest("B", "enqueued")); got != 1 {
		t.Fatalf("enqueued count = %v, want 1", got)
	}
}

func TestPlannerNoReleaseLeavesDemand(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := libmetrics.NewLibraryMetricsWithRegisterer(reg)
	d := &fakeDrainer{drained: []domain.AutocacheDemand{{MALID: "1", Episode: 5, Reason: domain.DemandReasonBackfill}}}
	p := &fakePresence{present: map[string]bool{}}
	e := &fakeEnqueuer{active: map[string]bool{}}
	s := &fakeSearcher{releases: nil} // no qualifying release
	c := &fakeConfig{cfg: enabledCfg()}

	pl := newPlannerForTest(d, p, e, s, c, m)
	pl.runOnce(context.Background())

	if len(e.created) != 0 {
		t.Fatalf("no-release must enqueue nothing, got %d", len(e.created))
	}
	if len(d.deletes) != 0 {
		t.Fatalf("no-release must LEAVE the demand row, got %d deletes", len(d.deletes))
	}
	if got := promValue(t, m.GetDownloadsTotalForTest("backfill", "no_release")); got != 1 {
		t.Fatalf("no_release count = %v, want 1", got)
	}
}

// TestPlannerReasonTriggerMapping is the load-bearing end-to-end attribution
// assertion: a drained reason="ongoing" demand increments trigger="A",
// "next_ep" → "B", "backfill" → "backfill" — NOT merely that the counter moved.
func TestPlannerReasonTriggerMapping(t *testing.T) {
	cases := []struct {
		reason      domain.DemandReason
		wantTrigger string
	}{
		{domain.DemandReasonOngoing, "A"},
		{domain.DemandReasonNextEp, "B"},
		{domain.DemandReasonBackfill, "backfill"},
	}
	for _, tc := range cases {
		t.Run(string(tc.reason), func(t *testing.T) {
			reg := prometheus.NewRegistry()
			m := libmetrics.NewLibraryMetricsWithRegisterer(reg)
			d := &fakeDrainer{drained: []domain.AutocacheDemand{{MALID: "7", Episode: 3, Reason: tc.reason}}}
			p := &fakePresence{present: map[string]bool{}}
			e := &fakeEnqueuer{active: map[string]bool{}}
			s := &fakeSearcher{releases: winningRelease()}
			c := &fakeConfig{cfg: enabledCfg()}

			pl := newPlannerForTest(d, p, e, s, c, m)
			pl.runOnce(context.Background())

			if got := promValue(t, m.GetDownloadsTotalForTest(tc.wantTrigger, "enqueued")); got != 1 {
				t.Fatalf("reason %q: trigger %q enqueued count = %v, want 1", tc.reason, tc.wantTrigger, got)
			}
		})
	}
}

// TestPlannerGcBackoffEvictsStale is the WR-03 regression: gcBackoff drops every
// lastSearched entry older than searchBackoff (bounding the map) while keeping
// fresh entries (so the no-release backoff still suppresses a re-search). Pre-fix
// the map grew without bound — entries were never deleted.
func TestPlannerGcBackoffEvictsStale(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := libmetrics.NewLibraryMetricsWithRegisterer(reg)
	pl := newPlannerForTest(&fakeDrainer{}, &fakePresence{present: map[string]bool{}}, &fakeEnqueuer{active: map[string]bool{}}, &fakeSearcher{}, &fakeConfig{cfg: enabledCfg()}, m)

	// Seed a stale entry (older than searchBackoff) and a fresh one.
	pl.lastSearched["stale:1"] = time.Now().Add(-2 * searchBackoff)
	pl.lastSearched["fresh:2"] = time.Now()

	pl.gcBackoff()

	if _, ok := pl.lastSearched["stale:1"]; ok {
		t.Fatal("gcBackoff must evict an entry older than searchBackoff (WR-03 unbounded-growth regression)")
	}
	if _, ok := pl.lastSearched["fresh:2"]; !ok {
		t.Fatal("gcBackoff must keep a fresh entry so the no-release backoff still applies")
	}
}

// TestPlannerForgetSearchedOnPresent asserts that when a demand is satisfied
// (present → deleted), its backoff entry is dropped eagerly (WR-03) so the map
// doesn't retain a dead key until the next gcBackoff sweep.
func TestPlannerForgetSearchedOnPresent(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := libmetrics.NewLibraryMetricsWithRegisterer(reg)
	d := &fakeDrainer{drained: []domain.AutocacheDemand{{MALID: "9", Episode: 4, Reason: domain.DemandReasonOngoing}}}
	p := &fakePresence{present: map[string]bool{"9:4": true}}
	e := &fakeEnqueuer{active: map[string]bool{}}
	s := &fakeSearcher{releases: winningRelease()}
	c := &fakeConfig{cfg: enabledCfg()}

	pl := newPlannerForTest(d, p, e, s, c, m)
	// Pre-seed a backoff entry as if a prior sweep had searched it.
	pl.lastSearched["9:4"] = time.Now()

	pl.runOnce(context.Background())

	if _, ok := pl.lastSearched["9:4"]; ok {
		t.Fatal("present-delete must forget the (mal:ep) backoff entry (WR-03 eager-eviction)")
	}
}
