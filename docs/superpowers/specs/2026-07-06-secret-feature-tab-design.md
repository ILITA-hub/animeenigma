# Secret Feature Tab («Секретная фича») — Design

**Date:** 2026-07-06
**Source:** feedback report `2026-07-04T07-37-57_tNeymik_manual` — *"Сделать вкладку «секретная фича» которая будет открывать случайную скрытую (или легаси) фичу из списка"* — plus the owner's follow-up defining the pool: Showcase editor, Anidle (remove from header), Downloads (remove from header for browser view), Status page (remove from footer).

**Metrics:** UXΔ = +2 (Better) · CDI = 0.04 * 5 · MVQ = Sprite 90%/85%

## Goal

Replace four scattered nav entries with a single playful «Секретная фича» tab that opens a **random** eligible hidden/legacy feature on each click. The hidden features stay fully functional at their direct URLs — they lose only their nav placement.

## Owner-decision assumptions (owner was AFK; veto reverts cheaply)

1. **Placement = header** (desktop nav + mobile drawer, shared `navLinks` render sites). The TODO says «вкладка», which maps to the header tabs. Footer alternative rejected as less discoverable for a deliberate fun affordance.
2. **Re-roll on every click** (not per-session). Never repeats the last pick or the page the user is already on, when the pool has alternatives.
3. **Downloads scope:** "browser view" is read as *not an installed PWA*. In standalone PWA, Downloads keeps its normal nav link (header + MobileTabBar) and is **excluded** from the secret pool; in a browser tab it leaves the nav (header + MobileTabBar) and joins the pool.
4. **MobileTabBar** gets the same standalone gate as the header — a browser-view mobile user finds Downloads via the secret tab, an installed-PWA user keeps the tab.
5. **No roulette animation** in v1 (YAGNI; the TODO only asks to *open* a random feature). A reveal animation is a possible v2 flourish.
6. **design-prototyping sandbox skipped** — nav-link-level change, not a visual rework.

## Architecture

Pure frontend. One new module + edits to four existing components. No backend, no new routes, no build flags.

### New: `frontend/web/src/utils/secretFeatures.ts`

Registry of pool entries and the roll logic:

```ts
interface SecretFeature {
  key: 'anidle' | 'status' | 'downloads' | 'showcase-editor'
  to: RouteLocationRaw          // navigation target
  eligible: () => boolean       // evaluated at click time
}
```

| key | target | eligible when |
|---|---|---|
| `anidle` | `/anidle` | always |
| `status` | `/status` | always |
| `downloads` | `/downloads` | `offlineDownloadsEnabled && !standalone` |
| `showcase-editor` | `/profile?showcase=edit` | `authStore.isAuthenticated && profileWallVisible` |

`pickSecretFeature(currentPath: string): RouteLocationRaw` — filters to eligible entries, drops the entry matching `currentPath` and the module-remembered last pick (only while ≥1 candidate remains), picks uniformly at random, remembers it. Pool is never empty: `anidle` + `status` are unconditional.

Eligibility reads existing gates — `offlineDownloadsEnabled` (`@/offline/flag`), `useStandaloneDisplay()` (`@/pwa/standalone`), `useProfileWallVisible()` (`@/utils/profileWallGate`) — no new flag surface.

### `Navbar.vue`

- `navLinks` drops `/anidle` and `/downloads`; becomes a `computed` including `/downloads` only when `offlineDownloadsEnabled && isStandalone` (standalone is reactive).
- New «Секретная фича» **button** (not router-link — it has no active state; each click re-rolls) rendered after the nav links in both the desktop bar and the mobile drawer, styled as `nav-link-nt` / drawer row, with a `Sparkles` lucide icon (named import). Click: `router.push(pickSecretFeature(route.path))`; drawer variant also closes the drawer.

### `MobileTabBar.vue`

Downloads tab condition tightens from `offlineDownloadsEnabled` to `offlineDownloadsEnabled && isStandalone` (`isStandalone` already exists in the component).

### `App.vue` footer

`/status` router-link and its bullet separator removed. Route stays registered.

### `Profile.vue` + `router/index.ts`

Honors `?showcase=edit`: when the profile shown is the user's own and `profileWallVisible` is true, call the existing `openShowcaseEditor()` once, then `router.replace` to strip the query. Ineligible visitors get the plain profile (query silently stripped). The `/profile` → `/user/:publicId` own-profile redirect in `router/index.ts` today drops the query string — it is fixed to preserve `to.query` (a latent bug regardless of this feature).

### i18n

`nav.secretFeature` added to `en.json` («Secret feature»), `ru.json` («Секретная фича»), `ja.json` (「シークレット機能」). Three-locale parity gates redeploy.

## Error handling

- Empty pool impossible by construction (two unconditional entries).
- Duplicate-navigation warnings avoided by excluding the current path from the roll.
- `showcase-editor` never appears for users who can't open the editor, so the deep-link never lands on a dead affordance.

## Testing

- `secretFeatures.spec.ts` — eligibility matrix (anon / authed non-admin / admin × browser / standalone), no-immediate-repeat, current-path exclusion, uniform pick over a seeded-ish loop.
- Navbar has no existing unit spec and a mount needs ~12 module mocks for a template-level change — covered instead by vue-tsc, DS-lint, the real build, and e2e; all roll logic is tested in `secretFeatures.spec.ts`.
- `MobileTabBar.spec.ts` — downloads tab visible only when standalone (existing spec mocks `offlineDownloadsEnabled: true`; add standalone mock both ways).
- `Profile` spec — `?showcase=edit` opens the editor for an eligible owner; stripped for others.
- Locale parity specs pick up the new key automatically.

## Out of scope

- Reveal/roulette animation (v2 candidate).
- Adding more legacy features to the pool (registry makes it a one-line append).
- Backend involvement of any kind.
