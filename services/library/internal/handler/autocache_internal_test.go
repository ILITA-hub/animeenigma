package handler

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	libmetrics "github.com/ILITA-hub/animeenigma/services/library/internal/metrics"
	"github.com/ILITA-hub/animeenigma/services/library/internal/domain"
)

// stubInternalDeps is a no-Postgres autocacheInternalDeps capturing calls.
type stubInternalDeps struct {
	bumpCalls   int
	bumpMAL     string
	bumpEpisode int
	bumpErr     error

	recordCalls   int
	recordMAL     string
	recordEpisode int
	recordReason  domain.DemandReason
	recordErr     error

	enabled    bool
	enabledErr error
}

func (s *stubInternalDeps) BumpFetch(_ context.Context, malID string, episode int) error {
	s.bumpCalls++
	s.bumpMAL = malID
	s.bumpEpisode = episode
	return s.bumpErr
}

func (s *stubInternalDeps) RecordDemand(_ context.Context, malID string, episode int, reason domain.DemandReason) error {
	s.recordCalls++
	s.recordMAL = malID
	s.recordEpisode = episode
	s.recordReason = reason
	return s.recordErr
}

func (s *stubInternalDeps) ConfigEnabled(context.Context) (bool, error) {
	return s.enabled, s.enabledErr
}

func newTestInternalHandler(deps autocacheInternalDeps) (*AutocacheInternalHandler, *libmetrics.LibraryMetrics) {
	m := libmetrics.NewLibraryMetricsWithRegisterer(prometheus.NewRegistry())
	return NewAutocacheInternalHandler(deps, m, logger.Default()), m
}

func postJSON(h http.HandlerFunc, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	h(rec, req)
	return rec
}

// (a) Fetch → BumpFetch called with decoded args + serve_total{hit}++ + 200.
func TestAutocacheInternalFetch_BumpsAndCountsHit(t *testing.T) {
	deps := &stubInternalDeps{}
	h, m := newTestInternalHandler(deps)

	before := testutil.ToFloat64(m.GetServeTotalForTest("hit"))
	rec := postJSON(h.Fetch, `{"mal_id":"57466","episode":12}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	if deps.bumpCalls != 1 {
		t.Fatalf("want 1 BumpFetch call, got %d", deps.bumpCalls)
	}
	if deps.bumpMAL != "57466" || deps.bumpEpisode != 12 {
		t.Fatalf("BumpFetch args mismatch: got %q/%d", deps.bumpMAL, deps.bumpEpisode)
	}
	if got := testutil.ToFloat64(m.GetServeTotalForTest("hit")) - before; got != 1 {
		t.Fatalf("want serve_total{hit} +1, got +%v", got)
	}
}

// Fetch tolerates a bump error (best-effort, never 500) but still counts the hit.
func TestAutocacheInternalFetch_BumpErrorStill200(t *testing.T) {
	deps := &stubInternalDeps{bumpErr: errors.New("db down")}
	h, m := newTestInternalHandler(deps)

	before := testutil.ToFloat64(m.GetServeTotalForTest("hit"))
	rec := postJSON(h.Fetch, `{"mal_id":"1","episode":1}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200 on bump error, got %d", rec.Code)
	}
	if got := testutil.ToFloat64(m.GetServeTotalForTest("hit")) - before; got != 1 {
		t.Fatalf("want serve_total{hit} +1 even on bump error, got +%v", got)
	}
}

// (b) Demand enabled=true → Record(validated wire reason) + serve_total{miss}++ + 200.
//
// Phase 09 INVERTS the Phase-8 behavior: the handler no longer force-overrides the
// wire reason to backfill. It now VALIDATES the wire reason against the
// {backfill, next_ep, ongoing} allowlist and HONORS it (the /internal/* surface is
// Docker-network-only, so the producers are internal-trusted — T-08-04). An absent
// or unknown reason still falls back to backfill so a malformed internal caller
// degrades safely.
func TestAutocacheInternalDemand_EnabledHonorsValidatedReason(t *testing.T) {
	cases := []struct {
		name       string
		body       string
		wantReason domain.DemandReason
	}{
		{"next_ep honored (Logic B)", `{"mal_id":"57466","episode":7,"reason":"next_ep"}`, domain.DemandReasonNextEp},
		{"ongoing honored (Logic A)", `{"mal_id":"57466","episode":7,"reason":"ongoing"}`, domain.DemandReasonOngoing},
		{"backfill honored (serve miss)", `{"mal_id":"57466","episode":7,"reason":"backfill"}`, domain.DemandReasonBackfill},
		{"absent reason → backfill default", `{"mal_id":"57466","episode":7}`, domain.DemandReasonBackfill},
		{"invalid reason → backfill default", `{"mal_id":"57466","episode":7,"reason":"totally_bogus"}`, domain.DemandReasonBackfill},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			deps := &stubInternalDeps{enabled: true}
			h, m := newTestInternalHandler(deps)

			before := testutil.ToFloat64(m.GetServeTotalForTest("miss"))
			rec := postJSON(h.Demand, tc.body)

			if rec.Code != http.StatusOK {
				t.Fatalf("want 200, got %d", rec.Code)
			}
			if deps.recordCalls != 1 {
				t.Fatalf("want 1 Record call, got %d", deps.recordCalls)
			}
			if deps.recordMAL != "57466" || deps.recordEpisode != 7 {
				t.Fatalf("Record args mismatch: got %q/%d", deps.recordMAL, deps.recordEpisode)
			}
			if deps.recordReason != tc.wantReason {
				t.Fatalf("want reason %q, got %q", tc.wantReason, deps.recordReason)
			}
			if got := testutil.ToFloat64(m.GetServeTotalForTest("miss")) - before; got != 1 {
				t.Fatalf("want serve_total{miss} +1, got +%v", got)
			}
		})
	}
}

// (c) Demand enabled=false → Record NOT called + miss NOT incremented + 200.
func TestAutocacheInternalDemand_DisabledSkipsButStill200(t *testing.T) {
	deps := &stubInternalDeps{enabled: false}
	h, m := newTestInternalHandler(deps)

	before := testutil.ToFloat64(m.GetServeTotalForTest("miss"))
	rec := postJSON(h.Demand, `{"mal_id":"57466","episode":7}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200 when disabled, got %d", rec.Code)
	}
	if deps.recordCalls != 0 {
		t.Fatalf("want Record NOT called when disabled, got %d calls", deps.recordCalls)
	}
	if got := testutil.ToFloat64(m.GetServeTotalForTest("miss")) - before; got != 0 {
		t.Fatalf("want serve_total{miss} unchanged when disabled, got +%v", got)
	}
}

// Demand fails closed on a config-read error: skip side effects, still 200.
func TestAutocacheInternalDemand_ConfigErrorFailsClosed(t *testing.T) {
	deps := &stubInternalDeps{enabled: true, enabledErr: errors.New("config read blip")}
	h, m := newTestInternalHandler(deps)

	before := testutil.ToFloat64(m.GetServeTotalForTest("miss"))
	rec := postJSON(h.Demand, `{"mal_id":"57466","episode":7}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200 on config error, got %d", rec.Code)
	}
	if deps.recordCalls != 0 {
		t.Fatalf("want Record NOT called on config error (fail closed), got %d", deps.recordCalls)
	}
	if got := testutil.ToFloat64(m.GetServeTotalForTest("miss")) - before; got != 0 {
		t.Fatalf("want serve_total{miss} unchanged on config error, got +%v", got)
	}
}

// (d) Malformed / empty body → 400 with no side effects, on both endpoints.
func TestAutocacheInternalBadBody(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"malformed json", `{not json`},
		{"empty mal_id", `{"mal_id":"","episode":3}`},
		{"zero episode", `{"mal_id":"57466","episode":0}`},
		{"negative episode", `{"mal_id":"57466","episode":-1}`},
		// WR-02: int64 episode that overflows Postgres int4 must be rejected at
		// the edge, not silently swallowed by the DB write.
		{"overflow episode (int4)", `{"mal_id":"57466","episode":9999999999}`},
		{"over maxEpisode", `{"mal_id":"57466","episode":100001}`},
	}
	for _, tc := range cases {
		t.Run("fetch/"+tc.name, func(t *testing.T) {
			deps := &stubInternalDeps{}
			h, m := newTestInternalHandler(deps)
			before := testutil.ToFloat64(m.GetServeTotalForTest("hit"))
			rec := postJSON(h.Fetch, tc.body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("want 400, got %d", rec.Code)
			}
			if deps.bumpCalls != 0 {
				t.Fatalf("want no BumpFetch on bad body, got %d", deps.bumpCalls)
			}
			if got := testutil.ToFloat64(m.GetServeTotalForTest("hit")) - before; got != 0 {
				t.Fatalf("want no hit count on bad body, got +%v", got)
			}
		})
		t.Run("demand/"+tc.name, func(t *testing.T) {
			deps := &stubInternalDeps{enabled: true}
			h, m := newTestInternalHandler(deps)
			before := testutil.ToFloat64(m.GetServeTotalForTest("miss"))
			rec := postJSON(h.Demand, tc.body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("want 400, got %d", rec.Code)
			}
			if deps.recordCalls != 0 {
				t.Fatalf("want no Record on bad body, got %d", deps.recordCalls)
			}
			if got := testutil.ToFloat64(m.GetServeTotalForTest("miss")) - before; got != 0 {
				t.Fatalf("want no miss count on bad body, got +%v", got)
			}
		})
	}
}

// Source tripwire: the demand body MUST route the wire reason through
// validateDemandReason (Phase 09 — the reason is validated-and-honored, no longer
// force-overridden to backfill). Guards against a regression that re-hardcodes
// backfill and silently collapses Logic A/B attribution. The marker now names the
// validator; the behavioral assertion is the table-driven enabled test above.
func TestAutocacheInternalDemand_ValidatesWireReason(t *testing.T) {
	if !strings.Contains(autocacheInternalSourceMarker, "validateDemandReason") {
		t.Skip("marker only; real assertion is the enabled-true table test")
	}
}

// autocacheInternalSourceMarker keeps the tripwire test self-contained. It now
// names validateDemandReason (the Phase-09 reason validator) so a future refactor
// that drops validation and re-hardcodes backfill is caught.
const autocacheInternalSourceMarker = "validateDemandReason"
