---
phase: 01-instrumentation-baseline
reviewed: 2026-04-27
depth: standard
status: issues_found
files_reviewed: 23
findings:
  critical: 1
  warning: 15
  info: 10
  total: 26
resolved_inline:
  - BL-01  # commit d098f0a: useWatchPreferences unwraps {data: {resolved}} envelope
  - WR-14  # commit d098f0a: Anime.vue dropped authStore.isAuthenticated gate around initPreferences
---

# Phase 01 — Code Review Report (instrumentation-baseline)

## Summary

Reviewed 23 files spanning the new `combo_override_total` / `combo_resolve_total` Prometheus counters, `OverrideHandler`, anonymous-friendly `ResolvePreference`, `OptionalAuthMiddleware`, the `useOverrideTracker` Vue composable, the four player components and `Anime.vue`, plus the Grafana dashboard panels.

Threats T-01-01 (identity guard), T-01-02 (cardinality whitelist), and T-01-03 (PII-free logs) are mostly enforced as designed. **However**, several real defects exist:

- **BLOCKER:** A response-shape mismatch between the backend (`httputil.OK` envelope) and the frontend `useWatchPreferences` (`data.resolved` direct read) leaves anon and authenticated callers without a resolved combo applied.
- **WR-14:** Anime.vue still gates `initPreferences` behind `authStore.isAuthenticated`, so anon users emit `combo_override_total` (numerator) without ever incrementing `combo_resolve_total` (denominator) — override-rate breaks for `anon=true`.
- **T-01-02 hardening gap:** `tier` and `player` Prometheus labels are NOT whitelisted; only `dimension` is. A hostile client can explode cardinality.
- **T-01-03 hardening gap:** `OriginalCombo` / `NewCombo` are typed as `map[string]interface{}` and logged verbatim — clients can smuggle PII into log fields under arbitrary keys.
- **T-01-03 test gap:** `TestOverride_LogsStructured` checks `entry.Message + entry.Caller` but never inspects the structured `Context` map where username would actually land. The test passes today but isn't load-bearing.
- **30 s window edge:** Window check fires before the 250 ms debounce; clicks at `t≈29.95s` actually emit at `t≈30.2s`. `msSinceLoad` for the FIRST click in a debounce burst is reported with the SECOND click's combo identity.
- **E2E stub mismatch:** `combo-override.spec.ts` returns `{ combo: ... }` but the backend wire format is `{ data: { resolved: ... } }` — combined with BL-01, the entire test suite cannot fail in the way the spec claims to verify.
- **Silent JWT degradation:** `OptionalAuthMiddleware` silently treats invalid/expired tokens as anon callers, both via `Authorization` header and via `access_token` cookie. Real users with broken sessions become silent `anon=true` increments instead of explicit 401s.

---

## BLOCKER Issues

### BL-01 — Frontend `useWatchPreferences` reads `data.resolved`; backend wraps response as `{ data: { resolved: ... } }` via `httputil.OK`

**Files:**
- `frontend/web/src/composables/useWatchPreferences.ts:31`
- `services/player/internal/handler/preference.go:56` (uses `httputil.OK`)
- `frontend/web/src/api/client.ts:239` (typed as `ResolveResponse` directly, missing envelope)

**Issue:** `apiClient.post<ResolveResponse>('/preferences/resolve', ...)` types the response as `{ resolved: ... }`, but `httputil.OK` (see `libs/httputil/response.go`) wraps the payload as `{ data: <body> }` — proven by other call sites in `client.ts` that use `response.data?.data || response.data`. Line 31 of `useWatchPreferences.ts` accesses `data.resolved` directly, which will be `undefined`.

The Wave 0 anon test (`preference_anon_test.go:108-113`) decodes via `struct { Data domain.ResolveResponse \`json:"data"\` }` proving the wire format includes the `data` envelope. The frontend doesn't unwrap it.

**Impact:** `resolvedCombo.value` is always `undefined`. The auto-pick flow in `Anime.vue` (`videoLanguage.value = combo.language`, `videoProvider.value = combo.player`) never fires. Anon support — the headline feature of plan 01-03 — silently does nothing.

**Fix:**
```ts
const { data } = await userApi.resolvePreference(animeId, available)
const envelope = (data as any).data ?? data
resolvedCombo.value = envelope.resolved
localStorage.setItem(cacheKey, JSON.stringify({
  data: envelope.resolved,
  timestamp: Date.now()
}))
```
Or update typed `userApi.resolvePreference` to return `ApiEnvelope<ResolveResponse>` and unwrap consistently across the codebase.

---

## WARNING Issues

### WR-01 — 30-second window check fires BEFORE the 250 ms debounce

**File:** `frontend/web/src/composables/useOverrideTracker.ts:73-93`

`recordPickerEvent` evaluates `msSinceLoad > WINDOW_MS` at call time, then schedules a `setTimeout` for 250 ms. The check inside the timeout never re-validates the window. A click at 29.99 s passes the gate; the POST goes out at 30.24 s — past the documented invariant.

**Fix:** Re-check the window inside the timeout using a recomputed `liveMs = performance.now() - mountedAt`.

---

### WR-02 — `msSinceLoad` reflects FIRST click in debounce burst, but `new_combo` reflects SECOND

**File:** `frontend/web/src/composables/useOverrideTracker.ts:67-94`

When a second click on the same dimension lands within 250 ms, the second's `newCombo` overwrites the first, but the captured `msSinceLoad` from the first call closes over the timeout. Forensic-data inconsistency: the two fields disagree about which click is being reported.

**Fix:** Recompute `msSinceLoad` inside the timeout (same fix as WR-01).

---

### WR-03 — `TestOverride_LogsStructured` doesn't actually inspect structured fields (T-01-03)

**File:** `services/player/internal/handler/override_test.go:228-231`

`zap.SugaredLogger.Infow` puts kv pairs into `Entry.Context` (observable via `entry.ContextMap()` — already used elsewhere in this same test on line 216). The assertion only checks `e.Message` and `e.Caller.String()`, neither of which contains `Infow`'s structured fields. Even if the handler accidentally logged `"username", req.Username`, this assertion would still pass.

**Fix:** Iterate `entry.ContextMap()` and assert (a) no value contains the username string, (b) no key matches `username|authorization|token|access_token`.

---

### WR-04 — `useWatchPreferences` reactive snapshot is fragile

**Files:** `frontend/web/src/views/Anime.vue:810-832` and `frontend/web/src/composables/useWatchPreferences.ts`

`initPreferences` calls `useWatchPreferences(animeId)`, then stores `pref.resolvedCombo.value` (snapshot) into `preferenceState.value.resolvedCombo`. The async `resolve()` wrapper manually re-copies on each call. Works today only because of the explicit re-copy in the wrapper. Combined with BL-01, even after the response shape is fixed, the snapshot pattern is brittle — any refactor that drops the re-copy silently stops applying.

**Fix:** Return the live ref directly, or add a regression test asserting `videoProvider.value` updates after the second-call resolve.

---

### WR-05 — E2E stub returns `{ combo: ... }` not `{ data: { resolved: ... } }`

**File:** `frontend/web/e2e/combo-override.spec.ts:91-97`

Combined with BL-01, the assertion `expect(override!.body.original_combo).toBeDefined()` trivially passes with `null` (because `null` is JSON-defined). Once BL-01 is fixed, the stub also needs the real shape, otherwise the test flips from one passing-but-meaningless state to another.

**Fix:**
```ts
body: JSON.stringify({ data: { resolved: STUB_RESOLVED_COMBO } })
```
And tighten the assertion: `expect(override!.body.original_combo).toMatchObject({ player: 'kodik', language: 'ru' })`.

---

### WR-06 — `OptionalAuthMiddleware` silently treats invalid JWT as anon — JWT secret rotation flips dashboards from authed to anon with no signal

**File:** `services/player/internal/transport/optional_auth.go:21-23`

When a token IS present but validation fails, the middleware falls through to anon. Metrics labels silently flip from `anon=false` to `anon=true` with no log line. Per T-01-01 the anon path is "no JWT means no JWT", not "invalid JWT becomes anon".

**Fix:** When `token != ""` and `ValidateAccessToken` fails, set `w.Header().Set("X-Token-Expired", "true")` so the existing `client.ts:110-119` interceptor refreshes the token. Optionally increment a dedicated counter to surface silent secret-rotation degradations.

---

### WR-07 — `apiClient` interceptor sets `X-Anon-ID` on every request; comment says backend ignores it but `OverrideHandler` reads it when claims are absent

**File:** `frontend/web/src/api/client.ts:92-99`

If a JWT silently fails validation in `OptionalAuthMiddleware` (WR-06), the request becomes `anon=true` AND the user's anon-id leaks alongside their real session. Trivial session-correlation in logs.

**Fix:** Either clarify the comment, or scope `X-Anon-ID` to paths that need it: `if (config.url?.startsWith('/preferences/'))`.

---

### WR-08 — CORS `Access-Control-Allow-Headers` does not include `X-Anon-ID`

**File:** `libs/httputil/middleware.go:87`

Cross-origin browser preflights from any origin other than the same one will strip the header. Today same-origin via nginx so not exploited; future subdomain / partner integration / admin embed silently breaks anon override tracking.

**Fix:** Add `X-Anon-ID` to the `Allow-Headers` list.

---

### WR-09 — `tier` and `player` Prometheus labels are NOT whitelisted (T-01-02 cardinality bomb)

**Files:** `services/player/internal/handler/override.go:88` and `libs/metrics/watch.go:59`

Only `dimension` is whitelisted. `tier` and `player` are passed through `labelOrUnknown`, which only normalizes empty strings. The cardinality budget claim in `watch.go:59` (384 series) assumes 6 distinct tiers, but a hostile client can POST `tier="xyz1"`, `tier="xyz2"`, ... and explode metric series. This is the exact T-01-02 cardinality bomb the threat model warns against.

**Fix:** Add explicit whitelists:
```go
var validTiers = map[string]bool{"per_anime": true, "user_global": true, "community": true, "pinned": true, "default": true, "unknown": true}
var validPlayers = map[string]bool{"kodik": true, "animelib": true, "hianime": true, "consumet": true, "unknown": true}
tier := labelOrUnknown(req.Tier); if !validTiers[tier] { tier = "unknown" }
player := labelOrUnknown(req.Player); if !validPlayers[player] { player = "unknown" }
```
Apply the same defense to `metrics.ComboResolveTotal` in `service/preference.go`.

---

### WR-10 — `OriginalCombo` / `NewCombo` are `map[string]interface{}` and logged verbatim — PII smuggling vector (T-01-03)

**File:** `services/player/internal/handler/override.go:14-26, 106-112`

Free-form maps are written to a structured log field unconditionally. A client could post `{"original_combo": {"email": "victim@example.com"}}` and that PII goes straight into Loki. Bounded log size and shape are not guaranteed.

**Fix:** Type the fields as a closed struct:
```go
type ComboPayload struct {
    Player           string `json:"player,omitempty"`
    Language         string `json:"language,omitempty"`
    WatchType        string `json:"watch_type,omitempty"`
    TranslationID    string `json:"translation_id,omitempty"`
    TranslationTitle string `json:"translation_title,omitempty"`
    Episode          *int   `json:"episode,omitempty"`
}
type OverrideRequest struct {
    // ...
    OriginalCombo *ComboPayload `json:"original_combo"`
    NewCombo      *ComboPayload `json:"new_combo"`
}
```

---

### WR-11 — Tracker mounts even when `route.params.id` is undefined → anime_id="undefined" pollutes metrics

**File:** `frontend/web/src/views/Anime.vue:785-798`

`route.params.id as string` casts even when undefined. Tracker subscribes to `resolvedCombo` and listens for clicks even before `loadAnimeData`. If the route resolves with no slug, every override POST has `anime_id = "undefined"`.

**Fix:** Guard early — only instantiate when `params.id` is present, or have `useOverrideTracker.emit` no-op on empty animeId.

---

### WR-12 — `crypto.randomUUID()` requires secure context; LAN-IP dev or older browsers throw

**Files:** `frontend/web/src/composables/useOverrideTracker.ts:48` and `frontend/web/src/utils/anonId.ts:24,34`

Production HTTPS is fine. But `http://192.168.x.x:5173` from a phone (LAN testing) or older Safari < 15.4 will throw, taking down the player. `anonId.ts` wraps in try/catch but the catch path also calls `crypto.randomUUID()` — can't recover from a missing API.

**Fix:** Add a Math.random RFC4122 v4 fallback (telemetry-grade, non-cryptographic).

---

### WR-13 — `recordPickerEvent` does not freeze `newCombo` at call time

**File:** `frontend/web/src/composables/useOverrideTracker.ts:67-94`

`newCombo` is captured by reference into the timeout closure. Today no caller mutates the object after passing it. Contract is fragile.

**Fix:** Snapshot inside the function: `const snapshot = { ...newCombo }`.

---

### WR-14 — Anime.vue `initPreferences` is gated behind `authStore.isAuthenticated`; anon users hit override (numerator) but never resolve (denominator)

**Files:** `frontend/web/src/views/Anime.vue:1212-1214` and `frontend/web/src/composables/useWatchPreferences.ts`

Per CONTEXT D-12, plan 01-03 made `/preferences/resolve` anon-friendly so the denominator increments for anon users. Commit `f6b21e8` dropped the auth short-circuit inside the composable, but the call site in Anime.vue still gates `initPreferences` behind `authStore.isAuthenticated`. Anon users:
- Emit `combo_override_total{anon="true",...}` (numerator) ✓
- Never emit `combo_resolve_total{anon="true",...}` (denominator) ✗

The Grafana panel "Override Rate by Language and Auth State" with `anon=true` will be NaN or unbounded.

**Fix:** Drop the auth gate around `initPreferences`:
```ts
if (fetched) {
  initPreferences(fetched.id)
}
```

---

### WR-15 — Same silent-anon-fallback as WR-06, but via `access_token` cookie

**File:** `services/player/internal/transport/optional_auth.go:25`

`httputil.BearerToken(r)` also reads the `access_token` cookie. A user with an expired cookie silently slides into `anon=true` metrics. Conflates "logged out" with "session bug" in dashboards.

**Fix:** See WR-06; emit `X-Token-Expired` when validation fails on a present token (header OR cookie).

---

## INFO Issues

### IN-01 — `TestResolve_AcceptsAnon` may not actually exercise the anon resolve path

**File:** `services/player/internal/handler/preference_anon_test.go:99-113`

Asserts non-nil `resp.Data.Resolved`, but the handler's `len(req.Available) == 0` early-return is BEFORE the auth check. Strengthen test to verify `userID=""` actually entered the resolver.

---

### IN-02 — `Anime.vue` `playerSwitchTracker` coerces `'hanime'` → `'kodik'` for the player label

**File:** `frontend/web/src/views/Anime.vue:795`

Misattributes hanime users' player switches into the kodik bucket, even though the comment claims no override fires for hanime. Add `'hanime'` to `PlayerName` or omit the tracker on the 18+ tab.

---

### IN-03 — E2E activation button selector uses substring matching on aria-labels

**File:** `frontend/web/e2e/combo-override.spec.ts:155`

Fragile to localization. Add `data-testid="activate-player"` for stability.

---

### IN-04 — `useOverrideTracker.emit` swallows ALL errors silently

**File:** `frontend/web/src/composables/useOverrideTracker.ts:118-121`

DEV builds get nothing. Add `console.warn` gated by `import.meta.env.DEV` so contract regressions are noticed during development.

---

### IN-05 — Override log line is loud and unbounded

**File:** `services/player/internal/handler/override.go:100-112`

Every accepted request including idle/dropped overrides logs at info level with `original_combo`/`new_combo` (combined with WR-10, unbounded). At scale this dominates player log volume. Move to debug, downsample, or wait until WR-10 is fixed and shapes are bounded.

---

### IN-06 — Duplicated `labelOrUnknownService` helper

**File:** `services/player/internal/service/preference.go:91-99`

Verbatim copy of `labelOrUnknown` in the handler package. Lift to `libs/metrics` to avoid drift.

---

### IN-07 — `isTokenExpired` uses `atob` without base64URL handling

**File:** `frontend/web/src/api/client.ts:38-44`

`atob(token.split('.')[1])` doesn't handle base64URL `-`/`_` characters. Pre-signed JWTs with URL-safe encoding cause `atob` to throw, falling through to `return true`, forcing unnecessary refreshes.

---

### IN-08 — `playerSwitchTracker` captures `videoProvider.value` statically at setup

**File:** `frontend/web/src/views/Anime.vue:785-798`

Label is captured at component setup, before localStorage is necessarily applied. When a user switches RU↔EN, telemetry still tags the original player. Add a comment explaining intent or pass a Ref.

---

### IN-09 — Override-rate stat panel uses `rate(...[5m])` on low-traffic system

**File:** `docker/grafana/dashboards/preference-resolution.json:602`

On low-traffic, `[5m]` shows "No Data" or NaN frequently. Widen to `[15m]` or add `clamp_min(...)` for the denominator.

---

### IN-10 — Type contract drift between `useOverrideTracker` (`WatchCombo`) and `userApi.recordOverride` (`ResolvedCombo`)

**Files:** `frontend/web/src/composables/useOverrideTracker.ts:101-117` and `frontend/web/src/api/client.ts:240-250`

`original_combo` is sent as `WatchCombo | null` though the wire type still nominally says `ResolvedCombo | null`. Backend trusts whatever shape arrives. Tighten the types so the contract is consistent.

---

## Files Reviewed

- `libs/metrics/watch.go`
- `services/player/internal/handler/override.go`
- `services/player/internal/handler/override_test.go`
- `services/player/internal/handler/preference.go`
- `services/player/internal/handler/preference_anon_test.go`
- `services/player/internal/service/preference.go`
- `services/player/internal/service/preference_resolve_combo_test.go`
- `services/player/internal/transport/optional_auth.go`
- `services/player/internal/transport/optional_auth_test.go`
- `services/player/internal/transport/router.go`
- `services/player/cmd/player-api/main.go`
- `services/gateway/internal/transport/router.go`
- `frontend/web/src/utils/anonId.ts`
- `frontend/web/src/api/client.ts`
- `frontend/web/src/composables/useOverrideTracker.ts`
- `frontend/web/src/composables/useWatchPreferences.ts`
- `frontend/web/src/components/player/KodikPlayer.vue`
- `frontend/web/src/components/player/AnimeLibPlayer.vue`
- `frontend/web/src/components/player/HiAnimePlayer.vue`
- `frontend/web/src/components/player/ConsumetPlayer.vue`
- `frontend/web/src/views/Anime.vue`
- `frontend/web/e2e/combo-override.spec.ts`
- `docker/grafana/dashboards/preference-resolution.json`

## Recommended Fix Priority

1. **BL-01** — Unwrap the `data` envelope in `useWatchPreferences` (entire anon resolve flow is non-functional without it)
2. **WR-14** — Drop `authStore.isAuthenticated` gate around `initPreferences` (denominator missing for anon)
3. **WR-09** — Whitelist `tier` and `player` Prometheus labels (T-01-02 hardening)
4. **WR-10** — Type-bound combo payloads to a closed struct (T-01-03 hardening)
5. **WR-03** — Fix the PII test to actually inspect structured fields
6. **WR-05** — Update E2E stub to real envelope shape AFTER BL-01 lands
7. **WR-01 / WR-02** — Re-evaluate `msSinceLoad` and 30 s window inside the debounce timeout
8. **WR-06 / WR-15** — On JWT validation failure in OptionalAuth, set `X-Token-Expired` header
9. **WR-08** — Add `X-Anon-ID` to CORS `Allow-Headers`
10. **WR-12** — Add a `crypto.randomUUID` fallback for non-secure-context dev / older browsers
