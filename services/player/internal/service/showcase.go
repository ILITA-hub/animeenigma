package service

import (
	"context"
	"encoding/json"
	"sort"

	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/player/internal/repo"
)

// ShowcaseService is the business layer for the profile showcase. It is a
// pure config store: validation is structural only and no content is
// resolved here (the frontend resolves posters/characters/cards/stats).
type ShowcaseService struct {
	repo *repo.ShowcaseRepository
	log  *logger.Logger
}

func NewShowcaseService(r *repo.ShowcaseRepository, log *logger.Logger) *ShowcaseService {
	return &ShowcaseService{repo: r, log: log}
}

// GetShowcase returns the user's blocks sorted by Order ascending plus the
// stored `enabled` (published) flag.
func (s *ShowcaseService) GetShowcase(ctx context.Context, userID string) ([]domain.Block, bool, error) {
	row, err := s.repo.Get(ctx, userID)
	if err != nil {
		return nil, false, err
	}
	var blocks []domain.Block
	if row.Blocks != "" && row.Blocks != "[]" {
		if err := json.Unmarshal([]byte(row.Blocks), &blocks); err != nil {
			// Corrupt config should not 500 the public profile — log + return empty.
			s.log.Errorw("failed to parse showcase blocks", "user_id", userID, "error", err)
			return []domain.Block{}, row.Enabled, nil
		}
	}
	sort.SliceStable(blocks, func(i, j int) bool { return blocks[i].Order < blocks[j].Order })
	return blocks, row.Enabled, nil
}

// SaveShowcase validates, re-numbers Order to the array index, and persists.
// Blocks are sorted by their incoming Order before re-numbering, so the
// caller's intended sequence is preserved canonically.
//
// Coerce rule: an empty showcase can never be enabled, so the stored/returned
// `enabled` is `enabled && len(blocks) > 0` — keeping the visible ⟹ non-empty
// invariant. The coerced value is returned so the handler can echo the
// authoritative state.
func (s *ShowcaseService) SaveShowcase(ctx context.Context, userID string, blocks []domain.Block, enabled bool) (bool, error) {
	if err := domain.ValidateBlocks(blocks); err != nil {
		return false, err
	}
	sort.SliceStable(blocks, func(i, j int) bool { return blocks[i].Order < blocks[j].Order })
	for i := range blocks {
		blocks[i].Order = i
	}
	enabled = enabled && len(blocks) > 0
	encoded, err := json.Marshal(blocks)
	if err != nil {
		return false, errors.Wrap(err, errors.CodeInternal, "failed to encode showcase blocks")
	}
	if err := s.repo.Upsert(ctx, userID, string(encoded), enabled); err != nil {
		return false, err
	}
	return enabled, nil
}
