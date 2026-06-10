package repo

import (
	"context"
	"time"

	apperrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/services/gacha/internal/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ErrInsufficientFunds is returned by DebitTx when the wallet balance is below
// the requested debit amount. The conditional UPDATE leaves the wallet
// untouched; callers MUST roll back the surrounding transaction (returning this
// error from a gorm.Transaction func does exactly that).
var ErrInsufficientFunds = apperrors.InvalidInput("insufficient balance")

// DebitTx debits amount «Энигмы» from the user's wallet inside the CALLER's
// transaction, then appends a negative ledger entry. The debit is a conditional
// UPDATE (`balance = balance - ? WHERE balance >= ?`) so a balance below cost
// affects zero rows → ErrInsufficientFunds with no side effects. ref is the
// pull ID (for audit / future idempotency); pull refs are unique per request so
// the ledger dedup index never collides here.
func DebitTx(tx *gorm.DB, userID string, amount int64, reason, ref string) error {
	res := tx.Model(&domain.Wallet{}).
		Where("user_id = ? AND balance >= ?", userID, amount).
		UpdateColumn("balance", gorm.Expr("balance - ?", amount))
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrInsufficientFunds
	}
	entry := domain.LedgerEntry{UserID: userID, Delta: -amount, Reason: reason, Ref: ref}
	return tx.Create(&entry).Error
}

// PullRepository owns the pity-counter and collection writes for the pull
// engine. All *Tx methods operate on the caller-supplied transaction so the
// whole pull (debit + ledger + pity + collection) is atomic.
type PullRepository struct {
	db *gorm.DB
}

func NewPullRepository(db *gorm.DB) *PullRepository { return &PullRepository{db: db} }

// GetPityForUpdate returns the (user, banner) pity row, creating it at 0 if it
// does not exist, and takes a `FOR UPDATE` row lock so concurrent pulls on the
// same banner cannot double-count. (On sqlite FOR UPDATE is a no-op — accepted;
// row-locking is exercised only against Postgres.)
func (r *PullRepository) GetPityForUpdate(tx *gorm.DB, userID, bannerID string) (*domain.PityCounter, error) {
	// Ensure the row exists first (idempotent), then SELECT ... FOR UPDATE.
	seed := domain.PityCounter{UserID: userID, BannerID: bannerID}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&seed).Error; err != nil {
		return nil, err
	}
	var p domain.PityCounter
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("user_id = ? AND banner_id = ?", userID, bannerID).
		First(&p).Error; err != nil {
		return nil, err
	}
	return &p, nil
}

// SavePityTx persists the updated pity counter.
func (r *PullRepository) SavePityTx(tx *gorm.DB, p *domain.PityCounter) error {
	return tx.Model(&domain.PityCounter{}).
		Where("user_id = ? AND banner_id = ?", p.UserID, p.BannerID).
		UpdateColumn("pulls_since_ssr", p.PullsSinceSSR).Error
}

// AddToCollectionTx records each obtained card. Cards are processed one at a
// time so a card appearing twice in the same slice increments twice. Each card
// is an upsert: INSERT ... ON CONFLICT (user_id, card_id) DO UPDATE SET
// count = count + 1. Returns which card IDs were newly obtained (first ever) and
// the resulting count for each card ID after all increments.
func (r *PullRepository) AddToCollectionTx(
	tx *gorm.DB, userID string, cardIDs []string,
) (newIDs map[string]bool, counts map[string]int, err error) {
	newIDs = make(map[string]bool)
	counts = make(map[string]int)
	now := time.Now().UTC()

	for _, cid := range cardIDs {
		// Detect whether the row already exists BEFORE upsert (to set New only
		// on the first ever obtain, and only once even within a batch).
		var existing int64
		if err = tx.Model(&domain.CollectionEntry{}).
			Where("user_id = ? AND card_id = ?", userID, cid).
			Count(&existing).Error; err != nil {
			return nil, nil, err
		}
		if existing == 0 {
			if _, seen := newIDs[cid]; !seen {
				newIDs[cid] = true
			}
		}

		entry := domain.CollectionEntry{
			UserID:          userID,
			CardID:          cid,
			Count:           1,
			FirstObtainedAt: now,
		}
		if err = tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "user_id"}, {Name: "card_id"}},
			DoUpdates: clause.Assignments(map[string]interface{}{"count": gorm.Expr("gacha_collection.count + 1")}),
		}).Create(&entry).Error; err != nil {
			return nil, nil, err
		}
	}

	// Read back the resulting counts for every distinct card touched.
	for _, cid := range cardIDs {
		if _, done := counts[cid]; done {
			continue
		}
		var c int
		if err = tx.Model(&domain.CollectionEntry{}).
			Select("count").
			Where("user_id = ? AND card_id = ?", userID, cid).
			Scan(&c).Error; err != nil {
			return nil, nil, err
		}
		counts[cid] = c
	}
	return newIDs, counts, nil
}

// ListCollection returns all of the user's owned card entries.
func (r *PullRepository) ListCollection(ctx context.Context, userID string) ([]domain.CollectionEntry, error) {
	var entries []domain.CollectionEntry
	err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Find(&entries).Error
	return entries, err
}

// GetPity returns the user's pity counter for a banner WITHOUT a lock (read
// path for the banners view). Returns 0 when no row exists.
func (r *PullRepository) GetPity(ctx context.Context, userID, bannerID string) (int, error) {
	var p domain.PityCounter
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND banner_id = ?", userID, bannerID).
		First(&p).Error
	if err == gorm.ErrRecordNotFound {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return p.PullsSinceSSR, nil
}

// CardsByRarity returns the banner's enabled, non-soft-deleted cards grouped by
// rarity tier. Used by the pull engine to build the roll pool.
func (r *PullRepository) CardsByRarity(ctx context.Context, bannerID string) (map[domain.Rarity][]domain.Card, error) {
	var cards []domain.Card
	err := r.db.WithContext(ctx).
		Model(&domain.Card{}).
		Joins("JOIN gacha_banner_cards bc ON bc.card_id = gacha_cards.id").
		Where("bc.banner_id = ?", bannerID).
		Where("gacha_cards.enabled = ?", true).
		Find(&cards).Error
	if err != nil {
		return nil, err
	}
	pool := make(map[domain.Rarity][]domain.Card)
	for _, c := range cards {
		pool[c.Rarity] = append(pool[c.Rarity], c)
	}
	return pool, nil
}
