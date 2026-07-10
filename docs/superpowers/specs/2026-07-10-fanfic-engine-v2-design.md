# Fanfic Engine v2 — Design Spec

- **Date:** 2026-07-10
- **Status:** Approved (brainstorming) — pending spec review → writing-plans
- **Source:** two v2-backlog feedback reports filed 2026-07-06:
  - `2026-07-06T01-48-16_tNeymik_manual` — *«Continue this story»* (append the next part to a saved fanfic).
  - `2026-07-06T01-48-17_tNeymik_manual` — *«Continuation of the series»* (generate a fanfic that continues the anime's real plot, with the true synopsis preloaded).
- **Predecessor:** `docs/superpowers/specs/2026-07-06-fanfic-engine-design.md` (v1, shipped). This spec builds the two follow-ups deferred in v1 §12.
- **Owner decisions (locked in brainstorming 2026-07-10):**
  - Build **both** features together, one spec + one plan.
  - #1 «Continue» = **append in place, sectioned** (`--- / ## Часть N`) into the same fanfic's `content`; **one-click, reuse everything** (no per-continue steering input).
  - #2 «Canon continuation» = a **toggle in the existing generate form**; synopsis fetched **server-side** via `CATALOG_URL` (reserved for exactly this in v1 §2); the free-prompt becomes an optional "direction" hint.

---

## 1. Goal

Extend the shipped admin-only fanfic engine (`services/fanfic/` :8097) with the two owner-requested v2 features, reusing the existing SSE generation pipeline, quota, persistence, and reader:

1. **«Continue this story»** — a one-click action on a *saved, complete* fanfic that generates the next part and **appends** it to that same fanfic's `content`, sectioned by a divider + `## Часть N` heading. Prior parts are fed back as context so the continuation stays coherent. Repeatable.
2. **«Continuation of the series»** — a generation **mode** (a toggle in the generate form) where the model continues the anime's *actual* plot beyond where it ended, with the real anime synopsis (catalog `Description`) preloaded server-side into the model context.

**Non-goal (v2):** per-episode / per-arc summaries (catalog stores none — synopsis only); sequel-chain rows / per-part deletion (we append in place); continuing someone else's or an incomplete fanfic; any public/non-admin exposure (the `FeatureGate("fanfic")` policy gate is inherited unchanged).

---

## 2. Architecture

**Extend `services/fanfic/` in place — no new service.** Both features are additive on top of v1:

- New SSE route `POST /api/fanfic/{id}/continue` alongside the existing `/generate`.
- New `canon` flag on the existing `/generate` request.
- A small server-to-server **catalog client** (`internal/catalog/`) for the synopsis preload — mirrors `services/anidle/internal/.../catalog_client.go`. `CATALOG_URL` (default `http://catalog:8081`) — the value v1 reserved for this.

**Gateway** already enumerates the `/api/fanfic/*` routes explicitly under `FeatureGate("fanfic")`; we add exactly one route (`/{id}/continue`) pointed at the existing flushing SSE proxy (`ProxyToFanficStream` → `proxyStreamFlush`).

**Frontend** extends the existing `/fanfics` view — a canon `Switch` in `GenerateForm.vue` and a «Продолжить» button in the library reader Modal. No new route.

**Rejected alternative:** modelling «Continue» as new linked sequel rows (a chain). Rejected in brainstorming — it diverges from v1 §12 ("append to content"), needs schema for parent/child + chain UI, and per-part deletion was an explicit non-goal. Append-in-place keeps one growing document, which is the requested behavior.

---

## 3. Data model — 2 new columns on `fanfics`

```go
// added to domain.Fanfic
Canon     bool `gorm:"default:false" json:"canon"`      // generated in canon-continuation mode
PartCount int  `gorm:"default:1"     json:"part_count"` // 1 on generate, +1 per continue; drives «Часть N»
```

- `Canon` — persisted so (a) the library can badge canon stories and (b) a Continue reuses the same canon mode. Zero value `false` is correct for existing rows (GORM AutoMigrate adds the column defaulted false).
- `PartCount` — **set explicitly to `1` in the generate service path** (not left to the GORM `default:` tag) to sidestep the known "GORM omits zero-value fields" trap. Incremented to `N` on each successful continue and used to render the `## Часть N` heading, so we never re-parse `content` to count parts.

Both are backward-compatible additive columns; no migration beyond AutoMigrate.

---

## 4. API contract (additions)

All under gateway `/api/fanfic/*`, JWT-required, guest-blocked, `FeatureGate("fanfic")` (unchanged from v1).

| Method | Path | Purpose |
|--------|------|---------|
| `POST` | `/api/fanfic/{id}/continue` | **NEW.** Generate the next part of a saved fanfic; SSE; appends to `content` on completion. **Empty body** — every parameter is reused from the stored row. |
| `POST` | `/api/fanfic/generate` | **CHANGED.** Request body gains `"canon": bool`. |

### 4.1 `POST /generate` — request gains `canon`

```json
{
  "anime": { "id": "<uuid>", "shikimori_id": "52991", "title": "Frieren", "japanese": "葬送のフリーレン", "poster": "https://..." },
  "characters": [ { "name": "Frieren" } ],
  "tags": ["adventure"],
  "length": "oneshot",
  "pov": "third",
  "rating": "teen",
  "language": "ru",
  "prompt": "куда двигаться сюжету дальше",
  "canon": true
}
```

- Validation: when `canon` is `true`, at least one of `anime.id` / `anime.shikimori_id` is **required** (needed to fetch the synopsis). All existing enum/cap validation is unchanged.

### 4.2 `POST /{id}/continue` — SSE, no body

Same SSE envelope as `/generate` (`meta` / `delta` / `done` / `error`), with `done` reporting the part number:

```
event: meta   data: {"id":"<uuid>","model":"llama-3.1-8b-instant","part":2}
event: delta  data: {"text":"—\n\n## Часть 2\n\n..."}
...
event: done   data: {"id":"<uuid>","part":2,"token_usage":1876}
```

- Preconditions: the fanfic exists and is **owned by the caller** (both missing and non-owner → `404`, since `repo.Get` is owner-scoped and returns `NotFound`), and is `status=complete` (a `generating`/`failed` row → `409`).
- The row is **not** flipped back to `generating`; its `content` is appended atomically only at `done`. On mid-stream failure the original content is left intact (the append never happens) and an `error` event is emitted — no partial/ghost append.
- **Server-side accumulation is authoritative** (same as v1 §4): a client disconnect does not abort the append — the handler detaches via `context.WithoutCancel`.

---

## 5. Generation pipeline changes

### 5.1 Canon mode (`generate`)

`service.Generate` gains a synopsis-preload step when `req.Canon`:

1. Fetch synopsis: `catalog.FetchSynopsis(ctx, req.Anime.ID, req.Anime.ShikimoriID)` → the anime's `Description` (+ canonical title).
2. **Fail-soft:** empty description, timeout, or unreachable catalog ⇒ proceed **without** the preload (log WARN). The model still knows popular series; a canon gen must never 500 on a catalog blip.
3. `BuildMessages` adds, in the system prompt, a canon instruction ("продолжи РЕАЛЬНЫЙ сюжет за пределами финала аниме, оставаясь верным канону"), and injects `Официальный синопсис: {synopsis}` into the user block. The free-prompt is framed as an optional *direction* ("куда двигаться сюжету"), not the whole brief.

### 5.2 Continue mode (`continue`)

New `service.Continue(ctx, userID, id string, emit Emit) error`:

1. `repo.Get(userID, id)` (owner-scoped) → 404 if missing; `409` if `status != complete`.
2. `quota.Acquire` (Continue counts toward the daily cap, same as a generation).
3. Reconstruct a `GenerateRequest` from the stored row (length, POV, rating, language, characters, tags, **canon**, anime snapshot) and build **continue-mode** messages:
   - System prompt is the same rating/POV/language shape **minus** the "start with `# Title`" instruction, **plus** "это ПРОДОЛЖЕНИЕ; не пиши заголовок, продолжай историю следующей частью".
   - The prior `content` is fed back as context, **capped to a budget** (`FANFIC_CONTINUE_CONTEXT_RUNES`, default ~24 000 runes ≈ ~6k tokens); if the story exceeds it, feed the **tail** (most recent text) — the recent scene matters most for continuity.
4. Build the part **prefix** `"\n\n---\n\n## {Часть|Part} {N}\n\n"` (N = `PartCount+1`, heading word by `language`) and **emit it as the opening `delta`** — so the live reader shows the divider + heading exactly as it will be persisted (live view == stored content). Then stream Groq (`max_tokens` from the stored `length`), relaying body deltas and accumulating the body.
5. On completion: strip any stray leading H1 the model emits (`SplitTitle`, keep body), then persist `prefix + strippedBody` via the atomic append below (§6). Emit `done` with the part number.

### 5.3 Prompt templates

`internal/service/prompt.go` gains: `BuildContinueMessages(f domain.Fanfic, priorContext string)` and canon branches inside `BuildMessages`. All RU+EN variants are deterministic and unit-tested (the v1 tests are the template).

---

## 6. Repository changes

```go
// AppendPart atomically appends a part to content, bumps part_count + token_usage,
// touches updated_at — owner-scoped. Uses a SQL expression to avoid a
// read-modify-write race with a concurrent read.
func (r *Repository) AppendPart(ctx, userID, id, appended string, addedUsage, newPartCount int) error
//   UPDATE fanfics
//   SET content = content || ?, part_count = ?, token_usage = token_usage + ?, updated_at = now()
//   WHERE id = ? AND user_id = ?   (0 rows ⇒ NotFound)
```

`gorm.Expr("content || ?", appended)` keeps the append atomic. `Get` already exists and is owner-scoped.

---

## 7. Catalog client (`internal/catalog`)

Thin HTTP client, `NewClient(baseURL, timeout, log)`:

- `FetchSynopsis(ctx, animeID, shikimoriID string) (title, synopsis string, err error)`:
  - Prefer `GET {base}/api/anime/{animeID}`; fall back to `GET {base}/api/anime/shikimori/{shikimoriID}` when only the shikimori id is present.
  - Parse the standard `{success,data}` envelope → `data.description` (+ `data.name`/`name_ru`).
  - Context + timeout bounded (`FANFIC_CATALOG_TIMEOUT`, default 5s); any error → `("","",err)` so the caller degrades gracefully.
- No auth header needed — catalog's `GET /api/anime/{id}` is a public route reached directly at `:8081` on the docker network (the same call anidle/recs/notifications already make).
- Unit-tested against a fake `httptest` server (no real catalog in tests).

---

## 8. Gateway wiring

- `services/gateway/internal/transport/router.go`, in the existing `/fanfic` group (already `FeatureGate("fanfic")` + JWT + guest-block):
  ```go
  r.Post("/{id}/continue", proxyHandler.ProxyToFanficStream) // SSE, per-chunk flush
  ```
- No new proxy handler — `ProxyToFanficStream` (→ `proxyStreamFlush`) is reused so continue deltas reach the browser live. `ProxyToFanfic` (buffered) still serves list/get/delete/tags.

---

## 9. Config / compose (fanfic service)

New env (all with safe defaults; none secret):

| Var | Default | Purpose |
|-----|---------|---------|
| `CATALOG_URL` | `http://catalog:8081` | server-to-server synopsis fetch |
| `FANFIC_CATALOG_TIMEOUT` | `5s` | synopsis fetch timeout (fail-soft) |
| `FANFIC_CONTINUE_CONTEXT_RUNES` | `24000` | max prior-content runes fed as continue context (tail-truncate beyond) |

`docker/docker-compose.yml`: add the three env vars to the `fanfic` service block and `catalog` to its `depends_on` (ordering only — the client is fail-soft, so a catalog outage never breaks fanfic startup or generation).

---

## 10. Frontend

**Types (`types/fanfic.ts`):** `GenerateInput` gains `canon?: boolean`; `Fanfic` gains `canon: boolean` and `part_count: number`.

**API (`api/fanfic.ts`):** add `continueStory(id, handlers, signal)` — a `fetch`+`ReadableStream` SSE reader identical in shape to `generate()` (Bearer header, 401-refresh-retry, `parseSSEBuffer`/`handleSSEEvent`), POSTing to `/fanfic/{id}/continue` with an empty body. `handleSSEEvent` learns the `part` field on `meta`/`done` (optional, backward-compatible).

**`GenerateForm.vue`:** a canon **Switch** (DS primitive) labelled «Продолжение канона / Canon continuation». When on: the prompt textarea relabels to a "куда двигаться сюжету / where the plot should go" hint (optional) and the character multiselect helper text notes it's optional. Emits `canon` in the `GenerateInput`.

**Library reader Modal (`FanficsView.vue`):** a **«Продолжить»** button in the reader dialog footer, shown only for an owned `status=complete` fanfic. Clicking:
- opens `continueStory(readerFanfic.id, ...)`, appends `delta` text live onto `readerFanfic.content` (the reader re-renders the growing document), and on `done` bumps the local `part_count` and re-`refresh()`es the library grid.
- disabled while a continue is streaming; an inline error surfaces on `error`.

**`renderFanfic.ts` + `FanficReader.vue`:** add an `hr` block type (`---` / `***` / `___` line → `{ type: 'hr' }` → `<hr>`), so the part divider renders as a rule instead of literal characters. The part heading is `## Часть N` — the reader already maps `# `→`<h2>` (title) and `## `→`<h3>` (section); it has **no** `### ` rule, so the heading must stay at `## `.

**`LibraryGrid.vue`:** a small «канон» Badge on cards where `f.canon` (brand-hue Badge, DS-compliant).

**i18n (en/ru/ja, parity-gated):** new keys under `fanfic.canon.*` (toggle label/hint), `fanfic.reader.continue`, `fanfic.reader.continuing`, `fanfic.reader.part` (`«Часть {n}»`), `fanfic.library.canonBadge`. Run `/frontend-verify` before finishing.

---

## 11. Quota / observability / safety

- **Quota:** Continue + canon generations both pass through `quota.Acquire` and the daily cap (`FANFIC_DAILY_CAP`); Redis outage fails open (unchanged).
- **Metrics:** unchanged — the standard `http_*` histograms (`metrics.NewCollector("fanfic")`) already cover both endpoints. No custom fanfic counter exists today; the new `action`/`canon` signal rides on structured logs (below) rather than a new metric — a dedicated counter is a deferred nicety, out of v2 scope.
- **Logging:** continue/canon log `user_id`, `fanfic_id`, `action` (`generate`|`continue`), `canon`, `part`, `token_usage`, `status` via `libs/logger`.
- **Safety stance:** unchanged from v1 — all characters framed 18+, Explicit behind the existing 18+ confirm; canon mode adds no new content surface (same rating tiers). The synopsis is treated as untrusted catalog data injected as prose context, never as instructions.

---

## 12. Testing

- **Go (unit):**
  - `prompt.go` — canon system/user assembly (RU+EN, per tier) and `BuildContinueMessages` (no-title instruction, prior-context inclusion, tail-truncation at the rune budget).
  - `catalog` client — synopsis parse from the `{success,data}` envelope; id vs shikimori fallback; error/empty → graceful `("","",err)`, against a fake `httptest` server.
  - `service.Continue` — owner scoping, `complete`-only guard, append text shape (`--- / ## Часть N`), `part_count`/`token_usage` accounting; canon `Generate` calls the client and degrades soft when it returns empty.
  - `repo.AppendPart` — atomic append + counters + owner scope (`NotFound` on non-owner), on the sqlite in-memory repo.
  - `handler` — `/{id}/continue` SSE happy path + `409` on non-complete + `404` on non-owner.
- **Frontend (vitest):** `continueStory` SSE reader parsing (incl. `part`), canon toggle wiring in `GenerateForm`, `renderFanfic` `hr` block, library canon badge; i18n en/ru/ja parity.
- **Manual smoke (after deploy, with the real key):** one canon generation (RU) + one Continue on a saved fanfic; confirm the appended part renders sectioned and the library reflects the new part.

---

## 13. File manifest (created / touched)

**Touched — `services/fanfic/`:** `internal/domain/fanfic.go` (Canon/PartCount), `internal/domain/request.go` (canon field + validation), `internal/service/prompt.go` (canon + continue templates), `internal/service/generate.go` (canon preload; set PartCount=1), `internal/repo/fanfic.go` (AppendPart), `internal/handler/fanfic.go` (Continue), `internal/transport/router.go` (route), `internal/config/config.go` (CATALOG_URL + timeouts), `cmd/fanfic-api/main.go` (wire catalog client) — plus `*_test.go` alongside.
**New — `services/fanfic/`:** `internal/catalog/client.go` (+ `client_test.go`).
**Touched — gateway:** `internal/transport/router.go` (one route).
**Touched — root/infra:** `docker/docker-compose.yml` (fanfic env + depends_on).
**Touched — frontend:** `src/types/fanfic.ts`, `src/api/fanfic.ts`, `src/components/fanfic/GenerateForm.vue`, `src/components/fanfic/FanficReader.vue`, `src/components/fanfic/renderFanfic.ts`, `src/components/fanfic/LibraryGrid.vue`, `src/views/FanficsView.vue`, `src/locales/{en,ru,ja}.json` (+ co-located specs).

---

## 14. Effort metrics (per `.planning/CONVENTIONS.md`)

- **UXΔ = +2 (Better)** — two net-new creative capabilities (steer a story forward; canon "what happens next") on the existing admin surface; no regression to v1 flows (additive, dark-shipped).
- **CDI = 0.03 * 13** — low spread (one service extended + 1 gateway route + FE toggle/button; 2 additive columns), low shift (mirrors the v1 SSE/quota/reader patterns exactly), Effort 13 (well-understood extension of a shipped service).
- **MVQ = Griffin 88%/85%** — a composite assembled from proven parts (SSE pipeline, quota, catalog-client pattern, XSS-safe reader); low slop risk given the v1 template and unit-test coverage.
