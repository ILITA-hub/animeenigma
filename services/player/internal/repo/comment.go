package repo

import (
	"context"

	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/pagination"
	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"gorm.io/gorm"
)

// Wave-0 scaffold: this file exists so handler/service/test code in waves 1-3 can
// compile against the documented method surface. Every method returns
// CodeUnavailable until plan 03 fills the bodies.
//
// _ keeps the pagination import referenced — plan 03 will use pagination.Cursor
// for opaque base64 (created_at, id) cursor encoding in ListByAnime.
var _ = pagination.Cursor{}

// CommentRepository is the data-access layer for the comments table.
type CommentRepository struct {
	db *gorm.DB
}

// NewCommentRepository wires a CommentRepository against the provided GORM handle.
func NewCommentRepository(db *gorm.DB) *CommentRepository {
	return &CommentRepository{db: db}
}

// Create inserts a new comment row.
func (r *CommentRepository) Create(ctx context.Context, c *domain.Comment) error {
	return errors.New(errors.CodeUnavailable, "comment repo Create: not implemented")
}

// GetByID returns a single comment by primary key (soft-deleted rows excluded).
func (r *CommentRepository) GetByID(ctx context.Context, id string) (*domain.Comment, error) {
	return nil, errors.New(errors.CodeUnavailable, "comment repo GetByID: not implemented")
}

// ListByAnime returns up to `limit` comments for the anime ordered newest-first.
// The optional `cursor` is the opaque base64-encoded (created_at, id) tuple
// returned by a previous call.
func (r *CommentRepository) ListByAnime(ctx context.Context, animeID, cursor string, limit int) (comments []*domain.Comment, nextCursor string, err error) {
	return nil, "", errors.New(errors.CodeUnavailable, "comment repo ListByAnime: not implemented")
}

// Update mutates the body of an existing comment.
func (r *CommentRepository) Update(ctx context.Context, id, body string) error {
	return errors.New(errors.CodeUnavailable, "comment repo Update: not implemented")
}

// SoftDelete sets deleted_at on the row; ListByAnime / GetByID exclude it afterwards.
func (r *CommentRepository) SoftDelete(ctx context.Context, id string) error {
	return errors.New(errors.CodeUnavailable, "comment repo SoftDelete: not implemented")
}
