package domain

import (
	"encoding/json"
	"time"
	"unicode/utf8"

	"github.com/ILITA-hub/animeenigma/libs/errors"
)

// ProfileShowcase is the Steam-style customizable profile "wall". One row
// per user. Blocks holds an ordered JSON array of Block (jsonb on Postgres,
// TEXT on SQLite in tests). The backend is a pure config store: content
// (posters, characters, cards, stats) is resolved on the frontend.
type ProfileShowcase struct {
	UserID    string    `gorm:"type:uuid;primaryKey" json:"user_id"`
	Blocks    string    `gorm:"type:jsonb;not null;default:'[]'" json:"-"`
	UpdatedAt time.Time `gorm:"not null;default:now()" json:"updated_at"`
}

func (ProfileShowcase) TableName() string { return "profile_showcases" }

// Block is one showcase block. Config is type-specific raw JSON, validated
// structurally by ValidateBlocks.
type Block struct {
	Type   string          `json:"type"`
	Order  int             `json:"order"`
	Config json.RawMessage `json:"config"`
}

const (
	BlockAbout             = "about"
	BlockFavoriteAnime     = "favorite_anime"
	BlockStats             = "stats"
	BlockFavoriteCharacter = "favorite_character"
	BlockCardCollection    = "card_collection"

	MaxBlocks     = 12
	MaxBlockItems = 12
	MaxAboutTitle = 64
	MaxAboutText  = 2000
)

type aboutConfig struct {
	Title string `json:"title"`
	Text  string `json:"text"`
}
type idListConfig struct {
	AnimeIDs     []string `json:"anime_ids"`
	CardIDs      []string `json:"card_ids"`
	CharacterIDs []int    `json:"character_ids"`
}

// ValidateBlocks enforces structural limits only (no ownership/visibility,
// no cross-service calls — see plan Global Constraints).
func ValidateBlocks(blocks []Block) error {
	if len(blocks) > MaxBlocks {
		return errors.InvalidInput("too many showcase blocks")
	}
	for _, b := range blocks {
		switch b.Type {
		case BlockStats:
			// no config required
		case BlockAbout:
			var c aboutConfig
			if err := json.Unmarshal(b.Config, &c); err != nil {
				return errors.InvalidInput("invalid about config")
			}
			if utf8.RuneCountInString(c.Title) > MaxAboutTitle {
				return errors.InvalidInput("about title too long")
			}
			if utf8.RuneCountInString(c.Text) > MaxAboutText {
				return errors.InvalidInput("about text too long")
			}
		case BlockFavoriteAnime, BlockCardCollection, BlockFavoriteCharacter:
			var c idListConfig
			if err := json.Unmarshal(b.Config, &c); err != nil {
				return errors.InvalidInput("invalid block config")
			}
			n := len(c.AnimeIDs) + len(c.CardIDs) + len(c.CharacterIDs)
			if n > MaxBlockItems {
				return errors.InvalidInput("too many items in showcase block")
			}
		default:
			return errors.InvalidInput("unknown showcase block type")
		}
	}
	return nil
}
