package repo

import (
	"context"

	apperrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/services/notifications/internal/domain"
	"gorm.io/gorm"
)

// MaxWatchedRepository answers "for each user × combo, what is the highest
// episode_number they have already watched?" — Phase 2's detector uses this
// to compute first_unwatched_episode = max_watched + 1 (NOTIF-DET-07).
//
// Single GROUP BY query over the existing read-only watch_history view.
// Combos with no watch_history rows are simply absent from the result —
// callers (the detector) iterate keys present in the map and skip the
// rest, which matches the bootstrap-protection semantics (a combo collected
// by HotCombosCollector but with no per-user rows means no notifications
// will fire for it).
type MaxWatchedRepository struct {
	db *gorm.DB
}

// NewMaxWatchedRepository constructs the repo.
func NewMaxWatchedRepository(db *gorm.DB) *MaxWatchedRepository {
	return &MaxWatchedRepository{db: db}
}

// maxWatchedRow is the scan target — flat shape, then folded into the
// nested map[Combo]map[userID]maxEp.
type maxWatchedRow struct {
	UserID        string `gorm:"column:user_id"`
	AnimeID       string `gorm:"column:anime_id"`
	Player        string `gorm:"column:player"`
	Language      string `gorm:"column:language"`
	WatchType     string `gorm:"column:watch_type"`
	TranslationID string `gorm:"column:translation_id"`
	MaxWatched    int    `gorm:"column:max_watched"`
}

// ForCombos returns user→max_episode for every (combo, user) that appears
// in watch_history within the given combo set.
//
// Result shape: combo → userID → maxEpisode
//
// One SQL via `(anime_id, player, language, watch_type, translation_id)
// IN (...)`. Empty input returns empty map without hitting the DB.
func (r *MaxWatchedRepository) ForCombos(
	ctx context.Context,
	combos []domain.Combo,
) (map[domain.Combo]map[string]int, error) {
	out := make(map[domain.Combo]map[string]int)
	if len(combos) == 0 {
		return out, nil
	}

	tuples := make([][]interface{}, 0, len(combos))
	for _, c := range combos {
		tuples = append(tuples, []interface{}{
			c.AnimeID, c.Player, c.Language, c.WatchType, c.TranslationID,
		})
	}

	var rows []maxWatchedRow
	err := r.db.WithContext(ctx).
		Table("watch_history").
		Select(`user_id, anime_id, player, language, watch_type, translation_id,
		         MAX(episode_number) AS max_watched`).
		Where("(anime_id, player, language, watch_type, translation_id) IN ?", tuples).
		Group("user_id, anime_id, player, language, watch_type, translation_id").
		Scan(&rows).Error
	if err != nil {
		return nil, apperrors.Wrap(err, apperrors.CodeInternal, "max-watched group by")
	}

	for _, row := range rows {
		key := domain.Combo{
			AnimeID:       row.AnimeID,
			Player:        row.Player,
			Language:      row.Language,
			WatchType:     row.WatchType,
			TranslationID: row.TranslationID,
		}
		inner, ok := out[key]
		if !ok {
			inner = make(map[string]int)
			out[key] = inner
		}
		inner[row.UserID] = row.MaxWatched
	}
	return out, nil
}
