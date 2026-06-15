# Anidle — Anime Guessing Game (Design Spec)

- **Date:** 2026-06-15
- **Status:** Approved design (pre-plan)
- **Scope:** v1 = single-player; multiplayer prescribed as Phase 2 (not built in v1)
- **Working service name:** `anidle` (anime + `-dle`). User-facing name TBD («Угадай аниме» / «Аниме дня»).

Inspired by the `-dle` guessing-game genre (dotadle.net, Loldle): a secret anime is chosen; the
player submits guesses and each guess is scored per-attribute (🟩 exact / 🟨 partial / ⬜ wrong, with
↑/↓ for numerics). Goal: turn the whole row green.

---

## 1. Goals & Non-Goals

**Goals (v1):**
- Solo anime guessing game over our existing catalog, two modes: **Daily** (one shared secret per day, streaks, shareable result) and **Endless** (unlimited random rounds).
- 8 comparison factors, unlimited attempts (track count).
- Playable by guests AND logged-in users; logged-in get server-persisted stats/streak + a daily leaderboard.
- Reuse existing infrastructure: catalog anime metadata, Shikimori posters via the streaming image-proxy, design-system tokens, i18n (en/ru/ja).

**Non-Goals (v1):**
- Multiplayer (Phase 2).
- "Guess by poster" image mode (Phase 2 candidate).
- Gacha-currency rewards tie-in (future candidate).
- Per-IP abuse hardening beyond basic rate limiting (deferred; log-only if needed).

---

## 2. Game Mechanics

### 2.1 Guessable pool
- **Pool = anime with `score > 8`** and complete metadata for the 8 factors.
- **Franchise collapsing:** multi-season franchises collapse to a **single guessable entry = the first-aired season** (the "original"). Sequels/specials/movies of the same franchise are excluded from BOTH the answer pool and the autocomplete index. Standalone anime (no franchise) are kept individually.
  - Grouping key = Shikimori `franchise` string (see §7 Catalog changes — new field).
  - Representative selection within a franchise: lowest `aired_on` (earliest). Its own attributes (year, episodes, name, poster, score) are used.
- Both the secret answers and the autocomplete suggestions come from this collapsed pool.

### 2.2 Factors (columns)
8 factors, chosen during design:

| Factor | Source field | Type | Feedback |
|---|---|---|---|
| Genres | `Genres` (m2m) | set | 🟩 equal / 🟨 intersection / ⬜ none |
| Studio | `Studios` (m2m) | set | 🟩 / 🟨 / ⬜ |
| Year | `Year` | numeric | 🟩 equal, else ⬜ + ↑/↓ |
| Episodes | `EpisodesCount` | numeric | 🟩 equal, else ⬜ + ↑/↓ |
| Score | `Score` (8.0–10) | numeric | 🟩 equal, else ⬜ + ↑/↓ |
| Status | `Status` | enum | 🟩 equal / ⬜ |
| Rating | `Rating` (age) | enum | 🟩 equal / ⬜ |
| Tags | `Tags` (m2m, AniList) | set | 🟩 equal / 🟨 intersection / ⬜ none |

(Format/`Kind` and Source/`MaterialSource` were intentionally dropped from the column set.)

### 2.3 Comparison algorithm (server-side)
- **Set columns** (genres, studios, tags): equal sets → `correct`; non-empty intersection → `partial`; empty intersection → `wrong`. Tags: display the top-N tags by rank; comparison uses the full tag set.
- **Numeric columns** (year, episodes, score): equal → `correct`; else `wrong` + `hint` = `higher`/`lower` (secret relative to the guess).
- **Enum columns** (status, rating): equal → `correct`; else `wrong` (no partial).
- **Win** = the guessed `anime_id` equals the secret `anime_id` (all cells turn green as a consequence).

### 2.4 Modes & session
- **Daily:** one secret per calendar day, identical for everyone. Streak + "share result" (emoji grid 🟩🟨⬜, no spoiler text). Resets at the server day boundary (UTC-based; see §5.4).
- **Endless:** unlimited random rounds from the pool. Server holds the per-round secret (no-cheat); no streak.
- **Attempts:** no hard cap. Track the attempt count (fewer = better). Optional "give up" reveals the answer and breaks the daily streak.
- **Streak:** on daily solve, if the previous played day was yesterday → `current_streak++`, else reset to 1; `max_streak` updated. Give-up / unsolved day breaks the streak.

### 2.5 No-cheat principle
The secret anime is **never sent to the client** until solved or given up. All guess comparison
happens server-side: the client sends a guessed `anime_id`, the server returns the per-column
coloring. Endless rounds keep the secret in a short-lived Redis key keyed by a round token.

---

## 3. Architecture

```
Frontend (Vue)  ──/api/anidle/*──►  gateway ──►  anidle:8095
  views/Anidle.vue                                  │
  components/anidle/*                                │ internal (Docker-net only)
                                                     ▼
                                          catalog: GET /internal/guessgame/pool
                                          (anime score>8, franchise-collapsed,
                                           8 attrs + names + poster_url)
anidle owns:
  • Postgres: daily_puzzle, user_game_result, user_stats
  • Redis: anidle:pool (snapshot), anidle:leaderboard:<date>, anidle:endless:<token>

Posters: PosterURL (Shikimori, shikimori.io) served via streaming /image-proxy?url=&width=
         (cached on our infra; RU-friendly). MAL/Jikan NOT used in production.
```

- **`anidle` is the single home of the game.** Phase-2 multiplayer (WebSocket rooms) is added here, not in `rooms` (which hosts a separate quiz scaffold).
- **Pool acquisition:** catalog exposes `GET /internal/guessgame/pool` (Docker-network-only, NOT gateway-proxied). `anidle` caches the result in Redis (`anidle:pool`, TTL ~12–24h) and builds an in-memory autocomplete index. Pool is small (a few hundred entries), so comparison is fully local after load.

---

## 4. Data Model

### 4.1 Postgres (owned by `anidle`, GORM automigrate)

```
daily_puzzle
  date            DATE   PK          -- one secret per day
  anime_id        TEXT               -- secret's catalog id
  answer_snapshot JSONB              -- frozen 8 attrs + names + poster_url
  created_at      TIMESTAMP
  -- snapshot freezes the answer: comparison never re-queries catalog, answer can't drift

user_game_result                      -- logged-in only (guests use localStorage)
  id           UUID PK
  user_id      TEXT NOT NULL
  puzzle_date  DATE                  -- NULL for endless
  mode         TEXT                  -- 'daily' | 'endless'
  solved       BOOL
  attempts     INT
  guesses      JSONB                 -- ordered list of guessed anime_ids (resume + audit)
  solved_at    TIMESTAMP NULL
  created_at, updated_at
  UNIQUE(user_id, puzzle_date, mode) -- one daily game per user per day

user_stats                            -- per-user aggregate
  user_id            TEXT PK
  games_played       INT
  games_won          INT
  current_streak     INT
  max_streak         INT
  guess_distribution JSONB            -- histogram attempts -> count
  last_played_date   DATE             -- for streak calc
  updated_at
```

### 4.2 Redis
- `anidle:pool` — cached collapsed pool (id + 8 attrs + names + poster_url) for autocomplete + answer selection. TTL ~12–24h, refreshed from catalog.
- `anidle:leaderboard:<date>` — sorted set; rank = (attempts, then solve-time); member = user_id. Daily leaderboard, logged-in only.
- `anidle:endless:<token>` — ephemeral endless-round secret (TTL 1h).

---

## 5. API

### 5.1 Gateway
`/api/anidle/*` → `anidle:8095` (optional JWT: guests can play; logged-in get persistence + leaderboard).
Internal `GET /internal/guessgame/pool` on catalog is Docker-network-only (not proxied).

### 5.2 Endpoints

| Method | Path | Purpose |
|---|---|---|
| `GET`  | `/api/anidle/daily` | Today's puzzle meta (date, column config) + resume for logged-in (prior guesses, status). **No secret in response.** |
| `POST` | `/api/anidle/daily/guess` | `{anime_id}` → per-column coloring + `solved` + attempt #. On win, reveals answer. Persists guess (logged-in). |
| `POST` | `/api/anidle/daily/giveup` | Reveal answer; break streak. |
| `GET`  | `/api/anidle/search?q=` | Autocomplete over the pool (id, names, poster_url). Public. |
| `POST` | `/api/anidle/endless/new` | Start endless round → `{round_token}` (secret stored in Redis). |
| `POST` | `/api/anidle/endless/guess` | `{round_token, anime_id}` → coloring. |
| `GET`  | `/api/anidle/stats` | Logged-in user's stats (guest → 204; uses localStorage). |
| `GET`  | `/api/anidle/leaderboard?date=` | Top-N of the day + caller's rank. |

### 5.3 Guess response shape
```json
{
  "anime": { "id": "…", "name_ru": "Магическая битва", "poster_url": "…", "year": 2020 },
  "result": {
    "genres":   {"status": "partial", "value": ["Фэнтези", "Экшен"]},
    "studios":  {"status": "correct", "value": ["Madhouse"]},
    "year":     {"status": "wrong",   "value": 2020, "hint": "higher"},
    "episodes": {"status": "wrong",   "value": 24,   "hint": "higher"},
    "score":    {"status": "wrong",   "value": 8.6,  "hint": "higher"},
    "status":   {"status": "correct", "value": "released"},
    "rating":   {"status": "wrong",   "value": "pg_13"},
    "tags":     {"status": "partial", "value": ["Магия", "Сёнен"]}
  },
  "solved": false,
  "attempt": 2,
  "answer": null
}
```
`status` ∈ `correct`|`partial`|`wrong`; `hint` ∈ `higher`|`lower`|`null`. `answer` populated only on solve/giveup.

### 5.4 Daily determinism
On first request for day `D` (or a midnight scheduler job): pick `index = hash(D) mod len(pool)`
from the current pool snapshot, **excluding answers used in the last N days** (default N = 30; query
`daily_puzzle`), then persist the `daily_puzzle` row. Once written, the answer for that day is immutable.

---

## 6. Frontend

- **Route:** lazy `/anidle`; entry point from navbar/home.
- **View:** `frontend/web/src/views/Anidle.vue` orchestrates; components under `components/anidle/`:
  - `AnidleSearch.vue` — debounced autocomplete (→ `/search`) on top of `ui/Input` + dropdown.
  - `GuessGrid.vue` — desktop table (horizontal scroll); on mobile renders `GuessCard.vue` per guess (responsive switch, single data source).
  - `GuessCell.vue` — colored cell by `status` + arrow for numerics.
  - `ModeTabs.vue` — Daily / Endless (on `ui/Tabs`).
  - `ResultModal.vue` + `ShareCard.vue` — win screen, emoji-grid share (🟩🟨⬜) copied to clipboard.
  - `StatsPanel.vue` — attempt histogram + streak; `Leaderboard.vue` — daily top.
- **State** (composable `useAnidle.ts`):
  - **Guest** → localStorage (today's progress keyed by date, stats, streak). Guess comparison still hits the server (no-cheat); only game results are local.
  - **Logged-in** → `GET /daily` restores progress; guesses/stats sync; appears in leaderboard.
  - One composable hides the guest/authed fork behind a single interface.
- **Posters:** rendered via streaming `/image-proxy?url=<PosterURL>&width=<thumb>` (Shikimori source, cached, RU-friendly). The secret's poster is hidden until solved.
- **Colors:** semantic design-system tokens only — `bg-success` (correct), `bg-warning` (partial), muted/secondary surface (wrong). NO off-palette Tailwind color classes (design-system-lint is a build gate).
- **i18n:** namespace `anidle.*` added to **en / ru / ja** (all three — i18n-lint gates `redeploy-web`).
- **Mobile:** 8 factors don't fit a row → each guess is a card with the poster header + a 2×4 chip grid.

---

## 7. Catalog changes (reusable beyond anidle)

1. **New `franchise` field** on the catalog `Anime` model:
   - Add column `franchise TEXT` (GORM tag, indexed).
   - Populate in the Shikimori parser from the GraphQL `franchise` string.
   - Generally reusable (franchise grouping helps recs, "continue the franchise", dedup of listings).
2. **Internal pool endpoint** `GET /internal/guessgame/pool`:
   - Returns anime with `score > 8` and complete metadata for the 8 factors, **franchise-collapsed to the first-aired entry**, each with id + names + poster_url + the 8 attributes.
   - Docker-network-only (not gateway-proxied). Cacheable; `anidle` snapshots it into Redis.

---

## 8. Phase 2 — Multiplayer (prescribed, NOT built in v1)

- Lives in `anidle` (adds a WebSocket layer), not `rooms`.
- Private invite-code room, 2–N players, one shared secret per match.
- Each player guesses on their own board; real-time **opponent progress** shown (attempt count / color grid, without revealing their guessed titles). Winner = first to solve / fewest attempts.
- Redis room state (pattern mirrors `watch-together`), WS auth via `?token=` query param.
- Reuses the same comparison engine + pool.
- Adjacent future candidates: "Guess by poster" mode (blurred/cropped poster that clarifies per attempt — posters already available); gacha-currency reward for streaks.

---

## 9. Testing

- **Backend (Go):** table-driven unit tests for the comparison engine (every column type; edge cases: empty sets, equality, partial), daily determinism + recent-exclusion, streak logic, franchise collapsing. Catalog pool is mocked (no live external calls).
- **Frontend (vitest):** `GuessCell` coloring by `status`, emoji-share-grid generation, desktop/mobile switch, `useAnidle` (guest localStorage vs server). i18n parity test for the `anidle` namespace across en/ru/ja.
- **e2e (Playwright):** `anidle.spec.ts` — full daily flow (guess → colors → win → share) under `ui_audit_bot`.
- **Gates:** design-system-lint (token-only colors) and i18n-lint (3 locales) must pass before redeploy.

---

## 10. Service wiring checklist (new service)

- `go.work` entry + `Dockerfile` for `anidle`; `docker-compose.yml` service (port 8095, env `DB_*` / `REDIS_*` / `JWT_SECRET` / `CATALOG_URL`).
- Gateway route `/api/anidle/*` → `anidle:8095` (optional JWT).
- GORM automigrate for the `anidle` tables.
- Catalog: `franchise` column + parser populate + `GET /internal/guessgame/pool`.
- (No new `libs/` module → the 13-Dockerfile fan-out tax does not apply unless a shared lib is later extracted.)

---

## 11. Effort / Impact (project metrics — per `.planning/CONVENTIONS.md`)

- **UXΔ = +3 (Better)** — a new, sticky, shareable engagement surface; daily-return hook.
- **CDI = 0.06 * 21** — new isolated service + a small catalog field/endpoint + a new frontend route; low spread (mostly additive, isolated), moderate effort.
- **MVQ = Griffin 85%/80%** — well-trodden `-dle` genre mapped cleanly onto existing data; low slop risk.

---

## 12. Open questions / deferred

- Exact `N` for recent-answer exclusion window (default proposal: 30 days) — finalize in plan.
- Whether Endless persists any per-user stats (proposal: no server persistence in v1; local count only).
- User-facing name + navbar placement — decide before frontend build.
- Leaderboard size (top-N) and privacy (show username vs anonymized) — proposal: top 50, show username for logged-in solvers.
