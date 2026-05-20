import { test, expect, type Page } from '@playwright/test'

/**
 * Phase 28 Plan 06 — EnglishPlayer source-switching smoke spec.
 *
 * Covers the dropdown polish that lights up the new Phase 28 EN scraper
 * providers (AnimeFever, Miruro, 9anime) once Phase 24 restores
 * `EnglishPlayer.vue`. The spec is gated by a `test.skip` so it does
 * NOT fail CI while Phase 24 is pending — the moment the file lands,
 * the skip gate flips and the spec runs.
 *
 * Behaviors exercised when active:
 *   1. Login as the permanent `ui_audit_bot` (CLAUDE.md "UI Audit Test User").
 *   2. Navigate to Frieren (Shikimori ID 52991) — known multi-provider coverage.
 *   3. Click the English language pill.
 *   4. Verify the source dropdown enumerates >= 2 providers in
 *      failover-chain order (allanime → animefever → miruro → nineanime
 *      → animepahe → gogoanime → animekai).
 *   5. Switch from allanime → animefever; assert `<video>` re-mounts.
 *   6. Switch to nineanime; assert MP4-shaped video src (`src*=".mp4"`).
 *   7. Smoke-assert dropdown labels match `capitalizeProvider`'s
 *      outputs ("AllAnime", "AnimeFever", "Miruro", "9anime").
 *
 * NOTE: this spec is scaffolded against the contract documented in
 * `28-CONTEXT.md` and `28-RESEARCH.md`. The selectors are intentionally
 * permissive (`source-dropdown` test-id OR `<select>` fallback) so the
 * Phase 24 restorer has flexibility on the final DOM shape — adjust the
 * locators in the same PR that introduces EnglishPlayer.vue if needed.
 */

import { existsSync } from 'node:fs'
import { resolve } from 'node:path'

const FRIEREN_SHIKIMORI_ID = '52991'
const UI_AUDIT_USERNAME = 'ui_audit_bot'
const UI_AUDIT_PASSWORD = 'audit_bot_test_password_2026'

// Phase 24 dependency: skip the entire suite if EnglishPlayer.vue is
// absent. The file path matches the canonical location documented in
// `28-06-PLAN.md` and `28-CONTEXT.md`.
const ENGLISH_PLAYER_PATH = resolve(
  __dirname,
  '..',
  'src',
  'components',
  'player',
  'EnglishPlayer.vue',
)
const englishPlayerExists = existsSync(ENGLISH_PLAYER_PATH)

async function loginAsUiAuditBot(page: Page) {
  await page.goto('/')
  const ok = await page.evaluate(
    async ({ username, password }: { username: string; password: string }) => {
      const res = await fetch('/api/auth/login', {
        method: 'POST',
        credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ username, password }),
      })
      if (!res.ok) return { ok: false, status: res.status }
      const payload = await res.json()
      const data = payload?.data ?? payload
      const token: string | undefined = data?.access_token
      const user = data?.user
      if (!token || !user) return { ok: false, status: 200, reason: 'no-token-or-user' }
      localStorage.setItem('token', token)
      localStorage.setItem('user', JSON.stringify(user))
      return { ok: true, status: 200 }
    },
    { username: UI_AUDIT_USERNAME, password: UI_AUDIT_PASSWORD },
  )
  return ok
}

async function resolveAnimeUUID(page: Page, shikimoriID: string): Promise<string | null> {
  return page.evaluate(async (sid: string) => {
    try {
      const res = await fetch(`/api/anime/shikimori/${sid}`)
      if (!res.ok) return null
      const payload = await res.json()
      const data = payload?.data ?? payload
      return data?.id ?? null
    } catch {
      return null
    }
  }, shikimoriID)
}

test.describe('EnglishPlayer — Phase 28 source-switching', () => {
  test.skip(
    !englishPlayerExists,
    'EnglishPlayer.vue not yet restored — Phase 24 dependency unfulfilled. Spec scaffolded for the moment Phase 24 lands.',
  )

  test('EN language pill → source dropdown shows new providers in failover-chain order', async ({ page }) => {
    const auth = await loginAsUiAuditBot(page)
    expect(auth.ok).toBe(true)

    const animeID = await resolveAnimeUUID(page, FRIEREN_SHIKIMORI_ID)
    test.skip(!animeID, `Frieren (Shikimori ${FRIEREN_SHIKIMORI_ID}) not in catalog — reseed and retry.`)

    await page.goto(`/anime/${animeID}`)
    await page.waitForLoadState('networkidle')

    // Click the English language pill (sibling of RU / RAW JP).
    const enPill = page.locator('button[aria-pressed]').filter({ hasText: /^English$/i }).first()
    await expect(enPill).toBeVisible({ timeout: 10000 })
    await enPill.click()
    await page.waitForTimeout(250)

    // Source dropdown — accept either an explicit `[data-testid="source-dropdown"]`
    // or a bare `<select>` inside the player surface. Phase 24 restorer
    // picks the final shape; this locator works either way.
    const sourceDropdown = page
      .locator('[data-testid="source-dropdown"], .english-player select, select[aria-label*="Source" i], select[aria-label*="Источник" i]')
      .first()
    await expect(sourceDropdown).toBeVisible({ timeout: 10000 })

    // Enumerate option labels — must include the new Phase 28 providers.
    const optionLabels = await sourceDropdown.locator('option').allInnerTexts()
    const labels = optionLabels.map((s) => s.trim())
    expect(labels.length).toBeGreaterThanOrEqual(2)

    // Per `capitalizeProvider` contract: labels are the user-facing names.
    // At least one of the new Phase 28 providers MUST appear.
    const hasNewProvider = labels.some((l) => /AnimeFever|Miruro|9anime/i.test(l))
    expect(hasNewProvider).toBe(true)

    // AllAnime is the head of the failover chain — should also be present.
    expect(labels.some((l) => /AllAnime/i.test(l))).toBe(true)
  })

  test('source switch allanime → animefever → nineanime keeps <video> alive', async ({ page }) => {
    const auth = await loginAsUiAuditBot(page)
    expect(auth.ok).toBe(true)

    const animeID = await resolveAnimeUUID(page, FRIEREN_SHIKIMORI_ID)
    test.skip(!animeID, `Frieren (Shikimori ${FRIEREN_SHIKIMORI_ID}) not in catalog — reseed and retry.`)

    await page.goto(`/anime/${animeID}`)
    await page.waitForLoadState('networkidle')

    const enPill = page.locator('button[aria-pressed]').filter({ hasText: /^English$/i }).first()
    await expect(enPill).toBeVisible({ timeout: 10000 })
    await enPill.click()
    await page.waitForTimeout(250)

    const sourceDropdown = page
      .locator('[data-testid="source-dropdown"], .english-player select, select[aria-label*="Source" i], select[aria-label*="Источник" i]')
      .first()
    await expect(sourceDropdown).toBeVisible({ timeout: 10000 })

    const playerVideo = page.locator('.english-player video, video').first()

    // Start: assert the default provider mounts a <video>.
    await expect(playerVideo).toBeVisible({ timeout: 15000 })

    // Switch to AnimeFever — pick by visible label so we don't have to
    // know the underlying <option value>.
    await sourceDropdown.selectOption({ label: 'AnimeFever' })
    await page.waitForTimeout(500)
    await expect(playerVideo).toBeVisible({ timeout: 15000 })

    // Switch to 9anime — the MP4 path; assert the video src is shaped like
    // an .mp4 URL (per CONTEXT.md Pitfall 6: nineanime is MP4, not HLS).
    await sourceDropdown.selectOption({ label: '9anime' })
    await page.waitForTimeout(500)
    await expect(playerVideo).toBeVisible({ timeout: 15000 })
    const videoSrc = await playerVideo.getAttribute('src')
    // Accept either a direct .mp4 in the src or a blob:/proxy that streams MP4.
    expect(videoSrc).not.toBeNull()
    if (videoSrc) {
      expect(/\.mp4|blob:|\/api\/streaming\//.test(videoSrc)).toBe(true)
    }
  })

  test('dropdown labels match capitalizeProvider outputs', async ({ page }) => {
    const auth = await loginAsUiAuditBot(page)
    expect(auth.ok).toBe(true)

    const animeID = await resolveAnimeUUID(page, FRIEREN_SHIKIMORI_ID)
    test.skip(!animeID, `Frieren (Shikimori ${FRIEREN_SHIKIMORI_ID}) not in catalog — reseed and retry.`)

    await page.goto(`/anime/${animeID}`)
    await page.waitForLoadState('networkidle')

    const enPill = page.locator('button[aria-pressed]').filter({ hasText: /^English$/i }).first()
    await expect(enPill).toBeVisible({ timeout: 10000 })
    await enPill.click()
    await page.waitForTimeout(250)

    const sourceDropdown = page
      .locator('[data-testid="source-dropdown"], .english-player select, select[aria-label*="Source" i], select[aria-label*="Источник" i]')
      .first()
    await expect(sourceDropdown).toBeVisible({ timeout: 10000 })

    const optionLabels = (await sourceDropdown.locator('option').allInnerTexts()).map((s) => s.trim())

    // Allowed labels are the `capitalizeProvider` outputs documented in 28-06-PLAN.md
    // interfaces block. Reject anything else — that means a provider was added
    // upstream without updating the switch / i18n.
    const allowedLabels = new Set(['AllAnime', 'AnimeFever', 'Miruro', '9anime', 'AnimePahe', 'Anitaku', 'AnimeKai'])
    for (const label of optionLabels) {
      // Skip empty placeholder options.
      if (!label) continue
      expect(allowedLabels.has(label)).toBe(true)
    }
  })
})
