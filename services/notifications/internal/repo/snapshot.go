package repo

import (
	"context"
	"time"

	apperrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/services/notifications/internal/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// SnapshotRepository owns parser_episode_snapshots access. Phase 2 v1.0
// Notifications Engine — detector reads via BulkLoad, writes via
// BulkUpsert. Phase 1 left the type as a stub; this file adds the real
// bulk operations.
//
// All operations operate on the COMPOSITE NATURAL KEY:
//
//	(anime_id, player, language, watch_type, translation_id)
//
// enforced by the uk_combo unique index on the table.
type SnapshotRepository struct {
	db *gorm.DB
}

// NewSnapshotRepository constructs the repo.
func NewSnapshotRepository(db *gorm.DB) *SnapshotRepository {
	return &SnapshotRepository{db: db}
}

// snapshotBulkChunkSize bounds the OnConflict batch size so we stay under
// Postgres' (~32k) bind-parameter limit even on pathological deployments
// where one user watches thousands of combos.
const snapshotBulkChunkSize = 200

// BulkLoad fetches the current latest_episode for each combo. Missing
// combos are simply absent from the result map — callers (the detector)
// treat absence as bootstrap (NOTIF-DET-06).
//
// One SQL round-trip via `(anime_id, player, language, watch_type,
// translation_id) IN (...)` row-constructor IN-clause.
func (r *SnapshotRepository) BulkLoad(ctx context.Context, combos []domain.Combo) (map[domain.Combo]int, error) {
	result := make(map[domain.Combo]int, len(combos))
	if len(combos) == 0 {
		return result, nil
	}

	tuples := make([][]interface{}, 0, len(combos))
	for _, c := range combos {
		tuples = append(tuples, []interface{}{
			c.AnimeID, c.Player, c.Language, c.WatchType, c.TranslationID,
		})
	}

	var rows []domain.ParserEpisodeSnapshot
	err := r.db.WithContext(ctx).
		Where("(anime_id, player, language, watch_type, translation_id) IN ?", tuples).
		Find(&rows).Error
	if err != nil {
		return nil, apperrors.Wrap(err, apperrors.CodeInternal, "snapshot bulk load")
	}

	for _, row := range rows {
		key := domain.Combo{
			AnimeID:       row.AnimeID,
			Player:        row.Player,
			Language:      row.Language,
			WatchType:     row.WatchType,
			TranslationID: row.TranslationID,
		}
		result[key] = row.LatestEpisode
	}
	return result, nil
}

// BulkUpsert inserts new snapshot rows or refreshes existing ones to
// `latest_episode = EXCLUDED.latest_episode`. NEVER lowers an existing
// value — the never-lower invariant lives in the detector (NOTIF-DET-10):
// the detector pre-filters its map to drop entries where the new value is
// less than the prior snapshot.
//
// Idempotent: re-running with the same map is a no-op except for
// `checked_at` / `updated_at` bumps.
func (r *SnapshotRepository) BulkUpsert(ctx context.Context, updates map[domain.Combo]int) error {
	if len(updates) == 0 {
		return nil
	}

	now := time.Now().UTC()
	rows := make([]domain.ParserEpisodeSnapshot, 0, len(updates))
	for combo, latest := range updates {
		rows = append(rows, domain.ParserEpisodeSnapshot{
			AnimeID:       combo.AnimeID,
			Player:        combo.Player,
			Language:      combo.Language,
			WatchType:     combo.WatchType,
			TranslationID: combo.TranslationID,
			LatestEpisode: latest,
			CheckedAt:     now,
			UpdatedAt:     now,
		})
	}

	conflict := clause.OnConflict{
		Columns: []clause.Column{
			{Name: "anime_id"},
			{Name: "player"},
			{Name: "language"},
			{Name: "watch_type"},
			{Name: "translation_id"},
		},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"latest_episode": gorm.Expr("EXCLUDED.latest_episode"),
			"checked_at":     now,
			"updated_at":     now,
		}),
	}

	for start := 0; start < len(rows); start += snapshotBulkChunkSize {
		end := start + snapshotBulkChunkSize
		if end > len(rows) {
			end = len(rows)
		}
		chunk := rows[start:end]
		if err := r.db.WithContext(ctx).Clauses(conflict).Create(&chunk).Error; err != nil {
			return apperrors.Wrap(err, apperrors.CodeInternal, "snapshot bulk upsert")
		}
	}
	return nil
}
