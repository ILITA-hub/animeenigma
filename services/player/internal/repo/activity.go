package repo

import (
	"context"
	"fmt"
	"time"

	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"gorm.io/gorm"
)

type ActivityRepository struct {
	db *gorm.DB
}

func NewActivityRepository(db *gorm.DB) *ActivityRepository {
	return &ActivityRepository{db: db}
}

// Create records a new activity event.
func (r *ActivityRepository) Create(ctx context.Context, event *domain.ActivityEvent) error {
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now()
	}
	return r.db.WithContext(ctx).Create(event).Error
}

// GetTodayByUserAnimeType finds an existing activity event for the same user+anime+type created today.
// Used for daily deduplication of review events.
func (r *ActivityRepository) GetTodayByUserAnimeType(ctx context.Context, userID, animeID, eventType string) (*domain.ActivityEvent, error) {
	today := time.Now().Truncate(24 * time.Hour)
	var event domain.ActivityEvent
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND anime_id = ? AND type = ? AND created_at >= ?", userID, animeID, eventType, today).
		First(&event).Error
	if err != nil {
		return nil, err
	}
	return &event, nil
}

// Update updates an existing activity event's values and timestamp.
func (r *ActivityRepository) Update(ctx context.Context, event *domain.ActivityEvent) error {
	return r.db.WithContext(ctx).
		Model(event).
		Updates(map[string]interface{}{
			"new_value":  event.NewValue,
			"old_value":  event.OldValue,
			"created_at": time.Now(),
		}).Error
}

// GetFeed returns the latest activity events with cursor-based pagination.
// Pass empty `before` for the first page. `before` is the ID of the last event from the previous page.
//
// Respects the author's users.activity_visibility setting at READ time (so
// toggling the setting retroactively hides/unhides history): 'none' drops all
// of the user's events, 'non_hentai' drops events on 18+ titles. LEFT JOIN +
// COALESCE keep events visible when the users row is missing or predates the
// column (pre-feature behaviour).
func (r *ActivityRepository) GetFeed(ctx context.Context, limit int, before string) ([]*domain.ActivityEvent, bool, error) {
	query := r.db.WithContext(ctx).
		Preload("Anime").
		Joins("LEFT JOIN users ON users.id = activity_events.user_id").
		Where("COALESCE(users.activity_visibility, 'all') <> 'none'").
		Where("NOT (COALESCE(users.activity_visibility, 'all') = 'non_hentai' AND "+
			fmt.Sprintf(hentaiAnimeExistsFmt, "activity_events.anime_id")+")").
		Order("activity_events.created_at DESC, activity_events.id DESC")

	if before != "" {
		// Get the created_at of the cursor event
		var cursor domain.ActivityEvent
		if err := r.db.WithContext(ctx).Select("created_at").Where("id = ?", before).First(&cursor).Error; err != nil {
			return nil, false, err
		}
		query = query.Where(
			"activity_events.created_at < ? OR (activity_events.created_at = ? AND activity_events.id < ?)",
			cursor.CreatedAt, cursor.CreatedAt, before,
		)
	}

	var events []*domain.ActivityEvent
	// Fetch one extra to determine hasMore
	err := query.Limit(limit + 1).Find(&events).Error
	if err != nil {
		return nil, false, err
	}

	hasMore := len(events) > limit
	if hasMore {
		events = events[:limit]
	}

	r.attachUserAvatars(ctx, events)

	return events, hasMore, nil
}

// attachUserAvatars populates UserAvatar on each event from the users table in
// a single batched query (shared fetchUserAvatars helper, also used by the
// reviews and comments read paths). Best-effort: on error the feed still
// renders (the frontend falls back to the username initial).
func (r *ActivityRepository) attachUserAvatars(ctx context.Context, events []*domain.ActivityEvent) {
	if len(events) == 0 {
		return
	}
	ids := make([]string, 0, len(events))
	for _, e := range events {
		ids = append(ids, e.UserID)
	}
	avatars := fetchUserAvatars(ctx, r.db, ids)
	for _, e := range events {
		e.UserAvatar = avatars[e.UserID]
	}
}
