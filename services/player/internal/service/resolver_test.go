package service

import (
	"testing"

	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"github.com/stretchr/testify/assert"
)

// Helper to build a WatchCombo quickly
func combo(player, lang, wtype, tid, title string) domain.WatchCombo {
	return domain.WatchCombo{
		Player:           player,
		Language:         lang,
		WatchType:        wtype,
		TranslationID:    tid,
		TranslationTitle: title,
	}
}

// Common available combos used across tests (real translation names)
var (
	// RU dubs (kodik)
	kodikAniLibria  = combo("kodik", "ru", "dub", "610", "AniLibria")
	kodikAniDUB     = combo("kodik", "ru", "dub", "609", "AniDUB")
	kodikSHIZA      = combo("kodik", "ru", "dub", "616", "SHIZA")
	kodikJAM        = combo("kodik", "ru", "dub", "971", "JAM")
	// RU subs (kodik)
	kodikCrunchyroll = combo("kodik", "ru", "sub", "963", "Crunchyroll")
	// RU dubs (animelib)
	animelibAniLibria = combo("animelib", "ru", "dub", "610", "AniLibria")
	animelibAniDUB    = combo("animelib", "ru", "dub", "609", "AniDUB")
	animelibAniRise   = combo("animelib", "ru", "dub", "1", "AniRise")
	// EN dubs (hianime)
	hianimeHD1 = combo("hianime", "en", "dub", "hd-1", "HD-1")
	hianimeHD2 = combo("hianime", "en", "dub", "hd-2", "HD-2")
	// EN subs (hianime)
	hianimeSubDefault = combo("hianime", "en", "sub", "default", "Default")
	// EN dubs (consumet)
	consumetHD1 = combo("consumet", "en", "dub", "hd-1", "HD-1")
)

func TestResolve(t *testing.T) {
	tests := []struct {
		name      string
		userPref  *domain.UserAnimePreference
		globalFav *domain.ComboCount
		community []domain.CommunityCombo
		pinned    []domain.PinnedTranslation
		available []domain.WatchCombo
		wantNil   bool
		wantTier  string
		wantTierN int
		wantCombo domain.WatchCombo // expected WatchCombo within ResolvedCombo
	}{
		// ──────────────────────────────────────────────
		// Group 1: Tier 1 — per-anime preference
		// ──────────────────────────────────────────────
		{
			name: "T1: exact match player+translation_id",
			userPref: &domain.UserAnimePreference{
				Player: "kodik", Language: "ru", WatchType: "dub",
				TranslationID: "610", TranslationTitle: "AniLibria",
			},
			available: []domain.WatchCombo{kodikAniLibria, kodikAniDUB, hianimeHD1},
			wantTier:  "per_anime", wantTierN: 1,
			wantCombo: kodikAniLibria,
		},
		{
			name: "T1: title match cross-player (saved kodik, available animelib)",
			userPref: &domain.UserAnimePreference{
				Player: "kodik", Language: "ru", WatchType: "dub",
				TranslationID: "610", TranslationTitle: "AniLibria",
			},
			// kodik AniLibria not available, but animelib AniLibria is
			available: []domain.WatchCombo{animelibAniLibria, animelibAniDUB, hianimeHD1},
			wantTier:  "per_anime", wantTierN: 1,
			wantCombo: animelibAniLibria,
		},
		{
			name: "T1: combo gone → locks lang+type, falls to lower tier",
			userPref: &domain.UserAnimePreference{
				Player: "kodik", Language: "ru", WatchType: "dub",
				TranslationID: "610", TranslationTitle: "AniLibria",
			},
			// AniLibria not available anywhere, but lock ru+dub is set
			community: []domain.CommunityCombo{
				{Player: "kodik", Language: "ru", WatchType: "dub",
					TranslationID: "609", TranslationTitle: "AniDUB", Viewers: 50},
			},
			available: []domain.WatchCombo{kodikAniDUB, hianimeHD1},
			wantTier:  "community", wantTierN: 3,
			wantCombo: kodikAniDUB,
		},

		// ──────────────────────────────────────────────
		// Group 2: Tier 2 — user global favorite #1 only
		// ──────────────────────────────────────────────
		{
			name: "T2: global fav team found in available",
			globalFav: &domain.ComboCount{
				Player: "kodik", Language: "ru", WatchType: "dub",
				TranslationTitle: "AniLibria", Count: 15,
			},
			available: []domain.WatchCombo{kodikAniLibria, kodikAniDUB, hianimeHD1},
			wantTier:  "user_global", wantTierN: 2,
			wantCombo: kodikAniLibria,
		},
		{
			name: "T2: global fav team not available → skip to Tier 3",
			globalFav: &domain.ComboCount{
				Player: "kodik", Language: "ru", WatchType: "dub",
				TranslationTitle: "SHIZA", Count: 10,
			},
			// SHIZA not in available; community has AniDUB as ru+dub
			community: []domain.CommunityCombo{
				{Player: "kodik", Language: "ru", WatchType: "dub",
					TranslationID: "609", TranslationTitle: "AniDUB", Viewers: 30},
			},
			available: []domain.WatchCombo{kodikAniDUB, hianimeHD1},
			wantTier:  "community", wantTierN: 3,
			wantCombo: kodikAniDUB,
		},

		// ──────────────────────────────────────────────
		// Group 3: Tier 3 — community popularity
		// ──────────────────────────────────────────────
		{
			name: "T3: clear winner in community",
			community: []domain.CommunityCombo{
				{Player: "kodik", Language: "ru", WatchType: "dub",
					TranslationID: "610", TranslationTitle: "AniLibria", Viewers: 100},
				{Player: "kodik", Language: "ru", WatchType: "dub",
					TranslationID: "609", TranslationTitle: "AniDUB", Viewers: 40},
				{Player: "hianime", Language: "en", WatchType: "dub",
					TranslationID: "hd-1", TranslationTitle: "HD-1", Viewers: 80},
			},
			available: []domain.WatchCombo{kodikAniLibria, kodikAniDUB, hianimeHD1},
			wantTier:  "community", wantTierN: 3,
			wantCombo: kodikAniLibria,
		},
		{
			name: "T3: filtered by lock from Tier 1 pref",
			userPref: &domain.UserAnimePreference{
				Player: "hianime", Language: "en", WatchType: "dub",
				TranslationID: "hd-1", TranslationTitle: "HD-1",
			},
			// HD-1 not in available, but lock is en+dub
			community: []domain.CommunityCombo{
				{Player: "kodik", Language: "ru", WatchType: "dub",
					TranslationID: "610", TranslationTitle: "AniLibria", Viewers: 200},
				{Player: "hianime", Language: "en", WatchType: "dub",
					TranslationID: "hd-2", TranslationTitle: "HD-2", Viewers: 50},
			},
			available: []domain.WatchCombo{kodikAniLibria, hianimeHD2},
			wantTier:  "community", wantTierN: 3,
			wantCombo: hianimeHD2, // must respect en+dub lock, not pick ru AniLibria
		},
		{
			name: "T3: no community data → fall to Tier 4",
			globalFav: &domain.ComboCount{
				Player: "kodik", Language: "ru", WatchType: "dub",
				TranslationTitle: "SHIZA", Count: 5,
			},
			// SHIZA not available, no community data, pinned AniLibria matches lock
			pinned: []domain.PinnedTranslation{
				{AnimeID: "123", TranslationID: 610, TranslationTitle: "AniLibria", TranslationType: "voice"},
			},
			available: []domain.WatchCombo{kodikAniLibria, hianimeHD1},
			wantTier:  "pinned", wantTierN: 4,
			wantCombo: kodikAniLibria,
		},
		{
			name: "T3: new user no lock → most popular sets lock",
			community: []domain.CommunityCombo{
				{Player: "hianime", Language: "en", WatchType: "dub",
					TranslationID: "hd-1", TranslationTitle: "HD-1", Viewers: 90},
				{Player: "kodik", Language: "ru", WatchType: "dub",
					TranslationID: "610", TranslationTitle: "AniLibria", Viewers: 60},
			},
			available: []domain.WatchCombo{hianimeHD1, kodikAniLibria},
			wantTier:  "community", wantTierN: 3,
			wantCombo: hianimeHD1, // most popular is en+dub HD-1
		},

		// ──────────────────────────────────────────────
		// Group 4: Tier 4 — pinned translations
		// ──────────────────────────────────────────────
		{
			name: "T4: pinned matches lock (ru+dub)",
			globalFav: &domain.ComboCount{
				Player: "kodik", Language: "ru", WatchType: "dub",
				TranslationTitle: "SHIZA", Count: 8,
			},
			// SHIZA not available, no community
			pinned: []domain.PinnedTranslation{
				{AnimeID: "123", TranslationID: 610, TranslationTitle: "AniLibria", TranslationType: "voice"},
			},
			available: []domain.WatchCombo{kodikAniLibria, kodikCrunchyroll, hianimeHD1},
			wantTier:  "pinned", wantTierN: 4,
			wantCombo: kodikAniLibria,
		},
		{
			name: "T4: pinned wrong type (voice pinned, lock is sub) → Tier 5",
			globalFav: &domain.ComboCount{
				Player: "kodik", Language: "ru", WatchType: "sub",
				TranslationTitle: "SomeSubTeam", Count: 12,
			},
			// SomeSubTeam not available, no community
			// Pinned is voice=dub but lock is ru+sub
			pinned: []domain.PinnedTranslation{
				{AnimeID: "123", TranslationID: 610, TranslationTitle: "AniLibria", TranslationType: "voice"},
			},
			available: []domain.WatchCombo{kodikAniLibria, kodikCrunchyroll},
			wantTier:  "default", wantTierN: 5,
			wantCombo: kodikCrunchyroll, // default picks kodik sub
		},
		{
			name: "T4: no pinned → Tier 5",
			globalFav: &domain.ComboCount{
				Player: "kodik", Language: "ru", WatchType: "sub",
				TranslationTitle: "SomeTeam", Count: 3,
			},
			available: []domain.WatchCombo{kodikCrunchyroll, kodikAniLibria},
			wantTier:  "default", wantTierN: 5,
			wantCombo: kodikCrunchyroll,
		},

		// ──────────────────────────────────────────────
		// Group 5: Tier 5 — default kodik sub
		// ──────────────────────────────────────────────
		{
			name: "T5: kodik sub exists → return it",
			available: []domain.WatchCombo{kodikCrunchyroll, kodikAniLibria, hianimeHD1},
			wantTier:  "default", wantTierN: 5,
			wantCombo: kodikCrunchyroll,
		},
		{
			name: "T5: kodik sub not available → nil",
			available: []domain.WatchCombo{kodikAniLibria, hianimeHD1},
			wantNil:   true,
		},

		// ──────────────────────────────────────────────
		// Group 6: Boundary rules
		// ──────────────────────────────────────────────
		{
			name: "B: never cross language (locked ru, en option more popular)",
			userPref: &domain.UserAnimePreference{
				Player: "kodik", Language: "ru", WatchType: "dub",
				TranslationID: "610", TranslationTitle: "AniLibria",
			},
			// AniLibria gone; community has en HD-1 (200 viewers) and ru AniDUB (10)
			community: []domain.CommunityCombo{
				{Player: "hianime", Language: "en", WatchType: "dub",
					TranslationID: "hd-1", TranslationTitle: "HD-1", Viewers: 200},
				{Player: "kodik", Language: "ru", WatchType: "dub",
					TranslationID: "609", TranslationTitle: "AniDUB", Viewers: 10},
			},
			available: []domain.WatchCombo{kodikAniDUB, hianimeHD1},
			wantTier:  "community", wantTierN: 3,
			wantCombo: kodikAniDUB, // must pick ru AniDUB despite lower popularity
		},
		{
			name: "B: never cross type (locked dub, sub option available)",
			userPref: &domain.UserAnimePreference{
				Player: "kodik", Language: "ru", WatchType: "dub",
				TranslationID: "610", TranslationTitle: "AniLibria",
			},
			// AniLibria gone; community has ru sub (high) and ru dub (low)
			community: []domain.CommunityCombo{
				{Player: "kodik", Language: "ru", WatchType: "sub",
					TranslationID: "963", TranslationTitle: "Crunchyroll", Viewers: 150},
				{Player: "kodik", Language: "ru", WatchType: "dub",
					TranslationID: "971", TranslationTitle: "JAM", Viewers: 5},
			},
			available: []domain.WatchCombo{kodikCrunchyroll, kodikJAM},
			wantTier:  "community", wantTierN: 3,
			wantCombo: kodikJAM, // must pick dub despite lower popularity
		},
		{
			name: "B: lock carries through tiers (Tier 2 lock → Tier 3 → Tier 4 → Tier 5)",
			globalFav: &domain.ComboCount{
				Player: "hianime", Language: "en", WatchType: "dub",
				TranslationTitle: "HD-1", Count: 20,
			},
			// HD-1 not available; community has ru options only; pinned is ru voice
			community: []domain.CommunityCombo{
				{Player: "kodik", Language: "ru", WatchType: "dub",
					TranslationID: "610", TranslationTitle: "AniLibria", Viewers: 100},
			},
			pinned: []domain.PinnedTranslation{
				{AnimeID: "123", TranslationID: 610, TranslationTitle: "AniLibria", TranslationType: "voice"},
			},
			// Only ru options available — en lock means nothing matches
			available: []domain.WatchCombo{kodikAniLibria, kodikCrunchyroll},
			wantNil:   true, // locked en+dub, no en options, kodik sub default is ru → nil
		},

		// ──────────────────────────────────────────────
		// Group 7: Input validation / edge cases
		// ──────────────────────────────────────────────
		{
			name:      "V: empty available → nil",
			available: []domain.WatchCombo{},
			wantNil:   true,
		},
		{
			name: "V: no data at all → kodik sub default",
			available: []domain.WatchCombo{
				kodikCrunchyroll, kodikAniLibria, hianimeHD1,
			},
			// No userPref, no globalFav, no community, no pinned
			wantTier:  "default", wantTierN: 5,
			wantCombo: kodikCrunchyroll,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Resolve(
				tt.userPref,
				tt.globalFav,
				tt.community,
				tt.pinned,
				tt.available,
			)

			if tt.wantNil {
				assert.Nil(t, got, "expected nil result")
				return
			}

			if !assert.NotNil(t, got, "expected non-nil result") {
				return
			}

			assert.Equal(t, tt.wantTier, got.Tier, "tier name")
			assert.Equal(t, tt.wantTierN, got.TierNumber, "tier number")
			assert.Equal(t, tt.wantCombo.Player, got.Player, "player")
			assert.Equal(t, tt.wantCombo.Language, got.Language, "language")
			assert.Equal(t, tt.wantCombo.WatchType, got.WatchType, "watch_type")
			assert.Equal(t, tt.wantCombo.TranslationID, got.TranslationID, "translation_id")
			assert.Equal(t, tt.wantCombo.TranslationTitle, got.TranslationTitle, "translation_title")
		})
	}
}
