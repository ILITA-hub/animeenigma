package repo

import (
	"context"
	"time"

	"gorm.io/gorm"
)

// ResolvePerson returns the canonical person_id for an event via the
// analytics_events_resolved view (identified user if known, else anon).
func ResolvePerson(ctx context.Context, db *gorm.DB, eventID string) (string, error) {
	var personID string
	err := db.WithContext(ctx).
		Table("analytics_events_resolved").
		Where("event_id = ?", eventID).
		Select("person_id").
		Scan(&personID).Error
	return personID, err
}

// EraseByUserID deletes all events + identities for a user_id. Covers both
// directly-identified events and anonymous events stitched via identities.
func EraseByUserID(ctx context.Context, db *gorm.DB, userID string) error {
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Anonymous ids that map to this user.
		var anonIDs []string
		if err := tx.Model(&Identity{}).Where("user_id = ?", userID).
			Distinct().Pluck("anonymous_id", &anonIDs).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ?", userID).Delete(&Event{}).Error; err != nil {
			return err
		}
		if len(anonIDs) > 0 {
			if err := tx.Where("anonymous_id IN ?", anonIDs).Delete(&Event{}).Error; err != nil {
				return err
			}
		}
		return tx.Where("user_id = ?", userID).Delete(&Identity{}).Error
	})
}

// EraseByAnonymousID deletes all events + identities for an anonymous_id.
func EraseByAnonymousID(ctx context.Context, db *gorm.DB, anonymousID string) error {
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("anonymous_id = ?", anonymousID).Delete(&Event{}).Error; err != nil {
			return err
		}
		return tx.Where("anonymous_id = ?", anonymousID).Delete(&Identity{}).Error
	})
}

// PurgeOlderThan deletes events with timestamp < cutoff. Returns the count
// deleted. Backs the 90-day retention cron.
func PurgeOlderThan(ctx context.Context, db *gorm.DB, cutoff time.Time) (int64, error) {
	res := db.WithContext(ctx).Where("timestamp < ?", cutoff).Delete(&Event{})
	return res.RowsAffected, res.Error
}
