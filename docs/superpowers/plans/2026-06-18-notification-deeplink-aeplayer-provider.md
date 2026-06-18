# Wire Notification Deep-Link Params into aePlayer — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make new-episode notification deep-links open aePlayer with the source provider (and team) preselected, by renaming the URL params `player`→`provider` and `translation`→`team` (carrying the team title) and honoring them in `UnifiedPlayer.vue`.

**Architecture:** The notifications backend emits `?provider=<id>&team=<title>&episode=<n>`. `Anime.vue` routes any `?provider=` link into aePlayer (forces `unifiedSelected`) and passes `initialProvider`/`initialTeam` props. `UnifiedPlayer.vue` pins that provider/team **before** its smart-default picker runs, guarded so an unmappable or inactive provider falls back to the smart default.

**Tech Stack:** Go (notifications service), Vue 3 + TypeScript (frontend), Vitest, `go test`.

## Global Constraints

- **Spec:** `docs/superpowers/specs/2026-06-18-notification-deeplink-aeplayer-provider-design.md`.
- **Frontend tooling:** use `bun`/`bunx` (NOT npm/pnpm). Type-check with `bunx vue-tsc --noEmit` (catches `.vue` template errors that `tsc` misses).
- **Commits:** path-scoped (`git commit <pathspec>`), never bare `git commit`/`git add -A` (shared tree, concurrent agents). Run `git show --stat HEAD` after each commit. Always `git push` after committing.
- **Co-authors on every commit:**
  ```
  Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
  Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
  Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
  ```
- **Clean break:** frontend reads only `provider`/`team`; no legacy `player=` fallback.
- **Team values use the title** (e.g. `AniLibria`), URL-encoded, never the numeric `translation_id`.
- **Out of scope:** restoring notifications for aePlayer-EN watchers (coarse `english`, empty `translation_id`); persisting granular provider ids into watch_history.

---

### Task 1: Backend — `BuildWatchURL` emits `provider`/`team` (title)

**Files:**
- Modify: `services/notifications/internal/service/payload_builder.go` (`BuildWatchURL` at lines 71-79; caller at line 61)
- Test: `services/notifications/internal/service/payload_builder_test.go` (create)

**Interfaces:**
- Produces: `BuildWatchURL(animeID, provider string, episode int, team string) string` → `/anime/{id}/watch?provider={provider}&team={team}&episode={n}` with `provider` and `team` URL-query-escaped.

- [ ] **Step 1: Write the failing test**

Create `services/notifications/internal/service/payload_builder_test.go`:

```go
package service

import "testing"

func TestBuildWatchURL_ProviderTeamEpisode(t *testing.T) {
	got := BuildWatchURL("abc-123", "kodik", 12, "AniLibria")
	want := "/anime/abc-123/watch?provider=kodik&team=AniLibria&episode=12"
	if got != want {
		t.Fatalf("BuildWatchURL = %q, want %q", got, want)
	}
}

func TestBuildWatchURL_EncodesTeamTitleWithSpace(t *testing.T) {
	got := BuildWatchURL("abc-123", "kodik", 3, "Studio Band")
	want := "/anime/abc-123/watch?provider=kodik&team=Studio+Band&episode=3"
	if got != want {
		t.Fatalf("BuildWatchURL = %q, want %q", got, want)
	}
}

func TestBuildWatchURL_EmptyTeam(t *testing.T) {
	got := BuildWatchURL("abc-123", "animelib", 5, "")
	want := "/anime/abc-123/watch?provider=animelib&team=&episode=5"
	if got != want {
		t.Fatalf("BuildWatchURL = %q, want %q", got, want)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/notifications && go test ./internal/service/ -run TestBuildWatchURL -v`
Expected: FAIL — current output uses `?player=...&episode=...&translation=...`.

- [ ] **Step 3: Update `BuildWatchURL` and its caller**

In `services/notifications/internal/service/payload_builder.go`, add `"net/url"` to the import block, then replace the function (lines 71-79):

```go
// BuildWatchURL formats the new-episode deep-link consumed by the frontend
// NotificationCard / store. aePlayer reads `provider` (its source id) and
// `team` (the team TITLE, e.g. a Kodik translation title) to preselect the
// source on mount; `episode` lands the user on the new episode:
//
//	/anime/{anime_id}/watch?provider={provider}&team={team}&episode={ep}
func BuildWatchURL(animeID, provider string, episode int, team string) string {
	return fmt.Sprintf("/anime/%s/watch?provider=%s&team=%s&episode=%d",
		animeID, url.QueryEscape(provider), url.QueryEscape(team), episode)
}
```

In the same file, change the caller (line 61) to pass the team **title** instead of the numeric id:

```go
		WatchURL:               BuildWatchURL(anime.ID, combo.Player, maxWatched+1, translationTitle),
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd services/notifications && go test ./internal/service/ -run TestBuildWatchURL -v`
Expected: PASS (all three).

- [ ] **Step 5: Run the full notifications suite (no regressions)**

Run: `cd services/notifications && go test ./... 2>&1 | tail -20`
Expected: all packages `ok` / no FAIL.

- [ ] **Step 6: Commit**

```bash
git commit services/notifications/internal/service/payload_builder.go services/notifications/internal/service/payload_builder_test.go \
  -m "feat(notifications): deep-link emits provider/team(title) params for aePlayer

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
git show --stat HEAD
git push
```

---

### Task 2: Frontend — pure `pickInitialProvider` helper

**Files:**
- Create: `frontend/web/src/composables/unifiedPlayer/initialProvider.ts`
- Test: `frontend/web/src/composables/unifiedPlayer/initialProvider.spec.ts`

**Interfaces:**
- Produces: `pickInitialProvider(initialProvider: string | undefined | null, rows: ProviderRow[]): string | null` — returns the id to pin iff it names a row whose `state === 'active'`, else `null`.

This mirrors the existing `smartDefault.ts` / `rankedProviderIds.ts` pure-helper-plus-spec pattern so the subtle "only honor a real, active provider" rule is unit-tested without mounting the giant `UnifiedPlayer.vue`.

- [ ] **Step 1: Write the failing test**

Create `frontend/web/src/composables/unifiedPlayer/initialProvider.spec.ts`:

```ts
import { describe, expect, it } from 'vitest'
import type { ProviderRow } from '@/types/unifiedPlayer'
import { pickInitialProvider } from './initialProvider'

const row = (id: string, state: ProviderRow['state']): ProviderRow =>
  ({ def: { id, name: id, hue: '#000', group: 'en', audios: ['sub'], langs: ['en'], content: ['common'], scraper: true }, state })

const rows: ProviderRow[] = [
  row('kodik', 'active'),
  row('gogoanime', 'active'),
  row('animelib', 'inactive'),
]

describe('pickInitialProvider', () => {
  it('returns the id when it names an active row', () => {
    expect(pickInitialProvider('kodik', rows)).toBe('kodik')
  })

  it('returns null for an inactive provider (falls back to smart default)', () => {
    expect(pickInitialProvider('animelib', rows)).toBeNull()
  })

  it('returns null for a coarse/unknown value like "english"', () => {
    expect(pickInitialProvider('english', rows)).toBeNull()
  })

  it('returns null when no initial provider is given', () => {
    expect(pickInitialProvider(undefined, rows)).toBeNull()
    expect(pickInitialProvider('', rows)).toBeNull()
    expect(pickInitialProvider(null, rows)).toBeNull()
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd frontend/web && bunx vitest run src/composables/unifiedPlayer/initialProvider.spec.ts`
Expected: FAIL — `pickInitialProvider` not found.

- [ ] **Step 3: Write the helper**

Create `frontend/web/src/composables/unifiedPlayer/initialProvider.ts`:

```ts
import type { ProviderRow } from '@/types/unifiedPlayer'

/**
 * Decide whether a notification deep-link's `?provider=` value should pin the
 * aePlayer source. Honored ONLY when it names a real provider row that is
 * currently `active` — coarse/legacy values (e.g. 'english') and
 * unavailable/inactive providers return null so the smart default picks.
 *
 * Pure + sync so it is unit-testable without mounting UnifiedPlayer.vue.
 */
export function pickInitialProvider(
  initialProvider: string | undefined | null,
  rows: ProviderRow[],
): string | null {
  if (!initialProvider) return null
  return rows.some((r) => r.def.id === initialProvider && r.state === 'active')
    ? initialProvider
    : null
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd frontend/web && bunx vitest run src/composables/unifiedPlayer/initialProvider.spec.ts`
Expected: PASS (4 tests).

- [ ] **Step 5: Commit**

```bash
git commit frontend/web/src/composables/unifiedPlayer/initialProvider.ts frontend/web/src/composables/unifiedPlayer/initialProvider.spec.ts \
  -m "feat(player): pickInitialProvider helper — pin only real active providers

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
git show --stat HEAD
git push
```

---

### Task 3: Frontend — `UnifiedPlayer.vue` consumes `initialProvider`/`initialTeam`

**Files:**
- Modify: `frontend/web/src/components/player/unified/UnifiedPlayer.vue` (props at 369-377; import near 342-349; `applyResolvedCombo` at 467-479; resolve `.finally()` at 512-515)

**Interfaces:**
- Consumes: `pickInitialProvider` (Task 2); `state.setProvider(id, '')`, `state.setTeam(team)` (existing).
- Produces: props `initialProvider?: string`, `initialTeam?: string` (consumed by Task 4).

- [ ] **Step 1: Add the import**

Add to the import group near line 342 (alongside the other `./` unified imports):

```ts
import { pickInitialProvider } from '@/composables/unifiedPlayer/initialProvider'
```

- [ ] **Step 2: Add the props**

Extend `defineProps` (lines 369-377) — add the two props after `initialEpisode`:

```ts
const props = defineProps<{
  animeId: string
  anime: { title: string; ep: number; eps: number; still?: string }
  theater: boolean
  isHentai?: boolean
  initialEpisode?: number
  /** Notification deep-link: aePlayer provider id to pin on mount (e.g. 'kodik').
   *  Ignored unless it names a real, active provider row. */
  initialProvider?: string
  /** Notification deep-link: team TITLE to preselect alongside initialProvider. */
  initialTeam?: string
  /** Shikimori id (= MAL id) for AniSkip skip-times. Absent ⇒ no skip UI. */
  malId?: string | number
}>()
```

- [ ] **Step 3: Add `applyInitialProvider` after `applyResolvedCombo`**

Immediately after the `applyResolvedCombo` function (after line 479), add:

```ts
// Notification deep-link override: pin the provider the user was watching
// BEFORE the smart default runs. Honored only for a real, active provider
// row (coarse/legacy/unavailable values fall through to the smart default).
// Runs after applyResolvedCombo so initialTeam wins over the saved-pref team,
// and after setAudio/setLang (which reset team → null) so the team sticks.
function applyInitialProvider() {
  if (state.combo.value.provider) return
  const id = pickInitialProvider(props.initialProvider, rows.value)
  if (!id) return
  providerAutoSelected = false // user-intent pin, not an auto-selection
  state.setProvider(id, '')
  if (props.initialTeam) state.setTeam(props.initialTeam)
}
```

- [ ] **Step 4: Call it in the preference-resolve `.finally()`**

Change the resolve block (lines 512-515) from:

```ts
  resolvePreference(available).finally(() => {
    applyResolvedCombo()
    preferenceSettled.value = true
  })
```

to:

```ts
  resolvePreference(available).finally(() => {
    applyResolvedCombo()
    applyInitialProvider()
    preferenceSettled.value = true
  })
```

- [ ] **Step 5: Type-check**

Run: `cd frontend/web && bunx vue-tsc --noEmit 2>&1 | grep -i "UnifiedPlayer\|initialProvider" || echo "no UnifiedPlayer type errors"`
Expected: `no UnifiedPlayer type errors` (full `vue-tsc` may surface unrelated pre-existing errors elsewhere — only this file must be clean).

- [ ] **Step 6: Run the unified player tests (no regressions)**

Run: `cd frontend/web && bunx vitest run src/components/player/unified/ src/composables/unifiedPlayer/`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git commit frontend/web/src/components/player/unified/UnifiedPlayer.vue \
  -m "feat(player): aePlayer honors initialProvider/initialTeam over smart default

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
git show --stat HEAD
git push
```

---

### Task 4: Frontend — `Anime.vue` routes the deep-link to aePlayer + passes props

**Files:**
- Modify: `frontend/web/src/views/Anime.vue` (query computeds near 1469-1487; `unifiedSelected` at 1378; `<UnifiedPlayer>` at 656-665)

**Interfaces:**
- Consumes: `route` (existing), `unifiedSelected` ref (line 1378), `UnifiedPlayer` props `initial-provider`/`initial-team` (Task 3).

- [ ] **Step 1: Add `queryProvider` / `queryTeam` computeds**

Immediately after the `queryEpisode` computed (after line 1477), add:

```ts
// Notification deep-link — `?provider=` is an aePlayer source id, `?team=` is a
// team TITLE. Both are HINTS preselected on the unified player; see Task 3.
const queryProvider = computed<string | undefined>(() => {
  const v = route.query.provider
  const s = Array.isArray(v) ? v[0] : v
  return typeof s === 'string' && s !== '' ? s : undefined
})
const queryTeam = computed<string | undefined>(() => {
  const v = route.query.team
  const s = Array.isArray(v) ? v[0] : v
  return typeof s === 'string' && s !== '' ? s : undefined
})

// A `?provider=` deep-link always opens aePlayer (the param speaks aePlayer's
// source vocabulary). Set the ref directly; its localStorage watcher persists
// the switch, which matches the retire-all-but-aePlayer direction.
if (queryProvider.value) unifiedSelected.value = true
```

- [ ] **Step 2: Pass the props to `<UnifiedPlayer>`**

In the `<UnifiedPlayer>` element (lines 656-665), add the two bindings after `:initial-episode`:

```html
        <UnifiedPlayer
          v-if="unifiedSelected && unifiedPlayerEnabled"
          :anime-id="anime.id"
          :anime="{ title: anime.title, ep: (anime.episodesAired || 1), eps: (anime.totalEpisodes || anime.episodesAired || 1), still: anime.coverImage }"
          :theater="theaterMode"
          :is-hentai="isHentai"
          :initial-episode="resumeStartEpisode"
          :initial-provider="queryProvider"
          :initial-team="queryTeam"
          :mal-id="anime.shikimoriId"
          @toggle-theater="setTheater(!theaterMode)"
        />
```

- [ ] **Step 3: Type-check the view**

Run: `cd frontend/web && bunx vue-tsc --noEmit 2>&1 | grep -i "Anime.vue" || echo "no Anime.vue type errors"`
Expected: `no Anime.vue type errors`.

- [ ] **Step 4: Commit**

```bash
git commit frontend/web/src/views/Anime.vue \
  -m "feat(anime): route ?provider= deep-link into aePlayer with initial source/team

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
git show --stat HEAD
git push
```

---

### Task 5: Frontend — fix stale comments + lock `translateWatchUrl` param preservation

**Files:**
- Modify: `frontend/web/src/stores/notifications.ts` (doc comment at 103-106)
- Modify: `frontend/web/src/router/index.ts` (alias comment near 36-42)
- Test: `frontend/web/src/stores/notifications.spec.ts` (append a `translateWatchUrl` describe block)

`translateWatchUrl` already preserves all query params generically (no logic change needed) — this task locks that with tests for the renamed params and corrects the two stale comments that still say `player`/`translation`.

- [ ] **Step 1: Write the failing test**

Append to `frontend/web/src/stores/notifications.spec.ts`. Add `translateWatchUrl` to the existing import on line 4:

```ts
import { useNotificationsStore, translateWatchUrl } from './notifications'
```

Then append at the end of the file:

```ts
describe('translateWatchUrl — notification deep-link params', () => {
  it('unwraps /watch and preserves provider/team/episode', () => {
    expect(
      translateWatchUrl('/anime/abc/watch?provider=kodik&team=AniLibria&episode=12'),
    ).toEqual({
      path: '/anime/abc',
      query: { provider: 'kodik', team: 'AniLibria', episode: '12' },
    })
  })

  it('decodes an encoded team title with a space', () => {
    expect(
      translateWatchUrl('/anime/abc/watch?provider=kodik&team=Studio+Band&episode=3'),
    ).toEqual({
      path: '/anime/abc',
      query: { provider: 'kodik', team: 'Studio Band', episode: '3' },
    })
  })
})
```

- [ ] **Step 2: Run test to verify it passes (behavior already correct)**

Run: `cd frontend/web && bunx vitest run src/stores/notifications.spec.ts`
Expected: PASS — `translateWatchUrl` already forwards arbitrary query params, so these lock current behavior. (If the import line is wrong it will FAIL to resolve — fix the import, not the function.)

- [ ] **Step 3: Fix the stale doc comment in `notifications.ts`**

Replace lines 103-106 (the `Backend ships …` block) with:

```ts
 * Backend ships `/anime/{id}/watch?provider=X&team=Y&episode=N`.
 * The live frontend route is `/anime/:id`, which consumes `?episode=N`
 * (lands the user on the episode) and `?provider=`/`?team=` (aePlayer
 * preselects that source + team on mount — see Anime.vue queryProvider).
 * This helper unwraps the `/watch` suffix and preserves all query params.
```

- [ ] **Step 4: Fix the router alias comment in `index.ts`**

In the `/anime/:id/watch` alias comment (near lines 36-42), update the param reference. Replace the first sentence fragment so it reads:

```ts
    // route is /anime/:id (which consumes the same query params:
    // provider/team/episode). This alias redirects without 404'ing for any
    // code path that pushes the raw watch_url (e.g. future email/Telegram
    // deep links). The store's translateWatchUrl helper produces the
    // canonical /anime/:id?... shape directly; this alias is belt + suspenders.
```

- [ ] **Step 5: Re-run the store spec**

Run: `cd frontend/web && bunx vitest run src/stores/notifications.spec.ts`
Expected: PASS (existing toast tests + 2 new translateWatchUrl tests).

- [ ] **Step 6: Commit**

```bash
git commit frontend/web/src/stores/notifications.ts frontend/web/src/stores/notifications.spec.ts frontend/web/src/router/index.ts \
  -m "refactor(notifications): provider/team deep-link comments + translateWatchUrl tests

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
git show --stat HEAD
git push
```

---

### Task 6: Full verification + deploy

**Files:** none (verification only).

- [ ] **Step 1: Backend suite**

Run: `cd services/notifications && go test ./... 2>&1 | tail -20`
Expected: no FAIL.

- [ ] **Step 2: Frontend touched specs + type-check**

Run:
```bash
cd frontend/web && bunx vitest run \
  src/composables/unifiedPlayer/initialProvider.spec.ts \
  src/components/player/unified/ \
  src/stores/notifications.spec.ts
bunx vue-tsc --noEmit 2>&1 | grep -iE "Anime.vue|UnifiedPlayer|initialProvider|notifications" || echo "no type errors in touched files"
```
Expected: vitest PASS; `no type errors in touched files`.

- [ ] **Step 3: Run the after-update skill**

Invoke `/animeenigma-after-update` to lint, redeploy `notifications` + `web`, health-check, write the Russian Trump-mode changelog entry, and push. (Web deploy: build from a CLEAN `origin/main` worktree per the deploy-from-clean-worktree rule; FE deploy gate is `vue-tsc`, and remember the DS-lint allowlist path-integrity pre-pass — none of these tasks rename `.vue` files, so allowlists are unaffected.)

- [ ] **Step 4 (optional): Manual smoke**

With a real new-episode notification (kodik/animelib combo), click it and confirm: aePlayer mounts, the named provider is selected, the team title is preselected, and the URL bar shows `?provider=…&team=…&episode=…`. (Per Chrome-smoke-opt-in: only if the owner asks.)

---

## Notes / known limitations

- **Same-anime consecutive notifications without remount:** `UnifiedPlayer` has no `:key`, and `applyInitialProvider` is a one-shot in the resolve `.finally()`. Clicking a second notification for the *same* anime (different provider) while the player is already mounted won't re-pin. Episode re-mount is already handled by the existing `queryEpisode` watcher; provider re-pin in that narrow case is intentionally not handled.
- **Team title matching is best-effort:** `initialTeam` sets `combo.team` before `resolver.listTeams()` resolves. If the title matches a loaded team chip it highlights; if not, the stream still resolves with the requested team string. No hard dependency on the chip list.
- **Coarse `english` / aePlayer-EN watchers:** these never reach a notification today (`hotcombos.go:59` filters empty `translation_id`), so `provider=english` never appears; if it did, `pickInitialProvider` returns null → smart default. Separate backend change tracked in the spec's Non-goals.
