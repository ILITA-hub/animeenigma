package repo

import (
	"context"

	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type FollowRepository struct {
	db *gorm.DB
}

func NewFollowRepository(db *gorm.DB) *FollowRepository {
	return &FollowRepository{db: db}
}

func (r *FollowRepository) UserExists(ctx context.Context, userID string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Table("users").Where("id = ? AND deleted_at IS NULL", userID).Count(&count).Error
	return count > 0, err
}

// Follow is idempotent so double-clicks and request retries cannot duplicate rows.
func (r *FollowRepository) Follow(ctx context.Context, followerID, followedID string) error {
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&domain.UserFollow{
		FollowerID: followerID,
		FollowedID: followedID,
	}).Error
}

func (r *FollowRepository) Unfollow(ctx context.Context, followerID, followedID string) error {
	return r.db.WithContext(ctx).
		Where("follower_id = ? AND followed_id = ?", followerID, followedID).
		Delete(&domain.UserFollow{}).Error
}

func (r *FollowRepository) IsFollowing(ctx context.Context, followerID, followedID string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&domain.UserFollow{}).
		Where("follower_id = ? AND followed_id = ?", followerID, followedID).
		Count(&count).Error
	return count > 0, err
}

func (r *FollowRepository) List(ctx context.Context, followerID string) ([]domain.FollowedUser, error) {
	var users []domain.FollowedUser
	err := r.db.WithContext(ctx).Table("user_follows").
		Select("users.id, users.username, users.public_id, users.avatar, user_follows.created_at AS followed_at").
		Joins("JOIN users ON users.id = user_follows.followed_id").
		Where("user_follows.follower_id = ? AND users.deleted_at IS NULL", followerID).
		Order("user_follows.created_at DESC").
		Scan(&users).Error
	return users, err
}
