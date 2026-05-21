package spotlight

import "time"

// DateSeedUTC returns the date-derived integer used by deterministic
// per-day pickers (HSB-BE-10 anime_of_day, HSB-BE-11 random_tail).
// UTC is the source of truth so a server timezone change does not
// shift the picked anime mid-day. Formula: YYYY*100*32 + MM*32 + DD.
//
// Example: 2026-05-21 UTC → 2026*100*32 + 5*32 + 21 == 6483381.
func DateSeedUTC(t time.Time) int {
	u := t.UTC()
	return u.Year()*100*32 + int(u.Month())*32 + u.Day()
}

// DateKeyUTC returns 'YYYY-MM-DD' (UTC) — the suffix used in
// spotlight:<card>:<date> Redis keys (HSB-NF-03 prefix convention).
func DateKeyUTC(t time.Time) string {
	return t.UTC().Format("2006-01-02")
}

// SnapshotKey returns the per-day fallback Redis key written best-effort
// by the aggregator after every successful Resolve. A nil userID maps to
// "anon"; a non-nil userID is interpolated verbatim. Per HSB-BE-04.
//
// Examples:
//
//	SnapshotKey(nil)        == "spotlight:snapshot:anon:2026-05-21"
//	id := "abc"; SnapshotKey(&id) == "spotlight:snapshot:abc:2026-05-21"
func SnapshotKey(userID *string) string {
	bucket := "anon"
	if userID != nil {
		bucket = *userID
	}
	return "spotlight:snapshot:" + bucket + ":" + DateKeyUTC(time.Now())
}
