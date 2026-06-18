package domain

import (
	"encoding/json"
	"strings"
	"testing"
)

func cfg(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	return b
}

func TestValidateBlocks_OK(t *testing.T) {
	blocks := []Block{
		{Type: BlockAbout, Order: 0, Config: cfg(t, map[string]string{"title": "Hi", "text": "about me"})},
		{Type: BlockStats, Order: 1, Config: cfg(t, map[string]any{})},
		{Type: BlockFavoriteAnime, Order: 2, Config: cfg(t, map[string][]string{"anime_ids": {"a1", "a2"}})},
	}
	if err := ValidateBlocks(blocks); err != nil {
		t.Fatalf("expected valid, got %v", err)
	}
}

func TestValidateBlocks_UnknownType(t *testing.T) {
	if err := ValidateBlocks([]Block{{Type: "bogus", Order: 0, Config: cfg(t, map[string]any{})}}); err == nil {
		t.Fatal("expected error for unknown block type")
	}
}

func TestValidateBlocks_TooManyBlocks(t *testing.T) {
	blocks := make([]Block, MaxBlocks+1)
	for i := range blocks {
		blocks[i] = Block{Type: BlockStats, Order: i, Config: cfg(t, map[string]any{})}
	}
	if err := ValidateBlocks(blocks); err == nil {
		t.Fatal("expected error when exceeding MaxBlocks")
	}
}

func TestValidateBlocks_TooManyItems(t *testing.T) {
	ids := make([]string, MaxBlockItems+1)
	for i := range ids {
		ids[i] = "id"
	}
	b := []Block{{Type: BlockFavoriteAnime, Order: 0, Config: cfg(t, map[string][]string{"anime_ids": ids})}}
	if err := ValidateBlocks(b); err == nil {
		t.Fatal("expected error when exceeding MaxBlockItems")
	}
}

func TestValidateBlocks_AboutTooLong(t *testing.T) {
	b := []Block{{Type: BlockAbout, Order: 0, Config: cfg(t, map[string]string{"text": strings.Repeat("x", MaxAboutText+1)})}}
	if err := ValidateBlocks(b); err == nil {
		t.Fatal("expected error for over-long about text")
	}
}

func TestValidateBlocks_AboutTitleTooLong(t *testing.T) {
	b := []Block{{Type: BlockAbout, Order: 0, Config: cfg(t, map[string]string{"title": strings.Repeat("x", MaxAboutTitle+1)})}}
	if err := ValidateBlocks(b); err == nil {
		t.Fatal("expected error for over-long about title")
	}
}

func TestValidateBlocks_FavoriteCharacterTooMany(t *testing.T) {
	ids := make([]int, MaxBlockItems+1)
	for i := range ids {
		ids[i] = i + 1
	}
	b := []Block{{Type: BlockFavoriteCharacter, Order: 0, Config: cfg(t, map[string][]int{"character_ids": ids})}}
	if err := ValidateBlocks(b); err == nil {
		t.Fatal("expected error when favorite_character exceeds MaxBlockItems")
	}
}

func TestValidateBlocks_VariantOK(t *testing.T) {
	b := []Block{{Type: BlockFavoriteAnime, Variant: "podium", Config: cfg(t, map[string][]string{"anime_ids": {"a"}})}}
	if err := ValidateBlocks(b); err != nil {
		t.Fatalf("expected valid variant, got %v", err)
	}
}

func TestValidateBlocks_EmptyVariantOK(t *testing.T) {
	b := []Block{{Type: BlockAbout, Config: cfg(t, map[string]string{"text": "hi"})}}
	if err := ValidateBlocks(b); err != nil {
		t.Fatalf("empty variant must be allowed (defaults), got %v", err)
	}
}

func TestValidateBlocks_UnknownVariant(t *testing.T) {
	b := []Block{{Type: BlockStats, Variant: "bogus", Config: cfg(t, map[string]any{})}}
	if err := ValidateBlocks(b); err == nil {
		t.Fatal("expected error for unknown variant")
	}
}

func TestValidateBlocks_NewTypesAccepted(t *testing.T) {
	for _, ty := range []string{BlockContinueWatching, BlockAnimeDNA, BlockCompatibility} {
		if err := ValidateBlocks([]Block{{Type: ty, Config: cfg(t, map[string]any{})}}); err != nil {
			t.Fatalf("auto type %s should validate, got %v", ty, err)
		}
	}
}

func TestValidateBlocks_OpEdLimit(t *testing.T) {
	ids := make([]string, MaxBlockItems+1)
	for i := range ids { ids[i] = "t" }
	b := []Block{{Type: BlockOpEd, Config: cfg(t, map[string][]string{"theme_ids": ids})}}
	if err := ValidateBlocks(b); err == nil {
		t.Fatal("expected error: too many op_ed theme_ids")
	}
}

func TestValidateBlocks_ClampsSize(t *testing.T) {
	// favorite_anime/row bounds: W2..4, H1..1, default 4x1
	blocks := []Block{{Type: BlockFavoriteAnime, Variant: "row", Width: 1, Height: 3,
		Config: cfg(t, map[string][]string{"anime_ids": {"a"}})}}
	if err := ValidateBlocks(blocks); err != nil {
		t.Fatalf("expected clamp, got error: %v", err)
	}
	if blocks[0].Width != 2 || blocks[0].Height != 1 {
		t.Fatalf("want 2x1 after clamp, got %dx%d", blocks[0].Width, blocks[0].Height)
	}
}

func TestValidateBlocks_BackfillsDefaultSize(t *testing.T) {
	blocks := []Block{{Type: BlockStats, Variant: "tiles"}} // w/h absent
	if err := ValidateBlocks(blocks); err != nil {
		t.Fatal(err)
	}
	if blocks[0].Width != 2 || blocks[0].Height != 1 {
		t.Fatalf("want default 2x1, got %dx%d", blocks[0].Width, blocks[0].Height)
	}
}

func TestSizeFor_FallsBackToDefaultVariant(t *testing.T) {
	got := SizeFor(BlockAbout, "") // empty -> default variant "quote"
	if got.DefW != 2 || got.DefH != 1 {
		t.Fatalf("about default want 2x1, got %dx%d", got.DefW, got.DefH)
	}
}
