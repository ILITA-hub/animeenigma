package repo

import (
	"context"

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

// MarkStarterGranted flips starter_granted to true and returns whether THIS
// call did the flip (false if it was already true). Used to grant the
// starter bonus exactly once. Atomic compare-and-set via WHERE guard.
func (r *WalletRepository) MarkStarterGranted(ctx context.Context, userID string) (didGrant bool, err error) {
	res := r.db.WithContext(ctx).Model(&domain.Wallet{}).
		Where("user_id = ? AND starter_granted = ?", userID, false).
		Update("starter_granted", true)
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected == 1, nil
}
