package domain

import (
	"time"

	"gorm.io/gorm"
)

// Comment is a single user comment on an anime detail page.
//
// Schema notes:
//   - composite index idx_comments_anime_created (anime_id, created_at DESC) — supports
//     ListByAnime newest-first.
//   - composite index idx_comments_user_created (user_id, created_at DESC) — supports
//     future "comments by user" view.
//   - ParentID is reserved for v1.0 threading; v0.1 always writes NULL.
//   - DeletedAt enables soft delete; GORM appends `WHERE deleted_at IS NULL` to reads.
//   - Username is denormalized onto the row (mirrors reviews.username) so list rendering
//     does not JOIN users on every request.
type Comment struct {
	ID        string         `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID    string         `gorm:"type:uuid;index:idx_comments_user_created" json:"user_id"`
	AnimeID   string         `gorm:"type:uuid;index:idx_comments_anime_created" json:"anime_id"`
	Username  string         `gorm:"size:32" json:"username"`
	Body      string         `gorm:"type:text" json:"body"`
	ParentID  *string        `gorm:"type:uuid" json:"parent_id,omitempty"`
	CreatedAt time.Time      `gorm:"index:idx_comments_anime_created,sort:desc;index:idx_comments_user_created,sort:desc" json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

func (Comment) TableName() string { return "comments" }

// CreateCommentRequest is the POST body for `POST /api/anime/:id/comments`.
//
// Pitfall guard (RESEARCH § Pitfall 8): this DTO intentionally OMITS the
// parent_id field. Threading is reserved for v1.0; allowing clients to pass
// the field today would create dangling rows the UI does not render. Server
// code never reads it from the request — it is server-owned and always NULL
// in v0.1.
type CreateCommentRequest struct {
	Body string `json:"body"`
}

// UpdateCommentRequest is the PATCH body for `PATCH /api/anime/:id/comments/:cid`.
type UpdateCommentRequest struct {
	Body string `json:"body"`
}

// CommentsListResponse is the GET response for `GET /api/anime/:id/comments`.
//
// Cursor pagination, newest-first; NextCursor is an opaque base64-encoded
// (created_at, id) tuple consumed by the client and passed back as ?cursor=.
type CommentsListResponse struct {
	Comments   []*Comment `json:"comments"`
	NextCursor string     `json:"next_cursor,omitempty"`
	HasMore    bool       `json:"has_more"`
}
