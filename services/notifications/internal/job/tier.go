package job

import (
	"time"

	"github.com/ILITA-hub/animeenigma/services/notifications/internal/domain"
)

// TierWindows are the cadence-tier durations (spec §4).
type TierWindows struct {
	Hot   time.Duration // ± window on next_episode_at that counts as "imminent/just-aired"
	Warm  time.Duration // minimum spacing between checks for non-hot titles
	Floor time.Duration // hard delivery floor: no combo checked less often than this
}

// within reports whether t is within ±w of now.
func within(t, now time.Time, w time.Duration) bool {
	d := t.Sub(now)
	if d < 0 {
		d = -d
	}
	return d <= w
}

// tierDue decides whether an anime should be checked this run.
//   - never checked → always (bootstrap / fail-safe).
//   - checked older than Floor → always (delivery guarantee).
//   - hot (next episode within ±Hot, or airing unknown) → every run.
//   - warm → only when Warm has elapsed since the last check.
func tierDue(nextEp *time.Time, lastChecked time.Time, checkedKnown bool, now time.Time, w TierWindows) bool {
	if !checkedKnown {
		return true
	}
	if now.Sub(lastChecked) >= w.Floor {
		return true
	}
	hot := nextEp == nil || within(*nextEp, now, w.Hot)
	if hot {
		return true
	}
	return now.Sub(lastChecked) >= w.Warm
}

// tierFilter returns the combos to check this run: every combo whose anime is
// due. Grouping by anime keeps the delivery floor per-combo (including an
// anime includes all its combos).
func tierFilter(combos []domain.Combo, airing map[string]*time.Time, lastChecked map[string]time.Time, now time.Time, w TierWindows) []domain.Combo {
	decision := map[string]bool{}
	out := make([]domain.Combo, 0, len(combos))
	for _, c := range combos {
		due, done := decision[c.AnimeID]
		if !done {
			lc, known := lastChecked[c.AnimeID]
			due = tierDue(airing[c.AnimeID], lc, known, now, w)
			decision[c.AnimeID] = due
		}
		if due {
			out = append(out, c)
		}
	}
	return out
}
