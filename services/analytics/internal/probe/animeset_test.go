package probe

import (
	"context"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAnimeSet_AnchorAlways(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"success":true,"data":{"cards":[{"anime_id":"a"},{"anime_id":"b"},{"anime_id":"c"}]}}`))
	}))
	defer srv.Close()
	as := NewHTTPAnimeSet(srv.URL, "ANCHOR", srv.Client(), rand.New(rand.NewSource(1)))
	refs, err := as.Resolve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	bySlot := map[AnimeSlot]string{}
	for _, r := range refs {
		bySlot[r.Slot] = r.UUID
	}
	if bySlot[SlotAnchor] != "ANCHOR" || bySlot[SlotFeatured] != "a" {
		t.Fatalf("got %+v", bySlot)
	}
	if _, ok := bySlot[SlotSpotlightRandom]; !ok {
		t.Fatalf("expected spotlight_random")
	}
}

func TestAnimeSet_SpotlightDown_AnchorOnly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()
	as := NewHTTPAnimeSet(srv.URL, "ANCHOR", srv.Client(), rand.New(rand.NewSource(1)))
	refs, _ := as.Resolve(context.Background())
	if len(refs) != 1 || refs[0].Slot != SlotAnchor {
		t.Fatalf("want anchor-only, got %+v", refs)
	}
}
