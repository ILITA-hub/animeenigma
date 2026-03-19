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
// user preferences, global favorites, community popularity, and pinned translations.
//
// It never crosses language or dub/sub boundaries once a lock is established.
func Resolve(
	userPref *domain.UserAnimePreference,
	globalFav *domain.ComboCount,
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
		for _, a := range available {
			if a.Language == lockLang && a.WatchType == lockType &&
				a.TranslationTitle == userPref.TranslationTitle {
				return resolved(a, 1)
			}
		}

		// Combo gone — lock is set, continue to Tier 2
	}

	// ── Tier 2: User's global favorite #1 ─────────────────────
	if globalFav != nil {
		// If no lock yet, lock from global favorite
		if lockLang == "" {
			lockLang = globalFav.Language
			lockType = globalFav.WatchType
		}

		// Only check exact translation_title match, filtered by lock
		for _, a := range available {
			if a.Language == lockLang && a.WatchType == lockType &&
				a.TranslationTitle == globalFav.TranslationTitle {
				return resolved(a, 2)
			}
		}

		// Not found — do NOT try #2, #3 favorites. Fall to Tier 3.
	}

	// ── Tier 3: Community popularity ──────────────────────────
	if len(community) > 0 {
		if lockLang == "" {
			// New user: pick the most popular combo to set lock
			top := community[0]
			for _, c := range community[1:] {
				if c.Viewers > top.Viewers {
					top = c
				}
			}
			lockLang = top.Language
			lockType = top.WatchType
		}

		// Filter community to locked language+type and find top available
		type candidate struct {
			combo   domain.WatchCombo
			viewers int
		}
		var best *candidate

		for _, c := range community {
			if c.Language != lockLang || c.WatchType != lockType {
				continue
			}
			// Check if this community combo exists in available
			for _, a := range available {
				if a.Language == c.Language && a.WatchType == c.WatchType &&
					a.TranslationTitle == c.TranslationTitle {
					if best == nil || c.Viewers > best.viewers {
						best = &candidate{combo: a, viewers: c.Viewers}
					}
					break
				}
			}
		}

		if best != nil {
			return resolved(best.combo, 3)
		}
	}

	// ── Tier 4: Pinned translations ───────────────────────────
	if len(pinned) > 0 {
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
			for _, a := range available {
				if a.Language == pinnedLang && a.WatchType == pinnedWatchType &&
					a.TranslationTitle == p.TranslationTitle {
					return resolved(a, 4)
				}
			}
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
