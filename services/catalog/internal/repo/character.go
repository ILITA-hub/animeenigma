package repo

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
)

// CharacterRepository persists characters and the anime<->character join.
type CharacterRepository struct {
	db *gorm.DB
}

func NewCharacterRepository(db *gorm.DB) *CharacterRepository {
	return &CharacterRepository{db: db}
}

// UpsertCharacter inserts or updates a character by shikimori_id and returns
// the stored row (with its generated UUID id populated).
// ID is generated at the Go level when blank — Postgres's gen_random_uuid()
// default works in prod, but generating here keeps the repo portable to
// SQLite tests and self-contained (mirrors CollectionRepository.Create).
func (r *CharacterRepository) UpsertCharacter(ctx context.Context, ch *domain.Character) (*domain.Character, error) {
	if ch.ID == "" {
		ch.ID = uuid.NewString()
	}
	if err := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "shikimori_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"mal_id", "name", "name_ru", "name_jp", "synonyms", "poster_url", "description", "url", "seyu", "updated_at"}),
		}).
		Create(ch).Error; err != nil {
		return nil, err
	}
	var stored domain.Character
	if err := r.db.WithContext(ctx).Where("shikimori_id = ?", ch.ShikimoriID).First(&stored).Error; err != nil {
		return nil, err
	}
	return &stored, nil
}

// GetByShikimoriID returns a single stored character, or gorm.ErrRecordNotFound.
func (r *CharacterRepository) GetByShikimoriID(ctx context.Context, shikimoriID string) (*domain.Character, error) {
	var ch domain.Character
	if err := r.db.WithContext(ctx).Where("shikimori_id = ?", shikimoriID).First(&ch).Error; err != nil {
		return nil, err
	}
	return &ch, nil
}

// ReplaceAnimeCharacters upserts every character and rebuilds the anime's join
// rows in one transaction. Position is the index within rows as passed.
//
// When chars and rows are index-aligned (rows[i] is the join for chars[i], as
// the catalog service builds them), the repo resolves each join row's
// CharacterID to the canonical *stored* id after the upsert. This matters on
// the conflict path: an OnConflict-by-shikimori_id upsert keeps the existing
// row's id, so a caller-assigned fresh UUID would not match the stored id and
// the join (anime_characters.character_id = characters.id) would silently drop
// the character. Resolving here lets the service bulk-upsert in ONE call
// instead of the old per-character UpsertCharacter loop (the 2N N+1).
func (r *CharacterRepository) ReplaceAnimeCharacters(ctx context.Context, animeID string, rows []domain.AnimeCharacter, chars []domain.Character) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for i := range chars {
			if chars[i].ID == "" {
				chars[i].ID = uuid.NewString()
			}
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "shikimori_id"}},
				DoUpdates: clause.AssignmentColumns([]string{"mal_id", "name", "name_ru", "name_jp", "poster_url", "updated_at"}),
			}).Create(&chars[i]).Error; err != nil {
				return err
			}
		}

		// Resolve join rows to the canonical stored character ids. Build a
		// shikimori_id -> stored id map from a single read-back so a conflicting
		// upsert (which keeps the pre-existing id) doesn't orphan the join.
		// Only applies when rows are index-aligned with chars.
		if len(chars) > 0 && len(rows) == len(chars) {
			shikiIDs := make([]string, 0, len(chars))
			for i := range chars {
				shikiIDs = append(shikiIDs, chars[i].ShikimoriID)
			}
			var stored []domain.Character
			if err := tx.Where("shikimori_id IN ?", shikiIDs).Find(&stored).Error; err != nil {
				return err
			}
			idByShikimori := make(map[string]string, len(stored))
			for _, s := range stored {
				idByShikimori[s.ShikimoriID] = s.ID
			}
			for i := range rows {
				if id, ok := idByShikimori[chars[i].ShikimoriID]; ok {
					rows[i].CharacterID = id
				}
			}
		}

		if err := tx.Where("anime_id = ?", animeID).Delete(&domain.AnimeCharacter{}).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		return tx.Create(&rows).Error
	})
}

// GetByAnimeID returns the anime's characters ordered Main-first then Position.
func (r *CharacterRepository) GetByAnimeID(ctx context.Context, animeID string) ([]domain.AnimeCharacterView, error) {
	var views []domain.AnimeCharacterView
	err := r.db.WithContext(ctx).Raw(`
		SELECT c.*, ac.role, ac.position
		FROM characters c
		JOIN anime_characters ac ON ac.character_id = c.id
		WHERE ac.anime_id = ? AND c.deleted_at IS NULL
		ORDER BY CASE WHEN ac.role = 'main' THEN 0 ELSE 1 END, ac.position
	`, animeID).Scan(&views).Error
	if err != nil {
		return nil, err
	}
	return views, nil
}
