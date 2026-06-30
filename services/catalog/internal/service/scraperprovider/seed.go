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
		Name: "allanime", Status: domain.StatusDegraded,
		Reason: "Stream broken — AllAnime sources behind Cloudflare Turnstile clock (2026-06-22)",
		Description: "AllAnime discovery still works, but its primary sources decode to " +
			"/apivtwo/clock.json behind a Cloudflare managed/Turnstile challenge (api.allanime.day) " +
			"or a down bare host — unsolvable from our egress. Degraded: out of auto-failover, " +
			"manually selectable (hacker mode). Its ok.ru ('Ok') sources are served clock-free by " +
			"the 'okru' provider. Existing DBs flipped via AllAnimeDegrade.",
		// sub_delivery "unknown": claimed hard, but unverifiable by the 2026-06-29
		// subprobe (stream stage down — Cloudflare clock). Do not assert burned-in.
		SupportsSub: true, SupportsDub: true, SubDelivery: "unknown",
		QualityCeiling: "1080p", PreferenceWeight: 90,
	},
	{
		Name: "okru", Status: domain.StatusEnabled,
		Reason: "AllAnime 'Ok' sources via ok.ru CDN (clock-free)",
		Description: "Reuses AllAnime's GraphQL discovery (api.allanime.day) and resolves ONLY its " +
			"ok.ru ('Ok') sources via ok.ru data-options metadata → okcdn.ru HLS, bypassing the " +
			"Cloudflare-Turnstile-walled /apivtwo/clock endpoint that broke allanime. EN sub/dub, " +
			"hardsubbed (ok.ru has no soft-sub track).",
		// sub_delivery "unknown": unverifiable by the 2026-06-29 subprobe (stream
		// stage down). ok.ru has no soft-sub track so subs are *likely* burned-in,
		// but we never confirmed it — don't assert hard.
		SupportsSub: true, SupportsDub: true, SubDelivery: "unknown",
		QualityCeiling: "1080p", PreferenceWeight: 35,
	},
	{
		Name: "gogoanime", Status: domain.StatusEnabled,
		// gogoanime → megaplay player resolves its stream id + (rotating) CDN
		// host at RUNTIME in JS, and the player only serves with the embedding
		// wrapper Referer — a curl-class scraper can't follow it. Resolved via
		// the Camoufox stealth-scraper sidecar (engine=browser): real Firefox in
		// a virtual display + network interception of the .m3u8. Verified
		// end-to-end on the clean server IP 2026-06-20 (no proxy needed). Engine
		// + BaseURL are DB-driven; the SCRAPER_GOGOANIME_BASE_URL env is retired.
		Engine: "browser", BaseURL: "https://gogoanimes.fi",
		Reason: "Browser-scraped via Camoufox sidecar (megaplay JS-runtime id + rotating CDN)",
		Description: "anitaku.to migrated to anineko.to. Mirror gogoanimes.fi (classic gogo " +
			"HTML: anime_muti_link + /search.html), whose newplayer.php embed nests the " +
			"megaplay.buzz player. The stream id + CDN host (mewstream.buzz → cinewave2.site " +
			"→ …) are built at runtime by the player JS and the CDN is Referer-gated, so the " +
			"stealth-scraper drives a real browser and intercepts the .m3u8 (engine=browser).",
		// sub_delivery verified soft 2026-06-29 (subprobe): megaplay serves CLEAN
		// video + multi-language soft .vtt tracks (EN/RU/+7), NOT burned-in.
		SupportsSub: true, SupportsDub: true, SubDelivery: "soft",
		QualityCeiling: "1080p", PreferenceWeight: 85,
	},
	{
		Name: "miruro", Status: domain.StatusEnabled,
		Reason: "DUB-only — upstream stopped serving sub streams (2026-06-19)",
		Description: "Miruro's upstream no longer returns sub servers; only English dub " +
			"plays. SupportsSub=false so it is never offered/auto-selected for SUB " +
			"(original-Japanese-audio) playback. Existing DBs flipped via MiruroDubOnly.",
		SupportsSub: false, SupportsDub: true, SubDelivery: "hard",
		QualityCeiling: "1080p", PreferenceWeight: 70,
	},
	{
		Name: "animefever", Status: domain.StatusDegraded,
		Reason: "Ad-substituted HLS segments (AUTO-484)",
		Description: "animefever.cc → am.vidstream.vip (StreamX.Me/JW player) returns a valid " +
			"manifest, but its HLS segments 302-redirect to an ad CDN " +
			"(sf16-scmcdn-sg.ibytedtos.com / ad-site-i18n-sg) that 403s for us, so playback " +
			"fails. The exact trigger for the ad swap is not confirmed. " +
			"Degraded: kept manually selectable (hacker mode) but out of the auto-failover chain. " +
			"Existing DBs updated via AnimefeverDeclaim.",
		// sub_delivery "unknown": claimed hard, unverifiable by the 2026-06-29
		// subprobe (degraded — ad-substituted segments 403, playback fails).
		SupportsSub: true, SupportsDub: false, SubDelivery: "unknown",
		QualityCeiling: "1080p", PreferenceWeight: 60,
	},
	{
		Name: "nineanime", Status: domain.StatusEnabled,
		// 9anime.me.uk's whole site is DDoS-Guard/JS-gated (discovery times out
		// for a curl-class client) and its popular catalog migrated to the
		// megaplay.buzz JS player (runtime stream id + rotating Referer-gated CDN).
		// Resolved via the Camoufox stealth-scraper sidecar (engine=browser):
		// discovery GETs route through the warm browser session; megaplay players
		// are intercepted for the .m3u8. Verified live 2026-06-21.
		Engine: "browser", BaseURL: "https://9anime.me.uk",
		Reason: "Browser-scraped via Camoufox sidecar (DDoS-Guard site + megaplay JS player)",
		Description: "9anime.me.uk discovery (WP-REST search + series/episode pages) is " +
			"DDoS-Guard/JS-gated and its popular catalog uses the megaplay.buzz player whose " +
			"stream id + CDN are built at runtime in JS; the stealth-scraper drives a real " +
			"browser for both discovery and the .m3u8 interception (engine=browser).",
		SupportsSub: true, SupportsDub: false, SubDelivery: "hard",
		QualityCeiling: "720p", PreferenceWeight: 40,
	},
	{
		// REVIVED 2026-06-26 via Camoufox: animepahe.pw's Cloudflare managed
		// (interactive Turnstile) challenge IS solvable from this server's own
		// datacenter IP — the stealth-scraper warm /fetch session clicks the
		// Turnstile checkbox + polls for cf_clearance (~10s, no proxy), then the
		// JSON API (search/release) + /play HTML ride the in-page fetch. The
		// kwik.cx stream leg is plain-Go reachable (Go KwikExtractor). Seeded
		// DEGRADED (owner pref — manually selectable, out of the auto-failover
		// chain) pending live soak; promote to enabled later. Existing DBs are
		// carried by AnimepaheBrowserRevival.
		Name: "animepahe", Status: domain.StatusDegraded,
		Engine: "browser", BaseURL: "https://animepahe.pw",
		Reason: "Browser-scraped via Camoufox sidecar (animepahe.pw Cloudflare managed challenge solved)",
		Description: "animepahe.pw sits behind a Cloudflare managed (interactive Turnstile) challenge. " +
			"The Camoufox stealth-scraper warm /fetch session solves it (clicks the Turnstile checkbox + " +
			"waits for cf_clearance, ~10s on our own IP, no residential proxy); discovery (search/release " +
			"JSON + /play HTML) then rides the in-page fetch (engine=browser). The kwik.cx stream leg is " +
			"extracted in Go. Degraded: manually selectable (hacker mode), out of the auto-failover chain " +
			"pending live soak.",
		SupportsSub: true, SupportsDub: true, SubDelivery: "hard",
		QualityCeiling: "1080p", PreferenceWeight: 30,
	},
	{
		Name: "animekai", Status: domain.StatusDisabled,
		Reason: "Stub — ListServers unimplemented (SCRAPER-KAI-03)",
		Description: "animekai provider is a stub; ListServers returns ErrProviderDown. " +
			"Disabled until implemented so it never wastes a failover slot.",
		// sub_delivery "unknown": claimed hard, but animekai is a disabled stub
		// (ListServers unimplemented) — never probed, so don't assert burned-in.
		SupportsSub: true, SupportsDub: false, SubDelivery: "unknown",
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
	{
		Name: "ae", Status: domain.StatusEnabled,
		Reason: "First-party AnimeEnigma source (survivor)",
		Description: "Self-hosted HLS from the private raw-library MinIO bucket. The " +
			"long-term user-facing player; all other players are being retired (2026-06-17).",
		SupportsSub: true, SupportsRaw: true, SubDelivery: "soft",
		QualityCeiling: "1080p", PreferenceWeight: 100,
	},
	{
		Name: "kodik-iframe", Status: domain.StatusEnabled,
		Reason: "RU iframe embed — playback not probeable (no direct stream)",
		Description: "Kodik iframe embed. The player has no direct video control, so the " +
			"playback probe cannot validate it (it reads '— not probed').",
		SupportsDub: true, SubDelivery: "none", PreferenceWeight: 0,
	},
	{
		Name: "kodik-noads", Status: domain.StatusEnabled,
		Reason: "Ad-free scraped Kodik HLS (kodikextract)",
		Description: "Direct ad-free Kodik HLS resolved via kodikextract (solodcdn CDN). " +
			"A real stream, so it is playback-probed.",
		SupportsSub: true, SupportsDub: true, SubDelivery: "hard",
		QualityCeiling: "1080p", PreferenceWeight: 0,
	},
	{
		Name: "animelib", Status: domain.StatusDisabled,
		Reason: "RU direct-MP4 player retired (Plan B)",
		Description: "AniLib direct MP4. Player surface retired in favor of aePlayer; " +
			"content dropped (2026-06-18, Plan B).",
		SupportsDub: true, SubDelivery: "none",
		QualityCeiling: "1080p", PreferenceWeight: 0,
	},
	{
		Name: "hanime", Status: domain.StatusEnabled,
		Reason: "18+ source restored into aePlayer (2026-06-19)",
		Description: "Hanime HLS. Selectable 18+ source inside aePlayer (hentai titles); " +
			"catalog-operated parser via /hanime/* routes.",
		SubDelivery: "none", QualityCeiling: "1080p", PreferenceWeight: 0,
	},
	{
		// animejoy itself is NOT a row — it is the shared discovery/reference base
		// (title→news_id→playlist, cached once); the two real provider rows below
		// each resolve their own leg off that shared discovery (mirrors how 'okru'
		// reuses AllAnime's GraphQL discovery). Promoted out of soak 2026-06-30
		// (probe-verified playable end-to-end): Status=enabled with the default
		// policy=auto/health=up, so they surface as normal selectable sources for ALL
		// users (deriveProviderView keys hacker-only on Policy==manual; enabled needs
		// no explicit policy/health). RU-SUB only — animejoy serves original (JP)
		// audio + burned-in Russian subs in the Sibnet/AllVideo mirror MP4s, so
		// SubDelivery=hard, no dub, no raw. Group is intrinsic ("ru", via
		// intrinsicGroups) and scraper_operated is intentionally false (NOT in
		// scraperOperatedNames — these are catalog-operated RU rows; adding them to
		// the EN scraper-failover chain would crash-loop boot via the EN-only
		// candidateProviders invariant). A stream_providers row with no family simply
		// doesn't surface, so they only appear on titles AnimeJoy actually carries.
		Name: "animejoy-sibnet", Status: domain.StatusEnabled,
		SupportsSub: true, SupportsDub: false, SupportsRaw: false,
		SubDelivery: "hard", QualityCeiling: "1080p", PreferenceWeight: 25,
		Reason:      "AnimeJoy RU-sub via Sibnet",
		Description: "Sibnet (AnimeJoy, RU-sub)",
	},
	{
		Name: "animejoy-allvideo", Status: domain.StatusEnabled,
		SupportsSub: true, SupportsDub: false, SupportsRaw: false,
		SubDelivery: "hard", QualityCeiling: "1080p", PreferenceWeight: 20,
		Reason:      "AnimeJoy RU-sub via AllVideo",
		Description: "AllVideo (AnimeJoy, RU-sub)",
	},
}

// intrinsicGroups mirrors services/scraper/internal/config/providers.go's
// providerGroups: group is INTRINSIC to a provider (a hentai source is always
// "adult"), so the seed can never move 18anime into the EN failover chain.
// Absent entries default to "en".
var intrinsicGroups = map[string]string{
	"18anime":           "adult",
	"hanime":            "adult",
	"ae":                "firstparty",
	"kodik-iframe":      "ru",
	"kodik-noads":       "ru",
	"animelib":          "ru",
	"animejoy-sibnet":   "ru",
	"animejoy-allvideo": "ru",
}

func intrinsicGroup(name string) string {
	if g, ok := intrinsicGroups[name]; ok {
		return g
	}
	return "en"
}

// scraperOperatedNames is the intrinsic set of providers operated by the scraper
// microservice (EN failover chain + 18+ orchestrator). Like Group, it is
// intrinsic — derived from the name, never operator-editable.
var scraperOperatedNames = map[string]bool{
	"gogoanime": true, "animepahe": true, "allanime": true, "animefever": true,
	"miruro": true, "nineanime": true, "animekai": true, "18anime": true,
	"okru": true,
}

func isScraperOperated(name string) bool { return scraperOperatedNames[name] }

// scraperOperatedNameList returns the intrinsic scraper-operated names as a slice
// (for the backfill UPDATE ... WHERE name IN (...)).
func scraperOperatedNameList() []string {
	out := make([]string, 0, len(scraperOperatedNames))
	for n := range scraperOperatedNames {
		out = append(out, n)
	}
	return out
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
		// Group + scraper_operated are intrinsic — always derive from the name.
		row.Group = intrinsicGroup(p.Name)
		row.ScraperOperated = isScraperOperated(p.Name)
		if row.SubDelivery == "" {
			row.SubDelivery = "hard"
		}
		if err := db.Create(&row).Error; err != nil {
			return fmt.Errorf("create %q: %w", p.Name, err)
		}
	}
	return nil
}
