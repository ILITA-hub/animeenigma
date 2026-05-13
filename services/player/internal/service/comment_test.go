package service

// Wave-0 scaffold for SOCIAL-04f (rate limit) and SOCIAL-05 (activity event
// emission). Mapped to `01-VALIDATION.md` rows 01-Comment-06 and 01-Activity-01.
// Plan 03 fills both bodies.

import "testing"

// TestCommentService_RateLimit validates SOCIAL-04f: the 11th POST inside an
// hour for the same (user, anime) pair returns errors.RateLimited().
func TestCommentService_RateLimit(t *testing.T) {
	t.Skip("Wave 0 scaffold — implementation in plan 03")
}

// TestCommentService_EmitsActivity validates SOCIAL-05: every successful
// CreateComment writes exactly one activity_events row with type='comment',
// the truncated content preview (≤ 300 runes + ellipsis), and the user/anime
// metadata.
func TestCommentService_EmitsActivity(t *testing.T) {
	t.Skip("Wave 0 scaffold — implementation in plan 03")
}
