import { test, expect } from '@playwright/test'

// Wave-0 scaffold for SOCIAL-06a..06d (`01-VALIDATION.md` rows 01-FE-Tabs-01
// through 01-FE-Tabs-04). Every test currently calls `test.skip(true, ...)`
// so the Playwright runner enumerates the four named tests but does not
// execute browser actions. Plan 06 fills the bodies.
//
// Plan-06 to-do (before flipping any skip):
//   1. Replace ANIME_ID = 'TBD' with a real seeded id from
//      scripts/seed-ui-audit-user.sh — the `ui_audit_bot` user is preloaded
//      with anime_list entries, pick a stable one.
//   2. Mirror the login pattern from frontend/web/e2e/anime.spec.ts (mock-token
//      localStorage shortcut) OR the refresh-cookie flow described in
//      CLAUDE.md § "UI Audit Test User" if the test needs real `/api/auth`
//      session state.
//   3. The describe block matches `-g "Anime comments"` so plan 06 can run
//      just this file via `bunx playwright test e2e/comments.spec.ts`.

// Replaced by plan 06 with the actual seeded anime id.
const ANIME_ID = 'TBD'

test.describe('Anime comments tab', () => {
  test('deep-link to ?ugc=comments mounts Comments tab on first paint', async () => {
    test.skip(true, 'Wave 0 scaffold — implementation in plan 06')
    // Plan 06: page.goto(`/anime/${ANIME_ID}?ugc=comments`) → expect the
    // Comments tab to be active on first paint (no flash of Reviews).
    expect(ANIME_ID).toBeTruthy()
  })

  test('URL persists across tab clicks via router.replace', async () => {
    test.skip(true, 'Wave 0 scaffold — implementation in plan 06')
    // Plan 06: click Reviews tab → expect URL ends ?ugc=reviews; click
    // Comments tab → expect URL ends ?ugc=comments; back button does NOT
    // step through tab states (router.replace, not push).
    expect(ANIME_ID).toBeTruthy()
  })

  test('anon login prompt shown to logged-out users on Comments tab', async () => {
    test.skip(true, 'Wave 0 scaffold — implementation in plan 06')
    // Plan 06: anonymous page.goto(`/anime/${ANIME_ID}?ugc=comments`) →
    // expect login CTA visible, no textarea rendered, comment list still
    // visible (public read).
    expect(ANIME_ID).toBeTruthy()
  })

  test('logged-in CRUD — post, edit, delete own comment', async () => {
    test.skip(true, 'Wave 0 scaffold — implementation in plan 06')
    // Plan 06: full lifecycle as ui_audit_bot — post a comment, see it
    // appear, click edit, save new body, click delete (window.confirm
    // dialog), verify it's gone from the list.
    expect(ANIME_ID).toBeTruthy()
  })
})
