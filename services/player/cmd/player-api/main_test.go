package main

// Wave-0 scaffold for SOCIAL-NF-02 (migration idempotency). Mapped to
// `01-VALIDATION.md` row 01-Migrate-01. Plan 01 will replace t.Skip with the
// real two-pass migration test (first run copies reviews → anime_list and
// drops `reviews`; second run is a no-op).
//
// This file lives in package `main` (same dir as main.go) so plan 01 can
// reach any unexported migration helper it extracts (e.g. runSocialMigration).

import "testing"

// TestSocialMigration_Idempotent validates SOCIAL-NF-02: running the social
// migration block twice produces the same DB state as running it once; the
// second run logs nothing.
func TestSocialMigration_Idempotent(t *testing.T) {
	t.Skip("Wave 0 scaffold — implementation in plan 01")
}
