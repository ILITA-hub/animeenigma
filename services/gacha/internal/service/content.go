package service

import (
	"context"
	"time"

	apperrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/services/gacha/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/gacha/internal/repo"
)

// ContentService is the validation + orchestration layer for cards, groups, and banners.
type ContentService struct {
	cards   *repo.ContentRepository
	banners *repo.BannerRepository
}

func NewContentService(cards *repo.ContentRepository, banners *repo.BannerRepository) *ContentService {
	return &ContentService{cards: cards, banners: banners}
}

// ─── Cards ────────────────────────────────────────────────────────────────────

// CreateCardRequest carries the fields for creating a new card.
type CreateCardRequest struct {
	Name        string        `json:"name"`
	SourceTitle string        `json:"source_title"`
	ImagePath   string        `json:"image_path"`
	BackPath    string        `json:"back_path"`
	Rarity      domain.Rarity `json:"rarity"`
	Enabled     bool          `json:"enabled"`
	GroupIDs    []string      `json:"group_ids"`
}

// UpdateCardRequest carries the fields for updating an existing card.
type UpdateCardRequest struct {
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	SourceTitle string        `json:"source_title"`
	ImagePath   string        `json:"image_path"`
	BackPath    string        `json:"back_path"`
	Rarity      domain.Rarity `json:"rarity"`
	Enabled     bool          `json:"enabled"`
	GroupIDs    []string      `json:"group_ids"`
}

// CreateCard validates the request and creates a new card.
func (s *ContentService) CreateCard(ctx context.Context, req CreateCardRequest) (*domain.Card, error) {
	if req.Name == "" {
		return nil, apperrors.InvalidInput("card name is required")
	}
	if !domain.ValidRarity(req.Rarity) {
		return nil, apperrors.InvalidInput("invalid rarity: must be N, R, SR, or SSR")
	}
	if req.ImagePath == "" {
		return nil, apperrors.InvalidInput("card image_path is required")
	}

	c := &domain.Card{
		Name:        req.Name,
		SourceTitle: req.SourceTitle,
		ImagePath:   req.ImagePath,
		BackPath:    req.BackPath,
		Rarity:      req.Rarity,
		Enabled:     req.Enabled,
	}
	if err := s.cards.CreateCard(ctx, c); err != nil {
		return nil, err
	}
	// Associate with groups if requested
	for _, gid := range req.GroupIDs {
		if err := s.cards.AddCardsToGroup(ctx, gid, []string{c.ID}); err != nil {
			return nil, err
		}
	}
	return c, nil
}

// UpdateCard validates and applies updates to an existing card.
func (s *ContentService) UpdateCard(ctx context.Context, req UpdateCardRequest) (*domain.Card, error) {
	if req.Name == "" {
		return nil, apperrors.InvalidInput("card name is required")
	}
	if !domain.ValidRarity(req.Rarity) {
		return nil, apperrors.InvalidInput("invalid rarity: must be N, R, SR, or SSR")
	}
	if req.ImagePath == "" {
		return nil, apperrors.InvalidInput("card image_path is required")
	}

	c, err := s.cards.GetCard(ctx, req.ID)
	if err != nil {
		return nil, err
	}
	c.Name = req.Name
	c.SourceTitle = req.SourceTitle
	c.ImagePath = req.ImagePath
	c.BackPath = req.BackPath
	c.Rarity = req.Rarity
	c.Enabled = req.Enabled

	if err := s.cards.UpdateCard(ctx, c); err != nil {
		return nil, err
	}
	return c, nil
}

// GetCard returns a card by ID.
func (s *ContentService) GetCard(ctx context.Context, id string) (*domain.Card, error) {
	return s.cards.GetCard(ctx, id)
}

// DeleteCard soft-deletes a card.
func (s *ContentService) DeleteCard(ctx context.Context, id string) error {
	return s.cards.DeleteCard(ctx, id)
}

// ListCards returns cards matching the filter.
func (s *ContentService) ListCards(ctx context.Context, f repo.CardFilter) ([]domain.Card, error) {
	return s.cards.ListCards(ctx, f)
}

// ─── Groups ───────────────────────────────────────────────────────────────────

// CreateGroup creates a new named group.
func (s *ContentService) CreateGroup(ctx context.Context, name string) (*domain.Group, error) {
	if name == "" {
		return nil, apperrors.InvalidInput("group name is required")
	}
	g := &domain.Group{Name: name}
	if err := s.cards.CreateGroup(ctx, g); err != nil {
		return nil, err
	}
	return g, nil
}

// ListGroups returns all groups.
func (s *ContentService) ListGroups(ctx context.Context) ([]domain.Group, error) {
	return s.cards.ListGroups(ctx)
}

// RenameGroup renames the group with the given ID.
func (s *ContentService) RenameGroup(ctx context.Context, id, name string) error {
	if name == "" {
		return apperrors.InvalidInput("group name is required")
	}
	return s.cards.RenameGroup(ctx, id, name)
}

// DeleteGroup removes the group and all its join rows.
func (s *ContentService) DeleteGroup(ctx context.Context, id string) error {
	return s.cards.DeleteGroup(ctx, id)
}

// AddCardsToGroup adds cards to a group (idempotent).
func (s *ContentService) AddCardsToGroup(ctx context.Context, groupID string, cardIDs []string) error {
	return s.cards.AddCardsToGroup(ctx, groupID, cardIDs)
}

// RemoveCardFromGroup removes a card from a group.
func (s *ContentService) RemoveCardFromGroup(ctx context.Context, groupID, cardID string) error {
	return s.cards.RemoveCardFromGroup(ctx, groupID, cardID)
}

// GroupCardIDs returns the card IDs for a group.
func (s *ContentService) GroupCardIDs(ctx context.Context, groupID string) ([]string, error) {
	return s.cards.GroupCardIDs(ctx, groupID)
}

// ─── Banners ──────────────────────────────────────────────────────────────────

// CreateBannerRequest carries the fields for creating a new banner.
type CreateBannerRequest struct {
	Name         string     `json:"name"`
	Description  string     `json:"description"`
	BackdropPath string     `json:"backdrop_path"`
	IsStandard   bool       `json:"is_standard"`
	Enabled      bool       `json:"enabled"`
	ActiveFrom   *time.Time `json:"active_from,omitempty"`
	ActiveTo     *time.Time `json:"active_to,omitempty"`
	SortOrder    int        `json:"sort_order"`
}

// UpdateBannerRequest carries the fields for updating a banner.
type UpdateBannerRequest struct {
	ID           string     `json:"id"`
	Name         string     `json:"name"`
	Description  string     `json:"description"`
	BackdropPath string     `json:"backdrop_path"`
	IsStandard   bool       `json:"is_standard"`
	Enabled      bool       `json:"enabled"`
	ActiveFrom   *time.Time `json:"active_from,omitempty"`
	ActiveTo     *time.Time `json:"active_to,omitempty"`
	SortOrder    int        `json:"sort_order"`
}

func validateBannerWindow(activeFrom, activeTo *time.Time) error {
	if activeFrom != nil && activeTo != nil && !activeTo.After(*activeFrom) {
		return apperrors.InvalidInput("active_to must be after active_from")
	}
	return nil
}

// CreateBanner validates the request and creates a new banner.
func (s *ContentService) CreateBanner(ctx context.Context, req CreateBannerRequest) (*domain.Banner, error) {
	if req.Name == "" {
		return nil, apperrors.InvalidInput("banner name is required")
	}
	if err := validateBannerWindow(req.ActiveFrom, req.ActiveTo); err != nil {
		return nil, err
	}

	b := &domain.Banner{
		Name:         req.Name,
		Description:  req.Description,
		BackdropPath: req.BackdropPath,
		IsStandard:   req.IsStandard,
		Enabled:      req.Enabled,
		ActiveFrom:   req.ActiveFrom,
		ActiveTo:     req.ActiveTo,
		SortOrder:    req.SortOrder,
	}
	if err := s.banners.CreateBanner(ctx, b); err != nil {
		return nil, err
	}
	return b, nil
}

// UpdateBanner validates and applies updates to an existing banner.
func (s *ContentService) UpdateBanner(ctx context.Context, req UpdateBannerRequest) (*domain.Banner, error) {
	if req.Name == "" {
		return nil, apperrors.InvalidInput("banner name is required")
	}
	if err := validateBannerWindow(req.ActiveFrom, req.ActiveTo); err != nil {
		return nil, err
	}

	b, err := s.banners.GetBanner(ctx, req.ID)
	if err != nil {
		return nil, err
	}
	b.Name = req.Name
	b.Description = req.Description
	b.BackdropPath = req.BackdropPath
	b.IsStandard = req.IsStandard
	b.Enabled = req.Enabled
	b.ActiveFrom = req.ActiveFrom
	b.ActiveTo = req.ActiveTo
	b.SortOrder = req.SortOrder

	if err := s.banners.UpdateBanner(ctx, b); err != nil {
		return nil, err
	}
	return b, nil
}

// GetBanner returns a banner by ID.
func (s *ContentService) GetBanner(ctx context.Context, id string) (*domain.Banner, error) {
	return s.banners.GetBanner(ctx, id)
}

// DeleteBanner soft-deletes a banner.
func (s *ContentService) DeleteBanner(ctx context.Context, id string) error {
	return s.banners.DeleteBanner(ctx, id)
}

// ListBanners returns all banners (admin view).
func (s *ContentService) ListBanners(ctx context.Context) ([]domain.Banner, error) {
	return s.banners.ListBanners(ctx)
}

// SetBannerCards atomically replaces the card pool of a banner.
func (s *ContentService) SetBannerCards(ctx context.Context, bannerID string, cardIDs []string) error {
	return s.banners.SetCards(ctx, bannerID, cardIDs)
}

// AddBannerCards appends cards to the banner's pool (idempotent).
func (s *ContentService) AddBannerCards(ctx context.Context, bannerID string, cardIDs []string) error {
	return s.banners.AddCards(ctx, bannerID, cardIDs)
}

// AddGroupCardsToBanner copies all group cards into a banner's pool.
func (s *ContentService) AddGroupCardsToBanner(ctx context.Context, bannerID, groupID string) error {
	return s.banners.AddGroupCards(ctx, bannerID, groupID)
}

// BannerCardIDs returns the card IDs in a banner's pool.
func (s *ContentService) BannerCardIDs(ctx context.Context, bannerID string) ([]string, error) {
	return s.banners.BannerCardIDs(ctx, bannerID)
}
