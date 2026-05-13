---
id: 21-01
phase: 21
plan: 01
type: execute
wave: 1
depends_on: []
files_modified:
  - go.work
  - libs/streamprobe/go.mod
  - libs/streamprobe/go.sum
  - libs/streamprobe/reason.go
  - libs/streamprobe/blocklist.go
  - libs/streamprobe/probe.go
  - libs/streamprobe/probe_test.go
  - libs/streamprobe/reason_test.go
  - libs/streamprobe/blocklist_test.go
  - libs/streamprobe/testdata/playable_master.m3u8
  - libs/streamprobe/testdata/playable_variant.m3u8
  - libs/streamprobe/testdata/ad_decoy_variant.m3u8
  - libs/streamprobe/testdata/zero_match_no_extm3u.m3u8
  - libs/streamprobe/testdata/empty_variant.m3u8
autonomous: true
requirements:
  - SCRAPER-HEAL-01
  - SCRAPER-HEAL-02
user_setup: []
must_haves:
  truths:
    - "Probe(ctx, masterURL, headers) returns Result{Playable bool, Reason Reason, Sampled []string}"
    - "All seven Reason values exist as typed string consts (playable, ad_decoy, zero_match, status_403, signed_url_expired, cdn_unreachable, empty_response)"
    - "Hardcoded ad-CDN host-suffix blocklist matches ibyteimg.com, p16-ad-sg, ad-site-i18n, tiktokcdn.com"
    - "Per-step timeout 4s; total Probe budget ≤ 10s enforced via context"
    - "SSRF safe: outbound URLs blocked from 127.0.0.0/8, 10.0.0.0/8, 169.254.0.0/16, 192.168.0.0/16, ::1"
    - "go.work lists ./libs/streamprobe"
  artifacts:
    - path: "libs/streamprobe/reason.go"
      provides: "type Reason string + 7 const values"
      contains: "type Reason string"
    - path: "libs/streamprobe/blocklist.go"
      provides: "var adCDNHostSuffixes []string + isAdCDNHost(host) bool"
      contains: "ibyteimg.com"
    - path: "libs/streamprobe/probe.go"
      provides: "func Probe(ctx context.Context, masterURL string, headers http.Header) Result"
      exports: ["Probe", "Result", "Reason"]
    - path: "libs/streamprobe/probe_test.go"
      provides: "table tests covering all 7 Reason values with synthetic m3u8 fixtures"
      contains: "TestProbe_AdDecoy"
  key_links:
    - from: "go.work"
      to: "libs/streamprobe"
      via: "use directive"
      pattern: "./libs/streamprobe"
    - from: "libs/streamprobe/probe.go"
      to: "libs/streamprobe/blocklist.go"
      via: "isAdCDNHost call on each segment URI host"
      pattern: "isAdCDNHost"
---

<objective>
Stand up the shared `libs/streamprobe/` package — the playability gate that classifies a master m3u8 into one of seven Reason outcomes by walking master → first variant → first segment HEAD with a hardcoded ad-CDN host-suffix blocklist. SCRAPER-HEAL-01 + SCRAPER-HEAL-02.

Purpose: Plan 21-03 (gogoanime integration) and Phase 23 canary cron both need this lib. Built first (Wave 1) with zero scraper-service code so the package can be unit-tested in isolation with synthetic m3u8 fixtures.

Output: A go-workspace-registered library with `Probe(ctx, masterURL, headers) Result` exported, table-tested for all 7 Reason values, and a TODO-stub comment in blocklist.go pointing at spec §4.1.c-TODO (Redis-lift trigger).
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/PROJECT.md
@.planning/ROADMAP.md
@.planning/STATE.md
@.planning/phases/21-playability-foundation/21-CONTEXT.md
@docs/plans/2026-05-13-scraper-self-healing-spec.md
@CLAUDE.md

<interfaces>
<!-- Existing patterns. Use these directly — no exploration needed. -->

From libs/cache/cache.go (libs/ go.mod convention — single-version go.mod, no replace blocks):
```go
module github.com/ILITA-hub/animeenigma/libs/cache
go 1.23.0
```

From libs/metrics/provider.go (NewCounterVec pattern — for reference, this plan does NOT add metrics):
```go
var ParserZeroMatchTotal = promauto.NewCounterVec(
    prometheus.CounterOpts{Name: "parser_zero_match_total", Help: "..."},
    []string{"provider", "selector"},
)
```

From go.work (current workspace use block — append `./libs/streamprobe`):
```
go 1.23.0
use (
    ./libs/animeparser
    ./libs/authz
    ./libs/cache
    ./libs/database
    ./libs/errors
    ./libs/httputil
    ./libs/idmapping
    ./libs/logger
    ./libs/metrics
    ./libs/pagination
    ./libs/tracing
    ./libs/videoutils
    ...
)
```

Memory note "Adding New libs/ Module" — for THIS plan only steps 1 + library go.mod are
required. Steps 2/3 (services/scraper/go.mod + Dockerfile COPY) are done in Plan 21-03
(the consumer plan).
</interfaces>
</context>

<tasks>

<task type="auto" tdd="true">
  <name>Task 1: Scaffold libs/streamprobe package — go.mod, Reason enum, blocklist with Redis-lift TODO</name>
  <files>libs/streamprobe/go.mod, libs/streamprobe/reason.go, libs/streamprobe/blocklist.go, libs/streamprobe/reason_test.go, libs/streamprobe/blocklist_test.go, go.work</files>
  <read_first>
    - libs/cache/go.mod (libs/ go.mod convention: module path, single-version, no replace blocks needed for stdlib-only lib)
    - libs/metrics/provider.go (Go file header comment style — "Package metrics — ..." one-line description + multi-line rationale)
    - go.work (existing use directive ordering — append ./libs/streamprobe alphabetically between ./libs/pagination and ./libs/tracing)
    - docs/plans/2026-05-13-scraper-self-healing-spec.md §4.1.c (Reason enum values) and §4.1.c-TODO (Redis-lift trigger)
    - .planning/phases/21-playability-foundation/21-CONTEXT.md D7 (Reason enum lives in libs/streamprobe/reason.go as typed string)
  </read_first>
  <behavior>
    - Test: reason.go exports `type Reason string` + 7 const values (Playable, AdDecoy, ZeroMatch, Status403, SignedURLExpired, CDNUnreachable, EmptyResponse) all with lowercase snake_case string values matching the metric label tokens.
    - Test: AllReasons() returns the 7-element slice in declaration order (used by tests to assert the maintenance-prompt covers every value).
    - Test: blocklist.isAdCDNHost("foo.ibyteimg.com") == true; ("ibyteimg.com") == true; ("p16-ad-sg.foo.com") == true; ("ad-site-i18n.example.org") == true; ("tiktokcdn.com") == true; ("example.com") == false; case-insensitive.
    - Test: blocklist.go contains a `// TODO:` block (≥ 3 lines) referencing `scraper:streamprobe:blocklist` Redis key and the spec §4.1.c-TODO trigger conditions.
  </behavior>
  <action>
    1. **Create libs/streamprobe/go.mod**:
       ```
       module github.com/ILITA-hub/animeenigma/libs/streamprobe
       go 1.23.0
       ```
       No external deps in this task. (probe.go in Task 2 only uses stdlib.)
    2. **Append to go.work** — insert `./libs/streamprobe` in the `use ( ... )` block, alphabetically between `./libs/pagination` and `./libs/tracing`.
    3. **Create libs/streamprobe/reason.go** with package-level doc, typed string, 7 consts. Use exact const values:
       ```go
       package streamprobe

       // Reason classifies the outcome of Probe. The string values are stable
       // Prometheus label tokens — DO NOT rename without coordinating with
       // .claude/maintenance-prompt.md Pattern 6/7 reason-enum dispatch table.
       type Reason string

       const (
           ReasonPlayable        Reason = "playable"
           ReasonAdDecoy         Reason = "ad_decoy"
           ReasonZeroMatch       Reason = "zero_match"
           ReasonStatus403       Reason = "status_403"
           ReasonSignedURLExpired Reason = "signed_url_expired"
           ReasonCDNUnreachable  Reason = "cdn_unreachable"
           ReasonEmptyResponse   Reason = "empty_response"
       )

       // AllReasons returns every defined Reason value in declaration order.
       // Used by tests to verify exhaustive maintenance-prompt coverage.
       func AllReasons() []Reason {
           return []Reason{
               ReasonPlayable, ReasonAdDecoy, ReasonZeroMatch, ReasonStatus403,
               ReasonSignedURLExpired, ReasonCDNUnreachable, ReasonEmptyResponse,
           }
       }
       ```
    4. **Create libs/streamprobe/blocklist.go** with the hardcoded host-suffix list, the Redis-lift TODO block, and case-insensitive matcher:
       ```go
       package streamprobe

       import "strings"

       // TODO(spec §4.1.c-TODO, Redis-lift trigger):
       //   When this slice grows past ~10 entries OR the maintenance bot needs
       //   to extend it without a redeploy, lift into Redis at key
       //   `scraper:streamprobe:blocklist` (sorted set or list of suffixes).
       //   The Redis path takes precedence; this hardcoded slice becomes the
       //   bootstrap default loaded once at scraper startup if Redis is empty.
       //   Tracked: docs/plans/2026-05-13-scraper-self-healing-spec.md §4.1.c-TODO.
       var adCDNHostSuffixes = []string{
           "ibyteimg.com",
           "p16-ad-sg",       // matches p16-ad-sg.* (TikTok ad CDN region tag)
           "ad-site-i18n",    // matches *.ad-site-i18n.* (TikTok i18n ad CDN)
           "tiktokcdn.com",
       }

       // isAdCDNHost reports whether host matches any blocklisted suffix.
       // Case-insensitive. Empty host returns false.
       func isAdCDNHost(host string) bool {
           if host == "" { return false }
           h := strings.ToLower(host)
           for _, suf := range adCDNHostSuffixes {
               s := strings.ToLower(suf)
               if h == s || strings.HasSuffix(h, "."+s) || strings.Contains(h, s) {
                   return true
               }
           }
           return false
       }
       ```
       Notes:
         * `strings.Contains` rather than pure suffix match because `p16-ad-sg` appears as a HOSTNAME PREFIX in production poison (`p16-ad-sg.ibyteimg.com`), not a suffix. Contains catches both the prefix-style and the suffix-style entries in the same loop.
         * Test cases below MUST cover both `p16-ad-sg.ibyteimg.com` (hits TWO entries — that is fine; we OR, not AND).
    5. **Create libs/streamprobe/reason_test.go** asserting:
       - `len(AllReasons()) == 7`
       - Each Reason value string matches its expected snake_case token (use a table test).
       - `Reason("ad_decoy") == ReasonAdDecoy`
    6. **Create libs/streamprobe/blocklist_test.go** with table tests:
       ```
       {"foo.ibyteimg.com", true}
       {"ibyteimg.com", true}
       {"IbyTeImG.com", true}
       {"p16-ad-sg.ibyteimg.com", true}
       {"p16-ad-sg-foo.example.com", true}
       {"sub.ad-site-i18n.example.org", true}
       {"tiktokcdn.com", true}
       {"example.com", false}
       {"premilkyway.com", false}
       {"dramiyos-cdn.com", false}
       {"", false}
       ```
       Plus: assert the blocklist.go file contains a `// TODO:` token + `scraper:streamprobe:blocklist` literal (read file via `os.ReadFile` from within the test — keeps the spec-anchor enforced in CI).
    7. **Sync workspace**: after writing go.work + libs/streamprobe/go.mod, run `cd /data/animeenigma && go work sync`.
  </action>
  <verify>
    <automated>cd /data/animeenigma && go work sync && cd libs/streamprobe && go test ./... -run "TestReason|TestBlocklist|TestIsAdCDNHost" -count=1</automated>
  </verify>
  <done>
    - File `libs/streamprobe/reason.go` contains `type Reason string` and 7 const values matching spec.
    - File `libs/streamprobe/blocklist.go` contains a `// TODO:` block referencing `scraper:streamprobe:blocklist` and lists all four ad-CDN host suffixes.
    - File `go.work` contains `./libs/streamprobe` inside the `use ( ... )` block.
    - `grep -c "ibyteimg.com" libs/streamprobe/blocklist.go` returns 1+.
    - `grep -c "scraper:streamprobe:blocklist" libs/streamprobe/blocklist.go` returns 1+.
    - `go test ./libs/streamprobe/...` passes with at least 12 sub-test cases.
  </done>
</task>

<task type="auto" tdd="true">
  <name>Task 2: Implement Probe — master m3u8 walk, variant fetch, segment HEAD, Reason classifier</name>
  <files>libs/streamprobe/probe.go, libs/streamprobe/probe_test.go, libs/streamprobe/testdata/playable_master.m3u8, libs/streamprobe/testdata/playable_variant.m3u8, libs/streamprobe/testdata/ad_decoy_variant.m3u8, libs/streamprobe/testdata/zero_match_no_extm3u.m3u8, libs/streamprobe/testdata/empty_variant.m3u8</files>
  <read_first>
    - libs/streamprobe/reason.go (created in Task 1 — Reason consts the classifier must return)
    - libs/streamprobe/blocklist.go (created in Task 1 — isAdCDNHost matcher)
    - libs/videoutils/proxy.go lines 1-100 (existing outbound-HTTP pattern in this codebase — User-Agent + Referer header handling; we are NOT consuming it but the patterns inform Probe's http.Client construction)
    - docs/plans/2026-05-13-scraper-self-healing-spec.md §4.1.c steps 1-7 (the algorithm)
    - .planning/phases/21-playability-foundation/21-CONTEXT.md §risks "Probe budget overshoot" (parallel-for-top-2 hint is for Plan 21-03, NOT this task — this task only ships sequential Probe)
  </read_first>
  <behavior>
    - Test: Probe(ctx, "<master url returning playable_master.m3u8 -> playable_variant.m3u8 with segments at example.com -> first segment HEAD returns 200>", nil) returns {Playable: true, Reason: ReasonPlayable, Sampled: ["example.com"]}.
    - Test: Probe with variant containing `https://p16-ad-sg.ibyteimg.com/seg-001.ts` returns {Playable: false, Reason: ReasonAdDecoy, Sampled: ["p16-ad-sg.ibyteimg.com"]} — the BLOCKLIST hit short-circuits before the HEAD.
    - Test: Probe against a 403 master response returns {Playable: false, Reason: ReasonStatus403}.
    - Test: Probe against a body that does NOT start with `#EXTM3U` returns {Playable: false, Reason: ReasonZeroMatch}.
    - Test: Probe where first-segment HEAD returns 403 returns {Playable: false, Reason: ReasonStatus403}.
    - Test: Probe where the master is reachable but variant body is empty (zero bytes after #EXTM3U) returns {Playable: false, Reason: ReasonEmptyResponse}.
    - Test: Probe where master URL dials a closed port (use httptest.NewServer().Close() to simulate) returns {Playable: false, Reason: ReasonCDNUnreachable}.
    - Test: Probe where the master URL `?e=<epoch>` expires-in-past (e.g. `?e=1000000000`) AND HEAD returns 403 returns {Playable: false, Reason: ReasonSignedURLExpired}. (Heuristic: if request URL has `?e=<unix-seconds>` and now > e, classify a 403 as signed-url-expired instead of plain 403.)
    - Test: Per-step timeout — Probe with a 5s-sleeping httptest server completes with ReasonCDNUnreachable within ≤ 5s (4s per-step budget + ~500ms slack), not 30s default.
    - Test: SSRF guard — Probe("http://127.0.0.1:80/master.m3u8", nil) returns {Playable: false, Reason: ReasonCDNUnreachable, Sampled: nil} BEFORE any dial happens, and the test asserts via httptest counter that NO request was attempted.
    - Test: Total budget — Probe respects ctx deadline; if ctx is cancelled before HEAD, returns ReasonCDNUnreachable.
  </behavior>
  <action>
    1. **Create libs/streamprobe/probe.go**:
       ```go
       // Package streamprobe — playability gate for the scraper service and
       // scheduler canary. Walks master m3u8 → first variant → first segment
       // HEAD, classifying the outcome into a typed Reason enum.
       //
       // SCRAPER-HEAL-01. Consumed by:
       //   - services/scraper/internal/providers/gogoanime (Plan 21-03)
       //   - services/scheduler/internal/jobs/scraper_playability_canary (Phase 23)
       package streamprobe

       import (
           "context"
           "errors"
           "io"
           "net"
           "net/http"
           "net/url"
           "regexp"
           "strconv"
           "strings"
           "time"
       )

       // Result is the structured output of Probe.
       type Result struct {
           Playable bool     // true only when Reason == ReasonPlayable
           Reason   Reason
           Sampled  []string // hostnames observed during the walk (for diagnostics)
       }

       const (
           perStepTimeout = 4 * time.Second
           totalBudget    = 10 * time.Second
           maxBodyBytes   = 1 << 20 // 1 MiB body cap (DoS guard for variant playlists)
           userAgent      = "AnimeEnigma-StreamProbe/1.0"
       )

       // Probe walks master m3u8 → first variant → first-segment HEAD and
       // returns a structured Result. masterURL MUST be an absolute http(s) URL.
       // headers are merged into the outbound request (Referer is the most
       // common caller-supplied header).
       //
       // Per-step timeout: 4s (master GET, variant GET, segment HEAD each).
       // Total budget: ≤ 10s via ctx with timeout.
       //
       // SSRF defense: rejects RFC1918 + loopback + link-local destinations
       // BEFORE dialling.
       func Probe(ctx context.Context, masterURL string, headers http.Header) Result {
           ctx, cancel := context.WithTimeout(ctx, totalBudget)
           defer cancel()

           // Step 1: validate URL + SSRF guard
           u, err := url.Parse(masterURL)
           if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
               return Result{Reason: ReasonZeroMatch}
           }
           if !isPublicHost(u.Hostname()) {
               return Result{Reason: ReasonCDNUnreachable, Sampled: nil}
           }

           client := newHTTPClient()

           // Step 2: GET master
           masterBody, status, err := doGet(ctx, client, masterURL, headers)
           if err != nil {
               return Result{Reason: ReasonCDNUnreachable, Sampled: []string{u.Hostname()}}
           }
           if status == http.StatusForbidden {
               return classify403(masterURL, []string{u.Hostname()})
           }
           if status != http.StatusOK {
               return Result{Reason: ReasonStatus403, Sampled: []string{u.Hostname()}}
           }
           if !bytesIsM3U8(masterBody) {
               return Result{Reason: ReasonZeroMatch, Sampled: []string{u.Hostname()}}
           }

           // Step 3: extract first variant URI; if master IS already a media
           // playlist (#EXTINF rows directly), use it as the variant.
           variantURL, isMaster := firstVariantURI(masterBody, u)
           if !isMaster {
               // Master IS the media playlist — re-use masterBody as variant.
               return checkSegments(ctx, client, masterBody, u, []string{u.Hostname()}, headers)
           }
           if variantURL == "" {
               return Result{Reason: ReasonZeroMatch, Sampled: []string{u.Hostname()}}
           }

           vu, err := url.Parse(variantURL)
           if err != nil || !isPublicHost(vu.Hostname()) {
               return Result{Reason: ReasonCDNUnreachable, Sampled: []string{u.Hostname()}}
           }

           variantBody, vstatus, verr := doGet(ctx, client, variantURL, headers)
           sampled := []string{u.Hostname(), vu.Hostname()}
           if verr != nil {
               return Result{Reason: ReasonCDNUnreachable, Sampled: sampled}
           }
           if vstatus == http.StatusForbidden {
               return classify403(variantURL, sampled)
           }
           if vstatus != http.StatusOK {
               return Result{Reason: ReasonStatus403, Sampled: sampled}
           }
           if !bytesIsM3U8(variantBody) {
               return Result{Reason: ReasonZeroMatch, Sampled: sampled}
           }
           return checkSegments(ctx, client, variantBody, vu, sampled, headers)
       }

       // checkSegments walks #EXTINF entries, classifies the FIRST segment.
       func checkSegments(ctx context.Context, client *http.Client, body []byte, base *url.URL, sampled []string, headers http.Header) Result {
           segs := extractSegmentURIs(body, base)
           if len(segs) == 0 {
               return Result{Reason: ReasonEmptyResponse, Sampled: sampled}
           }
           first := segs[0]
           fu, err := url.Parse(first)
           if err != nil {
               return Result{Reason: ReasonZeroMatch, Sampled: sampled}
           }
           segHost := fu.Hostname()
           sampled = append(sampled, segHost)
           if isAdCDNHost(segHost) {
               return Result{Reason: ReasonAdDecoy, Sampled: sampled}
           }
           if !isPublicHost(segHost) {
               return Result{Reason: ReasonCDNUnreachable, Sampled: sampled}
           }
           status, herr := doHead(ctx, client, first, headers)
           if herr != nil {
               return Result{Reason: ReasonCDNUnreachable, Sampled: sampled}
           }
           if status == http.StatusForbidden {
               return classify403(first, sampled)
           }
           if status < 200 || status >= 300 {
               return Result{Reason: ReasonStatus403, Sampled: sampled}
           }
           return Result{Playable: true, Reason: ReasonPlayable, Sampled: sampled}
       }

       // newHTTPClient builds a client with per-step timeout 4s and no
       // automatic redirect following on HEAD (we want to inspect the
       // immediate status code, not the redirected one).
       func newHTTPClient() *http.Client {
           tr := &http.Transport{
               DialContext: (&net.Dialer{Timeout: perStepTimeout}).DialContext,
               TLSHandshakeTimeout:   perStepTimeout,
               ResponseHeaderTimeout: perStepTimeout,
           }
           return &http.Client{Timeout: perStepTimeout, Transport: tr}
       }

       func doGet(ctx context.Context, client *http.Client, raw string, headers http.Header) ([]byte, int, error) {
           reqCtx, cancel := context.WithTimeout(ctx, perStepTimeout)
           defer cancel()
           req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, raw, nil)
           if err != nil {
               return nil, 0, err
           }
           req.Header.Set("User-Agent", userAgent)
           for k, vv := range headers {
               for _, v := range vv {
                   req.Header.Add(k, v)
               }
           }
           resp, err := client.Do(req)
           if err != nil {
               return nil, 0, err
           }
           defer resp.Body.Close()
           body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
           if err != nil {
               return nil, resp.StatusCode, err
           }
           return body, resp.StatusCode, nil
       }

       func doHead(ctx context.Context, client *http.Client, raw string, headers http.Header) (int, error) {
           reqCtx, cancel := context.WithTimeout(ctx, perStepTimeout)
           defer cancel()
           req, err := http.NewRequestWithContext(reqCtx, http.MethodHead, raw, nil)
           if err != nil {
               return 0, err
           }
           req.Header.Set("User-Agent", userAgent)
           for k, vv := range headers {
               for _, v := range vv {
                   req.Header.Add(k, v)
               }
           }
           resp, err := client.Do(req)
           if err != nil {
               return 0, err
           }
           defer resp.Body.Close()
           _, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
           return resp.StatusCode, nil
       }

       // bytesIsM3U8 reports whether body starts with the #EXTM3U sentinel
       // (allowing leading whitespace + UTF-8 BOM).
       func bytesIsM3U8(body []byte) bool {
           s := strings.TrimLeft(string(body), "\ufeff \t\r\n")
           return strings.HasPrefix(s, "#EXTM3U")
       }

       // firstVariantURI returns the resolved absolute URL of the first
       // #EXT-X-STREAM-INF variant entry, plus a flag indicating whether body
       // appeared to be a master playlist (variant list) vs a media playlist
       // (segment list). If no #EXT-X-STREAM-INF lines are found, returns
       // ("", false) — caller should treat body as a media playlist directly.
       func firstVariantURI(body []byte, base *url.URL) (string, bool) {
           lines := strings.Split(string(body), "\n")
           seenStreamInf := false
           for i, line := range lines {
               trim := strings.TrimSpace(line)
               if strings.HasPrefix(trim, "#EXT-X-STREAM-INF") {
                   seenStreamInf = true
                   // Next non-comment, non-empty line is the URI.
                   for j := i + 1; j < len(lines); j++ {
                       t := strings.TrimSpace(lines[j])
                       if t == "" || strings.HasPrefix(t, "#") {
                           continue
                       }
                       return resolveURI(base, t), true
                   }
               }
           }
           return "", seenStreamInf
       }

       // extractSegmentURIs returns the resolved absolute URLs of every
       // #EXTINF segment entry in body.
       func extractSegmentURIs(body []byte, base *url.URL) []string {
           lines := strings.Split(string(body), "\n")
           out := make([]string, 0, 16)
           expectURI := false
           for _, line := range lines {
               t := strings.TrimSpace(line)
               if t == "" {
                   continue
               }
               if strings.HasPrefix(t, "#EXTINF") {
                   expectURI = true
                   continue
               }
               if expectURI && !strings.HasPrefix(t, "#") {
                   out = append(out, resolveURI(base, t))
                   expectURI = false
               }
           }
           return out
       }

       func resolveURI(base *url.URL, raw string) string {
           if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
               return raw
           }
           ref, err := url.Parse(raw)
           if err != nil {
               return ""
           }
           return base.ResolveReference(ref).String()
       }

       // isPublicHost rejects loopback, RFC1918 private, link-local, and
       // unspecified hostnames before any HTTP dial. Hostnames that don't
       // resolve to an IP (true DNS names) are allowed — DNS resolution
       // happens at dial time, where the standard library's dialer will
       // re-check.
       func isPublicHost(host string) bool {
           if host == "" {
               return false
           }
           ip := net.ParseIP(host)
           if ip == nil {
               // Hostname (not raw IP) — defer to dial-time resolution.
               // Block obvious cases by literal name.
               lower := strings.ToLower(host)
               if lower == "localhost" || strings.HasSuffix(lower, ".localhost") {
                   return false
               }
               return true
           }
           return !ip.IsLoopback() && !ip.IsPrivate() && !ip.IsLinkLocalUnicast() && !ip.IsUnspecified()
       }

       // signedURLEpochRe captures a numeric `e=<unix-seconds>` or
       // `expires=<unix-seconds>` query param.
       var signedURLEpochRe = regexp.MustCompile(`(?:^|[?&])(?:e|expires)=(\d{8,12})(?:&|$)`)

       // classify403 distinguishes a generic 403 from an EXPIRED signed URL
       // (the latter is recoverable by re-fetching upstream).
       func classify403(raw string, sampled []string) Result {
           m := signedURLEpochRe.FindStringSubmatch(raw)
           if len(m) == 2 {
               epoch, err := strconv.ParseInt(m[1], 10, 64)
               if err == nil && time.Now().Unix() > epoch {
                   return Result{Reason: ReasonSignedURLExpired, Sampled: sampled}
               }
           }
           return Result{Reason: ReasonStatus403, Sampled: sampled}
       }

       var _ = errors.New // reserved for future error-wrap helpers
       ```
    2. **Create testdata fixtures** (all are static text files):
       - `testdata/playable_master.m3u8`:
         ```
         #EXTM3U
         #EXT-X-STREAM-INF:BANDWIDTH=1000000,RESOLUTION=1280x720
         /variant_720.m3u8
         ```
       - `testdata/playable_variant.m3u8`:
         ```
         #EXTM3U
         #EXT-X-TARGETDURATION:6
         #EXTINF:6.0,
         /seg/001.ts
         #EXTINF:6.0,
         /seg/002.ts
         #EXT-X-ENDLIST
         ```
       - `testdata/ad_decoy_variant.m3u8`:
         ```
         #EXTM3U
         #EXT-X-TARGETDURATION:6
         #EXTINF:6.0,
         https://p16-ad-sg.ibyteimg.com/seg/001.ts
         #EXT-X-ENDLIST
         ```
       - `testdata/zero_match_no_extm3u.m3u8`:
         ```
         <html><body>Not an m3u8</body></html>
         ```
       - `testdata/empty_variant.m3u8`:
         ```
         #EXTM3U
         #EXT-X-TARGETDURATION:6
         #EXT-X-ENDLIST
         ```
    3. **Create libs/streamprobe/probe_test.go** with `httptest.NewServer` table tests for each Reason:
       - TestProbe_Playable: master server returns playable_master.m3u8; same server returns playable_variant.m3u8 at /variant_720.m3u8; HEAD on /seg/001.ts returns 200. Assert Result.Playable && Reason == ReasonPlayable.
       - TestProbe_AdDecoy: master returns a variant directly pointing at ad-decoy host (use the synthetic master that IS-the-media-playlist with an absolute `p16-ad-sg.ibyteimg.com` segment). NO outbound HTTP to the ad-CDN host MUST occur — assert via `httptest.NewServer` request counter ON THE AD-CDN MOCK that count == 0.
       - TestProbe_Status403: master server returns 403. Assert Reason == ReasonStatus403.
       - TestProbe_ZeroMatch_NotM3U8: master returns the html fixture. Assert Reason == ReasonZeroMatch.
       - TestProbe_EmptyResponse: master returns empty_variant.m3u8 directly. Assert Reason == ReasonEmptyResponse.
       - TestProbe_CDNUnreachable: build a server, close it, then call Probe. Assert Reason == ReasonCDNUnreachable.
       - TestProbe_SignedURLExpired: master responds 403 AT URL `?e=1000000000`. Assert Reason == ReasonSignedURLExpired.
       - TestProbe_PerStepTimeout: master server sleeps 6s before responding. Assert call returns within ≤ 5s wall clock AND Reason == ReasonCDNUnreachable.
       - TestProbe_SSRF_Loopback: call Probe with `http://127.0.0.1:1/foo`. Assert Reason == ReasonCDNUnreachable AND no HTTP attempt was made (test by stubbing a transport with a counter — or simpler: assert wall-clock < 100ms, well below the dial timeout, proving the SSRF guard short-circuited).
       - TestProbe_SegmentHEAD_403: master + variant 200, but HEAD on segment returns 403. Assert Reason == ReasonStatus403.
       - TestProbe_RelativeSegmentURI: variant uses `/seg/001.ts` relative path. Assert resolveURI joined the master host correctly (Reason == ReasonPlayable when HEAD returns 200).
    4. **Run tests**: `cd libs/streamprobe && go test ./... -count=1 -race -v`.
  </action>
  <verify>
    <automated>cd /data/animeenigma/libs/streamprobe && go test ./... -count=1 -race</automated>
  </verify>
  <done>
    - `Probe(ctx, masterURL, headers) Result` exported from libs/streamprobe.
    - All 7 Reason values exercised by at least one unit test each.
    - SSRF guard rejects 127.0.0.0/8, RFC1918, link-local before dial.
    - Per-step timeout enforces ≤ 4s + ~500ms slack.
    - Total budget ≤ 10s.
    - Body read capped at 1 MiB.
    - `go test ./libs/streamprobe/... -race` passes.
    - `grep -c "ReasonPlayable\|ReasonAdDecoy\|ReasonZeroMatch\|ReasonStatus403\|ReasonSignedURLExpired\|ReasonCDNUnreachable\|ReasonEmptyResponse" libs/streamprobe/probe.go` returns 7+.
  </done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| caller→Probe | masterURL is caller-supplied; could be an SSRF vector if caller is exposed to untrusted input (today: only scraper-internal callers, but defense-in-depth required) |
| Probe→upstream m3u8 servers | external HLS CDNs; untrusted bytes parsed without size limits would be a DoS |
| Probe→ad-decoy hosts | a hostile m3u8 could try to trick us into HEAD-probing a blocked CDN; the blocklist check MUST short-circuit BEFORE the HEAD |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-21-01 | T (Tampering) | masterURL parameter | mitigate | URL must parse as absolute http/https; hostname rejected if loopback/RFC1918/link-local before dial (isPublicHost). |
| T-21-02 | D (DoS) | variant playlist body | mitigate | io.LimitReader at 1 MiB; per-step timeout 4s on every fetch; total ctx budget 10s. |
| T-21-03 | I (Information disclosure) | ad-CDN HEAD probe leaks our IP to TikTok | mitigate | isAdCDNHost check SHORT-CIRCUITS before the HEAD — test asserts via mock-server request counter that ad-CDN host receives ZERO requests when segment hostname is blocklisted. |
| T-21-04 | T | Reason enum drift between probe and metrics/maintenance prompt | mitigate | `AllReasons()` + table tests assert the maintenance prompt file mentions every reason value (test is added in Plan 21-03 once the metrics names exist; this plan ships the AllReasons() helper). |
| T-21-05 | R (Repudiation) | Probe failures invisible to ops | accept | Plan 21-02 ships the parser_unplayable_total metric; this plan only ships the library + tests. |
</threat_model>

<verification>
- `cd /data/animeenigma && go work sync` exits 0.
- `cd /data/animeenigma/libs/streamprobe && go test ./... -count=1 -race` passes with 7+ Reason-coverage tests.
- `grep -c "./libs/streamprobe" go.work` returns 1+.
- `grep -c "ReasonPlayable" libs/streamprobe/reason.go` returns 1+.
- `grep -c "ibyteimg.com" libs/streamprobe/blocklist.go` returns 1+.
- `grep -c "scraper:streamprobe:blocklist" libs/streamprobe/blocklist.go` returns 1+ (Redis-lift TODO anchored).
</verification>

<success_criteria>
- SCRAPER-HEAL-01: `libs/streamprobe/` exports `Probe(ctx, masterURL, headers) Result` with the 7 Reason classifications, per-step 4s + total 10s budgets.
- SCRAPER-HEAL-02: `libs/streamprobe/blocklist.go` holds the hardcoded ad-CDN host-suffix slice + `// TODO:` block pointing at the Redis-lift trigger.
- Unit tests cover every Reason value with synthetic m3u8 fixtures.
- SSRF guard prevents probing internal/loopback hosts.
- Package is workspace-registered (`go work sync` succeeds).
</success_criteria>

<output>
After completion, create `.planning/phases/21-playability-foundation/21-21-01-SUMMARY.md` documenting the package surface, the Reason enum values, the blocklist entries, and the TODO trigger for the Redis lift.
</output>
