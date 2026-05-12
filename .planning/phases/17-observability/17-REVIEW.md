---
phase: 17-observability
reviewed: 2026-05-12T12:08:22Z
depth: standard
files_reviewed: 36
files_reviewed_list:
  - docker/grafana/dashboards/scraper-health.json
  - docker/grafana/provisioning/alerting/rules.yml
  - docker/prometheus/prometheus.yml
  - libs/metrics/provider.go
  - libs/metrics/provider_test.go
  - services/gateway/Dockerfile
  - services/gateway/internal/config/config.go
  - services/gateway/internal/config/config_test.go
  - services/gateway/internal/handler/proxy.go
  - services/gateway/internal/handler/proxy_test.go
  - services/gateway/internal/service/proxy.go
  - services/gateway/internal/service/proxy_test.go
  - services/gateway/internal/transport/router.go
  - services/gateway/internal/transport/router_test.go
  - services/scraper/cmd/scraper-api/main.go
  - services/scraper/internal/domain/provider.go
  - services/scraper/internal/handler/scraper.go
  - services/scraper/internal/handler/scraper_test.go
  - services/scraper/internal/health/cache.go
  - services/scraper/internal/health/cache_test.go
  - services/scraper/internal/health/golden.go
  - services/scraper/internal/health/golden_test.go
  - services/scraper/internal/health/probe.go
  - services/scraper/internal/health/probe_test.go
  - services/scraper/internal/health/stage.go
  - services/scraper/internal/health/stage_test.go
  - services/scraper/internal/health/testutil_provider.go
  - services/scraper/internal/health/window.go
  - services/scraper/internal/health/window_test.go
  - services/scraper/internal/providers/animepahe/client.go
  - services/scraper/internal/providers/animepahe/client_test.go
  - services/scraper/internal/service/orchestrator.go
  - services/scraper/internal/service/orchestrator_test.go
  - services/scraper/internal/transport/router.go
  - services/scraper/internal/transport/router_test.go
findings:
  blocker: 3
  warning: 11
  info: 4
  total: 18
status: issues_found
---

# Phase 17: Code Review Report

**Reviewed:** 2026-05-12T12:08:22Z
**Depth:** standard
**Files Reviewed:** 36
**Status:** issues_found

## Summary

The Phase 17 observability implementation is broadly correct and well-tested.
Concurrency safety in the probe + cache layer, fail-open semantics, panic
recovery, sliding-window threshold logic, and Prometheus label cardinality
all check out. Threat-model claims from the plan (cache key whitelist via
fail-open, panic recovery, error-text redaction, admin auth at the gateway)
are anchored by tests and present in code.

However, the review surfaced three **BLOCKER** issues that ship security
and resource-leak risk to production:

1. The probe segment fetcher (`ProbeRunner.fetchSegment`) issues an HTTP
   GET against an **attacker-influenced URL** with no scheme/host allow-list
   and **follows redirects by default**. The URL originates from
   `provider.GetStream()` which on AnimePahe ultimately resolves through
   the Kwik extractor — an upstream-compromise SSRF vector that could
   probe the internal Docker network from inside the scraper.
2. The gateway proxy (`ProxyService.Forward`) verbatim-copies every header
   from the client request to the upstream service — **including hop-by-hop
   headers** (`Connection`, `Te`, `Proxy-Authorization`, etc.) — violating
   RFC 7230 §6.1 and creating a request-smuggling surface.
3. The outer `defer recover()` in `ProbeRunner.Start` re-spawns the runner
   via `go r.Start(ctx)` without **any backoff or panic-rate cap**. A
   provider whose method panics deterministically would spin a respawn
   loop at goroutine-creation speed until ctx is cancelled (the inner
   `runOneTickSafely` shields most cases, but any panic in the loop body
   or `nextSleep`/`time.After` plumbing escapes to the outer recover).

Eleven **WARNING** findings cover smaller correctness, test-pollution, and
defense-in-depth gaps. Four **INFO** items document style / future-proofing
observations.

## Blocker Issues

### BLK-01: SSRF — probe segment fetcher trusts attacker-controlled URLs and follows redirects

**File:** `services/scraper/internal/health/probe.go:320-347` (`fetchSegment`)
**Issue:**
`fetchSegment` does `http.NewRequestWithContext(ctx, GET, urlStr, nil)`
followed by `r.http.Do(req)` where `urlStr = stream.Sources[0].URL`. The
URL is the output of `provider.GetStream(...)` — for AnimePahe, that
chain is `play page → kwik.cx data-src → KwikExtractor.Extract → packed
JS → m3u8 URL`. **Any link in that chain that an attacker can poison
(DDoS-Guard MITM, upstream selector-drift exploit, DNS hijack of
animepahe.ru / kwik.cx) lets them choose an arbitrary URL the scraper
will GET from inside the docker network**, no auth required.

Compounding the risk:
- `r.http = &http.Client{Timeout: segmentTimeout}` has **no `CheckRedirect`
  policy**, so it follows redirects by default. The attacker only needs to
  return a 302 to e.g. `http://postgres:5432/`, `http://redis:6379/`,
  `http://auth:8080/internal/resolve-api-key`, or `http://169.254.169.254/`
  (cloud metadata).
- The 4 KiB read + discard limits exfiltration of large responses but
  does not prevent **blind probes** of internal services. A successful
  TCP/TLS handshake against `auth:8080/internal/...` is itself an
  internal-network reconnaissance signal.
- The probe runs every 15 min ± 20% so the SSRF is rate-limited but
  persistent for the lifetime of the compromised upstream.

This is the SCRAPER-OBS-02 oracle stage — the stage the orchestrator
gates on via `IsHealthy()`. Removing the safety controls here also
weakens the fail-open story: a redirect-loop or slow-loris would make
the probe time out, mark `stream_segment` DOWN, and turn off the
provider. The probe becomes a DoS vector too.

**Fix:**
```go
// In NewProbeRunner — add an allow-list and disable redirects entirely.
const probeSegmentMaxRedirects = 0 // explicit: do not follow

r.http = &http.Client{
    Timeout: segmentTimeout,
    CheckRedirect: func(req *http.Request, via []*http.Request) error {
        return http.ErrUseLastResponse // do NOT follow redirects
    },
    // Optional: install a Transport with a custom DialContext that
    // rejects RFC-1918 + 127.0.0.0/8 + 169.254.0.0/16 + ::1 destinations.
}

// In fetchSegment — validate scheme + host BEFORE any I/O.
func (r *ProbeRunner) fetchSegment(ctx context.Context, urlStr string) error {
    if urlStr == "" {
        return errors.New("stream_segment: empty source URL")
    }
    u, err := url.Parse(urlStr)
    if err != nil {
        return fmt.Errorf("stream_segment: parse url: %w", err)
    }
    if u.Scheme != "http" && u.Scheme != "https" {
        return fmt.Errorf("stream_segment: rejected scheme %q", u.Scheme)
    }
    host := strings.ToLower(u.Hostname())
    if host == "" || isPrivateOrLoopback(host) {
        return fmt.Errorf("stream_segment: rejected host %q", host)
    }
    // (proceed as today, but with the no-redirect client)
}
```
Document the allow-list policy (mirror the existing
`libs/videoutils/proxy.go` allow-list pattern: cdnlibs.org / kwik.* /
streaming CDNs). T-17-02-03 ("probe traffic bounds") in the threat
model only addresses request volume — it does NOT address SSRF
destination control, which is the larger threat surface.

---

### BLK-02: Gateway proxy forwards hop-by-hop headers and Authorization verbatim to scraper backend

**File:** `services/gateway/internal/service/proxy.go:95-100` (`Forward`)
**Issue:**
The `Forward` method copies **every** header from `r.Header` to the
upstream `req.Header` with no filtering:

```go
for key, values := range r.Header {
    for _, value := range values {
        req.Header.Add(key, value)
    }
}
```

This violates RFC 7230 §6.1 (hop-by-hop headers MUST NOT be forwarded):
`Connection`, `Keep-Alive`, `Proxy-Authenticate`, `Proxy-Authorization`,
`TE`, `Trailers`, `Transfer-Encoding`, `Upgrade`. A malicious client
header like `Connection: close, Authorization` would tell the backend to
strip the gateway's freshly-minted JWT, and a `Transfer-Encoding`
manipulation is a classic request-smuggling primitive.

Plan 17-03 specifically gates `/api/admin/scraper/*` behind
`JWTValidationMiddleware` + `AdminRoleMiddleware` and then re-mints a
short-lived JWT in `Authorization`. But the **client's original**
`Authorization` (an `ak_` API key, or attacker-injected JWT) is also
forwarded verbatim alongside the freshly-minted one, because
`req.Header.Add` appends rather than replaces. The scraper backend's
behaviour with two `Authorization` headers is implementation-defined
(net/http reads the first via `Get`, but `golang.org/x/net/http2` may
prefer the last); a downstream that reads `Authorization` via
`req.Header.Values("Authorization")` would see both.

The risk surface specifically for Phase 17:
- The scraper's `/scraper/health/admin` does not currently auth-check,
  so this is latent today. But the plan explicitly relies on the
  gateway gate (D6 in plan 17-03). Future scraper handlers that DO
  auth-check (e.g. a /scraper/admin/replay) would inherit this gap.
- The `Cookie` header is also forwarded — the auth service's
  `refresh_token` cookie would leak to the scraper, which has no
  business knowing it.

**Fix:**
```go
// services/gateway/internal/service/proxy.go
//
// RFC 7230 §6.1 hop-by-hop headers + Cookie + Authorization
// (Authorization is RESET by JWTValidationMiddleware; we should not
// re-forward the original client value alongside it).
var hopByHopHeaders = map[string]struct{}{
    "Connection":           {},
    "Keep-Alive":           {},
    "Proxy-Authenticate":   {},
    "Proxy-Authorization":  {},
    "Te":                   {},
    "Trailer":              {},
    "Trailers":             {},
    "Transfer-Encoding":    {},
    "Upgrade":              {},
    "Cookie":               {}, // backend services do not need refresh-cookie
}

func copyHeaders(dst, src http.Header) {
    // First, honour Connection: <header>, <header>... per RFC 7230
    // by stripping each named header too.
    for _, h := range strings.Split(src.Get("Connection"), ",") {
        h = http.CanonicalHeaderKey(strings.TrimSpace(h))
        if h != "" {
            dst.Del(h)
        }
    }
    for key, values := range src {
        if _, hop := hopByHopHeaders[http.CanonicalHeaderKey(key)]; hop {
            continue
        }
        for _, v := range values {
            dst.Add(key, v)
        }
    }
}
```

Replace the inline loop with `copyHeaders(req.Header, r.Header)`. Add a
regression test that posts `Authorization: Bearer attacker_value` +
valid admin JWT and asserts the backend sees exactly one
`Authorization` header (the minted one).

---

### BLK-03: Probe panic recovery re-spawns goroutine without backoff or panic-rate cap

**File:** `services/scraper/internal/health/probe.go:126-136` (`Start`)
**Issue:**
```go
func (r *ProbeRunner) Start(ctx context.Context) {
    defer func() {
        if rec := recover(); rec != nil {
            r.log.Errorw("scraper.probe: panicked, restarting", ...)
            go r.Start(ctx) // <-- unbounded respawn
        }
    }()
    // ...
}
```

If the outer panic recover ever trips (something panics outside
`runOneTickSafely` — e.g. a future regression in `computeInitialDelay`,
`nextSleep`, or an environmental panic like an OOM in
`time.After` allocation), the goroutine respawns IMMEDIATELY. There is
no backoff, no panic-rate cap, and no observation that the respawn
happened (no metric increment).

Worst-case: a deterministic panic in the loop body (say, an integer
overflow in `r.rng.Int64N(int64(probeBaseInterval/4))` if someone
shrinks `probeBaseInterval` to a sub-second test value) would spin a
goroutine creation hot-loop limited only by Go's goroutine scheduler.
Each respawn allocates ~8 KiB of stack and re-installs the deferred
recover, leaking memory until the context is cancelled.

The threat model T-17-02-01 cites panic recovery as a mitigation but
the chosen "re-spawn the whole goroutine" pattern is strictly worse
than "log + tick again next interval" — which is what
`runOneTickSafely` already does correctly.

The test `TestProbe_PanicInProviderRecovers` only exercises the inner
(per-tick) recover via `r.runOneTickSafely(...)`; the outer recover is
**not tested** at all, so a respawn-bomb regression would not be caught.

**Fix:**
Remove the outer respawn and rely solely on `runOneTickSafely`. The
outer recover should log + emit a metric + return — the loop is built
to survive per-tick panics already.

```go
func (r *ProbeRunner) Start(ctx context.Context) {
    defer func() {
        if rec := recover(); rec != nil {
            r.log.Errorw("scraper.probe: fatal panic, goroutine exiting",
                "provider", r.provider.Name(),
                "panic", rec,
                "stack", string(debug.Stack()), // help debugging
            )
            metrics.ProbeFatalPanicTotal.WithLabelValues(r.provider.Name()).Inc()
            // Do NOT respawn — the missing heartbeat will fire the
            // dead-probe alert (RESEARCH P-07).
        }
    }()
    // ... rest unchanged
}
```

Add a unit test that wraps `Start` and panics from the loop body to
verify the goroutine exits cleanly (uses a custom RNG injected to
panic on the first `Int64N`, then asserts the goroutine count remains
stable after a short wait).

---

## Warnings

### WR-01: `RegisteredProviders` snapshot is racy w.r.t. concurrent `Register` (lock release before return value materialized)

**File:** `services/scraper/internal/service/orchestrator.go:344-350`
**Issue:**
```go
func (o *Orchestrator) RegisteredProviders() []domain.Provider {
    o.mu.RLock()
    defer o.mu.RUnlock()
    out := make([]domain.Provider, len(o.providers))
    copy(out, o.providers)
    return out
}
```

This is actually correct as-written (copy under lock), but `main.go`
**calls this twice** at boot — once for the metric-seed loop
(lines 121-133) and again for the probe-spawn loop (lines 150-153) —
and `Register` is called between the two only if a future maintainer
inserts a Register call. The pattern invites a bug where the seeded
metric set and the spawned probe set diverge.

**Fix:**
Hoist the snapshot to one call in `main.go`:
```go
providers := orchestrator.RegisteredProviders()
for _, p := range providers {
    // ... seed metrics
}
for _, p := range providers {
    runner := health.NewProbeRunner(p, ...)
    go runner.Start(probeCtx)
}
```

---

### WR-02: AdminSnapshot returns shared StageStatus values but handler mutates Stages map via assignment

**File:** `services/scraper/internal/handler/scraper.go:243-254` (`GetAdminHealth`)
**Issue:**
The handler does:
```go
for prov, ph := range snap {
    for st, ss := range ph.Stages {
        if len(ss.LastErr) > health.MaxLastErrChars {
            ss.LastErr = ss.LastErr[:health.MaxLastErrChars]
            ph.Stages[st] = ss  // <-- mutates ph.Stages
        }
    }
    enriched[prov] = ph
}
```

`AdminSnapshot()` already deep-copies the Stages map, so this mutation
is safe. **However**, the in-place `ph.Stages[st] = ss` writes into the
copied-Stages map while iterating over it — Go specifies that
modifying a map during iteration may either reflect or not reflect the
change in the same iteration. Today's code only writes back the same
key the range loop is currently visiting, which is well-defined per
the Go spec — but it is brittle: a future change that adds a new key
(e.g. fanning out a "redacted" sibling key) would be undefined behaviour.

The comment on line 244-246 says "AdminSnapshot already deep-copies …
so it is safe to mutate the StageStatus values in place" which is
strictly true; the lurking concern is that the *iteration-mutate*
pattern is the kind of thing static analysers flag.

**Fix:**
Use a separate output map for clarity:
```go
for prov, ph := range snap {
    redactedStages := make(map[string]health.StageStatus, len(ph.Stages))
    for st, ss := range ph.Stages {
        if len(ss.LastErr) > health.MaxLastErrChars {
            ss.LastErr = ss.LastErr[:health.MaxLastErrChars]
        }
        redactedStages[st] = ss
    }
    enriched[prov] = health.ProviderHealth{Stages: redactedStages, LastUpdated: ph.LastUpdated}
}
```

---

### WR-03: `IsHealthy` reads from a stage that may have a zero `LastUpdated` (probe never wrote `stream_segment` because of short-circuit)

**File:** `services/scraper/internal/health/cache.go:81-96`
**Issue:**
The fail-open contract has four branches; branch 3 ("no `stream_segment`
key in the Stages map → true") was clearly intended to protect against
the case where the probe short-circuited on an earlier stage and never
wrote a `stream_segment` entry. The probe's `commit()` writes every
stage from `AllStages` to the cache's `Stages` map — **but only stages
that were exercised this tick**. Stages skipped due to short-circuit
do NOT get a key in `stages` (the local map in `runOneTick`).

So an upstream where `search` fails will write `Stages = {"search":
{Up:false, LastErr:...}}` only — the `stream_segment` key is missing,
which IsHealthy treats as fail-open. **The orchestrator therefore
keeps dispatching to a provider whose first pipeline stage is broken.**

This is *probably* the intended behaviour (the upstream might still
work for users — the probe's `FindID` failing doesn't prove user
requests will too) but it conflicts with the alert rule
`provider-health-stream-segment-down` which only fires on a fresh
`Up=false`, not on missing-key. So the alerting + the
orchestrator-gate use different definitions of "healthy".

**Fix:**
Either:
1. **commit() writes a "skipped" StageStatus for short-circuited stages**
   (Up = lastKnown, LastErr = "skipped: <earlier-stage> failed"), so
   IsHealthy always has positive evidence; or
2. **Update the comment + add a test** that locks the current behaviour
   ("a provider with no stream_segment entry is treated as fail-open
   even if search has flipped DOWN") so the divergence from alerting
   is documented intentionally.

The plan's SCRAPER-OBS-02 contract is ambiguous on which the orchestrator
should do; choose one and pin it.

---

### WR-04: Background cleanup goroutine in `NewIPRateLimiter` is never stopped, leaks one goroutine per gateway test

**File:** `services/gateway/internal/transport/router.go:460-477` (`NewIPRateLimiter`)
**Issue:**
`NewIPRateLimiter` spawns a `time.Ticker(5*time.Minute)` cleanup
goroutine. `Stop()` exists and closes `stopCh`, but `NewRouter` calls
`NewIPRateLimiter(...)` indirectly via `RateLimitMiddleware` (line 501)
and the returned `*IPRateLimiter` is discarded — there is no call to
`Stop()` anywhere in the codebase. In production this is benign (the
process lives forever). **In tests it leaks one goroutine per
`NewRouter` invocation**, and the gateway test file builds a fresh
router in `buildTestGatewayRouter` per test. Run `go test -race ./...
-count=10` and the leak is visible via `runtime.NumGoroutine()`.

**Fix:**
Either:
1. Make `RateLimitMiddleware` return both the middleware and a stop
   function the router-builder can register on a shutdown lifecycle; or
2. Add a `(t *testing.T).Cleanup(rl.Stop)` hook so the test fixture
   cleans up after itself.

Option 2 is the smaller change and matches the test-only scope of the
leak.

---

### WR-05: Test pollution — orchestrator tests share global metric vectors with `t.Parallel()`

**File:** `services/scraper/internal/service/orchestrator_test.go:147-208, 394-506, 602-716`
**Issue:**
Tests like `TestOrchestrator_FailoverOnProviderDown`,
`TestOrchestrator_FailoverFallbackTotalIncrementCount`, and
`TestOrchestrator_SkipsUnhealthyProvider` all call `t.Parallel()` AND
read/write the global `metrics.ParserFallbackTotal` counter with
unique label values per test. The before/after delta-checks **assume
no other parallel test is incrementing the same label pair**, which
holds today because each test uses unique provider names ("A_down" /
"B_ok" / "animepahe_skip" / etc.) but is fragile: a future test that
reuses a label combination will randomly fail under -race / -shuffle.

**Fix:**
Either:
1. Add a comment to each test explicitly stating the label-pair
   contract (already partly done in `TestOrchestrator_FailoverFallbackTotalIncrementCount`'s
   header comment — extend to all parallel tests); or
2. Use a unique `t.Name()`-derived suffix on every fake provider name:
   ```go
   pa := &fakeProvider{nameVal: "A_" + t.Name()}
   ```
   so collisions are structurally impossible.

The fact that the existing `TestOrchestrator_FailoverFallbackTotalIncrementCount`
comment mentions "WR-08: global-registry metric pollution" suggests
the team is aware; finish the job by making it impossible to collide,
not just unlikely.

---

### WR-06: Probe HTTP client uses default `Transport` — no `MaxConnsPerHost`, no `IdleConnTimeout`

**File:** `services/scraper/internal/health/probe.go:106`
**Issue:**
```go
http: &http.Client{Timeout: segmentTimeout},
```

This client uses `http.DefaultTransport` because no `Transport` is set
explicitly. `DefaultTransport` is process-shared with every other
caller that uses the default — including the AnimePahe `BaseHTTPClient`
(no, that one has its own Transport — but any future provider that
forgets to set one would compete for connections with the probe).

More concretely: the probe runs every 15 min for at most ~10 s; it
opens at most one connection per tick per provider. So this isn't an
acute resource leak today. But the `http.Client` has no
`CheckRedirect` (see BLK-01), no `Jar` (intentional — segment fetches
should not carry cookies), and is shared across all 15-min ticks via
the default transport's connection pool. If a future change adds 5+
providers, each probing in parallel, the default transport's
`MaxIdleConnsPerHost = 2` could become a bottleneck on the same kwik
CDN.

**Fix:**
Construct an explicit Transport with documented limits:
```go
r.http = &http.Client{
    Timeout: segmentTimeout,
    CheckRedirect: func(*http.Request, []*http.Request) error {
        return http.ErrUseLastResponse
    },
    Transport: &http.Transport{
        MaxIdleConnsPerHost: 2,
        MaxConnsPerHost:     4,
        IdleConnTimeout:     90 * time.Second,
        DisableKeepAlives:   false,
    },
}
```

---

### WR-07: `nextSleep` can theoretically return a value that rounds to zero (defensive lower bound missing)

**File:** `services/scraper/internal/health/probe.go:184-187`
**Issue:**
```go
func nextSleep(rng *rand.Rand) time.Duration {
    delta := (rng.Float64()*2 - 1) * probeJitterPct
    return time.Duration(float64(probeBaseInterval) * (1 + delta))
}
```

`probeJitterPct = 0.20`, so the result ranges in `[0.8 * 15min, 1.2 *
15min]`. This is safe with today's constants. **But** the function is
package-private and tested via `RunOnce` rather than the loop, so a
future maintainer who bumps `probeJitterPct` to 1.0 (a full jitter
range) would produce a 0-duration sleep — the loop would spin every
tick at full rate, hammering upstream.

**Fix:**
Add a defensive minimum:
```go
func nextSleep(rng *rand.Rand) time.Duration {
    delta := (rng.Float64()*2 - 1) * probeJitterPct
    out := time.Duration(float64(probeBaseInterval) * (1 + delta))
    if out < probeBaseInterval/2 {
        return probeBaseInterval / 2
    }
    return out
}
```

And a unit test that asserts `nextSleep(rng) >= probeBaseInterval/2`
across 10k iterations.

---

### WR-08: Probe HTTP segment fetch ignores `io.ReadFull`'s error — `ErrUnexpectedEOF` is swallowed

**File:** `services/scraper/internal/health/probe.go:341-345`
**Issue:**
```go
buf := make([]byte, 4096)
n, _ := io.ReadFull(resp.Body, buf)
if n == 0 {
    return errors.New("stream_segment: empty body")
}
return nil
```

For most upstreams the first 4 KiB will come back in one or two TCP
segments and `io.ReadFull` returns `(4096, nil)`. But for a small body
the function returns `(n, ErrUnexpectedEOF)` — currently treated as
success as long as `n != 0`. This is correct **for the contract as
documented** ("at least one non-empty byte"), but a partial-read
failure due to an aborted stream (e.g. connection reset after 200
bytes) is also treated as success. The metric will read UP for an
upstream that can't even keep a connection alive for one segment.

**Fix:**
Distinguish three cases:
```go
n, err := io.ReadFull(resp.Body, buf)
switch {
case n == 0:
    return errors.New("stream_segment: empty body")
case err != nil && !errors.Is(err, io.ErrUnexpectedEOF):
    return fmt.Errorf("stream_segment: read body: %w", err)
}
return nil
```

This still accepts the "real short body" case (`ErrUnexpectedEOF` with
`n > 0`) but escalates a network-error read.

---

### WR-09: `parseQuery` cap is post-trim, not pre-trim — a `prefer` with 64 spaces + 1 char survives

**File:** `services/scraper/internal/handler/scraper.go:91-104`
**Issue:**
```go
prefer := strings.TrimSpace(q.Get("prefer"))
if len(prefer) > maxPreferLength {
    prefer = prefer[:maxPreferLength]
}
```

The cap applies after `TrimSpace`. If an attacker sends `?prefer=<65
chars>`, the trimmed length is 65 and truncation kicks in — fine. But
the maxPreferLength comment claims it bounds log-line + response
payload growth; a value of 64 unprintable bytes (e.g. 0x01..0x40) would
pass through to `meta.tried` and a structured log field without further
sanitisation. The threat is small (provider names are echoed in JSON
which escapes control chars) but the comment overstates the protection.

**Fix:**
Add a `regexp` allow-list: provider names should match
`^[a-z0-9_-]{1,64}$`. Any value that doesn't match → drop to empty
string (silently ignored, matching the existing "unknown prefer
silently ignored" contract).

```go
var preferAllowed = regexp.MustCompile(`^[a-z0-9_-]{1,64}$`)
// ...
if !preferAllowed.MatchString(prefer) {
    prefer = ""
}
```

This also closes a subtle injection vector where a `prefer` like
`animepahe\n[FORGED_LOG_LINE]` would land verbatim into a
zap-structured log entry. (zap escapes newlines in JSON encoding so
the impact is bounded, but the value still appears in log queries.)

---

### WR-10: Scraper service binds via `cfg.Server.Address()` but `main.go` does not validate the bind address is loopback

**File:** `services/scraper/cmd/scraper-api/main.go:164-170`
**Issue:**
Plan 17-03 D6 documents that the scraper "binds to 127.0.0.1 inside
the docker network" and the admin handler therefore trusts the gateway
gate. There is **no code-level enforcement** of this — if a future
maintainer changes `SERVER_HOST=0.0.0.0` in docker-compose.yml (which
is already the default for the gateway), the scraper would expose
`/scraper/health/admin` to the entire docker network with no auth.

The `transport.NewRouter` comment at lines 35-39 explicitly cites this
trust assumption.

**Fix:**
Either:
1. Hard-bind to `127.0.0.1` in `config.Load()` for the scraper service
   (override `SERVER_HOST` env), with a `WARN` log on attempted override; or
2. Add a defense-in-depth auth check in `GetAdminHealth` that requires
   the request to come from a private/loopback IP, or a magic header
   the gateway sets (e.g. `X-Internal-Auth: <shared-secret>`).

Option 2 is closer to defense-in-depth without coupling the scraper to
the docker-compose layout.

---

### WR-11: `time.Now()` used directly in `GetAdminHealth` instead of injectable clock

**File:** `services/scraper/internal/handler/scraper.go:260`
**Issue:**
```go
"generated_at": time.Now().UTC().Format(time.RFC3339),
```

`ScraperHandler` does not have a `now func() time.Time` field, so this
timestamp is hard to lock in tests. The test
`TestAdminHealthHandler_IncludesGeneratedAt` only checks the field
parses as RFC3339; a regression that ships a stale `generated_at`
(e.g. from caching the response) would not be caught.

**Fix:**
Add a `now func() time.Time` field on `ScraperHandler`, default to
`time.Now`, and let tests override. Same pattern as `InMemoryHealthCache`.

---

## Info

### IN-01: `metaTried` test helper silently returns nil on success path with missing meta

**File:** `services/scraper/internal/handler/scraper_test.go:110-135`
**Note:**
`metaTried` returns nil when neither `data.meta.tried` nor top-level
`meta.tried` is present. Tests then check `len(tried) != 1` which
passes false on nil. A missing-`meta` regression would manifest as
`tried[0] != "fakeprov"`, not as a clear "meta is missing" failure. A
minor ergonomic improvement: have `metaTried` accept `t *testing.T`
and fail the test on missing meta unless the caller opts in via a
"may-be-absent" flag.

### IN-02: Golden pool MAL IDs documented as verified 2026-05-12 but verification is not automated

**File:** `services/scraper/internal/health/golden.go:7-8`
**Note:**
"Maintenance: MAL IDs were verified against jikan.moe on 2026-05-12.
Wrong MAL IDs cause permanent false-negatives." A future drift (MAL ID
gets reassigned or anime removed) silently flips the probe DOWN. Add a
build-tag-gated integration test that hits jikan.moe and asserts each
ID resolves to a live anime — runs nightly in CI, not on every commit.

### IN-03: Magic numbers in dashboard JSON (15 min, 24h, 0.5, 0.8) duplicated across alert rules

**File:** `docker/grafana/provisioning/alerting/rules.yml` + `services/scraper/internal/health/window.go:24-29`
**Note:**
The 15-minute window appears in (a) `failureWindow` Go constant, (b)
the `provider-health-stream-segment-down` alert's `for: 15m` field,
and (c) the dashboard's "Probe Last Tick" thresholds (900s warn,
1800s crit). All three should change together. Consider rendering
the alert YAML from a single source of truth, or at least adding
cross-reference comments.

### IN-04: `prober` package comment claims metric `provider_probe_last_tick_timestamp` is per-provider but no test asserts label cardinality bound

**File:** `libs/metrics/provider.go:31-40`
**Note:**
`ProviderProbeLastTick` is a `GaugeVec` with one label (`provider`).
Cardinality is bounded by the registered provider count (1 today).
The metrics tests verify label *names*, not that the label *values*
are bounded. A defensive test would inject 1000 fake providers and
assert the metric still serves at /metrics under 100ms — this is
overkill for v1 but worth a TODO.

---

_Reviewed: 2026-05-12T12:08:22Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
