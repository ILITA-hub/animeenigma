package service

import (
	"context"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/player/internal/repo"
)

type PreferenceService struct {
	prefRepo *repo.PreferenceRepository
	log      *logger.Logger
	tier2    Tier2Params
}

// Tier2Params carries the runtime-tunable Phase 6 inputs into the service.
// Mirrors config.Tier2Config but lives in the service package so the resolver
// has no compile-time dependency on the player config layer.
type Tier2Params struct {
	HalfLifeDays   float64
	MinConfidence  float64
	MaxHistoryRows int
	DurationFloor  int
}

// DefaultTier2Params returns sensible defaults — used by tests and as a
// fallback when callers construct a service without explicit tuning.
func DefaultTier2Params() Tier2Params {
	return Tier2Params{
		HalfLifeDays:   30.0,
		MinConfidence:  1800.0,
		MaxHistoryRows: 5000,
		DurationFloor:  60,
	}
}

func NewPreferenceService(prefRepo *repo.PreferenceRepository, log *logger.Logger) *PreferenceService {
	return &PreferenceService{
		prefRepo: prefRepo,
		log:      log,
		tier2:    DefaultTier2Params(),
	}
}

// NewPreferenceServiceWithTier2 wires Phase 6 tunables in. main.go uses this
// to thread cfg.Tier2 into the resolver.
func NewPreferenceServiceWithTier2(prefRepo *repo.PreferenceRepository, log *logger.Logger, tier2 Tier2Params) *PreferenceService {
	return &PreferenceService{
		prefRepo: prefRepo,
		log:      log,
		tier2:    tier2,
	}
}

// UpsertAnimePreference builds a UserAnimePreference from the request combo fields and upserts via repo
func (s *PreferenceService) UpsertAnimePreference(ctx context.Context, userID string, req *domain.UpdateProgressRequest) {
	pref := &domain.UserAnimePreference{
		UserID:           userID,
		AnimeID:          req.AnimeID,
		Player:           req.Player,
		Language:         req.Language,
		WatchType:        req.WatchType,
		TranslationID:    req.TranslationID,
		TranslationTitle: req.TranslationTitle,
		UpdatedAt:        time.Now(),
	}

	if err := s.prefRepo.UpsertAnimePreference(ctx, pref); err != nil {
		s.log.Errorw("failed to upsert anime preference",
			"user_id", userID,
			"anime_id", req.AnimeID,
			"error", err,
		)
	}
}

// Resolve loads all data sources from DB, computes the Phase 6 weighted Tier 2
// signals, and calls the pure Resolve function. Increments resolve metrics
// (and tier2_thin_signal_skip_total when the min-confidence floor declines).
func (s *PreferenceService) Resolve(ctx context.Context, userID string, req *domain.ResolveRequest) (*domain.ResolveResponse, error) {
	anonLabel := "true"
	if userID != "" {
		anonLabel = "false"
	}

	// Load Tier 1: per-anime preference
	userPref, _ := s.prefRepo.GetAnimePreference(ctx, userID, req.AnimeID)

	// Load Tier 2: weighted history aggregation (Phase 6 rewrite).
	// Skipped entirely for anonymous callers — they have no userID, so no
	// history to weight. Anonymous Tier 2 lives client-side via localStorage
	// in Phase 7.
	var tier2Lock *domain.Tier2Lock
	if userID != "" {
		history, err := s.prefRepo.GetUserHistoryForTier2(ctx, userID, s.tier2.MaxHistoryRows)
		if err != nil {
			s.log.Warnw("tier 2 history fetch failed; falling through to community",
				"user_id", userID,
				"error", err,
			)
		} else {
			coarse, fine, total := AggregateTier2(history, s.tier2.HalfLifeDays, time.Now(), s.tier2.DurationFloor)
			tier2Lock = ChooseTier2Lock(coarse, fine, total, s.tier2.MinConfidence)
			if tier2Lock == nil && total > 0 {
				// total>0 distinguishes "thin signal skip" (had some history,
				// but below floor) from "no history at all" (truly first-time
				// user). We only count the former.
				metrics.Tier2ThinSignalSkipTotal.WithLabelValues(anonLabel).Inc()
			}
		}
	}

	// Load Tier 3: community popularity for this anime
	community, _ := s.prefRepo.GetCommunityPopularity(ctx, req.AnimeID)

	// Load Tier 4: pinned translations for this anime
	pinned, _ := s.prefRepo.GetPinnedTranslations(ctx, req.AnimeID)

	// Call the pure resolver function
	result := Resolve(userPref, tier2Lock, community, pinned, req.Available)

	// Increment metrics
	tier := "null"
	language := ""
	player := ""
	if result != nil {
		tier = result.Tier
		language = result.Language
		player = result.Player
	}
	metrics.PreferenceResolutionTotal.WithLabelValues(tier).Inc()

	// ComboResolveTotal — the rate denominator for combo_override_total. Same label
	// derivation as the override handler so PromQL division by label group lines up.
	// anonLabel is set above (top of Resolve) so Tier 2 thin-signal skip metric
	// can also use it without recomputation.
	metrics.ComboResolveTotal.WithLabelValues(
		labelOrUnknownService(tier),
		labelOrUnknownService(language),
		anonLabel,
		labelOrUnknownService(player),
	).Inc()

	return &domain.ResolveResponse{Resolved: result}, nil
}

// labelOrUnknownService coerces empty strings to "unknown" for Prometheus label values.
// Mirror of services/player/internal/handler/override.go labelOrUnknown — kept package-local
// because the handler version is private to the handler package. T-01-02 cardinality
// bound also applies here at the service boundary.
func labelOrUnknownService(s string) string {
	if s == "" {
		return "unknown"
	}
	return s
}

// GetAnimePreference returns the user's saved preference for a specific anime
func (s *PreferenceService) GetAnimePreference(ctx context.Context, userID, animeID string) (*domain.UserAnimePreference, error) {
	return s.prefRepo.GetAnimePreference(ctx, userID, animeID)
}

// GetGlobalPreferences returns the user's top combos ranked by watch count
func (s *PreferenceService) GetGlobalPreferences(ctx context.Context, userID string) ([]domain.ComboCount, error) {
	return s.prefRepo.GetUserTopCombos(ctx, userID, 10)
}

// GetTier2DebugView produces the user's current Tier 2 weighted signals plus
// the active min-confidence floor and half-life, so the Advanced Settings UI
// can show "raw weights" and the resolved lock side-by-side. Phase 7 B-05.
func (s *PreferenceService) GetTier2DebugView(ctx context.Context, userID string) (*domain.Tier2DebugView, error) {
	history, err := s.prefRepo.GetUserHistoryForTier2(ctx, userID, s.tier2.MaxHistoryRows)
	if err != nil {
		return nil, err
	}
	coarse, fine, total := AggregateTier2(history, s.tier2.HalfLifeDays, time.Now(), s.tier2.DurationFloor)
	lock := ChooseTier2Lock(coarse, fine, total, s.tier2.MinConfidence)
	if coarse == nil {
		coarse = []domain.WeightedCoarse{}
	}
	if fine == nil {
		fine = []domain.WeightedFine{}
	}
	return &domain.Tier2DebugView{
		Coarse:        coarse,
		Fine:          fine,
		TotalWeight:   total,
		MinConfidence: s.tier2.MinConfidence,
		HalfLifeDays:  s.tier2.HalfLifeDays,
		Lock:          lock,
	}, nil
}

// ForceCombo writes a per-anime preference with explicit user intent.
// Underlying write is the same UpsertAnimePreference used by heartbeat saves,
// so prefs_version bumps for cache invalidation. Phase 7 B-05.
func (s *PreferenceService) ForceCombo(ctx context.Context, userID, animeID string, req *domain.ForceComboRequest) error {
	if !domain.ValidateCombo(req.Player, req.Language, req.WatchType) {
		return errors.InvalidInput("invalid combo: player/language/watch_type must be from the allowed set")
	}
	pref := &domain.UserAnimePreference{
		UserID:           userID,
		AnimeID:          animeID,
		Player:           req.Player,
		Language:         req.Language,
		WatchType:        req.WatchType,
		TranslationID:    req.TranslationID,
		TranslationTitle: req.TranslationTitle,
		UpdatedAt:        time.Now(),
	}
	return s.prefRepo.UpsertAnimePreference(ctx, pref)
}

// ResetLearnedPreferences deletes all per-anime preference rows for the user.
// Watch history is preserved — Tier 2 weights regrow naturally. Phase 7 B-05.
// Returns the new prefs_version (always > 0 because the reset itself bumps).
func (s *PreferenceService) ResetLearnedPreferences(ctx context.Context, userID string) (int64, error) {
	return s.prefRepo.ResetLearnedPreferences(ctx, userID)
}

// GetPrefsVersion returns the user's prefs_version generation counter.
// 0 = no preferences ever saved. Phase 7 D-03.
func (s *PreferenceService) GetPrefsVersion(ctx context.Context, userID string) (int64, error) {
	return s.prefRepo.GetPrefsVersion(ctx, userID)
}
