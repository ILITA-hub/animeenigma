import { test, expect } from '@playwright/test'

/**
 * Workstream raw-jp, Phase 04 — RawPlayer smoke verification.
 *
 * Covers the happy path defined in
 * `.planning/workstreams/raw-jp/milestones/v0.1-phases/04-frontend-wiring/04-SPEC.md`:
 *   1. Login as the permanent `ui_audit_bot`.
 *   2. Navigate to Bocchi the Rock (known AllAnime coverage).
 *   3. Switch to the new "RAW JP" language group.
 *   4. Click the AllAnime provider chip → RawPlayer mounts.
 *   5. Assert subtitle picker dropdown renders (at least the "Off" option).
 *   6. Click "Other subs" → the OtherSubsPanel modal opens.
 *
 * The test runs against the env-flag-gated chip — requires
 * `VITE_RAW_PROVIDER_ENABLED=true` at build/dev time (set in
 * `frontend/web/.env`). Without the flag the chip is hidden and the test
 * is skipped via the rawJpVisible early-return.
 */

const BOCCHI_SHIKIMORI_ID = '52082' // Bocchi the Rock!
const UI_AUDIT_USERNAME = 'ui_audit_bot'
const UI_AUDIT_PASSWORD = 'audit_bot_test_password_2026'

async function loginAsUiAuditBot(page: import('@playwright/test').Page) {
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

async function resolveAnimeUUID(page: import('@playwright/test').Page, shikimoriID: string): Promise<string | null> {
  const result = await page.evaluate(async (sid: string) => {
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
  return result
}

test.describe('RawPlayer — workstream raw-jp Phase 04 wiring', () => {
  test('RAW JP language pill + AllAnime chip mount RawPlayer + Other Subs button opens panel', async ({ page }) => {
    const auth = await loginAsUiAuditBot(page)
    expect(auth.ok).toBe(true)

    const animeID = await resolveAnimeUUID(page, BOCCHI_SHIKIMORI_ID)
    test.skip(!animeID, `Bocchi the Rock (Shikimori ${BOCCHI_SHIKIMORI_ID}) not in catalog — reseed and retry.`)

    await page.goto(`/anime/${animeID}`)
    await page.waitForLoadState('networkidle')

    // The chip group sits in the provider switcher. Look for the RAW JP pill
    // which is rendered only when VITE_RAW_PROVIDER_ENABLED=true at build.
    const rawJpPill = page.locator('button[aria-pressed]').filter({ hasText: 'RAW JP' }).first()
    const rawJpVisible = await rawJpPill.isVisible({ timeout: 3000 }).catch(() => false)
    test.skip(!rawJpVisible, 'VITE_RAW_PROVIDER_ENABLED not set at build — RAW JP pill hidden.')

    await rawJpPill.click()
    await page.waitForTimeout(200) // let switchLanguage propagate

    // Activate the player (the cover-image click-to-load placeholder fronts the
    // real player until first interaction).
    const activateBtn = page
      .locator('button')
      .filter({ hasText: /(Watch now|Continue|Resume)/i })
      .first()
    if (await activateBtn.isVisible({ timeout: 3000 }).catch(() => false)) {
      await activateBtn.click()
      await page.waitForTimeout(500)
    }

    // The AllAnime chip is the single provider in this group.
    const allanimeChip = page.locator('button').filter({ hasText: 'AllAnime' }).first()
    await expect(allanimeChip).toBeVisible({ timeout: 10000 })
    await allanimeChip.click()

    // RawPlayer mounts under .raw-player.
    const rawPlayer = page.locator('.raw-player')
    await expect(rawPlayer).toBeVisible({ timeout: 10000 })

    // The toolbar subtitle picker is a <select>; it must at least have the
    // "Off" option (and ideally one or more language tracks once /subtitles
    // resolves; we don't fail if the catalog doesn't surface any).
    const subPicker = rawPlayer.locator('select')
    await expect(subPicker).toBeVisible({ timeout: 10000 })

    // "Other subs" button is rendered next to the picker.
    const otherSubsBtn = rawPlayer.getByRole('button', { name: /other subs|другие сабы|他の字幕/i }).first()
    await expect(otherSubsBtn).toBeVisible({ timeout: 5000 })
    await otherSubsBtn.click()

    // Modal renders inside <Teleport to="body"> — query the document.
    const modalTitle = page.getByRole('dialog').filter({ hasText: /other subtitles|другие субтитры|その他の字幕/i }).first()
    await expect(modalTitle).toBeVisible({ timeout: 5000 })
  })
})
