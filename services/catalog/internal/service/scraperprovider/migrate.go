package scraperprovider

import (
	"fmt"
	"time"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service/providerpolicy"
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

// animefeverDisableGuardKey marks AnimefeverDisable as applied.
const animefeverDisableGuardKey = "animefever_disable"

// nineanimeBrowserGuardKey marks NineanimeBrowser as applied.
const nineanimeBrowserGuardKey = "nineanime_browser"

// animepaheSidecarRetiredGuardKey marks AnimepaheSidecarRetired as applied.
const animepaheSidecarRetiredGuardKey = "animepahe_sidecar_retired"

// animepaheBrowserRevivalGuardKey marks AnimepaheBrowserRevival as applied.
const animepaheBrowserRevivalGuardKey = "animepahe_browser_revival"

// addAnimejoyProvidersGuardKey marks AddAnimejoyProviders as applied.
const addAnimejoyProvidersGuardKey = "add_animejoy_providers"

// removeRawProviderGuardKey marks RemoveRawProvider as applied.
const removeRawProviderGuardKey = "remove_raw_provider"

// miruroCloudflareBlockGuardKey marks MiruroCloudflareBlock as applied.
const miruroCloudflareBlockGuardKey = "miruro_cloudflare_block_2026_07_02"

// miruroBrowserRevivalGuardKey marks MiruroBrowserRevival as applied.
const miruroBrowserRevivalGuardKey = "miruro_browser_revival"

// allanimeOkruCryptoBlockGuardKey marks AllanimeOkruCryptoBlock as applied.
const allanimeOkruCryptoBlockGuardKey = "allanime_okru_crypto_block_2026_07_07"

// bumpKodikNoadsPriorityGuardKey marks BumpKodikNoadsPriority as applied.
const bumpKodikNoadsPriorityGuardKey = "bump_kodik_noads_priority_90_2026_07_07"

// reconcilePolicyFromHealthGuardKey marks ReconcilePolicyFromHealthV1 as applied.
const reconcilePolicyFromHealthGuardKey = "reconcile_policy_from_health_v1_2026_07_13"

// allanimeOkruCryptoLiftedGuardKey marks AllanimeOkruCryptoGateLifted as applied.
const allanimeOkruCryptoLiftedGuardKey = "allanime_okru_crypto_lifted_2026_07_13"

// backfillProviderIdentityGuardKey marks BackfillProviderIdentityV1 as applied.
const backfillProviderIdentityGuardKey = "backfill_provider_identity_v1_2026_07_14"

// RemoveRawProvider hard-deletes the legacy standalone "raw" JP provider row
// (removed 2026-06-30 — AllAnime + ok.ru cover JP-original audio). The seed no
// longer creates it, but insert-if-absent seeding never deletes an existing
// prod row, and the Grafana provider roster reads stream_providers DIRECTLY, so
// the stale row would otherwise keep showing in the roster. Run-once via the
// catalog_migration_guards ledger; a no-op once applied (and a clean no-op on
// fresh DBs that never had the row — delete is idempotent). ScraperProvider has
// no soft-delete column, so Delete is a hard delete.
func RemoveRawProvider(db *gorm.DB) error {
	if err := db.AutoMigrate(&migrationGuard{}); err != nil {
		return fmt.Errorf("migrate catalog_migration_guards: %w", err)
	}
	var guards int64
	if err := db.Model(&migrationGuard{}).
		Where("key = ?", removeRawProviderGuardKey).Count(&guards).Error; err != nil {
		return fmt.Errorf("check remove-raw-provider guard: %w", err)
	}
	if guards > 0 {
		return nil // already applied
	}
	if err := db.Where("name = ?", "raw").Delete(&domain.ScraperProvider{}).Error; err != nil {
		return fmt.Errorf("delete raw provider row: %w", err)
	}
	if err := db.Create(&migrationGuard{Key: removeRawProviderGuardKey}).Error; err != nil {
		return fmt.Errorf("write remove-raw-provider guard: %w", err)
	}
	return nil
}

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
// from the animefever provider description (AUTO-484 follow-up). SUPERSEDED by
// AnimefeverDisable (2026-07-05), which flips status→disabled and rewrites the same
// reason/description fields; Declaim is retained only because its guard already ran
// on prod (a run-once migration can't be edited after the fact). The seed is
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

// AnimefeverDisable flips animefever to status=disabled exactly once, carrying
// the durable disable to live DBs. animefever is dead for EVERYONE (its HLS
// segments 100% swap to a ByteDance ad CDN that 403s; a residential external
// A/B on 2026-06-26 proved the content is gone regardless of egress — NOT
// IP-class-fixable, falsifying AUTO-484). The live prod row was manually set to
// disabled on 2026-06-26, but the seed was StatusDegraded and the earlier
// AnimefeverDeclaim only refreshed reason/description (never status), so fresh
// DBs re-seeded animefever DEGRADED. The seed default is now StatusDisabled and
// this RUN-ONCE guarded migration flips any existing DB still on degraded.
// The provider CODE was removed from the scraper binary; the tombstone row is
// kept (scraper_operated) as the historical record + so the scraper's remote
// loader validates. Guarded via catalog_migration_guards so it never clobbers a
// later operator re-enable. Idempotent; safe to call every boot.
func AnimefeverDisable(db *gorm.DB) error {
	if err := db.AutoMigrate(&migrationGuard{}); err != nil {
		return fmt.Errorf("migrate catalog_migration_guards: %w", err)
	}
	var guards int64
	if err := db.Model(&migrationGuard{}).
		Where("key = ?", animefeverDisableGuardKey).Count(&guards).Error; err != nil {
		return fmt.Errorf("check animefever-disable guard: %w", err)
	}
	if guards > 0 {
		return nil // already applied — never clobber a later operator re-enable
	}
	result := db.Model(&domain.ScraperProvider{}).
		Where("name = ?", "animefever").
		Updates(map[string]interface{}{
			"status":       domain.StatusDisabled,
			"reason":       "Dead upstream — content gone for everyone (2026-06-26)",
			"description":  "animefever.cc → am.vidstream.vip (StreamX.Me/JW player) returns a valid manifest, but 100% of its HLS segments 302-redirect to a ByteDance ad CDN (sf16-scmcdn-sg.ibytedtos.com / ad-site-i18n-sg) that 403s. Proven NOT egress-fixable: a residential external A/B (owner, 2026-06-26) got no real video either — the content is dead for EVERYONE, not IP-class-gated (falsifies AUTO-484). Not revivable by any browser/egress trick. Disabled + provider code removed from the scraper binary (tombstone); this row is kept as the historical record. Existing DBs flipped via AnimefeverDisable.",
			"supports_sub": false,
			"supports_dub": false,
			"sub_delivery": "none",
		})
	if result.Error != nil {
		return fmt.Errorf("animefever disable: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		// No animefever row to update (seed did not run / row hard-deleted). Do NOT
		// write the guard, so a later boot (after the row exists) retries.
		return fmt.Errorf("animefever disable: no row found for name=animefever")
	}
	if err := db.Create(&migrationGuard{Key: animefeverDisableGuardKey}).Error; err != nil {
		return fmt.Errorf("write animefever-disable guard: %w", err)
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

// allanimeOkruMergeGuardKey marks AllanimeOkruMerge as applied.
const allanimeOkruMergeGuardKey = "allanime_okru_merge"

// AllanimeOkruMerge carries the okru+allanime fold to live DBs, exactly once.
// It renames the existing `okru` row to `allanime-okru` (preserving status /
// weight / engine) and disables the standalone `allanime` row (tombstone —
// its clock stream path is dead; discovery+ok.ru now ship as allanime-okru).
// On a fresh DB the seed already wrote both rows correctly, so either UPDATE
// may affect 0 rows — that is EXPECTED, not an error. Guard-gated + idempotent.
func AllanimeOkruMerge(db *gorm.DB) error {
	if err := db.AutoMigrate(&migrationGuard{}); err != nil {
		return fmt.Errorf("migrate catalog_migration_guards: %w", err)
	}
	var guards int64
	if err := db.Model(&migrationGuard{}).
		Where("key = ?", allanimeOkruMergeGuardKey).Count(&guards).Error; err != nil {
		return fmt.Errorf("check allanime-okru-merge guard: %w", err)
	}
	if guards > 0 {
		return nil // already applied — never clobber a later operator edit
	}
	// 1) Rename okru -> allanime-okru (only if no allanime-okru row exists yet,
	//    so we never collide with a fresh-DB seed row).
	var already int64
	if err := db.Model(&domain.ScraperProvider{}).
		Where("name = ?", "allanime-okru").Count(&already).Error; err != nil {
		return fmt.Errorf("check allanime-okru presence: %w", err)
	}
	if already == 0 {
		if err := db.Model(&domain.ScraperProvider{}).
			Where("name = ?", "okru").
			Updates(map[string]interface{}{
				"name":        "allanime-okru",
				"reason":      "AllAnime discovery + ok.ru ('Ok') CDN streams (clock-free)",
				"description": "Folded okru+allanime (2026-07-06). AllAnime GraphQL discovery + ok.ru data-options → okcdn.ru HLS, bypassing the Cloudflare-Turnstile /apivtwo/clock endpoint. EN sub/dub, hardsubbed.",
			}).Error; err != nil {
			return fmt.Errorf("rename okru->allanime-okru: %w", err)
		}
	}
	// 2) Tombstone the standalone allanime row (degraded -> disabled).
	if err := db.Model(&domain.ScraperProvider{}).
		Where("name = ?", "allanime").
		Updates(map[string]interface{}{
			"status": domain.StatusDisabled,
			"reason": "Folded into allanime-okru (2026-07-06) — clock stream path was dead",
		}).Error; err != nil {
		return fmt.Errorf("disable allanime: %w", err)
	}
	if err := db.Create(&migrationGuard{Key: allanimeOkruMergeGuardKey}).Error; err != nil {
		return fmt.Errorf("write allanime-okru-merge guard: %w", err)
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

// AddAnimejoyProviders INSERTs the two catalog-side animejoy RU-sub provider rows
// — "animejoy-sibnet" and "animejoy-allvideo" — exactly once, IF ABSENT. animejoy
// itself is NOT a row (it is the shared discovery/reference base); these two rows
// each resolve their own leg (Sibnet primary 6/6, AllVideo 5/6 per AUTO-084) off
// that shared discovery. Seeded DEGRADED (soak first): registered + manually
// selectable (hacker mode), out of the auto-failover chain. RU-SUB only —
// original (JP) audio + burned-in Russian subs in the mirror MP4s (SubDelivery=hard,
// no dub, no raw).
//
// SeedDefaults covers fresh DBs (insert-if-absent); this RUN-ONCE guarded migration
// carries the same two rows to the EXISTING live prod DB (server IS prod). Because
// this is a raw INSERT (NOT SeedDefaults), the intrinsicGroup/scraper_operated
// stamping that SeedDefaults applies does NOT run here, so group='ru' and
// scraper_operated=false are set EXPLICITLY in the insert — scraper_operated=false
// keeps these catalog-operated RU rows out of the EN scraper-failover chain (the
// EN-only candidateProviders invariant crash-loops boot if an EN-unlisted provider
// is pulled in). policy=manual + health=down mirror BackfillPolicyHealth's degraded
// mapping so the DERIVED WireStatus() is degraded regardless of whether the
// policy/health backfill runs before or after this migration.
//
// Idempotent: guard ledger + per-row insert-if-absent (an operator who later deletes
// or re-configures either row is never clobbered once the guard is written). Safe to
// call every boot. The rows are intentionally dormant until the capability family /
// FE adapter ship in a later phase — a stream_providers row with no family simply
// does not surface, so this is safe to land now.
func AddAnimejoyProviders(db *gorm.DB) error {
	if err := db.AutoMigrate(&migrationGuard{}); err != nil {
		return fmt.Errorf("migrate catalog_migration_guards: %w", err)
	}
	var guards int64
	if err := db.Model(&migrationGuard{}).
		Where("key = ?", addAnimejoyProvidersGuardKey).Count(&guards).Error; err != nil {
		return fmt.Errorf("check add-animejoy-providers guard: %w", err)
	}
	if guards > 0 {
		return nil // already applied — never clobber later operator edits
	}

	now := time.Now().UTC()
	rows := []domain.ScraperProvider{
		{
			Name: "animejoy-sibnet", Status: domain.StatusDegraded,
			Group: "ru", ScraperOperated: false,
			Policy: domain.PolicyManual, Health: domain.HealthDown,
			PolicySince: now, HealthSince: now,
			SupportsSub: true, SupportsDub: false, SupportsRaw: false,
			SubDelivery: "hard", QualityCeiling: "1080p", PreferenceWeight: 25,
			Reason:      "Soaking — animejoy.ru via Sibnet",
			Description: "Sibnet (AnimeJoy, RU-sub)",
		},
		{
			Name: "animejoy-allvideo", Status: domain.StatusDegraded,
			Group: "ru", ScraperOperated: false,
			Policy: domain.PolicyManual, Health: domain.HealthDown,
			PolicySince: now, HealthSince: now,
			SupportsSub: true, SupportsDub: false, SupportsRaw: false,
			SubDelivery: "hard", QualityCeiling: "1080p", PreferenceWeight: 20,
			Reason:      "Soaking — animejoy.ru via AllVideo",
			Description: "AllVideo (AnimeJoy, RU-sub)",
		},
	}
	for _, r := range rows {
		var count int64
		if err := db.Model(&domain.ScraperProvider{}).
			Where("name = ?", r.Name).Count(&count).Error; err != nil {
			return fmt.Errorf("count %q: %w", r.Name, err)
		}
		if count > 0 {
			continue // insert-if-absent: never overwrite an existing/operator-edited row
		}
		row := r
		if err := db.Create(&row).Error; err != nil {
			return fmt.Errorf("create %q: %w", r.Name, err)
		}
	}

	if err := db.Create(&migrationGuard{Key: addAnimejoyProvidersGuardKey}).Error; err != nil {
		return fmt.Errorf("write add-animejoy-providers guard: %w", err)
	}
	return nil
}

// MiruroCloudflareBlock records, exactly once, the 2026-07-02 daily-recovery-run
// finding that www.miruro.tv now sits behind a Cloudflare WAF managed-rule block
// (a plain unauthenticated GET on "/" returns Cloudflare's "Sorry, you have been
// blocked" firewall page, not the interactive Turnstile challenge — confirmed on
// every path including static assets, and identically reproduced by the live Go
// scraper's own properly-encoded secure-pipe requests, not just ad-hoc curl).
// Miruro was healthy as recently as 2026-07-01T12:00Z (real stream resolutions
// logged); the block started at the 2026-07-02T00:00 UTC probe and has held
// steady across manual retries since. This reappears the exact T-28-04-01
// "Cloudflare challenge" threat doc.go already flagged: the provider's stdlib-only,
// no-headless-browser client (D3 gate 2) cannot solve it, so a real fix requires
// routing miruro through the Camoufox stealth-scraper roster (a "v3.2"-class
// architecture change, correctly out of scope for a single automated recovery run).
//
// Deliberately updates ONLY `description` — never `reason` (the probe/state-machine
// overwrites `reason` on every cycle; only `description` is durable operator
// context) and never `status`/`policy`/`health` (health is owned by the probe
// state machine, policy by the admin — no need to fight either here). Guarded
// via the ledger so a later operator's own description edit is never clobbered.
func MiruroCloudflareBlock(db *gorm.DB) error {
	if err := db.AutoMigrate(&migrationGuard{}); err != nil {
		return fmt.Errorf("migrate catalog_migration_guards: %w", err)
	}
	var guards int64
	if err := db.Model(&migrationGuard{}).
		Where("key = ?", miruroCloudflareBlockGuardKey).Count(&guards).Error; err != nil {
		return fmt.Errorf("check miruro-cloudflare-block guard: %w", err)
	}
	if guards > 0 {
		return nil // already applied — never clobber a later operator edit
	}
	result := db.Model(&domain.ScraperProvider{}).
		Where("name = ?", "miruro").
		Update("description", "Miruro aggregator: AnimePahe/kwik.cx HLS via the kiwi server (vault-*.uwucdn HLS), 1080p, AES-128 encrypted segments served through the streaming proxy. EN sub. Playback-probed. As of 2026-07-02, www.miruro.tv sits behind a Cloudflare WAF managed-rule block on every path (including the bare homepage and static assets) — a hard 'Sorry, you have been blocked' firewall page, not a solvable Turnstile challenge, and identically reproduced by the live scraper's own properly-encoded requests. Was healthy through 2026-07-01T12:00Z. This is the T-28-04-01 threat doc.go already anticipated: the stdlib-only Go client (no headless browser, D3 gate 2) cannot pass it. A real fix needs routing miruro through the Camoufox stealth-scraper roster (v3.2-class change) — flagged for human review, not attempted in an automated recovery run.")
	if result.Error != nil {
		return fmt.Errorf("miruro cloudflare-block: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		// No miruro row to update (seed did not run / row hard-deleted). Do NOT
		// write the guard, so a later boot (after the row exists) retries.
		return fmt.Errorf("miruro cloudflare-block: no row found for name=miruro")
	}
	if err := db.Create(&migrationGuard{Key: miruroCloudflareBlockGuardKey}).Error; err != nil {
		return fmt.Errorf("write miruro-cloudflare-block guard: %w", err)
	}
	return nil
}

// MiruroBrowserRevival flips the miruro roster row to engine="browser" exactly
// once, reviving it through the Camoufox stealth-scraper roster after the
// 2026-07-02 Cloudflare block (MiruroCloudflareBlock). A Phase-0 spike proved the
// migration viable (WORLD A): solving www.miruro.tv's homepage Turnstile lets an
// in-page fetch to the hard-WAF-blocked /api/secure/pipe return 200 + a decodable
// x-obfuscated body — so the block only applies to un-cleared clients, exactly
// like the animepahe revival (AnimepaheBrowserRevival). Go still builds the `e=`
// descriptor + decodes the x-obfuscated envelope (Approach 2); the sidecar only
// fetches through the solved session.
//
// Seeded DEGRADED (owner pref: manually selectable via ?prefer=miruro&exclusive,
// out of the auto-failover chain pending live soak — and miruro is dub-only). The
// scraper roster derives the wire status via WireStatus(policy, health), NOT the
// stored `status` column, so the authoritative levers are policy=manual +
// health=down (→ WireStatus=degraded); `status` is updated too for column
// consistency (cosmetic). Guarded via catalog_migration_guards so it's a no-op on
// every later boot and never clobbers a later operator change. Idempotent.
func MiruroBrowserRevival(db *gorm.DB) error {
	if err := db.AutoMigrate(&migrationGuard{}); err != nil {
		return fmt.Errorf("migrate catalog_migration_guards: %w", err)
	}
	var guards int64
	if err := db.Model(&migrationGuard{}).
		Where("key = ?", miruroBrowserRevivalGuardKey).Count(&guards).Error; err != nil {
		return fmt.Errorf("check miruro-browser-revival guard: %w", err)
	}
	if guards > 0 {
		return nil // already applied — never clobber a later operator change
	}
	now := time.Now().UTC()
	result := db.Model(&domain.ScraperProvider{}).
		Where("name = ?", "miruro").
		Updates(map[string]interface{}{
			"engine":       "browser",
			"base_url":     "https://www.miruro.tv",
			"status":       domain.StatusDegraded,
			"policy":       string(domain.PolicyManual),
			"health":       string(domain.HealthDown),
			"policy_since": now,
			"health_since": now,
			"reason":       "Browser-scraped via Camoufox sidecar (www.miruro.tv Cloudflare Turnstile solved)",
			"description":  "Miruro aggregator (AnimePahe/kwik.cx HLS via the kiwi server, 1080p AES-128, EN dub). As of 2026-07-02 www.miruro.tv sits behind Cloudflare — an interactive Turnstile on the SPA and a hard WAF block on /api/secure/pipe for un-cleared clients. Revived engine=browser: the Camoufox stealth-scraper warm /fetch session solves the homepage Turnstile (~9s on our own IP, no residential proxy); the in-page fetch to /api/secure/pipe then rides cf_clearance and is served as the SPA (verified live). Go builds the secure-pipe descriptor + decodes the x-obfuscated response (Approach 2); the x-obfuscated response header is surfaced through the sidecar /fetch header allowlist. Degraded: manually selectable (hacker mode), out of the auto-failover chain pending live soak.",
		})
	if result.Error != nil {
		return fmt.Errorf("miruro browser revival: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		// No miruro row to flip (seed did not run / row hard-deleted). Do NOT write
		// the guard, so a later boot (after the row exists) retries.
		return fmt.Errorf("miruro browser revival: no row found for name=miruro")
	}
	if err := db.Create(&migrationGuard{Key: miruroBrowserRevivalGuardKey}).Error; err != nil {
		return fmt.Errorf("write miruro-browser-revival guard: %w", err)
	}
	return nil
}

// AllanimeOkruCryptoBlock records, exactly once, the 2026-07-07 daily-recovery-run
// finding that AllAnime's own `api.allanime.day` GraphQL API now rejects every
// `episode(...)` (sourceUrls) query — the resolver allanime-okru's discovery
// depends on for BOTH its AllAnime-native sources and its ok.ru/okcdn.ru CDN
// leg — with a new application-level error `{"code":"AA_CRYPTO_MISSING"}` on
// `path:["episode"]`. Confirmed from clean egress: FindID (shows search) and
// ListEpisodes (show detail) still succeed normally; only the sourceUrls
// resolver is gated. Reproduced deterministically across 2 different shows,
// both sub and dub translationType, and with/without the full query text (vs
// persisted-query-only) — the persisted-query-only path (what our client
// sends) avoids Cloudflare's interactive Turnstile entirely and reaches the
// GraphQL resolver cleanly, which then itself returns AA_CRYPTO_MISSING. This
// is a deliberate, fresh anti-scraping gate on AllAnime's highest-value
// resolver (the one that leaks playable stream URLs), not a transport/CDN
// issue — no request-shape variation (Referer, User-Agent, translationType)
// changes the outcome. A real fix requires reverse-engineering what
// crypto/signature material AllAnime's own web client now sends (likely only
// derivable by rendering their real frontend in a browser — a "v3.2"-class
// architecture change in the same family as the miruro Cloudflare block
// (MiruroCloudflareBlock) and the original allanime clock.json Turnstile
// wall), correctly out of scope for a single automated recovery run.
//
// Deliberately updates ONLY `description` — never `reason` (the probe/state-machine
// overwrites `reason` on every cycle; only `description` is durable operator
// context) and never `status`/`policy`/`health` (health is owned by the probe
// state machine, policy by the admin). Guarded via the ledger so a later
// operator's own description edit is never clobbered.
func AllanimeOkruCryptoBlock(db *gorm.DB) error {
	if err := db.AutoMigrate(&migrationGuard{}); err != nil {
		return fmt.Errorf("migrate catalog_migration_guards: %w", err)
	}
	var guards int64
	if err := db.Model(&migrationGuard{}).
		Where("key = ?", allanimeOkruCryptoBlockGuardKey).Count(&guards).Error; err != nil {
		return fmt.Errorf("check allanime-okru-crypto-block guard: %w", err)
	}
	if guards > 0 {
		return nil // already applied — never clobber a later operator edit
	}
	result := db.Model(&domain.ScraperProvider{}).
		Where("name = ?", "allanime-okru").
		Update("description", "AllAnime GraphQL discovery + ok.ru ('Ok') CDN streams (clock-free). As of 2026-07-07, api.allanime.day's `episode` GraphQL resolver (sourceUrls — the field both the AllAnime-native and ok.ru CDN legs depend on) rejects every query with a new application-level error `{\"code\":\"AA_CRYPTO_MISSING\"}` on path:[\"episode\"] — confirmed from clean egress on 2 shows, sub+dub, with/without full query text. Search and show-detail queries are unaffected; only sourceUrls is gated. This is a deliberate fresh anti-scraping measure on AllAnime's highest-value resolver, not a CDN/transport blip — no request-shape variation changes the outcome. A real fix needs reverse-engineering AllAnime's client-side crypto/signature scheme (likely only derivable via a real browser render, a 'v3.2'-class change in the same family as MiruroCloudflareBlock) — flagged for human review, not attempted in an automated recovery run.")
	if result.Error != nil {
		return fmt.Errorf("allanime-okru crypto-block: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		// No allanime-okru row to update (seed did not run / row hard-deleted). Do
		// NOT write the guard, so a later boot (after the row exists) retries.
		return fmt.Errorf("allanime-okru crypto-block: no row found for name=allanime-okru")
	}
	if err := db.Create(&migrationGuard{Key: allanimeOkruCryptoBlockGuardKey}).Error; err != nil {
		return fmt.Errorf("write allanime-okru-crypto-block guard: %w", err)
	}
	return nil
}

// BumpKodikNoadsPriority raises the kodik-noads roster row's preference_weight to
// 90 exactly once, carrying the new Source-panel rank to live DBs (2026-07-07).
// The `kodik` capability family reads THIS row's weight into cap.Order, and the
// FE sorts the active bucket by order desc, so this lifts Kodik from dead-last
// (weight 0) to directly under the first-party `ae` (100) and above every other
// source — the EN scraper chain (gogoanime 85, …) and the AnimeJoy RU-sub legs
// (sibnet 25, allvideo 20). The seed is insert-if-absent and never updates an
// existing prod row; this RUN-ONCE guarded migration is the only thing that
// bumps live DBs. Guarded via the catalog_migration_guards ledger so it is a
// no-op on every later boot and an operator who re-tunes the weight in the DB is
// NOT clobbered. Idempotent; safe to call every boot.
func BumpKodikNoadsPriority(db *gorm.DB) error {
	if err := db.AutoMigrate(&migrationGuard{}); err != nil {
		return fmt.Errorf("migrate catalog_migration_guards: %w", err)
	}
	var guards int64
	if err := db.Model(&migrationGuard{}).
		Where("key = ?", bumpKodikNoadsPriorityGuardKey).Count(&guards).Error; err != nil {
		return fmt.Errorf("check bump-kodik-noads-priority guard: %w", err)
	}
	if guards > 0 {
		return nil // already applied — never clobber a later operator re-tune
	}
	result := db.Model(&domain.ScraperProvider{}).
		Where("name = ?", "kodik-noads").
		Update("preference_weight", 90)
	if result.Error != nil {
		return fmt.Errorf("bump kodik-noads priority (preference_weight=90): %w", result.Error)
	}
	if result.RowsAffected == 0 {
		// No kodik-noads row to bump (seed did not run / row hard-deleted). Do NOT
		// write the guard, so a later boot (after the row exists) retries.
		return fmt.Errorf("bump kodik-noads priority: no row found for name=kodik-noads")
	}
	if err := db.Create(&migrationGuard{Key: bumpKodikNoadsPriorityGuardKey}).Error; err != nil {
		return fmt.Errorf("write bump-kodik-noads-priority guard: %w", err)
	}
	return nil
}

// ReconcilePolicyFromHealthV1 aligns existing rows to the 2026-07-13
// health-driven policy rule exactly once: for every non-disabled row, policy =
// (health==down ? manual : auto), with policy_since stamped and status derived
// via WireStatus(policy, health). This is the one-shot backfill of what
// providerpolicy.ReconcilePolicyFromHealth now maintains on every probe tick —
// it flips already-down auto providers (animepahe / allanime-okru / gogoanime)
// to manual (parked, hacker-selectable) and leaves up/degraded/recovering as
// auto. disabled rows are untouched (admin hard-lock). Guarded so it never
// clobbers later machine writes on reboot; runs AFTER BackfillPolicyHealth.
func ReconcilePolicyFromHealthV1(db *gorm.DB) error {
	if err := db.AutoMigrate(&migrationGuard{}); err != nil {
		return fmt.Errorf("migrate catalog_migration_guards: %w", err)
	}
	var guards int64
	if err := db.Model(&migrationGuard{}).
		Where("key = ?", reconcilePolicyFromHealthGuardKey).Count(&guards).Error; err != nil {
		return fmt.Errorf("check reconcile-policy-from-health guard: %w", err)
	}
	if guards > 0 {
		return nil // already applied — never clobber later machine writes
	}

	now := time.Now().UTC()
	// Derive (policy, status) per health from the SAME source of truth the live
	// probe path uses — providerpolicy.ReconcilePolicyFromHealth + WireStatus() —
	// rather than hand-coding the mapping in SQL (down⇒manual/degraded,
	// up⇒auto/enabled, degraded⇒auto/enabled [one-blip buffer, stays in chain],
	// recovering⇒auto/degraded [soaks out]). One set-based UPDATE per health value.
	for _, hlth := range []domain.ProviderHealth{domain.HealthDown, domain.HealthUp, domain.HealthDegraded, domain.HealthRecovering} {
		p := domain.ScraperProvider{Policy: domain.PolicyAuto, Health: hlth}
		providerpolicy.ReconcilePolicyFromHealth(&p, now)
		if err := db.Model(&domain.ScraperProvider{}).
			Where("policy <> ? AND health = ?", domain.PolicyDisabled, hlth).
			Updates(map[string]any{
				"policy":       string(p.Policy),
				"policy_since": now,
				"status":       string(p.WireStatus()),
			}).Error; err != nil {
			return fmt.Errorf("reconcile %s: %w", hlth, err)
		}
	}

	if err := db.Create(&migrationGuard{Key: reconcilePolicyFromHealthGuardKey}).Error; err != nil {
		return fmt.Errorf("write reconcile-policy-from-health guard: %w", err)
	}
	return nil
}

// AllanimeOkruCryptoGateLifted refreshes ONLY allanime-okru's description once,
// superseding the stale 2026-07-07 AllanimeOkruCryptoBlock tombstone: as of
// 2026-07-13 the upstream AA_CRYPTO_MISSING gate has LIFTED — api.allanime.day's
// `episode` (sourceUrls) resolver decrypts again (verified from clean egress),
// and the provider serves real content per-title (Re:Zero S4 / Steel Ball Run:
// real 1080p MPEG-TS via ok.ru/okcdn.ru). Its "down" state is a per-title
// artifact: ok.ru copyright-blocks some high-profile titles (e.g. the probe
// anchor Frieren), so the anchor-gated probe can't self-recover it, but users
// can hand-select it and get real streams for the many titles ok.ru does carry.
// Mirrors MiruroCloudflareBlock/AllanimeOkruCryptoBlock exactly — touches only
// `description` (never reason/status/policy/health, which are machine/admin
// owned). A NEW guard key so it applies once on top of the earlier tombstone.
func AllanimeOkruCryptoGateLifted(db *gorm.DB) error {
	if err := db.AutoMigrate(&migrationGuard{}); err != nil {
		return fmt.Errorf("migrate catalog_migration_guards: %w", err)
	}
	var guards int64
	if err := db.Model(&migrationGuard{}).
		Where("key = ?", allanimeOkruCryptoLiftedGuardKey).Count(&guards).Error; err != nil {
		return fmt.Errorf("check allanime-okru-crypto-lifted guard: %w", err)
	}
	if guards > 0 {
		return nil // already applied — never clobber a later operator edit
	}
	result := db.Model(&domain.ScraperProvider{}).
		Where("name = ?", "allanime-okru").
		Update("description", "AllAnime GraphQL discovery + ok.ru ('Ok') CDN streams (clock-free). As of 2026-07-13 the 2026-07-07 AA_CRYPTO_MISSING gate has LIFTED — api.allanime.day's `episode`/sourceUrls resolver decrypts again (verified from clean egress). The provider serves REAL content per-title (e.g. Re:Zero S4, Steel Ball Run: real 1080p MPEG-TS via ok.ru/okcdn.ru). It is ok.ru-only, so availability is per-title: titles whose ok.ru copy is copyright-blocked (e.g. the probe anchor Frieren) fail with 'no data-options', which is why the anchor-gated probe can't auto-recover it — but it is hacker-selectable and serves real streams for the many titles ok.ru carries. Supersedes the stale AA_CRYPTO_MISSING tombstone.")
	if result.Error != nil {
		return fmt.Errorf("allanime-okru crypto-lifted: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		// No allanime-okru row to update. Do NOT write the guard, so a later boot
		// (after the row exists) retries.
		return fmt.Errorf("allanime-okru crypto-lifted: no row found for name=allanime-okru")
	}
	if err := db.Create(&migrationGuard{Key: allanimeOkruCryptoLiftedGuardKey}).Error; err != nil {
		return fmt.Errorf("write allanime-okru-crypto-lifted guard: %w", err)
	}
	return nil
}

// BackfillProviderIdentityV1 stamps display_name/player_key/anime_level onto
// pre-existing prod rows exactly once (AUTO-608). The seed is insert-if-absent
// and never updates prod rows; this run-once guarded migration is what carries
// the new identity columns to live DBs. It reads the values straight off
// defaultProviders (the seed table), so the two can never drift.
func BackfillProviderIdentityV1(db *gorm.DB) error {
	if err := db.AutoMigrate(&migrationGuard{}); err != nil {
		return fmt.Errorf("migrate catalog_migration_guards: %w", err)
	}
	var guards int64
	if err := db.Model(&migrationGuard{}).
		Where("key = ?", backfillProviderIdentityGuardKey).Count(&guards).Error; err != nil {
		return fmt.Errorf("check backfill-provider-identity guard: %w", err)
	}
	if guards > 0 {
		return nil // applied — never clobber later operator edits
	}
	for _, p := range defaultProviders {
		if err := db.Model(&domain.ScraperProvider{}).Where("name = ?", p.Name).
			Updates(map[string]any{
				"display_name": p.DisplayName,
				"player_key":   p.PlayerKey,
				"anime_level":  p.AnimeLevel,
			}).Error; err != nil {
			return fmt.Errorf("backfill provider identity %q: %w", p.Name, err)
		}
		// RowsAffected 0 is fine — absent rows are created complete by the seed.
	}
	if err := db.Create(&migrationGuard{Key: backfillProviderIdentityGuardKey}).Error; err != nil {
		return fmt.Errorf("write backfill-provider-identity guard: %w", err)
	}
	return nil
}
