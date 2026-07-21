// Daily community-review spotlight.
//
// DailyReviewResolver selects one written (non-blank) public review for the
// UTC day. Selection is deterministic for a date via md5(review_id + date),
// so every viewer sees the same card and multiple catalog replicas converge
// even before Redis is warm. The selected payload is cached under a date-keyed
// key for 24 hours. Empty pools are never cached, allowing later reviews to
// become eligible during the same day.

package cards

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service/spotlight"
)

type dailyReviewDB interface {
	RawScan(ctx context.Context, dest any, sql string, args ...any) error
}

type gormDailyReviewAdapter struct {
	db *gorm.DB
}

// NewGormDailyReviewAdapter constructs the production DB adapter. Keeping the
// interface local lets resolver tests use a small handwritten fake.
func NewGormDailyReviewAdapter(db *gorm.DB) dailyReviewDB {
	return &gormDailyReviewAdapter{db: db}
}

func (a *gormDailyReviewAdapter) RawScan(ctx context.Context, dest any, sql string, args ...any) error {
	return a.db.WithContext(ctx).Raw(sql, args...).Scan(dest).Error
}

// dailyReviewSQL returns exactly one non-empty written review. Public profile
// fields come from users so renamed users/current avatars stay fresh. The
// anime_list user_id is used only for the join and is never projected.
const dailyReviewSQL = `
SELECT al.id AS review_id,
       al.score,
       BTRIM(al.review_text) AS review_text,
       to_char(al.created_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"') AS created_at,
       COALESCE(u.username, '') AS username,
       COALESCE(u.public_id, '') AS public_id,
       COALESCE(u.avatar, '') AS avatar,
       a.id AS anime_id,
       COALESCE(a.name, '') AS anime_name,
       COALESCE(a.name_ru, '') AS anime_name_ru,
       COALESCE(a.name_jp, '') AS anime_name_jp,
       COALESCE(a.poster_url, '') AS poster_url
FROM anime_list al
JOIN users u  ON u.id = al.user_id
JOIN animes a ON a.id = al.anime_id
WHERE NULLIF(BTRIM(al.review_text), '') IS NOT NULL
  AND COALESCE(a.hidden, false) = false
ORDER BY md5(al.id::text || ?)
LIMIT 1
`

type dailyReviewRow struct {
	ReviewID    string `gorm:"column:review_id"`
	Score       int    `gorm:"column:score"`
	ReviewText  string `gorm:"column:review_text"`
	CreatedAt   string `gorm:"column:created_at"`
	Username    string `gorm:"column:username"`
	PublicID    string `gorm:"column:public_id"`
	Avatar      string `gorm:"column:avatar"`
	AnimeID     string `gorm:"column:anime_id"`
	AnimeName   string `gorm:"column:anime_name"`
	AnimeNameRU string `gorm:"column:anime_name_ru"`
	AnimeNameJP string `gorm:"column:anime_name_jp"`
	PosterURL   string `gorm:"column:poster_url"`
}

// DailyReviewResolver implements spotlight.Resolver for `daily_review`.
type DailyReviewResolver struct {
	db    dailyReviewDB
	cache cache.Cache
	log   *logger.Logger
}

func NewDailyReviewResolver(db dailyReviewDB, c cache.Cache, log *logger.Logger) *DailyReviewResolver {
	return &DailyReviewResolver{db: db, cache: c, log: log}
}

func (r *DailyReviewResolver) Type() string { return "daily_review" }

func (r *DailyReviewResolver) Resolve(ctx context.Context, _ *string) (*spotlight.Card, error) {
	now := time.Now()
	dateKey := spotlight.DateKeyUTC(now)
	key := "spotlight:daily_review:" + dateKey

	var cached spotlight.DailyReviewData
	if err := r.cache.Get(ctx, key, &cached); err == nil {
		return &spotlight.Card{Type: r.Type(), Data: cached}, nil
	} else if !errors.Is(err, cache.ErrNotFound) {
		r.log.Warnw("spotlight.cache_get_failed", "type", r.Type(), "key", key, "error", err)
	}

	var rows []dailyReviewRow
	if err := r.db.RawScan(ctx, &rows, dailyReviewSQL, dateKey); err != nil {
		return nil, fmt.Errorf("daily_review: db: %w", err)
	}
	if len(rows) == 0 {
		return nil, nil
	}

	row := rows[0]
	data := spotlight.DailyReviewData{
		ReviewID: row.ReviewID,
		Anime: spotlight.DailyReviewAnime{
			ID:        row.AnimeID,
			Name:      row.AnimeName,
			NameRU:    row.AnimeNameRU,
			NameJP:    row.AnimeNameJP,
			PosterURL: row.PosterURL,
		},
		Author: spotlight.DailyReviewAuthor{
			Username: row.Username,
			PublicID: row.PublicID,
			Avatar:   row.Avatar,
		},
		Score:      row.Score,
		ReviewText: row.ReviewText,
		CreatedAt:  row.CreatedAt,
	}

	if err := r.cache.Set(ctx, key, data, cardTTL); err != nil {
		r.log.Warnw("spotlight.cache_set_failed", "type", r.Type(), "key", key, "error", err)
	}
	return &spotlight.Card{Type: r.Type(), Data: data}, nil
}
