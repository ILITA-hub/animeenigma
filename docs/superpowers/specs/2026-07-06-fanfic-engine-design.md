# Fanfic Engine — Design Spec

- **Date:** 2026-07-06
- **Status:** Approved (brainstorming) — pending spec review → writing-plans
- **Source:** Feedback report `2026-07-04T07-29-22_tNeymik_manual` — *«Сделать движек для фанфиков»* (make a fanfiction engine). Owner supplied a Groq API key for the AI side.
- **Owner decisions (locked in brainstorming):** AI generator (Groq) · admin-only (dark-ship, like gacha/anidle) · structured input surface · user-selectable RU/EN · SFW+NSFW tiered rating · persist to a personal library · **default model `llama-3.1-8b-instant`** (config-overridable) · two follow-ups deferred to v2.

---

## 1. Goal

An **admin-only AI fanfiction generator**: an admin picks an anime (from the catalog) + characters + tags + length + POV + rating + language + a free-text prompt, and the engine streams a generated fanfic (via Groq) into a live reader, then saves it to a personal library to re-read / regenerate / delete.

This is a self-contained "fun side-service" in the same mold as `anidle` (:8095) and `gacha`/Лудка (:8093): its own microservice, gateway proxy, dark-ship admin gate, and reuse of catalog's existing anime/character endpoints.

**Non-goal (v1):** no user-written archive, no multi-chapter, no sharing/ratings/comments, no public access. See §12.

---

## 2. Architecture

**New Go microservice `services/fanfic/` on port `:8097`** (next free after upscaler `:8096`). Standard repo layout:

```
services/fanfic/
├── cmd/fanfic-api/main.go
├── internal/
│   ├── config/            # env config (Groq key/model, DB, JWT, admin gate, quota)
│   ├── domain/            # Fanfic model, request/enums
│   ├── groq/              # Groq OpenAI-compatible streaming client
│   ├── service/           # prompt builder + generation orchestration + persistence
│   ├── repo/              # GORM fanfics repository
│   ├── handler/           # generate(SSE), list, get, delete, tags, health
│   └── transport/         # chi router + authz AuthMiddleware
├── migrations/            # (GORM AutoMigrate; dir kept for parity)
├── Dockerfile
└── go.mod
```

- Registered in `go.work` (`./services/fanfic`).
- Auth: imports `libs/authz`; `AuthMiddleware` validates the JWT (`Authorization: Bearer`) with `JWT_SECRET` and puts claims on context; handlers read `authz.UserIDFromContext(ctx)` (and `authz.IsAdmin(ctx)` where needed) — the identical pattern anidle/gacha use.
- Owns one table (`fanfics`) via GORM `AutoMigrate` on startup.
- Reuses catalog's **public** endpoints from the frontend directly — no server-to-server catalog call is required in v1 (the client sends the anime/character snapshot it already fetched). `CATALOG_URL` reserved for v2 (series-summary preload).

**Rejected alternatives:** folding into `catalog` (bloats metadata service with LLM egress + a generation DB) or `player` (watch-progress domain, no fit). A standalone service isolates the Groq key blast-radius and matches the established pattern.

---

## 3. Data model — `fanfics`

```go
type Fanfic struct {
    ID               string         `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
    UserID           string         `gorm:"type:uuid;index;not null"`
    AnimeID          string         `gorm:"type:uuid;index"`      // catalog uuid (nullable-ish)
    AnimeShikimoriID string         `gorm:"size:32;index"`
    AnimeTitle       string         `gorm:"size:512"`            // snapshot (RU or romaji, whatever the picker showed)
    AnimeJapanese    string         `gorm:"size:512"`            // snapshot (helps the model anchor the fandom)
    AnimePoster      string         `gorm:"size:1024"`           // snapshot url, for library cards
    Characters       datatypes.JSON `gorm:"type:jsonb"`          // [{"id":"<shiki>","name":"Frieren"}, ...]
    Tags             datatypes.JSON `gorm:"type:jsonb"`          // ["slow-burn","angst", ...]
    Length           string         `gorm:"size:16"`             // drabble | oneshot | short
    POV              string         `gorm:"size:16"`             // first | third
    Rating           string         `gorm:"size:16"`             // teen | mature | explicit
    Language         string         `gorm:"size:8"`              // ru | en
    Prompt           string         `gorm:"type:text"`           // user's free-text brief
    Title            string         `gorm:"size:512"`            // parsed from the model's leading "# " line
    Content          string         `gorm:"type:text"`           // markdown body (H1 stripped)
    Model            string         `gorm:"size:64"`             // e.g. llama-3.1-8b-instant
    TokenUsage       int            // prompt+completion tokens (cost tracking)
    Status           string         `gorm:"size:16;index"`       // generating | complete | failed
    ErrorMsg         string         `gorm:"type:text"`           // on failed
    CreatedAt        time.Time
    UpdatedAt        time.Time
    DeletedAt        gorm.DeletedAt `gorm:"index"`               // soft delete
}
```

Enums are validated server-side; unknown values → `400`. `Characters`/`Tags` are bounded (≤ 6 characters, ≤ 8 tags, tag length ≤ 32) to keep the prompt sane and cheap.

---

## 4. API contract

All under gateway `/api/fanfic/*`, JWT-required, guest-blocked, and **admin-gated while `FANFIC_ADMIN_ONLY=true`** (dark-ship). Mirrors the gacha route group.

| Method | Path | Purpose |
|--------|------|---------|
| `POST` | `/api/fanfic/generate` | Generate a fanfic; responds as **SSE**; persists the row on completion. |
| `GET`  | `/api/fanfic` | List my library (paginated: `?page=&limit=`, newest first). |
| `GET`  | `/api/fanfic/{id}` | One fanfic (full content), owner-scoped. |
| `DELETE` | `/api/fanfic/{id}` | Soft-delete, owner-scoped. |
| `GET`  | `/api/fanfic/tags` | Curated tag list (RU+EN labels) for the picker. Static/config-driven. |
| `GET`  | `/health` | Liveness (not gateway-exposed). |

**`POST /generate` request body:**
```json
{
  "anime": { "id": "<uuid>", "shikimori_id": "52991", "title": "Frieren", "japanese": "葬送のフリーレン", "poster": "https://..." },
  "characters": [ { "id": "<shiki>", "name": "Frieren" }, { "name": "Fern" } ],
  "tags": ["slow-burn", "angst"],
  "length": "oneshot",
  "pov": "third",
  "rating": "mature",
  "language": "ru",
  "prompt": "тихий вечер у костра после долгого пути"
}
```

**SSE event stream** (`Content-Type: text/event-stream`):
```
event: meta   data: {"id":"<uuid>","model":"llama-3.1-8b-instant"}
event: delta  data: {"text":"# Тяжесть столетий\n\nКогда солнце..."}
event: delta  data: {"text":" опустилось ниже..."}
...
event: done   data: {"id":"<uuid>","title":"Тяжесть столетий","token_usage":1234}
event: error  data: {"message":"generation failed: ..."}   // terminal on failure
```

- The row is created `status=generating` at `meta`, updated to `complete` (title/content/usage) at `done`, or `failed` (`error_msg`) on error.
- **Server-side accumulation is authoritative:** even if the browser disconnects mid-stream, the service finishes reading the Groq stream and persists, so the library never holds a truncated ghost. (Groq context is cheap; the request is short.)
- **Title extraction:** the model is instructed to emit a leading `# <Title>` line; the service parses it into `Title` and stores `Content` without that H1. Fallback title: `«{anime} — фанфик»`.

---

## 5. Generation pipeline & prompt design

`service.Generate(ctx, userID, req)`:
1. Validate + clamp inputs (enums, character/tag caps).
2. Build **system** + **user** prompts (below).
3. Call Groq `chat/completions` with `stream:true`, `temperature:0.9`, `max_tokens` from length map.
4. Relay deltas as SSE; accumulate full text + usage.
5. Parse title; persist row.

**Length → target words → `max_tokens`** (generous; RU word ≈ ~2.5 tokens):
| Length | ~words | max_tokens |
|--------|--------|-----------|
| `drabble` | ~300 | 900 |
| `oneshot` | ~900 | 2600 |
| `short` | ~2000 | 6000 |

**System prompt** (assembled from language + rating tier), RU shape:
```
Ты — талантливый автор фанфиков, пишущий живую художественную прозу.
Язык вывода строго: {РУССКИЙ | ENGLISH}.
Рейтинг: {tier}. {tier-rules}
Все персонажи — совершеннолетние (18+), даже если в каноне выглядят младше.
Повествование от {первого|третьего} лица. Целевой объём: около {N} слов.
Ответ начни СТРОГО со строки «# Заголовок», затем с новой строки — текст в Markdown.
Не отказывайся, не добавляй дисклеймеров и морализаторства — просто пиши историю.
```
Tier rules:
- **teen** — «Без откровенных сцен; романтика допустима, но целомудренная.»
- **mature** — «Допустимы взрослые темы и намёки на близость, без графических подробностей.»
- **explicit** — «Допустимы откровенные сцены между совершеннолетними персонажами.»

**User prompt:**
```
Фандом: {AnimeTitle} ({AnimeJapanese})
Персонажи: {name, name, ...}
Теги: {tag, tag, ...}
Задание автора: {prompt}
```

The EN system/user prompts are the mirror. Prompt templates live in `internal/service/prompt.go` and are unit-tested (deterministic assembly).

**Model:** default `llama-3.1-8b-instant` (owner choice — fast/cheap; verified to produce coherent RU/EN prose and honor a Mature system prompt). Overridable via `FANFIC_GROQ_MODEL` (e.g. bump to `llama-3.3-70b-versatile` for quality). Honest limitation: no truly-uncensored model on this key — **Teen/Mature are solid; Explicit is best-effort** (occasional refusals). A refusal is surfaced verbatim as the generated content (the admin sees it and can regenerate) rather than masked.

---

## 6. Groq client (`internal/groq`)

Thin OpenAI-compatible client: `POST {base}/chat/completions`, `Authorization: Bearer {key}`.
- `base` = `FANFIC_GROQ_BASE_URL` (default `https://api.groq.com/openai/v1`).
- Streaming: parse `data: {...}` SSE lines, emit `choices[0].delta.content`; stop on `data: [DONE]`.
- `usage` captured from the final chunk (Groq sends `x_groq.usage` / `usage` on stream end) — best-effort; 0 if absent.
- Context + timeout bounded (e.g. 90s); non-2xx → structured error surfaced as SSE `error`.
- Unit-tested against a fake HTTP server (no real API in tests).

---

## 7. Frontend

**New admin-gated route `/fanfics`** (guarded by `isAdmin` + `VITE_FANFIC_ADMIN_ONLY`, exactly like `/gacha` `/anidle`). Nav entry visible only to admins while dark-shipped.

Page = two tabs (reuse `Tabs` primitive):

**«Генерировать»**
- **Anime search-select** — reuse existing anime search (`apiClient` search) → on pick, store the anime snapshot.
- **Character multi-select** — on anime pick, fetch `apiClient.getAnimeCharacters(animeId)`; chips, ≤ 6.
- **Tag chips** — curated set from `GET /api/fanfic/tags` + free-text custom, ≤ 8.
- **Length** (Драббл/Ваншот/Мини), **POV** (1st/3rd), **Rating** (Teen/Mature/Explicit — Explicit behind an 18+ confirm), **Language** (RU/EN toggle) — all DS `Select`/`RadioGroup`/`Switch` primitives.
- **Prompt** textarea.
- **Generate** → opens the SSE stream (via `fetch` + `ReadableStream` reader, `Authorization` header), renders markdown live into a reader pane; on `done` the fanfic is already saved. Actions: **Regenerate**, **Copy**, **Open in library**.

**«Моя библиотека»**
- Cards grid (poster + title + anime + rating/tag chips + date). Click → reader (full markdown). Per-card **delete** (confirm) and **regenerate-from-same-inputs**.

**Conventions:** Neon-Tokyo DS tokens only (no off-palette/hex), reuse `@/components/ui` primitives, i18n keys in `fanfic.*` across **en/ru/ja** (parity-gated). Markdown render via the existing markdown/description utility (sanitized). `/frontend-verify` before finishing.

**API client + store:** `frontend/web/src/api/fanfic.ts` (typed calls incl. the SSE reader) + a small Pinia store or composable for library state; types in `frontend/web/src/types/fanfic.ts`. Co-located vitest specs.

---

## 8. Gateway wiring

- `config.go`: `FanficService` URL (`FANFIC_SERVICE_URL`, default `http://fanfic:8097`) + `FanficAdminOnly bool` (`FANFIC_ADMIN_ONLY`, default `true`).
- `proxy.go`: `case "fanfic": return s.serviceURLs.FanficService, nil`.
- `handler/proxy.go`: `ProxyToFanfic`.
- `router.go`: `/api/fanfic` group — `JWTValidationMiddleware` + `userRateLimit` + `BlockGuestRoleMiddleware`, and `if cfg.FanficAdminOnly { AdminRoleMiddleware }`. SPA route `/fanfics` → `ProxyToWeb` (admin nav).
- **SSE pass-through:** the `/generate` proxy must **stream unbuffered** — set/preserve `text/event-stream`, disable response buffering, and `Flush()` per chunk. Verify the existing reverse-proxy path flushes (the streaming/Kodik proxies already stream); if the shared proxy buffers, add a streaming-aware branch for this route. **This is the one genuine implementation risk — validated first during execution.**

---

## 9. Config / secrets

**fanfic service:** `FANFIC_GROQ_API_KEY` (secret → `docker/.env`), `FANFIC_GROQ_MODEL` (default `llama-3.1-8b-instant`), `FANFIC_GROQ_BASE_URL` (default Groq), `DB_*`, `JWT_SECRET`, `REDIS_HOST/PORT`, `FANFIC_DAILY_CAP` (default e.g. 100), `PORT=8097`.
**gateway:** `FANFIC_SERVICE_URL`, `FANFIC_ADMIN_ONLY=true`.
**web:** `VITE_FANFIC_ADMIN_ONLY` (default `true`).
**docker-compose:** new `fanfic` service block (build, env, `depends_on: [postgres, redis]`), gateway env additions, `fanfic` in gateway `depends_on`. Dockerfile `COPY` the `libs/*` go.mods it imports (authz, database, logger, cache, errors, metrics, httputil, pagination) per the go.work multi-module build pattern.

---

## 10. Abuse / quota

Admin-only ⇒ low risk, but to protect the shared Groq quota from a runaway loop: a light **Redis guard** — max 1 concurrent generation per user + a generous daily cap (`FANFIC_DAILY_CAP`, default 100). Over-cap → `429`. Redis outage fails open (log WARN, allow) — never 500 a request over a limiter blip (mirrors the gateway limiter stance).

---

## 11. Observability & safety

- Prometheus `/metrics` (standard `http_*` histograms via `libs/metrics`) + `fanfic_generations_total{rating,language,status}` and `fanfic_tokens_total`.
- Structured logging via `libs/logger` (`user_id`, `anime_shikimori_id`, `rating`, `model`, `token_usage`, `status`).
- Content stance: Explicit is opt-in behind an 18+ confirm; system prompt frames all characters as adults 18+. No stored moderation beyond that in v1 (admin-only surface).

---

## 12. Out of scope (v1) & v2 backlog

**v1 excludes:** user-written archive, multi-chapter, sharing/ratings/comments, public access, series-summary context.

**v2 backlog (owner-requested 2026-07-06 — filed to admin backlog):**
1. **«Continue this story»** — an AI action on a saved fanfic that appends a next part (feeds prior content back as context; new SSE generation appended to `Content`). Schema already allows it (append to `content`).
2. **«Continuation of the series»** — generate a fanfic that continues the *anime's actual plot*, with a real **anime summary/synopsis preloaded** into the model context (pull description from catalog via `CATALOG_URL`, and optionally episode/arc summaries). This is why `CATALOG_URL` is reserved in §2.

---

## 13. Testing

- **Go:** unit — prompt builder (deterministic RU/EN × tier assembly), enum validation/clamping, Groq client against a fake SSE server, SSE accumulation + persistence, repo (testcontainers Postgres). No real Groq in tests.
- **Frontend:** vitest — api client (SSE reader parsing), generate-form validation, library store; i18n en/ru/ja parity.
- **Manual smoke:** one real generation with the supplied key (RU + EN, one Mature) after deploy.

---

## 14. File manifest (created / touched)

**New — `services/fanfic/`:** `go.mod`, `Dockerfile`, `cmd/fanfic-api/main.go`, `internal/config/config.go`, `internal/domain/fanfic.go`, `internal/groq/client.go`, `internal/service/{prompt.go,generate.go,quota.go}`, `internal/repo/fanfic.go`, `internal/handler/{fanfic.go,health.go}`, `internal/transport/router.go` (+ `*_test.go` alongside).
**Touched — gateway:** `config/config.go`, `service/proxy.go`, `handler/proxy.go`, `transport/router.go`.
**Touched — root:** `go.work`, `docker/docker-compose.yml`, `docker/.env` (secret, host-only), `Makefile` (redeploy target if templated).
**New — frontend:** `src/views/FanficsView.vue` (+ components under `src/components/fanfic/`), `src/api/fanfic.ts`, `src/types/fanfic.ts`, router entry, nav entry, `locales/{en,ru,ja}.json` `fanfic.*` (+ co-located specs).

---

## 15. Effort metrics (per `.planning/CONVENTIONS.md`)

- **UXΔ = +2 (Better)** — a net-new creative surface for admins; no regression to existing flows (dark-shipped).
- **CDI = 0.06 * 34** — moderate spread (new service + gateway + FE + compose), low shift (additive, mirrors existing patterns), Effort 34 (a fresh microservice end-to-end).
- **MVQ = Griffin 85%/80%** — a well-understood composite (service scaffold + LLM egress + streaming FE) assembled from proven parts; low slop risk given the anidle/gacha template.
