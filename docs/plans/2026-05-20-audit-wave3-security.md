# Audit Wave 3 — Security Hardening

**Date:** 2026-05-20
**Predecessors:** Wave 1 (`2026-05-19-audit-wave1-fixes.md`), Wave 2 (`2026-05-20-audit-wave2-quickwins.md`)
**Revision:** 1 — reviewed by code-reviewer subagent 2026-05-20, NEEDS_REVISION verdict. Corrected:
  - **WV3-T1 framing fundamentally rewritten** — there's no `session_revocations` table; revocation lives on `user_sessions.revoked_at` and no access-token middleware consults it. This task populates the SessionID claim only; enforcement is a separate task in a future wave.
  - **WV3-T1 mint sites** — fixed two sites (router.go:392 + :448), not one.
  - **WV3-T1 location** — committed to gateway-side derivation (avoids cross-service contract change).
  - **WV3-T3 motivation** — gateway already has per-IP rate limiting via `IPRateLimiter`; the new capability is per-authenticated-identity, not "global".
  - **WV3-T3 prereq** — gateway has NO Redis client wired up today; that scaffolding is now part of T3.
  - **WV3-T3 429 shape** — matches existing `errors.RateLimited()`; do not invent a new JSON shape.
  - **WV3-T3 algorithm** — pinned to `redis_rate` GCRA, not naive INCR+EXPIRE.
  - **WV3-T2 list scope** — `HLSProxyAllowedDomains` only (not `DefaultProxyConfig`).
  - **WV3-T2 consumers** — scraper also depends on the list; doc must cross-link.

**Goal:** Close three medium-priority security findings: API-key SessionID claim correctness (foundation for future revocation enforcement), HLS proxy allow-list auditability, per-user rate limiting at the gateway.
**Workflow:** subagent-driven.

## Aggregate metrics

- **UXΔ:** `+2 (Better)`
- **CDI:** `0.13 * 21`
- **MVQ:** `Griffin 85%/85%`

## Deferred from this wave

| Finding | Why deferred |
|---|---|
| **S3** — JWT → httpOnly cookie | Frontend auth refactor + CSRF plumbing; dedicated plan. |
| **C2** — `AutoMigrate` → `golang-migrate` | Cross-cuts 10+ services. |
| **C3** — Player crons → scheduler with leader election | Architectural; depends on scheduler service surface. |
| **Access-token revocation enforcement** (new — surfaced during plan review) | No middleware checks `user_sessions.revoked_at` on access tokens today. WV3-T1 populates SessionID; enforcement is its own task. |
| Single-host failover (K8s direction) | Memory: brainstorm required first. |

---

## Tasks

### WV3-T1 (S4) — API-key minted JWT carries a derived `SessionID` (foundation)

**Trace:** Audit finding S4 — "API-key minted JWT with empty SessionID"

**Where today:**

Two mint sites in the gateway both call `GenerateTokenPair(..., "" /* SessionID */)`:
- `services/gateway/internal/transport/router.go:392` — `JWTValidationMiddleware` (main `ak_*` → JWT path)
- `services/gateway/internal/transport/router.go:448` — `OptionalJWTValidationMiddleware`

Both need the fix or the optional-auth route silently keeps emitting empty-SID tokens.

**What the bug actually means (clarified by plan reviewer):**

The codebase has `user_sessions.revoked_at` (used by **refresh-token** validation only). There is **no access-token middleware that consults it** — so empty SessionID is not blocking active enforcement (there isn't any yet for access tokens), but it IS:

1. Defeating audit-log correlation across `ak_*` calls — every API-key request looks indistinguishable session-wise.
2. Blocking any future per-session revocation middleware from being able to act on `ak_*` traffic at all.

**Scope (explicit):** This task populates SessionID at both mint sites. **It does NOT add access-token revocation enforcement.** That's a separate task in a future wave. The win here is foundation + audit-log correlation.

**Design — deterministic SessionID, derived gateway-side:**

Compute SessionID right next to the mint, using data the gateway already has (resolved `userID` + the raw incoming `ak_*` token):

```go
day := clk.Now().UTC().Format("2006-01-02")
keyHash := sha256.Sum256([]byte(rawAPIKey))
sidInput := userID + "|" + hex.EncodeToString(keyHash[:]) + "|" + day
sidHash := sha256.Sum256([]byte(sidInput))
sessionID := "ak-" + hex.EncodeToString(sidHash[:8])  // 19 chars total: "ak-" + 16 hex = 64 bits
```

Properties:
- Same key + same UTC day → same SessionID (in-flight JWTs cached against this SID remain consistent).
- Different day → fresh SessionID space (revoking yesterday doesn't affect today).
- Rotating the key → entirely new SessionID space (old revocations become inert).
- Audit logs gain a stable correlation ID per identity-per-day.

**Known limitation (accepted):** A JWT minted at 23:59 with a 15-min access-token TTL will carry yesterday's SessionID for up to 15 minutes into today. Bounded by access-token TTL; acceptable for audit-correlation use. If future enforcement work needs tighter granularity, switch to hourly buckets or random+Redis SIDs at that time.

**Why gateway-side, not auth-side:**
- Gateway already has both the raw `ak_*` token and the resolved `userID` in scope.
- No cross-service contract change (`POST /internal/resolve-api-key` JSON response stays as-is — currently returns `user_id`, `username`, `role`).
- Single edit surface; both mint sites share one small helper.

**TDD:**

1. **Red** — `services/gateway/internal/transport/apikey_session_test.go` (new): given a known raw token + userID + fixed clock, assert `deriveAPIKeySessionID()` returns a 19-char string matching `^ak-[0-9a-f]{16}$`.
2. **Red** — assert determinism: same `(rawToken, userID, day)` → same SID; rotating rawToken → different SID; advancing the clock past midnight UTC → different SID.
3. **Red** — middleware-level test in `router_apikey_test.go`: an `ak_*` request through `JWTValidationMiddleware` ends with a JWT whose decoded `sid` claim matches the expected deterministic value. Also covers `OptionalJWTValidationMiddleware`.
4. **Green** — implement `deriveAPIKeySessionID` in a new helper file. Wire it at both mint sites. Inject a `clock func() time.Time` (default `time.Now`) for testability.

**Files:**
- `services/gateway/internal/transport/router.go` (lines ~392, ~448 — pass derived SessionID)
- `services/gateway/internal/transport/apikey_session.go` (new — helper + injectable clock)
- `services/gateway/internal/transport/apikey_session_test.go` (new)
- `services/gateway/internal/transport/router_apikey_test.go` (new or extend existing router tests)

**Acceptance:**
- New tests green.
- e2e: real `ak_*` request through gateway produces a JWT with non-empty `sid` claim matching the deterministic formula.
- Password-login flow unaffected (regression check).
- Two consecutive API-key requests on the same UTC day produce the same `sid` (decode and compare).
- Existing request-log middleware now carries `sid` in its structured output for `ak_*` calls (audit-correlation win).
- No change to `POST /internal/resolve-api-key` request or response shape.

**Commit:**
```
fix(gateway): API-key minted JWTs carry a derived SessionID

Two mint sites in gateway (transport/router.go:392, :448) previously
passed empty SessionID to GenerateTokenPair. The user_sessions table
has revoked_at for refresh-token revocation, but no access-token
middleware consults it today — so an empty SessionID isn't blocking
active enforcement (there is none yet for access tokens), but it IS:
  - Defeating audit-log correlation for ak_* calls
  - Blocking any future per-session revocation middleware

Derives a deterministic SessionID gateway-side from
(user_id, sha256(raw ak_*), UTC-date) — 19 chars, 64 bits of entropy.
Cacheable within a day, rotation-safe, audit-correlatable. Adds the
helper + tests + injectable clock.

Access-token revocation enforcement remains out of scope (separate task).
```

**Metrics:** UXΔ `+1 (Better)` · CDI `0.05 * 5` · MVQ `Griffin 80%/85%`

---

### WV3-T2 (S6) — Structured HLS proxy allow-list + audit tooling

**Trace:** Audit finding S6 — "HLS proxy allow-list quarterly audit"

**Where today (clarified by plan reviewer):**

`libs/videoutils/proxy.go` has **two** allow-lists:
- `DefaultProxyConfig().AllowedDomains` (~4 entries at lines 48-54) — used by general proxy callers; **out of scope for this task**.
- **`HLSProxyAllowedDomains`** (~40 entries at line 230+) — the actual HLS proxy allow-list used by `services/streaming/internal/handler/stream.go:38` AND referenced by scraper (`services/scraper/internal/handler/scraper_test.go:37, 471`). **This is the target.**

**Risk model:** Adding a domain to `HLSProxyAllowedDomains` is a one-line code change today; no provenance, no automated audit, no codeowner gate. A misjudgement or compromised contributor adds a domain → user clients (and scraper HTTP calls) trust attacker-controlled origins.

**Approach (no allow-list entries change semantically — pure tooling):**

1. Restructure `HLSProxyAllowedDomains` from bare `[]string` to a struct slice carrying provenance per entry (owner / date-added / reason). Export `HLSProxyAllowedDomainsList() []string` derived view so existing callers remain unchanged.
2. Add `scripts/audit-hls-allowlist.sh` — uses Go AST grep (e.g. `gopls`-driven or simple `go build`-based introspection, OR sentinel-comment bracketed parsing) to print current entries with provenance, robust against reformatting / reordering. **Not** raw textual `git diff` against `origin/main` — that's false-positive prone.
3. Add `.github/CODEOWNERS` (new file) gating `libs/videoutils/proxy.go` to a specified reviewer for required review.
4. Add `docs/security/hls-proxy-allowlist.md` (new directory) — process doc: provenance format, quarterly review cadence, audit script usage.

**TDD:** Light. Smoke-test the audit script against a known fixture (struct slice with N entries → script prints N annotated lines).

**Files:**
- `libs/videoutils/proxy.go` — restructure `HLSProxyAllowedDomains` with provenance; preserve `HLSProxyAllowedDomainsList() []string` API.
- `libs/videoutils/proxy_test.go` — update for new structure; verify list-view still matches the union of struct entries.
- `scripts/audit-hls-allowlist.sh` (new)
- `.github/CODEOWNERS` (new file)
- `docs/security/hls-proxy-allowlist.md` (new directory)

**Acceptance:**
- `scripts/audit-hls-allowlist.sh` lists every entry with owner / date-added / reason.
- Existing callers in streaming AND scraper compile and behave identically (run `make redeploy-streaming` and scraper smoke tests).
- `go test ./libs/videoutils/...` passes.
- `.github/CODEOWNERS` parsed correctly by GitHub (verify via `gh api repos/.../codeowners/errors` or PR test).
- Process doc cross-linked from BOTH `services/streaming/README.md` AND wherever scraper documents its dependencies (`services/scraper/README.md` if present, else its CLAUDE.md note).

**Commit:**
```
chore(security): structured HLS proxy allow-list with audit tooling

Targets HLSProxyAllowedDomains (libs/videoutils/proxy.go:230+) — the
list actually used by streaming HLS proxy and scraper URL validation.
DefaultProxyConfig().AllowedDomains is intentionally untouched.

Adds:
  - Provenance per entry (owner / date-added / reason) as a struct
    slice; preserves HLSProxyAllowedDomainsList() []string for callers
  - scripts/audit-hls-allowlist.sh — quarterly review tool, AST-aware
    (not raw textual diff)
  - .github/CODEOWNERS gate on libs/videoutils/proxy.go
  - docs/security/hls-proxy-allowlist.md — process doc, cross-linked
    from streaming + scraper

No allow-list entries change semantically.
```

**Metrics:** UXΔ `+1 (Better)` · CDI `0.03 * 5` · MVQ `Sprite 75%/80%`

---

### WV3-T3 (S7) — Per-user rate limit at gateway

**Trace:** Audit finding S7 — "Per-user rate limit at gateway"

**Where today (corrected by plan reviewer):**

Gateway already enforces a **per-IP** RPS bucket via `IPRateLimiter` (`services/gateway/internal/transport/router.go:519-573`), with `RATE_LIMIT_RPS=100`, `RATE_LIMIT_BURST=200`. This isolates per source IP, but:
- A single authenticated user behind a dynamic-IP residential ISP effectively gets multiple buckets.
- A single API key called from N IPs effectively gets N× the limit.
- There is no surgical throttle keyed on identity for an abuse incident.

The new capability: **per-authenticated-user** token bucket layered on top of the existing per-IP one. Anonymous traffic stays per-IP-limited unchanged.

**Prerequisite (surfaced during plan review — baked into the same commit):**

**Gateway has no Redis client wired up today.** `RedisAddr` is in `services/gateway/internal/config/config.go:55` but never consumed downstream. Before the middleware works:

1. Add `libs/cache` to `services/gateway/go.mod` (require + replace directives).
2. Add `COPY libs/cache/go.mod libs/cache/go.sum* ./libs/cache/` to `services/gateway/Dockerfile` deps stage.
3. In `cmd/gateway-api/main.go`, call `cache.New(cfg.RedisAddr, ...)` and plumb the `*cache.Client` (or raw `*redis.Client`) into the new middleware constructor.
4. `go work sync`.

These steps are part of the T3 commit — the middleware does not exist without them.

**Approach — `redis_rate` (GCRA), applied after auth:**

| Property | Value |
|---|---|
| Library | `github.com/go-redis/redis_rate/v10` (battle-tested GCRA, atomic Lua) |
| Bucket key | `ratelimit:user:{user_id}` |
| Default rate | `redis_rate.PerMinute(60)` |
| Default burst | 10 |
| Applied at | Gateway, AFTER auth middleware (so `user_id` is in context) |
| Anonymous routes | Skipped entirely — existing `IPRateLimiter` handles them unchanged |
| 429 response | Match existing `errors.RateLimited()` shape: `{"error":{"code":"RATE_LIMITED","message":"rate limit exceeded"}}` + `Retry-After: <seconds>` header |
| Metric | `gateway_rate_limit_user_blocked_total` — **no labels** (cardinality cap; forensics belong in structured logs) |
| Tuning env vars | `USER_RATE_LIMIT_PER_MINUTE`, `USER_RATE_LIMIT_BURST` (defaults 60, 10) |
| Failure mode | Redis outage → **fail open** (log warning at WARN level); do NOT 500 every authenticated request because Redis blipped |

**Why match existing 429 shape:** `libs/errors.RateLimited()` is already used by `IPRateLimiter`. Inventing a new shape forks the error model. Use the existing JSON + add `Retry-After` header (standard HTTP, language-agnostic).

**Why no labels on the counter:** Even hashed `user_id` would create one label-value per unique 429'd user — open to cardinality blowup under an abuse wave. Absolute counter is enough for alerts; structured logs carry the user_id for incident response.

**TDD:**

1. **Red** — middleware unit test using `miniredis`: 11 rapid authenticated requests from one user → first 10 pass, 11th returns 429 with `Retry-After` header populated and the expected JSON body.
2. **Red** — replenishment test (fast-forward `miniredis` clock): after enough time, bucket allows traffic again.
3. **Red** — anonymous-traffic test: requests with NO `user_id` in context skip the middleware (zero Redis interactions — verify via `miniredis.Stats`).
4. **Red** — metric test: blocked requests increment `gateway_rate_limit_user_blocked_total`.
5. **Red** — fail-open test: stop `miniredis`; subsequent authenticated request returns 200 (with a logged WARN), not 500.
6. **Green** — implement.

**Files:**
- `services/gateway/internal/transport/user_rate_limit.go` (new — gateway's middleware all live in `transport/` today)
- `services/gateway/internal/transport/user_rate_limit_test.go` (new)
- `services/gateway/internal/transport/router.go` — wire AFTER auth middleware; ensure ordering: IPRateLimiter → auth → user_rate_limit → handler.
- `services/gateway/internal/config/config.go` — `USER_RATE_LIMIT_PER_MINUTE`, `USER_RATE_LIMIT_BURST` env reads + struct fields; consume existing `RedisAddr`.
- `services/gateway/cmd/gateway-api/main.go` — instantiate Redis client via `libs/cache`; pass to middleware constructor.
- `services/gateway/go.mod` — add `libs/cache` dependency.
- `services/gateway/Dockerfile` — add `libs/cache` COPY line in deps stage.
- `docker/.env` (operator-managed; gitignored) — operator adds the new vars; document them in CLAUDE.md "Environment Variables" section.

**Acceptance:**
- All five new tests pass.
- e2e: hammering as `ui_audit_bot` triggers 429 within expected count; after a wait, requests succeed (replenishment).
- e2e: anonymous traffic continues per-IP-limited unchanged.
- e2e: `gateway_rate_limit_user_blocked_total` visible at `/metrics`.
- Existing auth flow + protected endpoint regression test passes.
- Stopping local Redis (or pointing `RedisAddr` to a black hole) → authenticated traffic still works with a logged WARN; no 500 spike.
- Redis client connection established at gateway startup (verify in startup logs).

**Commit:**
```
feat(gateway): per-user rate limit middleware (redis_rate GCRA)

Gateway already throttles per-IP via IPRateLimiter. But a user behind
a dynamic-IP residential ISP, or an API key called from multiple IPs,
can effectively exceed the intent. Adds a per-authenticated-user
token bucket layered on top.

Prerequisite folded into this commit: wires libs/cache + Redis client
in cmd/gateway-api/main.go. Gateway previously had RedisAddr in config
but no consumer.

Uses redis_rate GCRA limiter. Defaults: 60 req/min, burst 10. Tunable
via USER_RATE_LIMIT_PER_MINUTE / USER_RATE_LIMIT_BURST. Fails open on
Redis outage (logs WARN). 429 response matches the existing
errors.RateLimited shape and adds a Retry-After header.

Metric: gateway_rate_limit_user_blocked_total (no labels — cardinality
cap; forensic detail lives in structured logs).
```

**Metrics:** UXΔ `+1 (Better)` · CDI `0.05 * 13` · MVQ `Griffin 85%/90%`

---

## Execution discipline

- One commit per task, three co-authors, no `git add -A`, after-update bundled at end.
- **Pre-execution review:** done; revision 1 above incorporates findings.
- T1 and T3 touch live auth / limit paths. After each lands, smoke-test existing protected endpoints end-to-end before declaring done.
- T2 ships first as a zero-behavior-change warm-up.

## Suggested execution order

1. **WV3-T2** (zero behavior change) — warm up.
2. **WV3-T1** (SessionID claim populate) — well-scoped, two mint sites + helper.
3. **WV3-T3** (per-user rate limit + Redis prereq) — largest; needs solid auth context + Redis scaffolding.

## Out of scope (intentional)

- **S3** (JWT → httpOnly cookie) — separate plan.
- **Access-token revocation enforcement middleware** — separate plan; carved out explicitly during revision.
- Multi-tenant rate-limit profiles (per-plan, per-role) — premature.
- Distributed token-bucket consensus across gateway replicas — premature; single replica today.
- Labels on `gateway_rate_limit_user_blocked_total` — dropped to cap cardinality.

## Post-Wave 3 checklist

- [x] Plan reviewed by code-reviewer subagent (2026-05-20) — REVISION 1 applied
- [ ] WV3-T2 → review → merge
- [ ] WV3-T1 → review → merge
- [ ] WV3-T3 → review → merge (Redis-client prereq inside the same commit)
- [ ] `/animeenigma-after-update` (redeploy gateway + streaming; changelog entry)
- [ ] Push to `origin/main`
- [ ] Decide next dedicated plan: access-token revocation enforcement, JWT-httponly-cookie, golang-migrate, scheduler-leader-election, watch-history-partitioning, backup-restore-CI
