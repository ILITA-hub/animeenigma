// Package scraperprovider bootstraps the scraper_providers table from a
// Go-embedded default roster. Insert-if-absent only: a row that already exists is
// never overwritten, so operator edits in the DB survive re-seeding. The DB is
// the SINGLE source of truth — docker/scraper-providers.yaml was retired
// 2026-06-17 (AUTO-484); this seed only bootstraps a fresh database.
package scraperprovider

import (
	"fmt"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"gorm.io/gorm"
)

// defaultProviders is the bootstrap roster (formerly docker/scraper-providers.yaml).
// Group is intrinsic — derived from the name via intrinsicGroup, never trusted
// from this literal — so the 18+/EN separation can't be broken by a typo here.
// The reason/description columns record WHY each provider is in its state.
var defaultProviders = []domain.ScraperProvider{
	{
		Name: "allanime", Status: domain.StatusEnabled,
		SupportsSub: true, SupportsDub: true, SubDelivery: "hard",
		QualityCeiling: "1080p", PreferenceWeight: 90,
	},
	{
		Name: "gogoanime", Status: domain.StatusEnabled,
		Reason: "Revived via gogoanimes.fi mirror + megaplay",
		Description: "anitaku.to migrated to anineko.to (\"We Have Moved\"). Repointed " +
			"SCRAPER_GOGOANIME_BASE_URL to gogoanimes.fi (classic gogo HTML: " +
			"anime_muti_link + /search.html), whose newplayer.php embed nests the " +
			"megaplay.buzz player — now routed through the megaplay extractor " +
			"(gogoanime.me.uk added to its wrapper allowlist). Re-enabled 2026-06-05.",
		SupportsSub: true, SupportsDub: true, SubDelivery: "hard",
		QualityCeiling: "1080p", PreferenceWeight: 85,
	},
	{
		Name: "miruro", Status: domain.StatusEnabled,
		SupportsSub: true, SupportsDub: true, SubDelivery: "hard",
		QualityCeiling: "1080p", PreferenceWeight: 70,
	},
	{
		Name: "animefever", Status: domain.StatusDegraded,
		Reason: "Region-walled ad substitution (AUTO-484)",
		Description: "animefever.cc → am.vidstream.vip (StreamX.Me/JW player) returns a valid " +
			"manifest, but EVERY HLS segment 302-redirects to a TikTok/ByteDance ad CDN " +
			"(sf16-scmcdn-sg.ibytedtos.com / ad-site-i18n-sg) that 403s outside its target " +
			"region. Verified 2026-06-17 (AUTO-484): the ad swap is keyed on egress-IP class " +
			"and is identical on our DE datacenter IP, Cloudflare WARP, AND a residential RU IP " +
			"— so it is unwatchable for our users. uBlock/WARP/residential do not recover it. " +
			"Degraded: kept manually selectable (hacker mode) but out of the auto-failover chain.",
		SupportsSub: true, SupportsDub: false, SubDelivery: "hard",
		QualityCeiling: "1080p", PreferenceWeight: 60,
	},
	{
		Name: "nineanime", Status: domain.StatusEnabled,
		SupportsSub: true, SupportsDub: false, SubDelivery: "hard",
		QualityCeiling: "720p", PreferenceWeight: 40,
	},
	{
		Name: "animepahe", Status: domain.StatusDisabled,
		Reason: "Cloudflare challenge",
		Description: "animepahe.pw migrated DDoS-Guard -> Cloudflare managed challenge; the " +
			"stealth-Chromium sidecar can't solve it (0% solve rate). See ISS-023. " +
			"Disabled 2026-06-03.",
		SupportsSub: true, SupportsDub: true, SubDelivery: "hard",
		QualityCeiling: "1080p", PreferenceWeight: 30,
	},
	{
		Name: "animekai", Status: domain.StatusDisabled,
		Reason: "Stub — ListServers unimplemented (SCRAPER-KAI-03)",
		Description: "animekai provider is a stub; ListServers returns ErrProviderDown. " +
			"Disabled until implemented so it never wastes a failover slot.",
		SupportsSub: true, SupportsDub: false, SubDelivery: "hard",
		QualityCeiling: "1080p", PreferenceWeight: 0,
	},
	{
		Name: "18anime", Status: domain.StatusEnabled,
		Reason: "18+ provider (separate group)",
		Description: "18anime.me hentai source for the 18+ player. Runs in its own orchestrator " +
			"on /anime18/* — NEVER part of the EN (OurEnglish) failover chain.",
		SupportsSub: true, SupportsDub: false, SupportsRaw: true, SubDelivery: "hard",
		QualityCeiling: "1080p", PreferenceWeight: 0,
	},
}

// intrinsicGroups mirrors services/scraper/internal/config/providers.go's
// providerGroups: group is INTRINSIC to a provider (a hentai source is always
// "adult"), so the seed can never move 18anime into the EN failover chain.
// Absent entries default to "en".
var intrinsicGroups = map[string]string{
	"18anime": "adult",
}

func intrinsicGroup(name string) string {
	if g, ok := intrinsicGroups[name]; ok {
		return g
	}
	return "en"
}

// SeedDefaults inserts any default provider row not already present (insert-if-
// absent: existing rows / operator edits are never overwritten). Idempotent.
func SeedDefaults(db *gorm.DB) error {
	for _, p := range defaultProviders {
		if p.Name == "" {
			continue
		}
		var count int64
		if err := db.Model(&domain.ScraperProvider{}).Where("name = ?", p.Name).Count(&count).Error; err != nil {
			return fmt.Errorf("count %q: %w", p.Name, err)
		}
		if count > 0 {
			continue // insert-if-absent: never overwrite an existing row
		}
		row := p
		// Group is intrinsic — always derive from the name, never trust the literal.
		row.Group = intrinsicGroup(p.Name)
		if row.SubDelivery == "" {
			row.SubDelivery = "hard"
		}
		if err := db.Create(&row).Error; err != nil {
			return fmt.Errorf("create %q: %w", p.Name, err)
		}
	}
	return nil
}
