package repo

import (
	"context"
	"fmt"

	apperrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/services/gacha/internal/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ContentRepository handles cards and groups for the gacha content domain.
type ContentRepository struct {
	db *gorm.DB
}

func NewContentRepository(db *gorm.DB) *ContentRepository { return &ContentRepository{db: db} }

// CardFilter describes optional filters for ListCards.
type CardFilter struct {
	Rarity  domain.Rarity // "" = any
	Enabled *bool         // nil = any
	GroupID string        // "" = any; joins gacha_card_groups
}

// CreateCard inserts a new card, setting the ID on the struct if not set.
func (r *ContentRepository) CreateCard(ctx context.Context, c *domain.Card) error {
	return r.db.WithContext(ctx).Create(c).Error
}

// GetCard returns a card by ID. Returns apperrors.NotFound if missing or soft-deleted.
func (r *ContentRepository) GetCard(ctx context.Context, id string) (*domain.Card, error) {
	var c domain.Card
	err := r.db.WithContext(ctx).First(&c, "id = ?", id).Error
	if err == gorm.ErrRecordNotFound {
		return nil, apperrors.NotFound(fmt.Sprintf("card %s", id))
	}
	return &c, err
}

// UpdateCard saves all fields of the card by ID.
func (r *ContentRepository) UpdateCard(ctx context.Context, c *domain.Card) error {
	res := r.db.WithContext(ctx).Save(c)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return apperrors.NotFound(fmt.Sprintf("card %s", c.ID))
	}
	return nil
}

// DeleteCard soft-deletes the card with the given ID.
func (r *ContentRepository) DeleteCard(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Delete(&domain.Card{}, "id = ?", id).Error
}

// ListCards returns cards matching the filter, ordered by created_at DESC.
// Soft-deleted cards are excluded automatically by GORM.
func (r *ContentRepository) ListCards(ctx context.Context, f CardFilter) ([]domain.Card, error) {
	q := r.db.WithContext(ctx).Model(&domain.Card{})

	if f.GroupID != "" {
		q = q.Joins("JOIN gacha_card_groups cg ON cg.card_id = gacha_cards.id AND cg.group_id = ?", f.GroupID)
	}
	if f.Rarity != "" {
		q = q.Where("gacha_cards.rarity = ?", f.Rarity)
	}
	if f.Enabled != nil {
		q = q.Where("gacha_cards.enabled = ?", *f.Enabled)
	}

	var cards []domain.Card
	err := q.Order("gacha_cards.created_at DESC").Find(&cards).Error
	return cards, err
}

// CreateGroup inserts a new group.
func (r *ContentRepository) CreateGroup(ctx context.Context, g *domain.Group) error {
	return r.db.WithContext(ctx).Create(g).Error
}

// ListGroups returns all groups.
func (r *ContentRepository) ListGroups(ctx context.Context) ([]domain.Group, error) {
	var groups []domain.Group
	err := r.db.WithContext(ctx).Order("created_at DESC").Find(&groups).Error
	return groups, err
}

// RenameGroup updates the name of the group with the given ID.
func (r *ContentRepository) RenameGroup(ctx context.Context, id, name string) error {
	res := r.db.WithContext(ctx).Model(&domain.Group{}).Where("id = ?", id).Update("name", name)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return apperrors.NotFound(fmt.Sprintf("group %s", id))
	}
	return nil
}

// DeleteGroup removes the group and all its membership join rows in a transaction.
func (r *ContentRepository) DeleteGroup(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("group_id = ?", id).Delete(&domain.CardGroup{}).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", id).Delete(&domain.Group{}).Error
	})
}

// AddCardsToGroup inserts (group_id, card_id) join rows, ignoring duplicates.
func (r *ContentRepository) AddCardsToGroup(ctx context.Context, groupID string, cardIDs []string) error {
	if len(cardIDs) == 0 {
		return nil
	}
	rows := make([]domain.CardGroup, len(cardIDs))
	for i, cid := range cardIDs {
		rows[i] = domain.CardGroup{GroupID: groupID, CardID: cid}
	}
	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(&rows).Error
}

// RemoveCardFromGroup deletes the (groupID, cardID) join row.
func (r *ContentRepository) RemoveCardFromGroup(ctx context.Context, groupID, cardID string) error {
	return r.db.WithContext(ctx).
		Where("group_id = ? AND card_id = ?", groupID, cardID).
		Delete(&domain.CardGroup{}).Error
}

// GroupCardIDs returns the IDs of all non-soft-deleted cards in the group.
func (r *ContentRepository) GroupCardIDs(ctx context.Context, groupID string) ([]string, error) {
	var cardIDs []string
	err := r.db.WithContext(ctx).
		Model(&domain.CardGroup{}).
		Select("gacha_card_groups.card_id").
		Joins("JOIN gacha_cards ON gacha_cards.id = gacha_card_groups.card_id AND gacha_cards.deleted_at IS NULL").
		Where("gacha_card_groups.group_id = ?", groupID).
		Pluck("gacha_card_groups.card_id", &cardIDs).Error
	if err != nil {
		return nil, err
	}
	if cardIDs == nil {
		cardIDs = []string{}
	}
	return cardIDs, nil
}

// CardBulkSet carries the optional fields BulkUpdateCards applies.
// nil pointer = leave that column unchanged.
type CardBulkSet struct {
	Name        *string
	SourceTitle *string
	Rarity      *domain.Rarity
	Enabled     *bool
}

// BulkUpdateCards applies the non-nil fields of set to every card in ids.
// Soft-deleted cards are excluded by GORM's default scope; returns rows affected.
func (r *ContentRepository) BulkUpdateCards(ctx context.Context, ids []string, set CardBulkSet) (int64, error) {
	updates := map[string]any{}
	if set.Name != nil {
		updates["name"] = *set.Name
	}
	if set.SourceTitle != nil {
		updates["source_title"] = *set.SourceTitle
	}
	if set.Rarity != nil {
		updates["rarity"] = *set.Rarity
	}
	if set.Enabled != nil {
		updates["enabled"] = *set.Enabled
	}
	res := r.db.WithContext(ctx).Model(&domain.Card{}).Where("id IN ?", ids).Updates(updates)
	return res.RowsAffected, res.Error
}

// BulkDeleteCards soft-deletes every card in ids. Same semantics as DeleteCard:
// group/banner join rows stay in place — every join query already filters on
// gacha_cards.deleted_at. Returns rows affected.
func (r *ContentRepository) BulkDeleteCards(ctx context.Context, ids []string) (int64, error) {
	res := r.db.WithContext(ctx).Delete(&domain.Card{}, "id IN ?", ids)
	return res.RowsAffected, res.Error
}
