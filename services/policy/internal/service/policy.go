// Package service holds policy-service business logic: seeding defaults,
// resolving the compact ruleset (for the gateway) and the per-user feed (for the
// SPA), and admin writes. Everything defaults fail-open (empty store ⇒ roulette
// on, nothing gated beyond seed defaults).
package service

import (
	"context"
	"sort"

	liberrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/policy/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/policy/internal/repo"
)

type PolicyService struct {
	repo *repo.FeatureFlagRepository
	log  *logger.Logger
}

func NewPolicyService(r *repo.FeatureFlagRepository, log *logger.Logger) *PolicyService {
	return &PolicyService{repo: r, log: log}
}

// SeedDefaults inserts the dark-ship-equivalent defaults (insert-if-absent) plus
// the __roulette__ master (defaults ON). Idempotent across restarts.
func (s *PolicyService) SeedDefaults(ctx context.Context) error {
	for _, f := range domain.SeedFlags() {
		if err := s.repo.SeedIfAbsent(ctx, f); err != nil {
			return err
		}
	}
	return s.repo.SeedIfAbsent(ctx, domain.FeatureFlag{
		Key: domain.RouletteMasterKey, Roulette: true, FailSafe: "everyone",
	})
}

// Ruleset is the compact snapshot the gateway caches (Phase 2).
func (s *PolicyService) Ruleset(ctx context.Context) (domain.Ruleset, error) {
	rows, err := s.repo.GetAll(ctx)
	if err != nil {
		return domain.Ruleset{}, err
	}
	rs := domain.Ruleset{
		RouletteEnabled: true, // fail-open when master row absent
		Flags:           map[string]domain.Audience{},
		FailSafe:        map[string]string{},
		Roulette:        map[string]bool{},
	}
	for _, f := range rows {
		if f.Key == domain.RouletteMasterKey {
			rs.RouletteEnabled = f.Roulette
			continue
		}
		rs.Flags[f.Key] = f.Audience()
		rs.FailSafe[f.Key] = f.FailSafe
		rs.Roulette[f.Key] = f.Roulette
	}
	return rs, nil
}

// ResolveForUser computes the per-user visible + roulette-eligible key sets.
// Anonymous callers pass userID="" role="" and see only everyone-flags.
func (s *PolicyService) ResolveForUser(ctx context.Context, userID, role string) (domain.MineResponse, error) {
	rows, err := s.repo.GetAll(ctx)
	if err != nil {
		return domain.MineResponse{}, err
	}
	out := domain.MineResponse{RouletteEnabled: true, Visible: []string{}, Roulette: []string{}}
	for _, f := range rows {
		if f.Key == domain.RouletteMasterKey {
			out.RouletteEnabled = f.Roulette
			continue
		}
		if !f.CanAccess(userID, role) {
			continue
		}
		out.Visible = append(out.Visible, f.Key)
		if f.Roulette {
			out.Roulette = append(out.Roulette, f.Key)
		}
	}
	sort.Strings(out.Visible)
	sort.Strings(out.Roulette)
	return out, nil
}

// ListFlags returns the admin view: all non-master flags + the resolved master.
func (s *PolicyService) ListFlags(ctx context.Context) ([]domain.FeatureFlag, bool, error) {
	rows, err := s.repo.GetAll(ctx)
	if err != nil {
		return nil, false, err
	}
	rouletteEnabled := true
	flags := make([]domain.FeatureFlag, 0, len(rows))
	for _, f := range rows {
		if f.Key == domain.RouletteMasterKey {
			rouletteEnabled = f.Roulette
			continue
		}
		flags = append(flags, f)
	}
	sort.Slice(flags, func(i, j int) bool { return flags[i].Key < flags[j].Key })
	return flags, rouletteEnabled, nil
}

// SetFlag upserts one feature flag. Rejects the reserved master key and any
// failSafe outside {admin, everyone}.
func (s *PolicyService) SetFlag(ctx context.Context, key string, a domain.Audience, roulette bool, failSafe, label string) error {
	if key == "" || len(key) > 64 {
		return liberrors.InvalidInput("invalid feature key")
	}
	if key == domain.RouletteMasterKey {
		return liberrors.InvalidInput("reserved key")
	}
	if failSafe != "admin" && failSafe != "everyone" {
		return liberrors.InvalidInput("failSafe must be 'admin' or 'everyone'")
	}
	return s.repo.Upsert(ctx, domain.FeatureFlag{
		Key: key, Roles: a.Roles, AllowUsers: a.AllowUsers, DenyUsers: a.DenyUsers,
		Roulette: roulette, FailSafe: failSafe, Label: label,
	})
}

// SetRoulette flips the global master switch.
func (s *PolicyService) SetRoulette(ctx context.Context, enabled bool) error {
	return s.repo.Upsert(ctx, domain.FeatureFlag{
		Key: domain.RouletteMasterKey, Roulette: enabled, FailSafe: "everyone",
	})
}
