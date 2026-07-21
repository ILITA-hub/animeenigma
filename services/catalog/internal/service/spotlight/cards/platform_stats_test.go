package cards

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	spotlight "github.com/ILITA-hub/animeenigma/services/catalog/internal/service/spotlight"
)

// type alias so assertions read cleanly.
type spotlightPlatformStatsData = spotlight.PlatformStatsData

// fakePrometheus implements promQuerier with scripted return values.
// healthErr (if set) is returned by Health; queryErr (if set) is returned by
// every Query — used to simulate Prometheus being unreachable.
type fakePrometheus struct {
	healthAllUp bool
	healthPct   float64
	healthErr   error
	queryErr    error
}

func (f *fakePrometheus) Health(_ context.Context) (bool, float64, error) {
	return f.healthAllUp, f.healthPct, f.healthErr
}

func (f *fakePrometheus) Query(_ context.Context, _ string) (float64, error) {
	if f.queryErr != nil {
		return 0, f.queryErr
	}
	return 0, errors.New("no data")
}

// allNonZeroProm returns the same non-zero value for any Query and fixed
// health — used to guarantee tiles populate.
type allNonZeroProm struct {
	allUp bool
	pct   float64
	val   float64
}

func (p *allNonZeroProm) Health(_ context.Context) (bool, float64, error) {
	return p.allUp, p.pct, nil
}
func (p *allNonZeroProm) Query(_ context.Context, _ string) (float64, error) {
	return p.val, nil
}

// countingProm records how many times each method is called. Used to prove
// the cache-hit path never touches Prometheus.
type countingProm struct {
	queryCalls  int
	healthCalls int
}

func (p *countingProm) Health(_ context.Context) (bool, float64, error) {
	p.healthCalls++
	return true, 100, nil
}
func (p *countingProm) Query(_ context.Context, _ string) (float64, error) {
	p.queryCalls++
	return 1, nil
}

func newPlatformStatsResolverForTest(prom promQuerier) *PlatformStatsResolver {
	return NewPlatformStatsResolver(prom, &fakeCache{store: map[string][]byte{}}, testLogger())
}

func TestPlatformStats_HappyPath(t *testing.T) {
	prom := &allNonZeroProm{allUp: true, pct: 99.4, val: 12345}
	card, err := newPlatformStatsResolverForTest(prom).Resolve(context.Background(), nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if card == nil || card.Type != "platform_stats" {
		t.Fatalf("bad card: %+v", card)
	}
	data := card.Data.(spotlightPlatformStatsData)
	if !data.Hero.WorkingOK {
		t.Fatal("want WorkingOK=true")
	}
	if data.Hero.UptimePercent == nil || *data.Hero.UptimePercent != 99.4 {
		t.Fatalf("want uptime 99.4, got %v", data.Hero.UptimePercent)
	}
	if data.Hero.Tagline == "" || data.Hero.Service == "" || data.Hero.MVQ == "" {
		t.Fatalf("hero joke fields unpopulated: %+v", data.Hero)
	}
	if len(data.Tiles) != 4 {
		t.Fatalf("want 4 tiles, got %d", len(data.Tiles))
	}
	for i, tile := range data.Tiles {
		if tile.ID == "" {
			t.Fatalf("tile %d has no localization id: %+v", i, tile)
		}
	}
}

func TestPlatformStats_DailyStability(t *testing.T) {
	prom := &allNonZeroProm{allUp: true, pct: 50, val: 7}
	a, _ := newPlatformStatsResolverForTest(prom).Resolve(context.Background(), nil)
	b, _ := newPlatformStatsResolverForTest(prom).Resolve(context.Background(), nil)
	da := a.Data.(spotlightPlatformStatsData)
	db := b.Data.(spotlightPlatformStatsData)
	if da.Hero.Tagline != db.Hero.Tagline || da.Hero.Service != db.Hero.Service {
		t.Fatal("same day must yield identical hero picks")
	}
	if len(da.Tiles) != len(db.Tiles) {
		t.Fatal("same day must yield identical tile count")
	}
	for i := range da.Tiles {
		if da.Tiles[i].Window != db.Tiles[i].Window || da.Tiles[i].Label != db.Tiles[i].Label {
			t.Fatalf("tile %d differs across same-day calls", i)
		}
	}
}

func TestPlatformStats_PrometheusDownStillEligible(t *testing.T) {
	prom := &fakePrometheus{healthErr: errors.New("down"), queryErr: errors.New("down")}
	card, err := newPlatformStatsResolverForTest(prom).Resolve(context.Background(), nil)
	if err != nil {
		t.Fatalf("Resolve should not error: %v", err)
	}
	if card == nil {
		t.Fatal("card must remain eligible when Prometheus is down")
	}
	data := card.Data.(spotlightPlatformStatsData)
	if data.Hero.WorkingOK {
		t.Fatal("WorkingOK must be false when health errors")
	}
	if data.Hero.UptimePercent != nil {
		t.Fatal("UptimePercent must be nil when health errors")
	}
	if len(data.Tiles) != 0 {
		t.Fatalf("want 0 tiles when all queries fail, got %d", len(data.Tiles))
	}
	if data.Hero.Tagline == "" {
		t.Fatal("hero tagline must still render from the pool")
	}
}

func TestPlatformStats_FiltersZeroTiles(t *testing.T) {
	// Health ok, but every Query returns 0 -> all tiles filtered out.
	prom := &allNonZeroProm{allUp: true, pct: 100, val: 0}
	card, _ := newPlatformStatsResolverForTest(prom).Resolve(context.Background(), nil)
	data := card.Data.(spotlightPlatformStatsData)
	if len(data.Tiles) != 0 {
		t.Fatalf("zero-valued metrics must be filtered, got %d tiles", len(data.Tiles))
	}
}

func TestPlatformStats_CacheHitSkipsPrometheus(t *testing.T) {
	// A pre-populated cache entry for today's key must be returned verbatim
	// WITHOUT touching Prometheus (the once-per-day snapshot contract).
	pct := 42.0
	cached := spotlight.PlatformStatsData{
		Hero: spotlight.StatsHero{
			WorkingOK:     true,
			UptimePercent: &pct,
			UptimeQuip:    "CACHED",
			Service:       "catalog",
			Tagline:       "cached tagline",
			UXDelta:       "x",
			CDI:           "y",
			MVQ:           "z",
		},
		Tiles: []spotlight.StatsTile{{Label: "L", Value: 5, Window: "day", Format: "int"}},
	}
	key := "spotlight:stats:" + spotlight.DateKeyUTC(time.Now())
	b, err := json.Marshal(cached)
	if err != nil {
		t.Fatalf("marshal cached: %v", err)
	}
	fc := &fakeCache{store: map[string][]byte{key: b}}

	prom := &countingProm{}
	card, err := NewPlatformStatsResolver(prom, fc, testLogger()).Resolve(context.Background(), nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	data := card.Data.(spotlightPlatformStatsData)
	if data.Hero.UptimeQuip != "CACHED" || data.Hero.Tagline != "cached tagline" {
		t.Fatalf("expected cached payload, got %+v", data.Hero)
	}
	if prom.queryCalls != 0 || prom.healthCalls != 0 {
		t.Fatalf("Prometheus must not be called on cache hit: query=%d health=%d",
			prom.queryCalls, prom.healthCalls)
	}
}
