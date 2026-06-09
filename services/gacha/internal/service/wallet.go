package service

import (
	"context"

	apperrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/gacha/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/gacha/internal/repo"
)

// WalletService is the economy use-case layer over WalletRepository.
type WalletService struct {
	repo         *repo.WalletRepository
	starterBonus int64
	enabled      bool
	log          *logger.Logger
}

func NewWalletService(r *repo.WalletRepository, starterBonus int64, enabled bool, log *logger.Logger) *WalletService {
	return &WalletService{repo: r, starterBonus: starterBonus, enabled: enabled, log: log}
}

// GetOrCreate returns the user's wallet, granting the one-time starter bonus
// on first creation (only when the service is enabled). The starter grant is
// guarded by an atomic compare-and-set on starter_granted, so two concurrent
// first-accesses grant exactly once.
func (s *WalletService) GetOrCreate(ctx context.Context, userID string) (*domain.Wallet, error) {
	w, err := s.repo.GetOrCreate(ctx, userID)
	if err != nil {
		return nil, err
	}
	if s.enabled && !w.StarterGranted && s.starterBonus > 0 {
		didGrant, err := s.repo.MarkStarterGranted(ctx, userID)
		if err != nil {
			return nil, err
		}
		if didGrant {
			if _, err := s.repo.Credit(ctx, userID, s.starterBonus, domain.ReasonStarter, "starter"); err != nil {
				return nil, err
			}
			s.log.Infow("granted gacha starter bonus", "user_id", userID, "amount", s.starterBonus)
		}
		// Re-read so the returned wallet reflects the grant.
		return s.repo.GetOrCreate(ctx, userID)
	}
	return w, nil
}

// Credit adds delta «Энигмы» to a user's wallet under (reason, ref). Returns
// applied=false when the credit was a deduped no-op or the service is
// disabled. delta must be positive (spend paths use a separate debit method
// in a later phase).
func (s *WalletService) Credit(ctx context.Context, userID string, delta int64, reason, ref string) (bool, error) {
	if delta <= 0 {
		return false, apperrors.InvalidInput("credit amount must be positive")
	}
	if !s.enabled {
		return false, nil
	}
	// Ensure the wallet row exists before crediting (no starter side-effects
	// on the hot credit path — GetOrCreate at repo level, not service level).
	if _, err := s.repo.GetOrCreate(ctx, userID); err != nil {
		return false, err
	}
	return s.repo.Credit(ctx, userID, delta, reason, ref)
}
