package animepahe

import (
	"math"
	"testing"
	"time"
)

// TestComputeStreamTTL exercises the upstream-`expires`-aware TTL math used
// by Provider.GetStream when caching extracted Kwik HLS URLs.
//
// Cases (from plan 16-03 Task 1 behavior):
//
//   - expires = now+600s     → TTL clamped to streamTTLCap (5min)
//   - expires = now+60s      → TTL = ~30s (60s - 30s guard)
//   - expires = now-100s     → TTL = 0 (already expired; caller must not cache)
//   - URL with no expires=   → TTL = streamTTLFallback (5min)
func TestComputeStreamTTL(t *testing.T) {
	t.Parallel()
	now := time.Unix(1_700_000_000, 0)

	cases := []struct {
		name string
		url  string
		want time.Duration
		// approx: the TTL should be within +/- 2s of want (clock skew).
		approx bool
	}{
		{
			name: "expires_far_future_clamped_to_5min",
			url:  "https://kwik.cx/abc?expires=1700000600&token=xxx",
			want: 5 * time.Minute,
		},
		{
			name: "expires_near_future_returns_headroom_minus_guard",
			url:  "https://kwik.cx/abc?expires=1700000060&token=xxx",
			want: 30 * time.Second,
			approx: true,
		},
		{
			name: "expires_in_past_returns_zero",
			url:  "https://kwik.cx/abc?expires=1699999900&token=xxx",
			want: 0,
		},
		{
			name: "no_expires_param_falls_back_to_5min",
			url:  "https://kwik.cx/abc",
			want: 5 * time.Minute,
		},
		{
			name: "malformed_expires_falls_back_to_5min",
			url:  "https://kwik.cx/abc?expires=notanumber",
			want: 5 * time.Minute,
		},
		{
			name: "unparseable_url_falls_back_to_5min",
			url:  "://not a url",
			want: 5 * time.Minute,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := computeStreamTTL(tc.url, now)
			if tc.approx {
				delta := math.Abs(float64(got - tc.want))
				if delta > float64(2*time.Second) {
					t.Errorf("computeStreamTTL(%s) = %v; want ~%v (within 2s)", tc.url, got, tc.want)
				}
				return
			}
			if got != tc.want {
				t.Errorf("computeStreamTTL(%s) = %v; want %v", tc.url, got, tc.want)
			}
		})
	}
}

// TestJaroWinkler verifies the in-package Jaro-Winkler implementation against
// well-known reference values from the original Jaro-Winkler paper.
//
// The threshold used by Provider.FindID's fuzzy fallback is 0.85, so the
// table includes cases that bracket that boundary.
func TestJaroWinkler(t *testing.T) {
	t.Parallel()
	cases := []struct {
		a, b string
		// approx within 0.01.
		want float64
	}{
		{"naruto", "naruto", 1.00},                              // exact
		{"naruto", "narutoo", 0.97},                             // off-by-one
		{"naruto", "naruto shippuuden", 0.88},                   // prefix-match boost
		{"vinland saga", "vinland saga season 2", 0.92},         // shared prefix
		{"one piece", "two piece", 0.85},                        // partial-rewrite (e/o shared, prefix differs)
		{"naruto", "xxxxxxx", 0.00},                             // no common chars
		{"", "", 1.00},                                          // both empty: exact
		{"", "naruto", 0.00},                                    // one empty
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.a+"_vs_"+tc.b, func(t *testing.T) {
			t.Parallel()
			got := jaroWinkler(tc.a, tc.b)
			if math.Abs(got-tc.want) > 0.05 {
				t.Errorf("jaroWinkler(%q,%q) = %.4f; want ~%.4f", tc.a, tc.b, got, tc.want)
			}
		})
	}
}

// TestNormalizeTitle verifies the season-suffix folding used by Provider.FindID
// to bridge "Vinland Saga Season 2" / "Vinland Saga: 2nd Season" / "Vinland
// Saga Part 2" into one canonical form before Jaro-Winkler scoring.
func TestNormalizeTitle(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in, want string
	}{
		{"Vinland Saga: 2nd Season", "vinland saga season 2"},
		{"Vinland Saga Part 2", "vinland saga season 2"},
		{"Naruto", "naruto"},
		{"Naruto: Shippuuden", "naruto shippuuden"},
		{"  Spaces  collapsed  ", "spaces collapsed"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.in, func(t *testing.T) {
			t.Parallel()
			if got := normalizeTitle(tc.in); got != tc.want {
				t.Errorf("normalizeTitle(%q) = %q; want %q", tc.in, got, tc.want)
			}
		})
	}
}
