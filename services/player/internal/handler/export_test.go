package handler

import (
	"testing"

	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
)

func intPtr(n int) *int { return &n }

// Bug: exported anime had null shikimori_id / mal_id because toExportEntry read
// them from the sparse anime_list.mal_id (*int, ~43% populated) instead of the
// catalog-owned animes row (100% populated, Shikimori ID == MAL ID).
func TestToExportEntry_IDsFromAnimeRow(t *testing.T) {
	e := &domain.AnimeListEntry{
		AnimeID: "uuid-1",
		MalID:   nil, // the common case that produced null exports
		Status:  "watching",
		Anime: &domain.AnimeInfo{
			ID:          "uuid-1",
			Name:        "Frieren",
			ShikimoriID: "52991",
			MALID:       "52991",
		},
	}
	got := toExportEntry(e)
	if got.ShikimoriID == nil || *got.ShikimoriID != 52991 {
		t.Errorf("ShikimoriID = %v, want 52991", got.ShikimoriID)
	}
	if got.MalID == nil || *got.MalID != 52991 {
		t.Errorf("MalID = %v, want 52991", got.MalID)
	}
}

// When there's no catalog row, fall back to the per-list MalID override for both
// fields (Shikimori ID == MAL ID).
func TestToExportEntry_FallbackToListMalID(t *testing.T) {
	e := &domain.AnimeListEntry{AnimeID: "uuid-2", MalID: intPtr(123), Anime: nil}
	got := toExportEntry(e)
	if got.MalID == nil || *got.MalID != 123 {
		t.Errorf("MalID fallback = %v, want 123", got.MalID)
	}
	if got.ShikimoriID == nil || *got.ShikimoriID != 123 {
		t.Errorf("ShikimoriID fallback = %v, want 123 (shiki==mal)", got.ShikimoriID)
	}
}

// A non-numeric/empty catalog ID must not crash and must leave the field nil
// (rather than exporting garbage).
func TestToExportEntry_EmptyIDsStayNil(t *testing.T) {
	e := &domain.AnimeListEntry{
		AnimeID: "uuid-3",
		MalID:   nil,
		Anime:   &domain.AnimeInfo{ID: "uuid-3", Name: "x", ShikimoriID: "", MALID: ""},
	}
	got := toExportEntry(e)
	if got.ShikimoriID != nil || got.MalID != nil {
		t.Errorf("empty catalog IDs should stay nil, got shiki=%v mal=%v", got.ShikimoriID, got.MalID)
	}
}
