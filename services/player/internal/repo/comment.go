package repo

import (
	"context"
	stderrors "errors"

	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/pagination"
	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"gorm.io/gorm"
)

// CommentRepository is the data-access layer for the comments table.
type CommentRepository struct {
	db *gorm.DB
}

// NewCommentRepository wires a CommentRepository against the provided GORM handle.
func NewCommentRepository(db *gorm.DB) *CommentRepository {
	return &CommentRepository{db: db}
}

// Create inserts a new comment row. The `id` column has a Postgres
// `gen_random_uuid()` default; tests running against SQLite must set
// c.ID explicitly. CreatedAt / UpdatedAt are populated by the DB's
// `default:now()` on Postgres. On SQLite (tests) they are filled by
// GORM at INSERT time.
func (r *CommentRepository) Create(ctx context.Context, c *domain.Comment) error {
	if err := r.db.WithContext(ctx).Create(c).Error; err != nil {
		return errors.Wrap(err, errors.CodeInternal, "failed to insert comment")
	}
	return nil
}

// GetByID returns a single comment by primary key. The gorm.DeletedAt
// soft-delete filter auto-excludes rows with deleted_at IS NOT NULL.
func (r *CommentRepository) GetByID(ctx context.Context, id string) (*domain.Comment, error) {
	var c domain.Comment
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&c).Error
	if err != nil {
		if stderrors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.NotFound("comment")
		}
		return nil, errors.Wrap(err, errors.CodeInternal, "failed to load comment")
	}
	return &c, nil
}

// ListByAnime returns up to `limit` comments for the anime ordered
// newest-first. The optional `cursor` is the opaque base64-encoded
// (created_at, id) tuple returned by a previous call.
//
// Pagination strategy: query `Limit(limit + 1)`. If len > limit, drop
// the extra and emit a fresh cursor pointing at the last visible row.
// gorm.DeletedAt on the struct auto-injects `WHERE deleted_at IS NULL`
// so soft-deleted rows never appear.
func (r *CommentRepository) ListByAnime(ctx context.Context, animeID, cursor string, limit int) (comments []*domain.Comment, nextCursor string, err error) {
	if limit <= 0 {
		limit = 50
	}

	q := r.db.WithContext(ctx).
		Where("anime_id = ?", animeID).
		Order("created_at DESC, id DESC").
		Limit(limit + 1)

	if cursor != "" {
		cur, decErr := pagination.DecodeCursor(cursor)
		if decErr != nil {
			return nil, "", errors.InvalidInput("invalid cursor")
		}
		if cur != nil {
			// (created_at, id) < (cursor.Timestamp, cursor.ID) — strict
			// lexicographic comparison so a row with the cursor's ID is
			// not re-emitted on the next page.
			q = q.Where(
				"created_at < ? OR (created_at = ? AND id < ?)",
				cur.Timestamp, cur.Timestamp, cur.ID,
			)
		}
	}

	if err := q.Find(&comments).Error; err != nil {
		return nil, "", errors.Wrap(err, errors.CodeInternal, "failed to list comments")
	}

	if len(comments) > limit {
		comments = comments[:limit]
		last := comments[len(comments)-1]
		nextCursor = pagination.Cursor{ID: last.ID, Timestamp: last.CreatedAt}.Encode()
	}
	return comments, nextCursor, nil
}

// Update mutates the body of an existing comment. Returns errors.NotFound
// when no live (non-soft-deleted) row matches.
//
// REVIEW.md WR-01: GORM's automatic `WHERE deleted_at IS NULL` filter is
// NOT applied to `Model(...).Where(...).Update(...)` the way it is to
// First/Find/Delete. Explicitly add the soft-delete predicate so a
// soft-deleted comment row cannot have its body silently mutated by a
// caller that still holds its UUID. Defence-in-depth — service layer
// already gates on GetByID which respects soft-delete.
func (r *CommentRepository) Update(ctx context.Context, id, body string) error {
	res := r.db.WithContext(ctx).
		Model(&domain.Comment{}).
		Where("id = ? AND deleted_at IS NULL", id).
		Update("body", body)
	if res.Error != nil {
		return errors.Wrap(res.Error, errors.CodeInternal, "failed to update comment")
	}
	if res.RowsAffected == 0 {
		return errors.NotFound("comment")
	}
	return nil
}

// SoftDelete sets deleted_at on the row via gorm.DeletedAt — GORM
// converts Delete() into UPDATE deleted_at = NOW() because the struct
// has a gorm.DeletedAt field. Idempotent on missing rows.
func (r *CommentRepository) SoftDelete(ctx context.Context, id string) error {
	if err := r.db.WithContext(ctx).
		Where("id = ?", id).
		Delete(&domain.Comment{}).Error; err != nil {
		return errors.Wrap(err, errors.CodeInternal, "failed to soft-delete comment")
	}
	return nil
}
