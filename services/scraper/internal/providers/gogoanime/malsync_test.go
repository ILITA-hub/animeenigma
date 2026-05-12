package gogoanime

import "testing"

// Goldens live under services/scraper/testdata/gogoanime/ — resolved by the
// goldenPath helper in client_test.go (same package).

// TestMalSync_NegativeCacheForGogoanime verifies the forward-compat probe:
// the gogoanime malsync client calls api.malsync.moe with slug "Gogoanime"
// (matching malsync.moe's Sites-key capitalization convention) but the
// response (Plan 18-01 Task 3 golden malsync_no_gogo.json) lacks the key.
// The client returns ("", false, nil) and caches the miss for 24h. SCRAPER-9ANI-01.
func TestMalSync_NegativeCacheForGogoanime(t *testing.T) {
	_ = goldenPath(t, "malsync_no_gogo.json")
	t.Skip("RED — implementation arrives in Plan 18-02")
}
