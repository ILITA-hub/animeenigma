# Wire notification deep-link params into aePlayer

**Date:** 2026-06-18
**Status:** Design — approved for planning
**Origin:** `.planning/backlog/NOTIF-deadlink-player-translation-params.md` (Option 2)

## Problem

New-episode notification deep-links currently look like:

```
/anime/{id}/watch?player=kodik&episode=12&translation=1291
```

Only `episode` is honored by the frontend. `player` and `translation` are
**dead** — `grep route.query.player` across `src/` returns zero reads. The
notification lands the user on the new episode but silently drops the "open in
the same source + same team you were watching" intent.

Project direction is to retire all players except **aePlayer**
(`frontend/web/src/components/player/unified/UnifiedPlayer.vue`), so the fix is
to make the deep-link drive aePlayer's source selection rather than the legacy
per-player surfaces.

## Key facts (verified 2026-06-18)

1. **Notifications fire only for `kodik`/`animelib` combos.**
   `services/notifications/internal/job/hotcombos.go:59` filters
   `wh.translation_id != ''`. aePlayer/EN watchers persist `translation_id: ''`
   (all EN scrapers collapse to coarse `player='english'` via
   `frontend/web/src/composables/unifiedPlayer/comboMapping.ts`), so they drop
   out of detection entirely.
   - **Consequence:** the combos that DO fire carry `player` values
     (`kodik`, `animelib`) that are **already valid aePlayer provider ids**
     (see `providerRegistry.ts`). They map onto aePlayer's Source dropdown with
     zero translation.
   - **Out of scope:** the fact that aePlayer-EN watchers get no new-episode
     notifications at all is a separate backend gap. Not addressed here. Noted
     so a future change can revisit `comboMapping`/`hotcombos`.

2. **aePlayer initial-provider flow** (`UnifiedPlayer.vue`):
   `state.combo.value.provider` starts empty → `applyResolvedCombo()` restores
   saved audio/lang/team (NOT provider) → a watcher calls
   `pickSmartDefault(...).then(id => state.setProvider(id, ''))`. Every step is
   guarded by `if (state.combo.value.provider) return`, so **whoever sets the
   provider first wins** and the smart default won't override it.

3. **aePlayer keys teams by NAME (title), not id.** `comboToWatchCombo` sets
   `translation_id: ''`, `translation_title: combo.team`. So team preselection
   must use the team title, and the notification deep-link must carry the title
   (the payload already has `TranslationTitle`), not the numeric `translation_id`.

4. **`unifiedSelected`** (`Anime.vue:1378`) is a localStorage-backed ref that
   gates whether `UnifiedPlayer` mounts. Forcing it `true` mounts aePlayer.

## Final deep-link shape

```
/anime/{id}/watch?provider=<aePlayerProviderId>&team=<teamTitle>&episode=<n>
```

- `player` → renamed **`provider`** (value unchanged: `combo.Player`).
- `translation` (numeric id) → renamed **`team`**, carrying the team **title**
  (`TranslationTitle`), URL-encoded.
- `episode` → unchanged (already works).

**Clean break:** the frontend reads only `provider`/`team`. Legacy `player=`
links already in flight lose source-preselect (episode still works). No
backward-compat fallback.

## Changes

### 1. Backend — `services/notifications/internal/service/payload_builder.go`

- `BuildWatchURL(animeID, provider string, episode int, team string)`:
  emit `?provider=%s&team=%s&episode=%d`, URL-encoding `provider` and `team`
  (`url.QueryEscape`). Update the doc comment to the new pattern.
- Caller (line 61): pass `translationTitle` for the team arg (it is already in
  scope) instead of `combo.TranslationID`.
- Update `payload_builder_test.go` expectations.

### 2. Frontend — `frontend/web/src/views/Anime.vue`

- Add `queryProvider` / `queryTeam` computeds reading `route.query.provider` /
  `route.query.team`.
- When `queryProvider` is present on mount, set `unifiedSelected.value = true`
  so `UnifiedPlayer` mounts (the deep-link always opens aePlayer).
- Pass `:initial-provider="queryProvider"` and `:initial-team="queryTeam"` to
  `<UnifiedPlayer>`, mirroring the existing `:initial-episode`.

### 3. Frontend — `frontend/web/src/components/player/unified/UnifiedPlayer.vue`

- Add props `initialProvider?: string`, `initialTeam?: string`.
- In the provider-selection flow, **before** `pickSmartDefault` can run: if
  `initialProvider` resolves to a real provider row that is **active**
  (`rows.value.some(r => r.def.id === initialProvider && r.state === 'active')`),
  call `state.setProvider(initialProvider, '')` and, if `initialTeam`, set the
  team after audio/lang settle (mirroring `applyResolvedCombo`'s ordering —
  `setAudio`/`setLang` reset team to null, so `setTeam` comes last).
- If `initialProvider` is unmappable (e.g. coarse `'english'`) or its row is
  not active/available, do nothing → smart default runs as today.
- Set the same `providerAutoSelected = false` semantics as a manual pick (this
  is a user-intent pin, not an auto-selection) so availability-fallback logic
  treats it correctly. Confirm against existing `onSelectProvider` during
  planning.

### 4. Frontend — `frontend/web/src/stores/notifications.ts`

- `translateWatchUrl`: consume `provider`/`team` instead of `player`/`translation`.
- Fix the stale comment claiming `/anime/:id` "already consumes `?player=`,
  `?translation=`" — it does not (now it consumes `?provider=`, `?team=`).

### 5. Frontend — `frontend/web/src/router/index.ts`

- Update the `/anime/:id/watch` alias comment to reference `provider`/`team`.

## Testing

- **Go:** `payload_builder_test.go` — assert the new `?provider=…&team=…&episode=…`
  shape, including URL-encoding of a team title with spaces.
- **Vitest:** `notifications.ts` store spec — `translateWatchUrl` preserves the
  renamed params through the `/watch` → `/anime/:id` unwrap.
- **Vitest:** `UnifiedPlayer` — `initialProvider` for an active provider pins it
  over smart default; an unmappable/inactive `initialProvider` falls back to
  smart default; `initialTeam` is applied after audio/lang.
- **Anime.vue:** a `?provider=` query forces `unifiedSelected` and passes the
  props through (lightweight assertion or manual smoke).

## Non-goals

- Restoring new-episode notifications for aePlayer-EN (coarse `english`,
  empty `translation_id`) watchers — separate backend change.
- Persisting the granular aePlayer provider id into watch_history.
- Mapping coarse `'english'` → a specific EN scraper in the deep-link.
