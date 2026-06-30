# Episode-Selection Simplification — Design

- **Date:** 2026-06-30
- **Status:** Approved (brainstorming) → awaiting spec review → writing-plans
- **Branch / worktree:** `feat/episode-selection-simplify` @ `/data/ae-episode-selection`
- **Scope decision (locked):** Full untangle. UX may be simplified (collapse banner states, unify anon/authed).
- **Open decisions (locked):** Keep `loadedEpisodes` signal. Enable anonymous parity.

---

## 1. Problem

### 1.1 The bug (symptom)

Opening an anime mounts the player on the **latest aired episode** (e.g. ep 12) instead of episode 1 — observed even for an **authenticated, first-time** viewer.

Two compounding causes:

1. **Race / one-shot read.** `AePlayer` renders the moment `anime` resolves (`Anime.vue:402`, not gated on `playerActivated`) and reads its starting episode **once** in `onMounted` (`AePlayer.vue:2518 → initSelectedEpisode`, line 1279). But `resume.init()` (server `watch_progress`) resolves **later**, after the viewer-context fetch (`Anime.vue:2180`). At mount `resume.loaded === false`, so `resumeStartEpisode` (`Anime.vue:1237-1249`) returns `undefined`.
2. **`episodesAired` fallback.** With `initialEpisode === undefined`, `initSelectedEpisode` falls back to `props.anime.ep`, which `Anime.vue:405` binds to `(anime.episodesAired || 1)` — the latest aired episode. So the player lands on ep 12. When `resume` later resolves to `1`, it is too late — the episode is already selected (one-shot).

For an **anonymous** first-time viewer the same fallback fires directly: `resumeStartEpisode` returns `lastEpisode.value` which is `undefined` until/unless localStorage holds a `watch_progress:{id}` entry.

### 1.2 The tangle (root complexity)

"Which episode opens" is computed in **four** places with **different inputs**, two of which duplicate the `last+1` math and can diverge:

| Place | Computes | Inputs |
|---|---|---|
| `watchCta.ts` `computeWatchCta` | `startEpisode` for the button | `lastWatched, total, listStatus, isAuth` |
| `useResumeStateMachine.ts` `startEpisode` | `startEpisode` for the player (via 5 `kind`s) | `total, episodesAired, nextEpisodeAt, status, loadedEpisodes` |
| `Anime.vue` `resumeStartEpisode` | `override > query > resume > lastEpisode` glue | all of the above |
| `AePlayer.vue` `initSelectedEpisode` (+3 computeds) | `initialEpisode ?? anime.ep ?? 1` | prop + `episodesAired` |

Plus a hybrid `lastEpisode` ref (`Anime.vue:1150`) that is localStorage for anon **and** a mirror of `resume.lastWatched` for authed (watcher at `1184-1186`), and a one-shot mount read. Three orthogonal questions — *which episode to open*, *which banner / CTA verb to show*, *is the next episode available yet* — are interleaved, and the episode number is recomputed inconsistently in all three.

---

## 2. Goals / Non-goals

**Goals**
- One source of truth for the start episode, consumed identically by the CTA and the player.
- Close the race structurally (default 1, reactive consumption) — not via a fallback constant.
- One `lastWatched` resolver covering authed (server) and anon (localStorage).
- Resume banner / CTA verb derived by **pure functions** of metadata, with no 60s timer.
- Net deletions: `watchCta.ts` and `useResumeStateMachine.ts` removed.

**Non-goals**
- No backend/API changes. `watch_progress`, viewer-context aggregate, rewatch endpoint unchanged.
- No change to the rewatch flow, `?episode=N` deep-link contract, or Kodik iframe path.
- CTA verbs stay behavior-identical (5 distinct actions); only their computation relocates.

---

## 3. Target architecture (Variant 1 — one composable seam + pure functions)

```
watchState.ts                      (pure module, replaces watchCta.ts)
  resolveStartEpisode(lastWatched, total): number   // >=1
     first-time(0) -> 1 · caught-up(last>=total) -> last · else last+1   [clamped]
  resolveResumeState({ lastWatched, total, episodesAired, status,
                       nextEpisodeAt, loadedEpisodes, listStatus, isAuth, nowMs })
     -> { banner: BannerState, cta: { action, labelKey, params? } }       // SOLE authority

useWatchState.ts                   (composable, replaces useResumeStateMachine.ts)
  in:  reactive anime metadata refs + isAuth + optional prefetched progress
  out: { lastWatched, loaded, startEpisode, banner, cta }
  - lastWatched: ONE resolver — authed: max(completed) from server progress
                 (prefetched from viewer-context); anon: localStorage `watch_progress:{id}`.
                 Clamped to [0, total].
  - startEpisode / banner / cta = the pure functions above.
```

**Deleted**
- `useResumeStateMachine.ts` (5 `kind`s, `nowMs` 60s timer at lines 120-126, the player→resume coupling stays but moves to a pure input).
- `watchCta.ts` → folded into `watchState.ts`.
- In `Anime.vue`: `lastEpisode` ref (1150), `loadLastEpisode` (1177), the `resume.lastWatched` mirror watcher (1184), the `resumeStartEpisode` mega-computed (1237), `resumeNextEpisodeNumber` (1272), the hand-rolled `resumePillProps` (1285). **Kept:** `resumeOverrideEpisode` (rewatch) and `queryEpisode` (deep-link) — legitimate explicit overrides.

**Kept signals**
- `loadedEpisodes` — player emits max-loaded episode via `available-translations`; feeds `resolveResumeState` as a **pure input** (`aired = max(episodesAired, loadedEpisodes)`), so a freshly-uploaded episode is never mislabeled "not available". We delete the state machine + timer, not this signal.
- **No timer.** Dropping `episode-not-loaded-yet` removes the only consumer of the live 60s ticker (the "aired N ago" label). The remaining ETA label ("airs {when}") is a static formatted date; the future/past comparison runs once at compute time via `Date.now()` inside `resolveResumeState` (or an injected `nowMs` for test determinism). The label refreshes on navigation/reload, not on a ticker — acceptable for a date-grained string.

### 3.1 The single "which episode" path

```
resumeOverrideEpisode (rewatch)  ??  queryEpisode (?episode=N)  ??  watchState.startEpisode
        |                                                                    |
        |  always a number >= 1, default 1, NEVER episodesAired              |
        v                                                                    v
                       AePlayer :initial-episode (required number)
```

### 3.2 Race fix (core of the bug)

1. `:initial-episode` is **always a defined number** (default 1) → the player shows ep 1 immediately instead of falling to `episodesAired`.
2. `AePlayer` consumes `initial-episode` **reactively**, not one-shot: `watch(() => props.initialEpisode)` re-selects the episode **while the user has not manually picked one** (guard flag `userPickedEpisode`). When `resume` resolves to `last+1`, the player moves to it; for a first-time viewer it stays on 1.
3. Remove **all** `?? props.anime.ep` fallbacks from episode selection (`AePlayer.vue:1279, 1344, 1964, 2207`). The display badge uses `selectedEpisode?.number ?? props.initialEpisode ?? 1`. `episodesAired` no longer participates in choosing the start episode.

---

## 4. Behavior — `Use case | Было | Стало | Что упрощаем и как`

### 4.1 User-visible

| Use case | Было | Стало | Что упрощаем и как |
|---|---|---|---|
| Authed, opens **first time** | Race → `episodesAired` fallback → **12** | **Ep 1** | Drop one-shot mount read: `initial-episode` always a number + reactive re-pick → race gone structurally |
| Anon, first time (no localStorage) | `episodesAired` → **12** | **Ep 1** | Start default = 1, not `episodesAired`; `episodesAired` removed from selection |
| Watched up to N | `last+1`, but race could drop to `episodesAired` | `last+1`, stable | Same race fix + one `lastWatched` source |
| Caught up / finished | `last` + "finished" surface | Preserved | Move computation into `resolveResumeState` only |
| `?episode=N` deep-link | `queryEpisode` wins | Preserved | Keep as explicit override in the single chain |
| "Rewatch from ep. 1" | `resumeOverrideEpisode` | Preserved | Keep as the top override |
| Next episode not aired / not loaded | **2** banners + 60s ticker | **1** "coming soon" banner (+ETA if known) | Collapse `not-yet-aired` + `episode-not-loaded-yet`; remove `nowMs` timer + past/future air-time split |
| Manual episode pick, then resume resolves | One-shot → stale / race | Manual pick **not overwritten** | Reactive `watch(initialEpisode)` + `userPickedEpisode` guard |
| Anonymous resume banners | None (machine was authed-only) | Same banners via localStorage `lastWatched` | One unified path (anon parity, approved) |

### 4.2 Code structure

| Concern | Было | Стало | Что упрощаем и как |
|---|---|---|---|
| "Which episode" | Computed in **4 places**, different inputs | **1 chain**: `override ?? query ?? startEpisode` | Collapse to one chain; delete duplicate fallback branches in player + `Anime.vue` |
| `lastWatched` source | Hybrid `lastEpisode` ref + mirror watcher + split server/localStorage | **One resolver** in `useWatchState` | Delete the ref + watcher; resolver branches `authed→server / anon→localStorage` internally |
| `startEpisode` math | **Duplicated** in `watchCta.ts` and `useResumeStateMachine.ts` (diverge at edges) | **One** pure `resolveStartEpisode` | Extract to pure fn; delete both copies; both consumers call the same fn |
| Resume banner / CTA | 5-`kind` machine + `nowMs` timer + `loadedEpisodes` coupling | **One** pure `resolveResumeState` | Replace machine with pure fn of metadata; remove timer; `loadedEpisodes` kept as a pure input |
| Player start fallback | `?? anime.ep (=episodesAired)` in 4 spots | Always a number (**default 1**) | Remove `?? props.anime.ep` at lines 1279/1344/1964/2207 |
| `initial-episode` read | One-shot in `onMounted` (race source) | Reactive | `initSelectedEpisode` → `watch(() => props.initialEpisode)` + manual-pick guard |
| Files | `watchCta.ts` + `useResumeStateMachine.ts` + glue | `watchState.ts` (pure) + `useWatchState.ts` | Delete two tangled files; introduce two focused ones; decisions only in the pure module |

---

## 5. Banner & CTA taxonomy — `Use case | Было | Стало | Что упрощаем и как`

### 5.1 Banners (`ResumePill.vue`)

| Use case | Было | Стало | Что упрощаем и как |
|---|---|---|---|
| Just finished episode N | `kind='watching'` → `resume.justFinished` | Preserved (`resume.justFinished`) | Computation moves to `resolveResumeState`; no `kind` dependency |
| Next episode unaired, **ETA known** | `kind='not-yet-aired'` → `resume.notYetAvailableEta` | Preserved, same key | Condition becomes `nextEpisodeAt` in the future |
| Next episode unaired, **no ETA** | `kind='not-yet-aired'` → `resume.notYetAvailable` | Preserved, same key | — |
| Aired but not uploaded (translation lag) | `kind='episode-not-loaded-yet'` → `resume.episodeNotLoaded` + `airedAgo` label + 60s ticker | **Collapsed** into `resume.notYetAvailable` | **Delete** `resume.episodeNotLoaded`, `airedAgoLabel`, `episodeAiredAgoMs`, `nowMs` timer; stop discriminating past/future air-time |
| Finished / first-time | No surface | No surface | Unchanged |

**Net banners:** 5 `kind`s → **2 visible variants** (`justFinished`, `notYetAvailable[±ETA]`).

### 5.2 CTA (button)

| Use case | Было | Стало | Что упрощаем и как |
|---|---|---|---|
| Not started | `watch` (`anime.watchNow`) | Preserved | Logic moves into `resolveResumeState`, same key |
| In progress | `continue` (`anime.continueEp` {n}) | Preserved | `startEpisode` from shared `resolveStartEpisode` |
| `completed` list, 0 eps | `start-from-1` (`anime.startFromEp1`) | Preserved | — |
| Finished, not marked `completed` | `mark-watched` (`anime.markAsWatched`) | Preserved | — |
| Finished, `completed` | `rewatch` (`anime.resume.rewatch`) | Preserved | — |

**Net CTA:** 5 verbs unchanged; `computeWatchCta` as a separate function disappears. CTA i18n keys untouched.

---

## 6. Tests, i18n, migration, risks — `Use case | Было | Стало | Что упрощаем и как`

| Aspect | Было | Стало | Что упрощаем и как |
|---|---|---|---|
| Unit "which episode" | `watchCta.spec.ts` pins `computeWatchCta` | `watchState.spec.ts` pins `resolveStartEpisode` + `resolveResumeState` | Migrate spec; keep **all** prior CTA assertions (behavior preserved) → proves no regression |
| Resume-state tests | 5-`kind` machine spec | Pure-fn tests for `resolveResumeState` + a `useWatchState` composable test | Pure functions, no time mocks → faster, deterministic |
| Race / ep-12 regression | None | **New** test: unloaded resume → player picks 1; reactively moves to `last+1`; manual pick not overwritten | Codify the bug fix as a test |
| `AePlayer` spec | `?? anime.ep` allowed | Assert `episodesAired` not used, default 1, reactive re-pick + guard | Prevent fallback relapse |
| i18n en/ru/ja | 3 locales × `episodeNotLoaded` (+`airedAgo` format) | Remove `resume.episodeNotLoaded` in all 3; keep `justFinished`/`notYetAvailable`/`notYetAvailableEta` | Fewer keys; run `/frontend-verify` i18n parity gate |
| "aired N ago — translators" message | Distinct informative banner | Replaced by "not yet available" | **Risk accepted:** lose the "already aired, awaiting translation" nuance |
| `loadedEpisodes` | Player→resume overrides lagging Shikimori | **Kept** as a pure input to `resolveResumeState` | Cut only the machine + timer, not this signal → "fresh uploads visible" preserved |
| Anon vs authed | Machine authed-only; anon had no banners | **Unified**: anon gets the same banners via localStorage `lastWatched` | One path; added visible behavior for anon (approved) |
| Deploy | — | `/frontend-verify` → `/animeenigma-after-update` (redeploy-web) | Standard cycle |

---

## 7. Affected files (inventory)

**New**
- `frontend/web/src/composables/watchState.ts` — pure `resolveStartEpisode` + `resolveResumeState`.
- `frontend/web/src/composables/useWatchState.ts` — the single composable seam.
- `frontend/web/src/composables/__tests__/watchState.spec.ts` — migrated + new assertions.

**Deleted**
- `frontend/web/src/composables/watchCta.ts`
- `frontend/web/src/composables/useResumeStateMachine.ts`
- `frontend/web/src/composables/__tests__/watchCta.spec.ts` (migrated into `watchState.spec.ts`)

**Edited**
- `frontend/web/src/views/Anime.vue` — replace resume glue with `useWatchState`; `:initial-episode` always a number; **remove `ep` from the `:anime` prop object** (line 405) — `episodesAired` is no longer passed to the player.
- `frontend/web/src/components/player/aePlayer/AePlayer.vue` — reactive `initSelectedEpisode`; **remove `ep` from the `anime` prop type** (line 406) and audit every `props.anime.ep` read (selection at 1279/1344/1964/2207, the display badge at 92/200, and `anime_hasNextEp` ~1972), rebasing each on `selectedEpisode?.number ?? props.initialEpisode ?? 1` or the `eps` total; add the `userPickedEpisode` guard.
- `frontend/web/src/components/player/ResumePill.vue` — 2-variant banner; drop `episode-not-loaded-yet` branch + `airedAgo`.
- `frontend/web/src/locales/{en,ru,ja}.json` — remove `anime.resume.episodeNotLoaded`.

**Verify caller parity:** `KodikPlayer.vue` also takes `:initial-episode="resumeStartEpisode"` (`Anime.vue:441`) — it receives the same always-a-number value; confirm it has no `episodesAired` fallback of its own.

---

## 8. Metrics

- `UXΔ = +2 (Better)` — new/guest viewers land on ep 1, not mid-season; one coherent resume path.
- `CDI = 0.05 * 21` — Spread wide (4 files + i18n + tests), Shift moderate (behavior mostly preserved; changes = ep-12 fix, banner collapse, anon parity), Effort 21.
- `MVQ = Griffin 88%/80%` — an untangling guardian; high slop-resistance via pure-fn tests.

---

## 9. Open questions

None blocking. Both prior decisions locked: keep `loadedEpisodes`, enable anon parity.
