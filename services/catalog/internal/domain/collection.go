package domain

import (
	"time"

	"gorm.io/gorm"
)

// Collection is an admin-curated set of anime ("Подборки") rendered as a
// Home row + a public /collections/:slug detail page. Phase 17 (UX-33).
type Collection struct {
	ID            string           `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Slug          string           `gorm:"size:120;uniqueIndex" json:"slug"`
	Title         string           `gorm:"size:200" json:"title"`
	TitleRU       string           `gorm:"size:200" json:"title_ru,omitempty"`
	TitleJP       string           `gorm:"size:200" json:"title_jp,omitempty"`
	Description   string           `gorm:"type:text" json:"description,omitempty"`
	DescriptionRU string           `gorm:"type:text" json:"description_ru,omitempty"`
	DescriptionJP string           `gorm:"type:text" json:"description_jp,omitempty"`
	CoverImageURL string           `gorm:"type:text" json:"cover_image_url,omitempty"`
	Published     bool             `gorm:"default:false;index" json:"published"`
	CreatedBy     string           `gorm:"type:uuid;index" json:"created_by,omitempty"`
	Items         []CollectionItem `gorm:"foreignKey:CollectionID" json:"items,omitempty"`
	// ItemCount is computed (not persisted) — populated by repo list methods
	// so list views render without a second round-trip per row.
	ItemCount int            `gorm:"-" json:"item_count"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// CollectionItem is one anime in a collection with admin-defined order.
type CollectionItem struct {
	ID           string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CollectionID string    `gorm:"type:uuid;index" json:"collection_id"`
	AnimeID      string    `gorm:"type:uuid;index" json:"anime_id"`
	Anime        *Anime    `gorm:"foreignKey:AnimeID" json:"anime,omitempty"`
	SortOrder    int       `gorm:"default:0;index" json:"sort_order"`
	CreatedAt    time.Time `json:"created_at"`
}

// CreateCollectionRequest is the admin POST body for /api/admin/collections.
// Slug is optional — auto-generated from Title when blank.
type CreateCollectionRequest struct {
	Slug          string `json:"slug"`
	Title         string `json:"title" validate:"required"`
	TitleRU       string `json:"title_ru"`
	TitleJP       string `json:"title_jp"`
	Description   string `json:"description"`
	DescriptionRU string `json:"description_ru"`
	DescriptionJP string `json:"description_jp"`
	CoverImageURL string `json:"cover_image_url"`
	Published     bool   `json:"published"`
}

// UpdateCollectionRequest applies only non-nil pointer fields — partial
// updates. PUT /api/admin/collections/:id.
type UpdateCollectionRequest struct {
	Slug          *string `json:"slug,omitempty"`
	Title         *string `json:"title,omitempty"`
	TitleRU       *string `json:"title_ru,omitempty"`
	TitleJP       *string `json:"title_jp,omitempty"`
	Description   *string `json:"description,omitempty"`
	DescriptionRU *string `json:"description_ru,omitempty"`
	DescriptionJP *string `json:"description_jp,omitempty"`
	CoverImageURL *string `json:"cover_image_url,omitempty"`
	Published     *bool   `json:"published,omitempty"`
}

// AddCollectionItemRequest is the admin POST body for /api/admin/collections/:id/items.
// Idempotent on (collection_id, anime_id) — repo upserts SortOrder.
type AddCollectionItemRequest struct {
	AnimeID   string `json:"anime_id" validate:"required"`
	SortOrder int    `json:"sort_order"`
}
