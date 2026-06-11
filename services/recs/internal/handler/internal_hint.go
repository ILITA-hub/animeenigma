// Package handler — internal_hint.go: POST /internal/recs/recompute-hint.
//
// Phase 1 of the recs extraction (spec 2026-06-11). Replaces the two
// in-process couplings player's ListService.MarkEpisodeWatched used to have:
//
//  1. Debounced user-signal recompute (was userOrchestrator.TriggerForUser).
//  2. S6 seed update + per-user cache bust on qualifying completion
//     (status='completed' AND score>=7 AND completed_at set) — was a
//     synchronous repo call inside the player request path.
//
// Docker-network-only: the gateway does not proxy /internal/*.
package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"gorm.io/gorm"

	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/recs/internal/repo"
	"github.com/ILITA-hub/animeenigma/services/recs/internal/service/recs"
)

// hintSeedScoreThreshold mirrors the player-side Phase 13 gate: only
// completions scored >= 7 qualify as S6 pin seeds.
const hintSeedScoreThreshold = 7

// hintListEntry is the narrow anime_list projection the seed check needs.
type hintListEntry struct {
	Status      string
	Score       int
	CompletedAt *time.Time
}

// hintDeps is the narrow surface the handler depends on; production wires
// NewGormHintDeps (below), tests inject a fake.
type hintDeps interface {
	TriggerForUser(ctx context.Context, userID string) error
	LookupCompletion(ctx context.Context, userID, animeID string) (*hintListEntry, error)
	UpdateS6Seed(ctx context.Context, userID, animeID string, completedAt time.Time, score int) error
	DeleteCache(ctx context.Context, keys ...string) error
}

// InternalHintHandler serves POST /internal/recs/recompute-hint.
type InternalHintHandler struct {
	deps hintDeps
	log  *logger.Logger
}

// NewInternalHintHandler constructs an InternalHintHandler with the given
// deps and logger. Tests inject a fakeHintDeps; production uses NewGormHintDeps.
func NewInternalHintHandler(deps hintDeps, log *logger.Logger) *InternalHintHandler {
	return &InternalHintHandler{deps: deps, log: log}
}

type hintBody struct {
	UserID  string `json:"user_id"`
	AnimeID string `json:"anime_id"`
}

// PostRecomputeHint handles POST /internal/recs/recompute-hint.
//
// It performs two operations:
//
//  1. Debounced recompute trigger (always, for any valid user_id).
//  2. S6 seed update + cache bust when the anime_list entry for the given
//     (user_id, anime_id) pair qualifies: status='completed', score>=7,
//     completed_at set.
func (h *InternalHintHandler) PostRecomputeHint(w http.ResponseWriter, r *http.Request) {
	var body hintBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httputil.BadRequest(w, "invalid body")
		return
	}
	if body.UserID == "" {
		httputil.BadRequest(w, "user_id is required")
		return
	}
	ctx := r.Context()

	// 1. Debounced recompute. TriggerForUser always returns nil by contract
	//    (UserOrchestrator owns the SetNX debounce; best-effort since Phase 11).
	//    context.WithoutCancel so a caller disconnect can't cancel the SetNX
	//    mid-flight — same rationale as the original player-side trigger.
	_ = h.deps.TriggerForUser(context.WithoutCancel(r.Context()), body.UserID)

	// 2. S6 seed update on qualifying completion. anime_id may be empty for
	//    generic hints — skip the seed path then.
	if body.AnimeID != "" {
		entry, err := h.deps.LookupCompletion(ctx, body.UserID, body.AnimeID)
		if err != nil {
			h.log.Warnw("hint completion lookup failed (non-fatal)",
				"user_id", body.UserID, "anime_id", body.AnimeID, "error", err)
		} else if entry != nil && entry.Status == "completed" && entry.Score >= hintSeedScoreThreshold && entry.CompletedAt != nil {
			if err := h.deps.UpdateS6Seed(ctx, body.UserID, body.AnimeID, *entry.CompletedAt, entry.Score); err != nil {
				h.log.Errorw("hint s6 seed update failed (non-fatal)",
					"user_id", body.UserID, "anime_id", body.AnimeID, "error", err)
			} else if err := h.deps.DeleteCache(ctx, recs.UserTopNKey(recs.UserID(body.UserID))); err != nil {
				h.log.Warnw("hint cache bust failed (non-fatal)", "user_id", body.UserID, "error", err)
			}
		}
	}

	httputil.OK(w, map[string]bool{"ok": true})
}

// gormHintDeps is the production hintDeps implementation: GORM reads of the
// shared anime_list table + the recs repo + Redis cache + user orchestrator.
type gormHintDeps struct {
	db       *gorm.DB
	repo     *repo.RecsRepository
	cache    hintCache
	userOrch *recs.UserOrchestrator
}

// hintCache is the narrow cache surface this handler needs.
type hintCache interface {
	Delete(ctx context.Context, keys ...string) error
}

// NewGormHintDeps wires the production hintDeps.
func NewGormHintDeps(db *gorm.DB, recsRepo *repo.RecsRepository, cache hintCache, userOrch *recs.UserOrchestrator) hintDeps {
	return &gormHintDeps{db: db, repo: recsRepo, cache: cache, userOrch: userOrch}
}

func (g *gormHintDeps) TriggerForUser(ctx context.Context, userID string) error {
	return g.userOrch.TriggerForUser(ctx, recs.UserID(userID))
}

func (g *gormHintDeps) LookupCompletion(ctx context.Context, userID, animeID string) (*hintListEntry, error) {
	var row hintListEntry
	res := g.db.WithContext(ctx).
		Table("anime_list").
		Select("status, score, completed_at").
		Where("user_id = ? AND anime_id = ?", userID, animeID).
		Limit(1).
		Scan(&row)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, nil
	}
	return &row, nil
}

func (g *gormHintDeps) UpdateS6Seed(ctx context.Context, userID, animeID string, completedAt time.Time, score int) error {
	return g.repo.UpdateS6Seed(ctx, userID, animeID, completedAt, score)
}

func (g *gormHintDeps) DeleteCache(ctx context.Context, keys ...string) error {
	return g.cache.Delete(ctx, keys...)
}
