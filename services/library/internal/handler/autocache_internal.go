// Package handler — autocache_internal.go: the Docker-network-only serve-signal
// endpoints the catalog ae-resolution producer (Plan 03) calls.
//
//	POST /internal/library/autocache/fetch   HIT  → BumpFetch + serve_total{hit}
//	POST /internal/library/autocache/demand  MISS → DemandRepository.Record(backfill)
//	                                                + serve_total{miss}
//
// Both live OUTSIDE /api/library so the gateway never proxies them (same rule
// as recs' /internal/recs/recompute-hint + notifications' /internal/notifications).
// The library service owns the library_autocache_* metrics + the ledger writes,
// so the producer reports the resolution outcome here rather than emitting the
// counter itself.
//
// The master `enabled` switch (Phase 07 autocache_config) gates the demand
// side effects: when off, the demand endpoint still returns 200 but records
// nothing and counts no miss ("still serve, but skip demand recording" —
// CONTEXT). A config-read error fails closed (treated as disabled) so a config
// blip never floods the demand table.
package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	libmetrics "github.com/ILITA-hub/animeenigma/services/library/internal/metrics"
	"github.com/ILITA-hub/animeenigma/services/library/internal/domain"
)

// autocacheInternalDeps is the narrow surface the internal handler depends on —
// pulled out as an interface seam (mirrors recs' hintDeps + library's
// AutocacheConfigStore) so the httptest unit injects stubs without Postgres.
// Production wires NewGormAutocacheInternalDeps over the existing Phase-7/8
// repos.
type autocacheInternalDeps interface {
	// BumpFetch bumps the episode ledger (last_fetch_at=now, fetch_count++);
	// a no-op on an absent row (at-least-once acceptable — popularity, not money).
	BumpFetch(ctx context.Context, malID string, episode int) error
	// RecordDemand upserts a wanted (mal_id, episode) demand row. Phase 08
	// always passes domain.DemandReasonBackfill.
	RecordDemand(ctx context.Context, malID string, episode int, reason domain.DemandReason) error
	// ConfigEnabled reports the master autocache `enabled` switch.
	ConfigEnabled(ctx context.Context) (bool, error)
}

// AutocacheInternalHandler serves the two Docker-network-only serve-signal
// endpoints.
type AutocacheInternalHandler struct {
	deps    autocacheInternalDeps
	metrics *libmetrics.LibraryMetrics
	log     *logger.Logger
}

// NewAutocacheInternalHandler constructs the handler. Tests inject a stub
// autocacheInternalDeps; production uses NewGormAutocacheInternalDeps.
func NewAutocacheInternalHandler(deps autocacheInternalDeps, m *libmetrics.LibraryMetrics, log *logger.Logger) *AutocacheInternalHandler {
	return &AutocacheInternalHandler{deps: deps, metrics: m, log: log}
}

// autocacheSignalBody is the shared request body. demand also carries a
// "reason" field on the wire, but Phase 08 only writes backfill — any
// client-supplied reason is ignored/overridden server-side so a spoofed reason
// can never inject next_ep (T-08-05).
type autocacheSignalBody struct {
	MALID   string `json:"mal_id"`
	Episode int    `json:"episode"`
	Reason  string `json:"reason"`
}

// decodeSignal decodes + validates the shared body. Returns false (and writes a
// 400) when the body is malformed or mal_id/episode are missing/invalid.
func (h *AutocacheInternalHandler) decodeSignal(w http.ResponseWriter, r *http.Request) (autocacheSignalBody, bool) {
	var body autocacheSignalBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httputil.BadRequest(w, "invalid body")
		return autocacheSignalBody{}, false
	}
	if body.MALID == "" || body.Episode <= 0 {
		httputil.BadRequest(w, "mal_id and a positive episode are required")
		return autocacheSignalBody{}, false
	}
	return body, true
}

// Fetch handles POST /internal/library/autocache/fetch — the HIT path. It bumps
// the episode ledger (best-effort, never 500s on a bump error) and counts a
// serve_total{hit}. context.WithoutCancel so a caller disconnect can't cancel
// the bump mid-flight (resolution ≈ playback intent for ae).
func (h *AutocacheInternalHandler) Fetch(w http.ResponseWriter, r *http.Request) {
	body, ok := h.decodeSignal(w, r)
	if !ok {
		return
	}
	if err := h.deps.BumpFetch(context.WithoutCancel(r.Context()), body.MALID, body.Episode); err != nil {
		h.log.Warnw("autocache fetch bump failed (non-fatal)",
			"mal_id", body.MALID, "episode", body.Episode, "error", err)
	}
	h.metrics.IncServeTotal("hit")
	httputil.OK(w, map[string]bool{"ok": true})
}

// Demand handles POST /internal/library/autocache/demand — the MISS path. It
// reads the master `enabled` switch first; when enabled it records a backfill
// demand (best-effort) and counts a serve_total{miss}. When disabled — or when
// the config read errors (fail closed) — it skips BOTH the record and the miss
// counter but still returns 200, so the serve path never regresses on a config
// blip. The reason is forced to backfill regardless of client input.
func (h *AutocacheInternalHandler) Demand(w http.ResponseWriter, r *http.Request) {
	body, ok := h.decodeSignal(w, r)
	if !ok {
		return
	}
	ctx := context.WithoutCancel(r.Context())

	enabled, err := h.deps.ConfigEnabled(ctx)
	if err != nil {
		// Fail closed: a config-read blip must not flood the demand table.
		h.log.Warnw("autocache config read failed; skipping demand (fail closed)",
			"mal_id", body.MALID, "episode", body.Episode, "error", err)
		httputil.OK(w, map[string]bool{"ok": true})
		return
	}
	if !enabled {
		// Master switch off: still serve, but skip demand recording (CONTEXT).
		httputil.OK(w, map[string]bool{"ok": true})
		return
	}

	if err := h.deps.RecordDemand(ctx, body.MALID, body.Episode, domain.DemandReasonBackfill); err != nil {
		h.log.Warnw("autocache demand record failed (non-fatal)",
			"mal_id", body.MALID, "episode", body.Episode, "error", err)
	}
	h.metrics.IncServeTotal("miss")
	httputil.OK(w, map[string]bool{"ok": true})
}

// gormAutocacheInternalDeps is the production autocacheInternalDeps: it wires
// the existing Phase-7/8 repos (EpisodeRepository.BumpFetch,
// DemandRepository.Record, AutocacheConfigRepository.Get → cfg.Enabled).
type gormAutocacheInternalDeps struct {
	episodes   episodeBumper
	demand     demandRecorder
	configRepo AutocacheConfigStore
}

// episodeBumper is the narrow slice of *repo.EpisodeRepository the handler needs.
type episodeBumper interface {
	BumpFetch(ctx context.Context, malID string, episode int) error
}

// demandRecorder is the narrow slice of *repo.DemandRepository the handler needs.
type demandRecorder interface {
	Record(ctx context.Context, malID string, episode int, reason domain.DemandReason) error
}

// NewGormAutocacheInternalDeps wires the production autocacheInternalDeps from
// the existing repos already constructed in main.go.
func NewGormAutocacheInternalDeps(episodes episodeBumper, demand demandRecorder, configRepo AutocacheConfigStore) autocacheInternalDeps {
	return &gormAutocacheInternalDeps{episodes: episodes, demand: demand, configRepo: configRepo}
}

func (g *gormAutocacheInternalDeps) BumpFetch(ctx context.Context, malID string, episode int) error {
	return g.episodes.BumpFetch(ctx, malID, episode)
}

func (g *gormAutocacheInternalDeps) RecordDemand(ctx context.Context, malID string, episode int, reason domain.DemandReason) error {
	return g.demand.Record(ctx, malID, episode, reason)
}

func (g *gormAutocacheInternalDeps) ConfigEnabled(ctx context.Context) (bool, error) {
	cfg, err := g.configRepo.Get(ctx)
	if err != nil {
		return false, err
	}
	return cfg.Enabled, nil
}
