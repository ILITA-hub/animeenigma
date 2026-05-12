---
phase: 16-animepahe-and-new-englishplayer
plan: 02
subsystem: scraper
tags: [scraper, animepahe, kwik, embed-extractor, goja, tdd, ssrf-guard]
requires:
  - services/scraper/internal/domain.EmbedExtractor interface (plan 15-02)
  - services/scraper/testdata/animepahe/kwik_e_abc.html golden fixture (plan 16-01)
provides:
  - services/scraper/internal/embeds.KwikExtractor — in-process Dean-Edwards unpacker for kwik.cx / kwik.si
  - services/scraper/internal/embeds.WithKwikTimeout / WithKwikHTTPClient — functional options
  - services/scraper/internal/embeds.NewKwikExtractor() constructor with sane defaults (5s goja timeout, 15s HTTP timeout, 2 MiB body cap)
  - extractPacker + balanceUntil paren/brace/string balancer (hand-rolled because regex can't reliably parse nested args)
affects:
  - services/scraper/go.mod (adds github.com/dop251/goja direct dep)
  - services/scraper/go.sum (lockfile updates incl. dlclark/regexp2, go-sourcemap/sourcemap, google/pprof, Masterminds/semver indirects)
tech-stack:
  added:
    - github.com/dop251/goja v0.0.0-20260311135729-065cd970411c (ECMA-5.1 interpreter in pure Go; no DOM / browser APIs needed for Dean-Edwards unpack)
  patterns:
    - In-process JS unpack (NOT a sidecar) — Kwik is self-contained, no DOM dependency
    - Fresh goja.Runtime per Extract() call (RESEARCH.md Pitfall 2 — goja is NOT thread-safe)
    - vm.Interrupt() from a separate watchdog goroutine (RESEARCH.md Pitfall 3 — same-goroutine interrupt can't cancel runaway RunString)
    - Hand-rolled paren/brace/string-literal balancer instead of regex for nested IIFE args
    - HTML-comment strip before scanning so example packers in doc-comments cannot shadow the real <script>
    - Functional-option constructor (WithKwikTimeout, WithKwikHTTPClient)
    - SSRF guard: host equality + strict subdomain (HasSuffix on "."+known); substring matching forbidden + tested against
    - io.LimitReader 2 MiB DoS cap on response body
key-files:
  created:
    - services/scraper/internal/embeds/kwik.go (461 lines)
  modified:
    - services/scraper/go.mod
    - services/scraper/go.sum
    - services/scraper/internal/embeds/kwik_test.go (already committed in f288706 RED; unchanged in GREEN)
decisions:
  - Replaced the plan's regex-only packer matcher with a hand-rolled three-stage paren/brace/string balancer (extractPacker + balanceUntil). Rationale below in Deviations.
  - Stripped HTML comments before scanning so the fixture's doc-comment example IIFE could not shadow the real <script>eval(...)</script> block. The fix is also defensive against real-world Kwik HTML that may include the same pattern.
  - Deferred github.com/PuerkitoBio/goquery from Task 1 GREEN to Plan 16-03. The plan instructed to add it here, but `go mod tidy` strips unused deps unconditionally; Plan 16-03 introduces the import that pins the dep naturally. Allowlist already covers it.
  - Combined Task 1 GREEN (deps wired) and Task 2 GREEN (KwikExtractor impl) into a single commit because `go mod tidy` would otherwise strip goja absent the kwik.go import — they are inseparable.
metrics:
  duration: ~30m
  completed: 2026-05-12T04:20:00Z
  tasks: 2
  files_created: 1
  files_modified: 3
---

# Phase 16 Plan 02: Kwik Embed Extractor (in-process goja unpacker) Summary

Ship the Kwik EmbedExtractor — a self-contained, in-process Dean-Edwards-packer unpacker for kwik.cx / kwik.si HLS embed URLs, satisfying SCRAPER-PAHE-03. Implementation is the second `domain.EmbedExtractor` in the registry (after Megacloud) so the AnimePahe provider in Plan 16-03 can route Kwik URLs to it transparently via `registry.Find(embedURL)`. All test cases pass deterministically against an offline golden fixture; the goja runtime is fresh per call and interruptible from a watchdog goroutine (RESEARCH.md Pitfalls 2 and 3).

## Files Created / Modified

| File | Lines | Purpose |
|---|---|---|
| `services/scraper/internal/embeds/kwik.go` | 461 | `KwikExtractor` — `domain.EmbedExtractor` for kwik.cx / kwik.si. Wraps a single GET → packed-HTML fetch → paren-balanced IIFE extract → goja unpack → m3u8 + quality-label parse → `*domain.Stream`. |
| `services/scraper/go.mod` | +1 require | Adds `github.com/dop251/goja v0.0.0-20260311135729-065cd970411c` direct dep. |
| `services/scraper/go.sum` | +N | Lockfile updates: goja + transitive (dlclark/regexp2, go-sourcemap/sourcemap, google/pprof, Masterminds/semver). |

The RED scaffold (`services/scraper/internal/embeds/kwik_test.go`, 350 lines) was committed in the prior session as `f288706 test(16-02): add failing scaffold tests for KwikExtractor (RED)` and was NOT modified in the GREEN phase — the implementation satisfies the existing scaffold without test changes.

## Tasks Executed

| Task | Status | Commit | Notes |
|---|---|---|---|
| Task 1 RED — failing test scaffold | already done | `f288706` (pre-resume) | 8 test functions covering Name, Matches positive/negative, SSRF imposter set, golden-fixture decode, no-packer, ctx-cancel, timeout (with WithKwikTimeout), fresh-runtime concurrency (16 goroutines), and 2 MiB body limit. |
| Task 1 GREEN — wire deps + Task 2 GREEN — implement KwikExtractor | done | `f17d90e` | Combined into single commit (see Deviations below). |

## Test Results

```text
$ cd services/scraper && go test ./internal/embeds -count=1 -timeout 60s -v -run TestKwik
--- PASS: TestKwik_Name                                  (0.00s)
--- PASS: TestKwik_Matches                               (0.00s) [12 subtests]
--- PASS: TestKwik_Matches_RejectsSubdomainImposters     (0.00s) [5 subtests, SSRF guard]
--- PASS: TestKwik_Extract_GoldenFixture                 (0.00s)
--- PASS: TestKwik_Extract_NoPacker                      (0.00s)
--- PASS: TestKwik_Extract_ContextCancel                 (0.00s)
--- PASS: TestKwik_Extract_Timeout                       (0.05s) [goja Interrupt path]
--- PASS: TestKwik_Extract_FreshRuntime                  (0.01s) [16 concurrent extracts]
--- PASS: TestKwik_Extract_RespectsBodyLimit             (0.01s) [5 MiB body, 2 MiB cap]
ok    github.com/ILITA-hub/animeenigma/services/scraper/internal/embeds    0.058s
```

Test count by category:
- 1 Name pin
- 12 Matches positive/negative subtests
- 5 SSRF imposter subtests (`kwik.cx.attacker.com`, `kwik.si.evil.example.org`, `akwik.cx`, `akwik.si`, `notkwik.cx`)
- 6 Extract behavior tests (golden, no-packer, ctx-cancel, timeout, fresh-runtime concurrency, body-limit)

Full scraper test suite:
```text
$ go test ./... -count=1 -timeout 90s
ok    .../internal/domain      0.508s
ok    .../internal/embeds      0.067s
ok    .../internal/golint      0.003s  (forbidden-deps lint)
ok    .../internal/handler     0.007s
ok    .../internal/service     0.063s
ok    .../internal/testharness 0.005s
ok    .../internal/transport   0.010s
```

## Key Implementation Choices

### 1. Hand-rolled paren balancer instead of regex (replaces the plan's `packedJSRegex`)

The plan specified a regex `(?s)eval\((function\(p,a,c,k,e,d\).*?\}\([^)]*\))\)` to capture the IIFE. Two problems surfaced during GREEN:

1. The golden fixture's HTML comment contains the literal string `eval(function(p,a,c,k,e,d){...}(...))` as documentation. The non-greedy `.*?` matches that doc-comment example FIRST (shorter match wins) instead of the real `<script>` block.
2. Real Dean-Edwards IIFE args contain nested parens (e.g. `'placeholder'.split('|')`) AND trailing `{}` (the unused dict arg `0,{}`). The `[^)]*` arg matcher rejects the first because of `(`/`)`, while a `.*?` lookahead can match incorrectly into adjacent JS.

Fix:
- Pre-pass: strip HTML comments via `htmlCommentRegex` before any scanning.
- Replace the IIFE-capture regex with `extractPacker(body string) (string, bool)`. This function locates `eval(function(p,a,c,k,e,d)` via a narrow anchor regex, then balances the three structural regions of the IIFE in turn — param list `(...)`, function body `{...}`, call args `(...)` — using `balanceUntil(body, start, open, close byte)`. The balancer tracks string-literal state (`'...'` / `"..."`) and respects backslash-escapes (Dean-Edwards emits `\'` inside the packed string).

This is roughly 70 lines more code than the regex would have been but eliminates the regex's two failure modes. It also generalizes to future Kwik packer variants that might embed additional nested parens.

### 2. Quality-label annotation via ±80-char window scan

Real Kwik unpacked output has `const source='primary.m3u8';const sources=[{file:'...',label:'480p'},...]`. We capture every unique m3u8 URL via `sourceURLRegex` and then, for each, scan a ±80-char window around the URL's offset in the unpacked string for the nearest `qualityLabelRegex` match. Absent labels just leave `Source.Quality = ""` — non-fatal.

### 3. Watchdog goroutine + `defer close(done)`

Per RESEARCH.md Pitfall 3, `vm.Interrupt()` must come from a goroutine other than the one running `RunString`. The watchdog goroutine selects on three channels: `<-time.After(k.timeout)`, `<-ctx.Done()`, `<-done`. The third channel exists so a normal completion can exit the goroutine promptly — `defer close(done)` ensures we never leak the watchdog even if `RunString` panics. `TestKwik_Extract_Timeout` and `TestKwik_Extract_ContextCancel` both prove this path: timeout fires within 100ms of a 50ms cap; context cancel returns within 1s on a 10s-blocking server.

### 4. SSRF guard via host-equality + strict subdomain

Mirrors the megacloud.go pattern exactly. `Matches(embedURL)`:
1. `url.Parse(embedURL)` — malformed URLs reject.
2. `strings.ToLower(u.Hostname())` — case-insensitive host (RFC 3986).
3. For each `known` in `kwikHosts`: accept iff `host == known || strings.HasSuffix(host, "."+known)`.

`TestKwik_Matches_RejectsSubdomainImposters` locks this against five regression cases: `kwik.cx.attacker.com`, `kwik.si.evil.example.org`, `akwik.cx`, `akwik.si`, `notkwik.cx`. A regression to substring matching (`strings.Contains`) would light up by name in CI.

## Threat-Model Mitigations Verified

| Threat ID | Mitigation | Verification |
|---|---|---|
| T-16-02-01 (SSRF via Matches) | Host equality + `"."+known` suffix check | `TestKwik_Matches_RejectsSubdomainImposters` passes; 5 imposter cases |
| T-16-02-02 (goja sandbox escape) | Fresh `goja.New()`; no fs/net/process bindings; no custom global injection | Source inspection: `grep -c "goja.New()" kwik.go` returns 1 (production call); the only globals injected are none. |
| T-16-02-03 (DoS via infinite-loop JS / huge body) | `io.LimitReader(body, 2 MiB)` + `vm.Interrupt()` from watchdog | `TestKwik_Extract_Timeout` fires within ~100ms on infinite loop; `TestKwik_Extract_RespectsBodyLimit` returns ErrExtractFailed (no OOM) on 5 MiB junk body. |
| T-16-02-04 (data race on shared goja runtime) | Fresh runtime per call; no caching | `TestKwik_Extract_FreshRuntime` runs 16 concurrent extracts; all return identical Streams. (For full `-race` coverage: `go test -race ./internal/embeds`.) |
| T-16-02-06 (silent extraction failures) | Every error path wraps via `WrapExtractFailed` or `WrapProviderDown` | `TestKwik_Extract_NoPacker` asserts `errors.Is(err, ErrExtractFailed)` and message contains "no eval() packer". |

T-16-02-05 (stream URL points to attacker host) is a downstream-defense item: `libs/videoutils/proxy.go::HLSProxyAllowedDomains` allowlists AnimePahe's CDN hosts; out of scope for the extractor itself.

## Regex / Anchor Reference

For audit traceability, the four runtime regexes in `kwik.go`:

| Regex | Purpose |
|---|---|
| `htmlCommentRegex` = `(?s)<!--.*?-->` | Strip HTML comments before scanning. |
| `packerStartRegex` = `eval\(function\(p,a,c,k,e,d\)` | Anchor the IIFE entry point. |
| `sourceURLRegex` = `(?:const|var|let|file\s*:)\s*(?:source\s*=\s*)?\\?['"]?(https?://[^'"\\\s]+\.m3u8[^'"\\\s]*)` | Capture m3u8 URLs from the unpacked source. Tolerant of `const`/`var`/`let`/`file:` declarations and escaped/unescaped quotes. |
| `qualityLabelRegex` = ``label\s*:\s*\\?['"]?(\d{3,4}p)`` | Best-effort quality annotation (e.g. "720p"). |

## Deviations from Plan

### 1. [Rule 1 - Bug] Replaced regex-based packer capture with paren balancer

**Found during:** Task 2 GREEN (first test run after kwik.go landed)

**Issue:** The plan's `packedJSRegex` (`(?s)eval\((function\(p,a,c,k,e,d\).*?\}\([^)]*\))\)`) failed against the golden fixture in two ways:
1. The fixture's HTML comment contained an example `eval(function(p,a,c,k,e,d){...}(...))` string used as DOCUMENTATION of what the regex was supposed to match. The non-greedy `.*?` matched that example FIRST, capturing the literal string `function(p,a,c,k,e,d){...}(...)` — which goja then rejected with `SyntaxError: Unexpected token ...`.
2. The arg list `('packed',62,1,'placeholder'.split('|'),0,{})` contains nested parens (`.split('|')`) AND a trailing dict `{}`. The `[^)]*` arg matcher could not reach the outer `))`.

**Fix:** Two-stage solution:
1. Strip HTML comments via `htmlCommentRegex.ReplaceAll(body, "")` before any scanning.
2. Replace the IIFE-capture regex with a hand-rolled `extractPacker` + `balanceUntil` paren / brace / string balancer.

**Files modified:** services/scraper/internal/embeds/kwik.go (replaced `packedJSRegex` with `packerStartRegex` + `htmlCommentRegex` + `extractPacker` + `balanceUntil`).

**Commit:** f17d90e

### 2. [Rule 3 - Blocking] Combined Task 1 GREEN and Task 2 GREEN into one commit

**Found during:** Task 1 GREEN (`go mod tidy` after `go get goja@latest`)

**Issue:** The plan's Task 1 GREEN sequence (`go get goja; go get goquery; go mod tidy; commit`) would not survive `go mod tidy` — Go strips unused direct deps unconditionally. Both packages are unused at the end of Task 1; only Task 2's kwik.go actually imports goja. The plan implicitly assumed they could be wired with a standalone commit, but that's structurally impossible without a placeholder import.

**Fix:** Combined Task 1 GREEN and Task 2 GREEN into a single commit (`f17d90e`). The kwik.go implementation imports `github.com/dop251/goja`, which causes `go mod tidy` to retain the direct dep. The RED/GREEN history is still complete: `f288706` (RED) → `f17d90e` (GREEN).

**Files modified:** No additional files beyond what Task 2 already touched.

**Commit:** f17d90e (single combined commit)

### 3. [Rule 3 - Scope] Deferred github.com/PuerkitoBio/goquery from this plan to Plan 16-03

**Found during:** Task 1 GREEN

**Issue:** Same root cause as Deviation 2 — `go mod tidy` strips goquery because no .go file imports it in Plan 16-02. The plan explicitly anticipated this with the note "Do NOT remove it in go mod tidy after this task; 16-03 needs it", but that note is unenforceable: `go mod tidy` is non-interactive and Go does not support "keep this dep around for later" annotations short of a placeholder import (e.g. `tools.go` build-tagged file) — which would be wholly synthetic infrastructure for a one-plan gap.

**Fix:** Deferred to Plan 16-03. The forbidden-deps allowlist already covers goquery (forbidden_deps_test.go:227), so Plan 16-03's `go get goquery` will be admitted by CI without further changes here.

**Files modified:** Documented in this Deviations section; no files changed.

## Verification (plan checklist)

- [x] `cd services/scraper && go test ./internal/embeds -count=1 -timeout 60s -v` — every test green, includes golden-fixture decode offline
- [x] `cd services/scraper && go vet ./internal/embeds` — clean
- [x] `cd services/scraper && go test ./internal/golint/... -count=1 -timeout 30s` — forbidden-deps lint green
- [x] `grep -c "goja.New()" services/scraper/internal/embeds/kwik.go` = 2 (1 comment + 1 call); >= 1 ✓
- [x] `grep -c "vm.Interrupt" services/scraper/internal/embeds/kwik.go` = 4 (3 comments + 1 call); >= 1 ✓
- [x] `grep -c "io.LimitReader" services/scraper/internal/embeds/kwik.go` = 2 (1 comment + 1 call); >= 1 ✓
- [x] `git log --oneline -10 | grep "16-02"` shows both `f288706 test(16-02): ... (RED)` and `f17d90e feat(16-02): ... (GREEN)`

## Known Stubs

None.

## TDD Gate Compliance

RED commit: `f288706 test(16-02): add failing scaffold tests for KwikExtractor (RED)` (prior session).
GREEN commit: `f17d90e feat(16-02): implement KwikExtractor via dop251/goja with fresh-runtime-per-call (GREEN)`.

No REFACTOR commit — the GREEN implementation landed clean (paren-balancer fix WAS the GREEN debug iteration, not a refactor of a working implementation).

## Followups

- Plan 16-03 (AnimePahe provider): imports goquery; registers KwikExtractor in the embeds.Registry; uses `registry.Find(embedURL)` to dispatch Kwik URLs returned from AnimePahe's `/play/{anime}/{ep}` HTML to this extractor.
- `go test -race ./internal/embeds` was not run in this session (default `go test` only). The fresh-runtime test still surfaces concurrency-related decode failures, but a future plan should add a CI step that runs `-race` on the embeds package for stronger data-race coverage.

## Self-Check: PASSED

- [x] services/scraper/internal/embeds/kwik.go exists (461 lines) — verified via `wc -l`
- [x] services/scraper/go.mod has direct require `github.com/dop251/goja` — verified via `grep`
- [x] Commit f17d90e exists in `git log --oneline -5` — verified
- [x] Commit f288706 (prior RED) exists — verified
- [x] All 9 Kwik tests pass via `go test ./internal/embeds -count=1 -timeout 60s -v -run TestKwik`
- [x] Forbidden-deps lint stays green via `go test ./internal/golint/... -count=1`
- [x] Full scraper test suite green via `go test ./... -count=1 -timeout 90s`
