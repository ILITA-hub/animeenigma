package service

import (
	"context"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/player/internal/repo"
)

type PreferenceService struct {
	prefRepo *repo.PreferenceRepository
	log      *logger.Logger
}

func NewPreferenceService(prefRepo *repo.PreferenceRepository, log *logger.Logger) *PreferenceService {
	return &PreferenceService{prefRepo: prefRepo, log: log}
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

// Resolve loads all 4 data sources from DB, calls the pure Resolve function, and increments metrics
func (s *PreferenceService) Resolve(ctx context.Context, userID string, req *domain.ResolveRequest) (*domain.ResolveResponse, error) {
	// Load Tier 1: per-anime preference
	userPref, _ := s.prefRepo.GetAnimePreference(ctx, userID, req.AnimeID)

	// Load Tier 2: user's global favorite
	globalFav, _ := s.prefRepo.GetUserGlobalFavorite(ctx, userID)

	// Load Tier 3: community popularity for this anime
	community, _ := s.prefRepo.GetCommunityPopularity(ctx, req.AnimeID)

	// Load Tier 4: pinned translations for this anime
	pinned, _ := s.prefRepo.GetPinnedTranslations(ctx, req.AnimeID)

	// Call the pure resolver function
	result := Resolve(userPref, globalFav, community, pinned, req.Available)

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
	// anon="true" if userID is empty (caller had no JWT claims and possibly an X-Anon-ID
	// captured at the handler boundary; the service treats both as "anonymous").
	anonLabel := "true"
	if userID != "" {
		anonLabel = "false"
	}
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
