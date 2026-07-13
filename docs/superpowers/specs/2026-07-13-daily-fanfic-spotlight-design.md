# Daily Fanfic Spotlight — Design Spec

**Date:** 2026-07-13
**Branch:** `feat/daily-fanfic-spotlight`
**Origin:** owner request — "Make new Spotlight slide — Daily Fanfic".

## 1. Goal

Add a 10th `HeroSpotlightBlock` card — **«Фанфик дня»** — that showcases one
user-generated fanfic per UTC day (rotated daily) with its author. When **no
eligible user fanfic exists in the last 24h**, the platform auto-generates one
with randomized parameters under a system bot user. That generation call
doubles as a **daily Groq API-key health probe**: an auth failure (401 /
`invalid_api_key`) fires a Telegram alert to the maintenance chat.

Simultaneously, the fanfic feature is flipped **fully public** (it was
dark-shipped admin-only).

## 2. Locked decisions (owner Q&A, 2026-07-13)

| # | Decision | Choice |
|---|----------|--------|
| Eligible pool | Which fanfics may be featured | **Any `status=complete` fanfic, incl. 18+** (explicit excerpt is gated on render, see §4④ / §6) |
| Author display | Show the author's name? | **Only if the author opted in** (`SpotlightCredit`); else anonymized «участник сообщества» |
| AI fallback | What happens to the generated fanfic | **Bot-owned + labeled «✨ Сгенерировано ИИ» + saved**; forced **teen** rating |
| Card audience | Who sees the card | **Public** — fanfic feature goes fully public now (flip `FANFIC_ADMIN_ONLY`) |
| Job placement | Where the generate/key-test/alert job runs | **Scheduler cron → `POST /internal/fanfic/ensure-daily`** |
| 18+ on card | Rendering of an explicit pick | Logged-out → «18+ — войдите»; explicit full text routed to reader. Full **account-level 18+ setting = fast-follow (OUT OF SCOPE here)** |

**Explicitly out of scope (fast-follow spec):** the account-level
`AllowAdultContent` preference + Settings toggle + server enforcement. This
feature ships the card with explicit picks gated to login/reader; the granular
account toggle lands separately.

## 3. Grounding facts (verified 2026-07-13)

- Spotlight resolvers live in `services/catalog/internal/service/spotlight/`,
  run under an **800ms/card deadline** — generation cannot happen inside a
  resolver. Recipe: `docs/spotlight-card-guidelines.md` + `CLAUDE.md §Adding a
  Spotlight Card Type` (5 anchors).
- `fanfics` table + Groq egress are owned by `services/fanfic/` (`:8097`). No
  internal endpoints exist yet (`/api/fanfic/*` only, all JWT-gated).
- `libs/authz` `Claims` carries `Username` (`jwt.go:29`) → author name can be
  **denormalized at generate-time**, no cross-service username lookup.
- Telegram alert path exists (`services/maintenance/internal/telegram/client.go`;
  player `report.go`). Alerts use `TELEGRAM_ALERTS_BOT_TOKEN` +
  `TELEGRAM_ADMIN_CHAT_ID` (maintenance supergroup). The auth bot token CANNOT
  post there.
- Cross-service reads use `/internal/*` (docker-network-only; gateway does not
  proxy). Pattern: `spotlight/client/player_client.go`.
- Live state: 4 fanfics total, 1 `complete` (teen), feature still dark-shipped
  → the AI fallback is the **common** path at first, so the key-probe angle is
  genuinely useful.

## 4. Architecture

```
scheduler (cron ~04:30 UTC)
   └─ POST http://fanfic:8097/internal/fanfic/ensure-daily
        fanfic svc: eligible user fanfic in 24h?
            yes → no-op
            no  → generate (bot user, teen, random params) via Groq
                    success → persist AIGenerated=true
                    401/auth → Telegram alert (ALERTS bot)

home page render
   └─ catalog spotlight aggregator
        └─ DailyFanficResolver.Resolve (≤800ms)
             └─ GET http://fanfic:8097/internal/fanfic/daily  (compact DTO, excerpt only)
        → SpotlightCard{ type:"daily_fanfic", data: DailyFanficData }

card CTA «Читать»
   └─ GET /api/fanfic/daily  (public; full content; explicit → gated)
        → reader
```

### Components (in scope)

**① Fanfic service (`services/fanfic/`)**

- **Schema (GORM additive migration — new columns only):**
  - `AuthorUsername string gorm:"size:64"` — denormalized from JWT `Username` at create.
  - `SpotlightCredit bool gorm:"default:false"` — opt-in to show author name when featured.
  - `AIGenerated bool gorm:"default:false"` — drives the «✨ Сгенерировано ИИ» badge.
  - ⚠ GORM `default:false` on bool omits the field on `false` inserts — set explicitly in code paths where it matters (see `project_secret_feature_roulette_tab` gotcha).
- **Daily pick logic (`internal/service/daily.go`):** query
  `status=complete AND deleted_at IS NULL AND created_at > now()-24h`, stable
  order `(created_at, id)`. Pool = user fanfics (`AIGenerated=false`) if any,
  else the bot fanfic(s). Deterministic pick `pool[dateSeedUTC() % len(pool)]`
  (tiny day-of-epoch seed helper local to fanfic svc). Empty pool → no pick.
- **`GET /internal/fanfic/daily`** (docker-network, NO JWT): returns compact
  `DailyFanficDTO` (see §5). Explicit pick → `excerpt=""`, `explicit=true` (so
  no explicit text enters the globally-cached spotlight payload). 404 when no pick.
- **`GET /api/fanfic/daily`** (public, optional-JWT, added OUTSIDE the JWT
  group): full content of the same pick for the reader. Non-explicit → full
  content. Explicit → `content=""`, `gated=true`, `gate_reason` (`login` when
  anon; `adult_setting` reserved for the fast-follow). Shares pick logic.
- **`POST /internal/fanfic/ensure-daily`** (docker-network, NO JWT): the job.
  - Eligible user fanfic in 24h → `{generated:false, reason:"user_exists"}`, no Groq call.
  - Else → random params (curated anime pool + random tags/length/POV, **forced
    `teen`, RU**), blocking (non-SSE) generation via the existing
    `service/generate.go` core, persist under **bot user**
    (`UserID`=`FanficBotUserID` const, `AuthorUsername="AnimeEnigma"`,
    `AIGenerated=true`, `SpotlightCredit=true`).
  - Groq **auth failure** (HTTP 401/403 or `invalid_api_key` body) → Telegram
    alert via ALERTS bot + `{generated:false, error:"groq_auth"}`. Other errors
    → logged, `{generated:false, error:...}` (no alert spam).
- **Groq client:** surface HTTP status so the job can classify auth failures
  (add a typed `GroqAuthError` or status field on the returned error).
- **Bot user:** a fixed UUID constant + display name «AnimeEnigma». Internal
  generation bypasses JWT, so **no auth-service user row is required** — the
  fanfic row simply carries the bot UUID. The bot's fanfic is readable publicly
  only via `GET /api/fanfic/daily` (owner-scoped `/api/fanfic/{id}` is
  unchanged — no ownership bypass).
- **Env (compose `fanfic` service):** `TELEGRAM_ALERTS_BOT_TOKEN`,
  `TELEGRAM_ADMIN_CHAT_ID`, `FANFIC_DAILY_ANIME_POOL` (CSV of shikimori IDs,
  sane default), `FANFIC_BOT_LANGUAGE` (default `ru`).

**② Catalog service**

- `internal/service/spotlight/client/fanfic_client.go` — mirror
  `player_client.go`; `FetchDaily(ctx) (*DailyFanficDTO, error)` →
  `GET http://fanfic:8097/internal/fanfic/daily`, 700ms transport timeout,
  404 → `(nil, nil)`.
- `internal/service/spotlight/cards/daily_fanfic.go` — `DailyFanficResolver`
  implementing `spotlight.Resolver`. `Type() == "daily_fanfic"`. Cache key
  `spotlight:daily_fanfic:<DateKeyUTC>`, `cardTTL` (24h), manual
  get/set, **no-cache-on-empty** (`(nil,nil)` when the client returns nothing).
- `internal/service/spotlight/types.go` — `DailyFanficData` struct (JSON-shaped)
  added to the Card union + round-trip test in `types_test.go`.
- `cmd/catalog-api/main.go` — construct the fanfic client + append
  `cards.NewDailyFanficResolver(...)` to `spotlightResolvers`.

**③ Scheduler service**

- `internal/jobs/fanfic_daily.go` — cron ~`30 4 * * *` UTC → `POST
  http://fanfic:8097/internal/fanfic/ensure-daily`. Register alongside existing
  jobs (cleanup / top_anime / provider_ranking pattern). Timeout ~90s (a
  blocking generation can take tens of seconds). Logs outcome; never crashes the
  scheduler on failure.

**④ Frontend (`frontend/web/`)**

- `src/types/spotlight.ts` — extend the `SpotlightCard` discriminated union with
  `{ type:'daily_fanfic', data: DailyFanficData }` + the `DailyFanficData` iface.
- `src/components/home/spotlight/cards/DailyFanficCard.vue` — wraps
  `SpotlightCardShell`, accent **pink**, backdrop **poster-blur** (anime
  poster). Content: kicker «Фанфик дня» (+ icon), anime title + fanfic title,
  2–3-line clamped excerpt (rendered as **text node**, never `v-html`), author
  line (credited → username → profile-link; else «участник сообщества»), rating
  badge, «✨ Сгенерировано ИИ» badge when `aiGenerated`. CTA «Читать» → daily
  reader; secondary «Написать свой» → `/fanfics`. Single-root, `font-medium`/
  `font-semibold` only, `p-4 md:p-6 lg:p-8`, `min-h-[400px] md:min-h-[340px]
  lg:min-h-[320px]`, Tailwind-utility-only. Co-located `.spec.ts` (≥5 asserts).
- `src/components/home/spotlight/HeroSpotlightBlock.vue` — add
  `v-else-if="active.type === 'daily_fanfic'"` dispatch branch (keep the typed
  chain, no `<component :is>`); add the card's image(s) to `cardImageUrls()` at
  the same proxy bucket so idle-prefetch stays a cache hit.
- `src/components/home/spotlight/tokens.ts` — add the `daily_fanfic` entry to
  `cardTokens` (accent `pink` + `kickerKey` + icon; keep kicker ≤~24 chars).
- i18n `spotlight.dailyFanfic.*` in `en.json` + `ru.json` (+ `ja.json` — the
  spotlight namespace has ja; parity test enforces en/ru).
- **Explicit-pick rendering:** excerpt is empty from the API; show the rating
  badge + a gate line — anon → «18+ — войдите, чтобы прочитать»; logged-in →
  «Откройте, чтобы прочитать» (reader gate; the granular account toggle is the
  fast-follow). CTA still routes to the daily reader.

**⑤ Author opt-in UI**

- A checkbox in the fanfic generate form (`GenerateForm`): «Показывать моё имя в
  «Фанфик дня»» → sets `SpotlightCredit` on `POST /api/fanfic/generate`. Default
  unchecked. i18n `fanfic.spotlightCredit.*`.

**⑥ Go-public flip**

- `FANFIC_ADMIN_ONLY=false` (gateway env) + `VITE_FANFIC_ADMIN_ONLY=false` (web
  build env) in `docker/.env` + compose; verify the RBAC/`policy` default does
  not re-gate `/fanfics`. Web rebuild required. (Config change in the base tree
  `.env` is the sanctioned exception; compose/web edits go through the worktree.)

## 5. Wire contracts

**`DailyFanficDTO` (internal, `GET /internal/fanfic/daily`) & `DailyFanficData`
(spotlight payload) — same JSON shape:**

```json
{
  "id": "uuid",
  "fanfic_title": "…",
  "anime_title": "…",
  "anime_japanese": "…",
  "anime_poster": "https://…",
  "excerpt": "first ~240 runes, plain text (EMPTY when explicit)",
  "rating": "teen|mature|explicit",
  "language": "ru|en",
  "explicit": false,
  "author_username": "user123 | \"\"",
  "credited": true,
  "ai_generated": false,
  "part_count": 1,
  "created_at": "2026-07-13T…Z"
}
```

**`GET /api/fanfic/daily` (public reader)** — same fields **plus** full
`content` (empty when `gated`), `gated bool`, `gate_reason string`.

Excerpt derivation: strip leading markdown headings / `---`, take the first
paragraph, clamp to ~240 runes on a word boundary, plain text.

## 6. Error handling & edge cases

- **No pick at all** (empty DB / first run before the job) → resolver returns
  `(nil,nil)`; card simply absent from the carousel. No error surfaced.
- **Fanfic svc down / slow** → client 700ms timeout → resolver `(nil,nil)`
  (card dropped, other cards unaffected). Never fails the whole spotlight.
- **Groq auth failure in ensure-daily** → single Telegram alert (dedupe: only
  alert when generation was actually attempted, i.e. no user fanfic that day).
- **Groq non-auth failure** (rate limit, timeout, model error) → logged, no
  alert, no card that day if no other eligible fanfic.
- **Explicit content isolation** → explicit excerpt/content never enters the
  spotlight payload or the anon reader response (empty + gated), so the
  globally-cached JSON is safe for logged-out visitors.
- **Cache shape change on deploy** → adding a card changes the per-user
  `spotlight:snapshot:*` shape; **flush `spotlight:snapshot:*` once** after
  deploy, then runtime-smoke `/api/anime/spotlight` (or the live home feed).

## 7. Testing

- **Fanfic svc (sqlite in-mem repo, project convention — no testcontainers):**
  daily-pick determinism (same day → same pick; rollover → new pick; user
  preferred over bot; empty pool), DTO shaping (explicit → empty excerpt),
  ensure-daily branches (user-exists no-op; generate path with a fake Groq;
  auth-failure → alert fake invoked), excerpt derivation.
- **Catalog:** resolver cache get/set + no-cache-on-empty + `(nil,nil)` on
  client 404 (handwritten fake client, no testify); `types_test.go` round-trip.
- **Scheduler:** job posts to the right URL, tolerates non-200 (fake server).
- **Frontend:** `DailyFanficCard.spec.ts` ≥5 asserts (credited vs anon author,
  AI badge, explicit gate line, CTA hrefs, single-root); `spotlight-keys.spec.ts`
  en/ru parity; `vue-tsc` narrows the new union branch.
- **Verify commands:**
  `cd services/fanfic && go test ./... -count=1` ·
  `cd services/catalog && go test ./internal/service/spotlight/... -count=1 -race` ·
  `cd frontend/web && bunx vitest run src/components/home/spotlight/ src/locales/__tests__/spotlight-keys.spec.ts && bunx tsc --noEmit` ·
  `/frontend-verify` before finishing FE.
- **Runtime smoke:** trigger `POST /internal/fanfic/ensure-daily` manually,
  confirm a bot fanfic appears + card renders at 1440px & 390px; deliberately
  break the key in a scratch container to confirm the 401 alert path.

## 8. Effort & impact (per `.planning/CONVENTIONS.md`)

- **UXΔ = +2 (Better)** — a new daily discovery surface that also un-hides the
  fanfic feature; additive, low clutter risk.
- **CDI = 0.02 * 21** — Spread across fanfic/catalog/scheduler/web/config but
  almost entirely **additive** (new endpoints, new card, new job); low Shift to
  existing behavior. Effort_Fib 21 (multi-service + new background job + new
  card + public flip).
- **MVQ = Griffin 80%/75%** — composite, mostly-mechanical build on established
  patterns; slop risk sits in generation quality + the explicit-gate edge case.

## 9. Rollout order

1. Fanfic svc (schema + pick + internal/public endpoints + ensure-daily + alert) — deploy.
2. Manually POST `ensure-daily` to seed today's bot fanfic + verify key probe.
3. Catalog resolver + DI — deploy; flush `spotlight:snapshot:*`.
4. Scheduler cron — deploy.
5. Frontend card + opt-in checkbox — `/frontend-verify` → deploy web.
6. Go-public flip (`FANFIC_ADMIN_ONLY=false`, `VITE_FANFIC_ADMIN_ONLY=false`) — gateway restart + web rebuild.
7. `/animeenigma-after-update` (changelog Trump-mode, co-authors, push).
