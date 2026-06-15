# Smart Source Selection — Stage 1b Implementation Plan (watch-combo wiring)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the unified player remember and restore the source a returning user last watched for a given anime (provider category + audio + language + Kodik team), defer to that saved choice over the smart default, and show a gentle "the source you watched last time isn't available right now" message when the saved source is dead — by wiring the unified player into the existing `/preferences/resolve` watch-combo resolver.

**Architecture:** The reliability layer (Stage 1a) picks the *granular* provider; the existing watch-combo resolver picks the *coarse* category + audio + lang + team. Stage 1b connects the unified player to that resolver: it **persists** the unified `Combo` (mapped to the legacy `WatchCombo`) on playback, and on load **resolves** the saved/tiered combo and applies it as the default — falling back to the Stage 1a smart default when there's no saved combo or the saved source fails. The legacy resolver's tier logic is **unchanged**; we only add enum *values*.

**Tech Stack:** Go (player service), Vue 3 `<script setup>`, TypeScript, Vitest, Go testing.

**Scope boundary:** Stage 1b of `docs/superpowers/specs/2026-06-15-smart-source-selection-design.md`. Builds on Stage 1a (shipped: `pickSmartDefault`, `CURATED_TIER`, `isProviderAvailable`, `resolver.listTeams`, `teams` in SourcePanel). Stage 2 (telemetry, daily ranking, Redis same-day override, playback-time fallback) is a separate plan.

---

## Key decisions (please confirm at the plan-review gate)

1. **Coarse vs granular provider.** The legacy `WatchCombo.player` is coarse — `'english'` represents *all* EN scrapers. We do NOT extend it to hold granular scraper ids. Instead:
   - The resolver restores **category + audio + lang + team** (Kodik team via `translation_title`).
   - The exact EN scraper is restored from the **existing** `preferredScraperProvider` localStorage (`pref:scraper:{animeId}`, already in `useWatchPreferences`), else chosen by the Stage 1a smart default among EN providers.
2. **Additive enums only (no resolver-logic change).** Add players `raw`, `ae`; add language `ja`. These let unified combos validate and be stored. The resolver's tier matching is untouched — new values just form their own coarse buckets. (`'18anime'` maps to existing player `'hanime'`; revisit if you want a distinct `'18anime'` value.)
3. **Resolver owned by `UnifiedPlayer.vue`** (not threaded through `Anime.vue`). UnifiedPlayer already owns provider resolution and has `props.animeId`. It calls `useWatchPreferences(props.animeId)` directly. `tier1Combo` (the viewer-context prefetch shortcut) is left unset for now — `resolve()` still works via the backend round-trip; a later optimization can pass it as a prop.
4. **Restore wins over smart default**, coordinated by a `preferenceSettled` flag: the smart-default watcher defers until the resolve attempt completes; the resolved combo is applied first, and the smart default then fills any remaining gap (e.g. picks the specific EN scraper for a resolved `'english'`).

---

## File Structure

- **Modify** `services/player/internal/domain/preference.go` — additive `ValidPlayers` (`raw`,`ae`) + `ValidLanguages` (`ja`).
- **Modify** `services/player/internal/domain/preference_test.go` (create if absent) — assert the new values validate.
- **Modify** `frontend/web/src/types/preference.ts` — extend `WatchCombo.player` and `language` unions.
- **Create** `frontend/web/src/composables/unifiedPlayer/comboMapping.ts` — pure bidirectional mapping unified `Combo` ↔ legacy `WatchCombo`.
- **Create** `frontend/web/src/composables/unifiedPlayer/comboMapping.spec.ts` — tests.
- **Modify** `frontend/web/src/composables/unifiedPlayer/useWatchTracking.ts` — persist mapped combo on progress save + markWatched.
- **Modify** `frontend/web/src/composables/unifiedPlayer/useWatchTracking.spec.ts` — assert combo fields sent.
- **Modify** `frontend/web/src/components/player/unified/UnifiedPlayer.vue` — resolve + apply saved combo, `preferenceSettled` gate, dead-source toast + fallback.

---

### Task 1: Backend — additive enum values

**Files:**
- Modify: `services/player/internal/domain/preference.go`
- Test: `services/player/internal/domain/preference_test.go`

- [ ] **Step 1: Write the failing test**

Create/extend `services/player/internal/domain/preference_test.go`:

```go
package domain

import "testing"

func TestValidateCombo_NewEnumValues(t *testing.T) {
	cases := []struct {
		player, language, watchType string
		want                        bool
	}{
		{"ae", "en", "sub", true},
		{"ae", "ru", "dub", true},
		{"raw", "ja", "sub", true},
		{"kodik", "ru", "dub", true}, // existing still valid
		{"bogus", "en", "sub", false},
		{"ae", "klingon", "sub", false},
		{"", "", "", true}, // empty = no combo, valid
	}
	for _, c := range cases {
		if got := ValidateCombo(c.player, c.language, c.watchType); got != c.want {
			t.Errorf("ValidateCombo(%q,%q,%q)=%v want %v", c.player, c.language, c.watchType, got, c.want)
		}
	}
}
```

- [ ] **Step 2: Run test, verify it FAILS**

Run: `cd services/player && go test ./internal/domain/ -run TestValidateCombo_NewEnumValues`
Expected: FAIL (`ae`/`raw`/`ja` rejected).

- [ ] **Step 3: Add the enum values**

In `services/player/internal/domain/preference.go`, update the maps (keep existing entries):

```go
var ValidPlayers = map[string]bool{
	"kodik": true, "animelib": true,
	"raw": true, // AllAnime JP (Raw player)
	"ae":  true, // AnimeEnigma first-party library
}

var ValidLanguages = map[string]bool{
	"ru": true, "en": true,
	"ja": true, // JP audio (Raw player)
}
```

(Leave `ValidWatchTypes` and `ValidateCombo` unchanged. If `'hanime'` is not already present and you need 18anime persisted, add `"hanime": true` — confirm current contents while editing.)

- [ ] **Step 4: Run test, verify it PASSES**

Run: `cd services/player && go test ./internal/domain/ -run TestValidateCombo_NewEnumValues`
Expected: PASS.

- [ ] **Step 5: Commit (path-scoped)**

```bash
cd /data/animeenigma
git add services/player/internal/domain/preference.go services/player/internal/domain/preference_test.go
git commit -m "feat(player): allow raw/ae players + ja language in watch-combo validation

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```
Run `git show --stat HEAD` and confirm only those two files. Do NOT amend. Do NOT push (controller lands commits).

---

### Task 2: Frontend — pure combo mapping helper

**Files:**
- Modify: `frontend/web/src/types/preference.ts`
- Create: `frontend/web/src/composables/unifiedPlayer/comboMapping.ts`
- Test: `frontend/web/src/composables/unifiedPlayer/comboMapping.spec.ts`

- [ ] **Step 1: Extend the WatchCombo type**

In `frontend/web/src/types/preference.ts`, widen the unions (additive):

```ts
export interface WatchCombo {
  player: 'kodik' | 'animelib' | 'hanime' | 'english' | 'raw' | 'ae'
  language: 'ru' | 'en' | '18+' | 'ja'
  watch_type: 'dub' | 'sub'
  translation_id: string
  translation_title: string
  episodes_count?: number
}
```
(Leave `ResolvedCombo`/`ResolveResponse` as-is.)

- [ ] **Step 2: Write the failing test**

Create `frontend/web/src/composables/unifiedPlayer/comboMapping.spec.ts`:

```ts
import { describe, it, expect } from 'vitest'
import { providerToLegacyPlayer, comboToWatchCombo, watchComboToPartialCombo } from './comboMapping'

describe('providerToLegacyPlayer', () => {
  it('maps EN scraper ids to english', () => {
    for (const id of ['allanime', 'animepahe', 'gogoanime', 'nineanime', 'animefever', 'miruro']) {
      expect(providerToLegacyPlayer(id)).toBe('english')
    }
  })
  it('maps 1:1 providers', () => {
    expect(providerToLegacyPlayer('kodik')).toBe('kodik')
    expect(providerToLegacyPlayer('raw')).toBe('raw')
    expect(providerToLegacyPlayer('ae')).toBe('ae')
    expect(providerToLegacyPlayer('18anime')).toBe('hanime')
    expect(providerToLegacyPlayer('animelib')).toBe('animelib')
  })
  it('returns null for unknown', () => {
    expect(providerToLegacyPlayer('nope')).toBeNull()
  })
})

describe('comboToWatchCombo', () => {
  it('maps a unified combo to a legacy WatchCombo (team → translation_title)', () => {
    expect(comboToWatchCombo({ audio: 'dub', lang: 'ru', provider: 'kodik', server: '', team: 'AniLibria' }))
      .toEqual({ player: 'kodik', language: 'ru', watch_type: 'dub', translation_id: '', translation_title: 'AniLibria' })
  })
  it('maps ja-lang raw correctly', () => {
    expect(comboToWatchCombo({ audio: 'sub', lang: 'ja', provider: 'raw', server: '', team: null }))
      .toEqual({ player: 'raw', language: 'ja', watch_type: 'sub', translation_id: '', translation_title: '' })
  })
  it('returns null when provider has no legacy mapping', () => {
    expect(comboToWatchCombo({ audio: 'sub', lang: 'en', provider: 'nope', server: '', team: null })).toBeNull()
  })
})

describe('watchComboToPartialCombo', () => {
  it('maps a resolved combo back to unified audio/lang/team (provider left to caller)', () => {
    expect(watchComboToPartialCombo({ player: 'kodik', language: 'ru', watch_type: 'dub', translation_id: '1', translation_title: 'AniLibria', tier: 'per_anime', tier_number: 1 }))
      .toEqual({ audio: 'dub', lang: 'ru', team: 'AniLibria' })
  })
  it('maps english resolved combo to en lang, no team', () => {
    expect(watchComboToPartialCombo({ player: 'english', language: 'en', watch_type: 'sub', translation_id: '', translation_title: '', tier: 'community', tier_number: 3 }))
      .toEqual({ audio: 'sub', lang: 'en', team: null })
  })
})
```

- [ ] **Step 3: Run test, verify FAIL**

Run: `cd frontend/web && bunx vitest run src/composables/unifiedPlayer/comboMapping.spec.ts`
Expected: FAIL (cannot resolve `./comboMapping`).

- [ ] **Step 4: Implement the helper**

Create `frontend/web/src/composables/unifiedPlayer/comboMapping.ts`:

```ts
import type { WatchCombo, ResolvedCombo } from '@/types/preference'
import type { Combo, AudioKind, TrackLang } from '@/types/unifiedPlayer'

type LegacyPlayer = WatchCombo['player']

// EN scraper chain → coarse 'english'. Keep in sync with SCRAPER_IDS.
const EN_SCRAPER_IDS = new Set(['allanime', 'animepahe', 'gogoanime', 'nineanime', 'animefever', 'miruro'])

/** Map a granular unified provider id → coarse legacy WatchCombo.player (or null if unmappable). */
export function providerToLegacyPlayer(providerId: string): LegacyPlayer | null {
  if (EN_SCRAPER_IDS.has(providerId)) return 'english'
  switch (providerId) {
    case 'kodik': return 'kodik'
    case 'raw': return 'raw'
    case 'ae': return 'ae'
    case '18anime': return 'hanime'
    case 'animelib': return 'animelib'
    case 'hanime': return 'hanime'
    default: return null
  }
}

const langToLanguage: Record<TrackLang, WatchCombo['language']> = { en: 'en', ru: 'ru', ja: 'ja' }
const languageToLang: Partial<Record<WatchCombo['language'], TrackLang>> = { en: 'en', ru: 'ru', ja: 'ja', '18+': 'en' }

/** Map a unified Combo → legacy WatchCombo for persistence/resolve. Null if provider unmappable. */
export function comboToWatchCombo(combo: Combo): WatchCombo | null {
  const player = providerToLegacyPlayer(combo.provider)
  if (!player) return null
  return {
    player,
    language: langToLanguage[combo.lang],
    watch_type: combo.audio,
    translation_id: '',
    translation_title: combo.team ?? '',
  }
}

/** Map a resolved WatchCombo → the unified fields it can restore (audio/lang/team).
 *  The provider id is NOT derivable from a coarse player and is chosen by the caller
 *  (preferredScraperProvider for english, else smart default). */
export function watchComboToPartialCombo(rc: ResolvedCombo | WatchCombo): { audio: AudioKind; lang: TrackLang; team: string | null } {
  return {
    audio: rc.watch_type === 'dub' ? 'dub' : 'sub',
    lang: languageToLang[rc.language] ?? 'en',
    team: rc.translation_title ? rc.translation_title : null,
  }
}
```

- [ ] **Step 5: Run test, verify PASS**

Run: `cd frontend/web && bunx vitest run src/composables/unifiedPlayer/comboMapping.spec.ts` (expect all pass) and `bunx tsc --noEmit` (no new errors in the two files).

- [ ] **Step 6: Commit (path-scoped)**

```bash
cd /data/animeenigma
git add frontend/web/src/types/preference.ts \
        frontend/web/src/composables/unifiedPlayer/comboMapping.ts \
        frontend/web/src/composables/unifiedPlayer/comboMapping.spec.ts
git commit -m "feat(player): unified<->legacy watch-combo mapping helper

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```
`git show --stat HEAD` → only those 3 files. No amend. No push.

---

### Task 3: Persist the unified combo on playback

**Files:**
- Modify: `frontend/web/src/composables/unifiedPlayer/useWatchTracking.ts`
- Test: `frontend/web/src/composables/unifiedPlayer/useWatchTracking.spec.ts`

**Context:** `useWatchTracking` currently sends progress/markWatched WITHOUT combo fields (the comment at the top says so). We add an optional combo getter; when it yields a mappable `WatchCombo`, its fields ride along on `updateProgress` and `markEpisodeWatched`, so the backend's `UpsertAnimePreference` records the per-anime preference. Without this, there is nothing to restore in Task 4.

- [ ] **Step 1: Write the failing test**

Add to `useWatchTracking.spec.ts` a test that the progress payload includes combo fields when a combo getter is supplied. Mirror the existing spec's mock of `userApi`. Example:

```ts
it('includes mapped combo fields in updateProgress when a combo getter is provided', async () => {
  const updateProgress = vi.fn(async () => ({ data: {} }))
  // ...wire userApi mock so updateProgress is spied (follow the file's existing mock setup)...
  const tracking = useWatchTracking(
    () => 'anime-1',
    () => 3,
    {},
    () => ({ player: 'kodik', language: 'ru', watch_type: 'dub', translation_id: '', translation_title: 'AniLibria' }), // new 4th arg: combo getter
  )
  // simulate authenticated + a tick past SAVE_INTERVAL, then assert updateProgress called with player/language/watch_type/translation_title
  // (use the same technique the existing tests use to drive onTick + auth)
  // expect(updateProgress).toHaveBeenCalledWith(expect.objectContaining({ player: 'kodik', watch_type: 'dub', translation_title: 'AniLibria' }))
})
```
(Read the existing `useWatchTracking.spec.ts` first and match its auth-store + userApi mocking style exactly; fill in the driving code to match.)

- [ ] **Step 2: Run test, verify FAIL**

Run: `cd frontend/web && bunx vitest run src/composables/unifiedPlayer/useWatchTracking.spec.ts -t "combo fields"`
Expected: FAIL (4th arg ignored / fields absent).

- [ ] **Step 3: Add an optional combo getter and thread it into the payloads**

In `useWatchTracking.ts`:
- Add a 4th optional param `comboGetter?: () => WatchCombo | null` to `useWatchTracking(...)` (import `WatchCombo` from `@/types/preference`).
- Add a small helper inside:
  ```ts
  function comboFields(): Partial<WatchCombo> {
    const c = comboGetter?.()
    return c ? { player: c.player, language: c.language, watch_type: c.watch_type, translation_id: c.translation_id, translation_title: c.translation_title } : {}
  }
  ```
- In `saveServer(time)`, spread `...comboFields()` into the `userApi.updateProgress({...})` object.
- In `beaconSave()`, spread `...comboFields()` into the `postKeepalive('/users/progress', {...})` body.
- In `markWatched()`, pass the combo to `markEpisodeWatched`: replace `undefined` with `comboGetter?.() ?? undefined` (the api signature already accepts `combo?: Partial<WatchCombo>`).

(`updateProgress`'s request type must accept these fields — the backend `UpdateProgressRequest` already has them per Task 0 research. If the TS type for `updateProgress` is too narrow, widen it in `@/api/client.ts` to accept the optional combo fields.)

- [ ] **Step 4: Run test, verify PASS** + `bunx tsc --noEmit` clean for touched files.

- [ ] **Step 5: Commit (path-scoped)** — `useWatchTracking.ts` + `.spec.ts` (+ `api/client.ts` only if you widened the type). Message: `feat(player): unified player upserts watch-combo preference on playback`. No amend. No push.

---

### Task 4: Resolve + apply the saved combo on load (defer smart default)

**Files:**
- Modify: `frontend/web/src/components/player/unified/UnifiedPlayer.vue`

**Context:** Wire `useWatchPreferences` into UnifiedPlayer. On mount, after the provider rows are known, build an `available: WatchCombo[]` from the active providers, call `resolve(available)`, and apply the result (audio/lang/team + provider) BEFORE the smart default. Coordinate with the Stage 1a smart-default watcher via a `preferenceSettled` flag so the saved combo wins and the smart default only fills gaps.

⚠️ SHARED FILE: commit ONLY `UnifiedPlayer.vue`; never `--amend`; verify `git show --stat HEAD` after committing. Locate anchors by searching quoted code, not line numbers.

- [ ] **Step 1: Import the resolver composable + mapping + provider sets**

Add imports:
```ts
import { useWatchPreferences } from '@/composables/useWatchPreferences'
import { comboToWatchCombo, watchComboToPartialCombo, providerToLegacyPlayer } from '@/composables/unifiedPlayer/comboMapping'
import type { WatchCombo } from '@/types/preference'
```

- [ ] **Step 2: Add a `preferenceSettled` gate and instantiate the resolver**

Near the Stage 1a default block, add:
```ts
// Watch-combo restore (Stage 1b): defer the smart default until the saved/tiered
// preference has been resolved + applied, so a returning user's source wins.
const preferenceSettled = ref(false)
const { resolve: resolvePreference, resolvedCombo, preferredScraperProvider } = useWatchPreferences(props.animeId)
```

- [ ] **Step 3: Gate the Stage 1a smart-default watcher on `preferenceSettled`**

In the Stage 1a `watch(rows, …)` smart-default body, change the early guard so it waits for the preference attempt:
```ts
    if (state.combo.value.provider) return
    if (!preferenceSettled.value) return   // let the saved-combo restore go first
    void pickSmartDefault(/* …unchanged… */)
```

- [ ] **Step 4: Build `available` and resolve on first usable rows**

Add a one-shot watcher (fires once when rows first contain active providers):
```ts
const buildAvailable = (): WatchCombo[] => {
  const combos: WatchCombo[] = []
  const seen = new Set<string>()
  for (const r of rows.value) {
    if (r.state !== 'active') continue
    const player = providerToLegacyPlayer(r.def.id)
    if (!player) continue
    // One combo per (player, audio) the provider supports; Kodik teams are
    // resolved later by team match, so a representative combo per audio suffices.
    for (const audio of r.def.audios) {
      const key = `${player}:${audio}`
      if (seen.has(key)) continue
      seen.add(key)
      combos.push({
        player,
        language: (r.def.langs.includes(state.combo.value.lang) ? state.combo.value.lang : r.def.langs[0]) as WatchCombo['language'],
        watch_type: audio,
        translation_id: '',
        translation_title: '',
      })
    }
  }
  return combos
}

let resolveAttempted = false
watch(rows, () => {
  if (resolveAttempted) return
  const available = buildAvailable()
  if (available.length === 0) return            // wait for usable rows
  resolveAttempted = true
  resolvePreference(available).finally(() => {
    applyResolvedCombo()
    preferenceSettled.value = true              // release the smart default
  })
}, { immediate: true })
```

- [ ] **Step 5: Apply the resolved combo**

```ts
function applyResolvedCombo() {
  const rc = resolvedCombo.value
  if (!rc || state.combo.value.provider) return // nothing saved, or user already picked
  const { audio, lang, team } = watchComboToPartialCombo(rc)
  // Choose the granular provider for the resolved coarse player:
  let providerId: string | null = null
  if (rc.player === 'english') {
    // exact EN scraper from the existing per-anime localStorage, else let smart default pick
    providerId = preferredScraperProvider.value && rows.value.some(r => r.def.id === preferredScraperProvider.value && r.state === 'active')
      ? preferredScraperProvider.value
      : null
  } else {
    // 1:1 players: find the active row whose id maps back to this player
    const match = rows.value.find(r => r.state === 'active' && providerToLegacyPlayer(r.def.id) === rc.player)
    providerId = match?.def.id ?? null
  }
  state.setAudio(audio)
  state.setLang(lang)
  if (team) state.setTeam(team)
  if (providerId) state.setProvider(providerId, '')
  // If providerId is null (english with no saved scraper), the smart default
  // fires after preferenceSettled and picks the best EN provider; audio/lang/team stay applied.
}
```
(Note: `setAudio`/`setLang` reset `team` to null per `usePlayerState`, so call `setTeam` AFTER them — the order above is correct.)

- [ ] **Step 6: Type-check + tests**

`cd frontend/web && bunx tsc --noEmit 2>&1 | grep -i UnifiedPlayer` (none) and `bunx vitest run src/components/player/unified/` (all pass; update any test that asserted immediate smart-default selection to account for the `preferenceSettled` gate — the default now applies after a resolved tick).

- [ ] **Step 7: Commit (path-scoped, no amend)** — `UnifiedPlayer.vue`. Message: `feat(player): restore saved watch-combo as unified default (defer smart pick)`. `git show --stat HEAD` → only UnifiedPlayer.vue. No push.

---

### Task 5: Dead-source toast + fall-through

**Files:**
- Modify: `frontend/web/src/components/player/unified/UnifiedPlayer.vue`

**Context:** When the applied saved provider fails to resolve a stream (`NotAvailableError` / empty), show the gentle message and fall back to the smart default instead of leaving the user stuck.

- [ ] **Step 1: Import the toast**

```ts
import { useToast } from '@/composables/useToast'
// in setup:
const { push: pushToast } = useToast()
```

- [ ] **Step 2: Detect "the saved source failed" in `loadEpisodesAndStream`'s catch**

In the existing `catch` of `loadEpisodesAndStream` (where `sourceError.value` is set on `NotAvailableError`), add a one-shot fallback: if the failing provider was the one applied from the saved combo AND we haven't already fallen back, toast and re-pick via smart default.

Add a guard ref near the others: `let savedSourceFallbackDone = false`. In the `NotAvailableError` branch:
```ts
    if (isNotAvailable) {
      if (!savedSourceFallbackDone && providerWasFromSavedCombo) {
        savedSourceFallbackDone = true
        pushToast('Источник, который вы смотрели в прошлый раз, сейчас недоступен — переключаемся.', 'info', 5000)
        // clear provider and re-run smart default among the remaining active providers
        const next = await pickSmartDefault(
          rows.value.filter(r => r.def.id !== state.combo.value.provider),
          CURATED_TIER,
          { needsCheck: AE_NEEDS_CHECK, isAvailable: isProviderAvailable },
        )
        if (next) { state.setProvider(next, ''); return } // provider watcher re-resolves
      }
      sourceError.value = "This source isn't available yet"
    }
```
Set `providerWasFromSavedCombo` to `true` inside `applyResolvedCombo` when it calls `setProvider`, and reset it to `false` in `onSelectProvider` (a manual pick) and on `animeId` change.

- [ ] **Step 3: Type-check + tests** — `bunx tsc --noEmit` (UnifiedPlayer clean) + `bunx vitest run src/components/player/unified/`.

- [ ] **Step 4: i18n note** — the toast string is user-facing Russian; if the app routes player strings through i18n (`$t`), add the key to all three locales (`en`/`ru`/`ja`) per the i18n-lint gate. If player toasts are currently inline literals, match that pattern. Confirm by checking how `sourceError` strings are handled.

- [ ] **Step 5: Commit (path-scoped, no amend)** — `UnifiedPlayer.vue` (+ locale files if i18n). Message: `feat(player): toast + smart-default fallback when saved source is unavailable`. No push.

---

## Final verification

- [ ] `cd services/player && go test ./internal/...`
- [ ] `cd frontend/web && bunx vitest run src/composables/unifiedPlayer/ src/components/player/unified/`
- [ ] `cd frontend/web && bunx tsc --noEmit`
- [ ] `cd frontend/web && bunx eslint src/composables/unifiedPlayer/comboMapping.ts src/composables/unifiedPlayer/useWatchTracking.ts src/components/player/unified/UnifiedPlayer.vue`
- [ ] Owner-gated Chrome smoke (opt-in): returning user lands on last-watched source; killing that source shows the toast + auto-switches.
- [ ] `/animeenigma-after-update` (deploy player + web from a CLEAN main worktree — Stage 1a lesson — changelog, commit, push).

## Spec coverage (this plan)

| Spec section | Covered by |
|---|---|
| §1 Wire unified player into `/preferences/resolve` (+ additive enum) | Tasks 1, 2, 4 |
| §5 step 1 "saved combo wins; dead → toast → fall through" | Tasks 4 & 5 |
| §5 "defer to saved over smart default" | Task 4 (`preferenceSettled`) |
| Persistence prerequisite (was missing in unified player) | Task 3 |

## Assumptions to confirm before execution

- Persisting/restoring the **coarse** category + reusing `preferredScraperProvider` localStorage for the exact EN scraper is acceptable (we do NOT add a granular-provider column).
- Adding `ja` to `ValidLanguages` and `raw`/`ae` to `ValidPlayers` is the agreed additive change (no resolver-logic change).
- The resolver lives inside `UnifiedPlayer.vue` (not threaded via `Anime.vue`), and skipping `tier1Combo` (accepting one backend round-trip on first load) is fine for now.
- `'18anime'` maps to legacy player `'hanime'` (no distinct `'18anime'` enum value).

## Deferred to Stage 2

Telemetry (`player_resolve`/`player_stall` → ClickHouse), daily ranking job, Redis same-day override, ranking-aware default, playback-time (mid-watch) fallback suggestion, widened Grafana Playback dashboard.
