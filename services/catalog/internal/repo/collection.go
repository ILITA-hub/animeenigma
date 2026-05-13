package repo

import (
	"context"
	"errors"
	"fmt"

	liberrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// CollectionRepository persists admin-curated Collections and their
// CollectionItem rows. Phase 17 (UX-33).
type CollectionRepository struct {
	db *gorm.DB
}

func NewCollectionRepository(db *gorm.DB) *CollectionRepository {
	return &CollectionRepository{db: db}
}

// ListPublished returns published collections ordered by CreatedAt DESC.
// Drafts and soft-deleted rows are excluded by definition. ItemCount is
// populated via a per-row COUNT — collections are a low-cardinality
// curated table (<100 rows ever), so the loop overhead is negligible.
func (r *CollectionRepository) ListPublished(ctx context.Context, limit int) ([]*domain.Collection, error) {
	if limit <= 0 {
		limit = 12
	}
	var collections []*domain.Collection
	if err := r.db.WithContext(ctx).
		Where("published = ?", true).
		Order("created_at DESC").
		Limit(limit).
		Find(&collections).Error; err != nil {
		return nil, fmt.Errorf("list published collections: %w", err)
	}
	if err := r.populateItemCounts(ctx, collections); err != nil {
		return nil, err
	}
	return collections, nil
}

// GetBySlug returns the published, non-deleted collection at the slug,
// with all items preloaded (Items.Anime) and sorted by SortOrder ASC.
// Drafts return NotFound — drafts are admin-only via GetByID/ListAdmin.
func (r *CollectionRepository) GetBySlug(ctx context.Context, slug string) (*domain.Collection, error) {
	var collection domain.Collection
	err := r.db.WithContext(ctx).
		Preload("Items", func(db *gorm.DB) *gorm.DB {
			return db.Order("sort_order ASC, created_at ASC")
		}).
		Preload("Items.Anime").
		Where("slug = ? AND published = ?", slug, true).
		First(&collection).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, liberrors.NotFound("collection")
		}
		return nil, fmt.Errorf("get collection by slug: %w", err)
	}
	collection.ItemCount = len(collection.Items)
	return &collection, nil
}

// ListAdmin returns drafts + published, ordered by UpdatedAt DESC.
func (r *CollectionRepository) ListAdmin(ctx context.Context) ([]*domain.Collection, error) {
	var collections []*domain.Collection
	if err := r.db.WithContext(ctx).
		Order("updated_at DESC").
		Find(&collections).Error; err != nil {
		return nil, fmt.Errorf("list admin collections: %w", err)
	}
	if err := r.populateItemCounts(ctx, collections); err != nil {
		return nil, err
	}
	return collections, nil
}

// GetByID returns the collection by ID with all items preloaded
// (Items.Anime), sorted by SortOrder ASC. Admin-only — returns drafts.
func (r *CollectionRepository) GetByID(ctx context.Context, id string) (*domain.Collection, error) {
	var collection domain.Collection
	err := r.db.WithContext(ctx).
		Preload("Items", func(db *gorm.DB) *gorm.DB {
			return db.Order("sort_order ASC, created_at ASC")
		}).
		Preload("Items.Anime").
		Where("id = ?", id).
		First(&collection).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, liberrors.NotFound("collection")
		}
		return nil, fmt.Errorf("get collection by id: %w", err)
	}
	collection.ItemCount = len(collection.Items)
	return &collection, nil
}

// Create persists a new collection row. The caller is expected to have
// populated Slug + Title; service layer handles slug auto-generation.
// ID is generated at the Go level when blank — Postgres's
// gen_random_uuid() default works in prod, but generating here keeps the
// repo portable to SQLite tests and self-contained.
func (r *CollectionRepository) Create(ctx context.Context, c *domain.Collection) error {
	if c.ID == "" {
		c.ID = uuid.NewString()
	}
	if err := r.db.WithContext(ctx).Create(c).Error; err != nil {
		return fmt.Errorf("create collection: %w", err)
	}
	return nil
}

// Update saves the collection. Caller controls which fields were mutated.
func (r *CollectionRepository) Update(ctx context.Context, c *domain.Collection) error {
	result := r.db.WithContext(ctx).Save(c)
	if result.Error != nil {
		return fmt.Errorf("update collection: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return liberrors.NotFound("collection")
	}
	return nil
}

// Delete soft-deletes the collection (gorm.DeletedAt). Items remain in
// the DB but the collection no longer appears in any list/get path.
func (r *CollectionRepository) Delete(ctx context.Context, id string) error {
	result := r.db.WithContext(ctx).Delete(&domain.Collection{}, "id = ?", id)
	if result.Error != nil {
		return fmt.Errorf("delete collection: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return liberrors.NotFound("collection")
	}
	return nil
}

// AddItem upserts (collection_id, anime_id) — the second call with the
// same pair updates SortOrder rather than inserting a duplicate. Returns
// the freshly-loaded row so callers can echo it back.
func (r *CollectionRepository) AddItem(ctx context.Context, item *domain.CollectionItem) error {
	// Look up existing (collection_id, anime_id) pair. If present, update
	// its sort_order in-place; otherwise insert. This is portable across
	// Postgres and SQLite (clause.OnConflict requires a unique constraint
	// that we don't model on CollectionItem to keep the schema simple).
	var existing domain.CollectionItem
	err := r.db.WithContext(ctx).
		Where("collection_id = ? AND anime_id = ?", item.CollectionID, item.AnimeID).
		First(&existing).Error
	if err == nil {
		// Update sort_order on the existing row. Explicit WHERE clause —
		// GORM 1.30 requires a primary-key WHERE for raw map updates so
		// it doesn't auto-resolve from struct fields.
		if err := r.db.WithContext(ctx).Model(&domain.CollectionItem{}).
			Where("id = ?", existing.ID).
			Updates(map[string]interface{}{"sort_order": item.SortOrder}).Error; err != nil {
			return fmt.Errorf("update collection item: %w", err)
		}
		// Re-read to capture the updated sort_order and echo back via the
		// caller's pointer (callers may use this for response bodies).
		if err := r.db.WithContext(ctx).First(item, "id = ?", existing.ID).Error; err != nil {
			return fmt.Errorf("reload collection item: %w", err)
		}
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("lookup collection item: %w", err)
	}
	// Fresh insert. Generate UUID at the Go level for the same portability
	// reasons explained on Create.
	if item.ID == "" {
		item.ID = uuid.NewString()
	}
	if err := r.db.WithContext(ctx).Create(item).Error; err != nil {
		return fmt.Errorf("create collection item: %w", err)
	}
	return nil
}

// RemoveItem deletes the (collection_id, anime_id) join row. Returns
// NotFound when the pair does not exist.
func (r *CollectionRepository) RemoveItem(ctx context.Context, collectionID, animeID string) error {
	result := r.db.WithContext(ctx).
		Where("collection_id = ? AND anime_id = ?", collectionID, animeID).
		Delete(&domain.CollectionItem{})
	if result.Error != nil {
		return fmt.Errorf("remove collection item: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return liberrors.NotFound("collection item")
	}
	return nil
}

// populateItemCounts loops through a slice of collections and sets
// ItemCount on each via a single COUNT query per collection. At Phase
// 17's expected size (<100 collections, <50 items each) the overhead is
// well under 100ms even with a hot DB.
func (r *CollectionRepository) populateItemCounts(ctx context.Context, collections []*domain.Collection) error {
	for _, c := range collections {
		var count int64
		if err := r.db.WithContext(ctx).Model(&domain.CollectionItem{}).
			Where("collection_id = ?", c.ID).
			Count(&count).Error; err != nil {
			return fmt.Errorf("count collection items: %w", err)
		}
		c.ItemCount = int(count)
	}
	return nil
}
