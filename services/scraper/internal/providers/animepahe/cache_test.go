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
			name:   "expires_near_future_returns_headroom_minus_guard",
			url:    "https://kwik.cx/abc?expires=1700000060&token=xxx",
			want:   30 * time.Second,
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

// NOTE: TestJaroWinkler + TestNormalizeTitle migrated to
// services/scraper/internal/fuzzy/fuzzy_test.go in Phase 18 Plan 18-01 Task 2.
