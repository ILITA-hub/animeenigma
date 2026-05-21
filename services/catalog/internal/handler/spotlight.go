package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service/spotlight"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service/spotlight/cards"
)

// spotlightCtxTimeout is the per-request budget the handler imposes when
// calling the aggregator. The aggregator itself layers its own internal
// 2s overall budget (HSB-BE-04); this outer ctx exists so the handler
// can be sure of a hard cap even if the aggregator is misconfigured.
const spotlightCtxTimeout = 2 * time.Second

// aggregator is the subset of *spotlight.Aggregator that SpotlightHandler
// needs. Defined in this package so tests can substitute a handwritten
// fake (pattern: services/catalog/internal/handler/scraper_test.go).
// The production *spotlight.Aggregator satisfies this interface
// implicitly via its Resolve method.
type aggregator interface {
	Resolve(ctx context.Context, userID *string) (*spotlight.Response, error)
}

// SpotlightHandler serves GET /api/home/spotlight (workstream
// hero-spotlight, v1.0 Phase 1 — Phase 3 added optional-auth wiring).
//
// DELIBERATE DIVERGENCE 3 (non-negotiable, enforced by acceptance grep):
// this handler writes the bare {cards, generated_at} JSON envelope
// directly via json.NewEncoder — it deliberately avoids the shared
// response-helpers package that wraps payloads in
// {success, data} / {success:false, error:{...}} envelopes. The Phase 2
// frontend HSB-FE-02 would parse a wrapped envelope as truthy and
// surface the wrong cards path. See 01-PATTERNS.md §"HTTP response
// pattern (DIVERGENCE from the shared OK helper)" for the full rationale.
type SpotlightHandler struct {
	agg     aggregator
	enabled bool
	log     *logger.Logger
}

// NewSpotlightHandler constructs the handler. enabled mirrors
// cfg.SpotlightEnabled (HSB-BE-07) — when false, Get short-circuits to a
// bare 404 with no body.
func NewSpotlightHandler(agg aggregator, enabled bool, log *logger.Logger) *SpotlightHandler {
	return &SpotlightHandler{agg: agg, enabled: enabled, log: log}
}

// Get handles GET /api/home/spotlight. Status / body matrix:
//
//   - 200 + {cards:[...], generated_at:"ISO8601"} on success (including
//     partial success — at least one card resolved or zero cards
//     successfully resolved with the aggregator's snapshot fallback).
//   - 404 + empty body when SpotlightEnabled=false. Bare 404 — NO
//     {success:false, error:{...}} envelope. Frontend HSB-FE-02 treats
//     this as "block hides itself".
//   - 500 + {cards:[], generated_at:"ISO8601"} on a catastrophic
//     aggregator error (aggregator returned a non-nil err — in practice
//     this almost never happens because aggregator.Resolve absorbs all
//     per-card failures and returns partial-success).
//
// Phase 3 contract: this endpoint is mounted behind OptionalAuthMiddleware
// (transport/optional_auth.go), so claims may or may not be present on
// the request context. When present, the handler derives userID and
// passes it to aggregator.Resolve so login-only cards (personal_pick
// login path, not_time_yet, continue_watching_new) become eligible. The
// raw bearer token is forwarded on ctx via cards.ContextWithJWT so the
// personal_pick login path can fan out to player /api/users/recs with
// the caller's auth.
func (h *SpotlightHandler) Get(w http.ResponseWriter, r *http.Request) {
	// HSB-BE-07: feature flag short-circuit. Bare 404 — deliberately
	// NOT the shared NotFound helper (which writes a
	// {success:false,error:{...}} envelope that the frontend would
	// parse as truthy).
	if !h.enabled {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	started := time.Now()

	// Phase 3 — derive userID from authz claims (set by OptionalAuthMiddleware
	// upstream). Anon callers have no claims attached → userID stays nil.
	// Do NOT log the bearer token (Pitfall 6 in 01-RESEARCH.md — never leak
	// JWTs to logs). User label is "anon" or the userID — never the JWT.
	var userID *string
	userLabel := "anon"
	if claims, ok := authz.ClaimsFromContext(r.Context()); ok && claims != nil && claims.UserID != "" {
		id := claims.UserID
		userID = &id
		userLabel = id
	}
	h.log.Infow("spotlight.request", "user", userLabel)

	// Attach the raw bearer token (if present) to ctx so login-only resolvers
	// can forward it to player /api/users/recs. Empty string is fine —
	// cards.JWTFromContext returns ok=false on empty. We forward the token
	// regardless of claims presence: when OptionalAuthMiddleware stripped an
	// invalid JWT, claims will be absent and userID stays nil; login-only
	// resolvers gate on userID, not on JWT presence, so an invalid bearer is
	// harmless on ctx.
	ctx := cards.ContextWithJWT(r.Context(), httputil.BearerToken(r))
	ctx, cancel := context.WithTimeout(ctx, spotlightCtxTimeout)
	defer cancel()

	resp, err := h.agg.Resolve(ctx, userID)
	if err != nil {
		// Catastrophic — aggregator itself failed. In practice this
		// almost never happens because aggregator.Resolve absorbs all
		// per-card failures. When it DOES happen, still emit the bare
		// envelope shape (cards:[], generated_at:"now") so the frontend
		// parser never blows up — it just sees an empty card list and
		// hides the block.
		h.log.Errorw("spotlight.aggregator_failed", "error", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(&spotlight.Response{
			Cards:       []spotlight.Card{},
			GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		})
		return
	}

	h.log.Infow("spotlight.aggregated",
		"cards_returned", len(resp.Cards),
		"ms_total", time.Since(started).Milliseconds(),
	)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		h.log.Errorw("spotlight.encode_failed", "error", err)
	}
}
