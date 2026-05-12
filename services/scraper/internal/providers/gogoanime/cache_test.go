package gogoanime

import "testing"

// Goldens live under services/scraper/testdata/gogoanime/ — used by tests in
// dto_test.go / client_test.go for HTML parse coverage. This file covers
// pure-Go TTL math without needing a fixture.

// TestComputeStreamTTL_StreamHGSignedURL verifies cache.go's computeStreamTTL
// behaviour for StreamHG/Earnvids signed URLs: the query param is
// &e=<seconds_to_live> (delta, not absolute Unix ts), so the TTL math
// converts via time.Duration(e) * time.Second rather than time.Unix(e, 0).
// Plan 18-02 implementation must match Phase 16's clamp shape
// (min(parsed-30s, 5min)).
func TestComputeStreamTTL_StreamHGSignedURL(t *testing.T) {
	t.Skip("RED — implementation arrives in Plan 18-02")
}
