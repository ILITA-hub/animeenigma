# RBAC and roulette — Phase 2: gateway FeatureGate enforcement — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Turn the gateway into the runtime hard boundary for the dark-ship features (`fanfic`, `gacha`, `profile-wall`): replace their static `if cfg.*AdminOnly { AdminRoleMiddleware }` route gates with a per-request `FeatureGate(key)` middleware that resolves each caller against a cached copy of policy-service's ruleset — so an admin can grant/deny per-role and per-user at runtime, and per-user grants take effect end-to-end (the downstream services do NOT self-gate — the gateway is already the sole enforcement point).

**Architecture:** A small in-memory `rulesetCache` in the gateway (mirroring the existing `apikey_cache.go` pattern) polls policy-service `GET /internal/policy/ruleset` on a background ticker, fail-static (a failed refresh keeps the last-known-good snapshot). A `FeatureGate(key, cache)` chi middleware reads JWT claims from context and evaluates a locally-duplicated `canAccess` over the flag's audience; cold-start / unknown flag falls back to the flag's `failSafe` (admin ⇒ fail-closed, everyone ⇒ open). The 3 route groups are cut over; the `*AdminOnly` config bools + their compose env are removed.

**Tech Stack:** Go 1.25, chi/v5, `libs/{authz,httputil,logger}`, policy-service `/internal/policy/ruleset` feed (Phase 1).

## Global Constraints

- **Phase 1 is landed** on this branch (`services/policy` :8098, ruleset feed `GET /internal/policy/ruleset` returns httputil.OK-wrapped `{success, data:{rouletteEnabled, flags:{key→{roles,allowUsers,denyUsers}}, failSafe:{key→"admin"|"everyone"}, roulette:{key→bool}}}`). Gateway config already has `PolicyService` = `http://policy:8098` (P1 fix `c3f8a969`).
- **`canAccess` order (mirror policy `domain.FeatureFlag.CanAccess` EXACTLY):** guest→deny; deny-list→deny; allow-list→allow; `everyone` role→allow; role-match→allow; else deny. The gateway DUPLICATES this (does NOT import `services/policy/internal/domain`) to stay decoupled; policy is the schema source of truth — add a comment saying so.
- **Fail-static:** a failed ruleset refresh keeps the previous snapshot. **Cold start** (no successful load yet, or a flag key absent from the snapshot) falls back to the flag's `failSafe`; unknown/blank failSafe ⇒ **admin-only (fail-closed)**.
- **Day-one parity:** the P1 seed sets `fanfic`/`gacha`/`profile-wall` → `roles:[admin]`, `failSafe:"admin"`. So FeatureGate("fanfic") allows exactly the admins the old `FanficAdminOnly=true` allowed — behavior is identical on deploy. The dark-ship is now runtime-flippable per role/user instead of env+rebuild.
- **Middleware placement:** FeatureGate mounts AFTER a JWT middleware so claims are in context. `fanfic`/`gacha` keep `JWTValidationMiddleware` (login-required; "everyone" ⇒ all logged-in non-guest users). `profile-wall` uses `OptionalJWTValidationMiddleware` (so its eventual `everyone` state is public/anonymous, matching today's revealed `else` branch).
- **Admin-tool routes stay unconditionally `AdminRoleMiddleware`** (e.g. gacha admin-content API) — they are NOT feature-flagged.
- **No new service, no FE, no provider work** — all P2 changes are within `services/gateway/`, plus removing the 3 env vars from `docker/docker-compose.yml`.
- **Worktree discipline:** work in `/data/animeenigma/.claude/worktrees/rbac-and-roulette`; Write/Edit with worktree-root absolute paths (never `/data/animeenigma/...` bare — that edits the BASE tree). Commit, don't push. Co-authors on every commit:
  ```
  Co-Authored-By: Claude Code <noreply@anthropic.com>
  Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
  Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
  ```
- **Posture:** in-run gates = `cd services/gateway && go build ./... && go test ./...`. NO `make redeploy`/docker build/live curl (deploy deferred).
- **Effort metrics:** UXΔ / CDI / MVQ, never days/hours.

---

### Task 1: `rulesetCache` — in-memory snapshot + background refresh + fetcher

**Files:**
- Create: `services/gateway/internal/transport/ruleset_cache.go`
- Test: `services/gateway/internal/transport/ruleset_cache_test.go`

**Interfaces:**
- Produces:
  - `type audience struct{ Roles, AllowUsers, DenyUsers []string }` (JSON: `roles`/`allowUsers`/`denyUsers`)
  - `type rulesetSnapshot struct{ RouletteEnabled bool; Flags map[string]audience; FailSafe map[string]string; Roulette map[string]bool }`
  - `type rulesetFetchFunc func(ctx context.Context) (rulesetSnapshot, error)`
  - `newRulesetCache(fetch rulesetFetchFunc, log *logger.Logger) *rulesetCache`
  - `(*rulesetCache) snapshot() (rulesetSnapshot, bool)` — snapshot + loaded flag
  - `(*rulesetCache) refresh(ctx)` — one fetch, fail-static
  - `(*rulesetCache) Start(ctx, interval)` — immediate refresh then ticker
  - `httpRulesetFetch(policyBaseURL string, client *http.Client) rulesetFetchFunc`

- [ ] **Step 1: Write the failing test** `services/gateway/internal/transport/ruleset_cache_test.go`

```go
package transport

import (
	"context"
	"errors"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/logger"
)

func TestRulesetCache_failStatic_keepsLastGood(t *testing.T) {
	calls := 0
	fetch := func(ctx context.Context) (rulesetSnapshot, error) {
		calls++
		if calls == 1 {
			return rulesetSnapshot{Flags: map[string]audience{"fanfic": {Roles: []string{"admin"}}}}, nil
		}
		return rulesetSnapshot{}, errors.New("upstream down")
	}
	c := newRulesetCache(fetch, logger.Default())
	c.refresh(context.Background()) // success → loaded
	snap, loaded := c.snapshot()
	if !loaded || len(snap.Flags) != 1 {
		t.Fatalf("after first refresh: loaded=%v flags=%d", loaded, len(snap.Flags))
	}
	c.refresh(context.Background()) // error → keep previous
	snap, loaded = c.snapshot()
	if !loaded || snap.Flags["fanfic"].Roles[0] != "admin" {
		t.Fatalf("fail-static broken: loaded=%v snap=%+v", loaded, snap)
	}
}

func TestRulesetCache_coldStart_notLoaded(t *testing.T) {
	c := newRulesetCache(func(ctx context.Context) (rulesetSnapshot, error) {
		return rulesetSnapshot{}, errors.New("down")
	}, logger.Default())
	c.refresh(context.Background())
	if _, loaded := c.snapshot(); loaded {
		t.Fatal("cold start with only-failing fetch must stay unloaded")
	}
}
```

- [ ] **Step 2: Run — expect FAIL** (undefined `newRulesetCache`)

Run: `cd services/gateway && go test ./internal/transport/ -run TestRulesetCache -v`
Expected: FAIL — undefined.

- [ ] **Step 3: Implement** `services/gateway/internal/transport/ruleset_cache.go`

```go
package transport

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
)

// audience mirrors services/policy internal/domain.Audience (JSON shape).
// Duplicated (NOT imported) so the gateway stays decoupled from the policy
// service's internal packages; policy is the source of truth for the schema.
type audience struct {
	Roles      []string `json:"roles"`
	AllowUsers []string `json:"allowUsers"`
	DenyUsers  []string `json:"denyUsers"`
}

// rulesetSnapshot mirrors services/policy internal/domain.Ruleset.
type rulesetSnapshot struct {
	RouletteEnabled bool                `json:"rouletteEnabled"`
	Flags           map[string]audience `json:"flags"`
	FailSafe        map[string]string   `json:"failSafe"`
	Roulette        map[string]bool     `json:"roulette"`
}

// rulesetEnvelope is the httputil.OK wrapper policy-service returns.
type rulesetEnvelope struct {
	Success bool            `json:"success"`
	Data    rulesetSnapshot `json:"data"`
}

// rulesetFetchFunc fetches the current ruleset. Real impl GETs
// {policyURL}/internal/policy/ruleset; tests inject a fake.
type rulesetFetchFunc func(ctx context.Context) (rulesetSnapshot, error)

// rulesetCache holds the last-known-good ruleset in memory. Fail-static: a
// failed refresh keeps the previous snapshot. Until the first successful fetch
// loaded==false and FeatureGate falls back to each flag's failSafe.
type rulesetCache struct {
	mu     sync.RWMutex
	snap   rulesetSnapshot
	loaded bool
	fetch  rulesetFetchFunc
	log    *logger.Logger
}

func newRulesetCache(fetch rulesetFetchFunc, log *logger.Logger) *rulesetCache {
	return &rulesetCache{fetch: fetch, log: log}
}

func (c *rulesetCache) snapshot() (rulesetSnapshot, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.snap, c.loaded
}

// refresh does one fetch; on error it logs and keeps the previous snapshot.
func (c *rulesetCache) refresh(ctx context.Context) {
	snap, err := c.fetch(ctx)
	if err != nil {
		c.log.Warnw("ruleset refresh failed; keeping last-known-good", "error", err)
		return
	}
	c.mu.Lock()
	c.snap = snap
	c.loaded = true
	c.mu.Unlock()
}

// Start does an immediate refresh, then refreshes every interval until ctx is done.
func (c *rulesetCache) Start(ctx context.Context, interval time.Duration) {
	c.refresh(ctx)
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				c.refresh(ctx)
			}
		}
	}()
}

// httpRulesetFetch builds the production fetcher hitting policy-service's
// Docker-network-only ruleset feed (the gateway is on the same network).
func httpRulesetFetch(policyBaseURL string, client *http.Client) rulesetFetchFunc {
	return func(ctx context.Context) (rulesetSnapshot, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, policyBaseURL+"/internal/policy/ruleset", nil)
		if err != nil {
			return rulesetSnapshot{}, err
		}
		resp, err := client.Do(req)
		if err != nil {
			return rulesetSnapshot{}, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return rulesetSnapshot{}, fmt.Errorf("policy ruleset: status %d", resp.StatusCode)
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if err != nil {
			return rulesetSnapshot{}, err
		}
		var env rulesetEnvelope
		if err := json.Unmarshal(body, &env); err != nil {
			return rulesetSnapshot{}, err
		}
		return env.Data, nil
	}
}
```

- [ ] **Step 4: Run — expect PASS**

Run: `cd services/gateway && go test ./internal/transport/ -run TestRulesetCache -v`
Expected: PASS (2 tests).

- [ ] **Step 5: Commit**

```bash
git add services/gateway/internal/transport/ruleset_cache.go services/gateway/internal/transport/ruleset_cache_test.go
git commit -m "feat(gateway): in-memory policy ruleset cache (fail-static, background refresh)" \
  -m "Co-Authored-By: Claude Code <noreply@anthropic.com>" \
  -m "Co-Authored-By: 0neymik0 <0neymik0@gmail.com>" \
  -m "Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

### Task 2: `FeatureGate` middleware + local `canAccess`

**Files:**
- Create: `services/gateway/internal/transport/feature_gate.go`
- Test: `services/gateway/internal/transport/feature_gate_test.go`

**Interfaces:**
- Consumes: `rulesetCache`, `audience` (Task 1); `authz.UserIDFromContext`/`RoleFromContext`; `httputil.Forbidden`.
- Produces:
  - `canAccess(a audience, userID, role string) bool`
  - `FeatureGate(key string, cache *rulesetCache) func(http.Handler) http.Handler`
  - `featureAllowed(cache *rulesetCache, key, userID, role string) bool` (unexported helper the test can call directly)

- [ ] **Step 1: Write the failing test** `services/gateway/internal/transport/feature_gate_test.go`

```go
package transport

import (
	"context"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/logger"
)

func loadedCache(flags map[string]audience, failSafe map[string]string) *rulesetCache {
	c := newRulesetCache(func(ctx context.Context) (rulesetSnapshot, error) {
		return rulesetSnapshot{Flags: flags, FailSafe: failSafe}, nil
	}, logger.Default())
	c.refresh(context.Background())
	return c
}

func TestCanAccess_order(t *testing.T) {
	adminFlag := audience{Roles: []string{"admin"}}
	if !canAccess(adminFlag, "u1", "admin") {
		t.Fatal("admin should access admin flag")
	}
	if canAccess(adminFlag, "u1", "user") {
		t.Fatal("user should NOT access admin-only flag")
	}
	if !canAccess(audience{Roles: []string{"admin"}, AllowUsers: []string{"u1"}}, "u1", "user") {
		t.Fatal("allow-list should grant a non-admin")
	}
	if canAccess(audience{Roles: []string{"admin"}, AllowUsers: []string{"u1"}, DenyUsers: []string{"u1"}}, "u1", "admin") {
		t.Fatal("deny beats allow")
	}
	if !canAccess(audience{Roles: []string{"everyone"}}, "", "") {
		t.Fatal("everyone should allow anonymous")
	}
	if canAccess(audience{Roles: []string{"everyone"}}, "g1", "guest") {
		t.Fatal("guest is never granted")
	}
}

func TestFeatureAllowed_coldStart_failsafe(t *testing.T) {
	empty := newRulesetCache(func(ctx context.Context) (rulesetSnapshot, error) {
		return rulesetSnapshot{}, context.DeadlineExceeded
	}, logger.Default())
	empty.refresh(context.Background()) // stays unloaded
	// cold start, unknown flag → fail-closed to admin-only
	if featureAllowed(empty, "fanfic", "u1", "user") {
		t.Fatal("cold start must fail-closed for a non-admin")
	}
	if !featureAllowed(empty, "fanfic", "a1", "admin") {
		t.Fatal("cold start must still allow admin")
	}
}

func TestFeatureAllowed_loaded(t *testing.T) {
	c := loadedCache(
		map[string]audience{"fanfic": {Roles: []string{"admin"}, AllowUsers: []string{"oronemu"}}},
		map[string]string{"fanfic": "admin"},
	)
	if !featureAllowed(c, "fanfic", "oronemu", "user") {
		t.Fatal("allow-listed user should pass")
	}
	if featureAllowed(c, "fanfic", "rando", "user") {
		t.Fatal("non-listed user should 403")
	}
	// unknown flag with failSafe everyone in snapshot → open
	c2 := loadedCache(map[string]audience{}, map[string]string{"x": "everyone"})
	if !featureAllowed(c2, "x", "", "") {
		t.Fatal("failSafe everyone should allow anonymous for an unlisted flag")
	}
}
```

- [ ] **Step 2: Run — expect FAIL** (undefined `canAccess`/`featureAllowed`)

Run: `cd services/gateway && go test ./internal/transport/ -run 'TestCanAccess_order|TestFeatureAllowed' -v`
Expected: FAIL — undefined.

- [ ] **Step 3: Implement** `services/gateway/internal/transport/feature_gate.go`

```go
package transport

import (
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
)

// canAccess mirrors services/policy internal/domain.FeatureFlag.CanAccess
// EXACTLY (kept in sync by hand; policy is the source of truth). Order:
// guest→deny; deny-list→deny; allow-list→allow; everyone→allow; role→allow.
func canAccess(a audience, userID, role string) bool {
	if role == "guest" {
		return false
	}
	if userID != "" && audienceContains(a.DenyUsers, userID) {
		return false
	}
	if userID != "" && audienceContains(a.AllowUsers, userID) {
		return true
	}
	if audienceContains(a.Roles, "everyone") {
		return true
	}
	if role != "" && audienceContains(a.Roles, role) {
		return true
	}
	return false
}

func audienceContains(xs []string, v string) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}

// featureAllowed resolves whether (userID, role) may reach a flag-gated route.
// Loaded snapshot with the key present → evaluate its audience. Cold start or
// an unknown key → the flag's failSafe: "everyone" opens, anything else
// (incl. blank/unknown) fails closed to admin-only.
func featureAllowed(cache *rulesetCache, key, userID, role string) bool {
	snap, loaded := cache.snapshot()
	if loaded {
		if a, ok := snap.Flags[key]; ok {
			return canAccess(a, userID, role)
		}
	}
	failSafe := ""
	if loaded {
		failSafe = snap.FailSafe[key]
	}
	if failSafe == "everyone" {
		return canAccess(audience{Roles: []string{"everyone"}}, userID, role)
	}
	return canAccess(audience{Roles: []string{"admin"}}, userID, role)
}

// FeatureGate returns middleware that 403s callers not in the flag's audience.
// Mount AFTER a JWT middleware (required or optional) so claims are in context.
func FeatureGate(key string, cache *rulesetCache) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID := authz.UserIDFromContext(r.Context())
			role := string(authz.RoleFromContext(r.Context()))
			if featureAllowed(cache, key, userID, role) {
				next.ServeHTTP(w, r)
				return
			}
			httputil.Forbidden(w)
		})
	}
}
```
> **Collision check:** before implementing, `grep -n 'func audienceContains\|func contains' services/gateway/internal/transport/*.go`. If `audienceContains` already exists, reuse it; if a differently-named slice-contains helper exists, use that instead of adding a duplicate.

- [ ] **Step 4: Run — expect PASS**

Run: `cd services/gateway && go test ./internal/transport/ -run 'TestCanAccess_order|TestFeatureAllowed' -v`
Expected: PASS (3 tests).

- [ ] **Step 5: Commit**

```bash
git add services/gateway/internal/transport/feature_gate.go services/gateway/internal/transport/feature_gate_test.go
git commit -m "feat(gateway): FeatureGate middleware + local canAccess (mirrors policy resolver)" \
  -m "Co-Authored-By: Claude Code <noreply@anthropic.com>" \
  -m "Co-Authored-By: 0neymik0 <0neymik0@gmail.com>" \
  -m "Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

### Task 3: Wire the cache into the router + cut `fanfic` and `gacha` over

**Files:**
- Modify: `services/gateway/internal/transport/router.go` (`NewRouterWithCleanup`: build+start the cache; the fanfic + gacha route groups)
- Modify: `services/gateway/internal/config/config.go` (add `RulesetRefresh time.Duration`)
- Test: `services/gateway/internal/transport/router_feature_gate_test.go`

**Interfaces:**
- Consumes: `newRulesetCache`, `httpRulesetFetch`, `(*rulesetCache).Start`, `FeatureGate` (Tasks 1-2); `cfg.Services.PolicyService`.

- [ ] **Step 1: Add the refresh-interval config** — in `services/gateway/internal/config/config.go`, add to the `Config` struct `RulesetRefresh time.Duration` and in `Load()` set `RulesetRefresh: getEnvDuration("POLICY_RULESET_REFRESH", 15*time.Second),`. (Confirm a `getEnvDuration` helper exists in this config package; if not, mirror the `getEnvInt` helper's shape with `time.ParseDuration`.)

- [ ] **Step 2: Build + start the cache in `NewRouterWithCleanup`** — read `services/gateway/internal/transport/router.go` around `func NewRouterWithCleanup(` (line ~49). It already returns `(http.Handler, func())` (a cleanup). Near the top of the function body, before routes are mounted, add:

```go
	// Policy ruleset cache — polls policy-service's Docker-network-only feed and
	// backs the FeatureGate middleware. Fail-static; cold start uses per-flag
	// failSafe. The returned cleanup cancels the refresher.
	rulesetCtx, rulesetCancel := context.WithCancel(context.Background())
	featureRuleset := newRulesetCache(
		httpRulesetFetch(cfg.Services.PolicyService, &http.Client{Timeout: 5 * time.Second}),
		log,
	)
	featureRuleset.Start(rulesetCtx, cfg.RulesetRefresh)
```
Then fold `rulesetCancel()` into the returned cleanup func (call it alongside whatever the existing cleanup already cancels — if the current cleanup is a bare `func(){}` or cancels a redis limiter, add `rulesetCancel()` to it). Ensure `context`, `net/http`, and `time` are imported (some may already be).

- [ ] **Step 3: Write the failing router test** `services/gateway/internal/transport/router_feature_gate_test.go` — mirror the harness of `router_policy_test.go` (same package, stub backends, config with `PolicyService: <stub>.URL`). Because `NewRouterWithCleanup` fetches the ruleset at startup from `cfg.Services.PolicyService`, point that at a stub returning a ruleset where `fanfic` is `roles:[admin]`. Assert:

```go
// fanfic: admin JWT → 200 (reaches fanfic backend); valid non-admin JWT → 403;
// no token → 401 (JWTValidationMiddleware, before FeatureGate).
// gacha /wallet: same matrix.
```
Use the `router_policy_test.go` token-mint helper (`authz.NewJWTManager(cfg).GenerateTokenPair(id, name, role, sid)`) and the `sync.Once` shared-metrics-collector guard already established there. The stub policy backend must serve `GET /internal/policy/ruleset` with the httputil.OK envelope `{"success":true,"data":{"flags":{"fanfic":{"roles":["admin"]},"gacha":{"roles":["admin"]}},"failSafe":{"fanfic":"admin","gacha":"admin"}}}`. (Give the router a brief moment / call the cache's `refresh` deterministically if the test harness exposes the router build synchronously — `Start` does an immediate synchronous `refresh` before spawning the ticker, so the first snapshot is loaded by the time `NewRouterWithCleanup` returns.)

Run: `cd services/gateway && go test ./internal/transport/ -run FeatureGate -v`
Expected: FAIL (routes still gated by the old bool).

- [ ] **Step 4: Cut `fanfic` over** — in `router.go`, the `/fanfic` group currently reads:
```go
			if cfg.FanficAdminOnly {
				r.Use(AdminRoleMiddleware)
			}
```
Replace those 3 lines with:
```go
			r.Use(FeatureGate("fanfic", featureRuleset))
```
(Leave the surrounding `JWTValidationMiddleware` + `userRateLimit` + `BlockGuestRoleMiddleware` untouched.)

- [ ] **Step 5: Cut `gacha` over** — in the player-facing gacha group, replace:
```go
				if cfg.GachaAdminOnly {
					r.Use(AdminRoleMiddleware)
				}
```
with:
```go
				r.Use(FeatureGate("gacha", featureRuleset))
```
**Do NOT touch** the separate gacha admin-content group that is unconditionally `AdminRoleMiddleware` (the comment there says "ALWAYS admin-gated, independent of the dark-ship flag").

- [ ] **Step 6: Run — expect PASS**

Run: `cd services/gateway && go test ./internal/transport/ -run FeatureGate -v && go build ./...`
Expected: PASS; build clean. (The old `cfg.FanficAdminOnly`/`cfg.GachaAdminOnly` reads are now gone from router.go — the fields still exist in config until Task 4, so the build stays green.)

- [ ] **Step 7: Commit**

```bash
git add services/gateway/internal/transport/router.go services/gateway/internal/transport/router_feature_gate_test.go services/gateway/internal/config/config.go
git commit -m "feat(gateway): FeatureGate('fanfic'|'gacha') + ruleset refresher wiring" \
  -m "Co-Authored-By: Claude Code <noreply@anthropic.com>" \
  -m "Co-Authored-By: 0neymik0 <0neymik0@gmail.com>" \
  -m "Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

### Task 4: Cut `profile-wall` over (collapse if/else) + remove the `*AdminOnly` config bools + compose env

**Files:**
- Modify: `services/gateway/internal/transport/router.go` (profile-wall if/else)
- Modify: `services/gateway/internal/config/config.go` (remove 3 bools + loads)
- Modify: `docker/docker-compose.yml` (remove the 3 env vars if present on the gateway service)
- Test: extend `services/gateway/internal/transport/router_feature_gate_test.go`

- [ ] **Step 1: Read the full profile-wall block** — `sed -n '585,625p' services/gateway/internal/transport/router.go`. It is an `if cfg.ProfileWallAdminOnly { <admin group: JWT + userRateLimit + AdminRole + 3 routes> } else { <open group: OptionalJWT + userRateLimit + same 3 routes> }`. Confirm the exact route set in BOTH branches (expected: `GET /users/{userId}/showcase`, `PUT /users/me/showcase`, `GET /users/{userId}/compatibility` — verify the `else` branch registers the same set; if it differs, preserve the union).

- [ ] **Step 2: Add the failing profile-wall test** — extend `router_feature_gate_test.go`: with the stub ruleset giving `profile-wall` → `roles:[admin]`, `failSafe:"admin"`, assert `GET /api/users/u1/showcase`:
  - anonymous (no token) → 403 (OptionalJWT lets it through to FeatureGate, which denies) — NOT 401.
  - valid non-admin JWT → 403.
  - admin JWT → 200 (reaches player backend).
  Add `profile-wall` to the stub ruleset's flags+failSafe.

Run: `cd services/gateway && go test ./internal/transport/ -run FeatureGate -v`
Expected: FAIL (still the old if/else).

- [ ] **Step 3: Collapse the if/else** — replace the ENTIRE `if cfg.ProfileWallAdminOnly { ... } else { ... }` block with a single group mounted with OptionalJWT + FeatureGate (so the eventual `everyone` state is public/anonymous, matching today's revealed `else` branch, while the seed `roles:[admin]` reproduces today's dark-ship):

```go
		// Profile-showcase ("стена") — runtime-gated by the policy ruleset
		// (flag "profile-wall"). OptionalJWT so the flag's eventual "everyone"
		// audience is public (anonymous); the seed roles:[admin] reproduces the
		// prior admin-only dark-ship. The player enforces owner-only writes from
		// JWT claims downstream (defense-in-depth). Registered BEFORE the
		// protected /users/* group so chi matches these specific routes first.
		r.Group(func(r chi.Router) {
			r.Use(OptionalJWTValidationMiddleware(cfg.JWT, cfg.Services.AuthService))
			r.Use(userRateLimit)
			r.Use(FeatureGate("profile-wall", featureRuleset))
			r.Get("/users/{userId}/showcase", proxyHandler.ProxyToPlayer)
			r.Put("/users/me/showcase", proxyHandler.ProxyToPlayer)
			r.Get("/users/{userId}/compatibility", proxyHandler.ProxyToPlayer)
		})
```
(If Step 1 found extra routes in either branch, include them here.)

- [ ] **Step 4: Run — expect PASS**

Run: `cd services/gateway && go test ./internal/transport/ -run FeatureGate -v`
Expected: PASS (fanfic + gacha + profile-wall matrices).

- [ ] **Step 5: Remove the dead config bools** — in `services/gateway/internal/config/config.go` delete the three struct fields `FanficAdminOnly`, `GachaAdminOnly`, `ProfileWallAdminOnly` (and their doc comments) and their three `Load()` lines (`getEnvBool("GACHA_ADMIN_ONLY"...)`, `getEnvBool("PROFILE_WALL_ADMIN_ONLY"...)`, `getEnvBool("FANFIC_ADMIN_ONLY"...)`). Then `grep -rn 'AdminOnly' services/gateway/` to confirm ZERO remaining references (router.go no longer reads them after Tasks 3-4). If `getEnvBool` becomes unused, leave it (other bools may use it — check with `grep -n getEnvBool`).

- [ ] **Step 6: Remove the stale env vars from compose** — `grep -n 'FANFIC_ADMIN_ONLY\|GACHA_ADMIN_ONLY\|PROFILE_WALL_ADMIN_ONLY' docker/docker-compose.yml docker/docker-compose.prod.yml`. Delete any matching lines from the **gateway** service's `environment:` block (they are now unread). If none are present (they may have only ever been defaults), skip — note it in the report.

- [ ] **Step 7: Full gateway build + test**

Run: `cd services/gateway && go build ./... && go test ./...`
Expected: build clean; all transport/config tests green (new FeatureGate matrices + no regressions). Confirm no dangling reference to the removed bools anywhere: `grep -rn 'FanficAdminOnly\|GachaAdminOnly\|ProfileWallAdminOnly' services/gateway/` → empty.

- [ ] **Step 8: Commit**

```bash
git add services/gateway/internal/transport/router.go services/gateway/internal/config/config.go services/gateway/internal/transport/router_feature_gate_test.go docker/docker-compose.yml docker/docker-compose.prod.yml
git commit -m "feat(gateway): FeatureGate('profile-wall'), remove *AdminOnly bools + env (ruleset is authority)" \
  -m "Co-Authored-By: Claude Code <noreply@anthropic.com>" \
  -m "Co-Authored-By: 0neymik0 <0neymik0@gmail.com>" \
  -m "Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Phase-2 exit criteria
- Gateway resolves `fanfic`/`gacha`/`profile-wall` access per-request against the cached policy ruleset; per-user allow-list grants and per-role changes take effect within `POLICY_RULESET_REFRESH` (15s) — no gateway rebuild.
- Day-one identical: seed `roles:[admin]` ⇒ same admins allowed as the old `*AdminOnly=true`. Cold start / policy outage ⇒ fail-static / failSafe (fanfic/gacha/profile-wall = admin-only).
- `*AdminOnly` config bools + their env vars removed; `grep AdminOnly services/gateway/` clean (except `AdminRoleMiddleware`, which is the unconditional admin-tool gate — unrelated).
- `go build ./... && go test ./...` green in `services/gateway`.

## Deploy-time watch-items (added to the deferred `/animeenigma-after-update` pass)
- Through the gateway (not the container directly): non-admin `GET /api/fanfic/` ⇒ 403; admin ⇒ 200. Flip `fanfic` to `roles:[everyone]` via `PUT /api/admin/policy/flags/fanfic` ⇒ a logged-in non-admin now gets 200 within 15s (proves the runtime path).
- Confirm the gateway reaches `http://policy:8098/internal/policy/ruleset` on the Docker network at boot (log shows no repeated "ruleset refresh failed").
- Removing `FANFIC_ADMIN_ONLY`/`GACHA_ADMIN_ONLY`/`PROFILE_WALL_ADMIN_ONLY` from gateway env is inert (bools deleted); confirm no other service reads them (they never did — grep the repo).

## Self-review
- **Spec coverage (§5.2):** ruleset cache + refresh ✓ (Task 1); FeatureGate + cold-start failSafe + fail-static ✓ (Tasks 1-2); cut fanfic/gacha/profile-wall ✓ (Tasks 3-4); remove env bools ✓ (Task 4). Redis pub/sub invalidate is intentionally DEFERRED (poll-only at 15s is sufficient for infrequent admin toggles; note in report). Route→key mapping stays code-explicit in the router ✓.
- **Placeholder scan:** the two "read the exact block / confirm route set" steps (Task 3 Step 2 wiring into the real cleanup, Task 4 Step 1 profile-wall block) are verify-against-code instructions with concrete expected shapes, not unresolved work. `getEnvDuration`/`getEnvBool` existence checks have concrete fallbacks.
- **Type consistency:** `audience`/`rulesetSnapshot`/`rulesetFetchFunc`/`newRulesetCache`/`snapshot()`/`FeatureGate(key,cache)`/`featureAllowed`/`canAccess` used identically across Tasks 1→4. `audienceContains` is the single slice-contains helper (collision-checked).

## Metrics
- **UXΔ** = `+2 (Better)` — admins gain runtime, per-user/per-role control of the three dark-ship features with ≤15s propagation and no rebuild; end users unaffected day-one.
- **CDI** = `0.03 * 13` — contained to `services/gateway` (2 new files + router/config edits + compose env removal); additive, one integration seam (the ruleset feed).
- **MVQ** = `Griffin 87%/83%`.
