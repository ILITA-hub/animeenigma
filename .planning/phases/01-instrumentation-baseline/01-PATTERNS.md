# Phase 1: Instrumentation Baseline - Pattern Map

**Mapped:** 2026-04-27
**Files analyzed:** 18 (10 new, 8 modified)
**Analogs found:** 13 / 18
**No-analog (new infra):** 5 (vitest config + 3 vitest specs + anonId util — minor/templated)

This map sits between the locked decisions in `01-CONTEXT.md` (D-01..D-17) and `01-RESEARCH.md` Patterns 1–7. Where research already supplies a synthesized pattern, this doc adds the verified analog file:line and a copy-paste-ready excerpt. Where research deviates from CONTEXT, the gateway routing fix (Critical Finding 1) and resolver-anon enablement (Critical Finding 3) are reflected here as MODIFIED files.

## File Classification

### Files to CREATE

| File | Role | Data Flow | Closest Analog | Match Quality |
|------|------|-----------|----------------|---------------|
| `frontend/web/src/composables/useOverrideTracker.ts` | composable | event-driven | `frontend/web/src/composables/useWatchPreferences.ts` | role-match (different data flow — tracker emits, doesn't fetch) |
| `frontend/web/src/utils/anonId.ts` | utility | request-response (helper) | `frontend/web/src/composables/useImageProxy.ts` (sessionStorage helper section) | partial (localStorage idempotent get-or-create — no exact analog) |
| `frontend/web/src/composables/useOverrideTracker.test.ts` | test (unit) | — | NEW INFRA — no Vitest specs in repo today | no analog |
| `frontend/web/src/views/Anime.test.ts` | test (unit) | — | NEW INFRA — no Vue component tests in repo today | no analog |
| `frontend/web/tests/e2e/combo-override.spec.ts` | test (e2e) | request-response | `frontend/web/e2e/player.spec.ts` (note: actual dir is `e2e/`, not `tests/e2e/`) | exact |
| `frontend/web/vitest.config.ts` | config | — | NEW INFRA — `frontend/web/vite.config.ts` exists but no vitest config | no analog (template from research) |
| `services/player/internal/handler/override.go` | handler | request-response | `services/player/internal/handler/preference.go` | exact |
| `services/player/internal/handler/override_test.go` | test (unit) | — | `services/player/internal/handler/sync_test.go` and `report_test.go` | exact |
| `services/player/internal/transport/optional_auth.go` | middleware | request-response | `services/player/internal/transport/router.go:138-160` (`AuthMiddleware`) | role-match (must NOT reject on missing token — inverted control flow) |
| `services/player/internal/transport/optional_auth_test.go` | test (unit) | — | `services/gateway/internal/transport/router_test.go` (only existing transport test) | role-match |

### Files to MODIFY

| File | Role | Modification Type | Pattern Source | Match Quality |
|------|------|-------------------|----------------|---------------|
| `libs/metrics/watch.go` | metric def | add `CounterVec`s | self — `TranslationSelectionsTotal` lines 33-39 | exact (in-file mirror) |
| `services/player/internal/transport/router.go` | route registration | add public route group | `router.go:120-132` (anime reviews public+protected sub-route) | exact |
| `services/player/internal/service/preference.go` | service | add metric increment | self — line 66 already increments `PreferenceResolutionTotal` | exact (in-file mirror) |
| `services/player/internal/handler/preference.go` | handler | drop hard-401, accept anon | `handler/preference.go:41-45` (replace `Unauthorized` early-return with `userID := ""` fallback) | role-match |
| `services/gateway/internal/transport/router.go` | route registration | add public proxy line | `router.go:117-122` (anime reviews public proxy lines) | exact |
| `frontend/web/src/composables/useWatchPreferences.ts` | composable | drop auth short-circuit | self — line 26 `if (!authStore.isAuthenticated...) return` | exact (in-file edit) |
| `frontend/web/src/api/client.ts` | api client | add interceptor branch + new endpoint | self — interceptor lines 75-95; userApi block lines 228-235 | exact (in-file mirror) |
| `frontend/web/src/components/player/KodikPlayer.vue` | component | invoke composable, wrap pickers | self (current `selectEpisode` / `selectTranslation` handlers) | exact |
| `frontend/web/src/components/player/AnimeLibPlayer.vue` | component | invoke composable, wrap pickers | self | exact |
| `frontend/web/src/components/player/HiAnimePlayer.vue` | component | invoke composable, wrap pickers | self | exact |
| `frontend/web/src/components/player/ConsumetPlayer.vue` | component | invoke composable, wrap pickers | self | exact |
| `frontend/web/src/views/Anime.vue` | view | track `videoProvider` switch (player dimension) | self (player-switch site is in this file) | partial (composable used at parent scope for one dimension) |
| `docker/grafana/dashboards/preference-resolution.json` | dashboard config | add panel + row | self — existing rows/panels in same file | exact |

---

## Pattern Assignments

### `libs/metrics/watch.go` (MODIFY — metric def)

**Analog:** self (in-file). Mirror `TranslationSelectionsTotal` exactly — same `promauto.NewCounterVec` invocation, label slice convention, `Help` string format.

**Excerpt to mirror** (lines 33-47):
```go
TranslationSelectionsTotal = promauto.NewCounterVec(
    prometheus.CounterOpts{
        Name: "translation_selections_total",
        Help: "Total translation selections by users",
    },
    []string{"player", "language", "watch_type", "translation_title"},
)

PreferenceResolutionTotal = promauto.NewCounterVec(
    prometheus.CounterOpts{
        Name: "preference_resolution_total",
        Help: "Total preference resolution outcomes by tier",
    },
    []string{"tier"},
)
```

**Apply:** Add `ComboOverrideTotal` (labels: `tier, dimension, language, anon, player`) and `ComboResolveTotal` (labels: `tier, language, anon, player`) inside the same `var (...)` block, immediately after `PreferenceFallbackTotal`. Cardinality budget verified in 01-RESEARCH.md §Pattern 3 (384 + 96 series).

---

### `services/player/internal/handler/override.go` (NEW — handler, request-response)

**Analog:** `services/player/internal/handler/preference.go`

**Imports pattern** (lines 1-13):
```go
package handler

import (
    "net/http"

    "github.com/ILITA-hub/animeenigma/libs/authz"
    "github.com/ILITA-hub/animeenigma/libs/errors"
    "github.com/ILITA-hub/animeenigma/libs/httputil"
    "github.com/ILITA-hub/animeenigma/libs/logger"
    "github.com/ILITA-hub/animeenigma/services/player/internal/domain"
    "github.com/ILITA-hub/animeenigma/services/player/internal/service"
    "github.com/go-chi/chi/v5"
)
```

For `override.go` drop `domain`, `service`, `chi/v5` (no DB writes, no path params). Keep `authz`, `errors`, `httputil`, `logger`, plus add `metrics`.

**Constructor + handler shape** (lines 15-54):
```go
type PreferenceHandler struct {
    prefService *service.PreferenceService
    log         *logger.Logger
}

func NewPreferenceHandler(prefService *service.PreferenceService, log *logger.Logger) *PreferenceHandler {
    return &PreferenceHandler{prefService: prefService, log: log}
}

// ResolvePreference resolves the best watch combo for a user and anime
func (h *PreferenceHandler) ResolvePreference(w http.ResponseWriter, r *http.Request) {
    var req domain.ResolveRequest
    if err := httputil.Bind(r, &req); err != nil {
        httputil.Error(w, err)
        return
    }

    if req.AnimeID == "" {
        httputil.Error(w, errors.InvalidInput("anime_id is required"))
        return
    }
    // ...
    claims, ok := authz.ClaimsFromContext(r.Context())
    if !ok || claims == nil {
        httputil.Unauthorized(w)
        return
    }
    // ...
}
```

**Apply:** Mirror constructor + `Bind` → required-field validation → claims read → service call shape. KEY DIVERGENCE: instead of `Unauthorized()` on missing claims, fall through to `r.Header.Get("X-Anon-ID")` (research §Pattern 5 has the full template). Return `204 No Content` instead of `httputil.OK`. No `prefService` field — handler is self-contained (just metric + log).

---

### `services/player/internal/handler/override_test.go` (NEW — unit test)

**Analog:** `services/player/internal/handler/sync_test.go` (table-driven JWT/auth-state tests) and `services/player/internal/handler/report_test.go` (handler instantiation, claims fixture).

**Test setup pattern** (sync_test.go lines 1-20, 51-60):
```go
package handler

import (
    "context"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"
    "time"

    "github.com/ILITA-hub/animeenigma/libs/authz"
    "github.com/ILITA-hub/animeenigma/libs/logger"
    "github.com/ILITA-hub/animeenigma/services/player/internal/domain"
    "github.com/go-chi/chi/v5"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)
// ...
func TestSyncHandler_GetJobStatus_Unauthorized(t *testing.T) {
    // ...
    log := logger.Default()
    handler := NewSyncHandler(syncRepo, log)
    r := chi.NewRouter()
    r.Get("/api/users/import/{jobId}", handler.GetJobStatus)
    req := httptest.NewRequest("GET", "/api/users/import/job-123", nil)
```

**Claims-injection pattern** (report_test.go lines 21-30):
```go
claims := &authz.Claims{UserID: "user-1", Username: "testuser"}
report := &domain.ErrorReport{
    PlayerType:  "hianime",
    AnimeID:     "anime-123",
    // ...
}
filename := h.saveReportToDisk(claims, report)
require.NotEmpty(t, filename, "...")
```

**Apply:** Use `httptest.NewRequest` + `httptest.NewRecorder`. For auth scenarios inject claims via `authz.ContextWithClaims(req.Context(), &authz.Claims{UserID: "u-1"})`. For anon scenarios set `req.Header.Set("X-Anon-ID", "anon-abc")`. Test cases (minimum):
1. Authed user → 204, counter increments with `anon=false`.
2. Anon header → 204, `anon=true`.
3. Neither → 400 (per research's §Pattern 5 cardinality-protection rule).
4. Invalid `dimension` → 400.
5. Missing `anime_id` / `load_session_id` → 400.

Use `prometheus/client_golang/prometheus/testutil.ToFloat64(metrics.ComboOverrideTotal.WithLabelValues(...))` to assert counter delta.

---

### `services/player/internal/transport/optional_auth.go` (NEW — middleware)

**Analog:** `services/player/internal/transport/router.go:138-160` (`AuthMiddleware`)

**Excerpt to mirror** (router.go:138-160):
```go
// AuthMiddleware validates JWT tokens
func AuthMiddleware(jwtConfig authz.JWTConfig) func(http.Handler) http.Handler {
    jwtManager := authz.NewJWTManager(jwtConfig)

    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            token := httputil.BearerToken(r)
            if token == "" {
                httputil.Unauthorized(w)
                return
            }

            claims, err := jwtManager.ValidateAccessToken(token)
            if err != nil {
                httputil.Unauthorized(w)
                return
            }

            ctx := authz.ContextWithClaims(r.Context(), claims)
            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}
```

**Apply:** Inverted control flow — replace BOTH `httputil.Unauthorized(w); return` paths with `next.ServeHTTP(w, r); return` (no token → continue without claims; bad token → continue without claims). Only attach claims to context on the success path. Pattern is research §Pattern 6, exact form:
```go
func OptionalAuthMiddleware(jwtConfig authz.JWTConfig) func(http.Handler) http.Handler {
    jwtManager := authz.NewJWTManager(jwtConfig)
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            token := httputil.BearerToken(r)
            if token != "" {
                if claims, err := jwtManager.ValidateAccessToken(token); err == nil {
                    r = r.WithContext(authz.ContextWithClaims(r.Context(), claims))
                }
            }
            next.ServeHTTP(w, r)
        })
    }
}
```

**Note on placement:** Research suggests inline in `router.go`. RECOMMEND splitting into `optional_auth.go` (this file) for unit-testability without `NewRouter` setup. Keep package `transport`, exported name `OptionalAuthMiddleware`.

---

### `services/player/internal/transport/optional_auth_test.go` (NEW — middleware test)

**Analog:** `services/gateway/internal/transport/router_test.go` (only existing transport-package test, lines 1-32 for shape).

**Excerpt to mirror** (router_test.go:1-32):
```go
package transport

import (
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/ILITA-hub/animeenigma/libs/authz"
    "github.com/ILITA-hub/animeenigma/services/gateway/internal/config"
)

func TestRateLimitMiddleware_AllowsNormalTraffic(t *testing.T) {
    // ...
    inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
    })
    handler := RateLimitMiddleware(cfg)(inner)
    req := httptest.NewRequest(http.MethodGet, "/", nil)
    req.RemoteAddr = "192.168.1.1:12345"
    w := httptest.NewRecorder()
    handler.ServeHTTP(w, req)
    if w.Code != http.StatusOK {
        t.Errorf("...")
    }
}
```

**Apply:** Three table-driven cases:
1. No `Authorization` header → next handler called, `authz.ClaimsFromContext` returns `(nil, false)`.
2. Valid bearer token → next handler called, claims attached (assert `claims.UserID` matches expected).
3. Malformed/expired token → next handler still called (no rejection), claims absent.

The "next handler called" assertion uses a `bool` flag set inside `inner`. Use real `authz.NewJWTManager` with a fixed `JWTConfig{SigningKey: "test-key"}` to mint test tokens.

---

### `services/player/internal/service/preference.go` (MODIFY — service)

**Analog:** self — line 66 already increments `PreferenceResolutionTotal`.

**Excerpt** (lines 60-69):
```go
// Increment metrics
tier := "null"
if result != nil {
    tier = result.Tier
}
metrics.PreferenceResolutionTotal.WithLabelValues(tier).Inc()

return &domain.ResolveResponse{Resolved: result}, nil
```

**Apply:** Insert `ComboResolveTotal.WithLabelValues(...).Inc()` directly after the existing `PreferenceResolutionTotal` line. Compute the four labels from `result` (or sentinel `"unknown"` if nil/empty per research's `labelOrUnknown` helper). The `anon` label is `"true"` if `userID == ""` else `"false"`. Reference research §Pattern 4 for the exact label-derivation block.

---

### `services/player/internal/handler/preference.go` (MODIFY — handler)

**Analog:** self — lines 41-45 are the JWT-required block to relax.

**Current excerpt** (lines 41-47):
```go
claims, ok := authz.ClaimsFromContext(r.Context())
if !ok || claims == nil {
    httputil.Unauthorized(w)
    return
}

resp, err := h.prefService.Resolve(r.Context(), claims.UserID, &req)
```

**Apply:** Replace with:
```go
var userID string
if claims, ok := authz.ClaimsFromContext(r.Context()); ok && claims != nil {
    userID = claims.UserID
}
// userID == "" is acceptable: prefRepo.GetAnimePreference(ctx, "", id) returns
// nothing → Tier 1 + Tier 2 skipped, fall through to Tier 3+ as research §Pattern 6 confirms.
resp, err := h.prefService.Resolve(r.Context(), userID, &req)
```

**Critical:** Move route registration of `/preferences/resolve` OUTSIDE the `r.Use(AuthMiddleware(jwtConfig))` group in `router.go` and into the new `OptionalAuthMiddleware`-protected group (per Critical Finding 3 + research §Pattern 6). Leave `GetAnimePreference` and `GetGlobalPreferences` inside the JWT-required group — those still require an authenticated user (no anon equivalent of "MY saved preference for anime X").

---

### `services/player/internal/transport/router.go` (MODIFY — route registration)

**Analog:** self — `router.go:120-132` (anime reviews public+protected sub-route)

**Excerpt to mirror** (lines 119-132):
```go
// Anime reviews routes
r.Route("/anime/{animeId}", func(r chi.Router) {
    // Public routes
    r.Get("/reviews", reviewHandler.GetAnimeReviews)
    r.Get("/rating", reviewHandler.GetAnimeRating)

    // Protected routes
    r.Group(func(r chi.Router) {
        r.Use(AuthMiddleware(jwtConfig))
        r.Post("/reviews", reviewHandler.CreateOrUpdateReview)
        r.Get("/reviews/me", reviewHandler.GetUserReview)
        r.Delete("/reviews", reviewHandler.DeleteReview)
    })
})
```

**Apply:** Add a sibling route block under `r.Route("/api", ...)` (peer of `/users` and `/anime/{animeId}`):
```go
// Public preference routes (anon-friendly via OptionalAuth)
r.Route("/preferences", func(r chi.Router) {
    r.Use(OptionalAuthMiddleware(jwtConfig))
    r.Post("/resolve", preferenceHandler.ResolvePreference)
    r.Post("/override", overrideHandler.RecordOverride)
})
```

**Constructor signature change:** `NewRouter` gains an `overrideHandler *handler.OverrideHandler` parameter (alphabetically after `preferenceHandler`). `cmd/player-api/main.go` needs the matching constructor call site updated.

**Removal:** Delete `r.Post("/preferences/resolve", preferenceHandler.ResolvePreference)` from inside the `r.Route("/users", ...)` group (line 104). The other two preference GETs stay there.

---

### `services/gateway/internal/transport/router.go` (MODIFY — route registration)

**Analog:** self — lines 117-122 (anime reviews public proxy block).

**Excerpt to mirror** (lines 116-122):
```go
// Player service routes - reviews (must be before /anime/* catch-all)
r.Post("/anime/ratings/batch", proxyHandler.ProxyToPlayer)
r.Get("/anime/{animeId}/reviews", proxyHandler.ProxyToPlayer)
r.Get("/anime/{animeId}/reviews/me", proxyHandler.ProxyToPlayer)
r.Post("/anime/{animeId}/reviews", proxyHandler.ProxyToPlayer)
r.Delete("/anime/{animeId}/reviews", proxyHandler.ProxyToPlayer)
r.Get("/anime/{animeId}/rating", proxyHandler.ProxyToPlayer)
```

**Apply:** Add OUTSIDE the JWT-protected `/users/*` group (lines 148-151), in the public-routes section near line 145 (after `/activity/feed`):
```go
// Player service routes - preferences (public, OptionalAuth on player side)
r.HandleFunc("/preferences/*", proxyHandler.ProxyToPlayer)
```

A wildcard is fine — the player service's `OptionalAuthMiddleware` is the only gate that matters for this path family. Verified in research's Critical Finding 1.

---

### `frontend/web/src/composables/useOverrideTracker.ts` (NEW — composable, event-driven)

**Analog:** `frontend/web/src/composables/useWatchPreferences.ts` (closest existing composable shape)

**Excerpt to mirror — composable factory shape** (useWatchPreferences.ts:1-11, 25-44):
```ts
import { ref } from 'vue'
import { userApi } from '@/api/client'
import { useAuthStore } from '@/stores/auth'
import type { WatchCombo, ResolvedCombo } from '@/types/preference'

const CACHE_TTL = 24 * 60 * 60 * 1000 // 24 hours

export function useWatchPreferences(animeId: string) {
  const resolvedCombo = ref<ResolvedCombo | null>(null)
  const isLoading = ref(false)
  const authStore = useAuthStore()
  // ...
  async function resolve(available: WatchCombo[]) {
    if (!authStore.isAuthenticated || available.length === 0) return
    isLoading.value = true
    try {
      const { data } = await userApi.resolvePreference(animeId, available)
      // ...
    } catch (err) {
      console.error('Failed to resolve preference:', err)
    } finally {
      isLoading.value = false
    }
  }
  return { resolvedCombo, isLoading, resolve }
}
```

**Apply:** Mirror `export function use*(opts)` factory shape. The composable accepts an options object (NOT a string ID — multiple refs needed) per research §Pattern 1. Returns `{ recordPickerEvent, loadSessionId }`. Use `crypto.randomUUID()` for `loadSessionId`. Wrap network call in try/catch + swallow per the same defensive pattern as line 38-40 here, but use empty catch (instrumentation MUST never throw to caller).

Full body in research §Pattern 1 lines 360-451. Pay close attention to:
- `mountedAt = null` until `resolvedCombo.value` first transitions truthy (D-10).
- `emittedDimensions.add(dimension)` BEFORE `await emit()` (lock-pattern, prevents double-fire under network latency).
- `onUnmounted` cleanup of debounce timers.

---

### `frontend/web/src/utils/anonId.ts` (NEW — utility)

**Analog (partial):** `frontend/web/src/composables/useImageProxy.ts` lines 22-54 (sessionStorage idempotent get/set with try/catch).

**Excerpt to mirror** (useImageProxy.ts:22-28):
```ts
function isBlocked(): boolean {
  try {
    return sessionStorage.getItem(STORAGE_KEY_BLOCKED) === 'true'
  } catch {
    return false
  }
}
```

**Apply:** Single exported function `getOrCreateAnonId()`. Read localStorage key `aenig_anon_id`; if absent, mint via `crypto.randomUUID()` and store. On localStorage exception (private browsing) return ephemeral UUID without persisting. Full template in research §Pattern 7 lines 749-765 — copy verbatim.

**Cache the value in module scope** to avoid repeated localStorage hits per axios request:
```ts
let cached: string | null = null
export function getOrCreateAnonId(): string {
  if (cached) return cached
  // ... read/mint/store ...
  cached = id
  return id
}
```

---

### `frontend/web/src/api/client.ts` (MODIFY — interceptor + endpoint)

**Analog:** self — interceptor block (lines 75-95) and `userApi` block (lines 228-235).

**Interceptor excerpt** (lines 75-95):
```ts
apiClient.interceptors.request.use(
  async (config: InternalAxiosRequestConfig) => {
    if (config.url?.includes('/auth/refresh') || config.url?.includes('/auth/login')) {
      return config
    }

    let token = localStorage.getItem('token')
    if (token && isTokenExpired(token)) {
      const newToken = await doTokenRefresh()
      token = newToken
    }
    if (token && config.headers) {
      config.headers.Authorization = `Bearer ${token}`
    }
    return config
  },
  // ...
)
```

**Apply:** Extend the `if (token && config.headers) { ... } else if (config.headers) { config.headers['X-Anon-ID'] = getOrCreateAnonId() }` branch — research §Pattern 7 recommends ALWAYS setting (cheaper, harmless). Import `getOrCreateAnonId` from `@/utils/anonId` at top of file.

**userApi block excerpt** (lines 228-234):
```ts
// Watch preferences
resolvePreference: (animeId: string, available: WatchCombo[]) =>
  apiClient.post<ResolveResponse>('/users/preferences/resolve', { anime_id: animeId, available }),
getAnimePreference: (animeId: string) =>
  apiClient.get<WatchCombo & { updated_at: string }>(`/users/preferences/${animeId}`),
getGlobalPreferences: () =>
  apiClient.get<{ top_combos: (WatchCombo & { count: number })[] }>('/users/preferences/global'),
```

**Apply two changes:**
1. **URL path migration:** `'/users/preferences/resolve'` → `'/preferences/resolve'` (matches gateway move per Critical Finding 1).
2. **Add new endpoint method** at the bottom of `userApi` (or in a new exported `preferenceApi` if a cleanup pass is desired):
```ts
recordOverride: (data: {
  anime_id: string
  load_session_id: string
  dimension: 'language' | 'player' | 'team' | 'episode'
  original_combo: ResolvedCombo | null
  new_combo: Partial<WatchCombo> & { episode?: number }
  ms_since_load: number
  tier: string | null
  tier_number: number | null
  player: 'kodik' | 'animelib' | 'hianime' | 'consumet'
}) => apiClient.post('/preferences/override', data),
```

Note: `getAnimePreference` and `getGlobalPreferences` paths stay `/users/preferences/...` — those routes did NOT move (still JWT-required).

---

### `frontend/web/src/composables/useWatchPreferences.ts` (MODIFY — composable)

**Analog:** self — line 26 is the short-circuit to drop.

**Excerpt** (line 26):
```ts
async function resolve(available: WatchCombo[]) {
  if (!authStore.isAuthenticated || available.length === 0) return
```

**Apply:** Drop the `!authStore.isAuthenticated` clause. Keep the empty-`available` short-circuit:
```ts
async function resolve(available: WatchCombo[]) {
  if (available.length === 0) return
```

The X-Anon-ID header is set by the axios interceptor; the composable itself doesn't need anon awareness. The backend `ResolvePreference` handler now accepts both authed and anon (see `services/player/internal/handler/preference.go` modify entry).

---

### `frontend/web/src/components/player/{Kodik,AnimeLib,HiAnime,Consumet}Player.vue` (MODIFY — components)

**Analog:** self (each player). Each already has `selectEpisode` / `selectTranslation` / `selectServer` click handlers. Insert composable invocation in `<script setup>` and wrap the user-click entry points.

**Pattern (apply uniformly to all four):**
```ts
import { toRef } from 'vue'
import { useOverrideTracker } from '@/composables/useOverrideTracker'

// after props/state are declared:
const tracker = useOverrideTracker({
  animeId: props.animeId,
  player: 'kodik', // or 'animelib' | 'hianime' | 'consumet'
  resolvedCombo: toRef(props, 'preferredCombo'),
  currentEpisode: selectedEpisode,
})

function selectEpisode(ep: number) {
  tracker.recordPickerEvent('episode', { episode: ep })
  // ... existing body unchanged ...
}

function selectTranslation(/* ... */) {
  tracker.recordPickerEvent('team', { /* new combo subset */ })
  // ... existing body unchanged ...
}
```

**Per-player click-handler entry points** (verified in research §Pattern 2):

| Player | Episode pick | Team pick | Language toggle |
|--------|--------------|-----------|-----------------|
| Kodik (`KodikPlayer.vue`) | `selectEpisode` (~line 545) | `selectTranslation` (~line 531) | `translationType` toggle (lines 116, 128) |
| AnimeLib (`AnimeLibPlayer.vue`) | `selectEpisode` (~line 495) | `selectTranslation` (~line 511) | `translationFilter` (lines 163, 172, 181) |
| HiAnime (`HiAnimePlayer.vue`) | `selectEpisode` (~line 1036) | `selectServer` (~line 1077) | `selectedCategory` toggle |
| Consumet (`ConsumetPlayer.vue`) | `selectEpisode` (~line 657) | `selectServer` (~line 674) | (sub/dub equivalent) |

**Critical anti-pattern (research §Anti-Patterns):** auto-advance call sites must NOT funnel through `selectEpisode`. Audit each player and refactor auto-advance to call a sibling `_advanceEpisode(nextEp)` that bypasses the wrapper. HiAnime: `tryNextServer()` (line ~1071) and end-of-episode handler (line ~1264) are confirmed offenders.

---

### `frontend/web/src/views/Anime.vue` (MODIFY — view)

**Analog:** self. The `videoProvider` switching site lives at this scope (NOT inside any player component).

**Apply:** Invoke `useOverrideTracker` once at this level for the `player` dimension only. Watch `videoProvider` ref (or whatever drives the active player choice — planner verifies the exact identifier). On change-event from the user-facing player picker (NOT from auto-fallback when a parser fails), call `tracker.recordPickerEvent('player', { player: newProvider })`.

```ts
const playerSwitchTracker = useOverrideTracker({
  animeId,
  player: videoProvider.value, // initial — composable doesn't re-read this
  resolvedCombo: resolvedCombo,
  currentEpisode,
})

function onUserPickedProvider(newProvider: string) {
  playerSwitchTracker.recordPickerEvent('player', { player: newProvider })
  videoProvider.value = newProvider
}
```

The four per-player composable instances handle `episode | team | language`; this Anime.vue instance handles `player`. The `load_session_id` differs across the five composable instances on a single page, which is fine — the dashboard joins on `(tier, anon, player)` not on `load_session_id` (that label only lives in the Loki line for forensic queries).

---

### `docker/grafana/dashboards/preference-resolution.json` (MODIFY — dashboard config)

**Analog:** self. The file already has `panels` array with multiple `row` headers and stat/timeseries panels. Mirror the existing JSON shape — same `datasource.uid: "${DS_PROMETHEUS}"`, same `gridPos` math.

**Existing panel shape** (verified preference-resolution.json:1-49):
```json
{
  "panels": [
    {
      "collapsed": false,
      "gridPos": { "h": 1, "w": 24, "x": 0, "y": 0 },
      "id": 1,
      "panels": [],
      "title": "Resolution Overview (Last 7 Days)",
      "type": "row"
    },
    {
      "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
      // ...
      "targets": [{
        "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
        "expr": "sum by(tier)(increase(preference_resolution_total[7d]))",
        "legendFormat": "Tier {{tier}}",
        "refId": "A"
      }],
      "title": "Resolution Tier Distribution",
      "type": "piechart"
    },
```

**Apply:** Append a new `row` panel (`title: "Auto-Pick Override Rate (Phase 1 Baseline)"`) and three child panels: a stat for global rate, a timeseries by `dimension`, and a timeseries by `(player, anon)`. Full JSON in research's "Recommended — New Dashboard Panel JSON" section (lines 1028-1100+ of 01-RESEARCH.md). PromQL expressions:
- Global: `sum(rate(combo_override_total[5m])) / sum(rate(combo_resolve_total[5m]))`
- By dimension: `sum by(dimension)(rate(combo_override_total[5m])) / ignoring(dimension) group_left sum(rate(combo_resolve_total[5m]))`
- By (player, anon): `sum by(player, anon)(rate(combo_override_total[5m])) / sum by(player, anon)(rate(combo_resolve_total[5m]))`

Allocate `id`s ≥ 100 (existing IDs use 1..N+1; 100+ leaves room). Include the description string from research about "Override rate is computed against fresh resolves only..." per Pitfall 4 documentation requirement.

---

### `frontend/web/tests/e2e/combo-override.spec.ts` (NEW — e2e test)

**Note:** Path correction — actual repo dir is `frontend/web/e2e/`, NOT `frontend/web/tests/e2e/`. Place file at `frontend/web/e2e/combo-override.spec.ts`.

**Analog:** `frontend/web/e2e/player.spec.ts`

**Excerpt to mirror** (player.spec.ts:1-15):
```ts
import { test, expect } from '@playwright/test'

test.describe('Video Player', () => {
  test.describe('Kodik Player on Anime Page', () => {
    test('should display video player section', async ({ page }) => {
      await page.goto('/browse')
      await page.waitForSelector('a[href^="/anime/"]', { timeout: 10000 })

      await page.locator('a[href^="/anime/"]').first().click()
      await expect(page).toHaveURL(/\/anime\//)

      // Look for player container
      const playerContainer = page.locator('iframe, [class*="player"], [class*="video"]')
      await expect(playerContainer.first()).toBeVisible({ timeout: 10000 })
    })
```

**Apply:** Create a `test.describe('Combo Override Tracking', ...)` block. Use `page.route('**/api/preferences/override', route => { /* capture request */ route.fulfill({ status: 204 }) })` to intercept the POST and assert payload shape. Test scenarios:
1. User clicks alt episode within 10s of player load → POST fired with `dimension: 'episode'`.
2. Two clicks on different episodes within 250ms → only one POST (debounce).
3. Episode click after 31s → no POST (window closed).
4. Same dimension clicked twice with 500ms gap → only first POST emits (per-session-per-dimension lock).
5. Anon user (no login) → request includes `X-Anon-ID` header, no `Authorization`.

Reuse `ui_audit_bot` API key flow (per CLAUDE.md "UI Audit Test User") for the auth-required scenarios.

---

### `frontend/web/vitest.config.ts` and Vue/composable unit tests (NEW INFRA)

**No analog in repo.** Vitest is not currently set up (`bun run test` → no script; `package.json devDependencies` has no `vitest`).

**Implication:** Adding Vitest is a small new-infra task. Two viable options:
1. **Skip Vitest entirely; cover the composable via Playwright e2e only.** Lower scope; tests the actual integration. Recommended given research's "Phase 1 is wiring existing infrastructure together — minimal authored code."
2. **Stand up Vitest** for `useOverrideTracker` unit tests and `Anime.vue` player-switch test. Requires: `bun add -D vitest @vue/test-utils jsdom`, `vitest.config.ts` with jsdom environment, `bun run test:unit` script.

**Recommendation:** Option 1 — Vitest setup is yak-shaving for Phase 1's narrow instrumentation goal. Move the three vitest-related files OUT of phase 1 plan, document as "Vitest infrastructure is a future phase (not blocking M-01 / M-02 success criteria)."

If the planner disagrees and keeps Option 2, use the standard Vitest + Vue + jsdom setup — no project analog exists to copy, but research's vitest skeleton is sufficient.

---

## Shared Patterns

### Authentication (OptionalAuth)

**Source:** Research §Pattern 6 (synthesized from `services/player/internal/transport/router.go:138-160`).
**Apply to:** `services/player/internal/transport/optional_auth.go` (new), and the `/preferences/*` route group in `router.go`.
**Key invariant:** Missing/invalid token → continue without claims (do NOT call `httputil.Unauthorized`). Handlers downstream MUST check `ClaimsFromContext` returned `(nil, false)` and fall back to `X-Anon-ID` header read.

```go
// claims read pattern in handlers under OptionalAuth:
var userID, anonID string
if claims, ok := authz.ClaimsFromContext(r.Context()); ok && claims != nil {
    userID = claims.UserID
} else {
    anonID = r.Header.Get("X-Anon-ID")
}
if userID == "" && anonID == "" {
    httputil.Error(w, errors.InvalidInput("X-Anon-ID required for unauthenticated requests"))
    return
}
```

### Error Handling

**Source:** `libs/httputil` (already in use everywhere) — `httputil.Bind`, `httputil.Error`, `httputil.OK`, `httputil.Unauthorized`. CLAUDE.md confirms this convention.
**Apply to:** All Go handler files (`override.go`, modified `preference.go`).
**Anti-pattern:** No raw `http.Error()` calls. No bespoke JSON error shapes — `httputil.Error` produces the canonical `{"error": ...}` body that the frontend already knows how to parse.

### Frontend defensive instrumentation (never throw)

**Source:** `useWatchPreferences.ts:37-40` (existing convention).
**Apply to:** `useOverrideTracker.ts` `emit()` function.
```ts
} catch (err) {
  console.error('Failed to ...:', err)
}
```
For instrumentation, prefer empty catch (no `console.error`) — research's exact rule: "Best-effort instrumentation: never throw to caller, never block UX. Counter loss is acceptable."

### Prometheus label hygiene

**Source:** Research §Pattern 3 cardinality budget + Pitfall 3 (untrusted-label cardinality explosion).
**Apply to:** `libs/metrics/watch.go` (label set definition) AND `services/player/internal/handler/override.go` (label-emission site).
**Rule:** Every label value emitted to Prometheus MUST come from a closed set OR a `labelOrUnknown(s string) string` helper that coerces empty/unrecognized inputs to `"unknown"`. The handler validates `dimension` against an explicit whitelist `{language, player, team, episode}` and 400s on miss — this is the single defense against attacker-driven cardinality.

### Structured logging

**Source:** `libs/logger` zap usage everywhere; CLAUDE.md confirms `log.Infow(message, k, v, ...)` pattern.
**Apply to:** `services/player/internal/handler/override.go` `RecordOverride` function — emit ONE `log.Infow("combo_override", ...)` line per request, after metric increment, before 204.
**Index discipline:** Loki indexes only the auto-attached labels (`container`, `service`, `project`); zap structured fields are searchable via `| json` parser only. So labels like `anon_id`, `anime_id`, `original_combo`, `new_combo` — all safe as fields, none of them affect Loki cardinality.

### Test fixture: claims injection in handler tests

**Source:** `services/player/internal/handler/report_test.go:21-30`.
**Apply to:** `services/player/internal/handler/override_test.go` (new).
```go
claims := &authz.Claims{UserID: "user-1", Username: "testuser"}
ctx := authz.ContextWithClaims(context.Background(), claims)
req := httptest.NewRequest(...)
req = req.WithContext(ctx)
```

---

## No Analog Found

| File | Role | Reason | Mitigation |
|------|------|--------|------------|
| `frontend/web/vitest.config.ts` | config | Vitest not yet set up in repo | Recommend dropping vitest from Phase 1 (see entry above). If kept, use standard `defineConfig({ test: { environment: 'jsdom' } })`. |
| `frontend/web/src/composables/useOverrideTracker.test.ts` | unit test | No Vue unit tests exist | Same — recommend e2e-only coverage. |
| `frontend/web/src/views/Anime.test.ts` | unit test | No Vue unit tests exist | Same. |
| `frontend/web/src/utils/anonId.ts` | utility | No localStorage idempotent-mint helper exists; closest is `useImageProxy.ts` sessionStorage tracker | Use research §Pattern 7 template directly — small enough that the lack of analog is not a risk. |
| `services/player/internal/transport/optional_auth_test.go` | middleware test | Only one transport-package test exists in the entire repo (`gateway/internal/transport/router_test.go`), and it tests rate-limit, not auth | Use that file's package/import shape; assert behavior via inner-handler bool flag + `httptest.NewRecorder` per the standard Go middleware test idiom. |

---

## Metadata

**Analog search scope:**
- `frontend/web/src/composables/` (6 files)
- `frontend/web/src/api/client.ts`
- `frontend/web/e2e/` (28 specs)
- `services/player/internal/handler/` (18 files including 5 `*_test.go`)
- `services/player/internal/transport/router.go`
- `services/player/internal/service/preference.go`
- `services/gateway/internal/transport/router.go` + `router_test.go`
- `libs/metrics/watch.go`
- `libs/authz/jwt.go`
- `libs/httputil/middleware.go`
- `docker/grafana/dashboards/preference-resolution.json`

**Files scanned:** ~30
**Pattern extraction date:** 2026-04-27

**Notable analog gaps acknowledged in research:**
- Vitest infrastructure (not present)
- Optional-auth middleware (must invert existing AuthMiddleware control flow)
- Anonymous user identity (Phase 1 introduces it; no prior pattern)

**Path correction noted:** Frontend e2e tests live at `frontend/web/e2e/`, not `frontend/web/tests/e2e/` as the orchestrator brief stated. Plan should target the actual location.
