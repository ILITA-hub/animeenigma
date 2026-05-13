import { test, expect, type Page } from '@playwright/test'

// Plan 1-6 (SOCIAL-06): Anime comments tab — four Playwright tests covering
// the four acceptance behaviors. Implementation notes:
//
//   - ANIME_ID: a stable seeded anime (Frieren) picked by sort_priority DESC
//     in the catalog. Override at runtime via E2E_ANIME_ID if the row is ever
//     deleted/replaced.
//   - Login pattern: `ui_audit_bot` (CLAUDE.md § "UI Audit Test User").
//     Password login via the page-context fetch sets the refresh cookie
//     correctly; the JWT + user are then injected into localStorage to match
//     auth.ts:42-43.
//   - Tests are independent — each opens its own context. CRUD test leaves
//     no permanent state (post + edit + delete in a single run); if a step
//     fails mid-flight, a stray comment may remain. Acceptable.

const ANIME_ID = process.env.E2E_ANIME_ID || 'f0b40660-6627-4a59-8dcf-7ec8596b3623'
const AUDIT_USERNAME = 'ui_audit_bot'
const AUDIT_PASSWORD = 'audit_bot_test_password_2026'

async function loginAsAuditBot(page: Page): Promise<void> {
  // Visit the app root to establish the origin for cookie scope before
  // issuing the login request in page context.
  await page.goto('/')
  const loginResult = await page.evaluate(
    async ({ username, password }) => {
      const resp = await fetch('/api/auth/login', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'include',
        body: JSON.stringify({ username, password }),
      })
      if (!resp.ok) {
        return { ok: false, status: resp.status, body: await resp.text() }
      }
      const json = await resp.json()
      const data = json?.data || json
      return { ok: true, data }
    },
    { username: AUDIT_USERNAME, password: AUDIT_PASSWORD }
  )
  if (!loginResult.ok) {
    throw new Error(
      `Login as ${AUDIT_USERNAME} failed (HTTP ${loginResult.status}): ${loginResult.body}`
    )
  }
  // Mirror frontend/web/src/stores/auth.ts:42-43 — token + user keys.
  await page.evaluate((data) => {
    if (data?.token) localStorage.setItem('token', data.token)
    if (data?.user) localStorage.setItem('user', JSON.stringify(data.user))
  }, loginResult.data)
}

test.describe('Anime comments tab', () => {
  test('deep-link to ?ugc=comments mounts Comments tab on first paint', async ({ page }) => {
    await page.goto(`/anime/${ANIME_ID}?ugc=comments`)
    // Wait for the tab strip to mount.
    await page.waitForSelector('[role="tab"]', { timeout: 15000 })

    // Comments tab must be selected on first paint (no flash of Reviews).
    const commentsTab = page.getByRole('tab', { name: /Comments|Комментарии|コメント/ })
    await expect(commentsTab).toBeVisible()
    await expect(commentsTab).toHaveAttribute('aria-selected', 'true')

    // Reviews tab must not be selected.
    const reviewsTab = page.getByRole('tab', { name: /^Reviews|Отзывы|レビュー/ })
    await expect(reviewsTab).toBeVisible()
    await expect(reviewsTab).toHaveAttribute('aria-selected', 'false')
  })

  test('URL persists across tab clicks via router.replace', async ({ page }) => {
    // Start from home so goBack() has a real previous entry.
    await page.goto('/')
    await page.waitForLoadState('domcontentloaded')

    await page.goto(`/anime/${ANIME_ID}`)
    await page.waitForSelector('[role="tab"]', { timeout: 15000 })

    // First paint with no ?ugc= → URL does not contain the query param yet.
    expect(page.url()).not.toMatch(/ugc=/)

    // Click Comments → URL gains ?ugc=comments.
    await page.getByRole('tab', { name: /Comments|Комментарии|コメント/ }).click()
    await expect(page).toHaveURL(/ugc=comments/)

    // Click Reviews → URL flips to ?ugc=reviews.
    await page.getByRole('tab', { name: /^Reviews|Отзывы|レビュー/ }).click()
    await expect(page).toHaveURL(/ugc=reviews/)

    // History check: goBack should leave the anime page entirely (because
    // we used router.replace, not router.push — back-button skips through
    // tab states straight to whatever was previously in history).
    await page.goBack()
    await expect(page).not.toHaveURL(new RegExp(`/anime/${ANIME_ID}`))
  })

  test('anon login prompt shown to logged-out users on Comments tab', async ({ browser }) => {
    // Fresh context — no auth state in storage.
    const context = await browser.newContext()
    const page = await context.newPage()
    try {
      await page.goto(`/anime/${ANIME_ID}?ugc=comments`)
      await page.waitForSelector('[role="tab"]', { timeout: 15000 })

      // The tabpanel must contain NO textarea (the write form is gated on auth).
      const panel = page.locator('[role="tabpanel"]')
      await expect(panel).toBeVisible()
      await expect(panel.locator('textarea')).toHaveCount(0)

      // The login prompt copy must be visible (locale-agnostic regex).
      const prompt = page.getByText(
        /Sign in to join the conversation|Войдите, чтобы присоединиться к обсуждению|会話に参加するにはログインしてください/
      )
      await expect(prompt).toBeVisible()
    } finally {
      await context.close()
    }
  })

  test('logged-in CRUD — post, edit, delete own comment', async ({ page }) => {
    await loginAsAuditBot(page)

    // Auto-confirm window.confirm() for the delete step.
    page.on('dialog', (dialog) => dialog.accept())

    await page.goto(`/anime/${ANIME_ID}?ugc=comments`)
    await page.waitForSelector('[role="tab"]', { timeout: 15000 })

    // Wait for the write-form textarea to render (proves auth gate passed).
    const writeTextarea = page.locator('[role="tabpanel"] textarea').first()
    await expect(writeTextarea).toBeVisible({ timeout: 10000 })

    // 1) POST a unique comment.
    const unique = `e2e comment ${Date.now()}`
    await writeTextarea.fill(unique)
    await page.getByRole('button', { name: /Post comment|Опубликовать комментарий|コメントを投稿/ }).click()

    // The new comment card must appear with the unique body text.
    const newCard = page.locator('article', { hasText: unique }).first()
    await expect(newCard).toBeVisible({ timeout: 15000 })

    // 2) EDIT — click the pencil button on the new comment card.
    await newCard.getByRole('button', { name: /Edit comment|Редактировать комментарий|コメントを編集/ }).click()
    const editTextarea = newCard.locator('textarea')
    await expect(editTextarea).toBeVisible({ timeout: 5000 })
    const editedBody = `${unique} edited`
    await editTextarea.fill(editedBody)
    await newCard.getByRole('button', { name: /Save edit|Сохранить правку|編集を保存/ }).click()

    // After save, edit mode collapses and the updated body is visible.
    await expect(newCard.locator('p', { hasText: editedBody })).toBeVisible({ timeout: 10000 })
    await expect(newCard.locator('textarea')).toHaveCount(0)

    // 3) DELETE — click trash; the window.confirm handler above auto-accepts.
    await newCard.getByRole('button', { name: /Delete comment|Удалить комментарий|コメントを削除/ }).click()

    // The card disappears from the list.
    await expect(page.locator('article', { hasText: editedBody })).toHaveCount(0, { timeout: 10000 })
  })
})
