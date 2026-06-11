package repo

import (
	"context"

	"gorm.io/gorm"
)

// fetchUserAvatars returns a userID→avatar map for the given user IDs from
// the users table in a single batched query. Shared by the activity feed,
// reviews, and comments read paths so all three render the author's CURRENT
// avatar. Best-effort: returns nil on error so callers degrade to the
// frontend's username-initials fallback (a nil map indexes to "").
func fetchUserAvatars(ctx context.Context, db *gorm.DB, userIDs []string) map[string]string {
	ids := make([]string, 0, len(userIDs))
	seen := make(map[string]struct{}, len(userIDs))
	for _, id := range userIDs {
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil
	}

	type userAvatar struct {
		ID     string
		Avatar string
	}
	var rows []userAvatar
	if err := db.WithContext(ctx).
		Table("users").
		Select("id, avatar").
		Where("id IN ?", ids).
		Scan(&rows).Error; err != nil {
		return nil // best-effort
	}

	avatars := make(map[string]string, len(rows))
	for _, row := range rows {
		avatars[row.ID] = row.Avatar
	}
	return avatars
}
