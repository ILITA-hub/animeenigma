package spotlight

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
)

// TestTypes_JSONShape pins the wire shape of every Card variant against
// the TypeScript discriminated union in design doc §4.1. If this test
// fails, the Phase 2 frontend breaks.
func TestTypes_JSONShape(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name           string
		card           Card
		wantType       string
		wantInnerKey   string // a key that MUST be present in `data`
		mustNotContain string // a substring that MUST NOT appear (omitempty check)
	}{
		{
			name:           "featured omits reason_i18n_key when empty",
			card:           Card{Type: "featured", Data: FeaturedData{Anime: domain.Anime{Name: "X"}}},
			wantType:       "featured",
			wantInnerKey:   "anime",
			mustNotContain: "reason_i18n_key",
		},
		{
			name:         "random_tail has anime key",
			card:         Card{Type: "random_tail", Data: RandomTailData{Anime: domain.Anime{Name: "Y"}}},
			wantType:     "random_tail",
			wantInnerKey: "anime",
		},
		{
			name:         "latest_news has entries key",
			card:         Card{Type: "latest_news", Data: LatestNewsData{Entries: []ChangelogEntry{{Date: "2026-05-21", Message: "hi"}}}},
			wantType:     "latest_news",
			wantInnerKey: "entries",
		},
		{
			name: "platform_stats has hero key",
			card: Card{Type: "platform_stats", Data: PlatformStatsData{
				Hero:  StatsHero{WorkingOK: true, UptimeQuip: "fine", Service: "catalog", Tagline: "ok"},
				Tiles: []StatsTile{},
			}},
			wantType:     "platform_stats",
			wantInnerKey: "hero",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			raw, err := json.Marshal(tc.card)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}

			// Top-level keys must be exactly {type, priority, data}.
			var top map[string]any
			if err := json.Unmarshal(raw, &top); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if len(top) != 3 {
				t.Fatalf("expected exactly 3 top-level keys (type, priority, data); got %d: %v", len(top), top)
			}
			if _, ok := top["type"]; !ok {
				t.Fatalf("missing top-level key `type` in %s", raw)
			}
			if _, ok := top["priority"]; !ok {
				t.Fatalf("missing top-level key `priority` in %s", raw)
			}
			if _, ok := top["data"]; !ok {
				t.Fatalf("missing top-level key `data` in %s", raw)
			}

			gotType, _ := top["type"].(string)
			if gotType != tc.wantType {
				t.Errorf("type = %q; want %q", gotType, tc.wantType)
			}

			inner, ok := top["data"].(map[string]any)
			if !ok {
				t.Fatalf("data is not an object: %v", top["data"])
			}
			if _, ok := inner[tc.wantInnerKey]; !ok {
				t.Errorf("inner key %q missing from data: %v", tc.wantInnerKey, inner)
			}

			if tc.mustNotContain != "" && strings.Contains(string(raw), tc.mustNotContain) {
				t.Errorf("output unexpectedly contains %q: %s", tc.mustNotContain, raw)
			}
		})
	}
}

// TestNewCardDataShapes_RoundTrip verifies the 5 Phase 3 Data structs
// round-trip through json.Marshal + json.Unmarshal without dropping any
// declared fields and with the correct omitempty behavior. Regression guard
// against accidental tag drift.
func TestNewCardDataShapes_RoundTrip(t *testing.T) {
	t.Parallel()

	t.Run("PersonalPickData", func(t *testing.T) {
		t.Parallel()
		in := PersonalPickData{
			Items: []PersonalPickItem{
				{Anime: domain.Anime{ID: "id1", Name: "A"}, ReasonI18nKey: "spotlight.personalPick.reason.trending"},
				{Anime: domain.Anime{ID: "id2", Name: "B"}},
			},
			Source: "trending",
		}
		raw, err := json.Marshal(in)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		var out PersonalPickData
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if len(out.Items) != 2 {
			t.Fatalf("expected 2 items, got %d", len(out.Items))
		}
		if out.Source != "trending" {
			t.Errorf("Source = %q; want trending", out.Source)
		}
		if out.Items[0].ReasonI18nKey != "spotlight.personalPick.reason.trending" {
			t.Errorf("ReasonI18nKey lost: %q", out.Items[0].ReasonI18nKey)
		}
		// omitempty check — item 2 has no ReasonI18nKey
		if strings.Contains(string(raw), `"reason_i18n_key":""`) {
			t.Errorf("omitempty failed for empty ReasonI18nKey: %s", raw)
		}
	})

	t.Run("TelegramNewsData", func(t *testing.T) {
		t.Parallel()
		in := TelegramNewsData{
			Posts: []TelegramPost{
				{
					Title:    "T",
					Excerpt:  "ex",
					Link:     "https://t.me/x/1",
					Date:     "2026-05-21T12:00:00Z",
					ImageURL: "https://cdn4.telesco.pe/file/abc.jpg",
				},
				{Excerpt: "ex2"},
			},
		}
		raw, err := json.Marshal(in)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		var out TelegramNewsData
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if len(out.Posts) != 2 {
			t.Fatalf("expected 2 posts, got %d", len(out.Posts))
		}
		if out.Posts[0].Link != "https://t.me/x/1" {
			t.Errorf("Link lost: %q", out.Posts[0].Link)
		}
		if out.Posts[0].ImageURL != "https://cdn4.telesco.pe/file/abc.jpg" {
			t.Errorf("ImageURL lost: %q", out.Posts[0].ImageURL)
		}
		// JSON tag must be `image_url` (snake_case end-to-end — Pitfall 8).
		if !strings.Contains(string(raw), `"image_url":"https://cdn4.telesco.pe/file/abc.jpg"`) {
			t.Errorf("expected image_url JSON tag, got: %s", raw)
		}
		// omitempty checks: post 2 has no title/link/date/image_url
		if strings.Contains(string(raw), `"title":""`) {
			t.Errorf("omitempty failed for empty Title: %s", raw)
		}
		if strings.Contains(string(raw), `"link":""`) {
			t.Errorf("omitempty failed for empty Link: %s", raw)
		}
		if strings.Contains(string(raw), `"image_url":""`) {
			t.Errorf("omitempty failed for empty ImageURL: %s", raw)
		}
	})

	t.Run("NowWatchingData", func(t *testing.T) {
		t.Parallel()
		in := NowWatchingData{
			Sessions: []NowWatchingSession{
				{
					Username:      "user1",
					PublicID:      "user-1",
					AnimeID:       "anime-1",
					AnimeName:     "Anime One",
					AnimeNameRU:   "Аниме Один",
					PosterURL:     "/p.jpg",
					EpisodeNumber: 5,
					UpdatedAt:     "2026-05-21T12:00:00Z",
				},
			},
		}
		raw, err := json.Marshal(in)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		var out NowWatchingData
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if len(out.Sessions) != 1 {
			t.Fatalf("expected 1 session, got %d", len(out.Sessions))
		}
		s := out.Sessions[0]
		if s.Username != "user1" || s.PublicID != "user-1" {
			t.Errorf("public fields lost: %+v", s)
		}
		if s.EpisodeNumber != 5 {
			t.Errorf("EpisodeNumber lost: %d", s.EpisodeNumber)
		}
	})

	t.Run("NotTimeYetData", func(t *testing.T) {
		t.Parallel()
		in := NotTimeYetData{Anime: domain.Anime{ID: "a1", Name: "A"}, Status: "planned"}
		raw, err := json.Marshal(in)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		var out NotTimeYetData
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if out.Status != "planned" {
			t.Errorf("Status lost: %q", out.Status)
		}
		if out.Anime.ID != "a1" {
			t.Errorf("Anime.ID lost: %q", out.Anime.ID)
		}
	})

	t.Run("ContinueWatchingNewData", func(t *testing.T) {
		t.Parallel()
		in := ContinueWatchingNewData{
			Anime:              domain.Anime{ID: "a1", Name: "A"},
			LastWatchedEpisode: 3,
			NewEpisodeNumber:   5,
		}
		raw, err := json.Marshal(in)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		var out ContinueWatchingNewData
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if out.LastWatchedEpisode != 3 || out.NewEpisodeNumber != 5 {
			t.Errorf("episode fields lost: %+v", out)
		}
	})
}

// TestPersonalPickData_ItemsMarshalAsArray ensures an empty Items slice
// marshals as `"items":[]` (mirror of the `Cards: []Card{}` rule). Phase 2
// frontend treats `null` as a parse failure.
func TestPersonalPickData_ItemsMarshalAsArray(t *testing.T) {
	t.Parallel()
	data := PersonalPickData{Items: []PersonalPickItem{}, Source: "trending"}
	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(raw), `"items":[]`) {
		t.Fatalf(`expected '"items":[]' in output, got: %s`, raw)
	}
	if strings.Contains(string(raw), `"items":null`) {
		t.Fatalf(`output must NOT contain '"items":null', got: %s`, raw)
	}
}

// TestTypes_EmptyCardsMarshalArray guards against `var cards []Card`
// producing `"cards":null`. The Phase 2 frontend rejects null.
func TestTypes_EmptyCardsMarshalArray(t *testing.T) {
	t.Parallel()
	resp := Response{Cards: []Card{}, GeneratedAt: "2026-05-21T00:00:00Z"}
	raw, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(raw), `"cards":[]`) {
		t.Fatalf(`expected output to contain '"cards":[]', got: %s`, raw)
	}
	if strings.Contains(string(raw), `"cards":null`) {
		t.Fatalf(`output must NOT contain '"cards":null', got: %s`, raw)
	}
}

// TestDateSeedUTC asserts the seed formula and timezone-invariance.
func TestDateSeedUTC(t *testing.T) {
	t.Parallel()

	msk := time.FixedZone("MSK", 3*3600)
	cases := []struct {
		name string
		in   time.Time
		want int
	}{
		{
			name: "May 21 2026 midnight UTC",
			in:   time.Date(2026, 5, 21, 0, 0, 0, 0, time.UTC),
			want: 2026*100*32 + 5*32 + 21, // 6483381
		},
		{
			name: "May 21 2026 23:00 MSK (still May 21 UTC)",
			in:   time.Date(2026, 5, 21, 23, 0, 0, 0, msk),
			want: 6483381,
		},
		{
			name: "May 22 2026 00:30 MSK (UTC May 21 21:30)",
			in:   time.Date(2026, 5, 22, 0, 30, 0, 0, msk),
			want: 6483381,
		},
		{
			name: "Jan 1 2027 noon UTC",
			in:   time.Date(2027, 1, 1, 12, 0, 0, 0, time.UTC),
			want: 2027*100*32 + 1*32 + 1, // 6486433
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := DateSeedUTC(tc.in)
			if got != tc.want {
				t.Errorf("DateSeedUTC(%v) = %d; want %d", tc.in, got, tc.want)
			}
		})
	}
}

// TestDateKeyUTC asserts the key format and timezone-invariance.
func TestDateKeyUTC(t *testing.T) {
	t.Parallel()

	msk := time.FixedZone("MSK", 3*3600)
	cases := []struct {
		name string
		in   time.Time
		want string
	}{
		{
			name: "May 21 2026 03:00 UTC",
			in:   time.Date(2026, 5, 21, 3, 0, 0, 0, time.UTC),
			want: "2026-05-21",
		},
		{
			name: "May 22 2026 00:30 MSK -> May 21 UTC",
			in:   time.Date(2026, 5, 22, 0, 30, 0, 0, msk),
			want: "2026-05-21",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := DateKeyUTC(tc.in)
			if got != tc.want {
				t.Errorf("DateKeyUTC(%v) = %q; want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestSnapshotKey asserts the bucket + date prefix structure. We don't
// freeze time.Now() (SnapshotKey reads it internally); instead we assert
// the prefix and the YYYY-MM-DD-shaped suffix.
func TestSnapshotKey(t *testing.T) {
	t.Parallel()

	dateRE := regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

	gotAnon := SnapshotKey(nil)
	const wantAnonPrefix = "spotlight:snapshot:anon:"
	if !strings.HasPrefix(gotAnon, wantAnonPrefix) {
		t.Fatalf("SnapshotKey(nil) = %q; want prefix %q", gotAnon, wantAnonPrefix)
	}
	anonSuffix := strings.TrimPrefix(gotAnon, wantAnonPrefix)
	if !dateRE.MatchString(anonSuffix) {
		t.Errorf("SnapshotKey(nil) suffix = %q; want YYYY-MM-DD shape", anonSuffix)
	}

	uid := "abc"
	gotUser := SnapshotKey(&uid)
	const wantUserPrefix = "spotlight:snapshot:abc:"
	if !strings.HasPrefix(gotUser, wantUserPrefix) {
		t.Fatalf("SnapshotKey(&\"abc\") = %q; want prefix %q", gotUser, wantUserPrefix)
	}
	userSuffix := strings.TrimPrefix(gotUser, wantUserPrefix)
	if !dateRE.MatchString(userSuffix) {
		t.Errorf("SnapshotKey(&\"abc\") suffix = %q; want YYYY-MM-DD shape", userSuffix)
	}
}

func TestPlatformStatsData_RoundTrip(t *testing.T) {
	t.Parallel()
	pct := 99.4
	in := PlatformStatsData{
		Hero: StatsHero{
			WorkingOK:     true,
			UptimePercent: &pct,
			UptimeQuip:    "ОЧЕНЬ МНОГО",
			Service:       "catalog",
			UXDelta:       "+5 (Tremendous)",
			CDI:           "0.00 * 99",
			MVQ:           "Dragon 99%/99%",
			Tagline:       "Лучшая платформа. Поверьте.",
		},
		Tiles: []StatsTile{
			{Label: "Запросов обработано", Value: 48201, Window: "day", Format: "int"},
		},
	}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out PlatformStatsData
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Hero.UptimeQuip != "ОЧЕНЬ МНОГО" || out.Hero.UptimePercent == nil || *out.Hero.UptimePercent != 99.4 {
		t.Fatalf("hero round-trip mismatch: %+v", out.Hero)
	}
	if len(out.Tiles) != 1 || out.Tiles[0].Value != 48201 || out.Tiles[0].Window != "day" {
		t.Fatalf("tiles round-trip mismatch: %+v", out.Tiles)
	}
}

func TestPlatformStatsData_EmptyTilesMarshalArray(t *testing.T) {
	t.Parallel()
	b, err := json.Marshal(PlatformStatsData{Hero: StatsHero{}, Tiles: []StatsTile{}})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(b), `"tiles":[]`) {
		t.Fatalf("expected tiles:[] in %s", b)
	}
}

func TestStatsHero_UptimePercentOmittedWhenNil(t *testing.T) {
	t.Parallel()
	b, err := json.Marshal(StatsHero{WorkingOK: false, UptimeQuip: "x", Service: "s", Tagline: "t"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(b), "uptime_percent") {
		t.Fatalf("uptime_percent should be omitted when nil: %s", b)
	}
}

// TestNewAggregator_ConstructsEmpty confirms the constructor never leaves
// the unexported resolvers slice as nil — a nil slice would NPE when the
// Plan 03 fan-out loop ranges over it.
func TestNewAggregator_ConstructsEmpty(t *testing.T) {
	t.Parallel()
	a := NewAggregator(nil, nil, nil)
	if a == nil {
		t.Fatal("NewAggregator returned nil")
	}
	if a.Resolvers() == nil {
		t.Fatal("Aggregator.Resolvers() must return non-nil empty slice, got nil")
	}
	if len(a.Resolvers()) != 0 {
		t.Errorf("expected zero resolvers; got %d", len(a.Resolvers()))
	}

	// And the stub Resolve must not panic when called with no resolvers.
	resp, err := a.Resolve(context.Background(), nil)
	if err != nil {
		t.Fatalf("Resolve returned err: %v", err)
	}
	if resp == nil {
		t.Fatal("Resolve returned nil response")
	}
	if resp.Cards == nil {
		t.Error("Resolve response Cards must be non-nil empty slice")
	}
	if len(resp.Cards) != 0 {
		t.Errorf("expected empty cards; got %d", len(resp.Cards))
	}
	if resp.GeneratedAt == "" {
		t.Error("Resolve response GeneratedAt must be populated")
	}
}
