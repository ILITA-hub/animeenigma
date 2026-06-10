package repo

import (
	"context"
	"fmt"
	"time"

	apperrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/services/gacha/internal/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// BannerRepository handles banners and their card pool operations.
type BannerRepository struct {
	db *gorm.DB
}

func NewBannerRepository(db *gorm.DB) *BannerRepository { return &BannerRepository{db: db} }

// CreateBanner inserts a new banner.
func (r *BannerRepository) CreateBanner(ctx context.Context, b *domain.Banner) error {
	return r.db.WithContext(ctx).Create(b).Error
}

// GetBanner returns the banner with the given ID. Returns apperrors.NotFound if missing or soft-deleted.
func (r *BannerRepository) GetBanner(ctx context.Context, id string) (*domain.Banner, error) {
	var b domain.Banner
	err := r.db.WithContext(ctx).First(&b, "id = ?", id).Error
	if err == gorm.ErrRecordNotFound {
		return nil, apperrors.NotFound(fmt.Sprintf("banner %s", id))
	}
	return &b, err
}

// UpdateBanner saves all fields of the banner by ID.
func (r *BannerRepository) UpdateBanner(ctx context.Context, b *domain.Banner) error {
	res := r.db.WithContext(ctx).Save(b)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return apperrors.NotFound(fmt.Sprintf("banner %s", b.ID))
	}
	return nil
}

// DeleteBanner soft-deletes the banner.
func (r *BannerRepository) DeleteBanner(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Delete(&domain.Banner{}, "id = ?", id).Error
}

// ListBanners returns all banners (admin view), ordered by created_at DESC.
// Soft-deleted banners are excluded by GORM.
func (r *BannerRepository) ListBanners(ctx context.Context) ([]domain.Banner, error) {
	var banners []domain.Banner
	err := r.db.WithContext(ctx).Order("created_at DESC").Find(&banners).Error
	return banners, err
}

// SetCards atomically replaces the full card pool for a banner.
// Duplicate card IDs in the input are silently deduplicated (ON CONFLICT DO NOTHING).
func (r *BannerRepository) SetCards(ctx context.Context, bannerID string, cardIDs []string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("banner_id = ?", bannerID).Delete(&domain.BannerCard{}).Error; err != nil {
			return err
		}
		if len(cardIDs) == 0 {
			return nil
		}
		rows := make([]domain.BannerCard, len(cardIDs))
		for i, cid := range cardIDs {
			rows[i] = domain.BannerCard{BannerID: bannerID, CardID: cid}
		}
		return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&rows).Error
	})
}

// AddCards appends cards to the banner's pool, ignoring duplicates.
func (r *BannerRepository) AddCards(ctx context.Context, bannerID string, cardIDs []string) error {
	if len(cardIDs) == 0 {
		return nil
	}
	rows := make([]domain.BannerCard, len(cardIDs))
	for i, cid := range cardIDs {
		rows[i] = domain.BannerCard{BannerID: bannerID, CardID: cid}
	}
	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(&rows).Error
}

// AddGroupCards copies all non-soft-deleted cards from a group into the banner
// pool using an INSERT...SELECT, ignoring duplicates.
func (r *BannerRepository) AddGroupCards(ctx context.Context, bannerID, groupID string) error {
	return r.db.WithContext(ctx).Exec(
		`INSERT INTO gacha_banner_cards (banner_id, card_id)
		 SELECT ?, cg.card_id
		 FROM gacha_card_groups cg
		 JOIN gacha_cards c ON c.id = cg.card_id AND c.deleted_at IS NULL
		 WHERE cg.group_id = ?
		 ON CONFLICT DO NOTHING`,
		bannerID, groupID,
	).Error
}

// BannerCardIDs returns the IDs of all non-soft-deleted cards in the banner's pool.
func (r *BannerRepository) BannerCardIDs(ctx context.Context, bannerID string) ([]string, error) {
	var cardIDs []string
	err := r.db.WithContext(ctx).
		Model(&domain.BannerCard{}).
		Select("gacha_banner_cards.card_id").
		Joins("JOIN gacha_cards ON gacha_cards.id = gacha_banner_cards.card_id AND gacha_cards.deleted_at IS NULL").
		Where("gacha_banner_cards.banner_id = ?", bannerID).
		Pluck("gacha_banner_cards.card_id", &cardIDs).Error
	if err != nil {
		return nil, err
	}
	if cardIDs == nil {
		cardIDs = []string{}
	}
	return cardIDs, nil
}

// ActiveNow returns banners visible to players at the given time: enabled AND
// (active_from IS NULL OR active_from <= now) AND (active_to IS NULL OR active_to >= now).
// Ordered: is_standard DESC, sort_order ASC, created_at ASC.
// The `now` parameter is explicit for deterministic tests.
func (r *BannerRepository) ActiveNow(ctx context.Context, now time.Time) ([]domain.Banner, error) {
	var banners []domain.Banner
	err := r.db.WithContext(ctx).
		Where("enabled = ?", true).
		Where("(active_from IS NULL OR active_from <= ?)", now).
		Where("(active_to IS NULL OR active_to >= ?)", now).
		Order("is_standard DESC, sort_order ASC, created_at ASC").
		Find(&banners).Error
	return banners, err
}
