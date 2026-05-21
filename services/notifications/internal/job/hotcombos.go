package job

import (
	"context"

	apperrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/notifications/internal/domain"
	"gorm.io/gorm"
)

// HotCombosCollector runs the single DISTINCT join that turns the entire
// (watch_history × anime_list × animes) trust surface into the set of combos
// the detector actually needs to ask the catalog about.
//
// NOTIF-DET-02 mandate (literal SQL):
//
//	SELECT DISTINCT
//	    wh.anime_id, a.shikimori_id, wh.player, wh.language,
//	    wh.watch_type, wh.translation_id
//	FROM watch_history wh
//	JOIN anime_list al ON al.user_id = wh.user_id AND al.anime_id = wh.anime_id
//	JOIN animes a ON a.id = wh.anime_id
//	WHERE al.status = 'watching'
//	  AND a.status = 'ongoing'
//	  AND wh.translation_id != '';
//
// Filtering on status='watching' (user-side) AND 'ongoing' (catalog-side)
// + non-empty translation_id keeps the result set bounded to "combos that
// could legitimately produce a new_episode notification". English-player
// rows in watch_history naturally drop out — their translation_id either
// isn't populated or the watch_type doesn't match the catalog allowlist
// {kodik, animelib} (D-DET-03).
type HotCombosCollector struct {
	db  *gorm.DB
	log *logger.Logger
}

// NewHotCombosCollector constructs the collector.
func NewHotCombosCollector(db *gorm.DB, log *logger.Logger) *HotCombosCollector {
	return &HotCombosCollector{db: db, log: log}
}

// Collect executes the DISTINCT join and returns the active hot combos.
func (c *HotCombosCollector) Collect(ctx context.Context) ([]domain.Combo, error) {
	const q = `
		SELECT DISTINCT
		    wh.anime_id      AS anime_id,
		    a.shikimori_id   AS shikimori_id,
		    wh.player        AS player,
		    wh.language      AS language,
		    wh.watch_type    AS watch_type,
		    wh.translation_id AS translation_id
		FROM watch_history wh
		JOIN anime_list al ON al.user_id = wh.user_id AND al.anime_id = wh.anime_id
		JOIN animes a ON a.id = wh.anime_id
		WHERE al.status = 'watching'
		  AND a.status = 'ongoing'
		  AND wh.translation_id != ''
	`

	var rows []domain.Combo
	if err := c.db.WithContext(ctx).Raw(q).Scan(&rows).Error; err != nil {
		return nil, apperrors.Wrap(err, apperrors.CodeInternal, "collect hot combos")
	}
	return rows, nil
}
