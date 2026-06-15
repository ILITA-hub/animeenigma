package service

import (
	"testing"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
)

func TestCollapseByFranchise(t *testing.T) {
	in := []*domain.Anime{
		{ID: "jjk1", Franchise: "jujutsu_kaisen"}, // earliest first (repo orders aired_on ASC)
		{ID: "jjk2", Franchise: "jujutsu_kaisen"},
		{ID: "standalone-a", Franchise: ""},
		{ID: "standalone-b", Franchise: ""},
		{ID: "frieren", Franchise: "frieren"},
	}
	out := collapseByFranchise(in)

	var ids []string
	for _, a := range out {
		ids = append(ids, a.ID)
	}
	want := []string{"jjk1", "standalone-a", "standalone-b", "frieren"}
	if len(ids) != len(want) {
		t.Fatalf("want %v, got %v", want, ids)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("at %d want %s, got %s (full %v)", i, want[i], ids[i], ids)
		}
	}
}
