package service

import (
	"context"
	"time"

	apperrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/gacha/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/gacha/internal/repo"
)

// WalletService is the economy use-case layer over WalletRepository.
type WalletService struct {
	repo            *repo.WalletRepository
	starterBonus    int64
	dailyBase       int64
	dailyStreakStep int64
	dailyStreakCap  int64
	enabled         bool
	log             *logger.Logger
}

func NewWalletService(
	r *repo.WalletRepository,
	starterBonus int64,
	dailyBase, dailyStreakStep, dailyStreakCap int64,
	enabled bool,
	log *logger.Logger,
) *WalletService {
	return &WalletService{
		repo:            r,
		starterBonus:    starterBonus,
		dailyBase:       dailyBase,
		dailyStreakStep: dailyStreakStep,
		dailyStreakCap:  dailyStreakCap,
		enabled:         enabled,
		log:             log,
	}
}

// DailyResult is returned by Daily.
type DailyResult struct {
	Claimed bool
	Amount  int64
	Streak  int
	Wallet  *domain.Wallet
}

// Daily processes a daily claim for userID using now as the authoritative
// clock. now is injected so tests can be fully deterministic.
//
// Streak semantics (UTC dates):
//   - same-day  → already claimed, return Claimed=false, no writes.
//   - yesterday → streak = streak+1
//   - gap (>1d) → streak = 1
//
// Bonus = min(streak-1, dailyStreakCap/dailyStreakStep) * dailyStreakStep
// Amount = dailyBase + bonus
//
// The ledger INSERT with OnConflict DoNothing makes same-day double-claims
// collapse even under concurrent races (race-safety falls to the DB index).
func (s *WalletService) Daily(ctx context.Context, userID string, now time.Time) (DailyResult, error) {
	if !s.enabled {
		return DailyResult{}, nil
	}

	w, err := s.repo.GetOrCreate(ctx, userID)
	if err != nil {
		return DailyResult{}, err
	}

	today := now.UTC().Truncate(24 * time.Hour)

	// Same-day check before the tx (optimistic fast-path; the tx re-checks
	// via the unique index regardless).
	if w.LastDailyAt != nil {
		lastDay := w.LastDailyAt.UTC().Truncate(24 * time.Hour)
		if lastDay.Equal(today) {
			return DailyResult{Claimed: false, Wallet: w}, nil
		}
	}

	// Compute new streak.
	newStreak := 1
	if w.LastDailyAt != nil {
		lastDay := w.LastDailyAt.UTC().Truncate(24 * time.Hour)
		yesterday := today.Add(-24 * time.Hour)
		if lastDay.Equal(yesterday) {
			newStreak = w.DailyStreak + 1
		}
	}

	// Bonus: min(streak-1, cap/step) * step
	maxSteps := s.dailyStreakCap / s.dailyStreakStep
	streakSteps := int64(newStreak - 1)
	if streakSteps > maxSteps {
		streakSteps = maxSteps
	}
	bonus := streakSteps * s.dailyStreakStep
	amount := s.dailyBase + bonus

	res, err := s.repo.DailyClaimTx(ctx, userID, amount, newStreak, now)
	if err != nil {
		return DailyResult{}, err
	}
	if !res.Claimed {
		return DailyResult{Claimed: false, Wallet: res.Wallet}, nil
	}

	s.log.Infow("daily claim applied",
		"user_id", userID,
		"amount", amount,
		"streak", newStreak,
	)
	return DailyResult{
		Claimed: true,
		Amount:  amount,
		Streak:  newStreak,
		Wallet:  res.Wallet,
	}, nil
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
		granted, err := s.repo.GrantStarterOnce(ctx, userID, s.starterBonus)
		if err != nil {
			return nil, err
		}
		if granted {
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
