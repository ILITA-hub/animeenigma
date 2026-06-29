# Profile Showcase — per-user opt-in visibility

**Date:** 2026-06-29
**Status:** Approved (brainstorm) — ready for plan
**Feedback:** `2026-06-29T02-50-32_claude-code_feedback-2`
**Touches:** `services/auth`, `services/player`, `frontend/web`

## Problem

The profile showcase ("витрина", Steam-style wall) is dark-shipped behind a
single admin-only gate (`PROFILE_WALL_ADMIN_ONLY` gateway + FE
`VITE_PROFILE_WALL_ADMIN_ONLY`). Its visibility is all-or-nothing: when the
flag flips, **every** authenticated user suddenly gets a showcase tab —
including the vast majority who never built one (an empty/noise tab).

We want a per-user opt-in model so that, at release, the showcase only appears
for people who actually made one and chose to publish it.

## Desired behaviour

A new per-user **`enabled`** flag on the showcase, plus an entry button for
owners who have no visible showcase.

| Viewer | `enabled = true` (and non-empty) | otherwise |
| --- | --- | --- |
| **Visitor** | Showcase tab visible | Nothing — no tab, no button |
| **Owner (own profile)** | Showcase tab visible; edits + can disable inside | No tab. Button next to **Share**: `Add Showcase` (no content) / `Edit Showcase` (has content but disabled) |

**One rule:** *tab shown ⟺ showcase is `visible`* (for everyone). The only
owner-specific surface is the header button, shown when the showcase is not
`visible`.

Confirmed decisions (owner, 2026-06-29):

1. **Keep dark-shipped.** `PROFILE_WALL_ADMIN_ONLY` stays as-is. Everything
   below is built *underneath* the existing gate, ready for the eventual
   flag-flip. No gateway change. No public changelog.
2. **Hidden until enabled.** A fresh showcase is hidden; the editor's enable
   toggle defaults OFF; the user explicitly publishes. On save-while-hidden
   with content, show a non-blocking nudge to enable.
3. **Owner + disabled → button only, no tab.** When the owner's showcase is
   disabled they see only the header button (no tab); the tab returns only
   when `visible`.

## Request-minimal data flow (owner directive)

Do **not** fetch showcase blocks on profile load. The **existing** profile
request must carry a cheap visibility signal; blocks load lazily only when the
tab is opened.

### `showcase_state` on the public profile — zero extra requests

The profile page already calls `GET /auth/users/{publicId}` (auth service).
We add a single field:

```
showcase_state: "none" | "hidden" | "visible"
```

- `none` — no showcase row, or blocks empty.
- `hidden` — has content but `enabled = false`.
- `visible` — `enabled = true` (implies non-empty; see coerce rule).

Auth computes it via a **co-located read** of the player-owned
`profile_showcases` table. Auth, player, catalog, … all share the same
`animeenigma` Postgres (only `library`/`upscaler` have separate DBs), so this
is an in-DB read, **not** a cross-service HTTP call — it respects the
showcase's "no cross-service calls" principle (that rule targets the
save/content-resolution path). Defensive: any error or missing table/row →
`none` (self-heals once player has migrated).

### Lazy blocks

`Tabs.vue` renders only the active slot (`<slot :name="modelValue" />`), so
`ProfileShowcase` (which fetches `GET /users/{userId}/showcase`) mounts **only
when its tab is active**. Therefore:

- `visible` → showcase is the first, default-active tab → mounts → fetches
  blocks once. (Only profiles with a visible showcase ever fetch.)
- `hidden`/`none` → no tab → **no blocks fetch**. Owner sees the button.
- Owner clicks Add/Edit → tab is force-revealed + activated → mounts in edit
  mode → fetches blocks (lazy).

## Backend design

### Player (`services/player`)

- **Domain** (`domain/showcase.go`): add
  `Enabled bool \`gorm:"not null;default:false" json:"enabled"\`` to
  `ProfileShowcase`.
- **Coerce rule:** in `ShowcaseService.SaveShowcase`, persist
  `enabled = req.Enabled && len(blocks) > 0`. An empty showcase can never be
  `enabled` → keeps `visible ⟹ non-empty` invariant.
- **Repo** (`repo/showcase.go`): `Get` already returns the full
  `*ProfileShowcase` (so `Enabled` comes for free). `Upsert` signature gains
  `enabled bool`; use `clause.Assignments(map[string]any{"blocks":…,
  "enabled":…, "updated_at": time.Now()})` on conflict (explicit values, robust
  across the GORM default-omit gotcha + portable Postgres/SQLite).
- **Handler** (`handler/showcase.go`): `showcaseResponse` + `saveShowcaseRequest`
  gain `Enabled bool \`json:"enabled"\``. GET returns `{blocks, enabled}`; PUT
  reads `enabled`.
- **Service** `GetShowcase` keeps returning blocks; add a sibling that also
  surfaces `enabled` (or return `(blocks, enabled, err)`) so the handler can
  fill the response. Keep the corrupt-blocks-don't-500 behaviour.

#### One-shot migration backfill

AutoMigrate adds the `enabled` column (default `false`) to existing rows. A
guarded run-once backfill then sets `enabled` from current content, so
showcases that already exist (admin dark-ship test data, future migrations)
become `visible` rather than silently hidden.

Pattern mirrors `runSocialMigration` in `player-api/main.go`:

```go
// BEFORE the AutoMigrate block:
showcaseTableExisted := db.Migrator().HasTable(&domain.ProfileShowcase{})
hadEnabledCol := showcaseTableExisted &&
    db.Migrator().HasColumn(&domain.ProfileShowcase{}, "Enabled")
// ... AutoMigrate(...) ...
if showcaseTableExisted && !hadEnabledCol {
    backfillShowcaseEnabled(db.DB, log)
}
```

```go
// Runs exactly once — only on the first boot after the column is added to a
// pre-existing table. Fresh DBs (table absent) skip it: new rows default
// false, which is correct. Portable: the RHS is a boolean expression
// (Postgres bool / SQLite 0|1).
func backfillShowcaseEnabled(db *gorm.DB, log *logger.Logger) error {
    return db.Exec(
        `UPDATE profile_showcases
         SET enabled = (blocks IS NOT NULL AND blocks <> '[]' AND blocks <> '')`,
    ).Error
}
```

This can't re-enable a later-disabled showcase: it runs only on the single
boot that introduces the column.

### Auth (`services/auth`)

- **DTO** (`domain/user.go`): `PublicUser` gains
  `ShowcaseState string \`json:"showcase_state"\``. Add consts
  `ShowcaseStateNone/Hidden/Visible`.
- **Repo** (`repo/user.go`): new
  `GetShowcaseState(ctx, userID string) string` — raw read of the co-located
  table:
  ```sql
  SELECT enabled, blocks FROM profile_showcases WHERE user_id = ?
  ```
  Map: missing table/row or empty blocks → `none`; non-empty + `enabled` →
  `visible`; non-empty + not enabled → `hidden`. Any scan error → `none`.
  Documented as a read-only denormalization of a player-owned table.
- **Service** (`service/user.go`): `GetPublicProfile` and
  `GetPublicProfileByPublicID` set `pub.ShowcaseState =
  userRepo.GetShowcaseState(ctx, user.ID)` after `ToPublic()`.

`showcase_state` is exposed publicly (a visitor could learn a `hidden`
showcase exists — negligible; it reveals no content). The owner views their
own profile through the same public endpoint, so they read their state from
the same field.

## Frontend design

### Types + API (`types/showcase.ts`, `api/client.ts`)

- `export type ShowcaseState = 'none' | 'hidden' | 'visible'`.
- Profile/public-user FE type gains `showcase_state?: ShowcaseState`.
- `showcaseApi.getShowcase` return union gains `enabled: boolean`.
- `showcaseApi.saveShowcase(blocks, enabled)` → body `{ blocks, enabled }`.

### `Profile.vue`

- `effectiveShowcaseState` = local-after-save override ?? `profileUser.showcase_state` ?? `'none'`.
- `forceShowcaseEditing = ref(false)` (owner reveal).
- `showcaseTabVisible = profileWallVisible && (effectiveState === 'visible' || (isOwnProfile && forceShowcaseEditing))`.
- `tabs`: push `showcase` first when `showcaseTabVisible`.
- `activeTab` initial `'watchlist'`; a watcher promotes to `'showcase'` once
  state is `visible` **and** the user hasn't manually picked a tab
  (`tabTouched` flag on `update:modelValue`). Preserves the v3 "showcase is the
  default tab" feel without fighting the async profile fetch.
- **Header button** (next to Share, inside the existing `flex gap-2`):
  shown when `profileWallVisible && isOwnProfile && effectiveState !== 'visible'
  && !forceShowcaseEditing`. Label: `hidden → profile.editShowcase`,
  else `profile.addShowcase`. Click → `forceShowcaseEditing = true; activeTab = 'showcase'`.
- After save: update local state override + reset `forceShowcaseEditing` per
  the editor-closed contract below.

### `ProfileShowcase.vue` (controlled-ish)

- Props gain `autoEdit?: boolean` (start in edit mode for the owner-reveal path).
- `load()` reads `{ blocks, enabled }`; keep `enabled` in local state.
- `onSave(blocks, enabled)` → `saveShowcase(blocks, enabled)`; update local
  state; emit `change` + close editor.
- Emits:
  - `loaded: [count]` (existing; keep the visitor empty-bounce safety net).
  - `change: [{ enabled: boolean, count: number }]` — after a successful save,
    so Profile updates `showcase_state` without refetching the profile.
  - `editorClosed: []` — on cancel or save; Profile drops `forceShowcaseEditing`
    (tab visibility then falls back to effective state).
- The owner "Edit showcase" in-tab button still works for the `visible` path.

### `ShowcaseEditor.vue`

- Props gain `enabled: boolean`; `save` emits `[ShowcaseBlock[], boolean]`.
- Add the **enable/disable toggle** using the DS `Switch` primitive (profile is
  not `player/`, so reka primitives are fine). Toggle is **disabled when
  `local.length === 0`** (can't publish nothing).
- **Nudge:** when the editor is saved with `≥1 block` but `enabled === false`,
  show a non-blocking inline notice + toast ("Your showcase is hidden — enable
  it so others can see it") with an inline **Enable** action that flips the
  toggle on and re-saves.

### i18n (en / ru / ja — full parity)

- `profile.addShowcase`, `profile.editShowcase`
- `showcase.visibility` (label), `showcase.visibleToggle` (toggle label /
  "Show on my profile"), `showcase.hiddenNotice` (nudge), `showcase.enableNow`,
  `showcase.disabledEmptyHint` (why the toggle is greyed at 0 blocks)

## Testing

- **auth Go:** `GetShowcaseState` → none (missing table / missing row / empty),
  hidden (content + disabled), visible (content + enabled); `ToPublic` +
  service wires `showcase_state`.
- **player Go:** save/get round-trips `enabled`; coerce (enabled + empty → false);
  `backfillShowcaseEnabled` sets enabled from content and is a no-op on rerun /
  fresh DB.
- **FE (vitest):** `Profile.showcase.spec.ts` — tab/button visibility per
  `showcase_state` × owner/visitor; `ShowcaseEditor.spec.ts` — toggle present,
  disabled at 0 blocks, `save` emits `enabled`, nudge on hidden-save;
  `ProfileShowcase.spec.ts` — fetch reads `enabled`, `autoEdit` opens editor,
  emits `change`/`editorClosed`.
- Gates: DS-lint, i18n en/ru/ja parity, real `bun run build` / `vue-tsc`,
  `go test ./...` (auth + player).

## Out of scope / deferred

- **No gateway change** — dark-ship stays; admins exercise the new model.
- **Release hardening (documented, not built now):** owner-gate the player
  `GET /users/{userId}/showcase` so a non-owner can't fetch a `hidden`
  showcase's blocks directly via the API. Moot while the gateway admin-gates
  the route; **required before the public flag-flip** (add optional-auth on the
  GET route → return `{blocks: [], enabled: false}` for non-owner + disabled).
- Gacha coupling untouched (`card_collection` still renders the viewer's cards).

## Metrics

- **UXΔ** = +2 (Better) — opt-in showcase removes the empty-tab noise the
  all-or-nothing flag would inflict at release; owners get a clear entry point.
- **CDI** = 0.03 * 13 — spread across two Go services + several FE files, but
  each change is additive and bounded; no shared-contract churn beyond the two
  new fields.
- **MVQ** = Griffin 85%/80% — composes an existing feature with a clean
  visibility seam; resists slop because the one-rule model collapses the
  owner/visitor matrix.
