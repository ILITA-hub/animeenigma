---
phase: 21
plan: 01
subsystem: libs/streamprobe
tags: [streamprobe, scraper, playability-gate, ad-decoy, hls, ssrf]
requires: []
provides:
  - "libs/streamprobe.Probe(ctx, masterURL, headers) Result"
  - "libs/streamprobe.Reason (typed string, 7 values)"
  - "libs/streamprobe.AllReasons()"
  - "libs/streamprobe.isAdCDNHost(host) bool (package-private)"
affects:
  - "go.work (workspace registration)"
tech_stack_added: []
tech_stack_patterns:
  - "stdlib-only libs/ package (no external deps)"
  - "table-tested Reason enum with declaration-order assertion"
  - "httptest.NewServer-driven m3u8 fixtures in testdata/"
  - "test-only escape hatch via package-private var (allowLoopbackForTests)"
key_files_created:
  - libs/streamprobe/go.mod
  - libs/streamprobe/reason.go
  - libs/streamprobe/blocklist.go
  - libs/streamprobe/probe.go
  - libs/streamprobe/reason_test.go
  - libs/streamprobe/blocklist_test.go
  - libs/streamprobe/probe_test.go
  - libs/streamprobe/testdata/playable_master.m3u8
  - libs/streamprobe/testdata/playable_variant.m3u8
  - libs/streamprobe/testdata/ad_decoy_variant.m3u8
  - libs/streamprobe/testdata/zero_match_no_extm3u.m3u8
  - libs/streamprobe/testdata/empty_variant.m3u8
key_files_modified:
  - go.work
decisions:
  - "Reason enum lives in libs/streamprobe/reason.go as typed string (D7)"
  - "Gate lives in libs/streamprobe/ as a shared lib for scraper + scheduler canary (D1)"
  - "Hardcoded blocklist in source code; Redis-lift TODO anchored at spec §4.1.c-TODO for when list grows or maintenance bot needs to extend without redeploy"
  - "Test-only escape hatch (allowLoopbackForTests var) for httptest.NewServer 127.0.0.1 binding — production code cannot flip; defense-in-depth: RFC1918 + link-local still always blocked"
metrics:
  duration_minutes: ~25
  completed_date: 2026-05-13
  tasks_completed: 2
  files_created: 12
  files_modified: 1
  tests_added: 28
  commits:
    - 6aeac90 "feat(21-01): scaffold libs/streamprobe package with Reason enum + ad-CDN blocklist"
    - abe5199 "feat(21-01): implement Probe — master m3u8 walk + ad-CDN gate + SSRF guard"
---

# Phase 21 Plan 01: libs/streamprobe Package Summary

Stand-up of the shared playability-gate library — `Probe(ctx, masterURL, headers) Result` — that walks master m3u8 → first variant → first-segment HEAD, classifies into one of seven Reason outcomes, short-circuits on a hardcoded ad-CDN host-suffix blocklist BEFORE leaking our IP to TikTok's ad CDN, and defends against SSRF before any dial. Wave-1 dependency for Plan 21-03 (gogoanime gate integration) and Phase 23 (scraper canary cron).

## What Shipped

### Package surface

```go
// libs/streamprobe
type Reason string
const (
    ReasonPlayable         Reason = "playable"
    ReasonAdDecoy          Reason = "ad_decoy"
    ReasonZeroMatch        Reason = "zero_match"
    ReasonStatus403        Reason = "status_403"
    ReasonSignedURLExpired Reason = "signed_url_expired"
    ReasonCDNUnreachable   Reason = "cdn_unreachable"
    ReasonEmptyResponse    Reason = "empty_response"
)
func AllReasons() []Reason

type Result struct {
    Playable bool     // true only when Reason == ReasonPlayable
    Reason   Reason
    Sampled  []string // hostnames observed during the walk
}

func Probe(ctx context.Context, masterURL string, headers http.Header) Result
```

All seven Reason string values match the stable Prometheus label tokens called out in the maintenance prompt Patterns 6/7 reason-enum dispatch table.

### Ad-CDN host-suffix blocklist (SCRAPER-HEAL-02)

Hardcoded in `libs/streamprobe/blocklist.go`:

| Suffix | Source |
|---|---|
| `ibyteimg.com` | PoC 2026-05-13 — VibePlayer ad-decoy CDN |
| `p16-ad-sg` | Hostname-prefix match (`p16-ad-sg.ibyteimg.com`) — TikTok region tag |
| `ad-site-i18n` | TikTok i18n ad CDN (subdomain pattern) |
| `tiktokcdn.com` | TikTok CDN root |

Matcher uses `strings.Contains` rather than pure suffix match — production poison `p16-ad-sg.ibyteimg.com` is a hostname PREFIX, not a suffix, so the same loop catches both prefix-style and suffix-style entries without per-entry config.

`// TODO:` multi-line block anchored to spec §4.1.c-TODO: when the list grows past ~10 entries OR the maintenance bot needs to extend it without a redeploy, lift into Redis at key `scraper:streamprobe:blocklist`. Test `TestBlocklist_RedisLiftTODOAnchored` enforces both anchors are present in CI.

### Probe algorithm budgets

- **Per-step timeout:** 4s (master GET, variant GET, segment HEAD each) — enforced via `context.WithTimeout` per request + dial/TLS/response-header timeouts on the transport.
- **Total budget:** 10s — outer `context.WithTimeout` wraps the entire Probe call.
- **Body cap:** 1 MiB via `io.LimitReader` on every fetch (DoS guard on hostile playlists, T-21-02).

### SSRF defense (T-21-01)

`isPublicHost` rejects loopback / RFC1918 (10/8, 172.16/12, 192.168/16) / link-local (169.254/16) / unspecified BEFORE any dial. Hostnames-not-IPs are allowed (DNS resolves at dial time, where the stdlib dialer will re-check). Literal `localhost` / `*.localhost` blocked.

Test-only escape hatch: `allowLoopbackForTests` package-private bool, flipped to true by `TestMain` so httptest.NewServer (127.0.0.1) can drive the suite. SSRF tests explicitly toggle it OFF and verify the guard short-circuits within < 100ms — well below the 4s dial timeout that an actual dial would take to fail. RFC1918 + link-local rejection is NOT gated by this flag — defense-in-depth.

### Ad-CDN short-circuit verification (T-21-03)

`TestProbe_AdDecoy` registers a mock `httptest.NewServer` simulating the ad-CDN with an atomic request counter. The fixture's segment URL points at the production-poison hostname `p16-ad-sg.ibyteimg.com`. Test asserts the counter remains at zero after Probe runs — proving the blocklist hits BEFORE the HEAD probe so we never leak our IP to TikTok.

### Signed-URL-expired heuristic

`classify403` parses `?e=<unix-seconds>` or `?expires=<unix-seconds>` query params on a 403 response. If `now > epoch`, returns `ReasonSignedURLExpired` (recoverable by re-fetching upstream) instead of generic `ReasonStatus403`. Regex requires 8-12 digits to avoid false positives on short numeric IDs.

## Verification Evidence

| Check | Command | Result |
|---|---|---|
| Workspace sync | `go work sync` | exits 0 |
| Build | `go build ./libs/streamprobe/...` | exits 0 |
| Vet | `go vet ./libs/streamprobe/...` | exits 0 |
| All tests + race | `cd libs/streamprobe && go test ./... -count=1 -race` | `ok ... 7.033s` — 28 tests pass |
| Workspace registration | `grep -c "./libs/streamprobe" go.work` | 1 |
| Reason enum present | `grep -c "ReasonPlayable" libs/streamprobe/reason.go` | 2 |
| Blocklist entry | `grep -c "ibyteimg.com" libs/streamprobe/blocklist.go` | 2 |
| Redis-lift anchor | `grep -c "scraper:streamprobe:blocklist" libs/streamprobe/blocklist.go` | 1 |
| All Reasons covered | `TestProbe_AllReasonsCovered` meta-test | PASS |

### Test coverage matrix

| Reason | Driving test | Method of validation |
|---|---|---|
| `playable` | `TestProbe_Playable`, `TestProbe_RelativeSegmentURI` | full master → variant → segment HEAD 200 |
| `ad_decoy` | `TestProbe_AdDecoy` | blocklist short-circuits BEFORE HEAD; ad-CDN mock receives 0 hits (atomic counter) |
| `zero_match` | `TestProbe_ZeroMatch_NotM3U8` | master responds with html (no `#EXTM3U` sentinel) |
| `status_403` | `TestProbe_Status403`, `TestProbe_SegmentHEAD_403` | master returns 403 without `?e=`; segment HEAD returns 403 |
| `signed_url_expired` | `TestProbe_SignedURLExpired` | master returns 403 at `?e=1000000000` |
| `cdn_unreachable` | `TestProbe_CDNUnreachable`, `TestProbe_PerStepTimeout`, `TestProbe_SSRF_*`, `TestProbe_CtxCancelled` | closed-server dial fails; sleeping server cuts at per-step timeout; SSRF guards short-circuit; ctx cancelled before HEAD |
| `empty_response` | `TestProbe_EmptyResponse` | valid `#EXTM3U` body with zero `#EXTINF` segments |

### Performance assertions

- `TestProbe_PerStepTimeout` — sleeping master cut at ≤ 5s wall-clock (4s budget + 1s slack).
- `TestProbe_SSRF_Loopback` — < 100ms wall-clock confirms guard short-circuits BEFORE dial.

## Deviations from Plan

### Auto-fixed during execution

**1. [Rule 1 — Bug] UTF-8 BOM literal in source caused `vet: illegal byte order mark` compile error**
- Found during: Task 2 build/vet
- Issue: Plan-supplied snippet used `strings.TrimLeft(s, "﻿ \t\r\n")` with a literal BOM character in the cutset; Go's source parser rejects raw BOM mid-file.
- Fix: Split into two operations — `bytes.TrimPrefix(body, utf8BOM)` (where `utf8BOM = []byte{0xEF, 0xBB, 0xBF}`) followed by `strings.TrimLeft(string(body), " \t\r\n")`. Cleaner and avoids the mid-file BOM restriction.
- Files modified: libs/streamprobe/probe.go
- Commit: abe5199

**2. [Rule 1 — Bug] SSRF guard too strict for httptest-driven unit tests**
- Found during: Task 2 test runs (every Probe test failed with `Reason=cdn_unreachable` because `httptest.NewServer` binds to 127.0.0.1)
- Issue: The production SSRF guard correctly rejects 127.0.0.0/8 — but with no escape hatch, the entire test suite is unrunnable.
- Fix: Added package-private `allowLoopbackForTests` bool, flipped to true in `TestMain`. SSRF tests explicitly toggle it OFF to validate production behavior. Defense-in-depth preserved: RFC1918 + link-local rejection is NOT gated by the flag — only loopback is escape-hatched.
- Why not a public option: the gate is a fixed-behavior contract for production callers; making it configurable risks future misuse. The escape hatch lives entirely in the same package so external callers can never set it.
- Files modified: libs/streamprobe/probe.go, libs/streamprobe/probe_test.go
- Commit: abe5199

### Spec-vs-output filename note

Plan's `<output>` section called for `21-21-01-SUMMARY.md` (typo — phase number duplicated). Orchestrator brief asked for `21-01-SUMMARY.md`, which matches the GSD convention. Used the orchestrator's filename.

## Threat Model Coverage

| Threat | Disposition | Implementation evidence |
|---|---|---|
| T-21-01 (SSRF via masterURL) | mitigate | `isPublicHost` rejects loopback/RFC1918/link-local before dial; `TestProbe_SSRF_*` asserts < 100ms short-circuit |
| T-21-02 (DoS via hostile playlist body) | mitigate | `io.LimitReader` at 1 MiB on every fetch; per-step 4s timeout; total 10s budget |
| T-21-03 (info disclosure to TikTok ad CDN) | mitigate | `isAdCDNHost` short-circuits before HEAD; `TestProbe_AdDecoy` asserts ad-CDN mock receives 0 hits via atomic counter |
| T-21-04 (Reason enum drift) | mitigate | `AllReasons()` helper + `TestProbe_AllReasonsCovered` meta-test enforces every Reason has a driving test |
| T-21-05 (Probe failures invisible to ops) | accept | Plan 21-02 ships `parser_unplayable_total` metric; this plan only delivers the library |

## Known Stubs

None — Probe is fully wired, all 7 Reason classifications are exercised, no placeholder return values.

## What's Unblocked

- **Plan 21-03** (gogoanime server-priority + per-server fallback using the gate) — can now import `github.com/ILITA-hub/animeenigma/libs/streamprobe` and call `Probe(ctx, candidate.URL, headers)` in a priority loop.
- **Phase 23** (scraper canary cron) — same import path; runs Probe across canary anime daily.

## Self-Check: PASSED

Verification commands:

```bash
$ ls libs/streamprobe/
blocklist.go  blocklist_test.go  go.mod  probe.go  probe_test.go  reason.go  reason_test.go  testdata/
$ git log --oneline --all | grep -E "(6aeac90|abe5199)"
abe5199 feat(21-01): implement Probe — master m3u8 walk + ad-CDN gate + SSRF guard
6aeac90 feat(21-01): scaffold libs/streamprobe package with Reason enum + ad-CDN blocklist
$ cd libs/streamprobe && go test ./... -count=1 -race
ok  	github.com/ILITA-hub/animeenigma/libs/streamprobe	7.033s
```

All files present, both commits exist, full suite passes with `-race`.
