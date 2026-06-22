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
	drained       []domain.AutocacheDemand
	deletes       [][2]any // {malID, episode}
	expireCutoffs []time.Time
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

// expireCutoffs records each DeleteExpired cutoff so tests can assert the
// expiry safety-valve runs (audit #20).
func (f *fakeDrainer) DeleteExpired(_ context.Context, cutoff time.Time) (int64, error) {
	f.expireCutoffs = append(f.expireCutoffs, cutoff)
	return 0, nil
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

// fakeEvictor is a scriptable budgetEvictor seam fake: admitted/err are returned
// verbatim and calls records each EnsureRoom(estBytes).
type fakeEvictor struct {
	admitted bool
	err      error
	calls    []int64 // estBytes per EnsureRoom call
}

func (f *fakeEvictor) EnsureRoom(_ context.Context, estBytes int64) (bool, error) {
	f.calls = append(f.calls, estBytes)
	return f.admitted, f.err
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
	// nil evictor → the pre-admit gate is skipped, preserving the existing cases'
	// assertions (these tests pre-date the Phase-10 budget gate).
	return NewPlanner(d, p, e, s, c, nil, m, nil)
}

// newPlannerWithEvictor wires a scriptable budgetEvictor so the Phase-10 pre-admit
// gate cases can drive admit/reject/error paths.
func newPlannerWithEvictor(d *fakeDrainer, p *fakePresence, e *fakeEnqueuer, s *fakeSearcher, c *fakeConfig, ev budgetEvictor, m *libmetrics.LibraryMetrics) *Planner {
	return NewPlanner(d, p, e, s, c, ev, m, nil)
}

func winningRelease() []domain.Release {
	return []domain.Release{{Title: "Anime - 05 [1080p]", Magnet: "magnet:?xt=urn:btih:abc", Uploader: "Ohys-Raws", Quality: "1080p", SizeBytes: 123, Seeders: 9}}
}

func TestPlannerDisabledNoOp(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := libmetrics.NewLibraryMetricsWithRegisterer(reg)
	d := &fakeDrainer{drained: []domain.AutocacheDemand{{MALID: "1", Episode: 5, Reason: domain.DemandReasonOngoing, Titles: "Anime"}}}
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
	d := &fakeDrainer{drained: []domain.AutocacheDemand{{MALID: "1", Episode: 5, Reason: domain.DemandReasonBackfill, Titles: "Anime"}}}
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
	d := &fakeDrainer{drained: []domain.AutocacheDemand{{MALID: "1", Episode: 5, Reason: domain.DemandReasonNextEp, Titles: "Anime"}}}
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
	d := &fakeDrainer{drained: []domain.AutocacheDemand{{MALID: "42", Episode: 5, Reason: domain.DemandReasonNextEp, Titles: "Anime"}}}
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
	d := &fakeDrainer{drained: []domain.AutocacheDemand{{MALID: "1", Episode: 5, Reason: domain.DemandReasonBackfill, Titles: "Anime"}}}
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
			d := &fakeDrainer{drained: []domain.AutocacheDemand{{MALID: "7", Episode: 5, Reason: tc.reason, Titles: "Anime"}}}
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
	d := &fakeDrainer{drained: []domain.AutocacheDemand{{MALID: "9", Episode: 4, Reason: domain.DemandReasonOngoing, Titles: "Anime"}}}
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

// TestPlannerBudgetAdmittedEnqueues: when the Evictor admits the incoming download,
// the Planner proceeds to the existing Create path (EVICT-05 gate passes). It also
// asserts the estimate passed to EnsureRoom is the selected release size.
func TestPlannerBudgetAdmittedEnqueues(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := libmetrics.NewLibraryMetricsWithRegisterer(reg)
	d := &fakeDrainer{drained: []domain.AutocacheDemand{{MALID: "42", Episode: 5, Reason: domain.DemandReasonNextEp, Titles: "Anime"}}}
	p := &fakePresence{present: map[string]bool{}}
	e := &fakeEnqueuer{active: map[string]bool{}}
	s := &fakeSearcher{releases: winningRelease()} // SizeBytes: 123
	c := &fakeConfig{cfg: enabledCfg()}
	ev := &fakeEvictor{admitted: true}

	pl := newPlannerWithEvictor(d, p, e, s, c, ev, m)
	pl.runOnce(context.Background())

	if len(e.created) != 1 {
		t.Fatalf("budget-admitted must enqueue exactly one job, got %d", len(e.created))
	}
	if len(ev.calls) != 1 {
		t.Fatalf("EnsureRoom must be called once, got %d", len(ev.calls))
	}
	if ev.calls[0] != 123 {
		t.Fatalf("EnsureRoom estBytes = %d, want 123 (selected release size)", ev.calls[0])
	}
	if got := promValue(t, m.GetDownloadsTotalForTest("B", "enqueued")); got != 1 {
		t.Fatalf("enqueued count = %v, want 1", got)
	}
}

// TestPlannerBudgetRejectedLeavesDemandAndBacksOff is the EVICT-04 reject-path
// regression: a budget-rejected demand enqueues NOTHING, increments
// rejected_total{budget_full} + downloads_total{rejected}, LEAVES the demand row,
// and arms the searchBackoff (T-10-08 hot-loop guard).
func TestPlannerBudgetRejectedLeavesDemandAndBacksOff(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := libmetrics.NewLibraryMetricsWithRegisterer(reg)
	d := &fakeDrainer{drained: []domain.AutocacheDemand{{MALID: "42", Episode: 5, Reason: domain.DemandReasonNextEp, Titles: "Anime"}}}
	p := &fakePresence{present: map[string]bool{}}
	e := &fakeEnqueuer{active: map[string]bool{}}
	s := &fakeSearcher{releases: winningRelease()}
	c := &fakeConfig{cfg: enabledCfg()}
	ev := &fakeEvictor{admitted: false} // pool can't fit

	pl := newPlannerWithEvictor(d, p, e, s, c, ev, m)
	pl.runOnce(context.Background())

	if len(e.created) != 0 {
		t.Fatalf("budget-rejected must enqueue nothing, got %d", len(e.created))
	}
	if len(d.deletes) != 0 {
		t.Fatalf("budget-rejected must LEAVE the demand row, got %d deletes", len(d.deletes))
	}
	if got := promValue(t, m.GetRejectedTotalForTest("budget_full")); got != 1 {
		t.Fatalf("rejected_total{budget_full} = %v, want 1", got)
	}
	if got := promValue(t, m.GetDownloadsTotalForTest("B", "rejected")); got != 1 {
		t.Fatalf("downloads_total{B,rejected} = %v, want 1", got)
	}
	if !pl.inBackoff(demandKey("42", 5)) {
		t.Fatal("budget-rejected must arm the searchBackoff (T-10-08 hot-loop guard)")
	}
}

// TestPlannerBudgetErrorFailsOpen: a budget-read error must NOT drop the download —
// the Planner fails open and proceeds to enqueue (mirrors disk-guard fail-open).
func TestPlannerBudgetErrorFailsOpen(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := libmetrics.NewLibraryMetricsWithRegisterer(reg)
	d := &fakeDrainer{drained: []domain.AutocacheDemand{{MALID: "42", Episode: 5, Reason: domain.DemandReasonNextEp, Titles: "Anime"}}}
	p := &fakePresence{present: map[string]bool{}}
	e := &fakeEnqueuer{active: map[string]bool{}}
	s := &fakeSearcher{releases: winningRelease()}
	c := &fakeConfig{cfg: enabledCfg()}
	ev := &fakeEvictor{admitted: false, err: liberrors.Internal("budget read blip")}

	pl := newPlannerWithEvictor(d, p, e, s, c, ev, m)
	pl.runOnce(context.Background())

	if len(e.created) != 1 {
		t.Fatalf("budget-error must fail open and enqueue, got %d jobs", len(e.created))
	}
	if got := promValue(t, m.GetRejectedTotalForTest("budget_full")); got != 0 {
		t.Fatalf("budget-error must NOT increment rejected_total, got %v", got)
	}
	if got := promValue(t, m.GetDownloadsTotalForTest("B", "enqueued")); got != 1 {
		t.Fatalf("enqueued count = %v, want 1", got)
	}
}

// TestPlannerBudgetFallbackEstimate: when a selected release reports SizeBytes <= 0,
// the Planner passes the AvgRawEpSize const to EnsureRoom (not 0).
func TestPlannerBudgetFallbackEstimate(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := libmetrics.NewLibraryMetricsWithRegisterer(reg)
	d := &fakeDrainer{drained: []domain.AutocacheDemand{{MALID: "42", Episode: 5, Reason: domain.DemandReasonNextEp, Titles: "Anime"}}}
	p := &fakePresence{present: map[string]bool{}}
	e := &fakeEnqueuer{active: map[string]bool{}}
	// A qualifying release (allowlisted RAW uploader) with no reported size.
	s := &fakeSearcher{releases: []domain.Release{{Title: "Anime - 05 [1080p]", Magnet: "magnet:?xt=urn:btih:abc", Uploader: "Ohys-Raws", Quality: "1080p", SizeBytes: 0, Seeders: 9}}}
	c := &fakeConfig{cfg: enabledCfg()}
	ev := &fakeEvictor{admitted: true}

	pl := newPlannerWithEvictor(d, p, e, s, c, ev, m)
	pl.runOnce(context.Background())

	if len(ev.calls) != 1 {
		t.Fatalf("EnsureRoom must be called once, got %d", len(ev.calls))
	}
	if ev.calls[0] != AvgRawEpSize {
		t.Fatalf("EnsureRoom estBytes = %d, want AvgRawEpSize=%d (fallback)", ev.calls[0], AvgRawEpSize)
	}
}
