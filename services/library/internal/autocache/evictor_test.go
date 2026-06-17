package autocache

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/library/internal/domain"
	libmetrics "github.com/ILITA-hub/animeenigma/services/library/internal/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

// --- handwritten fakes for the Evictor seams (no testify/mock, no live Postgres/MinIO) ---

// fakeAccountant is an in-memory poolAccountant. sumBytes is returned by SumPoolBytes;
// candidates is the scripted Stale candidate slice ListStaleEvictionCandidates returns
// (already in the locked 4-tier order). deleted records each DeleteByID(id). Errors can
// be injected per-call.
type fakeAccountant struct {
	sumBytes   int64
	sumErr     error
	candidates []domain.Episode
	candErr    error
	deleted    []string
	deleteErr  map[string]error // id → error (e.g. row-delete fails)
}

func (f *fakeAccountant) SumPoolBytes(_ context.Context) (int64, error) {
	return f.sumBytes, f.sumErr
}

func (f *fakeAccountant) ListStaleEvictionCandidates(_ context.Context, _ *domain.AutocacheConfig, _ time.Time) ([]domain.Episode, error) {
	if f.candErr != nil {
		return nil, f.candErr
	}
	return f.candidates, nil
}

func (f *fakeAccountant) DeleteByID(_ context.Context, id string) error {
	if f.deleteErr != nil {
		if err := f.deleteErr[id]; err != nil {
			return err
		}
	}
	f.deleted = append(f.deleted, id)
	return nil
}

// fakeDeleter is an in-memory objectDeleter. prefixes records each DeletePrefix(prefix);
// failPrefix maps a prefix → error so a test can fail the object-delete of one candidate.
type fakeDeleter struct {
	prefixes   []string
	failPrefix map[string]error
}

func (f *fakeDeleter) DeletePrefix(_ context.Context, prefix string) error {
	if f.failPrefix != nil {
		if err := f.failPrefix[prefix]; err != nil {
			return err // record nothing — the object delete did not complete
		}
	}
	f.prefixes = append(f.prefixes, prefix)
	return nil
}

func ptrI64(n int64) *int64          { return &n }
func ptrTime(t time.Time) *time.Time { return &t }

func evictCfg() domain.AutocacheConfig {
	return domain.AutocacheConfig{
		Enabled:               true,
		BudgetBytes:           1000,
		AutoFreshDownloadDays: 10,
		AutoFreshFetchDays:    3,
		AdminFreshDays:        30,
		SweepIntervalMin:      20,
	}
}

func newEvictorForTest(c *fakeConfig, a *fakeAccountant, d *fakeDeleter, m *libmetrics.LibraryMetrics) *Evictor {
	return NewEvictor(c, a, d, m, nil)
}

// staleEp builds a Stale autocache episode of the given size + id/path.
func staleEp(id, path string, size int64, source domain.EpisodeSource) domain.Episode {
	return domain.Episode{ID: id, MinioPath: path, SizeBytes: ptrI64(size), Source: source}
}

// --- Classify ---

func TestClassifyAutocacheFreshByDownload(t *testing.T) {
	cfg := evictCfg()
	now := time.Now()
	// downloaded 2 days ago, never fetched → within the 10d download window → Fresh.
	ep := domain.Episode{
		Source:       domain.EpisodeSourceAutocache,
		DownloadedAt: ptrTime(now.AddDate(0, 0, -2)),
		LastFetchAt:  nil,
	}
	if got := Classify(ep, &cfg, now); got != FreshnessFresh {
		t.Fatalf("recently-downloaded autocache row should be Fresh, got %q", got)
	}
}

func TestClassifyAutocacheFreshByFetch(t *testing.T) {
	cfg := evictCfg()
	now := time.Now()
	// downloaded long ago (out of 10d), but fetched 1 day ago (within 3d) → Fresh.
	ep := domain.Episode{
		Source:       domain.EpisodeSourceAutocache,
		DownloadedAt: ptrTime(now.AddDate(0, 0, -100)),
		LastFetchAt:  ptrTime(now.AddDate(0, 0, -1)),
	}
	if got := Classify(ep, &cfg, now); got != FreshnessFresh {
		t.Fatalf("recently-fetched autocache row should be Fresh, got %q", got)
	}
}

func TestClassifyAutocacheStaleOutOfBothWindows(t *testing.T) {
	cfg := evictCfg()
	now := time.Now()
	// downloaded 11 days ago (>10d), fetched 5 days ago (>3d) → Stale.
	ep := domain.Episode{
		Source:       domain.EpisodeSourceAutocache,
		DownloadedAt: ptrTime(now.AddDate(0, 0, -11)),
		LastFetchAt:  ptrTime(now.AddDate(0, 0, -5)),
	}
	if got := Classify(ep, &cfg, now); got != FreshnessStale {
		t.Fatalf("out-of-window autocache row should be Stale, got %q", got)
	}
}

func TestClassifyNullDownloadedClassifiesByFetch(t *testing.T) {
	cfg := evictCfg()
	now := time.Now()
	// NULL downloaded_at contributes nothing to rule 1; fetched 1 day ago → Fresh by rule 2.
	ep := domain.Episode{
		Source:       domain.EpisodeSourceAutocache,
		DownloadedAt: nil,
		LastFetchAt:  ptrTime(now.AddDate(0, 0, -1)),
	}
	if got := Classify(ep, &cfg, now); got != FreshnessFresh {
		t.Fatalf("NULL downloaded_at + recent fetch should be Fresh, got %q", got)
	}
	// NULL downloaded_at + old fetch → Stale.
	ep.LastFetchAt = ptrTime(now.AddDate(0, 0, -10))
	if got := Classify(ep, &cfg, now); got != FreshnessStale {
		t.Fatalf("NULL downloaded_at + old fetch should be Stale, got %q", got)
	}
}

func TestClassifyNullLastFetchNeverFetched(t *testing.T) {
	cfg := evictCfg()
	now := time.Now()
	// NULL last_fetch_at = never fetched; downloaded 11 days ago (>10d) → Stale.
	ep := domain.Episode{
		Source:       domain.EpisodeSourceAutocache,
		DownloadedAt: ptrTime(now.AddDate(0, 0, -11)),
		LastFetchAt:  nil,
	}
	if got := Classify(ep, &cfg, now); got != FreshnessStale {
		t.Fatalf("never-fetched out-of-window autocache row should be Stale, got %q", got)
	}
	// both NULL → Stale (very old + never fetched).
	ep.DownloadedAt = nil
	if got := Classify(ep, &cfg, now); got != FreshnessStale {
		t.Fatalf("both-NULL row should be Stale, got %q", got)
	}
}

func TestClassifyAdminWindow(t *testing.T) {
	cfg := evictCfg()
	now := time.Now()
	// admin uses the 30d window for BOTH download + fetch.
	// downloaded 20 days ago → within 30d → Fresh.
	fresh := domain.Episode{
		Source:       domain.EpisodeSourceAdmin,
		DownloadedAt: ptrTime(now.AddDate(0, 0, -20)),
		LastFetchAt:  nil,
	}
	if got := Classify(fresh, &cfg, now); got != FreshnessFresh {
		t.Fatalf("admin row within 30d should be Fresh, got %q", got)
	}
	// downloaded 40 days ago, fetched 35 days ago → both out of 30d → Stale.
	stale := domain.Episode{
		Source:       domain.EpisodeSourceAdmin,
		DownloadedAt: ptrTime(now.AddDate(0, 0, -40)),
		LastFetchAt:  ptrTime(now.AddDate(0, 0, -35)),
	}
	if got := Classify(stale, &cfg, now); got != FreshnessStale {
		t.Fatalf("admin row out of 30d should be Stale, got %q", got)
	}
	// admin fetched 10 days ago → within 30d (uses admin window, not the 3d auto window) → Fresh.
	adminFetchFresh := domain.Episode{
		Source:       domain.EpisodeSourceAdmin,
		DownloadedAt: ptrTime(now.AddDate(0, 0, -100)),
		LastFetchAt:  ptrTime(now.AddDate(0, 0, -10)),
	}
	if got := Classify(adminFetchFresh, &cfg, now); got != FreshnessFresh {
		t.Fatalf("admin row fetched within 30d should be Fresh, got %q", got)
	}
}

// --- EnsureRoom ---

func TestEnsureRoomFitsNoEvict(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := libmetrics.NewLibraryMetricsWithRegisterer(reg)
	c := &fakeConfig{cfg: evictCfg()} // budget 1000
	a := &fakeAccountant{sumBytes: 500, candidates: []domain.Episode{staleEp("x", "aeProvider/x/", 400, domain.EpisodeSourceAutocache)}}
	d := &fakeDeleter{}

	ev := newEvictorForTest(c, a, d, m)
	admitted, err := ev.EnsureRoom(context.Background(), 400) // 500+400=900 <= 1000
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !admitted {
		t.Fatalf("expected admitted=true when estimate fits")
	}
	if len(a.deleted) != 0 || len(d.prefixes) != 0 {
		t.Fatalf("no eviction expected when it fits; deleted=%v prefixes=%v", a.deleted, d.prefixes)
	}
}

func TestEnsureRoomEvictsUntilFitInTierOrder(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := libmetrics.NewLibraryMetricsWithRegisterer(reg)
	c := &fakeConfig{cfg: evictCfg()} // budget 1000
	// used 900, incoming 300 → need to free 200. Candidates in locked 4-tier order:
	// first autocache (size 150), then admin (size 150). Evicting both frees 300 → fits.
	a := &fakeAccountant{
		sumBytes: 900,
		candidates: []domain.Episode{
			staleEp("auto1", "aeProvider/auto1/", 150, domain.EpisodeSourceAutocache),
			staleEp("admin1", "aeProvider/admin1/", 150, domain.EpisodeSourceAdmin),
		},
	}
	d := &fakeDeleter{}

	ev := newEvictorForTest(c, a, d, m)
	admitted, err := ev.EnsureRoom(context.Background(), 300)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !admitted {
		t.Fatalf("expected admitted=true after evicting enough")
	}
	// Both evicted, in candidate (tier) order — objects then row.
	wantPrefixes := []string{"aeProvider/auto1/", "aeProvider/admin1/"}
	if len(d.prefixes) != 2 || d.prefixes[0] != wantPrefixes[0] || d.prefixes[1] != wantPrefixes[1] {
		t.Fatalf("expected object-deletes in tier order %v, got %v", wantPrefixes, d.prefixes)
	}
	wantDeleted := []string{"auto1", "admin1"}
	if len(a.deleted) != 2 || a.deleted[0] != wantDeleted[0] || a.deleted[1] != wantDeleted[1] {
		t.Fatalf("expected row-deletes in tier order %v, got %v", wantDeleted, a.deleted)
	}
	// evicted_total{source} incremented once per source.
	if got := promValue(t, m.GetEvictedTotalForTest("autocache")); got != 1 {
		t.Fatalf("evicted_total{autocache} = %v, want 1", got)
	}
	if got := promValue(t, m.GetEvictedTotalForTest("admin")); got != 1 {
		t.Fatalf("evicted_total{admin} = %v, want 1", got)
	}
}

func TestEnsureRoomStopsEarlyOnceFit(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := libmetrics.NewLibraryMetricsWithRegisterer(reg)
	c := &fakeConfig{cfg: evictCfg()} // budget 1000
	// used 1050, incoming 0 → need to free 50. First candidate (size 100) alone fits;
	// the second must NOT be evicted (early break).
	a := &fakeAccountant{
		sumBytes: 1050,
		candidates: []domain.Episode{
			staleEp("first", "aeProvider/first/", 100, domain.EpisodeSourceAutocache),
			staleEp("second", "aeProvider/second/", 100, domain.EpisodeSourceAutocache),
		},
	}
	d := &fakeDeleter{}

	ev := newEvictorForTest(c, a, d, m)
	admitted, err := ev.EnsureRoom(context.Background(), 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !admitted {
		t.Fatalf("expected admitted=true after one eviction")
	}
	if len(a.deleted) != 1 || a.deleted[0] != "first" {
		t.Fatalf("expected only the first candidate evicted, got %v", a.deleted)
	}
}

func TestEnsureRoomQueueExhaustedRejects(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := libmetrics.NewLibraryMetricsWithRegisterer(reg)
	c := &fakeConfig{cfg: evictCfg()} // budget 1000
	// used 2000, incoming 100. Draining all Stale (total 300) still leaves 1800 > 1000.
	a := &fakeAccountant{
		sumBytes: 2000,
		candidates: []domain.Episode{
			staleEp("a", "aeProvider/a/", 150, domain.EpisodeSourceAutocache),
			staleEp("b", "aeProvider/b/", 150, domain.EpisodeSourceAutocache),
		},
	}
	d := &fakeDeleter{}

	ev := newEvictorForTest(c, a, d, m)
	admitted, err := ev.EnsureRoom(context.Background(), 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if admitted {
		t.Fatalf("expected admitted=false when the Stale queue is exhausted and still short")
	}
	// All candidates were drained in the attempt.
	if len(a.deleted) != 2 {
		t.Fatalf("expected all candidates drained, got %v", a.deleted)
	}
}

func TestEnsureRoomObjectDeleteFailSkipsRow(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := libmetrics.NewLibraryMetricsWithRegisterer(reg)
	c := &fakeConfig{cfg: evictCfg()} // budget 1000
	// used 1100, incoming 0 → need to free 100. First candidate's object-delete FAILS
	// (so its row must NOT be deleted, byte not reclaimed); the second candidate
	// succeeds and frees enough.
	a := &fakeAccountant{
		sumBytes: 1100,
		candidates: []domain.Episode{
			staleEp("bad", "aeProvider/bad/", 100, domain.EpisodeSourceAutocache),
			staleEp("good", "aeProvider/good/", 100, domain.EpisodeSourceAutocache),
		},
	}
	d := &fakeDeleter{failPrefix: map[string]error{"aeProvider/bad/": errors.New("minio down")}}

	ev := newEvictorForTest(c, a, d, m)
	admitted, err := ev.EnsureRoom(context.Background(), 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !admitted {
		t.Fatalf("expected admitted=true after the second candidate freed room")
	}
	// The failed candidate's row was NOT deleted (no orphaned serving pointer).
	for _, id := range a.deleted {
		if id == "bad" {
			t.Fatalf("object-delete-fail candidate 'bad' must NOT have its row deleted")
		}
	}
	if len(a.deleted) != 1 || a.deleted[0] != "good" {
		t.Fatalf("expected only 'good' row deleted, got %v", a.deleted)
	}
	// evicted_total counted ONLY the completed eviction (the 'good' autocache row).
	if got := promValue(t, m.GetEvictedTotalForTest("autocache")); got != 1 {
		t.Fatalf("evicted_total{autocache} = %v, want 1 (only the completed eviction)", got)
	}
}

func TestEnsureRoomRowDeleteFailSkipsByteReclaim(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := libmetrics.NewLibraryMetricsWithRegisterer(reg)
	c := &fakeConfig{cfg: evictCfg()} // budget 1000
	// used 1100, incoming 0 → need to free 100. First candidate's OBJECT delete
	// succeeds but its ROW delete fails (evictOne returns the error) → byte NOT
	// reclaimed, IncEvictedTotal NOT called; the second candidate completes.
	a := &fakeAccountant{
		sumBytes:  1100,
		deleteErr: map[string]error{"bad": errors.New("db down")},
		candidates: []domain.Episode{
			staleEp("bad", "aeProvider/bad/", 100, domain.EpisodeSourceAutocache),
			staleEp("good", "aeProvider/good/", 100, domain.EpisodeSourceAutocache),
		},
	}
	d := &fakeDeleter{}

	ev := newEvictorForTest(c, a, d, m)
	admitted, err := ev.EnsureRoom(context.Background(), 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !admitted {
		t.Fatalf("expected admitted=true after the second candidate freed room")
	}
	// Object-deletes attempted for both; only the 'good' row recorded as deleted.
	if len(a.deleted) != 1 || a.deleted[0] != "good" {
		t.Fatalf("expected only 'good' row recorded deleted, got %v", a.deleted)
	}
	if got := promValue(t, m.GetEvictedTotalForTest("autocache")); got != 1 {
		t.Fatalf("evicted_total{autocache} = %v, want 1 (row-delete-fail not counted)", got)
	}
}
