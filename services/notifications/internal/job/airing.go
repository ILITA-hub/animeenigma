package job

import (
	"context"
	"time"

	apperrors "github.com/ILITA-hub/animeenigma/libs/errors"
)

// AiringTimes returns next_episode_at per anime id, for the tier decision.
// Rows with a NULL next_episode_at (and ids not present) are omitted — the
// caller treats an absent value as "airing unknown → hot".
func (c *HotCombosCollector) AiringTimes(ctx context.Context, animeIDs []string) (map[string]*time.Time, error) {
	out := map[string]*time.Time{}
	if len(animeIDs) == 0 {
		return out, nil
	}
	var rows []struct {
		ID            string
		NextEpisodeAt *time.Time
	}
	if err := c.db.WithContext(ctx).
		Table("animes").
		Select("id, next_episode_at").
		Where("id IN ?", animeIDs).
		Scan(&rows).Error; err != nil {
		return nil, apperrors.Wrap(err, apperrors.CodeInternal, "airing times")
	}
	for _, r := range rows {
		if r.NextEpisodeAt != nil {
			out[r.ID] = r.NextEpisodeAt
		}
	}
	return out, nil
}
