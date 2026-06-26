package scraperprovider

import (
	"fmt"
	"time"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"gorm.io/gorm"
)

// RenameScraperProvidersTable renames the legacy scraper_providers table to
// stream_providers exactly once. Guarded: only renames when the old table exists
// and the new one does not, so it is a no-op on fresh DBs and on every boot after
// the rename. Must run BEFORE AutoMigrate(&domain.ScraperProvider{}) so the new
// scraper_operated column is added to the renamed (data-carrying) table rather
// than to a fresh empty stream_providers. Works on SQLite (tests) + Postgres.
func RenameScraperProvidersTable(db *gorm.DB) error {
	m := db.Migrator()
	if m.HasTable("scraper_providers") && !m.HasTable("stream_providers") {
		if err := db.Exec("ALTER TABLE scraper_providers RENAME TO stream_providers").Error; err != nil {
			return fmt.Errorf("rename scraper_providers -> stream_providers: %w", err)
		}
	}
	return nil
}

// migrationGuard is a tiny key→applied ledger that records one-time data
// migrations so they don't re-run on every boot. Kept in its OWN table (not a
// sentinel row inside stream_providers) so it can never leak into a roster read /
// served provider list / Prometheus series. Mirrors the schema-state guard
// philosophy of RenameScraperProvidersTable, but for a status-flip that has no
// schema signal of its own.
type migrationGuard struct {
	Key       string    `gorm:"primaryKey;size:64"`
	AppliedAt time.Time `gorm:"autoCreateTime"`
}

func (migrationGuard) TableName() string { return "catalog_migration_guards" }

// retireHanimeAnimelibGuardKey marks RetireHanimeAnimelib as applied.
const retireHanimeAnimelibGuardKey = "retire_hanime_animelib"

// RetireHanimeAnimelib disables the hanime + animelib roster rows exactly once
// (Plan B: those player surfaces are retired and their content dropped). RUN-ONCE
// guarded via the catalog_migration_guards ledger, so on every subsequent boot it
// is a no-op and an operator who later re-enables either row in the DB is NOT
// clobbered. All other rows (ae, kodik, raw, 18anime, the EN scraper chain) are
// untouched. Idempotent; safe to call every boot.
func RetireHanimeAnimelib(db *gorm.DB) error {
	if err := db.AutoMigrate(&migrationGuard{}); err != nil {
		return fmt.Errorf("migrate catalog_migration_guards: %w", err)
	}
	var guards int64
	if err := db.Model(&migrationGuard{}).
		Where("key = ?", retireHanimeAnimelibGuardKey).Count(&guards).Error; err != nil {
		return fmt.Errorf("check retire-hanime-animelib guard: %w", err)
	}
	if guards > 0 {
		return nil // already applied — never clobber later operator re-enables
	}

	if err := db.Model(&domain.ScraperProvider{}).
		Where("name IN ?", []string{"hanime", "animelib"}).
		Update("status", domain.StatusDisabled).Error; err != nil {
		return fmt.Errorf("retire hanime+animelib (status=disabled): %w", err)
	}

	if err := db.Create(&migrationGuard{Key: retireHanimeAnimelibGuardKey}).Error; err != nil {
		return fmt.Errorf("write retire-hanime-animelib guard: %w", err)
	}
	return nil
}

// reEnableHanimeGuardKey marks ReEnableHanime as applied.
const reEnableHanimeGuardKey = "reenable_hanime"

// ReEnableHanime re-enables the hanime roster row exactly once. Forward-only
// counterpart to RetireHanimeAnimelib: hanime was retired in Plan B (2026-06-18)
// but restored as an in-aePlayer 18+ source (2026-06-19). RUN-ONCE guarded via
// the catalog_migration_guards ledger, so on every subsequent boot it is a no-op
// and an operator who later re-disables hanime is NOT clobbered. Must run AFTER
// SeedDefaults + RetireHanimeAnimelib so it wins the final status on fresh DBs
// (seed=enabled -> retire disables -> this re-enables). animelib is intentionally
// left disabled. Idempotent; safe to call every boot.
func ReEnableHanime(db *gorm.DB) error {
	if err := db.AutoMigrate(&migrationGuard{}); err != nil {
		return fmt.Errorf("migrate catalog_migration_guards: %w", err)
	}
	var guards int64
	if err := db.Model(&migrationGuard{}).
		Where("key = ?", reEnableHanimeGuardKey).Count(&guards).Error; err != nil {
		return fmt.Errorf("check reenable-hanime guard: %w", err)
	}
	if guards > 0 {
		return nil // already applied — never clobber a later operator re-disable
	}

	result := db.Model(&domain.ScraperProvider{}).
		Where("name = ?", "hanime").
		Update("status", domain.StatusEnabled)
	if result.Error != nil {
		return fmt.Errorf("re-enable hanime (status=enabled): %w", result.Error)
	}
	if result.RowsAffected == 0 {
		// No hanime row to flip (seed did not run / row hard-deleted). Do NOT
		// write the guard, so a later boot (after the row exists) retries.
		return fmt.Errorf("re-enable hanime: no row found for name=hanime")
	}

	if err := db.Create(&migrationGuard{Key: reEnableHanimeGuardKey}).Error; err != nil {
		return fmt.Errorf("write reenable-hanime guard: %w", err)
	}
	return nil
}

// BackfillScraperOperated sets the intrinsic scraper_operated flag on every row.
// Idempotent and safe to run every boot: like Group, the flag is intrinsic (NOT
// operator-editable), so re-deriving it from the canonical name set is always
// correct. Bounded row count (~13).
func BackfillScraperOperated(db *gorm.DB) error {
	names := scraperOperatedNameList()
	if err := db.Model(&domain.ScraperProvider{}).
		Where("name IN ?", names).Update("scraper_operated", true).Error; err != nil {
		return fmt.Errorf("backfill scraper_operated=true: %w", err)
	}
	if err := db.Model(&domain.ScraperProvider{}).
		Where("name NOT IN ?", names).Update("scraper_operated", false).Error; err != nil {
		return fmt.Errorf("backfill scraper_operated=false: %w", err)
	}
	return nil
}

// splitKodikGuardKey marks SplitKodik as applied.
const splitKodikGuardKey = "split_kodik_2026_06_21"

// SplitKodik renames the legacy single "kodik" roster row to "kodik-iframe" (the
// un-probeable embed). The new "kodik-noads" row (the scraped ad-free HLS) is
// inserted by SeedDefaults, so this migration is rename-only.
//
// MUST run BEFORE SeedDefaults: the seed is insert-if-absent, so if it ran first
// it would insert a fresh "kodik-iframe" while the old "kodik" row still existed,
// and this rename would then collide on the name primary key. Running the rename
// first means the seed sees kodik-iframe already present (skip) and only inserts
// kodik-noads.
//
// RUN-ONCE guarded via the catalog_migration_guards ledger. On a fresh DB there
// is no "kodik" row (the seed uses kodik-iframe directly), so the rename affects
// 0 rows — that is the correct terminal state, so the guard is still written.
// Idempotent; safe to call every boot. The functional data-source key "kodik"
// used elsewhere (capability families, parsers, FE, watch-together) is a separate
// identifier and is intentionally untouched.
func SplitKodik(db *gorm.DB) error {
	if err := db.AutoMigrate(&migrationGuard{}); err != nil {
		return fmt.Errorf("migrate catalog_migration_guards: %w", err)
	}
	var guards int64
	if err := db.Model(&migrationGuard{}).
		Where("key = ?", splitKodikGuardKey).Count(&guards).Error; err != nil {
		return fmt.Errorf("check split-kodik guard: %w", err)
	}
	if guards > 0 {
		return nil // already applied
	}

	if err := db.Model(&domain.ScraperProvider{}).
		Where("name = ?", "kodik").
		Update("name", "kodik-iframe").Error; err != nil {
		return fmt.Errorf("split kodik (rename kodik -> kodik-iframe): %w", err)
	}

	if err := db.Create(&migrationGuard{Key: splitKodikGuardKey}).Error; err != nil {
		return fmt.Errorf("write split-kodik guard: %w", err)
	}
	return nil
}

// miruroDubOnlyGuardKey marks MiruroDubOnly as applied.
const miruroDubOnlyGuardKey = "miruro_dub_only"

// animefeverDeclaimGuardKey marks AnimefeverDeclaim as applied.
const animefeverDeclaimGuardKey = "animefever_declaim"

// nineanimeBrowserGuardKey marks NineanimeBrowser as applied.
const nineanimeBrowserGuardKey = "nineanime_browser"

// allanimeDegradeGuardKey marks AllAnimeDegrade as applied.
const allanimeDegradeGuardKey = "allanime_degrade"

// animepaheSidecarRetiredGuardKey marks AnimepaheSidecarRetired as applied.
const animepaheSidecarRetiredGuardKey = "animepahe_sidecar_retired"

// animepaheBrowserRevivalGuardKey marks AnimepaheBrowserRevival as applied.
const animepaheBrowserRevivalGuardKey = "animepahe_browser_revival"

// MiruroDubOnly flips the miruro roster row to supports_sub=false exactly once.
// Miruro's upstream stopped serving sub streams (only English dub plays), so it
// must not advertise/auto-select for SUB (original-Japanese-audio) playback. The
// seed is insert-if-absent and so never updates the existing prod row; this
// RUN-ONCE guarded migration carries the flip to live DBs. Guarded via the
// catalog_migration_guards ledger so it is a no-op on every later boot and an
// operator who re-enables sub in the DB is NOT clobbered. Idempotent; safe to
// call every boot. supports_dub is left as-is (true).
func MiruroDubOnly(db *gorm.DB) error {
	if err := db.AutoMigrate(&migrationGuard{}); err != nil {
		return fmt.Errorf("migrate catalog_migration_guards: %w", err)
	}
	var guards int64
	if err := db.Model(&migrationGuard{}).
		Where("key = ?", miruroDubOnlyGuardKey).Count(&guards).Error; err != nil {
		return fmt.Errorf("check miruro-dub-only guard: %w", err)
	}
	if guards > 0 {
		return nil // already applied — never clobber a later operator re-enable
	}

	result := db.Model(&domain.ScraperProvider{}).
		Where("name = ?", "miruro").
		Update("supports_sub", false)
	if result.Error != nil {
		return fmt.Errorf("miruro dub-only (supports_sub=false): %w", result.Error)
	}
	if result.RowsAffected == 0 {
		// No miruro row to flip (seed did not run / row hard-deleted). Do NOT
		// write the guard, so a later boot (after the row exists) retries.
		return fmt.Errorf("miruro dub-only: no row found for name=miruro")
	}

	if err := db.Create(&migrationGuard{Key: miruroDubOnlyGuardKey}).Error; err != nil {
		return fmt.Errorf("write miruro-dub-only guard: %w", err)
	}
	return nil
}

// AnimefeverDeclaim removes the unverified "Region-walled" / egress-IP-class claims
// from the animefever provider description (AUTO-484 follow-up). The seed is
// insert-if-absent and so never updates an existing prod row; this RUN-ONCE guarded
// migration carries the corrected reason/description to live DBs. Guarded via the
// catalog_migration_guards ledger so it is a no-op on every later boot. Idempotent;
// safe to call every boot.
func AnimefeverDeclaim(db *gorm.DB) error {
	if err := db.AutoMigrate(&migrationGuard{}); err != nil {
		return fmt.Errorf("migrate catalog_migration_guards: %w", err)
	}
	var guards int64
	if err := db.Model(&migrationGuard{}).
		Where("key = ?", animefeverDeclaimGuardKey).Count(&guards).Error; err != nil {
		return fmt.Errorf("check animefever-declaim guard: %w", err)
	}
	if guards > 0 {
		return nil // already applied — never clobber a later operator edit
	}

	result := db.Model(&domain.ScraperProvider{}).
		Where("name = ?", "animefever").
		Updates(map[string]interface{}{
			"reason":      "Ad-substituted HLS segments (AUTO-484)",
			"description": "animefever.cc → am.vidstream.vip (StreamX.Me/JW player) returns a valid manifest, but its HLS segments 302-redirect to an ad CDN (sf16-scmcdn-sg.ibytedtos.com / ad-site-i18n-sg) that 403s for us, so playback fails. The exact trigger for the ad swap is not confirmed. Degraded: kept manually selectable (hacker mode) but out of the auto-failover chain. Existing DBs updated via AnimefeverDeclaim.",
		})
	if result.Error != nil {
		return fmt.Errorf("animefever declaim: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		// No animefever row to update (seed did not run / row hard-deleted). Do NOT
		// write the guard, so a later boot (after the row exists) retries.
		return fmt.Errorf("animefever declaim: no row found for name=animefever")
	}

	if err := db.Create(&migrationGuard{Key: animefeverDeclaimGuardKey}).Error; err != nil {
		return fmt.Errorf("write animefever-declaim guard: %w", err)
	}
	return nil
}

// NineanimeBrowser flips nineanime onto the Camoufox stealth-scraper sidecar
// (engine=browser) with its DB-driven base URL. 9anime.me.uk's whole site is
// DDoS-Guard/JS-gated (discovery times out for a curl-class client) and its
// megaplay player resolves the stream id + rotating CDN at runtime in JS, so a
// real browser is required. The seed is insert-if-absent and never updates the
// existing prod row; this RUN-ONCE guarded migration carries the flip to live
// DBs. Guarded via catalog_migration_guards so it's a no-op on every later boot
// and an operator who reverts engine in the DB is NOT clobbered. Idempotent.
func NineanimeBrowser(db *gorm.DB) error {
	if err := db.AutoMigrate(&migrationGuard{}); err != nil {
		return fmt.Errorf("migrate catalog_migration_guards: %w", err)
	}
	var guards int64
	if err := db.Model(&migrationGuard{}).
		Where("key = ?", nineanimeBrowserGuardKey).Count(&guards).Error; err != nil {
		return fmt.Errorf("check nineanime-browser guard: %w", err)
	}
	if guards > 0 {
		return nil // already applied — never clobber a later operator revert
	}
	result := db.Model(&domain.ScraperProvider{}).
		Where("name = ?", "nineanime").
		Updates(map[string]interface{}{
			"engine":   "browser",
			"base_url": "https://9anime.me.uk",
		})
	if result.Error != nil {
		return fmt.Errorf("nineanime browser flip: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		// No nineanime row to flip (seed did not run / row hard-deleted). Do NOT
		// write the guard, so a later boot (after the row exists) retries.
		return fmt.Errorf("nineanime browser flip: no row found for name=nineanime")
	}
	if err := db.Create(&migrationGuard{Key: nineanimeBrowserGuardKey}).Error; err != nil {
		return fmt.Errorf("write nineanime-browser guard: %w", err)
	}
	return nil
}

// AnimepaheBrowserRevival REVIVES animepahe (2026-06-26): flips it onto the
// Camoufox stealth-scraper sidecar (engine=browser, base_url=https://animepahe.pw)
// and promotes it from disabled → degraded. animepahe.pw's Cloudflare managed
// (interactive Turnstile) challenge IS solvable from this server's own datacenter
// IP — the warm /fetch session clicks the Turnstile checkbox and waits for
// cf_clearance (~10s, no residential proxy) — so the earlier "0% solve" verdict
// (AnimepaheSidecarRetired) is superseded. Seeded DEGRADED (owner pref: manually
// selectable, out of the auto-failover chain) pending live soak. The seed is
// insert-if-absent and never updates the existing prod row; this RUN-ONCE guarded
// migration carries the flip to live DBs. Guarded via catalog_migration_guards so
// it's a no-op on every later boot and an operator who reverts engine/status in
// the DB is NOT clobbered. Idempotent; safe to call every boot.
func AnimepaheBrowserRevival(db *gorm.DB) error {
	if err := db.AutoMigrate(&migrationGuard{}); err != nil {
		return fmt.Errorf("migrate catalog_migration_guards: %w", err)
	}
	var guards int64
	if err := db.Model(&migrationGuard{}).
		Where("key = ?", animepaheBrowserRevivalGuardKey).Count(&guards).Error; err != nil {
		return fmt.Errorf("check animepahe-browser-revival guard: %w", err)
	}
	if guards > 0 {
		return nil // already applied — never clobber a later operator change
	}
	// The scraper roster derives the wire status via WireStatus(policy, health),
	// NOT the stored `status` column — so the AUTHORITATIVE levers are policy +
	// health. On the live DB animepahe is policy=disabled (set when it was
	// disabled), so flipping `status` alone would leave WireStatus()=disabled.
	// Set policy=manual + health=down (→ WireStatus=degraded), mirroring
	// BackfillPolicyHealth's degraded mapping so a fresh DB (seed=degraded →
	// backfill manual/down) and a live DB converge to the same (manual, down).
	// `status` is updated too for column consistency (cosmetic; derived on wire).
	now := time.Now().UTC()
	result := db.Model(&domain.ScraperProvider{}).
		Where("name = ?", "animepahe").
		Updates(map[string]interface{}{
			"engine":       "browser",
			"base_url":     "https://animepahe.pw",
			"status":       domain.StatusDegraded,
			"policy":       string(domain.PolicyManual),
			"health":       string(domain.HealthDown),
			"policy_since": now,
			"health_since": now,
			"reason":       "Browser-scraped via Camoufox sidecar (animepahe.pw Cloudflare managed challenge solved)",
			"description":  "animepahe.pw sits behind a Cloudflare managed (interactive Turnstile) challenge. The Camoufox stealth-scraper warm /fetch session solves it (clicks the Turnstile checkbox + waits for cf_clearance, ~10s on our own IP, no residential proxy); discovery (search/release JSON + /play HTML) then rides the in-page fetch (engine=browser). The kwik.cx stream leg is extracted in Go. Degraded: manually selectable (hacker mode), out of the auto-failover chain pending live soak.",
		})
	if result.Error != nil {
		return fmt.Errorf("animepahe browser revival: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		// No animepahe row to flip (seed did not run / row hard-deleted). Do NOT
		// write the guard, so a later boot (after the row exists) retries.
		return fmt.Errorf("animepahe browser revival: no row found for name=animepahe")
	}
	if err := db.Create(&migrationGuard{Key: animepaheBrowserRevivalGuardKey}).Error; err != nil {
		return fmt.Errorf("write animepahe-browser-revival guard: %w", err)
	}
	return nil
}

// backfillPolicyHealthGuardKey marks BackfillPolicyHealth as applied.
const backfillPolicyHealthGuardKey = "backfill_policy_health_v1"

// BackfillPolicyHealth maps the legacy status tri-state onto the new
// (policy, health) dimensions exactly once. Guarded so it never clobbers
// later machine/operator writes on reboot.
//
//   - enabled  → policy=auto,     health=up
//   - degraded → policy=manual,   health=down
//   - disabled → policy=disabled, health=down
//
// Idempotent; safe to call every boot.
func BackfillPolicyHealth(db *gorm.DB) error {
	if err := db.AutoMigrate(&migrationGuard{}); err != nil {
		return fmt.Errorf("migrate catalog_migration_guards: %w", err)
	}
	var guards int64
	if err := db.Model(&migrationGuard{}).
		Where("key = ?", backfillPolicyHealthGuardKey).Count(&guards).Error; err != nil {
		return fmt.Errorf("check backfill-policy-health guard: %w", err)
	}
	if guards > 0 {
		return nil // already applied — never clobber later machine/operator writes
	}

	now := time.Now().UTC()
	if err := db.Model(&domain.ScraperProvider{}).
		Where("status = ?", domain.StatusEnabled).
		Updates(map[string]any{
			"policy":       string(domain.PolicyAuto),
			"health":       string(domain.HealthUp),
			"health_since": now,
			"policy_since": now,
		}).Error; err != nil {
		return fmt.Errorf("backfill enabled → auto/up: %w", err)
	}
	if err := db.Model(&domain.ScraperProvider{}).
		Where("status = ?", domain.StatusDegraded).
		Updates(map[string]any{
			"policy":       string(domain.PolicyManual),
			"health":       string(domain.HealthDown),
			"health_since": now,
			"policy_since": now,
		}).Error; err != nil {
		return fmt.Errorf("backfill degraded → manual/down: %w", err)
	}
	if err := db.Model(&domain.ScraperProvider{}).
		Where("status = ?", domain.StatusDisabled).
		Updates(map[string]any{
			"policy":       string(domain.PolicyDisabled),
			"health":       string(domain.HealthDown),
			"health_since": now,
			"policy_since": now,
		}).Error; err != nil {
		return fmt.Errorf("backfill disabled → disabled/down: %w", err)
	}

	if err := db.Create(&migrationGuard{Key: backfillPolicyHealthGuardKey}).Error; err != nil {
		return fmt.Errorf("write backfill-policy-health guard: %w", err)
	}
	return nil
}

// AllAnimeDegrade flips allanime to status=degraded exactly once. AllAnime's
// stream leg is dead (its sources decode to /apivtwo/clock.json behind a
// Cloudflare Turnstile, unsolvable from our egress); the ok.ru sources are
// served clock-free by the new 'okru' provider. The seed is insert-if-absent
// and never updates an existing prod row, so this RUN-ONCE guarded migration
// carries the flip to live DBs. Guarded via catalog_migration_guards so it is
// a no-op on later boots and never clobbers an operator re-enable. Idempotent.
func AllAnimeDegrade(db *gorm.DB) error {
	if err := db.AutoMigrate(&migrationGuard{}); err != nil {
		return fmt.Errorf("migrate catalog_migration_guards: %w", err)
	}
	var guards int64
	if err := db.Model(&migrationGuard{}).
		Where("key = ?", allanimeDegradeGuardKey).Count(&guards).Error; err != nil {
		return fmt.Errorf("check allanime-degrade guard: %w", err)
	}
	if guards > 0 {
		return nil // already applied — never clobber a later operator re-enable
	}
	result := db.Model(&domain.ScraperProvider{}).
		Where("name = ?", "allanime").
		Updates(map[string]interface{}{
			"status":      domain.StatusDegraded,
			"reason":      "Stream broken — AllAnime sources behind Cloudflare Turnstile clock (2026-06-22)",
			"description": "AllAnime discovery still works, but its primary sources decode to /apivtwo/clock.json behind a Cloudflare managed/Turnstile challenge (api.allanime.day) or a down bare host — unsolvable from our egress. Degraded: out of auto-failover, manually selectable (hacker mode). Its ok.ru ('Ok') sources are served clock-free by the 'okru' provider. Existing DBs flipped via AllAnimeDegrade.",
		})
	if result.Error != nil {
		return fmt.Errorf("allanime degrade: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("allanime degrade: no row found for name=allanime")
	}
	if err := db.Create(&migrationGuard{Key: allanimeDegradeGuardKey}).Error; err != nil {
		return fmt.Errorf("write allanime-degrade guard: %w", err)
	}
	return nil
}

// AnimepaheSidecarRetired records, exactly once, that the dedicated
// animepahe-resolver stealth-Chromium sidecar was retired (2026-06-24) and that
// animepahe is OFF but intentionally KEPT in the roster for possible later revival.
// animepahe was already disabled (Cloudflare challenge, 0% solve rate, 2026-06-03);
// this carries a refreshed reason/description to live DBs so the roster row tells the
// true current story (sidecar gone, revivable) instead of citing a sidecar that no
// longer exists. status is re-asserted as disabled (a no-op on the live row, which is
// already disabled) so the intended terminal state is explicit. The seed is
// insert-if-absent and never updates an existing prod row, so this RUN-ONCE guarded
// migration is the only thing that flips live DBs. Guarded via the
// catalog_migration_guards ledger so it is a no-op on every later boot and never
// clobbers a later operator re-enable. Idempotent; safe to call every boot.
func AnimepaheSidecarRetired(db *gorm.DB) error {
	if err := db.AutoMigrate(&migrationGuard{}); err != nil {
		return fmt.Errorf("migrate catalog_migration_guards: %w", err)
	}
	var guards int64
	if err := db.Model(&migrationGuard{}).
		Where("key = ?", animepaheSidecarRetiredGuardKey).Count(&guards).Error; err != nil {
		return fmt.Errorf("check animepahe-sidecar-retired guard: %w", err)
	}
	if guards > 0 {
		return nil // already applied — never clobber a later operator re-enable
	}
	result := db.Model(&domain.ScraperProvider{}).
		Where("name = ?", "animepahe").
		Updates(map[string]interface{}{
			"status":      domain.StatusDisabled,
			"reason":      "Off — animepahe-resolver sidecar retired (2026-06-24)",
			"description": "animepahe.pw migrated DDoS-Guard -> Cloudflare managed challenge that the stealth-Chromium sidecar couldn't solve (0% solve rate, ISS-023); disabled 2026-06-03. The dedicated animepahe-resolver sidecar was retired 2026-06-24 (no separate anti-DDoS-Guard browser is run anymore — Camoufox covers the live providers). The Go provider is KEPT in the failover roster so animepahe can be revived: flip this row to enabled and restore a transport (the sidecar from git history, or point SCRAPER_ANIMEPAHE_RESOLVER_URL at a new resolver).",
		})
	if result.Error != nil {
		return fmt.Errorf("animepahe sidecar-retired: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		// No animepahe row to update (seed did not run / row hard-deleted). Do NOT
		// write the guard, so a later boot (after the row exists) retries.
		return fmt.Errorf("animepahe sidecar-retired: no row found for name=animepahe")
	}
	if err := db.Create(&migrationGuard{Key: animepaheSidecarRetiredGuardKey}).Error; err != nil {
		return fmt.Errorf("write animepahe-sidecar-retired guard: %w", err)
	}
	return nil
}
