// Package handler — autocache_internal.go: the Docker-network-only serve-signal
// endpoints the catalog ae-resolution producer (Plan 03) calls.
//
//	POST /internal/library/autocache/fetch   HIT  → BumpFetch + serve_total{hit}
//	POST /internal/library/autocache/demand  MISS → DemandRepository.Record(reason)
//	                                                + serve_total{miss}
//
// Both live OUTSIDE /api/library so the gateway never proxies them (same rule
// as recs' /internal/recs/recompute-hint + notifications' /internal/notifications).
// The library service owns the library_autocache_* metrics + the ledger writes,
// so the producer reports the resolution outcome here rather than emitting the
// counter itself.
//
// Phase 09 — the demand reason is VALIDATED-AND-HONORED, no longer force-
// overridden to backfill. The wire `reason` is validated against the
// {backfill, next_ep, ongoing} allowlist and recorded as-is (an absent or unknown
// value falls back to backfill). This is safe because /internal/* is Docker-
// network-only (the gateway never proxies it — T-08-04), so the only callers are
// the internal-trusted producers: player Logic B → next_ep, scheduler Logic A →
// ongoing, catalog serve-miss → backfill. The original T-08-05 "spoofed next_ep"
// concern was about an UNTRUSTED external caller, which cannot reach this route;
// a malformed internal caller still degrades safely to backfill.
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
	// RecordDemand upserts a wanted (mal_id, episode) demand row, carrying the
	// ordered title list (name_jp → romaji → name_en) the producer supplied so the
	// Planner can search trackers by title. Phase 08 passed only backfill + no
	// titles; v4.1 threads titles from all three producers.
	RecordDemand(ctx context.Context, malID string, episode int, reason domain.DemandReason, titles []string) error
	// ConfigEnabled reports the master autocache `enabled` switch.
	ConfigEnabled(ctx context.Context) (bool, error)
	// RecordTrigger appends a cause→effect trigger-log row (best-effort).
	RecordTrigger(ctx context.Context, row *domain.AutocacheTriggerLog) error
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
// "reason" field on the wire; Phase 09 validates it against the
// {backfill, next_ep, ongoing} allowlist and honors it (defaulting to backfill on
// an absent/unknown value) — safe because the /internal/* route is Docker-network-
// only so the callers are internal-trusted producers (T-08-04). Fetch ignores it.
type autocacheSignalBody struct {
	MALID   string `json:"mal_id"`
	Episode int    `json:"episode"`
	Reason  string `json:"reason"`
	// Titles is the ordered, fallback-ranked anime title list (name_jp → romaji →
	// name_en) the producer supplies on a demand POST. The library has no anime
	// titles of its own, so without this the Planner can only search "<mal> <ep>".
	// Optional + ignored by the fetch endpoint; producers send non-empty + deduped.
	Titles []string `json:"titles,omitempty"`
	// Trigger is the optional cause→effect context for a USER-DRIVEN demand (player
	// Logic B, catalog backfill): who/when/what-watched. When present, the handler
	// appends an autocache_trigger_log row keyed on (mal_id, episode=target) so the
	// dashboard can show the watch that caused each download. Absent for the
	// aggregate scheduler Logic A push (no single user). Ignored by fetch.
	Trigger *demandTrigger `json:"trigger,omitempty"`
}

// demandTrigger is the watcher context attached to a user-driven demand. The
// demand's `episode` field is the TARGET episode (what the autocache fetches);
// WatchedEpisode is the episode the user was actually watching (the cause).
type demandTrigger struct {
	UserID         string `json:"user_id,omitempty"`
	Username       string `json:"username,omitempty"`
	Player         string `json:"player,omitempty"`
	Language       string `json:"language,omitempty"`
	WatchType      string `json:"watch_type,omitempty"`
	WatchedEpisode int    `json:"watched_episode,omitempty"`
}

// maxEpisode bounds the accepted episode number (WR-02). Go decodes JSON
// `episode` into a 64-bit int, but the autocache_demand.episode and
// library_episodes.episode_number columns are Postgres INT (int4, max
// 2147483647): an oversized value passes a bare `> 0` check, then overflows on
// the DB write (numeric value out of range) and is silently swallowed. Reject
// it at the edge instead — 100000 is generous (no anime has that many episodes)
// and well inside int4.
const maxEpisode = 100000

// decodeSignal decodes + validates the shared body. Returns false (and writes a
// 400) when the body is malformed or mal_id/episode are missing/out of range.
func (h *AutocacheInternalHandler) decodeSignal(w http.ResponseWriter, r *http.Request) (autocacheSignalBody, bool) {
	var body autocacheSignalBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httputil.BadRequest(w, "invalid body")
		return autocacheSignalBody{}, false
	}
	if body.MALID == "" || body.Episode <= 0 || body.Episode > maxEpisode {
		httputil.BadRequest(w, "mal_id and a sane positive episode are required")
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

// validateDemandReason maps the wire `reason` to a domain.DemandReason. It returns
// the wire value when it is one of the known enum literals
// (backfill / next_ep / ongoing) and falls back to DemandReasonBackfill for an
// absent ("") or unknown value. The /internal/* route is Docker-network-only so the
// producers are internal-trusted (T-08-04); the allowlist + safe default mean a
// malformed internal caller degrades to backfill rather than writing an invalid
// enum value (T-09-15).
func validateDemandReason(raw string) domain.DemandReason {
	switch domain.DemandReason(raw) {
	case domain.DemandReasonBackfill, domain.DemandReasonNextEp, domain.DemandReasonOngoing:
		return domain.DemandReason(raw)
	default:
		return domain.DemandReasonBackfill
	}
}

// Demand handles POST /internal/library/autocache/demand — the MISS path. It
// reads the master `enabled` switch first; when enabled it records the demand with
// the validated wire reason (best-effort) and counts a serve_total{miss}. When
// disabled — or when the config read errors (fail closed) — it skips BOTH the
// record and the miss counter but still returns 200, so the serve path never
// regresses on a config blip. The reason is validated against
// {backfill, next_ep, ongoing} and honored; an absent/unknown reason defaults to
// backfill (validateDemandReason).
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

	reason := validateDemandReason(body.Reason)
	if err := h.deps.RecordDemand(ctx, body.MALID, body.Episode, reason, body.Titles); err != nil {
		h.log.Warnw("autocache demand record failed (non-fatal)",
			"mal_id", body.MALID, "episode", body.Episode, "error", err)
	}
	// Cause→effect: a user-driven demand (Logic B / backfill) carries a trigger
	// block — append an autocache_trigger_log row so the dashboard can tie the
	// resulting download back to who/when/what was watched. Best-effort; a logging
	// failure never affects the demand. body.Episode is the TARGET episode (the
	// join key to the eventual download).
	if body.Trigger != nil {
		tl := &domain.AutocacheTriggerLog{
			MalID:          body.MALID,
			TargetEpisode:  body.Episode,
			Reason:         string(reason),
			UserID:         body.Trigger.UserID,
			Username:       body.Trigger.Username,
			WatchPlayer:    body.Trigger.Player,
			WatchLanguage:  body.Trigger.Language,
			WatchType:      body.Trigger.WatchType,
			WatchedEpisode: body.Trigger.WatchedEpisode,
		}
		if err := h.deps.RecordTrigger(ctx, tl); err != nil {
			h.log.Warnw("autocache trigger log insert failed (non-fatal)",
				"mal_id", body.MALID, "episode", body.Episode, "error", err)
		}
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
	triggers   triggerRecorder
}

// triggerRecorder is the narrow slice of *repo.TriggerLogRepository the handler
// needs to append a cause→effect row.
type triggerRecorder interface {
	Insert(ctx context.Context, row *domain.AutocacheTriggerLog) error
}

// episodeBumper is the narrow slice of *repo.EpisodeRepository the handler needs.
type episodeBumper interface {
	BumpFetch(ctx context.Context, malID string, episode int) error
}

// demandRecorder is the narrow slice of *repo.DemandRepository the handler needs.
type demandRecorder interface {
	Record(ctx context.Context, malID string, episode int, reason domain.DemandReason, titles []string) error
}

// NewGormAutocacheInternalDeps wires the production autocacheInternalDeps from
// the existing repos already constructed in main.go.
func NewGormAutocacheInternalDeps(episodes episodeBumper, demand demandRecorder, configRepo AutocacheConfigStore, triggers triggerRecorder) autocacheInternalDeps {
	return &gormAutocacheInternalDeps{episodes: episodes, demand: demand, configRepo: configRepo, triggers: triggers}
}

func (g *gormAutocacheInternalDeps) BumpFetch(ctx context.Context, malID string, episode int) error {
	return g.episodes.BumpFetch(ctx, malID, episode)
}

func (g *gormAutocacheInternalDeps) RecordDemand(ctx context.Context, malID string, episode int, reason domain.DemandReason, titles []string) error {
	return g.demand.Record(ctx, malID, episode, reason, titles)
}

func (g *gormAutocacheInternalDeps) ConfigEnabled(ctx context.Context) (bool, error) {
	cfg, err := g.configRepo.Get(ctx)
	if err != nil {
		return false, err
	}
	return cfg.Enabled, nil
}

func (g *gormAutocacheInternalDeps) RecordTrigger(ctx context.Context, row *domain.AutocacheTriggerLog) error {
	return g.triggers.Insert(ctx, row)
}
