---
id: 21-03
phase: 21
plan: 03
type: execute
wave: 2
depends_on:
  - 21-01
  - 21-02
files_modified:
  - services/scraper/go.mod
  - services/scraper/Dockerfile
  - services/scraper/internal/config/config.go
  - services/scraper/internal/config/config_test.go
  - services/scraper/internal/providers/gogoanime/server_priority.go
  - services/scraper/internal/providers/gogoanime/server_priority_test.go
  - services/scraper/internal/providers/gogoanime/client.go
  - services/scraper/internal/providers/gogoanime/client_test.go
  - services/scraper/internal/service/orchestrator.go
  - services/scraper/internal/service/orchestrator_test.go
  - services/scraper/internal/handler/scraper.go
  - services/scraper/internal/handler/scraper_test.go
  - services/scraper/cmd/scraper-api/main.go
autonomous: true
requirements:
  - SCRAPER-HEAL-03
  - SCRAPER-HEAL-04
  - SCRAPER-HEAL-05
user_setup: []
must_haves:
  truths:
    - "env SCRAPER_SERVER_PRIORITY sets the CSV order in gogoanime.ListServers; default streamhg,earnvids,vibeplayer"
    - "Unknown server names in SCRAPER_SERVER_PRIORITY cause scraper startup failure with the unknown names listed"
    - "gogoanime.GetStream iterates priority list, runs streamprobe.Probe, returns first playable server"
    - "Total GetStream in-call budget ≤ 8s across servers"
    - "Top-2 priority servers probed in parallel (per CONTEXT.md risks: probe budget overshoot mitigation); positions 3+ sequential"
    - "Winning (anime,episode)→serverID cached at scraper:winning_server:<provider>:<anime>:<ep> for 5 min"
    - "Cache hit path skips the gate (meta.gated=false)"
    - "Cache miss path runs the gate and emits meta.gated=true via the handler"
    - "parser_unplayable_total{provider,server,reason} increments on every gate failure"
    - "parser_ad_decoy_total{provider,server} increments on every reason=ad_decoy classification (in addition to parser_unplayable_total)"
    - "Production smoke: Frieren ep1 sub plays end-to-end via StreamHG or Earnvids (no ibyteimg.com / p16-ad-sg.* segments in returned m3u8)"
  artifacts:
    - path: "services/scraper/internal/providers/gogoanime/server_priority.go"
      provides: "SortByPriority + ValidatePriorityList against known-extractor names"
      contains: "SortByPriority"
    - path: "services/scraper/internal/providers/gogoanime/client.go"
      provides: "GetStream iterates priority list + probes + caches winner"
      contains: "streamprobe.Probe"
    - path: "services/scraper/internal/config/config.go"
      provides: "GogoanimeConfig.ServerPriority []string"
      contains: "SCRAPER_SERVER_PRIORITY"
  key_links:
    - from: "services/scraper/internal/providers/gogoanime/client.go"
      to: "libs/streamprobe"
      via: "import + Probe() call inside GetStream"
      pattern: "streamprobe.Probe"
    - from: "services/scraper/internal/providers/gogoanime/client.go"
      to: "libs/metrics.ParserUnplayableTotal"
      via: "Inc on each failed gate"
      pattern: "ParserUnplayableTotal.WithLabelValues"
    - from: "services/scraper/internal/providers/gogoanime/client.go"
      to: "scraper:winning_server:gogoanime:<anime>:<ep> redis key"
      via: "p.cache.Get/Set"
      pattern: "winning_server"
    - from: "services/scraper/cmd/scraper-api/main.go"
      to: "ValidatePriorityList"
      via: "fatal at startup on unknown server names"
      pattern: "ValidatePriorityList"
    - from: "services/scraper/internal/handler/scraper.go"
      to: "orchestrator.GetStream"
      via: "new (*Stream, gated bool, err) return triple consumed by writeSuccess(..., gated)"
      pattern: "gated"
---

<objective>
Wire the playability gate into `gogoanime.GetStream` and ship the server-priority + winning-server cache. The cold path probes the top-2 priority servers in parallel (probe budget mitigation per CONTEXT.md risks), iterates the rest sequentially, caches the winner in Redis, and surfaces `meta.gated:true` through the handler. SCRAPER-HEAL-03 + SCRAPER-HEAL-04 + SCRAPER-HEAL-05.

Purpose: This is the plan that fixes production. Once shipped, Frieren ep 1 sub plays end-to-end through StreamHG or Earnvids transparently — VibePlayer (ad-decoy) gets gate-failed and skipped without the user noticing anything beyond ~1-2s extra cold-path latency masked by Plan 21-04's Phase-3 loader.

Output: Cold-path GetStream iterates server priority via the gate; winning server cached 5 min; metrics increment on every fail; handler emits `meta.gated:true` whenever the gate ran on this call.
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
@.planning/phases/21-playability-foundation/21-21-01-SUMMARY.md
@.planning/phases/21-playability-foundation/21-21-02-SUMMARY.md
@CLAUDE.md

<interfaces>
<!-- Plan 21-01 outputs (libs/streamprobe): -->

```go
package streamprobe

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
type Result struct {
    Playable bool
    Reason   Reason
    Sampled  []string
}
func Probe(ctx context.Context, masterURL string, headers http.Header) Result
```

<!-- Plan 21-02 outputs (libs/metrics): -->

```go
var ParserUnplayableTotal = promauto.NewCounterVec(
    prometheus.CounterOpts{Name: "parser_unplayable_total", ...},
    []string{"provider", "server", "reason"},
)
var ParserAdDecoyTotal = promauto.NewCounterVec(
    prometheus.CounterOpts{Name: "parser_ad_decoy_total", ...},
    []string{"provider", "server"},
)
```

<!-- Plan 21-02 outputs (handler): -->

```go
func (h *ScraperHandler) writeSuccess(w http.ResponseWriter, data map[string]any, tried []string, gated bool)
// GetStream currently passes gated=false (placeholder). THIS plan changes
// it to pass a real bool sourced from orchestrator.GetStream's new return
// triple.
```

<!-- Existing gogoanime.Provider GetStream signature (services/scraper/internal/providers/gogoanime/client.go:652) — to be SUPPLEMENTED by a new method that accepts the priority list + returns a gated bool. The original GetStream stays for backward-compat with non-priority code paths (none in production but keep tests green): -->

```go
func (p *Provider) GetStream(ctx context.Context, providerID, episodeID, serverID string, category domain.Category) (*domain.Stream, error)
```

<!-- This plan adds: -->

```go
// GetStreamWithGate is the new entry point. Tries serverID-pinned path FIRST
// (caller-specified server override). On falsy serverID or cache miss, iterates
// the configured priority list, runs streamprobe.Probe on each candidate, and
// returns (stream, gated=true, nil) on first success.
//
// The orchestrator's GetStream method will be widened to expose this gated
// bool through the response envelope.
func (p *Provider) GetStreamWithGate(ctx context.Context, providerID, episodeID, serverID string, category domain.Category, servers []domain.Server) (*domain.Stream, bool, error)
```

<!-- Memory note "Adding New libs/ Module": for this plan steps 2 (services/scraper/go.mod
require + replace) and 3 (Dockerfile COPY) must be done. -->

<!-- Existing client.go embed extractor names (used as `server` label values): -->

- "vibeplayer" (registered via embeds.NewVibePlayerExtractor)
- "streamhg" (registered via embeds.NewStreamHGExtractor)
- "earnvids" (registered via embeds.NewEarnvidsExtractor)

<!-- ListServers populates domain.Server.Name from the visible HTML label
("HD-1", "HD-2", "StreamHG", "Earnvids"). The PRIORITY KEY is NOT the label —
it is the embed extractor's name(), derived from the server URL's host via
the registry's Find() call. Map host → extractor name → priority. -->
</interfaces>
</context>

<tasks>

<task type="auto" tdd="true">
  <name>Task 1: Add SCRAPER_SERVER_PRIORITY config + ValidatePriorityList + SortByPriority helpers</name>
  <files>services/scraper/internal/config/config.go, services/scraper/internal/config/config_test.go, services/scraper/internal/providers/gogoanime/server_priority.go, services/scraper/internal/providers/gogoanime/server_priority_test.go</files>
  <read_first>
    - services/scraper/internal/config/config.go (full file — extend GogoanimeConfig with ServerPriority + parse SCRAPER_SERVER_PRIORITY env)
    - services/scraper/internal/providers/gogoanime/client.go lines 60-115 (existing constants + selector ID pattern — match the file-header doc style)
    - services/scraper/internal/embeds/streamhg.go (extractor.Name() returns "streamhg" — confirm by reading the file)
    - services/scraper/internal/embeds/vibeplayer.go (extractor.Name() returns "vibeplayer")
    - services/scraper/internal/embeds/earnvids.go (extractor.Name() returns "earnvids")
    - .planning/phases/21-playability-foundation/21-CONTEXT.md §risks "Server-priority env typo silently demotes a real server" (fail-fast requirement)
    - docs/plans/2026-05-13-scraper-self-healing-spec.md §4.3 modified files row "services/scraper/internal/config/config.go"
  </read_first>
  <behavior>
    - Test: `cfg, _ := config.Load()` with SCRAPER_SERVER_PRIORITY unset → `cfg.Gogoanime.ServerPriority == ["streamhg", "earnvids", "vibeplayer"]`.
    - Test: `SCRAPER_SERVER_PRIORITY=earnvids,streamhg,vibeplayer` → parsed slice matches in that order; whitespace trimmed; case-lowered.
    - Test: `SCRAPER_SERVER_PRIORITY=`<empty>` → defaults to `["streamhg", "earnvids", "vibeplayer"]`.
    - Test: `SCRAPER_SERVER_PRIORITY=streamhg, , vibeplayer` → empty entries dropped, result `["streamhg", "vibeplayer"]`.
    - Test: `ValidatePriorityList([]string{"streamhg","earnvids","vibeplayer"}, knownNames=["streamhg","earnvids","vibeplayer","kwik","megacloud"])` returns nil.
    - Test: `ValidatePriorityList([]string{"streamg","earnvids","vibeplayer"}, knownNames)` returns an error whose `.Error()` contains the literal string `"streamg"` AND the literal string `"unknown server"` (so a typo'd entry's name is visible in the boot log).
    - Test: `SortByPriority([]domain.Server, priority, registry)` reorders the slice such that any server whose extractor.Name() is in `priority` appears at the position dictated by priority; unknowns trail in original source-HTML order; ties broken by original index. Determinism: same input + same priority always produces the same output (table test).
  </behavior>
  <action>
    1. **Edit services/scraper/internal/config/config.go** — extend GogoanimeConfig:
       ```go
       type GogoanimeConfig struct {
           BaseURL        string
           ServerPriority []string // CSV per env SCRAPER_SERVER_PRIORITY; default [streamhg, earnvids, vibeplayer]
       }
       ```
       In `Load()`, after the existing Gogoanime.BaseURL parse:
       ```go
       priorityCSV := getEnv("SCRAPER_SERVER_PRIORITY", "streamhg,earnvids,vibeplayer")
       parts := strings.Split(priorityCSV, ",")
       priority := make([]string, 0, len(parts))
       for _, p := range parts {
           p = strings.ToLower(strings.TrimSpace(p))
           if p == "" {
               continue
           }
           priority = append(priority, p)
       }
       cfg.Gogoanime.ServerPriority = priority
       ```
       Note: this task does NOT validate the priority entries against known names — that happens in main.go boot wiring (Task 5) where the embeds.Registry is available. config.Load stays registry-agnostic.
    2. **Update services/scraper/internal/config/config_test.go** — add tests for the default + override + whitespace cases above. If TestLoad_Defaults exists, extend it; otherwise add `TestLoad_ServerPriorityDefault`, `TestLoad_ServerPriorityOverride`, `TestLoad_ServerPriorityWhitespace`. Use `t.Setenv` for env vars (Go 1.17+ idiom).
    3. **Create services/scraper/internal/providers/gogoanime/server_priority.go**:
       ```go
       // server_priority.go — sort gogoanime ListServers output by the
       // SCRAPER_SERVER_PRIORITY config + validate the list against the
       // embeds.Registry's known extractor names.
       //
       // SCRAPER-HEAL-03.
       package gogoanime

       import (
           "fmt"
           "net/url"
           "strings"

           "github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
       )

       // ValidatePriorityList rejects any entry in priority that is NOT in
       // knownNames. Returns nil on success; returns an error LISTING the
       // unknown names so the boot log surfaces the typo.
       //
       // SCRAPER-HEAL-03 risk mitigation: a SCRAPER_SERVER_PRIORITY env typo
       // (e.g. streamg instead of streamhg) must fail-fast at startup, NOT
       // silently sort the typo'd name into the trailing "unknown" bucket.
       func ValidatePriorityList(priority, knownNames []string) error {
           known := make(map[string]struct{}, len(knownNames))
           for _, n := range knownNames {
               known[strings.ToLower(n)] = struct{}{}
           }
           var unknown []string
           for _, p := range priority {
               if _, ok := known[strings.ToLower(p)]; !ok {
                   unknown = append(unknown, p)
               }
           }
           if len(unknown) > 0 {
               return fmt.Errorf("gogoanime: SCRAPER_SERVER_PRIORITY contains unknown server name(s): %s (known: %s)",
                   strings.Join(unknown, ", "),
                   strings.Join(knownNames, ", "))
           }
           return nil
       }

       // hostnameToExtractorName maps a server URL's hostname to the
       // canonical embed extractor name used as the priority key.
       //
       // The registry's Find(embedURL) returns the matching extractor; its
       // Name() is the canonical key. But Find allocates + walks the slice;
       // for sorting we want a cheap lookup, so we expose a thin helper that
       // takes a pre-computed map[hostname-suffix]extractorName.
       //
       // NOTE: callers build the map ONCE from the registry at boot. See
       // main.go (Task 5) for the bootstrap.
       func hostnameToExtractorName(rawURL string, hostExtractor map[string]string) string {
           u, err := url.Parse(rawURL)
           if err != nil {
               return ""
           }
           host := strings.ToLower(u.Hostname())
           if name, ok := hostExtractor[host]; ok {
               return name
           }
           // suffix match — for *.example.com style registry hosts
           for suf, name := range hostExtractor {
               if strings.HasSuffix(host, "."+suf) {
                   return name
               }
           }
           return ""
       }

       // SortByPriority reorders servers so entries whose extractor name
       // appears earliest in priority come first; entries whose extractor
       // name is NOT in priority trail in original order (stable sort over
       // an original-index tiebreaker).
       //
       // hostExtractor is the pre-built host→extractor-name map (see
       // hostnameToExtractorName).
       func SortByPriority(servers []domain.Server, priority []string, hostExtractor map[string]string) []domain.Server {
           priIdx := make(map[string]int, len(priority))
           for i, p := range priority {
               priIdx[strings.ToLower(p)] = i
           }

           type entry struct {
               s      domain.Server
               pri    int  // priority index; len(priority) for unknown
               origIx int
           }
           entries := make([]entry, len(servers))
           for i, s := range servers {
               name := hostnameToExtractorName(s.ID, hostExtractor)
               p, ok := priIdx[strings.ToLower(name)]
               if !ok {
                   p = len(priority)
               }
               entries[i] = entry{s: s, pri: p, origIx: i}
           }

           // Stable sort by (pri, origIx). Avoid sort.Slice's panic-on-mutate
           // and use sort.SliceStable for determinism.
           // (Go std-lib's sort.SliceStable is sufficient — same as ListEpisodes
           // sort pattern in client.go:426.)
           sortSliceStableByPriPlusOrig(entries)

           out := make([]domain.Server, len(entries))
           for i, e := range entries {
               out[i] = e.s
           }
           return out
       }

       func sortSliceStableByPriPlusOrig(entries []entry) {
           // inline sort.SliceStable to keep type local
           // (avoids reflect overhead; len(entries) is always <8 in practice)
           for i := 1; i < len(entries); i++ {
               for j := i; j > 0 && (entries[j-1].pri > entries[j].pri); j-- {
                   entries[j-1], entries[j] = entries[j], entries[j-1]
               }
           }
       }
       ```
       (If `entry` collides with another local type in the package, name it `priorityEntry`. Adjust if needed.)
    4. **Create services/scraper/internal/providers/gogoanime/server_priority_test.go** with the test cases listed in <behavior> — table tests for SortByPriority (5+ cases) and ValidatePriorityList (3+ cases including the typo case).
  </action>
  <verify>
    <automated>cd /data/animeenigma/services/scraper && go test ./internal/config/... ./internal/providers/gogoanime/... -count=1 -run "TestLoad_ServerPriority|TestValidatePriorityList|TestSortByPriority"</automated>
  </verify>
  <done>
    - `services/scraper/internal/config/config.go` has `GogoanimeConfig.ServerPriority []string` field populated from `SCRAPER_SERVER_PRIORITY` env (CSV parse, lowercase, trim, drop-empty).
    - Default `["streamhg", "earnvids", "vibeplayer"]` when env unset.
    - `ValidatePriorityList` rejects unknowns with an error containing all unknown names.
    - `SortByPriority` is stable and deterministic.
    - `grep -c "SCRAPER_SERVER_PRIORITY" services/scraper/internal/config/config.go` returns 1+.
    - All new tests pass.
  </done>
</task>

<task type="auto" tdd="true">
  <name>Task 2: Wire libs/streamprobe into services/scraper — go.mod, Dockerfile COPY, replace directive</name>
  <files>services/scraper/go.mod, services/scraper/Dockerfile</files>
  <read_first>
    - services/scraper/go.mod (full file — find the require block + the replace block, append streamprobe to both)
    - services/scraper/Dockerfile (full file — find the COPY lines for libs/cache/go.mod etc; insert COPY for libs/streamprobe/go.mod in the same position alphabetically AND in the deps-list)
    - libs/streamprobe/go.mod (created in Plan 21-01)
    - Memory note "Adding New libs/ Module" — confirms the 4 steps required
  </read_first>
  <behavior>
    - After this task: `cd services/scraper && go mod tidy && go build ./...` succeeds with libs/streamprobe in the import path.
    - Dockerfile contains `COPY libs/streamprobe/go.mod libs/streamprobe/go.sum* ./libs/streamprobe/` in the same COPY block as the other lib go.mods.
    - services/scraper/go.mod's require block lists `github.com/ILITA-hub/animeenigma/libs/streamprobe v0.0.0`.
    - services/scraper/go.mod's replace block lists `github.com/ILITA-hub/animeenigma/libs/streamprobe => ../../libs/streamprobe` (matching the pattern of existing lib replaces).
  </behavior>
  <action>
    1. **Edit services/scraper/go.mod**:
       - Append to `require ( ... )` block (alphabetical position — between `libs/metrics` and the next entry):
         ```
         github.com/ILITA-hub/animeenigma/libs/streamprobe v0.0.0-00010101000000-000000000000
         ```
       - Append to `replace ( ... )` block (or add a standalone replace if the file uses standalone replaces):
         ```
         github.com/ILITA-hub/animeenigma/libs/streamprobe => ../../libs/streamprobe
         ```
         Match the pattern of `github.com/ILITA-hub/animeenigma/libs/cache => ../../libs/cache` already present.
    2. **Edit services/scraper/Dockerfile** — insert the COPY line in the lib go.mod block, alphabetical position (after libs/pagination, before libs/tracing, before libs/videoutils — verify by reading the existing block):
       ```
       COPY libs/streamprobe/go.mod libs/streamprobe/go.sum* ./libs/streamprobe/
       ```
       Then later in the Dockerfile, where the source is COPYed (look for `COPY libs/ ./libs/` or similar pattern; if it's an explicit copy of each lib's source, add `COPY libs/streamprobe ./libs/streamprobe`).
    3. **Run** `cd /data/animeenigma && go work sync` then `cd services/scraper && go mod tidy` to refresh go.sum. Verify `go build ./...` succeeds.
  </action>
  <verify>
    <automated>cd /data/animeenigma && go work sync && cd services/scraper && go mod tidy && go build ./...</automated>
  </verify>
  <done>
    - `grep -c "libs/streamprobe" services/scraper/go.mod` returns 2+ (require + replace).
    - `grep -c "libs/streamprobe" services/scraper/Dockerfile` returns 1+ (COPY).
    - `cd services/scraper && go build ./...` succeeds.
  </done>
</task>

<task type="auto" tdd="true">
  <name>Task 3: Add GetStreamWithGate to gogoanime.Provider — parallel top-2 probe + winning-server cache + metrics</name>
  <files>services/scraper/internal/providers/gogoanime/client.go, services/scraper/internal/providers/gogoanime/client_test.go</files>
  <read_first>
    - services/scraper/internal/providers/gogoanime/client.go FULL FILE (the GetStream method at line 652; the Deps + Provider struct; the existing cache key pattern; the embeds.Registry usage)
    - services/scraper/internal/providers/gogoanime/cache.go (existing cache helper conventions — keys, TTLs, the computeStreamTTL function)
    - services/scraper/internal/providers/gogoanime/server_priority.go (from Task 1 — SortByPriority + hostnameToExtractorName)
    - libs/streamprobe/probe.go (Probe signature + Result type)
    - libs/metrics/provider.go (ParserUnplayableTotal, ParserAdDecoyTotal — added by Plan 21-02)
    - services/scraper/internal/domain/embed.go (Registry.Find signature)
    - services/scraper/internal/providers/gogoanime/cache_test.go (test pattern for cache assertions)
  </read_first>
  <behavior>
    - Test: GetStreamWithGate with empty serverID + 3 servers (streamhg, earnvids, vibeplayer) — fake streamprobe returns Playable for streamhg first; result.Sources == streamhg's, gated=true, no error. parser_unplayable_total NOT incremented for streamhg; cache key `scraper:winning_server:gogoanime:<anime>:<ep>` SET to streamhg's serverID with TTL 5m.
    - Test: GetStreamWithGate where streamhg gate fails (ReasonAdDecoy), earnvids passes — result.Sources == earnvids's, gated=true; parser_unplayable_total{provider="gogoanime",server="streamhg",reason="ad_decoy"} incremented by 1; parser_ad_decoy_total{provider="gogoanime",server="streamhg"} incremented by 1; cache key set to earnvids's serverID.
    - Test: parallel top-2 probe — both streamhg AND earnvids probed concurrently (the fake streamprobe records call timestamps; assert delta < perStepTimeout/2 ≈ 2s, proving parallelism, NOT sequential 4s+4s). The first to return Playable wins; the loser's extract is discarded (no Source leak).
    - Test: GetStreamWithGate with all 3 servers failing — returns ErrProviderDown wrapped with the last reason; parser_unplayable_total incremented 3 times (once per server); gated=true (the gate ran even though no winner).
    - Test: Warm path — pre-set cache `scraper:winning_server:gogoanime:<anime>:<ep>` to a serverID; GetStreamWithGate skips the priority iteration, calls the embed extractor directly on the cached server, returns gated=false. parser_unplayable_total NOT incremented; no Probe call made (verify via fake streamprobe counter == 0).
    - Test: Cache hit but the cached serverID is no longer in the supplied servers slice (e.g. ListServers shape changed) — fall back to cold-path priority iteration; the stale cache entry is DELETED to prevent re-stick. gated=true on this call.
    - Test: GetStreamWithGate with caller-pinned serverID (non-empty) — bypasses priority list, calls embed extractor on the pinned server, returns gated=false. parser_unplayable_total NOT incremented; matches Phase 16's per-server pin contract.
    - Test: Total in-call budget — when ALL 3 server probes take 7s each (fake streamprobe), GetStreamWithGate returns within 8.5s wall clock (≤ 8s soft + 500ms slack); error is ErrProviderDown wrapping context.DeadlineExceeded.
    - Test: cache key shape — exact format `scraper:winning_server:gogoanime:<providerID>:<episodeID>` (no hash), TTL exactly 5*time.Minute.
  </behavior>
  <action>
    1. **Edit services/scraper/internal/providers/gogoanime/client.go**:
       a. Add new imports:
          ```go
          "github.com/ILITA-hub/animeenigma/libs/metrics"  // already present
          "github.com/ILITA-hub/animeenigma/libs/streamprobe"
          ```
       b. Extend the `Deps` struct with `ServerPriority []string` and `HostExtractor map[string]string`:
          ```go
          type Deps struct {
              BaseURL        string
              HTTP           *domain.BaseHTTPClient
              Embeds         *domain.Registry
              MalSync        malSyncClient
              Cache          cache.Cache
              Log            *logger.Logger
              ServerPriority []string         // from SCRAPER_SERVER_PRIORITY; nil → no priority sort (Phase 16 behavior)
              HostExtractor  map[string]string // host → extractor.Name(); built at boot from embeds registry
              Probe          streamprobe.ProbeFunc // injectable for tests; nil → libs/streamprobe.Probe
          }
          ```
          Add to Provider struct similarly. Add a `// ProbeFunc` type alias inside libs/streamprobe in this task ONLY IF it does not exist; otherwise inline `func(context.Context, string, http.Header) streamprobe.Result`. The cleaner path: declare the func type inline in client.go to keep libs/streamprobe pure:
          ```go
          // probeFunc is the streamprobe.Probe signature, exposed for test injection.
          type probeFunc func(ctx context.Context, masterURL string, headers http.Header) streamprobe.Result
          ```
       c. In `New(d Deps)`:
          - Default `d.Probe` to `streamprobe.Probe` if nil.
          - Store `d.ServerPriority`, `d.HostExtractor`, `d.Probe` on the Provider.
       d. Add the new method:
          ```go
          // GetStreamWithGate is the priority-aware + gated entry for the
          // EnglishPlayer cold path. servers is the result of ListServers
          // already sorted by SortByPriority (caller's responsibility — keeps
          // this method registry-agnostic).
          //
          // serverID semantics:
          //   - non-empty: caller pinned a specific server; bypass priority +
          //     gate, call the embed extractor directly, return gated=false.
          //   - empty: cold path; check cache `scraper:winning_server:...`;
          //     on hit, call extractor on the cached server, return
          //     gated=false; on miss, iterate `servers` (already in priority
          //     order), probe each, return first playable + cache the winner
          //     for 5 min, return gated=true.
          //
          // SCRAPER-HEAL-04 + SCRAPER-HEAL-05.
          func (p *Provider) GetStreamWithGate(
              ctx context.Context, providerID, episodeID, serverID string,
              category domain.Category, servers []domain.Server,
          ) (*domain.Stream, bool, error) {
              // Caller-pinned path: no priority, no gate.
              if serverID != "" {
                  s, err := p.GetStream(ctx, providerID, episodeID, serverID, category)
                  return s, false, err
              }
              if len(servers) == 0 {
                  return nil, false, domain.WrapNotFound(nil, "gogoanime: no servers for gated stream")
              }

              winnerKey := fmt.Sprintf("scraper:winning_server:%s:%s:%s", providerName, providerID, episodeID)

              // Warm path: cached winner.
              var cachedServerID string
              if err := p.cache.Get(ctx, winnerKey, &cachedServerID); err == nil && cachedServerID != "" {
                  // Validate the cached serverID is still in the supplied list.
                  if hasServer(servers, cachedServerID) {
                      s, err := p.GetStream(ctx, providerID, episodeID, cachedServerID, category)
                      if err == nil {
                          return s, false, nil
                      }
                      // Cached winner errored on extract — fall through to cold path
                      // AND delete the stale cache entry.
                  }
                  _ = p.cache.Delete(ctx, winnerKey)
              }

              // Cold path: priority iteration.
              return p.coldPathGated(ctx, providerID, episodeID, category, servers, winnerKey)
          }

          // coldPathGated runs the priority + gate iteration. Probes top-2
          // candidates in parallel (CONTEXT.md risks: probe budget overshoot
          // mitigation), iterates positions 3+ sequentially.
          //
          // Total in-call budget: 8s via ctx with timeout.
          func (p *Provider) coldPathGated(
              ctx context.Context, providerID, episodeID string,
              category domain.Category, servers []domain.Server,
              winnerKey string,
          ) (*domain.Stream, bool, error) {
              callCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
              defer cancel()

              probe := p.probe
              if probe == nil {
                  probe = streamprobe.Probe
              }

              type result struct {
                  serverID string
                  stream   *domain.Stream
                  reason   streamprobe.Reason
                  err      error
              }

              // attemptOne extracts then probes one server. Returns a result
              // with stream+reason=Playable on success, or err+reason set on
              // failure.
              attemptOne := func(ctx context.Context, srv domain.Server) result {
                  // Step 1: extract URL (cheap — uses embed registry).
                  s, err := p.GetStream(ctx, providerID, episodeID, srv.ID, category)
                  if err != nil {
                      return result{serverID: srv.ID, err: err, reason: streamprobe.ReasonZeroMatch}
                  }
                  if s == nil || len(s.Sources) == 0 {
                      return result{serverID: srv.ID, err: errors.New("empty sources"), reason: streamprobe.ReasonEmptyResponse}
                  }
                  // Step 2: gate the master URL.
                  hdrs := http.Header{}
                  if ref := s.Headers["Referer"]; ref != "" {
                      hdrs.Set("Referer", ref)
                  }
                  res := probe(ctx, s.Sources[0].URL, hdrs)
                  if !res.Playable {
                      // Metric labeling: server label = extractor name (NOT srv.Name HTML label).
                      extName := hostnameToExtractorName(srv.ID, p.hostExtractor)
                      if extName == "" {
                          extName = "unknown"
                       }
                      metrics.ParserUnplayableTotal.WithLabelValues(providerName, extName, string(res.Reason)).Inc()
                      if res.Reason == streamprobe.ReasonAdDecoy {
                          metrics.ParserAdDecoyTotal.WithLabelValues(providerName, extName).Inc()
                      }
                      return result{serverID: srv.ID, err: fmt.Errorf("gate failed: %s", res.Reason), reason: res.Reason}
                  }
                  return result{serverID: srv.ID, stream: s, reason: streamprobe.ReasonPlayable}
              }

              // Parallel top-2.
              topN := 2
              if len(servers) < topN {
                  topN = len(servers)
              }
              parallelResults := make(chan result, topN)
              parCtx, parCancel := context.WithCancel(callCtx)
              for i := 0; i < topN; i++ {
                  go func(srv domain.Server) {
                      parallelResults <- attemptOne(parCtx, srv)
                  }(servers[i])
              }
              var lastReason streamprobe.Reason
              for i := 0; i < topN; i++ {
                  select {
                  case <-callCtx.Done():
                      parCancel()
                      return nil, true, domain.WrapProviderDown(callCtx.Err(), "gogoanime: gated stream budget exceeded")
                  case r := <-parallelResults:
                      if r.stream != nil {
                          parCancel() // cancel the other in-flight probe
                          _ = p.cache.Set(ctx, winnerKey, r.serverID, 5*time.Minute)
                          return r.stream, true, nil
                      }
                      lastReason = r.reason
                  }
              }
              parCancel()

              // Sequential positions 3+.
              for i := topN; i < len(servers); i++ {
                  if callCtx.Err() != nil {
                      return nil, true, domain.WrapProviderDown(callCtx.Err(), "gogoanime: gated stream budget exceeded")
                  }
                  r := attemptOne(callCtx, servers[i])
                  if r.stream != nil {
                      _ = p.cache.Set(ctx, winnerKey, r.serverID, 5*time.Minute)
                      return r.stream, true, nil
                  }
                  lastReason = r.reason
              }

              return nil, true, domain.WrapProviderDown(
                  fmt.Errorf("all %d servers gate-failed; last reason=%s", len(servers), lastReason),
                  "gogoanime: no playable server",
              )
          }

          // hasServer reports whether servers contains an entry with ID == id.
          func hasServer(servers []domain.Server, id string) bool {
              for _, s := range servers {
                  if s.ID == id {
                      return true
                  }
              }
              return false
          }
          ```
       e. Store `probe` + `hostExtractor` on the Provider struct.
    2. **Create services/scraper/internal/providers/gogoanime/client_test.go additions** (or new file `client_gated_test.go` if naming collisions exist):
       - Use a fake probe func that maps serverURL → Result based on a per-test table.
       - Use the existing fake cache + fake embeds.Registry from the package's test helpers (read helpers_test.go for the pattern).
       - Implement the 9 test cases listed under <behavior>.
       - For the parallelism test: fake probe sleeps 1s then returns Playable; assert wall-clock ≤ 1.5s when probing 2 servers concurrently (NOT 2s sequential).
       - For the metrics test: call `metrics.ParserUnplayableTotal.Reset()` + `metrics.ParserAdDecoyTotal.Reset()` at test start, then assert deltas via `testutil.ToFloat64`.
  </action>
  <verify>
    <automated>cd /data/animeenigma/services/scraper && go test ./internal/providers/gogoanime/... -count=1 -race -run "TestGetStreamWithGate"</automated>
  </verify>
  <done>
    - `gogoanime.Provider` has `GetStreamWithGate(ctx, providerID, episodeID, serverID, category, servers) (*Stream, bool, error)`.
    - Cold path probes top-2 servers in parallel, positions 3+ sequential.
    - Winning serverID cached at `scraper:winning_server:gogoanime:<anime>:<ep>` for 5 min.
    - parser_unplayable_total + parser_ad_decoy_total increment correctly.
    - Total in-call budget ≤ 8s enforced via ctx.
    - `grep -c "scraper:winning_server" services/scraper/internal/providers/gogoanime/client.go` returns 1+.
    - `grep -c "streamprobe.Probe\\|streamprobe.Reason" services/scraper/internal/providers/gogoanime/client.go` returns 2+.
    - `grep -c "ParserUnplayableTotal" services/scraper/internal/providers/gogoanime/client.go` returns 1+.
    - `grep -c "ParserAdDecoyTotal" services/scraper/internal/providers/gogoanime/client.go` returns 1+.
    - All 9 new tests pass.
  </done>
</task>

<task type="auto" tdd="true">
  <name>Task 4: Orchestrator + handler — propagate the gated bool end-to-end</name>
  <files>services/scraper/internal/service/orchestrator.go, services/scraper/internal/service/orchestrator_test.go, services/scraper/internal/handler/scraper.go, services/scraper/internal/handler/scraper_test.go</files>
  <read_first>
    - services/scraper/internal/service/orchestrator.go (entire file — find GetStream + ListServers; add a new method that calls GetStreamWithGate when the provider supports it)
    - services/scraper/internal/handler/scraper.go (already touched by Plan 21-02 — find GetStream which currently calls `h.svc.GetStream(...)`; change to a new method or extend the signature)
    - services/scraper/internal/providers/gogoanime/client.go (Task 3 of THIS plan — GetStreamWithGate signature)
    - Plan 21-02 SUMMARY (.planning/phases/21-playability-foundation/21-21-02-SUMMARY.md) — confirms writeSuccess accepts gated bool
  </read_first>
  <behavior>
    - Test: Orchestrator.GetStreamGated invokes GetStreamWithGate on a provider that implements an optional interface; falls back to GetStream + gated=false on providers that don't (animepahe, animekai).
    - Test: Handler GET /scraper/stream emits `data.meta.gated:true` when the orchestrator returns gated=true; field absent when gated=false.
    - Test (regression): Phase 16 envelope shape preserved on all 4 endpoints (episodes, servers, stream, health).
    - Test: With server=<vibeplayer-host> caller pin, handler still calls the gated path BUT GetStreamWithGate bypasses the gate (caller-pinned semantics from Task 3) and returns gated=false.
  </behavior>
  <action>
    1. **Edit services/scraper/internal/service/orchestrator.go**:
       - Define an internal interface that the gogoanime provider satisfies:
         ```go
         // gatedProvider is the optional interface a provider implements
         // when it can run a playability gate. Phase 21: only
         // gogoanime.Provider implements this. animepahe + animekai do not
         // (they're treated as gated=false fallback).
         type gatedProvider interface {
             GetStreamWithGate(ctx context.Context, providerID, episodeID, serverID string, category domain.Category, servers []domain.Server) (*domain.Stream, bool, error)
             ListServers(ctx context.Context, providerID, episodeID string) ([]domain.Server, error)
         }
         ```
       - Add new orchestrator method:
         ```go
         // GetStreamGated runs the failover chain and returns a Stream plus
         // a gated bool indicating whether the playability gate ran. Providers
         // that implement gatedProvider use their GetStreamWithGate; others
         // fall back to plain GetStream with gated=false.
         //
         // SCRAPER-HEAL-04.
         func (o *Orchestrator) GetStreamGated(
             ctx context.Context, providerID, episodeID, serverID string,
             cat domain.Category, prefer string,
         ) (*domain.Stream, bool, error) {
             var lastErr error
             for _, p := range o.orderedProviders(prefer) {
                 if o.cache != nil && !o.cache.IsUp(p.Name(), health.StageStream) {
                     metrics.ParserFallbackTotal.WithLabelValues(p.Name(), "skipped_unhealthy").Inc()
                     continue
                 }
                 if gp, ok := p.(gatedProvider); ok {
                     servers, err := gp.ListServers(ctx, providerID, episodeID)
                     if err != nil {
                         lastErr = err
                         continue
                     }
                     // NOTE: the caller (handler) has not yet sorted servers
                     // by priority. The handler MUST sort BEFORE calling this
                     // method (see handler GetStream update below) — OR the
                     // provider MUST sort internally. Choose handler-side
                     // sorting so the priority list lives in main.go config.
                     stream, gated, err := gp.GetStreamWithGate(ctx, providerID, episodeID, serverID, cat, servers)
                     if err != nil {
                         lastErr = err
                         continue
                     }
                     return stream, gated, nil
                 }
                 // Non-gated provider fallback.
                 stream, err := p.GetStream(ctx, providerID, episodeID, serverID, cat)
                 if err != nil {
                     lastErr = err
                     continue
                 }
                 return stream, false, nil
             }
             if lastErr == nil {
                 lastErr = domain.WrapProviderDown(errors.New("all providers exhausted"), "orchestrator: no playable stream")
             }
             return nil, false, lastErr
         }
         ```
       (Use the orchestrator's existing iteration + cache-skip + metric pattern — match it exactly; the snippet above shows the shape but the existing `GetStream` method's loop in orchestrator.go is the canonical reference.)
       - Decision: server sorting lives in the HANDLER (it has access to the config + the embeds registry's hostExtractor map). Update `gatedProvider` interface and the comment accordingly. Actually, cleaner: orchestrator sorts via a callback the provider exposes. But to keep this plan tractable, push sorting into the gogoanime provider itself — it has access to its own ServerPriority + HostExtractor via Deps. Adjust the contract: the provider's GetStreamWithGate is RESPONSIBLE for sorting the passed `servers` slice using its own priority config. Update Task 3's coldPathGated to call `SortByPriority(servers, p.serverPriority, p.hostExtractor)` BEFORE iterating. (Recommended: do this — keeps orchestrator generic, provider-specific behavior contained.)
    2. **Edit services/scraper/internal/handler/scraper.go**:
       - In `GetStream` handler, replace `h.svc.GetStream(...)` with `h.svc.GetStreamGated(...)`. Receive `(stream, gated, err)`. Pass `gated` to `writeSuccess`.
       - For backward compat, keep `h.svc.GetStream` for any tests/callers that need the non-gated path; mark with a doc comment that production uses GetStreamGated.
    3. **Update services/scraper/internal/service/orchestrator_test.go**:
       - Add `TestOrchestrator_GetStreamGated_GatedProvider` (gogoanime fake satisfies gatedProvider — returns gated=true).
       - Add `TestOrchestrator_GetStreamGated_NonGatedFallback` (animepahe fake doesn't satisfy gatedProvider — returns gated=false from plain GetStream).
       - Add `TestOrchestrator_GetStreamGated_AllProvidersFail` (returns error + gated=false; lastErr surfaced).
    4. **Update services/scraper/internal/handler/scraper_test.go**:
       - Update or add tests asserting GET /scraper/stream:
         - With orchestrator returning gated=true → response body has `data.meta.gated == true`.
         - With orchestrator returning gated=false → `data.meta.gated` ABSENT from response JSON.
         - tried field still present in both cases.
    5. **Sort placement (authoritative — supersedes Task 3 "caller's responsibility" comment):** Priority sorting MUST happen inside `coldPathGated` as the FIRST statement, so callers never need to know about priority. The executor MUST:
       - In `gogoanime/client.go` `coldPathGated`, prepend `servers = SortByPriority(servers, p.serverPriority, p.hostExtractor)` before the parallel top-2 loop.
       - Update the `GetStreamWithGate` Godoc to drop the "caller's responsibility" framing; document that priority sorting happens internally in the cold path.
       - When implementing Task 3 tests (or here, if amending), pass UNSORTED fixtures like `[vibeplayer, earnvids, streamhg]` and assert that streamhg is probed first. This proves the internal sort works rather than test-pre-sorting masking a bug.
  </action>
  <verify>
    <automated>cd /data/animeenigma/services/scraper && go test ./internal/service/... ./internal/handler/... ./internal/providers/gogoanime/... -count=1 -race</automated>
  </verify>
  <done>
    - `orchestrator.GetStreamGated(...)` exists and returns `(*Stream, bool, error)`.
    - `gatedProvider` interface declared in orchestrator.go; gogoanime.Provider satisfies it.
    - Handler GET /scraper/stream wires the bool through to `writeSuccess(..., gated)`.
    - `grep -c "GetStreamGated" services/scraper/internal/handler/scraper.go` returns 1+.
    - `grep -c "GetStreamGated" services/scraper/internal/service/orchestrator.go` returns 1+.
    - `grep -c "gatedProvider" services/scraper/internal/service/orchestrator.go` returns 1+.
    - All new + existing handler + orchestrator tests pass.
  </done>
</task>

<task type="auto" tdd="true">
  <name>Task 5: Boot wiring — main.go threads ServerPriority + HostExtractor + ValidatePriorityList fail-fast</name>
  <files>services/scraper/cmd/scraper-api/main.go</files>
  <read_first>
    - services/scraper/cmd/scraper-api/main.go (entire file — find the registry.Register calls + the gogoanime.New call; insert the priority/hostExtractor wiring before gogoanime.New)
    - services/scraper/internal/providers/gogoanime/server_priority.go (Task 1 — ValidatePriorityList signature)
    - services/scraper/internal/embeds/streamhg.go (extractor.Name(), hosts slice — to build the hostExtractor map)
  </read_first>
  <behavior>
    - main.go boots without error when SCRAPER_SERVER_PRIORITY is unset (default applied).
    - main.go FATALs with a clear error message when SCRAPER_SERVER_PRIORITY contains an unknown name (e.g. SCRAPER_SERVER_PRIORITY=streamg,earnvids,vibeplayer); the error string contains the typo'd name.
    - gogoanime.Provider receives Deps.ServerPriority + Deps.HostExtractor populated correctly.
    - hostExtractor map is built by iterating the registered embed extractors' hosts (each extractor exposes a Name() + a slice of hosts; see streamhg.go's streamhgHosts).
  </behavior>
  <action>
    1. **Edit services/scraper/cmd/scraper-api/main.go**, just BEFORE the gogoanime.New(...) call:
       ```go
       // Build the hostExtractor map: every registered embed extractor's
       // hosts are reverse-mapped to its Name(). Used by gogoanime's
       // SortByPriority + coldPathGated to map a server URL → extractor name
       // → priority index.
       //
       // We rely on a small interface satisfied by every embed extractor in
       // services/scraper/internal/embeds — each exposes Hosts() []string
       // alongside its Name(). If the extractor type does NOT expose Hosts(),
       // add a method on it. (StreamHG/Earnvids/VibePlayer all have hosts
       // slices today — exposing them via Hosts() is a 3-line change per
       // extractor; do it as part of this task.)
       hostExtractor := map[string]string{}
       for _, ext := range []embeds.HostingExtractor{
           vibeplayerExtractor, streamhgExtractor, earnvidsExtractor,
       } {
           for _, h := range ext.Hosts() {
               hostExtractor[strings.ToLower(h)] = ext.Name()
           }
       }

       // Validate SCRAPER_SERVER_PRIORITY against known extractor names.
       // Fail-fast at boot if an unknown name is configured (typo guard).
       known := []string{}
       for _, ext := range []embeds.HostingExtractor{
           vibeplayerExtractor, streamhgExtractor, earnvidsExtractor,
       } {
           known = append(known, ext.Name())
       }
       if err := gogoanime.ValidatePriorityList(cfg.Gogoanime.ServerPriority, known); err != nil {
           log.Fatalw("invalid SCRAPER_SERVER_PRIORITY", "error", err)
       }
       ```
    2. **Define embeds.HostingExtractor** in services/scraper/internal/embeds/embed.go or a new tiny file `services/scraper/internal/embeds/types.go`:
       ```go
       // HostingExtractor is the optional surface every URL-host-bound
       // embed extractor implements. Hosts() returns the lowercase suffix
       // list this extractor matches. Used by main.go to build the
       // host→extractor-name map for gogoanime's SortByPriority.
       type HostingExtractor interface {
           Name() string
           Hosts() []string
       }
       ```
       Then add `func (e *VibePlayerExtractor) Hosts() []string { return vibeplayerHosts }` (and equivalents for StreamHG, Earnvids). The `*Hosts` constants exist in those files already.
    3. **Update the gogoanime.New(...) call in main.go** to pass the new fields:
       ```go
       gogoanimeProvider, err := gogoanime.New(gogoanime.Deps{
           BaseURL:        cfg.Gogoanime.BaseURL,
           HTTP:           gogoanimeBaseHTTP, // existing
           Embeds:         registry,
           MalSync:        gogoanimeMalSync, // existing
           Cache:          redisCache,
           Log:            log,
           ServerPriority: cfg.Gogoanime.ServerPriority,
           HostExtractor:  hostExtractor,
           Probe:          nil, // defaults to streamprobe.Probe inside New
       })
       ```
    4. Run `cd services/scraper && go build ./...` to confirm the wiring compiles.
  </action>
  <verify>
    <automated>cd /data/animeenigma/services/scraper && go build ./...</automated>
  </verify>
  <done>
    - main.go builds + the priority validation lives BEFORE gogoanime.New.
    - `embeds.HostingExtractor` interface defined; VibePlayer, StreamHG, Earnvids each implement Hosts().
    - `grep -c "ValidatePriorityList" services/scraper/cmd/scraper-api/main.go` returns 1+.
    - `grep -c "HostExtractor" services/scraper/cmd/scraper-api/main.go` returns 1+.
    - `cd services/scraper && go build ./...` succeeds.
  </done>
</task>

<task type="auto" tdd="true">
  <name>Task 6: Maintenance-prompt reason-enum sync invariant test</name>
  <files>services/scraper/internal/providers/gogoanime/maintenance_prompt_sync_test.go</files>
  <read_first>
    - .claude/maintenance-prompt.md (find Pattern 6/7 + the "Scraper Playability Regression" alert section + the reason-enum dispatch table)
    - libs/streamprobe/reason.go (AllReasons() helper from Plan 21-01)
    - .planning/phases/21-playability-foundation/21-CONTEXT.md D7 ("typed string + compile-time check via a sentinel const + a comment-block reference")
  </read_first>
  <behavior>
    - Test: reads `.claude/maintenance-prompt.md` from the repo root (find via `go test` working dir + `..` traversal up to the project root — use `runtime.Caller` + `filepath.Join` walk-up). Asserts every Reason value from `streamprobe.AllReasons()` appears as a literal substring in the file. If a NEW Reason is added later without updating the prompt, this test fails — surfaces the prompt-drift risk at CI time, not in production.
    - Test (skip): if maintenance-prompt.md is unreadable (filesystem layout differs in CI), skip with `t.Skip` rather than fail — keeps the test useful locally without blocking CI runs in container builds where the file path may differ. (The test asserts on file existence first; missing file → Skip.)
  </behavior>
  <action>
    1. **Create services/scraper/internal/providers/gogoanime/maintenance_prompt_sync_test.go**:
       ```go
       package gogoanime

       import (
           "os"
           "path/filepath"
           "runtime"
           "strings"
           "testing"

           "github.com/ILITA-hub/animeenigma/libs/streamprobe"
       )

       // TestMaintenancePromptCoversAllReasons enforces the SCRAPER-HEAL spec
       // invariant: every libs/streamprobe.Reason value must appear in
       // .claude/maintenance-prompt.md so the maintenance bot's reason-enum
       // dispatch (Patterns 6/7) covers every possible failure mode.
       //
       // Phase 21 D7. Drift surfaces at CI time, not in production.
       func TestMaintenancePromptCoversAllReasons(t *testing.T) {
           _, thisFile, _, _ := runtime.Caller(0)
           // walk up to the repo root: services/scraper/internal/providers/gogoanime/ → 4 levels
           repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "..", "..")
           promptPath := filepath.Join(repoRoot, ".claude", "maintenance-prompt.md")
           data, err := os.ReadFile(promptPath)
           if err != nil {
               t.Skipf("maintenance-prompt.md not readable at %s: %v", promptPath, err)
           }
           content := string(data)
           for _, r := range streamprobe.AllReasons() {
               if !strings.Contains(content, string(r)) {
                   t.Errorf("maintenance-prompt.md does not mention Reason=%q; add it to the reason-enum dispatch table (Patterns 6/7)", r)
               }
           }
       }
       ```
    2. **Verify the prompt currently contains all 7 reason values**:
       - playable: likely absent (only failures get fixed; "playable" is success). The Reason=playable is a control case — exclude it from the prompt assertion OR document this. **Decision:** exclude `ReasonPlayable` from the assertion (it's not a failure-mode dispatch). Filter via:
         ```go
         for _, r := range streamprobe.AllReasons() {
             if r == streamprobe.ReasonPlayable { continue }
             ...
         }
         ```
       - The remaining 6 (ad_decoy, zero_match, status_403, signed_url_expired, cdn_unreachable, empty_response): the existing prompt covers ad_decoy + zero_match (Patterns 6/7) and 403_upstream + signed_url_expired + cdn_unreachable + empty_response (the Scraper Playability Regression section). Verify by reading the file; if any is missing, this test SHOULD fail — that's the design — and the maintenance-prompt.md MUST be updated by the developer as part of this task to add the missing entries.
       - Cross-reference the prompt's existing names: the prompt uses `403_upstream` (no leading `status_`) for the 403 alert label. Decide: either (a) align the Reason value to `403_upstream` to match the prompt, or (b) add `status_403` to the prompt as an additional dispatch case. Plan 21-01 already shipped `status_403` as the Reason string, so go with (b): edit .claude/maintenance-prompt.md to add `status_403` next to the existing `403_upstream` mention. This is a 1-line tweak in the prompt; verify after editing that the test passes.
    3. **If the prompt needs editing**: edit `.claude/maintenance-prompt.md` line containing `403_upstream` to read `status_403 / 403_upstream` (or similar dual-mention). This is a documentation-style adjustment, NOT a behavioral change to the bot.
  </action>
  <verify>
    <automated>cd /data/animeenigma/services/scraper && go test ./internal/providers/gogoanime/... -count=1 -run "TestMaintenancePromptCoversAllReasons"</automated>
  </verify>
  <done>
    - `maintenance_prompt_sync_test.go` exists; asserts every non-`playable` Reason value is a substring of `.claude/maintenance-prompt.md`.
    - Test passes against the current `.claude/maintenance-prompt.md`.
    - Any future addition of a Reason value will fail this test until the prompt is updated.
  </done>
</task>

<task type="checkpoint:human-verify" gate="blocking">
  <name>Task 7: Production smoke — Frieren ep1 sub plays real video through gogoanime</name>
  <what-built>
    Cold-path GetStream now iterates server priority via streamprobe gate. VibePlayer (ad-decoy) is gate-failed and skipped; StreamHG or Earnvids serves real video. Latency masked by Plan 21-04's loader (ships in this wave too).
  </what-built>
  <how-to-verify>
    1. Run `make redeploy-scraper` from /data/animeenigma (NOT inside docker — the Makefile target handles container rebuild).
    2. Wait for scraper to be healthy: `curl -sf http://localhost:8088/health` returns 200.
    3. Confirm metric registration: `curl -s http://localhost:8088/metrics | grep -E "parser_unplayable_total|parser_ad_decoy_total"` returns at least two HELP/TYPE lines (counters may be 0 — the line presence is what matters).
    4. Confirm config: `docker compose -f docker/docker-compose.yml exec scraper env | grep SCRAPER_SERVER_PRIORITY` shows the value (unset is fine — default applies).
    5. Force a cold-path stream resolution for Frieren ep 1 sub:
       ```
       # Frieren MAL ID = 52991
       curl -s "http://localhost:8000/api/anime/52991/scraper/stream?mal_id=52991&episode=frieren-beyond-journeys-end-episode-1&server=https://otakuhg.site/e/<resolved-id>&category=sub" | jq .
       ```
       (Use a real serverID from `curl http://localhost:8000/api/anime/52991/scraper/servers?mal_id=52991&episode=...` first.)
    6. Assert the response body has `data.meta.gated == true` AND `data.stream.sources[0].url` does NOT contain `ibyteimg.com` or `p16-ad-sg`.
    7. Open https://animeenigma.ru in a logged-in browser tab. Search Frieren. Click episode 1. Confirm real video plays (no permanent spinner, no "ad" overlay, network tab segments come from `premilkyway.com` or `dramiyos-cdn.com`).
    8. After a successful first play, repeat the same episode within 5 minutes — confirm `data.meta.gated` is absent (warm cache path).
    9. Check scraper logs for any ERROR-level lines: `make logs-scraper | grep -i error | tail -20`.
  </how-to-verify>
  <resume-signal>
    Reply "approved" if Frieren ep1 plays real video AND meta.gated=true on first call, absent on second. Reply with details if any assertion fails — the orchestrator will run /gsd-plan-phase --gaps to address.
  </resume-signal>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| upstream embed URL → Probe | the gate fetches third-party m3u8s; libs/streamprobe's SSRF guard (Plan 21-01) covers this |
| SCRAPER_SERVER_PRIORITY env → priority sort | env is operator-controlled; typo silently demoting a real server is the documented risk (CONTEXT.md) — mitigated by ValidatePriorityList fail-fast |
| Redis winning_server cache → next-call path | cached value is a serverID (URL string) we wrote ourselves; not user input; cache poisoning requires Redis compromise |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-21-09 | T (Tampering) | SCRAPER_SERVER_PRIORITY env typo | mitigate | Task 1's ValidatePriorityList rejects unknowns at boot; Task 5 calls it before gogoanime.New; service fails to start with a clear log line naming the typo'd entry. |
| T-21-10 | I (Information disclosure) | parser_unplayable_total label cardinality | mitigate | server label = extractor.Name() (closed set of ~5 values); reason = streamprobe.Reason (closed set of 7); provider = providerName (1 value here). Total cardinality bounded at ~35 series. |
| T-21-11 | D (DoS) | Probe-on-every-cold-path adds 1-2s per call | accept | Per CONTEXT.md D2: gate runs on every cold-path stream resolution; warm-path cached 5m. Latency cost masked by Plan 21-04 Phase 3 loader. Cold-path budget 8s total (top-2 parallel + sequential 3+). |
| T-21-12 | E (Elevation) | Redis cache poisoning writes arbitrary serverID | mitigate | The cache value is a serverID (URL string) only WE write — never accept it back from user input. On cache hit, Task 3 validates the cached serverID is still in the supplied servers list before using it; stale entries deleted on extract failure. |
| T-21-13 | R (Repudiation) | Gate failures invisible to ops | mitigate | parser_unplayable_total + parser_ad_decoy_total emit per fail; Phase 23 alerts on rate > 0. |
</threat_model>

<verification>
- `cd /data/animeenigma && go work sync` exits 0.
- `cd /data/animeenigma/services/scraper && go test ./... -count=1 -race` passes.
- `grep -c "GetStreamWithGate\\|GetStreamGated\\|scraper:winning_server\\|streamprobe.Probe\\|SCRAPER_SERVER_PRIORITY\\|ValidatePriorityList" services/scraper/ -r` returns ≥ 10 across the modified files.
- Production smoke (Task 7) confirms real video playback + meta.gated semantics.
- `curl -s http://localhost:8088/metrics | grep -c "parser_unplayable_total\\|parser_ad_decoy_total"` returns ≥ 4 (HELP + TYPE for each).
</verification>

<success_criteria>
- SCRAPER-HEAL-03: `SCRAPER_SERVER_PRIORITY` env (CSV, default `streamhg,earnvids,vibeplayer`) sorts ListServers output; typo'd entries fail-fast at boot.
- SCRAPER-HEAL-04: `gogoanime.GetStreamWithGate` iterates priority list, runs `streamprobe.Probe` on each candidate, returns first playable. Total budget ≤ 8s. Top-2 parallel.
- SCRAPER-HEAL-05: Winning serverID cached at `scraper:winning_server:gogoanime:<anime>:<ep>` for 5 min; warm path skips the gate.
- parser_unplayable_total + parser_ad_decoy_total increment correctly on every gate fail.
- meta.gated flows from provider → orchestrator → handler → JSON response.
- Production smoke: Frieren ep1 plays real video end-to-end.
</success_criteria>

<output>
After completion, create `.planning/phases/21-playability-foundation/21-21-03-SUMMARY.md` documenting:
- The GetStreamWithGate flow (caller-pinned → cache-hit → priority cold path with parallel top-2)
- The cache key + TTL
- The metric label values exercised
- The Production smoke result (which server actually won for Frieren ep1)
- Any deviations from plan (e.g. if the parallel top-2 needed tuning)
</output>
