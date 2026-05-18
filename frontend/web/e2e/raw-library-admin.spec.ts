// Workstream raw-jp v0.2 / Phase 05 (LIB-09) — RawLibrary admin view e2e.
//
// REQUIRES: a local docker compose stack with postgres reachable on the
// host. CI runners without `docker compose` access will test.skip().
//
// The `ui_audit_bot` user is seeded as role=user; this spec promotes it
// to admin in beforeAll and reverts to user in afterAll. Direct DB writes
// via `docker compose exec -T postgres psql` — same pattern as the
// Phase-3 / Phase-4 smoke scripts.

import { test, expect } from '@playwright/test'
import { execSync } from 'node:child_process'

const UI_AUDIT_USERNAME = 'ui_audit_bot'
const UI_AUDIT_PASSWORD = 'audit_bot_test_password_2026'

function setUserRole(role: 'admin' | 'user') {
  execSync(
    `docker compose -f docker/docker-compose.yml exec -T postgres psql -U postgres -d animeenigma -c "UPDATE users SET role='${role}' WHERE username='${UI_AUDIT_USERNAME}'"`,
    { stdio: 'pipe', cwd: process.cwd().replace(/\/frontend\/web$/, '') },
  )
}

async function loginAsUiAuditBot(page: import('@playwright/test').Page) {
  await page.goto('/')
  return await page.evaluate(
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
}

test.describe('RawLibrary admin view — workstream raw-jp Phase 05', () => {
  test.beforeAll(() => {
    try {
      setUserRole('admin')
    } catch (err) {
      // Docker not reachable from the runner — skip the whole suite.
      console.warn('Could not promote ui_audit_bot to admin via docker compose:', err)
      test.skip(true, 'docker compose unavailable; skipping admin-gated e2e')
    }
  })

  test.afterAll(() => {
    try {
      setUserRole('user')
    } catch {
      // Best-effort revert; if docker is unreachable here we already skipped.
    }
  })

  test('admin can render the view + search + see jobs panel', async ({ page }) => {
    const auth = await loginAsUiAuditBot(page)
    expect(auth.ok).toBe(true)

    await page.goto('/admin/raw-library')
    await page.waitForLoadState('networkidle')

    // Title visible.
    await expect(page.locator('h1').filter({ hasText: /Raw Library|Сырая библиотека|生ライブラリ/i })).toBeVisible({
      timeout: 10000,
    })

    // Stats strip — at least the diskFree label.
    await expect(
      page.locator('section[aria-label="library stats"]'),
    ).toBeVisible({ timeout: 5000 })

    // Search input accepts text + submit.
    const searchInput = page.locator('input[placeholder*="title"], input[placeholder*="Название"], input[placeholder*="タイトル"]').first()
    await searchInput.fill('test')

    const submitBtn = page.locator('button[type="submit"]').filter({ hasText: /Search|Найти|検索/ }).first()
    await submitBtn.click()

    // Either results render OR the empty-state copy shows. Don't fail on
    // upstream-down — Nyaa / AnimeTosho can be flaky.
    await page.waitForTimeout(2000) // allow debounce + fetch
    // No hard assertion here — just confirm we didn't crash.

    // Jobs panel header visible.
    await expect(
      page.locator('h2').filter({ hasText: /Active jobs|Активные задания|アクティブなジョブ/i }),
    ).toBeVisible({ timeout: 5000 })
  })

  test('non-admin is redirected away from /admin/raw-library', async ({ page }) => {
    // Temporarily revert to user role inside this single test.
    setUserRole('user')
    try {
      const auth = await loginAsUiAuditBot(page)
      expect(auth.ok).toBe(true)

      await page.goto('/admin/raw-library')
      // beforeEach guard fires synchronously — give it a moment.
      await page.waitForTimeout(800)

      // Should have bounced off /admin/raw-library.
      const url = page.url()
      expect(url).not.toContain('/admin/raw-library')

      // sessionStorage marker should be set.
      const marker = await page.evaluate(() => sessionStorage.getItem('admin_redirect_reason'))
      expect(marker).toBe('admin.errors.notAdmin')
    } finally {
      // Re-promote so afterAll's revert is the only state change. The
      // afterAll handler is the canonical revert; if this restore fails
      // we still leave ui_audit_bot as user, which matches afterAll's
      // intent.
      setUserRole('admin')
    }
  })
})
