package probe

import (
	"context"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"testing"
)

// realSpotlightPayload is the real bare envelope shape returned by
// GET /api/home/spotlight — no "data" wrapper at the top level.
const realSpotlightPayload = `{
  "cards":[
    {"type":"featured","data":{"anime":{"id":"feat-uuid"}}},
    {"type":"random_tail","data":{"anime":{"id":"rand-uuid"}}},
    {"type":"latest_news","data":{"entries":[]}}
  ],
  "generated_at":"2026-06-20T00:00:00Z"
}`

// onlyNonAnimePayload has cards but none with data.anime.id → anchor-only.
const onlyNonAnimePayload = `{
  "cards":[
    {"type":"latest_news","data":{"entries":[]}},
    {"type":"platform_stats","data":{"users":42}}
  ],
  "generated_at":"2026-06-20T00:00:00Z"
}`

func TestAnimeSet_AnchorAlways(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(realSpotlightPayload))
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

	// Anchor must always be present.
	if bySlot[SlotAnchor] != "ANCHOR" {
		t.Fatalf("anchor missing or wrong, got %+v", bySlot)
	}
	// Featured slot must match the "featured"-type card.
	if bySlot[SlotFeatured] != "feat-uuid" {
		t.Fatalf("SlotFeatured want feat-uuid, got %+v", bySlot)
	}
	// spotlight_random must be one of the anime-bearing cards.
	sr, ok := bySlot[SlotSpotlightRandom]
	if !ok {
		t.Fatalf("SlotSpotlightRandom missing, got %+v", bySlot)
	}
	validUUIDs := map[string]bool{"feat-uuid": true, "rand-uuid": true}
	if !validUUIDs[sr] {
		t.Fatalf("SlotSpotlightRandom %q not in valid set %v", sr, validUUIDs)
	}
}

func TestAnimeSet_LatestNewsCardSkipped(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(realSpotlightPayload))
	}))
	defer srv.Close()

	as := NewHTTPAnimeSet(srv.URL, "ANCHOR", srv.Client(), rand.New(rand.NewSource(42)))
	refs, err := as.Resolve(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	// No slot UUID should be empty (latest_news has no data.anime.id and must be skipped).
	for _, r := range refs {
		if r.UUID == "" {
			t.Fatalf("got empty UUID in slot %v — non-anime card leaked", r.Slot)
		}
	}
	// Confirm only valid anime UUIDs appear (not some sentinel from latest_news).
	allowed := map[string]bool{"ANCHOR": true, "feat-uuid": true, "rand-uuid": true}
	for _, r := range refs {
		if !allowed[r.UUID] {
			t.Fatalf("unexpected UUID %q in refs — non-anime card must be skipped", r.UUID)
		}
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

// namedSpotlightPayload carries title fields on the anime cards so the resolver
// can populate AnimeRef.Name.
const namedSpotlightPayload = `{
  "cards":[
    {"type":"featured","data":{"anime":{"id":"feat-uuid","name":"Gintama","name_ru":"Гинтама"}}},
    {"type":"random_tail","data":{"anime":{"id":"rand-uuid","name":"Bleach"}}}
  ],
  "generated_at":"2026-06-20T00:00:00Z"
}`

func TestAnimeSet_PopulatesNames(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Anchor name comes from the detail endpoint; spotlight from the list.
		if r.URL.Path == "/api/anime/ANCHOR" {
			w.Write([]byte(`{"data":{"name":"Sousou no Frieren","name_ru":"Фрирен"}}`))
			return
		}
		w.Write([]byte(namedSpotlightPayload))
	}))
	defer srv.Close()

	as := NewHTTPAnimeSet(srv.URL, "ANCHOR", srv.Client(), rand.New(rand.NewSource(1)))
	refs, err := as.Resolve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	bySlot := map[AnimeSlot]AnimeRef{}
	for _, r := range refs {
		bySlot[r.Slot] = r
	}
	// Anchor name from the detail endpoint, Russian preferred.
	if bySlot[SlotAnchor].Name != "Фрирен" {
		t.Fatalf("anchor name = %q, want Фрирен", bySlot[SlotAnchor].Name)
	}
	// Featured name from the spotlight card, Russian preferred.
	if bySlot[SlotFeatured].Name != "Гинтама" {
		t.Fatalf("featured name = %q, want Гинтама", bySlot[SlotFeatured].Name)
	}
}

func TestAnimeSet_OnlyNonAnimeCards_AnchorOnly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(onlyNonAnimePayload))
	}))
	defer srv.Close()

	as := NewHTTPAnimeSet(srv.URL, "ANCHOR", srv.Client(), rand.New(rand.NewSource(1)))
	refs, err := as.Resolve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 || refs[0].Slot != SlotAnchor {
		t.Fatalf("want anchor-only for non-anime payload, got %+v", refs)
	}
}

func TestSortByPopularity(t *testing.T) {
	cases := []struct {
		name    string
		in      []AnimeRef
		wantIDs []string
	}{
		{
			name: "out-of-order scores sorted descending",
			in: []AnimeRef{
				{UUID: "a", Score: 6.1},
				{UUID: "b", Score: 8.9},
				{UUID: "c", Score: 7.5},
			},
			wantIDs: []string{"b", "c", "a"},
		},
		{
			name: "already sorted stays sorted",
			in: []AnimeRef{
				{UUID: "x", Score: 9.0},
				{UUID: "y", Score: 7.0},
				{UUID: "z", Score: 5.0},
			},
			wantIDs: []string{"x", "y", "z"},
		},
		{
			name: "equal scores preserve original order (stable)",
			in: []AnimeRef{
				{UUID: "p", Score: 8.0},
				{UUID: "q", Score: 8.0},
				{UUID: "r", Score: 5.0},
			},
			wantIDs: []string{"p", "q", "r"},
		},
		{
			name: "single element",
			in:   []AnimeRef{{UUID: "solo", Score: 7.7}},
			wantIDs: []string{"solo"},
		},
		{
			name:    "empty slice",
			in:      []AnimeRef{},
			wantIDs: []string{},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := sortByPopularity(tc.in)
			if len(got) != len(tc.wantIDs) {
				t.Fatalf("len=%d want %d", len(got), len(tc.wantIDs))
			}
			for i, id := range tc.wantIDs {
				if got[i].UUID != id {
					t.Errorf("pos %d: got %q want %q (full: %v)", i, got[i].UUID, id, uuids(got))
				}
			}
			// Original slice must not be mutated.
			for i, orig := range tc.in {
				if orig.UUID != tc.in[i].UUID {
					t.Errorf("input mutated at pos %d", i)
				}
			}
		})
	}
}

func uuids(refs []AnimeRef) []string {
	ids := make([]string, len(refs))
	for i, r := range refs {
		ids[i] = r.UUID
	}
	return ids
}
