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
