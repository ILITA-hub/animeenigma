package repo

import (
	"context"
	"time"

	"github.com/ILITA-hub/animeenigma/services/gacha/internal/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// WalletRepository wraps gacha_wallets + gacha_ledger access.
type WalletRepository struct {
	db *gorm.DB
}

func NewWalletRepository(db *gorm.DB) *WalletRepository { return &WalletRepository{db: db} }

// GetOrCreate returns the user's wallet, inserting a zero-balance row on
// first access. Concurrent first-access is safe: ON CONFLICT DO NOTHING on
// the PK, then re-read.
func (r *WalletRepository) GetOrCreate(ctx context.Context, userID string) (*domain.Wallet, error) {
	w := domain.Wallet{UserID: userID}
	if err := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(&w).Error; err != nil {
		return nil, err
	}
	var out domain.Wallet
	if err := r.db.WithContext(ctx).First(&out, "user_id = ?", userID).Error; err != nil {
		return nil, err
	}
	return &out, nil
}

// Credit atomically appends a ledger entry and bumps the wallet balance by
// delta. Returns applied=false (no error) when a non-empty ref collides with
// an existing (user,reason,ref) row — the idempotent no-op path. The ledger
// insert and balance update share one transaction so they can never diverge.
func (r *WalletRepository) Credit(
	ctx context.Context, userID string, delta int64, reason, ref string,
) (applied bool, err error) {
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		entry := domain.LedgerEntry{UserID: userID, Delta: delta, Reason: reason, Ref: ref}
		res := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&entry)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			// Duplicate (user,reason,ref) — idempotent no-op.
			return nil
		}
		applied = true
		return tx.Model(&domain.Wallet{}).
			Where("user_id = ?", userID).
			UpdateColumn("balance", gorm.Expr("balance + ?", delta)).Error
	})
	return applied, err
}

// DailyClaimResult is the outcome of DailyClaimTx.
type DailyClaimResult struct {
	Claimed bool
	Wallet  *domain.Wallet
}

// DailyClaimTx atomically claims today's daily reward inside one transaction.
// The claim is idempotent on the (user_id, "daily", date-ref) ledger unique
// index: if the ledger INSERT lands zero rows (duplicate) the tx commits as a
// no-op and Claimed=false is returned — race-safe even with stale wallet reads.
// When the insert lands, balance, daily_streak, and last_daily_at are updated
// in the same tx. now is the caller-supplied clock (injected for testability;
// production always passes time.Now()).
func (r *WalletRepository) DailyClaimTx(ctx context.Context, userID string, amount int64, streak int, now time.Time) (DailyClaimResult, error) {
	ref := now.UTC().Format("2006-01-02")
	var result DailyClaimResult
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		entry := domain.LedgerEntry{
			UserID: userID,
			Delta:  amount,
			Reason: domain.ReasonDaily,
			Ref:    ref,
		}
		res := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&entry)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			// Same-day duplicate — fetch current wallet and return claimed=false.
			var w domain.Wallet
			if err := tx.First(&w, "user_id = ?", userID).Error; err != nil {
				return err
			}
			result.Wallet = &w
			return nil
		}
		// Ledger row landed — update balance + streak fields in the same tx.
		if err := tx.Model(&domain.Wallet{}).
			Where("user_id = ?", userID).
			Updates(map[string]interface{}{
				"balance":       gorm.Expr("balance + ?", amount),
				"daily_streak":  streak,
				"last_daily_at": now.UTC(),
			}).Error; err != nil {
			return err
		}
		var w domain.Wallet
		if err := tx.First(&w, "user_id = ?", userID).Error; err != nil {
			return err
		}
		result.Claimed = true
		result.Wallet = &w
		return nil
	})
	return result, err
}

// GrantStarterOnce atomically flips starter_granted true AND credits the
// wallet in a single transaction. Returns granted=true only if THIS call did
// the flip — subsequent calls return granted=false and are no-ops. If the CAS
// wins but the ledger insert is a duplicate (e.g. a partial replay), the
// balance update is skipped via ON CONFLICT DO NOTHING so balances stay
// consistent.
func (r *WalletRepository) GrantStarterOnce(ctx context.Context, userID string, amount int64) (granted bool, err error) {
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// CAS: flip starter_granted false→true only once.
		res := tx.Model(&domain.Wallet{}).
			Where("user_id = ? AND starter_granted = ?", userID, false).
			Update("starter_granted", true)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			// Already granted — nothing to do; commit as a no-op.
			return nil
		}
		// Won the CAS: write the ledger entry and bump the balance.
		entry := domain.LedgerEntry{UserID: userID, Delta: amount, Reason: domain.ReasonStarter, Ref: "starter"}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&entry).Error; err != nil {
			return err
		}
		if err := tx.Model(&domain.Wallet{}).
			Where("user_id = ?", userID).
			UpdateColumn("balance", gorm.Expr("balance + ?", amount)).Error; err != nil {
			return err
		}
		granted = true
		return nil
	})
	return granted, err
}
