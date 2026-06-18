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
	Width   int             `json:"w,omitempty"`
	Height  int             `json:"h,omitempty"`
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

// SizeBound is the grid-cell size range for one (block type, variant).
type SizeBound struct{ MinW, MaxW, MinH, MaxH, DefW, DefH int }

// VariantSizeAllowlist maps block type → variant → size bounds (grid cells,
// W 1..4, H 1..3). Keep in sync with frontend src/types/showcase.ts VARIANT_SIZE.
var VariantSizeAllowlist = map[string]map[string]SizeBound{
	BlockAbout: {
		"quote": {2, 4, 1, 2, 2, 1}, "bio": {2, 4, 1, 2, 2, 2}, "terminal": {2, 4, 1, 2, 2, 2},
		"minimal": {2, 4, 1, 2, 2, 1}, "vn": {2, 2, 1, 1, 2, 1},
	},
	BlockFavoriteAnime: {
		"row": {2, 4, 1, 1, 4, 1}, "podium": {2, 2, 2, 2, 2, 2}, "grid": {2, 4, 1, 3, 4, 2},
		"list": {2, 2, 1, 3, 2, 2}, "banner": {2, 4, 1, 3, 2, 2},
	},
	BlockFavoriteCharacter: {
		"circles": {1, 4, 1, 3, 2, 1}, "portraits": {2, 4, 1, 3, 2, 2},
		"hero": {2, 4, 1, 3, 2, 2}, "hex": {1, 4, 1, 3, 2, 2},
	},
	BlockCardCollection: {
		"row": {2, 4, 1, 1, 2, 1}, "fan": {2, 4, 2, 3, 2, 2}, "grid": {2, 4, 1, 3, 2, 2},
		"hero": {2, 4, 1, 2, 2, 2}, "tilt3d": {2, 4, 2, 3, 3, 2},
	},
	BlockStats: {
		"tiles": {2, 2, 1, 1, 2, 1}, "rings": {2, 2, 1, 1, 2, 1},
		"bars": {2, 2, 1, 1, 2, 1}, "strip": {2, 2, 1, 1, 2, 1},
	},
	BlockContinueWatching: {"cards": {2, 4, 1, 3, 2, 2}},
	BlockOpEd:             {"grid": {2, 4, 1, 3, 2, 2}},
	BlockAnimeDNA:         {"bars": {1, 2, 1, 3, 1, 2}},
	BlockCompatibility:    {"ring": {1, 2, 1, 1, 2, 1}},
}

// SizeFor returns the bound for (type, variant); empty/unknown variant falls
// back to the type's default variant (VariantAllowlist[type][0]).
func SizeFor(blockType, variant string) SizeBound {
	byVariant := VariantSizeAllowlist[blockType]
	if b, ok := byVariant[variant]; ok {
		return b
	}
	if defaults := VariantAllowlist[blockType]; len(defaults) > 0 {
		return byVariant[defaults[0]]
	}
	return SizeBound{1, 4, 1, 3, 2, 1}
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// clampBlockSize backfills absent w/h to the variant default and clamps any
// provided value into the variant's range. Never rejects.
func clampBlockSize(b *Block) {
	sb := SizeFor(b.Type, b.Variant)
	if b.Width == 0 {
		b.Width = sb.DefW
	}
	if b.Height == 0 {
		b.Height = sb.DefH
	}
	b.Width = clampInt(b.Width, sb.MinW, sb.MaxW)
	b.Height = clampInt(b.Height, sb.MinH, sb.MaxH)
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
	for i := range blocks {
		b := &blocks[i]
		if _, known := VariantAllowlist[b.Type]; !known {
			return errors.InvalidInput("unknown showcase block type")
		}
		if !variantAllowed(b.Type, b.Variant) {
			return errors.InvalidInput("unknown variant for block type")
		}
		clampBlockSize(b)
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
