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
// structurally by ValidateBlocks. Variant selects the display layout for the
// block; an empty string means "use the default variant".
type Block struct {
	Type    string          `json:"type"`
	Variant string          `json:"variant,omitempty"`
	Order   int             `json:"order"`
	Config  json.RawMessage `json:"config"`
}

const (
	BlockAbout             = "about"
	BlockFavoriteAnime     = "favorite_anime"
	BlockStats             = "stats"
	BlockFavoriteCharacter = "favorite_character"
	BlockCardCollection    = "card_collection"
	BlockContinueWatching  = "continue_watching"
	BlockOpEd              = "op_ed"
	BlockAnimeDNA          = "anime_dna"
	BlockCompatibility     = "compatibility"

	MaxBlocks     = 12
	MaxBlockItems = 12
	MaxAboutTitle = 64
	MaxAboutText  = 2000
)

// VariantAllowlist maps each block type to its permitted variants. The first
// entry is the default (used when Block.Variant is empty). Keep in sync with
// frontend src/types/showcase.ts.
var VariantAllowlist = map[string][]string{
	BlockAbout:             {"quote", "bio", "terminal", "minimal", "vn"},
	BlockFavoriteAnime:     {"row", "podium", "grid", "list", "banner"},
	BlockFavoriteCharacter: {"circles", "portraits", "hero", "hex"},
	BlockCardCollection:    {"row", "fan", "grid", "hero", "tilt3d"},
	BlockStats:             {"tiles", "rings", "bars", "strip"},
	BlockContinueWatching:  {"cards"},
	BlockOpEd:              {"grid"},
	BlockAnimeDNA:          {"bars"},
	BlockCompatibility:     {"ring"},
}

type aboutConfig struct {
	Title string `json:"title"`
	Text  string `json:"text"`
}
type idListConfig struct {
	AnimeIDs     []string `json:"anime_ids"`
	CardIDs      []string `json:"card_ids"`
	CharacterIDs []int    `json:"character_ids"`
}
type opEdConfig struct {
	ThemeIDs []string `json:"theme_ids"`
}

func variantAllowed(blockType, variant string) bool {
	if variant == "" {
		return true // empty ⇒ default
	}
	for _, v := range VariantAllowlist[blockType] {
		if v == variant {
			return true
		}
	}
	return false
}

// ValidateBlocks enforces structural limits only (no ownership/visibility,
// no cross-service calls — see plan Global Constraints).
func ValidateBlocks(blocks []Block) error {
	if len(blocks) > MaxBlocks {
		return errors.InvalidInput("too many showcase blocks")
	}
	for _, b := range blocks {
		if _, known := VariantAllowlist[b.Type]; !known {
			return errors.InvalidInput("unknown showcase block type")
		}
		if !variantAllowed(b.Type, b.Variant) {
			return errors.InvalidInput("unknown variant for block type")
		}
		switch b.Type {
		case BlockStats, BlockContinueWatching, BlockAnimeDNA, BlockCompatibility:
			// auto blocks: no config required
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
		case BlockFavoriteAnime:
			var c idListConfig
			if err := json.Unmarshal(b.Config, &c); err != nil {
				return errors.InvalidInput("invalid block config")
			}
			if len(c.AnimeIDs) > MaxBlockItems {
				return errors.InvalidInput("too many items in showcase block")
			}
		case BlockCardCollection:
			var c idListConfig
			if err := json.Unmarshal(b.Config, &c); err != nil {
				return errors.InvalidInput("invalid block config")
			}
			if len(c.CardIDs) > MaxBlockItems {
				return errors.InvalidInput("too many items in showcase block")
			}
		case BlockFavoriteCharacter:
			var c idListConfig
			if err := json.Unmarshal(b.Config, &c); err != nil {
				return errors.InvalidInput("invalid block config")
			}
			if len(c.CharacterIDs) > MaxBlockItems {
				return errors.InvalidInput("too many items in showcase block")
			}
		case BlockOpEd:
			var c opEdConfig
			if err := json.Unmarshal(b.Config, &c); err != nil {
				return errors.InvalidInput("invalid op_ed config")
			}
			if len(c.ThemeIDs) > MaxBlockItems {
				return errors.InvalidInput("too many op_ed themes")
			}
		}
	}
	return nil
}
