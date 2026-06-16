# Smart Source Selection — Stage 2c: Ranking-Aware Default + Same-Day Override + Staged Playback Fallback

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the unified player actually *use* the Stage 2b learned reliability ranking: pick the best-working provider by default (without ever overriding a user's saved choice), record a same-day `srcfix` override when a fallback rescues a failed resolve, and offer a one-shot auto-switch when a source dies mid-playback. This is the user-visible payoff of Smart Source Selection.

**Architecture:** The ranking layer stays an INNER provider-selection layer beneath the untouched watch-combo resolver (Stage 1b). Catalog gains a tiny `srcfix:{animeId}` write endpoint (24h TTL, same Redis the Stage 2b reader already uses — DB 0) and the `/source-ranking` reader merges it. The frontend fetches `/api/anime/{id}/source-ranking` once per watch page, converts `{fix → perAnime → global}` into a preferred-provider order, and prepends it to `CURATED_TIER` in `pickSmartDefault`. On a successful resolve-time OR playback-time fallback, the player POSTs the now-working provider back as the `srcfix`. The saved-combo restore path (Stage 1b `preferenceSettled`) is unchanged and still wins — the ranking only influences the auto-pick that runs when there is no saved choice.

**Tech Stack:** Go (catalog), Vue 3 `<script setup>` + Vitest, `bun`/`bunx`, chi, Redis (`libs/cache`), vue-i18n (3 locales en/ru/ja).

---

## Decision order (the contract this stage implements)

When the player needs a provider for an anime, the selection precedence is:

1. **User's saved combo** (Stage 1b `/preferences/resolve` → `applyResolvedCombo`, gated by `preferenceSettled`) — NEVER overridden by the ranking. If the saved source is dead, Stage 1b already shows the "source you watched last time isn't available" toast and falls through to the smart default.
2. **`srcfix:{animeId}`** — a same-day override written when a fallback rescued a failed resolve (24h TTL). Merged first in the ranking order.
3. **Per-anime top** (`player_ranking:anime:{id}`, best-first).
4. **Global top** (`player_ranking:global`, best-first).
5. **`CURATED_TIER`** (hand-ranked, Stage 1a).
6. **First remaining active row.**

Steps 2–6 are exactly what `pickSmartDefault` walks: it already takes a `curated: string[]` order, filters to `active` rows, and applies the `ae` availability probe. Stage 2c builds the `[srcfix, ...perAnime, ...global]` prefix and passes `[...prefix, ...CURATED_TIER]` as `curated`. **No `pickSmartDefault` signature change** — only an internal dedup so the prefix+curated overlap can't double-probe a gated id.

## Abuse / caution notes (per the "ship gradually" directive)

- `srcfix` is a **public, unauthenticated** write (the watch page is public for anon users). It is **self-healing**: a bad `srcfix` provider that itself fails gets overwritten by the next user's successful fallback, and the daily Stage 2b recompute supersedes it after at most 24h. The write **validates the provider against the known-id allowlist** (rejects garbage) and is **log-only** for abuse (no per-IP cap yet — consistent with the deferred OpenSubtitles hardening). Document this; don't add a rate limiter in this stage.
- Playback-time fallback is a **single-hop, one-shot-per-episode auto-switch on demonstrable mid-play failure** (`media_fatal` after playback already started), with an informative toast. Silent multi-hop auto-advance is explicitly **deferred** — this is the conservative "opt-in-feeling" stage.

## File Structure

| File | Responsibility | Action |
|---|---|---|
| `services/catalog/internal/service/sourceranking/reader.go` | add `Fix string` to `Ranking`; read `srcfix:{id}` key | Modify |
| `services/catalog/internal/service/sourceranking/writer.go` | new `Writer` — validate provider + SET `srcfix:{id}` 24h | Create |
| `services/catalog/internal/service/sourceranking/writer_test.go` | validation + key/TTL tests with a fake setter | Create |
| `services/catalog/internal/service/sourceranking/reader_test.go` | add a `Fix` read test | Modify |
| `services/catalog/internal/handler/source_ranking.go` | add `Post` (source-fix write) handler method | Modify |
| `services/catalog/internal/handler/source_ranking_test.go` | POST 204 + bad-provider 400 tests | Modify |
| `services/catalog/internal/transport/router.go` | register `POST /{animeId}/source-fix` | Modify |
| `services/catalog/cmd/catalog-api/main.go` | build `Writer` + a `stringSetter` cache adapter; pass to handler | Modify |
| `frontend/web/src/types/sourceRanking.ts` | `SourceRanking` + `SourceRecord` TS types | Create |
| `frontend/web/src/api/client.ts` | `catalogApi.getSourceRanking` + `postSourceFix` | Modify |
| `frontend/web/src/composables/unifiedPlayer/rankingOrder.ts` | pure `rankingToOrder(ranking): string[]` | Create |
| `frontend/web/src/composables/unifiedPlayer/rankingOrder.spec.ts` | unit tests | Create |
| `frontend/web/src/composables/unifiedPlayer/smartDefault.ts` | internal dedup of the curated walk | Modify |
| `frontend/web/src/composables/unifiedPlayer/smartDefault.spec.ts` | dedup test (create if absent) | Modify/Create |
| `frontend/web/src/components/player/unified/UnifiedPlayer.vue` | fetch ranking; prepend order; srcfix write on fallback; playback-time one-shot switch | Modify |
| `frontend/web/src/locales/{en,ru,ja}.json` | `player.sourceFallback.*` keys | Modify |
| `frontend/web/public/changelog.json` | Trump-mode user entry | Modify |

---

## Task 1: Catalog — `srcfix` write endpoint + reader merge

**Files:** `services/catalog/internal/service/sourceranking/{reader.go,writer.go,writer_test.go,reader_test.go}`, `internal/handler/source_ranking.go` (+test), `internal/transport/router.go`, `cmd/catalog-api/main.go`

**Context:** Stage 2b shipped `sourceranking.Reader` (reads `player_ranking:global` + `player_ranking:anime:{id}` from Redis DB 0) and `GET /api/anime/{id}/source-ranking`. This task adds the `srcfix:{id}` write (a same-day per-anime override) and merges it into the read response as a new `Fix` field. Catalog both writes and reads `srcfix` on its own Redis (DB 0) — no cross-service DB concern (see ISS-033).

- [ ] **Step 1: Extend the reader to surface `srcfix`.**

In `reader.go`, add a `Fix` field to `Ranking` and read the key:

```go
// Ranking is the assembled response for one anime.
type Ranking struct {
	Global   []Record `json:"global"`
	PerAnime []Record `json:"perAnime"`
	// Fix is a same-day per-anime override provider (srcfix:{id}, 24h TTL),
	// written when a client-side fallback rescued a failed resolve. Empty = none.
	Fix string `json:"fix"`
}
```

In `Read`, after the per-anime read, add:

```go
	if animeID != "" {
		if s, ok := r.cache.GetString(ctx, "srcfix:"+animeID); ok {
			out.Fix = s
		}
	}
```

(Place the `srcfix` read inside the existing `if animeID != "" {` block, or add a second guarded block — either is fine; keep the existing `player_ranking:anime:` read intact.)

- [ ] **Step 2: Add the `Fix` read test** to `reader_test.go` (mirror the existing `TestReadRanking_GlobalAndAnime` fake-cache style):

```go
func TestReadRanking_Fix(t *testing.T) {
	f := fakeGetter{vals: map[string]string{
		"srcfix:uuid-1": "allanime",
	}}
	out := NewReader(f).Read(context.Background(), "uuid-1")
	if out.Fix != "allanime" {
		t.Errorf("Fix = %q, want allanime", out.Fix)
	}
}
```

Run: `cd services/catalog && go test ./internal/service/sourceranking/ -run TestReadRanking -v 2>&1 | tail`. Expect PASS (this also re-runs the existing reader tests).

- [ ] **Step 3: Write the failing writer test** `writer.go`'s `Writer` (TDD). Create `writer_test.go`:

```go
package sourceranking

import (
	"context"
	"testing"
	"time"
)

type fakeSetter struct {
	key string
	val string
	ttl time.Duration
}

func (f *fakeSetter) SetString(_ context.Context, key, val string, ttl time.Duration) error {
	f.key, f.val, f.ttl = key, val, ttl
	return nil
}

func TestWriter_ValidProvider(t *testing.T) {
	s := &fakeSetter{}
	w := NewWriter(s)
	if err := w.SetFix(context.Background(), "uuid-1", "allanime"); err != nil {
		t.Fatalf("SetFix: %v", err)
	}
	if s.key != "srcfix:uuid-1" {
		t.Errorf("key = %q, want srcfix:uuid-1", s.key)
	}
	if s.val != "allanime" {
		t.Errorf("val = %q, want allanime", s.val)
	}
	if s.ttl != 24*time.Hour {
		t.Errorf("ttl = %v, want 24h", s.ttl)
	}
}

func TestWriter_RejectsUnknownProvider(t *testing.T) {
	s := &fakeSetter{}
	w := NewWriter(s)
	if err := w.SetFix(context.Background(), "uuid-1", "totally-fake"); err == nil {
		t.Fatal("want error for unknown provider, got nil")
	}
	if s.key != "" {
		t.Errorf("must not write on rejection, wrote key=%q", s.key)
	}
}

func TestWriter_RejectsEmptyAnimeID(t *testing.T) {
	s := &fakeSetter{}
	if err := NewWriter(s).SetFix(context.Background(), "", "allanime"); err == nil {
		t.Fatal("want error for empty animeID")
	}
}
```

Run: `cd services/catalog && go test ./internal/service/sourceranking/ -run TestWriter 2>&1 | tail`. Expect FAIL (undefined `NewWriter`).

- [ ] **Step 4: Implement `writer.go`:**

```go
package sourceranking

import (
	"context"
	"errors"
	"time"
)

// srcfixTTL bounds a same-day override; the daily Stage 2b recompute supersedes
// it, and a bad fix self-heals when the next client's fallback overwrites it.
const srcfixTTL = 24 * time.Hour

// knownProviders is the allowlist a srcfix value must match. It mirrors the
// frontend CURATED_TIER ids (providerRegistry.ts) + the EN scraper ids the
// player can resolve. Rejecting anything else keeps garbage out of the override
// key (the write is public + unauthenticated). Keep in sync with the registry.
var knownProviders = map[string]struct{}{
	"ae": {}, "allanime": {}, "gogoanime": {}, "miruro": {}, "animepahe": {},
	"animefever": {}, "nineanime": {}, "animekai": {}, "kodik": {}, "raw": {},
	"18anime": {}, "animelib": {}, "hanime": {},
}

// stringSetter is the narrow cache surface the writer needs (a string SET with
// TTL). *cache.RedisCache is adapted to this in main.go.
type stringSetter interface {
	SetString(ctx context.Context, key, val string, ttl time.Duration) error
}

// Writer persists the same-day srcfix override.
type Writer struct{ cache stringSetter }

func NewWriter(c stringSetter) *Writer { return &Writer{cache: c} }

// SetFix validates the provider against the known-id allowlist, then writes
// srcfix:{animeID}=provider with a 24h TTL. Unknown providers and empty animeIDs
// are rejected without writing.
func (w *Writer) SetFix(ctx context.Context, animeID, provider string) error {
	if animeID == "" {
		return errors.New("empty animeID")
	}
	if _, ok := knownProviders[provider]; !ok {
		return errors.New("unknown provider")
	}
	return w.cache.SetString(ctx, "srcfix:"+animeID, provider, srcfixTTL)
}
```

Run the writer tests → PASS.

- [ ] **Step 5: Add the `Post` handler method** to `source_ranking.go`. The handler needs a writer; extend its struct + constructor (keep the existing `Get`):

```go
// fixWriter is the subset of *sourceranking.Writer the handler needs.
type fixWriter interface {
	SetFix(ctx context.Context, animeID, provider string) error
}
```

Add `writer fixWriter` to `SourceRankingHandler` and a param to `NewSourceRankingHandler(reader rankReader, writer fixWriter, log *logger.Logger)`. Add:

```go
// Post handles POST /api/anime/{animeId}/source-fix with body {"provider":"..."}.
// Records a same-day same-anime override after a client-side fallback rescued a
// failed resolve. Public + unauthenticated (the watch page is public); the
// writer validates the provider against an allowlist. 204 on success, 400 on a
// bad body or rejected provider.
func (h *SourceRankingHandler) Post(w http.ResponseWriter, r *http.Request) {
	animeID := chi.URLParam(r, "animeId")
	var body struct {
		Provider string `json:"provider"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&body); err != nil {
		httputil.Error(w, errors.BadRequest("invalid body"))
		return
	}
	if err := h.writer.SetFix(r.Context(), animeID, body.Provider); err != nil {
		if h.log != nil {
			h.log.Warnw("source-fix rejected", "anime_id", animeID, "provider", body.Provider, "error", err)
		}
		httputil.Error(w, errors.BadRequest("invalid provider"))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
```

Add imports: `encoding/json`, `io`, and confirm the catalog `errors` package import path + that it has a `BadRequest` constructor (read another catalog handler — e.g. one returning 400 — to match the exact helper name; if it's `errors.BadRequest`/`errors.Invalid`/`errors.Validation`, use whichever exists). `httputil.Error` is already imported.

- [ ] **Step 6: Handler tests** in `source_ranking_test.go` — update the existing `TestSourceRankingHandler_OK` constructor call to pass a fake writer (add a `fakeFixWriter`), and add:

```go
type fakeFixWriter struct {
	called   bool
	animeID  string
	provider string
	err      error
}

func (f *fakeFixWriter) SetFix(_ context.Context, animeID, provider string) error {
	f.called, f.animeID, f.provider = true, animeID, provider
	return f.err
}

func TestSourceFixHandler_OK(t *testing.T) {
	fw := &fakeFixWriter{}
	h := NewSourceRankingHandler(fakeRankReader{}, fw, nil)
	r := chi.NewRouter()
	r.Post("/api/anime/{animeId}/source-fix", h.Post)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/anime/uuid-1/source-fix", strings.NewReader(`{"provider":"allanime"}`))
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d", rec.Code)
	}
	if !fw.called || fw.animeID != "uuid-1" || fw.provider != "allanime" {
		t.Errorf("writer got called=%v anime=%q provider=%q", fw.called, fw.animeID, fw.provider)
	}
}

func TestSourceFixHandler_BadProvider(t *testing.T) {
	fw := &fakeFixWriter{err: errors.New("unknown provider")} // import stdlib "errors" in the test
	h := NewSourceRankingHandler(fakeRankReader{}, fw, nil)
	r := chi.NewRouter()
	r.Post("/api/anime/{animeId}/source-fix", h.Post)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/anime/uuid-1/source-fix", strings.NewReader(`{"provider":"x"}`))
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rec.Code)
	}
}
```

Add imports to the test file as needed (`strings`, stdlib `errors`). Run the handler tests → PASS.

- [ ] **Step 7: Register the route** in `router.go`. Beside the existing `r.Get("/{animeId}/source-ranking", sourceRankingHandler.Get)` (nil-guarded), add:

```go
				r.Post("/{animeId}/source-fix", sourceRankingHandler.Post)
```

(inside the same `if sourceRankingHandler != nil {` block).

- [ ] **Step 8: Wire `main.go`.** The Stage 2b wiring built `rankCacheAdapter{c: redisCache, log: log}` implementing `GetString`. Add a `SetString` method to that SAME adapter (so it satisfies both `stringGetter` and `stringSetter`):

```go
func (a rankCacheAdapter) SetString(ctx context.Context, key, val string, ttl time.Duration) error {
	return a.c.Client().Set(ctx, key, val, ttl).Err()
}
```

Then build the writer and pass it to the handler:

```go
	sourceRankingReader := sourceranking.NewReader(rankCacheAdapter{c: redisCache, log: log})
	sourceRankingWriter := sourceranking.NewWriter(rankCacheAdapter{c: redisCache, log: log})
	sourceRankingHandler := handler.NewSourceRankingHandler(sourceRankingReader, sourceRankingWriter, log)
```

(Confirm `time` is imported in main.go — it almost certainly is; add if not.)

- [ ] **Step 9: Verify + commit.**

Run: `cd services/catalog && go build ./... && go test ./internal/handler/ ./internal/service/sourceranking/ 2>&1 | tail` (all pass); `go vet ./internal/handler/ ./internal/service/sourceranking/`.

```bash
git add services/catalog/internal/service/sourceranking/ services/catalog/internal/handler/source_ranking.go services/catalog/internal/handler/source_ranking_test.go services/catalog/internal/transport/router.go services/catalog/cmd/catalog-api/main.go
git commit -m "feat(catalog): srcfix same-day override write + merge into source-ranking"
```

---

## Task 2: Frontend — types, API client, ranking→order helper, pickSmartDefault dedup

**Files:** `frontend/web/src/types/sourceRanking.ts` (create), `src/api/client.ts` (modify), `src/composables/unifiedPlayer/rankingOrder.ts` (create) + `.spec.ts`, `src/composables/unifiedPlayer/smartDefault.ts` (modify) + `.spec.ts`

- [ ] **Step 1: Types.** Create `frontend/web/src/types/sourceRanking.ts`:

```ts
/** One provider's learned-reliability record (mirrors catalog sourceranking.Record JSON). */
export interface SourceRecord {
  provider: string
  score: number
  reached_rate: number
  ok_rate: number
  p95_ms: number
  stall_rate: number
  samples: number
}

/** GET /api/anime/{id}/source-ranking payload (the {success,data} `data`). */
export interface SourceRanking {
  global: SourceRecord[]
  perAnime: SourceRecord[]
  /** Same-day override provider id (srcfix), or '' when none. */
  fix: string
}
```

- [ ] **Step 2: API client.** In `src/api/client.ts`, add a `catalogApi` (or extend the existing anime API object — match the file's existing export style; the scraper/raw/ae APIs are standalone `export const xApi = {...}` objects, so add a sibling):

```ts
export const catalogApi = {
  getSourceRanking: (animeId: string) =>
    apiClient.get<{ success: boolean; data: import('@/types/sourceRanking').SourceRanking }>(
      `/anime/${animeId}/source-ranking`,
    ),
  postSourceFix: (animeId: string, provider: string) =>
    apiClient.post(`/anime/${animeId}/source-fix`, { provider }),
}
```

(If the file already imports the `SourceRanking` type at top, use a normal import instead of the inline `import('...')`. Match the file's conventions.)

- [ ] **Step 3: Write the failing ranking-order test.** Create `frontend/web/src/composables/unifiedPlayer/rankingOrder.spec.ts`:

```ts
import { describe, it, expect } from 'vitest'
import { rankingToOrder } from './rankingOrder'
import type { SourceRanking } from '@/types/sourceRanking'

const rec = (provider: string, score: number) =>
  ({ provider, score, reached_rate: 0, ok_rate: 0, p95_ms: 0, stall_rate: 0, samples: 0 })

describe('rankingToOrder', () => {
  it('orders fix → perAnime → global, deduped, fix first', () => {
    const r: SourceRanking = {
      fix: 'kodik',
      perAnime: [rec('allanime', 0.9), rec('kodik', 0.5)],
      global: [rec('miruro', 0.8), rec('allanime', 0.7)],
    }
    expect(rankingToOrder(r)).toEqual(['kodik', 'allanime', 'miruro'])
  })

  it('handles empty ranking', () => {
    expect(rankingToOrder({ fix: '', perAnime: [], global: [] })).toEqual([])
  })

  it('skips empty fix', () => {
    const r: SourceRanking = { fix: '', perAnime: [rec('ae', 1)], global: [] }
    expect(rankingToOrder(r)).toEqual(['ae'])
  })
})
```

Run: `cd frontend/web && bunx vitest run src/composables/unifiedPlayer/rankingOrder.spec.ts 2>&1 | tail` → FAIL (no module).

- [ ] **Step 4: Implement `rankingOrder.ts`:**

```ts
import type { SourceRanking } from '@/types/sourceRanking'

/**
 * Flattens a SourceRanking into a deduped, best-first provider-id order:
 * fix (same-day override) → per-anime ranking → global ranking. Records are
 * assumed already sorted best-first by the backend. The result is meant to be
 * PREPENDED to CURATED_TIER and passed to pickSmartDefault (which filters to
 * active rows + applies the availability probe). First occurrence wins.
 */
export function rankingToOrder(r: SourceRanking | null | undefined): string[] {
  if (!r) return []
  const out: string[] = []
  const seen = new Set<string>()
  const push = (id: string) => {
    if (id && !seen.has(id)) {
      seen.add(id)
      out.push(id)
    }
  }
  push(r.fix)
  for (const rec of r.perAnime ?? []) push(rec.provider)
  for (const rec of r.global ?? []) push(rec.provider)
  return out
}
```

Run the test → PASS.

- [ ] **Step 5: Dedup `pickSmartDefault`.** In `smartDefault.ts`, the `ordered` array can now contain duplicates (the ranking prefix overlaps `CURATED_TIER`). Dedup the curated walk so a gated id (e.g. `ae`) is never probed twice. Replace the `ordered` construction:

```ts
  const seen = new Set<string>()
  const ordered: string[] = []
  for (const id of curated) {
    if (activeIds.has(id) && !seen.has(id)) { seen.add(id); ordered.push(id) }
  }
  for (const r of rows) {
    if (r.state === 'active' && !seen.has(r.def.id)) { seen.add(r.def.id); ordered.push(r.def.id) }
  }
```

(This preserves behavior for the existing single-`CURATED_TIER` callers and makes the prefixed call safe. The rest of the function — the `for (const id of ordered)` probe loop — is unchanged.)

- [ ] **Step 6: Dedup test.** Add to `smartDefault.spec.ts` (create the file if it doesn't exist; import the existing types/fakes the same way other unified-player specs do — check a sibling spec for the `ProviderRow` fake shape):

```ts
it('does not probe a gated id twice when curated has duplicates', async () => {
  let probes = 0
  const rows = [
    { def: { id: 'ae' }, state: 'active' },
    { def: { id: 'allanime' }, state: 'active' },
  ] as any
  const id = await pickSmartDefault(rows, ['ae', 'allanime', 'ae'], {
    needsCheck: new Set(['ae']),
    isAvailable: async () => { probes++; return false },
  })
  expect(probes).toBe(1)      // ae probed once despite appearing twice
  expect(id).toBe('allanime') // ae unavailable → next
})
```

Run: `cd frontend/web && bunx vitest run src/composables/unifiedPlayer/smartDefault.spec.ts src/composables/unifiedPlayer/rankingOrder.spec.ts 2>&1 | tail` → PASS. Then `bunx tsc --noEmit 2>&1 | grep -iE "sourceRanking|rankingOrder|smartDefault|client.ts" || echo "no type errors in touched files"`.

- [ ] **Step 7: Commit.**

```bash
git add frontend/web/src/types/sourceRanking.ts frontend/web/src/api/client.ts frontend/web/src/composables/unifiedPlayer/rankingOrder.ts frontend/web/src/composables/unifiedPlayer/rankingOrder.spec.ts frontend/web/src/composables/unifiedPlayer/smartDefault.ts frontend/web/src/composables/unifiedPlayer/smartDefault.spec.ts
git commit -m "feat(player): source-ranking types + API + ranking→order helper + pickSmartDefault dedup"
```

---

## Task 3: Frontend — wire the ranking into UnifiedPlayer (default + srcfix + playback-time fallback)

**Files:** `frontend/web/src/components/player/unified/UnifiedPlayer.vue`

**Context:** This is the integration. Read the existing anchors before editing (line numbers approximate — locate by content):
- Smart-default watcher `watch([rows, preferenceSettled], …)` (~491–509) — calls `pickSmartDefault(rows.value, CURATED_TIER, {needsCheck: AE_NEEDS_CHECK, isAvailable: isProviderAvailable})`.
- Resolve-time dead-source fallback (~887–902) — on `isNotAvailable`, one-shot `pickSmartDefault(rows.filter(!== failed), …)` + toast.
- `recordPlayerEvent` media_fatal site (~1207–1220) and first-frame `reachedReported` site (~1169–1183) and `resolveStartedAt`.
- Imports block (`useToast`, `recordPlayerEvent`, `CURATED_TIER`, `pickSmartDefault`).

- [ ] **Step 1: Imports + refs.** Add near the other unified-player imports:

```ts
import { catalogApi } from '@/api/client'
import { rankingToOrder } from '@/composables/unifiedPlayer/rankingOrder'
import type { SourceRanking } from '@/types/sourceRanking'
```

Add a ref to hold the fetched order (near `preferenceSettled`):

```ts
const rankingOrder = ref<string[]>([])
```

- [ ] **Step 2: Fetch the ranking once on mount.** Add an `onMounted` (or extend an existing one) that fetches the ranking and fills `rankingOrder`. Must never throw into the player:

```ts
onMounted(async () => {
  try {
    const resp = await catalogApi.getSourceRanking(props.animeId)
    const data = (resp.data?.data ?? null) as SourceRanking | null
    rankingOrder.value = rankingToOrder(data)
  } catch {
    rankingOrder.value = [] // advisory only — never block playback
  }
})
```

(If `onMounted` isn't imported, add it to the `vue` import. If there's already an `onMounted`, add the try/catch block inside it.)

- [ ] **Step 3: Prepend the ranking in the smart-default watcher.** Change the watcher to depend on `rankingOrder` too, and pass the prefixed curated order:

```ts
watch(
  [rows, preferenceSettled, rankingOrder],
  () => {
    if (state.combo.value.provider) return
    if (!preferenceSettled.value) return // saved-combo restore wins
    void pickSmartDefault(rows.value, [...rankingOrder.value, ...CURATED_TIER], {
      needsCheck: AE_NEEDS_CHECK,
      isAvailable: isProviderAvailable,
    }).then((id) => {
      if (id && !state.combo.value.provider &&
          rows.value.some((r) => r.def.id === id && r.state === 'active')) {
        state.setProvider(id, '')
      }
    })
  },
  { immediate: true },
)
```

- [ ] **Step 4: Extract a one-shot fallback helper + write srcfix.** Today the resolve-time fallback is inline at ~887–902. Refactor it into a local async function so the playback-time path can reuse it, and POST the working provider as `srcfix` on success. Add (near the other player functions):

```ts
// Switches to the next-best still-active source after the current one failed,
// at most once per episode. Records the rescued provider as a same-day srcfix so
// other viewers of this anime get it first. Returns true if it switched.
async function fallbackToNextSource(reason: 'resolve' | 'playback'): Promise<boolean> {
  const failed = state.combo.value.provider
  const next = await pickSmartDefault(
    rows.value.filter((r) => r.def.id !== failed),
    [...rankingOrder.value, ...CURATED_TIER],
    { needsCheck: AE_NEEDS_CHECK, isAvailable: isProviderAvailable },
  )
  if (!next) return false
  toast.push(
    reason === 'resolve'
      ? t('player.sourceFallback.resolve')
      : t('player.sourceFallback.playback'),
    'info',
    5000,
  )
  providerWasFromSavedCombo = false
  state.setProvider(next, '') // provider watcher re-runs loadEpisodesAndStream
  // Fire-and-forget srcfix; never block on it.
  catalogApi.postSourceFix(props.animeId, next).catch(() => undefined)
  return true
}
```

Then replace the inline resolve-time fallback block (the `if (!savedSourceFallbackDone && providerWasFromSavedCombo) { … }`) with a call:

```ts
if (isNotAvailable) {
  if (!savedSourceFallbackDone && providerWasFromSavedCombo) {
    savedSourceFallbackDone = true
    if (await fallbackToNextSource('resolve')) return
  }
  sourceError.value = t('player.sourceFallback.unavailable') // or keep existing literal
}
```

(Preserve the exact existing guard variables `savedSourceFallbackDone` / `providerWasFromSavedCombo` and the early `return`. If `t` (i18n) isn't already in scope, use `const { t } = useI18n()` — check how the component already does i18n; if it currently uses string literals, you MAY keep literals for the existing line and only use `t()` for the new keys, but prefer wiring `useI18n` if it's already imported elsewhere in the file.)

- [ ] **Step 5: Playback-time one-shot auto-switch.** At the `media_fatal` `recordPlayerEvent` site (~1207–1220), AFTER recording telemetry, add a one-shot-per-episode auto-switch — but ONLY when playback had actually started (`reachedReported`), so we don't double-handle the resolve-time path:

```ts
// (existing recordPlayerEvent media_fatal call stays)
if (reachedReported && !playbackFallbackDone) {
  playbackFallbackDone = true
  void fallbackToNextSource('playback')
}
```

Declare the guard alongside the other per-episode latches (where `savedSourceFallbackDone` lives): `let playbackFallbackDone = false`. Reset it wherever `savedSourceFallbackDone`/`reachedReported` are reset on episode/provider change (find that reset site — search for `savedSourceFallbackDone = false` or the episode-change handler — and add `playbackFallbackDone = false` beside it). If `reachedReported` is reset on provider switch, the new source can itself fall back once if it also dies, giving single-hop-per-source behavior (bounded by the shrinking active set since `fallbackToNextSource` excludes the failed id and writes a fresh srcfix).

- [ ] **Step 6: Verify (no Chrome smoke unless owner asks — DS-NF-06).**

```bash
cd frontend/web
bunx vitest run src/composables/unifiedPlayer/ 2>&1 | tail
bunx tsc --noEmit 2>&1 | grep -i UnifiedPlayer || echo "no UnifiedPlayer type errors"
bunx eslint src/components/player/unified/UnifiedPlayer.vue 2>&1 | tail
```

All clean. (UnifiedPlayer has no direct unit test; the extracted helpers are covered by Task 2's specs. Cascade-sensitive styling is untouched, so no Tailwind-cascade caveat applies.)

- [ ] **Step 7: Commit.**

```bash
git add frontend/web/src/components/player/unified/UnifiedPlayer.vue
git commit -m "feat(player): ranking-aware smart default + srcfix on fallback + playback-time auto-switch"
```

---

## Task 4: i18n keys (en/ru/ja) + user changelog

**Files:** `frontend/web/src/locales/{en,ru,ja}.json`, `frontend/web/public/changelog.json`

**Context:** The 3-locale rule is build-enforced at `make redeploy-web` (i18n-lint fails on any key missing from en/ru/ja). Add the same keys to ALL THREE.

- [ ] **Step 1: Add `player.sourceFallback.*` to all three locales.** Find the existing `player.*` namespace in each file (or add it) and insert:

`en.json`:
```json
"player": {
  "sourceFallback": {
    "resolve": "The source you picked isn't available right now — switching to the best working one.",
    "playback": "This source stopped working — switching to another.",
    "unavailable": "This source isn't available yet."
  }
}
```

`ru.json`:
```json
"player": {
  "sourceFallback": {
    "resolve": "Выбранный источник сейчас недоступен — переключаемся на лучший рабочий.",
    "playback": "Источник перестал работать — переключаемся на другой.",
    "unavailable": "Этот источник пока недоступен."
  }
}
```

`ja.json`:
```json
"player": {
  "sourceFallback": {
    "resolve": "選択したソースは現在利用できません。最適なソースに切り替えます。",
    "playback": "このソースが停止しました。別のソースに切り替えます。",
    "unavailable": "このソースはまだ利用できません。"
  }
}
```

(If a `player` object already exists in each file, MERGE `sourceFallback` into it — don't create a duplicate `player` key. Match each file's existing nesting/indentation. If the keys used in UnifiedPlayer differ from `player.sourceFallback.*`, use whatever the component actually references — keep them identical across all three files.)

- [ ] **Step 2: Run the i18n lint gate locally** (it is the redeploy prerequisite):

```bash
cd frontend/web && bash scripts/i18n-lint.sh 2>&1 | tail
```

Expect no missing-key errors for the new keys.

- [ ] **Step 3: Changelog (Trump-mode RU).** Prepend a user-facing entry to the current date group in `frontend/web/public/changelog.json` (follow the 2026-05-19 gold-standard style — bombastic, ALL-CAPS key words, signature closer, emojis). Example entry text:

```
🎯 УМНЫЙ ВЫБОР ИСТОЧНИКА — ТЕПЕРЬ ПЛЕЕР САМ выбирает САМЫЙ РАБОЧИЙ источник для каждого аниме, опираясь на РЕАЛЬНУЮ статистику запусков. Не работает — МГНОВЕННО переключаемся на лучший запасной, и запоминаем это для остальных. Ваш сохранённый выбор НИКТО не трогает. ВЕЛИКОЛЕПНО. Никто другой так не делает!
```

(Match the JSON shape of the existing newest group — same fields, valid JSON. Validate with `node -e "JSON.parse(require('fs').readFileSync('frontend/web/public/changelog.json','utf8'))"`.)

- [ ] **Step 4: Commit.**

```bash
git add frontend/web/src/locales/en.json frontend/web/src/locales/ru.json frontend/web/src/locales/ja.json frontend/web/public/changelog.json
git commit -m "i18n(player): source-fallback strings (en/ru/ja) + changelog"
```

---

## Final verification

- [ ] **Backend:** `cd services/catalog && go build ./... && go test ./internal/handler/ ./internal/service/sourceranking/ && go vet ./...` — green.
- [ ] **Frontend:** `cd frontend/web && bunx vitest run src/composables/unifiedPlayer/ && bunx tsc --noEmit && bunx eslint src/components/player/unified/UnifiedPlayer.vue src/composables/unifiedPlayer/ && bash scripts/i18n-lint.sh && bash scripts/design-system-lint.sh` — all pass (design-lint: no off-palette/hex/weight regressions; we added no styling).
- [ ] **Deploy** (build from an origin/main worktree — the shared tree is stale; `redeploy.sh` self-heals `.env`, compose project stays `docker`): redeploy `catalog`, then `web` (CLEAN from the worktree). Order: catalog first (endpoint exists) then web (calls it).
- [ ] **End-to-end smoke (prove the loop):**
  1. `POST /api/anime/<real-uuid>/source-fix` `{"provider":"allanime"}` → 204; then `curl /api/anime/<uuid>/source-ranking` shows `"fix":"allanime"`.
  2. `POST` with `{"provider":"garbage"}` → 400; ranking `fix` unchanged.
  3. (If the owner wants a Chrome checkup — opt-in per DS-NF-06): open a watch page with no saved combo and confirm the auto-picked provider follows the ranking; kill the source and confirm the one-shot switch toast.
- [ ] **Changelog** shipped (Trump-mode). Update memory `project_smart_source_selection.md`: Stage 2c shipped (ranking-aware default + srcfix + playback fallback); the whole Smart Source Selection feature is complete.

## Spec coverage (this plan)

| Spec / 2c-outline requirement | Covered by |
|---|---|
| Fold `dailyTop` + `srcfix` into the decision order (saved → override → daily top → curated) | Task 2 (`rankingToOrder`) + Task 3 (Step 3 prefix) |
| Never override a manually-saved choice | Task 3 (watcher gated on `preferenceSettled`; saved-combo restore unchanged) |
| Same-day `srcfix` written on resolve-time fallback success | Task 1 (write endpoint) + Task 3 (Step 4 `fallbackToNextSource` POST) |
| Reader merges `srcfix` ahead of the daily top | Task 1 (reader `Fix`) + Task 2 (`rankingToOrder` puts fix first) |
| Staged playback-time fallback (spec §7, conservative) | Task 3 (Step 5 one-shot single-hop auto-switch; multi-hop silent advance deferred) |
| User-facing payoff visible | Task 4 (changelog + toasts) |

## Assumptions to confirm during implementation

- **Catalog `errors.BadRequest`** helper name — Task 1 Step 5 flags reading a sibling handler to match the exact constructor.
- **`useI18n`/`t` availability** in UnifiedPlayer — Task 3 Step 4 flags checking; keep literals if i18n isn't already wired, but prefer `t()`.
- **`reachedReported` reset site** — Task 3 Step 5 needs the per-episode/per-provider latch reset location; add `playbackFallbackDone` reset beside it.
- **`onMounted` import** — add to the `vue` import if absent.
- **`apiClient.post`/`.get` envelope** — `getSourceRanking` returns `{success,data}`; read `resp.data.data`.

## Effort & Impact (per `.planning/CONVENTIONS.md`)

- **UXΔ = +3 (Better)** — the visible payoff: the player now defaults to the source that actually works for each title, auto-rescues a dead source, and learns within the day — all without touching a user's saved choice.
- **CDI = 0.05 * 21** — moderate spread (catalog endpoint + 3 FE units + the hot UnifiedPlayer + i18n), low shift (additive; the watch-combo resolver and saved-combo path are untouched; `pickSmartDefault` signature unchanged).
- **MVQ = Phoenix 90%/88%** — completes the Smart Source Selection arc by closing the learn→rank→apply→re-learn loop; slop-resistant (pure helpers unit-tested; backend write validated + tested; the risky UX is bounded to a single-hop opt-in-feeling switch).
