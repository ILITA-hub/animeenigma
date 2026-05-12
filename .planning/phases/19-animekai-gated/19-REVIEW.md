---
phase: 19-animekai-gated
reviewed: 2026-05-12T00:00:00Z
depth: standard
files_reviewed: 12
files_reviewed_list:
  - services/scraper/internal/providers/animekai/doc.go
  - services/scraper/internal/providers/animekai/client.go
  - services/scraper/internal/providers/animekai/dto.go
  - services/scraper/internal/providers/animekai/client_test.go
  - services/scraper/internal/providers/animekai/helpers_test.go
  - services/scraper/internal/config/config.go
  - services/scraper/internal/config/config_test.go
  - services/scraper/cmd/scraper-api/main.go
  - docker/megacloud-extractor/server.js
  - docker/docker-compose.yml
  - docker/.env.example
  - .planning/REQUIREMENTS.md
findings:
  critical: 1
  warning: 5
  info: 4
  total: 10
status: fixed
fixed_at: 2026-05-12T00:00:00Z
fix_report: .planning/phases/19-animekai-gated/19-REVIEW-FIX.md
---

# Phase 19: Code Review Report

**Reviewed:** 2026-05-12
**Depth:** standard
**Files Reviewed:** 12
**Status:** issues_found

## Summary

Phase 19 ships the AnimeKai provider as an intentionally-minimal escape hatch:
a Go stub whose every Provider method returns wrapped `domain.ErrProviderDown`,
a sidecar route that returns HTTP 501, and a default-off env flag
(`SCRAPER_ANIMEKAI_ENABLED`). The review focused on the five risks the
prompt flagged: (1) flag default-off, (2) stub returning ErrProviderDown not
silent empty success, (3) sidecar returning 501 not 500, (4) no enc-dec.app
dependency leakage, (5) boot-invariant and config parsing.

Findings: 1 blocker, 5 warnings, 4 info. The blocker is a documented-contract
violation between two files in the same submission: `client.go` claims
"Grafana never sees a green panel for the flag-on window" but `main.go`
unconditionally calls `ProviderHealthUp.Set(1)` for every (provider, stage)
pair at boot — including AnimeKai when the flag is on — overriding the
escape-hatch invariant for ~15 min until the first probe tick lands.

Items (4) and (5) from the prompt are correctly enforced; item (3) is
correct; item (1) is correctly default-off. Item (2) is correct at the Go
level but is undermined by the metrics-seeding blocker for the observability
half of the contract.

## Critical Issues

### CR-01: Boot-time metric seed contradicts escape-hatch "no green panel" invariant

**File:** `services/scraper/cmd/scraper-api/main.go:206-219`
**Issue:** When `SCRAPER_ANIMEKAI_ENABLED=true`, the AnimeKai provider is
registered and then included in the `providers := orchestrator.RegisteredProviders()`
snapshot. The subsequent loop unconditionally seeds:

```go
for _, p := range providers {
    for _, stage := range health.AllStages {
        metrics.ProviderHealthUp.WithLabelValues(p.Name(), stage).Set(1)
    }
    metrics.ProviderProbeLastTick.WithLabelValues(p.Name()).Set(0)
    ...
}
```

This sets `provider_health_up{provider="animekai", stage=<every stage>} = 1`
at boot, which is exactly the "Grafana shows a green panel for the ~15 min
before the first probe tick" anti-pattern that `client.go:98-101` and
`doc.go:9-16` explicitly claim to prevent:

> CRITICAL: stages are pre-seeded with Up=false (NOT Up=true) so Grafana
> does not show a green panel for the ~15 min before the first probe tick
> fires when the flag is on.

The internal `Provider.stages` map *is* seeded with `Up=false` (correct), but
the EXPORTED Prometheus gauge that Grafana actually reads from is seeded with
`1`. The in-memory stage map is only surfaced via `/scraper/health/admin`;
Grafana panels and the SCRAPER-OBS-04 alert read `provider_health_up{}` from
Prometheus. Result: the documented invariant is violated end-to-end whenever
the flag is on, and the SCRAPER-OBS-04 alert ("any `stream_segment` reads 0
for 15 min") will *not* fire during the escape-hatch window because the
gauge says 1, not 0.

Additionally, the `client.go:36-41` `stageNames` only lists 4 stages
(no `stream_segment`), so the in-memory snapshot omits it — but `main.go`
seeds 5 stages from `health.AllStages` (including `stream_segment`), so the
Prometheus surface and the in-memory surface disagree on which stages exist
for animekai even on the success path.

**Fix:** Special-case the metric seed when registering the escape-hatch
provider. Either skip the seed entirely for animekai, or seed with 0 (the
escape-hatch contract), or fold the seeding into the provider constructor
so each provider declares its own boot state. Minimal patch:

```go
for _, p := range providers {
    seedValue := 1.0
    if p.Name() == "animekai" { // escape-hatch: never start green
        seedValue = 0.0
    }
    for _, stage := range health.AllStages {
        metrics.ProviderHealthUp.WithLabelValues(p.Name(), stage).Set(seedValue)
    }
    metrics.ProviderProbeLastTick.WithLabelValues(p.Name()).Set(0)
    metrics.ParserZeroMatchTotal.WithLabelValues(p.Name(), "_seeded").Add(0)
}
```

A name-based special case is ugly; a cleaner long-term fix is for the
Provider interface to expose an optional `BootHealthSeed() float64` method
the seeding loop calls instead of hard-coding `Set(1)`. Either way, the
current contradiction between `client.go` comments and `main.go` behavior
must not ship as-is — at minimum, one of the two is wrong and needs to be
reconciled.

## Warnings

### WR-01: `New()` accepts `Deps.MalSync == nil` silently — v3.1 fill-in lands a nil-pointer footgun

**File:** `services/scraper/internal/providers/animekai/client.go:92-94, 102-114`
**Issue:** The constructor explicitly allows `Deps.MalSync` to be nil for
the stub, with a comment promising "the v3.1 fill-in PR will tighten this
to required". Until that tightening lands, any maintainer who fills in
`FindID` body but forgets to wire `gogoanime.NewMalSyncClient(redisCache)`
into `main.go:170-177` will get a runtime nil pointer dereference inside
the first FindID call rather than a clear boot-time error. The current
`main.go:174` passes literal `nil` for `MalSync`, so the fill-in PR has
two places to change atomically (main.go wiring + client.go validation)
or it breaks.

**Fix:** Add a TODO-style guard now so the failure mode is explicit:

```go
// MalSync is nil for the Phase 19 stub; v3.1 fill-in MUST set it.
// When you remove this comment, also tighten the New() validation to
// `if d.MalSync == nil { return nil, errors.New("...") }`.
```

Alternatively, add the strict validation now and pass a no-op malSyncClient
implementation in main.go for the stub. Either approach removes the v3.1
footgun.

### WR-02: Sidecar `/animekai-token` does not drain the POST request body

**File:** `docker/megacloud-extractor/server.js:251-260`
**Issue:** The handler immediately writes 501 and ends the response without
consuming `req`. Node's HTTP server allows this, but on slower TCP paths
(and especially for keep-alive connections where the client has already
started piping a body) closing the response before reading the request can
race with the client-side write and produce ECONNRESET on the caller. The
Go side calls this via the standard `http.Client`, which will surface this
as `unexpected EOF` or `connection reset` — distinct from a clean 501, and
would mask the intended `domain.ErrProviderDown` mapping for what should
be a deterministic stub response.

**Fix:** Drain the body before writing:

```js
if (parsed.pathname === "/animekai-token" && req.method === "POST") {
  console.warn(`/animekai-token called — escape-hatch stub returning 501`);
  req.on("data", () => {}); // drain
  req.on("end", () => {
    res.writeHead(501, { "Content-Type": "application/json" });
    res.end(JSON.stringify({
      error: "AnimeKai sidecar not yet converged — carry to v3.1",
    }));
  });
  return;
}
```

### WR-03: `getEnvBool` silently swallows unparseable values — operator confusion latent

**File:** `services/scraper/internal/config/config.go:190-197`
**Issue:** `SCRAPER_ANIMEKAI_ENABLED=yes-please` (or `=on`, `=enabled`,
`=YES`) all silently fall through to the default false. An operator who
typo'd the value will see the flag effectively off without any log line
indicating their value was rejected. This is consistent with the existing
`getEnvInt` / `getEnvDuration` lenient pattern (see config.go test
`TestLoad_AnimeKaiEnabledInvalid`), so it is documented behavior — but
the "default-off-in-production" guarantee depends on operators reading
docs correctly. A warning log on parse failure is cheap insurance.

**Fix:** Log on parse failure. Either change the helper, or wrap at the
call site:

```go
func getEnvBool(key string, defaultVal bool) bool {
    val := os.Getenv(key)
    if val == "" {
        return defaultVal
    }
    if b, err := strconv.ParseBool(val); err == nil {
        return b
    }
    // Fall back silently — but caller may want to log. Consider returning
    // an error from Load() instead so the operator sees a boot warning.
    return defaultVal
}
```

Or strict-parse it (preferred for a security-relevant flag) so a malformed
value causes Load() to return an error like the URL validators already do.
The strict path matches the pattern used for MEGACLOUD_EXTRACTOR_URL and
the three BaseURL env vars.

### WR-04: `client.go:36-41` `stageNames` diverges from `health.AllStages`

**File:** `services/scraper/internal/providers/animekai/client.go:36-41`
**Issue:** `stageNames` lists 4 stages; `health.AllStages` (used by the
metric seed in main.go and by `ProbeRunner`) lists 5 (adds
`StageStreamSegment`). The comment claims this is intentional ("the fifth
is owned by the probe runner"), which is true for the in-memory
`Provider.stages` map — but the divergence is a footgun. A future
maintainer iterating `stageNames` from a v3.1 fill-in (e.g. "mark every
stage as healthy when /animekai-token succeeds") will silently miss
`stream_segment`, which is precisely the stage SCRAPER-OBS-04 alerts on.
The escape-hatch test (`TestProvider_HealthCheck_AllStagesDownAtBoot`)
asserts only the 4-stage subset — so the test will not catch this drift
either.

**Fix:** Either iterate `health.AllStages` directly when seeding and stop
exposing the truncated local copy, or add a regression test asserting
`len(stageNames) == len(health.AllStages) - 1` so a future addition of
a stage to AllStages triggers a build break here.

### WR-05: Wiring invariant gives a misleading error message on misuse

**File:** `services/scraper/cmd/scraper-api/main.go:264-272`
**Issue:** The invariant is correct (2 providers flag-off, 3 flag-on),
but the `log.Fatalw` message ("Phase 19 wiring invariant broken") only
prints `got`, `want`, and `flag`. It does not enumerate WHICH providers
were registered, so an on-call who hits this in production has to read
source to find the answer. The previous fatal messages in the file
include relevant context for the same reason (e.g. line 116: "failed to
construct AnimePahe provider", "error", err). Also note: this invariant
check is dead code in the success path of every prior phase that did not
have it — it's only a tripwire if a *future* maintainer adds/removes a
Register() call, in which case the friendlier message helps.

**Fix:** Include the actual provider names in the fatal log:

```go
names := make([]string, 0, len(orchestrator.RegisteredProviders()))
for _, p := range orchestrator.RegisteredProviders() {
    names = append(names, p.Name())
}
if got := len(names); got != expectedProviders {
    log.Fatalw("Phase 19 wiring invariant broken",
        "got", got, "want", expectedProviders,
        "flag", cfg.AnimeKai.Enabled,
        "registered", names)
}
```

## Info

### IN-01: Pre-existing SSRF surface in sidecar `/extract` is not made worse, but is adjacent

**File:** `docker/megacloud-extractor/server.js:216-240`
**Issue:** Not introduced by Phase 19 — but worth noting as adjacent
context to the prompt's "no enc-dec.app dependency leakage" check. The
existing `/extract` endpoint accepts `url` from query string and feeds it
directly into `new URL()` and `https.request()` with no host allowlist.
A caller inside the docker network (or anyone who can reach port 3200
externally if the bind ever leaks) can use this to probe arbitrary HTTPS
hosts. The new `/animekai-token` handler does NOT have this surface — it
takes no parameters and returns 501 unconditionally — so Phase 19 itself
does not regress SSRF posture. The `/extract` handler should be
allowlisted in a follow-up, but that is out of Phase 19 scope.

**Fix:** Track separately; ensure the v3.1 fill-in for `/animekai-token`
includes a host allowlist (`anikai.to`, `megaup.cc`) for the embed-URL
parameter it will introduce.

### IN-02: `dto.go` allocates structs that the stub never uses

**File:** `services/scraper/internal/providers/animekai/dto.go`
**Issue:** All four DTOs (`searchResult`, `episodeRow`, `serverRow`,
`malSyncEntry`, `malSyncResponse`) are unused in the Phase 19 stub. Go's
unused-private-field detection (and the `unused` linter) typically flags
this. The file does compile because the types are exported within the
package but referenced nowhere — and Go does not warn on unused private
types the way it warns on unused private variables/funcs. Acceptable for
a "v3.1 fill-in is body-only" forward-compat shim, but worth a clear
`//nolint:unused` annotation or a `_ = searchResult{}` sentinel in a test
file so future linter changes don't break the build.

**Fix:** Add a compile-time anchor in `client_test.go`:

```go
// Pin the DTO surface so the v3.1 fill-in PR is body-only — these types
// are intentionally unused by the Phase 19 stub but referenced in v3.1.
var _ = []interface{}{
    searchResult{}, episodeRow{}, serverRow{},
    malSyncEntry{}, malSyncResponse{},
}
```

### IN-03: Test count comment is a fragile contract

**File:** `services/scraper/internal/providers/animekai/client_test.go:174-182`
**Issue:** `TestProvider_ConformsToInterface` is a no-op marker test
documented as "so the test count matches the plan". This couples the
test file to an external plan document (19-01-PLAN.md) by test count
rather than by test name or capability. If the plan changes count, the
test file needs to change too — and the existing compile-time assertion
in `client.go:207` already provides the actual guarantee. Either remove
this test (the compile-time assertion is the real assertion) or make
its purpose intrinsic to the test code:

**Fix:**

```go
// Sentinel test — the real assertion is the compile-time line in
// client.go: `var _ domain.Provider = (*Provider)(nil)`. This wrapper
// exists so `go test -run TestProvider_ConformsToInterface` shows up
// in test runs as a documented capability.
func TestProvider_ConformsToInterface(t *testing.T) {
    var _ domain.Provider = (*Provider)(nil)
}
```

(Same body, better comment — drop the "test count" coupling.)

### IN-04: Documentation says `provider_health_up{provider="animekai"} flips to 0 after 3 consecutive 501s` but stub never calls the sidecar

**File:** `services/scraper/internal/providers/animekai/client.go:43-52`
**Issue:** The comment on `errAnimeKaiStub` claims:

> the probe runner flips provider_health_up{provider="animekai"} to 0
> after 3 consecutive 501s from the sidecar.

But the Phase 19 stub returns ErrProviderDown immediately from `GetStream`
without ever calling the sidecar `/animekai-token` endpoint. The probe
runner exercises the same `GetStream` method, so it will see
ErrProviderDown at the stream stage and flip the gauge — but for the
reason "stub returns error", not "sidecar returned 501". The two paths
are functionally equivalent for the gauge transition, but the comment
implies a code path that does not exist in Phase 19. This is purely a
documentation accuracy issue; it does not affect runtime behavior.

**Fix:** Reword the comment to match the actual mechanism:

> the probe runner flips provider_health_up{provider="animekai"} to 0
> on the first probe tick after registration (the stub returns
> ErrProviderDown synchronously; the sidecar 501 path is the v3.1 path
> when the Go body calls `/animekai-token`).

---

## Verification of prompt-flagged risks

| Risk | Verdict | Evidence |
|---|---|---|
| Flag wiring defaults off | OK | `config.go:117` `getEnvBool("SCRAPER_ANIMEKAI_ENABLED", false)`; `docker-compose.yml:168` `${SCRAPER_ANIMEKAI_ENABLED:-false}`; `.env.example:101` commented out |
| Stub returns ErrProviderDown not silent empty | OK | All four methods in `client.go:176-202` return zero value + wrapped error; tests `client_test.go:51-114` lock the contract |
| Sidecar returns 501 not 500 | OK | `server.js:253` `res.writeHead(501, ...)` |
| No enc-dec.app dependency leaked | OK | grep for `enc-dec\|encdec` across all reviewed files returns 0 hits in code; only mentioned in `.env.example`/`REQUIREMENTS.md` as the failure mode being avoided |
| Boot invariant + config parsing | OK at logic level; misleading error message (WR-05); but metric-seed contradicts escape-hatch contract (CR-01) |

---

_Reviewed: 2026-05-12_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
