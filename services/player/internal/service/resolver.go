package service

import (
	"strings"

	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
)

// tierNames maps tier numbers to human-readable names.
var tierNames = map[int]string{
	1: "per_anime",
	2: "user_global",
	3: "community",
	4: "pinned",
	5: "default",
}

// Resolve is a pure function that implements the 5-tier preference resolution
// algorithm. It selects the best WatchCombo from the available list based on
// user preferences, the Phase 6 weighted Tier 2 lock, community popularity,
// and pinned translations.
//
// It never crosses language or dub/sub boundaries once a lock is established
// (VAL-02).
func Resolve(
	userPref *domain.UserAnimePreference,
	tier2Lock *domain.Tier2Lock,
	community []domain.CommunityCombo,
	pinned []domain.PinnedTranslation,
	available []domain.WatchCombo,
) *domain.ResolvedCombo {
	if len(available) == 0 {
		return nil
	}

	var lockLang, lockType string

	// ── Tier 1: Per-anime preference ──────────────────────────
	if userPref != nil {
		// Lock language+type from the saved preference regardless of match
		lockLang = userPref.Language
		lockType = userPref.WatchType

		// Try exact match: same player + same translation_id
		for _, a := range available {
			if a.Player == userPref.Player && a.TranslationID == userPref.TranslationID {
				return resolved(a, 1)
			}
		}

		// Try title match: same translation_title in same language+type
		if a := findInLock(available, lockLang, lockType, userPref.TranslationTitle); a != nil {
			return resolved(*a, 1)
		}

		// Combo gone — lock is set, continue to Tier 2
	}

	// ── Tier 2: Weighted, time-decayed inference (Phase 6) ────
	// tier2Lock is nil when total weighted history is below the
	// min-confidence floor — caller has already counted the skip in
	// metrics.Tier2ThinSignalSkipTotal. We fall straight through to Tier 3.
	if tier2Lock != nil {
		// If no lock yet, lock from the coarse signal
		if lockLang == "" {
			lockLang = tier2Lock.Language
			lockType = tier2Lock.WatchType
		}

		// Match the fine-signal top translation_title within the lock.
		// VAL-02: only inside the locked language+watch_type — never crosses.
		if tier2Lock.TopTranslationTitle != "" {
			if a := findInLock(available, lockLang, lockType, tier2Lock.TopTranslationTitle); a != nil {
				return resolved(*a, 2)
			}
		}

		// Top translation not currently available — fall to Tier 3 (community)
		// rather than picking a random in-lock combo. The community signal at
		// Tier 3 will itself respect the lock.
	}

	// ── Tier 3: Community popularity ──────────────────────────
	if len(community) > 0 {
		if lockLang == "" {
			// New user: pick the most popular combo to set lock
			lockLang, lockType = mostPopularLock(community)
		}

		if best := topCommunityInLock(community, available, lockLang, lockType); best != nil {
			return resolved(*best, 3)
		}
	}

	// ── Tier 4: Pinned translations ───────────────────────────
	for _, p := range pinned {
		// Map pinned translation_type to watch_type
		pinnedWatchType := mapPinnedType(p.TranslationType)

		// Pinned translations are always Kodik = "ru" language
		pinnedLang := "ru"

		// Check against lock
		if lockLang != "" && (pinnedLang != lockLang || pinnedWatchType != lockType) {
			continue
		}

		// Match by translation_title in available
		if a := findInLock(available, pinnedLang, pinnedWatchType, p.TranslationTitle); a != nil {
			return resolved(*a, 4)
		}
	}

	// ── Tier 5: Default — first kodik sub ─────────────────────
	// Only return kodik sub if no lock, or lock matches ru+sub
	if lockLang != "" && (lockLang != "ru" || lockType != "sub") {
		return nil
	}

	for _, a := range available {
		if a.Player == "kodik" && a.Language == "ru" && a.WatchType == "sub" {
			return resolved(a, 5)
		}
	}

	return nil
}

// findInLock returns the first available combo matching the locked
// language+watch_type and the given translation_title, or nil if none match.
func findInLock(available []domain.WatchCombo, lockLang, lockType, title string) *domain.WatchCombo {
	for i := range available {
		a := available[i]
		if a.Language == lockLang && a.WatchType == lockType && a.TranslationTitle == title {
			return &a
		}
	}
	return nil
}

// mostPopularLock returns the language+watch_type of the community combo with
// the most viewers. community must be non-empty.
func mostPopularLock(community []domain.CommunityCombo) (lang, watchType string) {
	top := community[0]
	for _, c := range community[1:] {
		if c.Viewers > top.Viewers {
			top = c
		}
	}
	return top.Language, top.WatchType
}

// topCommunityInLock returns the available combo with the highest community
// viewer count that exists within the locked language+watch_type, or nil if
// none qualify.
func topCommunityInLock(community []domain.CommunityCombo, available []domain.WatchCombo, lockLang, lockType string) *domain.WatchCombo {
	var best *domain.WatchCombo
	var bestViewers int

	for _, c := range community {
		if c.Language != lockLang || c.WatchType != lockType {
			continue
		}
		// Check if this community combo exists in available
		if a := findInLock(available, c.Language, c.WatchType, c.TranslationTitle); a != nil {
			if best == nil || c.Viewers > bestViewers {
				best = a
				bestViewers = c.Viewers
			}
		}
	}
	return best
}

// resolved builds a ResolvedCombo from a WatchCombo and tier number.
func resolved(c domain.WatchCombo, tier int) *domain.ResolvedCombo {
	return &domain.ResolvedCombo{
		WatchCombo: c,
		Tier:       tierNames[tier],
		TierNumber: tier,
	}
}

// mapPinnedType converts pinned translation_type to watch_type.
func mapPinnedType(pinnedType string) string {
	switch strings.ToLower(pinnedType) {
	case "voice":
		return "dub"
	case "subtitles":
		return "sub"
	default:
		return pinnedType
	}
}
