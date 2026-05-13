package service

// Phase 17 (UX-33) — admin-curated editorial collections.
//
// Thin wrapper over CollectionRepository plus:
//   - slug auto-generation on Create (kebab-case from Title, retry suffix
//     on unique-constraint collision)
//   - partial-update semantics on Update (apply only non-nil pointer
//     fields from the request).

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"regexp"
	"strings"

	liberrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/repo"
)

type CollectionService struct {
	repo *repo.CollectionRepository
	log  *logger.Logger
}

func NewCollectionService(r *repo.CollectionRepository, log *logger.Logger) *CollectionService {
	return &CollectionService{repo: r, log: log}
}

func (s *CollectionService) ListPublished(ctx context.Context, limit int) ([]*domain.Collection, error) {
	if limit <= 0 {
		limit = 12
	}
	if limit > 50 {
		limit = 50
	}
	return s.repo.ListPublished(ctx, limit)
}

func (s *CollectionService) GetBySlug(ctx context.Context, slug string) (*domain.Collection, error) {
	return s.repo.GetBySlug(ctx, slug)
}

func (s *CollectionService) ListAdmin(ctx context.Context) ([]*domain.Collection, error) {
	return s.repo.ListAdmin(ctx)
}

func (s *CollectionService) GetByID(ctx context.Context, id string) (*domain.Collection, error) {
	return s.repo.GetByID(ctx, id)
}

// Create persists a new collection. Slug auto-generates from Title when
// blank. Retries once with a 6-char random suffix on slug-uniqueness
// collision — defence-in-depth, admin can supply an explicit slug to
// avoid the retry path entirely.
func (s *CollectionService) Create(ctx context.Context, req *domain.CreateCollectionRequest, createdBy string) (*domain.Collection, error) {
	if req.Title == "" {
		return nil, liberrors.InvalidInput("title is required")
	}

	slug := strings.TrimSpace(req.Slug)
	if slug == "" {
		slug = slugify(req.Title)
		if slug == "" {
			// Title was all-symbols / non-alnum — fall back to a random
			// 8-char hex slug so we never insert with an empty slug.
			slug = "c-" + randomSuffix(8)
		}
	}

	c := &domain.Collection{
		Slug:          slug,
		Title:         req.Title,
		TitleRU:       req.TitleRU,
		TitleJP:       req.TitleJP,
		Description:   req.Description,
		DescriptionRU: req.DescriptionRU,
		DescriptionJP: req.DescriptionJP,
		CoverImageURL: req.CoverImageURL,
		Published:     req.Published,
		CreatedBy:     createdBy,
	}

	err := s.repo.Create(ctx, c)
	if err != nil && isUniqueViolation(err) {
		// Slug collision — append a random suffix and retry once.
		c.ID = "" // let Create regenerate
		c.Slug = slug + "-" + randomSuffix(6)
		if retryErr := s.repo.Create(ctx, c); retryErr != nil {
			return nil, retryErr
		}
		return c, nil
	}
	if err != nil {
		return nil, err
	}
	return c, nil
}

// Update applies only non-nil pointer fields from the request. The
// loaded row is mutated in place then saved — Save() always emits all
// columns, but only the ones we copied over change semantically.
func (s *CollectionService) Update(ctx context.Context, id string, req *domain.UpdateCollectionRequest) (*domain.Collection, error) {
	c, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if req.Slug != nil {
		c.Slug = strings.TrimSpace(*req.Slug)
	}
	if req.Title != nil {
		c.Title = *req.Title
	}
	if req.TitleRU != nil {
		c.TitleRU = *req.TitleRU
	}
	if req.TitleJP != nil {
		c.TitleJP = *req.TitleJP
	}
	if req.Description != nil {
		c.Description = *req.Description
	}
	if req.DescriptionRU != nil {
		c.DescriptionRU = *req.DescriptionRU
	}
	if req.DescriptionJP != nil {
		c.DescriptionJP = *req.DescriptionJP
	}
	if req.CoverImageURL != nil {
		c.CoverImageURL = *req.CoverImageURL
	}
	if req.Published != nil {
		c.Published = *req.Published
	}

	if err := s.repo.Update(ctx, c); err != nil {
		return nil, err
	}
	return c, nil
}

func (s *CollectionService) Delete(ctx context.Context, id string) error {
	return s.repo.Delete(ctx, id)
}

func (s *CollectionService) AddItem(ctx context.Context, collectionID string, req *domain.AddCollectionItemRequest) (*domain.CollectionItem, error) {
	if req.AnimeID == "" {
		return nil, liberrors.InvalidInput("anime_id is required")
	}
	// Defence-in-depth: ensure the collection exists before linking.
	if _, err := s.repo.GetByID(ctx, collectionID); err != nil {
		return nil, err
	}
	item := &domain.CollectionItem{
		CollectionID: collectionID,
		AnimeID:      req.AnimeID,
		SortOrder:    req.SortOrder,
	}
	if err := s.repo.AddItem(ctx, item); err != nil {
		return nil, err
	}
	return item, nil
}

func (s *CollectionService) RemoveItem(ctx context.Context, collectionID, animeID string) error {
	return s.repo.RemoveItem(ctx, collectionID, animeID)
}

// slugify produces a kebab-case slug from arbitrary text. Lowercase,
// strips non-alnum, collapses whitespace to '-'. Truncated at 80 chars.
var nonAlnumRE = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(title string) string {
	s := strings.ToLower(strings.TrimSpace(title))
	s = nonAlnumRE.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if len(s) > 80 {
		s = s[:80]
		s = strings.TrimRight(s, "-")
	}
	return s
}

func randomSuffix(n int) string {
	b := make([]byte, (n+1)/2)
	if _, err := rand.Read(b); err != nil {
		// Vanishingly unlikely; fall back to a static suffix rather than
		// fail the whole admin Create flow.
		return "fallback"
	}
	return hex.EncodeToString(b)[:n]
}

// isUniqueViolation reports whether the error came from a UNIQUE
// constraint violation. Recognises both Postgres ("duplicate key value")
// and SQLite ("UNIQUE constraint failed") error texts — matching against
// the wrapped %w chain via Error() text is good enough for the slug
// retry; the alternative (driver-specific error type assertions) would
// pull in pgconn for one branch.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate key") ||
		strings.Contains(msg, "unique constraint")
}
