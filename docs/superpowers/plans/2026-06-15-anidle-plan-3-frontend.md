# Anidle Plan 3 — Frontend Page

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship the complete user-facing anidle frontend: Vue 3 route `/anidle`, composable `useAnidle.ts`, API client `api/anidle.ts`, all components under `components/anidle/`, i18n keys in all 3 locales, Vitest unit tests, and a Playwright e2e spec — so the game is fully playable in the browser.

**Backend is LIVE.** Plans 2a + 2b are shipped and verified. The 8 endpoints exist at `/api/anidle/*` (optional JWT, gateway-proxied). No backend changes needed in this plan.

**Reference spec:** `docs/superpowers/specs/2026-06-15-anidle-anime-guessing-game-design.md` — §2 (mechanics), §5 (API shapes), §6 (frontend spec), §9 (test requirements).

**Analogous existing work to read before implementing any task:**
- `src/api/gacha.ts` — the pattern for a standalone API module with typed interfaces and `apiClient` calls; envelope unwrap is `response.data?.data ?? response.data`
- `src/composables/useRecs.ts` — async API call pattern with `ref`, `isLoading`, `error`, `onMounted`; shows auth-state awareness via `useAuthStore`
- `src/views/Gacha.vue` — shows how a feature view consumes a composable + UI primitives + i18n `$t()`
- `src/components/gacha/GachaSlider.spec.ts` — vitest + `@vue/test-utils` + `createI18n` + handwritten fixtures pattern (NO testify-style mocks — use vi.mock sparsely)
- `src/stores/auth.ts` — `isAuthenticated` (computed) + `user.value?.username`; composables read it to branch guest vs logged-in
- `src/components/ui/SearchAutocomplete.vue` — keyboard-navigable autocomplete input; AnidleSearch wraps pool results into the same UX pattern
- `frontend/web/e2e/raw-player.spec.ts` — `loginAsUiAuditBot` helper pattern for Playwright

---

## Design Decisions (locked)

1. **No secret on the wire.** Client never receives `answer` until `solved === true` or `gave_up === true` on a GiveUp response. The composable enforces this: `answer` ref is only set from a response field, never computed locally.
2. **Guest state in localStorage.** Guests store today's guesses keyed by `anidle:daily:<date>` (JSON array of `GuessOutcome`). Stats (streak, games_played) stored at `anidle:stats`. On login-change the composable re-fetches from the server and overwrites local state.
3. **Single composable interface.** `useAnidle()` returns identical reactive refs regardless of auth state. The guest/server fork is hidden inside the composable.
4. **Mobile = card-per-guess.** 8 columns don't fit a row on mobile. At `<md` breakpoint, `GuessGrid` renders `GuessCard` per row; at `≥md`, it renders a horizontal scroll table. Same data, same coloring logic.
5. **Colors via DS tokens only.** `correct` → `bg-success text-success-foreground`; `partial` → `bg-warning text-warning-foreground`; `wrong` → `bg-muted text-muted-foreground`. No off-palette Tailwind color classes (design-system-lint is a build gate — `correct` must NOT use `bg-green-*`).
6. **Poster hidden until solved.** The answer anime's poster is revealed in `ResultModal` only. During play, `AnidleSearch` dropdown shows poster thumbnails for guesses (not the answer).
7. **Posters via image-proxy.** Use `cardPosterUrl(posterUrl, 128)` from `composables/useImageProxy.ts` for 64×90 thumbnails; `cardPosterUrl(posterUrl, 256)` for the result reveal poster.
8. **No route guard.** `/anidle` is public — guests can play. No `requiresAuth` meta.
9. **Navbar entry.** Add `{ to: '/anidle', label: 'nav.anidle' }` to the `navLinks` array in `Navbar.vue` (line ~391). No feature gate needed — this is fully public.
10. **Dark ship default OFF.** Unlike gacha, anidle is shipped publicly from day one (backend is live, all plans shipped). No `VITE_ANIDLE_ENABLED` gate.

---

## API Response Shapes (from live backend)

```ts
// GET /api/anidle/daily → wrapped in {success, data}
interface DailyState {
  date: string          // "2026-06-15"
  solved: boolean
  gave_up: boolean      // present on result row if user gave up
  guesses: GuessOutcome[]
  answer?: VisibleAnime // populated only when solved || gave_up
}

interface Taxon { id: string; name: string }

// RECONCILED 2026-06-15: backend now returns FULL guessed-anime attributes in
// guess/resume responses (commit 1f2f38bf) so the grid can render each cell's
// VALUE next to the server's status. Same shape as a search result.
interface VisibleAnime {
  id: string
  name_ru: string
  name_en: string
  name_jp: string
  poster_url: string
  year: number
  episodes: number
  score: number
  status: string
  rating: string
  genres: Taxon[]
  studios: Taxon[]
  tags: Taxon[]
}

interface ColumnResult {
  status: 'correct' | 'partial' | 'wrong'
  hint?: 'higher' | 'lower'   // only for numeric columns
}

interface GuessComparison {
  genres:   ColumnResult
  studios:  ColumnResult
  year:     ColumnResult
  episodes: ColumnResult
  score:    ColumnResult
  status:   ColumnResult
  rating:   ColumnResult
  tags:     ColumnResult
}

interface GuessOutcome {
  anime:   VisibleAnime
  result:  GuessComparison
  solved:  boolean
  attempt: number
  answer?: VisibleAnime  // populated only on solve
}

// GET /api/anidle/search?q= → wrapped in {success, data}
// Each item is the SAME shape as VisibleAnime (full attributes). When the user
// picks a search item to guess, keep the full object so the grid can show
// values even before the (status-only) guess response returns.
type SearchResult = VisibleAnime[]

// POST /api/anidle/daily/guess → {success, data: GuessOutcome}  (anime = full VisibleAnime)
// POST /api/anidle/daily/giveup → {success, data: VisibleAnime}
// POST /api/anidle/endless/new → {success, data: {round_token: string}}
// POST /api/anidle/endless/guess → {success, data: GuessOutcome}
// GET /api/anidle/stats → {success, data: UserStats} or 204 for guests
// GET /api/anidle/leaderboard?date= → {success, data: LeaderEntry[]}

interface UserStats {
  user_id: string; games_played: number; games_won: number
  current_streak: number; max_streak: number
  guess_distribution: Record<string, number>; last_played_date: string; updated_at: string
}

interface LeaderEntry { username: string; attempts: number }
```

---

## File Structure (all new files)

```
frontend/web/src/
├── api/anidle.ts                         # Task 1
├── composables/useAnidle.ts              # Task 2
├── views/Anidle.vue                      # Task 3
├── components/anidle/
│   ├── AnidleSearch.vue                  # Task 4
│   ├── GuessGrid.vue                     # Task 5
│   ├── GuessCell.vue                     # Task 5
│   ├── GuessCard.vue                     # Task 5
│   ├── ModeTabs.vue                      # Task 6
│   ├── ResultModal.vue                   # Task 7
│   ├── ShareCard.vue                     # Task 7
│   ├── StatsPanel.vue                    # Task 8
│   └── Leaderboard.vue                   # Task 8
└── locales/
    ├── en.json                           # Task 9 (add "anidle" key)
    ├── ru.json                           # Task 9
    └── ja.json                           # Task 9
```

**Mutations to existing files:**
- `frontend/web/src/router/index.ts` — add `/anidle` route (Task 3)
- `frontend/web/src/components/layout/Navbar.vue` — add nav link (Task 3)

**Test files (new):**
```
frontend/web/src/
├── api/__tests__/anidle.spec.ts          # Task 1
├── composables/useAnidle.spec.ts         # Task 2 (optional, hard — vi.mock localStorage; at minimum assertions on types)
├── components/anidle/
│   ├── GuessCell.spec.ts                 # Task 5
│   ├── AnidleSearch.spec.ts              # Task 4
│   ├── StatsPanel.spec.ts                # Task 8
│   └── ResultModal.spec.ts               # Task 7
└── locales/__tests__/anidle-keys.spec.ts # Task 9
```

**e2e:**
```
frontend/web/e2e/anidle.spec.ts           # Task 10
```

---

## Wave 1 (parallel): API + Composable + i18n keys

### Task 1 — `src/api/anidle.ts`

**Read first:**
- `frontend/web/src/api/gacha.ts` — exact pattern to mirror: typed interfaces, `export const anidleApi = {...}`, `apiClient.get/post`, path relative to `/api/anidle/`
- `frontend/web/src/api/client.ts` lines 1–30 — shows `import { apiClient }` pattern; note envelope: backend wraps with `{success, data}` via `httputil.OK`

**Action:** Create `frontend/web/src/api/anidle.ts`. Define TypeScript interfaces for all backend types (copy the shapes from the "API Response Shapes" section above). Export `anidleApi` object with all 8 endpoint functions. **All functions return the raw axios promise** (composable does the unwrap with `res.data?.data ?? res.data`). Envelope unwrap pattern used in gacha: callers do `response.data?.data ?? response.data` — apply consistently.

Functions to implement:
- `getDailyState(): Promise<...>` → `apiClient.get<{data: DailyState}>('/anidle/daily')`
- `dailyGuess(animeId: string): Promise<...>` → `apiClient.post<{data: GuessOutcome}>('/anidle/daily/guess', { anime_id: animeId })`
- `dailyGiveUp(): Promise<...>` → `apiClient.post<{data: VisibleAnime}>('/anidle/daily/giveup', {})`
- `search(q: string): Promise<...>` → `apiClient.get<{data: SearchResult}>('/anidle/search', { params: { q } })`
- `endlessNew(): Promise<...>` → `apiClient.post<{data: {round_token: string}}>('/anidle/endless/new', {})`
- `endlessGuess(roundToken: string, animeId: string): Promise<...>` → `apiClient.post<{data: GuessOutcome}>('/anidle/endless/guess', { round_token: roundToken, anime_id: animeId })`
- `getStats(): Promise<...>` → `apiClient.get<{data: UserStats}>('/anidle/stats')`
- `getLeaderboard(date: string): Promise<...>` → `apiClient.get<{data: LeaderEntry[]}>('/anidle/leaderboard', { params: { date } })`

Export all interfaces (`DailyState`, `GuessOutcome`, `VisibleAnime`, `GuessComparison`, `ColumnResult`, `UserStats`, `LeaderEntry`, `SearchResultItem`).

- [ ] File created at `frontend/web/src/api/anidle.ts`
- [ ] All 8 interfaces exported
- [ ] `anidleApi` object with 8 functions exported
- [ ] All functions use `apiClient` (never `fetch` directly)
- [ ] Path segments use `/anidle/...` (apiClient prepends `/api/`)
- [ ] `bunx tsc --noEmit` in `frontend/web/` exits 0

**Acceptance criteria:**
- `frontend/web/src/api/anidle.ts` exists
- `grep "export const anidleApi" frontend/web/src/api/anidle.ts` finds the export
- `grep "export interface DailyState" frontend/web/src/api/anidle.ts` finds it
- `bunx tsc --noEmit` in `frontend/web/` exits 0 (no new type errors)

---

### Task 2 — `src/composables/useAnidle.ts`

**Read first:**
- `frontend/web/src/composables/useRecs.ts` — ref/computed/onMounted pattern, auth-state check via `useAuthStore()`, error handling
- `frontend/web/src/stores/auth.ts` lines 46–55 — `isAuthenticated` computed, `user.value`
- `frontend/web/src/api/anidle.ts` (just created) — all types and `anidleApi`
- Spec §2.3, §2.4 — no-cheat contract (never set `answer` from local computation; only from server response)

**Action:** Create `frontend/web/src/composables/useAnidle.ts`. The composable exports a single function `useAnidle()`. It has two inner modes:
- **Guest:** Uses `localStorage` to persist today's guesses (`anidle:daily:<date>` → `GuessOutcome[]`) and stats (`anidle:stats` → partial `UserStats`). All guess comparison hits the server (no-cheat). On mount, loads from localStorage for today.
- **Logged-in:** On mount, calls `anidleApi.getDailyState()` and hydrates from server response.

**Exported reactive interface:**
```ts
interface UseAnidleReturn {
  // Daily
  mode: Ref<'daily' | 'endless'>
  dailyDate: Ref<string>
  dailyGuesses: Ref<GuessOutcome[]>
  dailySolved: Ref<boolean>
  dailyGaveUp: Ref<boolean>
  dailyAnswer: Ref<VisibleAnime | null>
  dailyAttempts: ComputedRef<number>

  // Endless
  endlessToken: Ref<string | null>
  endlessGuesses: Ref<GuessOutcome[]>
  endlessSolved: Ref<boolean>
  endlessAnswer: Ref<VisibleAnime | null>

  // UI state
  isLoading: Ref<boolean>
  isGuessing: Ref<boolean>
  error: Ref<string | null>

  // Stats
  stats: Ref<UserStats | null>
  leaderboard: Ref<LeaderEntry[]>

  // Actions
  submitDailyGuess(animeId: string): Promise<void>
  submitGiveUp(): Promise<void>
  startEndless(): Promise<void>
  submitEndlessGuess(animeId: string): Promise<void>
  setMode(m: 'daily' | 'endless'): void
  fetchStats(): Promise<void>
  fetchLeaderboard(date: string): Promise<void>
  shareResult(): string  // returns emoji-grid string for clipboard
}
```

**Guest localStorage schema:**
- `anidle:daily:<YYYY-MM-DD>` → `JSON.stringify(GuessOutcome[])` (rehydrate on mount if date matches today)
- `anidle:daily:solved:<YYYY-MM-DD>` → `'1'` if solved
- `anidle:daily:gaveup:<YYYY-MM-DD>` → `'1'` if gave up
- `anidle:daily:answer:<YYYY-MM-DD>` → `JSON.stringify(VisibleAnime)` if revealed
- `anidle:stats` → `JSON.stringify({current_streak, max_streak, games_played, games_won})` guest aggregate

**`shareResult()` function:** returns a shareable emoji text string built from `dailyGuesses`. Format:
```
Anidle <date> — <attempt_count> попыток 🎯

<per-row emoji line: 🟩=correct, 🟨=partial, ⬜=wrong, order: genres/studios/year/episodes/score/status/rating/tags>
```

No anime names in the share text (no spoiler). The view component calls `navigator.clipboard.writeText(shareResult())`.

**Rules:**
- Never set `dailyAnswer` to anything except a `VisibleAnime` received from the backend
- After `submitDailyGuess`, if `outcome.solved === true`, set `dailyAnswer = outcome.answer`
- After `submitGiveUp`, set `dailyAnswer = response` (the revealed `VisibleAnime`)
- `dailySolved` and `dailyGaveUp` are terminal — once true, `submitDailyGuess` is a no-op
- On `isAuthenticated` change (watch): re-fetch server state

- [ ] File created at `frontend/web/src/composables/useAnidle.ts`
- [ ] Guest mode: localStorage read on mount for today's date
- [ ] Auth mode: `getDailyState()` called on mount when `isAuthenticated`
- [ ] `shareResult()` returns emoji grid string (no anime names)
- [ ] `answer` ref only set from backend response, never computed
- [ ] `bunx tsc --noEmit` exits 0

**Acceptance criteria:**
- `grep "export function useAnidle" frontend/web/src/composables/useAnidle.ts` finds it
- `grep "shareResult" frontend/web/src/composables/useAnidle.ts` finds the function
- `grep "anidle:daily:" frontend/web/src/composables/useAnidle.ts` finds localStorage key pattern
- `bunx tsc --noEmit` in `frontend/web/` exits 0

---

### Task 9 — i18n keys (en / ru / ja)

> Do this task in parallel with Tasks 1–2 so the keys are ready before the components need them.

**Read first:**
- `frontend/web/src/locales/en.json` lines 1–18 (nav namespace) and line 1509 (gacha namespace — structural reference)
- `frontend/web/scripts/i18n-lint.sh` — parity enforcement: ALL three locales must have identical key structure or `make redeploy-web` fails
- `frontend/web/src/locales/ru.json` — same structure, Russian values
- `frontend/web/src/locales/ja.json` — same structure, Japanese values

**Action:** Add an `"anidle"` top-level namespace to ALL THREE locale files. Also add `"anidle": "Anidle"` (EN), `"anidle": "Аниме-dle"` (RU), `"anidle": "アニメdle"` (JA) to the `"nav"` object in each file.

Add the `"anidle"` namespace **after** `"gacha"` in all three files. The key structure in ALL THREE must be identical (values differ by language). Exact key set:

```json
"anidle": {
  "nav_item": "Anidle",
  "page_title": "Anidle — Guess the Anime",
  "page_subtitle": "Daily and endless anime guessing game",
  "mode_daily": "Daily",
  "mode_endless": "Endless",
  "mode_tabs_label": "Game mode",
  "search_placeholder": "Search anime…",
  "search_no_results": "No results",
  "search_loading": "Searching…",
  "column_genres": "Genres",
  "column_studios": "Studio",
  "column_year": "Year",
  "column_episodes": "Episodes",
  "column_score": "Score",
  "column_status": "Status",
  "column_rating": "Rating",
  "column_tags": "Tags",
  "hint_higher": "↑ Higher",
  "hint_lower": "↓ Lower",
  "guess_button": "Guess",
  "give_up_button": "Give up",
  "give_up_confirm": "Reveal the answer and end today's game? This breaks your streak.",
  "give_up_confirm_yes": "Give up",
  "give_up_confirm_no": "Keep playing",
  "result_win_title": "Correct! 🎉",
  "result_loss_title": "The answer was…",
  "result_attempts": "{n} attempts",
  "result_share_button": "Share result",
  "result_share_copied": "Copied to clipboard!",
  "result_close": "Close",
  "stats_title": "Your stats",
  "stats_games_played": "Games played",
  "stats_games_won": "Won",
  "stats_streak_current": "Current streak",
  "stats_streak_max": "Best streak",
  "stats_distribution_title": "Guess distribution",
  "stats_guest_notice": "Sign in to save your stats and appear on the leaderboard.",
  "leaderboard_title": "Today's leaderboard",
  "leaderboard_empty": "No solvers yet today.",
  "leaderboard_attempts": "{n} attempts",
  "leaderboard_rank": "#{n}",
  "daily_complete_played": "You already played today! Come back tomorrow.",
  "endless_new_round": "New round",
  "endless_win_title": "Correct! Start a new round?",
  "loading": "Loading game…",
  "error_generic": "Something went wrong. Try refreshing."
}
```

- [ ] `"anidle"` namespace added to `en.json` with all keys
- [ ] `"anidle"` namespace added to `ru.json` with Russian values
- [ ] `"anidle"` namespace added to `ja.json` with Japanese values
- [ ] `"nav.anidle"` key added to all three `"nav"` objects
- [ ] `bash frontend/web/scripts/i18n-lint.sh` exits 0

**Acceptance criteria:**
- `python3 -c "import json; d=json.load(open('frontend/web/src/locales/en.json')); print('anidle' in d)"` prints `True`
- `python3 -c "import json; d=json.load(open('frontend/web/src/locales/ru.json')); print(set(d['anidle'].keys()) == set(json.load(open('frontend/web/src/locales/en.json'))['anidle'].keys()))"` prints `True`
- Same key parity check for `ja.json` prints `True`
- `bash frontend/web/scripts/i18n-lint.sh` exits 0

---

## Wave 2 (parallel): GuessCell + GuessCard + GuessGrid + AnidleSearch

All depend on Task 1 (types from `api/anidle.ts`) and Task 9 (i18n keys).

### Task 4 — `AnidleSearch.vue`

**Read first:**
- `frontend/web/src/components/ui/SearchAutocomplete.vue` — keyboard-navigable autocomplete; AnidleSearch uses the same Input + listbox pattern but with a poster thumbnail per result item
- `frontend/web/src/composables/useImageProxy.ts` `cardPosterUrl(url, 128)` function — use for poster thumbnails in the dropdown
- `frontend/web/src/components/ui/Spinner.vue` — for loading state inside the input
- `frontend/web/src/api/anidle.ts` — `SearchResultItem` type and `anidleApi.search(q)`

**Action:** Create `frontend/web/src/components/anidle/AnidleSearch.vue`.

Props:
```ts
defineProps<{
  disabled?: boolean  // true while a guess is submitting or game is over
}>()
```

Emits: `select(id: string)` — emitted when user picks a result. The parent composable handles the actual guess submission.

Behavior:
- Debounce input 300ms before calling `anidleApi.search(q)`; min 2 chars to search
- Show `<Spinner size="sm">` inside the field while loading
- Dropdown listbox with `role="listbox"`, `role="option"` on each item; keyboard ↑/↓ to highlight, Enter to select, Escape to close
- Each result: poster thumbnail (48×64 rounded, use `cardPosterUrl(item.poster_url, 128)`) + `name_ru` (primary) + `name_en` (secondary, smaller, text-muted-foreground) + year chip
- After selection: clear the input, close the dropdown
- The component does NOT submit the guess — it only emits `select(id)`

Design: Use `@/components/ui/Input` as the text field. Dropdown: `absolute top-full mt-2` overlay with `bg-background/95 backdrop-blur-xl border border-white/10 rounded-xl shadow-2xl`.

Token rules: `text-muted-foreground` for year/EN name; hover state `bg-white/10`; no off-palette Tailwind color classes.

- [ ] `AnidleSearch.vue` created
- [ ] Debounced search 300ms
- [ ] Keyboard navigation (↑/↓/Enter/Escape)
- [ ] Poster thumbnail via `cardPosterUrl(..., 128)`
- [ ] `select(id)` emit on pick; input cleared after
- [ ] `disabled` prop prevents interaction
- [ ] No off-palette color classes (design-system-lint compliant)
- [ ] `bunx tsc --noEmit` exits 0

**Acceptance criteria:**
- `grep "defineEmits\|emit.*select\|select.*emit" frontend/web/src/components/anidle/AnidleSearch.vue` finds the emit
- `grep "cardPosterUrl" frontend/web/src/components/anidle/AnidleSearch.vue` finds it
- `grep "debounce\|setTimeout\|useDebounce" frontend/web/src/components/anidle/AnidleSearch.vue` finds debounce
- `grep "font-bold\|font-extrabold\|font-black\|font-light\|font-thin" frontend/web/src/components/anidle/AnidleSearch.vue` finds nothing (DS font-weight rule)
- `bunx tsc --noEmit` exits 0

---

### Task 5 — `GuessCell.vue` + `GuessCard.vue` + `GuessGrid.vue`

**Read first:**
- `frontend/web/src/api/anidle.ts` — `ColumnResult`, `GuessComparison`, `GuessOutcome`, `VisibleAnime` types
- `frontend/web/src/styles/DESIGN-SYSTEM.md` — Tier 2/3 semantic tokens: `bg-success`/`text-success-foreground` (correct), `bg-warning`/`text-warning-foreground` (partial), `bg-muted`/`text-muted-foreground` (wrong)
- `frontend/web/src/locales/en.json` (just added) — `anidle.column_*` keys, `anidle.hint_higher`, `anidle.hint_lower`

**Action A — `GuessCell.vue`:** Single colored cell. Props: `{ status: 'correct' | 'partial' | 'wrong', value: string | number, hint?: 'higher' | 'lower' | null }`. 

Mapping to DS tokens:
- `status === 'correct'` → `bg-success text-success-foreground`
- `status === 'partial'` → `bg-warning text-warning-foreground`
- `status === 'wrong'` → `bg-muted text-muted-foreground`

Arrow: when `hint` is `'higher'`, append ` ↑`; when `'lower'`, append ` ↓`. Use `t('anidle.hint_higher')` / `t('anidle.hint_lower')` for screen-reader accessible label but show only the arrow visually.

The cell is a `<div>` with `rounded-lg p-2 text-sm font-medium text-center transition-colors min-w-[64px]`.

**Action B — `GuessCard.vue`:** Mobile layout for one guess row. Props: `{ guess: GuessOutcome }`. Shows poster (48×64) + anime name above a 2×4 chip grid of the 8 factor cells. Column labels above each chip. Uses `GuessCell` for each factor. Props include `guess.anime.poster_url` (via `cardPosterUrl(..., 128)` with fallback). Shows `guess.anime.name_ru` as title.

**Action C — `GuessGrid.vue`:** Container that switches between desktop table and mobile cards. Props: `{ guesses: GuessOutcome[] }`.

- At `≥md`: horizontal-scroll table with header row (Anime | Genres | Studios | Year | Episodes | Score | Status | Rating | Tags) and one data row per guess. Anime column shows poster thumb (32×44) + name_ru. Each attribute cell is `<GuessCell>`. `overflow-x-auto` on the wrapper.
- At `<md` (hidden): renders `<GuessCard>` per guess, stacked.
- The responsive switch uses Tailwind: header/table `hidden md:block`; card stack `block md:hidden`.
- Newest guess at the top (`guesses` prop should be passed in reverse, or reverse internally — pick one, document it; reversed internally is simpler).
- Empty state: nothing rendered when `guesses.length === 0`.

- [ ] `GuessCell.vue` created; uses `bg-success`/`bg-warning`/`bg-muted` (no green/yellow/gray Tailwind colors)
- [ ] `GuessCard.vue` created; uses `GuessCell` for all 8 columns
- [ ] `GuessGrid.vue` created; responsive switch `md:` breakpoint
- [ ] Horizontal scroll on desktop table (overflow-x-auto)
- [ ] `bunx tsc --noEmit` exits 0

**Acceptance criteria:**
- `grep "bg-success\|bg-warning\|bg-muted" frontend/web/src/components/anidle/GuessCell.vue` finds all three
- `grep "bg-green\|bg-yellow\|bg-gray\|bg-red\|bg-emerald" frontend/web/src/components/anidle/GuessCell.vue` finds NOTHING
- `grep "GuessCell" frontend/web/src/components/anidle/GuessCard.vue` finds it (imports and uses GuessCell)
- `grep "md:block\|md:hidden" frontend/web/src/components/anidle/GuessGrid.vue` finds the responsive switch
- `bash frontend/web/scripts/design-system-lint.sh 2>&1 | grep ERRORS` shows 0 after adding these files

---

## Wave 3 (parallel): ModeTabs + ResultModal/ShareCard + StatsPanel/Leaderboard

All depend on Tasks 4+5, i18n keys available.

### Task 6 — `ModeTabs.vue`

**Read first:**
- `frontend/web/src/components/ui/Tabs.vue` — the existing Tabs primitive; props and slot pattern
- `frontend/web/src/locales/en.json` `anidle.mode_daily`, `anidle.mode_endless`
- `frontend/web/src/api/anidle.ts` — no API needed here; purely UI

**Action:** Create `frontend/web/src/components/anidle/ModeTabs.vue`. A thin wrapper around `<Tabs>` that models `'daily' | 'endless'`. 

Props: `{ modelValue: 'daily' | 'endless' }`. Emits: `update:modelValue`.

Uses `<Tabs :model-value="modelValue" @update:model-value="emit('update:modelValue', $event)">` with two `<TabsTrigger value="daily">` and `<TabsTrigger value="endless">` items. Labels from `$t('anidle.mode_daily')` and `$t('anidle.mode_endless')`. `aria-label` from `$t('anidle.mode_tabs_label')` on the Tabs root.

- [ ] `ModeTabs.vue` created; v-model on `'daily' | 'endless'`
- [ ] Uses `<Tabs>` primitive (not a custom tab implementation)
- [ ] i18n labels from `anidle.mode_daily` + `anidle.mode_endless`
- [ ] `bunx tsc --noEmit` exits 0

**Acceptance criteria:**
- `grep "import.*Tabs" frontend/web/src/components/anidle/ModeTabs.vue` finds `@/components/ui`
- `grep "anidle.mode_daily\|anidle.mode_endless" frontend/web/src/components/anidle/ModeTabs.vue` finds both

---

### Task 7 — `ResultModal.vue` + `ShareCard.vue`

**Read first:**
- `frontend/web/src/components/ui/Modal.vue` (at `src/components/ui/`) — `<Modal v-model="open" :title="...">` pattern used in Game.vue
- `frontend/web/src/composables/useImageProxy.ts` `cardPosterUrl(url, 256)` — for revealed poster
- `frontend/web/src/api/anidle.ts` — `VisibleAnime`, `GuessOutcome[]`
- `frontend/web/src/locales/en.json` — `anidle.result_win_title`, `anidle.result_loss_title`, `anidle.result_attempts`, `anidle.result_share_button`, `anidle.result_share_copied`, `anidle.result_close`

**Action A — `ShareCard.vue`:** A pure display component that renders the shareable emoji grid. Props: `{ guesses: GuessOutcome[], date: string, solved: boolean }`. Renders an emoji grid (one line per guess, 8 emoji per guess: `🟩`/`🟨`/`⬜` per column in order genres/studios/year/episodes/score/status/rating/tags). Shows the date and attempt count as header. Used inside `ResultModal` and also as a standalone visual preview in the modal. No anime names (no spoiler).

Column-to-emoji mapping: `correct` → `🟩`, `partial` → `🟨`, `wrong` → `⬜`.

**Action B — `ResultModal.vue`:** Modal shown on solve or give-up. Props: `{ open: boolean, answer: VisibleAnime, guesses: GuessOutcome[], date: string, solved: boolean }`. Emits: `close`.

Contents:
1. Title: `$t(solved ? 'anidle.result_win_title' : 'anidle.result_loss_title')`
2. Answer poster: `<img :src="cardPosterUrl(answer.poster_url, 256)" class="w-32 h-44 rounded-xl mx-auto object-cover">`
3. Answer name: `answer.name_ru` (large), `answer.name_en` (small, text-muted-foreground)
4. Attempt count: `$t('anidle.result_attempts', { n: guesses.length })`
5. `<ShareCard :guesses="guesses" :date="date" :solved="solved">`
6. Share button: copies `shareResult()` text to `navigator.clipboard.writeText(...)`. Shows `$t('anidle.result_share_copied')` for 2s after copy (use `ref<boolean>` + `setTimeout`).
7. Close button.

The `shareResult()` function is defined in `useAnidle.ts` but the modal also needs to build the string from its props. Simplest: define a `buildShareText(guesses, date, solved)` pure function in `api/anidle.ts` or a separate util `utils/anidleShare.ts`, and import it in both places.

- [ ] `ShareCard.vue` created; emoji grid no anime names
- [ ] `ResultModal.vue` created; poster via `cardPosterUrl(..., 256)`
- [ ] Share button copies text to clipboard; shows copied feedback 2s
- [ ] Uses `<Modal>` primitive (not custom modal)
- [ ] `bunx tsc --noEmit` exits 0

**Acceptance criteria:**
- `grep "cardPosterUrl" frontend/web/src/components/anidle/ResultModal.vue` finds it
- `grep "navigator.clipboard.writeText\|clipboard" frontend/web/src/components/anidle/ResultModal.vue` finds clipboard usage
- `grep "🟩\|🟨\|⬜" frontend/web/src/components/anidle/ShareCard.vue` finds the emoji constants
- `grep "font-bold\|font-extrabold\|font-black\|font-light\|font-thin" frontend/web/src/components/anidle/ResultModal.vue` finds nothing

---

### Task 8 — `StatsPanel.vue` + `Leaderboard.vue`

**Read first:**
- `frontend/web/src/api/anidle.ts` — `UserStats`, `LeaderEntry` types
- `frontend/web/src/locales/en.json` — all `anidle.stats_*` and `anidle.leaderboard_*` keys
- `frontend/web/src/components/ui/Card.vue` — `variant="default"` for stat boxes
- `frontend/web/src/stores/auth.ts` — `isAuthenticated` for the guest notice

**Action A — `StatsPanel.vue`:** Props: `{ stats: UserStats | null, isAuthenticated: boolean }`.

When `!isAuthenticated`: show `$t('anidle.stats_guest_notice')` with a sign-in link (`<router-link to="/auth">`).

When `stats` is non-null: show 4 stat boxes (games_played, games_won, current_streak, max_streak) in a 2×2 grid using `<Card>` primitives. Below: guess distribution histogram (`stats.guess_distribution` — key=attempt count, value=count). Histogram: simple bar chart using `<div class="bg-success h-4 rounded" :style="{width: ...%}">` (proportional to max bar). Labels: attempt number on left, count on right.

When `stats` is null and `isAuthenticated`: show `<LoadingState>`.

**Action B — `Leaderboard.vue`:** Props: `{ entries: LeaderEntry[], loading: boolean }`.

Table or list of top entries with rank, username, attempt count. Rank derived from array index+1. Empty state: `$t('anidle.leaderboard_empty')`. `<LoadingState>` while loading.

- [ ] `StatsPanel.vue` created; guest notice + 4 stat boxes + histogram
- [ ] `Leaderboard.vue` created; rank+username+attempts; empty state
- [ ] Both use `<Card>` primitives (not custom glass-card divs)
- [ ] Histogram bar uses `bg-success` (not `bg-green-*`)
- [ ] `bunx tsc --noEmit` exits 0

**Acceptance criteria:**
- `grep "bg-success" frontend/web/src/components/anidle/StatsPanel.vue` finds it (histogram bar)
- `grep "bg-green\|bg-emerald" frontend/web/src/components/anidle/StatsPanel.vue` finds NOTHING
- `grep "stats_guest_notice\|sign" frontend/web/src/components/anidle/StatsPanel.vue` finds the guest notice
- `grep "leaderboard_empty" frontend/web/src/components/anidle/Leaderboard.vue` finds the empty state

---

## Wave 4: View + Router + Navbar

Depends on Tasks 4–8 (all components ready).

### Task 3 — `views/Anidle.vue` + Router + Navbar

**Read first:**
- `frontend/web/src/router/index.ts` lines 109–120 (Game route), lines 235–252 (Gacha route) — `() => import('@/views/...')` lazy pattern; `meta: { titleKey, requiresAuth }` shape
- `frontend/web/src/components/layout/Navbar.vue` lines 389–395 — `navLinks` array, add one entry
- All component files just created (Tasks 4–8) — know the props/emits before wiring
- `frontend/web/src/composables/useAnidle.ts` — the full interface to consume

**Action A — `views/Anidle.vue`:** The main orchestrator view.

Structure:
```
<template>
  <div class="min-h-screen bg-base pt-20">
    <div class="container mx-auto px-4 py-8 max-w-4xl">
      <!-- Page header -->
      <h1 class="text-3xl font-semibold text-white mb-1">{{ $t('anidle.page_title') }}</h1>
      <p class="text-muted-foreground text-sm mb-6">{{ $t('anidle.page_subtitle') }}</p>

      <!-- Loading / error states -->
      <LoadingState v-if="isLoading" :label="$t('anidle.loading')" />
      <Alert v-else-if="error" variant="destructive">{{ error }}</Alert>

      <template v-else>
        <!-- Mode tabs -->
        <ModeTabs v-model="mode" class="mb-6" />

        <!-- Daily mode -->
        <template v-if="mode === 'daily'">
          <!-- Already played notice -->
          <Alert v-if="dailySolved && !showResult" variant="default" class="mb-4">
            {{ $t('anidle.daily_complete_played') }}
          </Alert>

          <!-- Search + Give Up row (disabled when game over) -->
          <div v-if="!dailySolved && !dailyGaveUp" class="flex gap-3 mb-6">
            <AnidleSearch :disabled="isGuessing" class="flex-1" @select="onDailyGuess" />
            <Button variant="outline" size="sm" :disabled="isGuessing" @click="onGiveUp">
              {{ $t('anidle.give_up_button') }}
            </Button>
          </div>

          <!-- Guess grid -->
          <GuessGrid :guesses="dailyGuesses" class="mb-6" />
        </template>

        <!-- Endless mode -->
        <template v-else-if="mode === 'endless'">
          <div v-if="!endlessToken" class="flex justify-center py-8">
            <Button @click="startEndless">{{ $t('anidle.endless_new_round') }}</Button>
          </div>
          <template v-else>
            <div v-if="!endlessSolved" class="flex gap-3 mb-6">
              <AnidleSearch :disabled="isGuessing" class="flex-1" @select="onEndlessGuess" />
            </div>
            <GuessGrid :guesses="endlessGuesses" class="mb-6" />
            <Button v-if="endlessSolved" @click="startEndless" class="mt-2">
              {{ $t('anidle.endless_new_round') }}
            </Button>
          </template>
        </template>

        <!-- Stats + Leaderboard (below the game area) -->
        <div class="grid grid-cols-1 md:grid-cols-2 gap-6 mt-10">
          <StatsPanel :stats="stats" :is-authenticated="isAuthenticated" />
          <Leaderboard :entries="leaderboard" :loading="loadingLeaderboard" />
        </div>
      </template>
    </div>

    <!-- Result modal (daily solve or give-up) -->
    <ResultModal
      v-if="dailyAnswer"
      :open="showResult"
      :answer="dailyAnswer"
      :guesses="dailyGuesses"
      :date="dailyDate"
      :solved="dailySolved"
      @close="showResult = false"
    />
  </div>
</template>
```

`<script setup lang="ts">` block: imports all components, `useAnidle`, `useAuthStore`, `useI18n`. Destructures `useAnidle()`. Local refs: `showResult` (auto-set true when `dailySolved` or `dailyGaveUp` becomes true), `loadingLeaderboard`. On mount: `fetchStats()` + `fetchLeaderboard(dailyDate.value)` for logged-in users.

`onDailyGuess(id)`: calls `await submitDailyGuess(id)`. If `dailySolved` or `dailyGaveUp` after, sets `showResult = true`.

`onGiveUp()`: calls `await submitGiveUp()`. Sets `showResult = true`.

`onEndlessGuess(id)`: calls `await submitEndlessGuess(id)`.

**Action B — Router:** In `frontend/web/src/router/index.ts`, add the anidle route **before** the gacha routes. Read the file first to find the insertion point.

```ts
{
  path: '/anidle',
  name: 'anidle',
  component: () => import('@/views/Anidle.vue'),
  meta: { titleKey: 'anidle.nav_item' }
},
```

No `requiresAuth` — public route (guests can play).

**Action C — Navbar:** In `frontend/web/src/components/layout/Navbar.vue`, add `{ to: '/anidle', label: 'nav.anidle' }` to the `navLinks` array (line ~391). Add it after `/schedule`.

- [ ] `Anidle.vue` view created; orchestrates all components
- [ ] `showResult` auto-opens `ResultModal` on solve/give-up
- [ ] Route `/anidle` added to `router/index.ts`; no `requiresAuth`
- [ ] `navLinks` updated in `Navbar.vue` with `{ to: '/anidle', label: 'nav.anidle' }`
- [ ] `bunx tsc --noEmit` exits 0

**Acceptance criteria:**
- `grep "path.*anidle\|anidle.*path" frontend/web/src/router/index.ts` finds the route
- `grep "requiresAuth" frontend/web/src/router/index.ts | grep anidle` finds NOTHING (must be absent — public route)
- `grep "nav.anidle\|to.*anidle" frontend/web/src/components/layout/Navbar.vue` finds the nav link entry
- `grep "font-bold\|font-extrabold\|font-black\|font-light\|font-thin" frontend/web/src/views/Anidle.vue` finds NOTHING
- `bunx tsc --noEmit` exits 0

---

## Wave 5 (parallel): Tests

### Task 4b — `AnidleSearch.spec.ts`

**Read first:**
- `frontend/web/src/components/gacha/GachaSlider.spec.ts` — `vi.hoisted`, `createI18n`, `mount`, handwritten fixture pattern
- `frontend/web/src/components/anidle/AnidleSearch.vue` (just created)

**Action:** Create `frontend/web/src/components/anidle/AnidleSearch.spec.ts`. At minimum 5 assertions:
1. Renders an `<input>` element
2. Does not emit `select` without user interaction
3. With `disabled=true`, input has disabled attribute
4. After typing ≥2 chars and waiting for mock search response, dropdown items appear
5. Click on a dropdown item emits `select` with the item's `id`

Mock `@/api/anidle` with `vi.mock`. Use a synthetic `SearchResultItem` fixture. `mount` with `global.plugins: [i18n]` and `stubs: { Spinner: {template:'<span/>'} }`.

- [ ] `AnidleSearch.spec.ts` created with ≥5 assertions
- [ ] `bunx vitest run src/components/anidle/AnidleSearch.spec.ts` exits 0

**Acceptance criteria:**
- `bunx vitest run src/components/anidle/AnidleSearch.spec.ts` exits 0
- Test file has ≥5 `expect(` calls

---

### Task 5b — `GuessCell.spec.ts`

**Read first:**
- `frontend/web/src/components/anidle/GuessCell.vue` (just created)
- `frontend/web/src/components/gacha/GachaSlider.spec.ts` — test structure reference

**Action:** Create `frontend/web/src/components/anidle/GuessCell.spec.ts`. At minimum 5 assertions:
1. `status="correct"` → has class `bg-success`
2. `status="partial"` → has class `bg-warning`
3. `status="wrong"` → has class `bg-muted`
4. `hint="higher"` → rendered output contains `↑`
5. `hint="lower"` → rendered output contains `↓`
6. No off-palette color class (`bg-green`, `bg-yellow`, `bg-gray`) in the rendered HTML for any status

This is a pure presentation component — `mount` with `global.plugins: [i18n]` is sufficient; no mocks needed.

- [ ] `GuessCell.spec.ts` created with ≥5 assertions
- [ ] `bunx vitest run src/components/anidle/GuessCell.spec.ts` exits 0

**Acceptance criteria:**
- `bunx vitest run src/components/anidle/GuessCell.spec.ts` exits 0
- Test contains assertion for `bg-success`, `bg-warning`, `bg-muted`

---

### Task 7b — `ResultModal.spec.ts`

**Read first:**
- `frontend/web/src/components/anidle/ResultModal.vue` and `ShareCard.vue` (just created)

**Action:** Create `frontend/web/src/components/anidle/ResultModal.spec.ts`. At minimum 5 assertions:
1. When `solved=true`, shows the win title text (i18n key `anidle.result_win_title`)
2. When `solved=false`, shows the loss title text
3. Shows `answer.name_ru` when `open=true`
4. `ShareCard` emoji grid contains `🟩` when a guess has a correct column
5. `ShareCard` emoji grid contains `⬜` when a guess has a wrong column
6. No font-weight violation: rendered HTML does not contain `font-bold`

Mock `Modal` as a pass-through stub. Use `mount` with `global.stubs: { Modal: {template:'<div><slot/><slot name="default"/></div>'} }`.

- [ ] `ResultModal.spec.ts` created with ≥5 assertions (tests ResultModal + ShareCard together)
- [ ] `bunx vitest run src/components/anidle/ResultModal.spec.ts` exits 0

**Acceptance criteria:**
- `bunx vitest run src/components/anidle/ResultModal.spec.ts` exits 0
- At least one assertion checks emoji content

---

### Task 8b — `StatsPanel.spec.ts`

**Read first:**
- `frontend/web/src/components/anidle/StatsPanel.vue` (just created)

**Action:** Create `frontend/web/src/components/anidle/StatsPanel.spec.ts`. At minimum 5 assertions:
1. Shows guest notice when `isAuthenticated=false`
2. Guest notice contains a link to `/auth`
3. Shows games_played count when `stats` is provided
4. Shows current_streak count
5. Shows `bg-success` (histogram bar) in rendered HTML — confirms no off-palette color used
6. Shows `<LoadingState>` when `isAuthenticated=true` and `stats=null`

- [ ] `StatsPanel.spec.ts` created with ≥5 assertions
- [ ] `bunx vitest run src/components/anidle/StatsPanel.spec.ts` exits 0

**Acceptance criteria:**
- `bunx vitest run src/components/anidle/StatsPanel.spec.ts` exits 0
- Test contains `stats_guest_notice` assertion

---

### Task 9b — `anidle-keys.spec.ts` (i18n parity)

**Read first:**
- `frontend/web/src/locales/__tests__/spotlight-keys.spec.ts` — parity test pattern for a single namespace (check that en/ru/ja have identical keys under the namespace)
- `frontend/web/src/locales/en.json`, `ru.json`, `ja.json` (all now have `anidle` namespace)

**Action:** Create `frontend/web/src/locales/__tests__/anidle-keys.spec.ts`. Test that `en.json`, `ru.json`, and `ja.json` all have identical key structure under the `"anidle"` top-level namespace. Use `Object.keys` recursively or flatten the object. Pattern from `spotlight-keys.spec.ts`.

- [ ] `anidle-keys.spec.ts` created
- [ ] `bunx vitest run src/locales/__tests__/anidle-keys.spec.ts` exits 0

**Acceptance criteria:**
- `bunx vitest run src/locales/__tests__/anidle-keys.spec.ts` exits 0
- Test checks `en.anidle`, `ru.anidle`, `ja.anidle` have same keys

---

## Wave 6: e2e Playwright

### Task 10 — `e2e/anidle.spec.ts`

**Read first:**
- `frontend/web/e2e/raw-player.spec.ts` lines 22–55 — `loginAsUiAuditBot` helper, `page.goto`, `page.evaluate`
- `frontend/web/e2e/spotlight.spec.ts` lines 1–40 — `test.skip` for graceful backend-down handling
- `docs/ui-audit-test-user.md` — `ui_audit_bot` credentials (`audit_bot_test_password_2026`)
- `frontend/web/src/api/anidle.ts` — endpoint paths to test against

**Action:** Create `frontend/web/e2e/anidle.spec.ts`. Copy `loginAsUiAuditBot` helper verbatim from `raw-player.spec.ts` (same function, same credentials). Tests:

```
test.describe('anidle game (Plan 3)', () => {
```

**Test 1 — anonymous daily meta:** `GET /anidle` without login. The page should mount (no 401, no crash), show the page title, show the search input. `test.skip` if the backend `/api/anidle/daily` returns non-200.

**Test 2 — search autocomplete:** Type 2+ characters into the search input. Expect dropdown to appear with at least 1 result item containing a poster `<img>` and name text.

**Test 3 — daily guess flow (anonymous):** Pick the first autocomplete result. Expect a new row to appear in the grid. Row should contain at least one colored cell (class includes `bg-success` or `bg-warning` or `bg-muted`). The `answer` should NOT appear in the page DOM before solving.

**Test 4 — daily give up (logged-in):** `loginAsUiAuditBot`. Navigate to `/anidle`. Click "Give up" button. Expect the result modal to appear. Expect the revealed anime name to be visible in the modal. Expect the poster `<img>` to appear.

**Test 5 — endless new round:** Click "Endless" tab. Expect "New round" button to appear. Click it. Expect search input to become active (endless round started). Make one guess. Expect a grid row to appear.

**Test 6 — share button:** After test 4 (give-up state), click the share button in the result modal. Expect the button text to briefly change to the "copied" text (check for `.result_share_copied` i18n value in the button).

All tests: `test.skip(false, ...)` on network errors (resilient to backend outage). Use `page.waitForLoadState('networkidle')` after navigation.

- [ ] `e2e/anidle.spec.ts` created with ≥5 test cases
- [ ] Uses `loginAsUiAuditBot` helper from `raw-player.spec.ts`
- [ ] All tests gracefully skip on backend unavailability
- [ ] `bunx playwright test anidle --reporter=list` runs (may skip in CI without live backend)

**Acceptance criteria:**
- `frontend/web/e2e/anidle.spec.ts` exists
- `grep "loginAsUiAuditBot" frontend/web/e2e/anidle.spec.ts` finds the helper
- `grep "test.skip\|skip" frontend/web/e2e/anidle.spec.ts` finds graceful skips
- File has ≥5 `test(` calls

---

## Final Gate — Design System + i18n + TypeScript

After all tasks are implemented:

- [ ] `bash frontend/web/scripts/design-system-lint.sh` exits 0 (ERRORS=0)
- [ ] `bash frontend/web/scripts/i18n-lint.sh` exits 0
- [ ] `bunx tsc --noEmit` in `frontend/web/` exits 0
- [ ] `bunx vitest run src/components/anidle/ src/locales/__tests__/anidle-keys.spec.ts` exits 0
- [ ] Route `/anidle` reachable in browser (no 404, no blank white screen)
- [ ] Search input visible on `/anidle`
- [ ] Guess grid visible after submitting one guess

---

## After All Tasks: Invoke `/animeenigma-after-update`

This is the ONLY time changelog / redeploy / commit+push happens. Do NOT run `animeenigma-after-update` after each task — batch it to the end of this plan.

The changelog entry (Trump-mode) should describe the new `/anidle` game page as an epic feature: "АНИМЕ-DLE НА НАШЕЙ ПЛАТФОРМЕ. Угадывай АНИМЕ по 8 факторам. ЕЖЕДНЕВНЫЙ режим — один секрет на всех. БЕСКОНЕЧНЫЙ режим — играй без ограничений. ВСЕ вычисления на сервере — никаких читов. СТАТИСТИКА и таблица лидеров. Работает без регистрации."

---

## Metrics

- **UXΔ = +3 (Better)** — adds a new sticky interactive engagement surface; game loop creates daily-return habit
- **CDI = 0.04 × 13** — additive frontend-only changes to one isolated route; low spread, moderate effort
- **MVQ = Griffin 85%/80%** — proven `-dle` UX pattern applied cleanly; backend is live and verified, FE just connects the dots
