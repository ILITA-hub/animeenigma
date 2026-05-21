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
			name:           "anime_of_day omits reason_i18n_key when empty",
			card:           Card{Type: "anime_of_day", Data: AnimeOfDayData{Anime: domain.Anime{Name: "X"}}},
			wantType:       "anime_of_day",
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
			name:         "platform_stats has metrics key",
			card:         Card{Type: "platform_stats", Data: PlatformStatsData{Metrics: []StatsMetric{{Key: "anime_added_7d", Value: 42}}}},
			wantType:     "platform_stats",
			wantInnerKey: "metrics",
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

			// Top-level keys must be exactly {type, data}.
			var top map[string]any
			if err := json.Unmarshal(raw, &top); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if len(top) != 2 {
				t.Fatalf("expected exactly 2 top-level keys (type, data); got %d: %v", len(top), top)
			}
			if _, ok := top["type"]; !ok {
				t.Fatalf("missing top-level key `type` in %s", raw)
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

// TestTypes_StatsMetric_DeltaOmittedWhenNil confirms `delta` is absent
// from the JSON when StatsMetric.Delta is nil.
func TestTypes_StatsMetric_DeltaOmittedWhenNil(t *testing.T) {
	t.Parallel()
	m := StatsMetric{Key: "x", Value: 5}
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(raw), `"delta"`) {
		t.Fatalf(`output must NOT contain 'delta' when Delta is nil, got: %s`, raw)
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
