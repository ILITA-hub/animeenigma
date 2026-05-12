package gogoanime

import (
	"testing"
	"time"
)

// TestComputeStreamTTL_StreamHGSignedURL verifies cache.go's computeStreamTTL
// behaviour for StreamHG/Earnvids signed URLs. The query param is
// &e=<seconds_to_live> (delta, not absolute Unix ts), so the TTL math
// converts via time.Duration(e) * time.Second rather than time.Unix(e, 0).
// Phase 16's clamp shape (min(parsed-30s, 5min)) is preserved.
func TestComputeStreamTTL_StreamHGSignedURL(t *testing.T) {
	t.Parallel()
	now := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	cases := []struct {
		name     string
		streamURL string
		now      time.Time
		want     time.Duration
		// allowSlack accepts any duration within +/- 1s of want (covers
		// floor/ceil rounding around the 30s guard).
		allowSlack bool
	}{
		{
			// (s + e) gives a future expiry many minutes out → capped to 5min.
			name:      "absolute_far_future_caps_at_5min",
			streamURL: "https://x.premilkyway.com/master.m3u8?t=abc&s=1747000000&e=129600",
			now:       now,
			want:      streamTTLCap,
		},
		{
			// s+e in the past → expired → 0.
			name:      "absolute_expired_returns_zero",
			streamURL: "https://x.premilkyway.com/master.m3u8?t=abc&s=1700000000&e=10",
			now:       now,
			want:      0,
		},
		{
			// No e= at all (vibeplayer-style static m3u8) → fallback.
			name:      "no_expiry_param_returns_fallback",
			streamURL: "https://vibeplayer.site/abc.m3u8",
			now:       now,
			want:      streamTTLFallback,
		},
		{
			// e=300, no s= → delta-from-now interpretation, headroom = 300s - 30s = 270s.
			name:       "delta_only_300_seconds_minus_guard",
			streamURL:  "https://x.example.com/abc?e=300",
			now:        now,
			want:       4*time.Minute + 30*time.Second,
			allowSlack: true,
		},
		{
			// e=10 only → 10s - 30s guard = -20s → 0 (expired).
			name:      "delta_only_too_small_returns_zero",
			streamURL: "https://x.example.com/abc?e=10",
			now:       now,
			want:      0,
		},
		{
			// e=99999 only → very large delta-from-now, capped at 5min.
			name:      "delta_only_large_caps_at_5min",
			streamURL: "https://x.example.com/abc?e=99999",
			now:       now,
			want:      streamTTLCap,
		},
		{
			// Non-integer e= falls back to streamTTLFallback (parse error path).
			name:      "non_integer_e_returns_fallback",
			streamURL: "https://x.example.com/abc?e=notanumber",
			now:       now,
			want:      streamTTLFallback,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := computeStreamTTL(tc.streamURL, tc.now)
			if tc.allowSlack {
				diff := got - tc.want
				if diff < -time.Second || diff > time.Second {
					t.Errorf("computeStreamTTL(%q) = %v; want %v (±1s)", tc.streamURL, got, tc.want)
				}
				return
			}
			if got != tc.want {
				t.Errorf("computeStreamTTL(%q) = %v; want %v", tc.streamURL, got, tc.want)
			}
		})
	}
}
