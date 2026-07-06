package service

import (
	"context"
	"sort"

	liberrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/repo"
)

// SecretFeatureService resolves admin on/off overrides for the footer
// «Секретная фича» roulette. Everything defaults to ENABLED (fail-open): an
// empty store means the roulette runs exactly as the frontend defines it.
type SecretFeatureService struct {
	repo *repo.SecretFeatureRepository
	log  *logger.Logger
}

func NewSecretFeatureService(r *repo.SecretFeatureRepository, log *logger.Logger) *SecretFeatureService {
	return &SecretFeatureService{repo: r, log: log}
}

// GetConfig is the admin view: resolved master switch + the sparse per-feature
// override map (the reserved master key is stripped out).
func (s *SecretFeatureService) GetConfig(ctx context.Context) (*domain.SecretFeatureConfig, error) {
	flags, err := s.repo.GetAll(ctx)
	if err != nil {
		return nil, err
	}
	features := make(map[string]bool, len(flags))
	for k, v := range flags {
		if k == domain.RouletteMasterKey {
			continue
		}
		features[k] = v
	}
	return &domain.SecretFeatureConfig{
		RouletteEnabled: resolveEnabled(flags, domain.RouletteMasterKey),
		Features:        features,
	}, nil
}

// PublicState is the anonymous-readable enforcement view the roulette consumes.
func (s *SecretFeatureService) PublicState(ctx context.Context) (*domain.SecretFeaturePublicState, error) {
	flags, err := s.repo.GetAll(ctx)
	if err != nil {
		return nil, err
	}
	disabled := make([]string, 0)
	for k, enabled := range flags {
		if k == domain.RouletteMasterKey || enabled {
			continue
		}
		disabled = append(disabled, k)
	}
	sort.Strings(disabled) // deterministic output
	return &domain.SecretFeaturePublicState{
		RouletteEnabled: resolveEnabled(flags, domain.RouletteMasterKey),
		DisabledKeys:    disabled,
	}, nil
}

// SetRoulette flips the master switch.
func (s *SecretFeatureService) SetRoulette(ctx context.Context, enabled bool) error {
	return s.repo.Set(ctx, domain.RouletteMasterKey, enabled)
}

// SetFeature flips a single feature. The key is validated so the reserved
// master sentinel can't be written through the per-feature path.
func (s *SecretFeatureService) SetFeature(ctx context.Context, key string, enabled bool) error {
	if key == "" {
		return liberrors.InvalidInput("feature key is required")
	}
	if len(key) > 64 {
		return liberrors.InvalidInput("feature key too long")
	}
	if key == domain.RouletteMasterKey {
		return liberrors.InvalidInput("reserved key")
	}
	return s.repo.Set(ctx, key, enabled)
}

// resolveEnabled returns the stored value for key, defaulting to true (enabled)
// when the key has no explicit row.
func resolveEnabled(flags map[string]bool, key string) bool {
	if v, ok := flags[key]; ok {
		return v
	}
	return true
}
