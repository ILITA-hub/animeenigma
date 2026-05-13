---
id: 21-02
phase: 21
plan: 02
type: execute
wave: 1
depends_on: []
files_modified:
  - libs/metrics/provider.go
  - libs/metrics/provider_test.go
  - services/scraper/internal/handler/scraper.go
  - services/scraper/internal/handler/scraper_test.go
autonomous: true
requirements:
  - SCRAPER-HEAL-06
  - SCRAPER-HEAL-07
user_setup: []
must_haves:
  truths:
    - "Scraper /metrics exposes parser_unplayable_total{provider,server,reason}"
    - "Scraper /metrics exposes parser_ad_decoy_total{provider,server}"
    - "GET /scraper/stream response carries meta.gated boolean when the gate ran on this call"
    - "Existing parser_zero_match_total counter remains unchanged"
    - "meta.tried (Phase 16) still emitted on every response — meta is a single object containing both tried and gated"
  artifacts:
    - path: "libs/metrics/provider.go"
      provides: "ParserUnplayableTotal, ParserAdDecoyTotal counter vars"
      contains: "ParserUnplayableTotal"
    - path: "services/scraper/internal/handler/scraper.go"
      provides: "writeSuccess accepts a gated bool; emits meta:{tried,gated}"
      contains: "gated"
  key_links:
    - from: "services/scraper/internal/handler/scraper.go"
      to: "libs/metrics/provider.go"
      via: "import + counter Reset() in tests"
      pattern: "ParserUnplayableTotal"
    - from: "libs/metrics/provider.go"
      to: "libs/streamprobe/reason.go"
      via: "label values match Reason consts (string identity, not import)"
      pattern: "ad_decoy"
---

<objective>
Land the metrics + handler-envelope changes that don't depend on libs/streamprobe at the type level. Adds two new counters (`parser_unplayable_total`, `parser_ad_decoy_total`) to libs/metrics, threads a `gated bool` parameter through the scraper handler's `writeSuccess` so `GET /scraper/stream` JSON responses include `meta.gated`. SCRAPER-HEAL-06 + SCRAPER-HEAL-07.

Purpose: Wave-1 alongside 21-01 (no file overlap, no import overlap). Plan 21-03 lights up these counters from the gogoanime GetStream path (cold-path increments). Plan 21-04 reads `meta.gated` in EnglishPlayer.vue.

Output: Counters declared + tested in libs/metrics; handler emits `meta:{tried, gated}` envelope; existing 200-path tests updated to assert `gated:false` default; new test verifies `gated:true` propagation.
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
<!-- Existing counter pattern from libs/metrics/provider.go (extracted): -->

```go
var ParserZeroMatchTotal = promauto.NewCounterVec(
    prometheus.CounterOpts{
        Name: "parser_zero_match_total",
        Help: "Total count of HTML/JSON selector-miss events per (provider, selector)",
    },
    []string{"provider", "selector"},
)
```

<!-- Existing test pattern from libs/metrics/provider_test.go: -->

```go
func TestParserZeroMatchTotal_IncrementsCorrectly(t *testing.T) {
    ParserZeroMatchTotal.Reset()
    before := testutil.ToFloat64(ParserZeroMatchTotal.WithLabelValues("animepahe", "episode_list_item"))
    ParserZeroMatchTotal.WithLabelValues("animepahe", "episode_list_item").Inc()
    after := testutil.ToFloat64(ParserZeroMatchTotal.WithLabelValues("animepahe", "episode_list_item"))
    if d := after - before; d != 1.0 {
        t.Fatalf("ParserZeroMatchTotal delta = %v; want 1.0", d)
    }
    name, labels := descMeta(t, ParserZeroMatchTotal)
    ...
}
```

<!-- Existing writeSuccess signature in services/scraper/internal/handler/scraper.go (extracted): -->

```go
func (h *ScraperHandler) writeSuccess(w http.ResponseWriter, data map[string]any, tried []string) {
    if tried == nil {
        tried = []string{}
    }
    data["meta"] = map[string]any{"tried": tried}
    httputil.OK(w, data)
}
```

<!-- This plan changes writeSuccess to accept a `gated bool` and emit
     meta:{tried, gated}. Callsites: GetEpisodes (gated=false), GetServers
     (gated=false), GetStream (gated=false in this plan — Plan 21-03 will pass
     true via a new orchestrator return path). -->

<!-- Reason values that will appear as label values on ParserUnplayableTotal —
     match libs/streamprobe/reason.go (Plan 21-01) by STRING IDENTITY, not import.
     The metrics package MUST NOT import libs/streamprobe (avoids the cyclic
     potential and keeps libs/metrics dependency-free for downstream consumers). -->
</interfaces>
</context>

<tasks>

<task type="auto" tdd="true">
  <name>Task 1: Add ParserUnplayableTotal + ParserAdDecoyTotal counters to libs/metrics</name>
  <files>libs/metrics/provider.go, libs/metrics/provider_test.go</files>
  <read_first>
    - libs/metrics/provider.go (full file — extend the existing var block; preserve the package doc comment that explains label-value cardinality discipline)
    - libs/metrics/provider_test.go (full file — copy the TestParserZeroMatchTotal_IncrementsCorrectly pattern + the descMeta() helper)
    - docs/plans/2026-05-13-scraper-self-healing-spec.md §4.3.a (metric names, label sets)
    - .planning/milestones/v3.1-REQUIREMENTS.md SCRAPER-HEAL-06 (exact counter names + label sets)
  </read_first>
  <behavior>
    - Test: `ParserUnplayableTotal.WithLabelValues("gogoanime", "vibeplayer", "ad_decoy").Inc()` increments by 1.0; metric NAME is exactly `parser_unplayable_total`; label names are exactly `{provider, server, reason}` in that order.
    - Test: `ParserAdDecoyTotal.WithLabelValues("gogoanime", "vibeplayer").Inc()` increments by 1.0; metric NAME is exactly `parser_ad_decoy_total`; label names are exactly `{provider, server}` in that order.
    - Test: `ParserZeroMatchTotal` declaration is unchanged (name + label set).
    - Test: Each Reason value from the spec (`playable, ad_decoy, zero_match, status_403, signed_url_expired, cdn_unreachable, empty_response`) is exercisable as a `reason` label value without panicking — a table test iterates the 7 strings, asserting each `.WithLabelValues(...).Inc()` returns a non-nil counter (defense against accidental cardinality limits).
  </behavior>
  <action>
    1. **Edit libs/metrics/provider.go** — extend the var block AFTER `ParserZeroMatchTotal`:
       ```go
       // ParserUnplayableTotal counts playability-gate failures observed inside
       // a provider's GetStream path. label `reason` MUST be one of the 7 typed
       // values defined in libs/streamprobe/reason.go (string identity — this
       // package does not import libs/streamprobe to keep libs/metrics
       // dependency-free).
       //
       // SCRAPER-HEAL-06.
       ParserUnplayableTotal = promauto.NewCounterVec(
           prometheus.CounterOpts{
               Name: "parser_unplayable_total",
               Help: "Total count of playability-gate failures per (provider, server, reason). reason is one of libs/streamprobe.Reason values.",
           },
           []string{"provider", "server", "reason"},
       )

       // ParserAdDecoyTotal counts the subset of ParserUnplayableTotal where
       // reason == "ad_decoy" — a dedicated counter so the Prometheus alert
       // rule "ScraperAdDecoySurge" (Phase 23) can fire on a simple non-zero
       // rate without label-matching.
       //
       // SCRAPER-HEAL-06.
       ParserAdDecoyTotal = promauto.NewCounterVec(
           prometheus.CounterOpts{
               Name: "parser_ad_decoy_total",
               Help: "Total count of playability-gate ad-decoy classifications per (provider, server). Subset of parser_unplayable_total with reason='ad_decoy'.",
           },
           []string{"provider", "server"},
       )
       ```
       Also update the package doc-comment at the top of the file: append the two new counter names to the existing bullet list ("Three collectors" → "Five collectors" and list the two new ones).
    2. **Edit libs/metrics/provider_test.go** — append three new tests modeled on the existing pattern:
       - `TestParserUnplayableTotal_IncrementsCorrectly`: reset, increment with ("gogoanime", "vibeplayer", "ad_decoy"), assert delta = 1.0; assert metric name == `parser_unplayable_total`; assert label names ordered as `provider, server, reason`.
       - `TestParserAdDecoyTotal_IncrementsCorrectly`: reset, increment with ("gogoanime", "vibeplayer"), assert delta = 1.0; assert metric name + label names.
       - `TestParserUnplayableTotal_AllReasonsAccepted`: table test iterating the 7 reason strings literal (`"playable", "ad_decoy", "zero_match", "status_403", "signed_url_expired", "cdn_unreachable", "empty_response"`); for each, call `.WithLabelValues("gogoanime", "vibeplayer", reason)` and assert no nil + no panic.
  </action>
  <verify>
    <automated>cd /data/animeenigma/libs/metrics && go test ./... -count=1 -run "TestParserUnplayableTotal|TestParserAdDecoyTotal|TestParserZeroMatchTotal"</automated>
  </verify>
  <done>
    - `libs/metrics/provider.go` declares `ParserUnplayableTotal` (CounterVec, labels: provider, server, reason) and `ParserAdDecoyTotal` (CounterVec, labels: provider, server).
    - `grep -c "ParserUnplayableTotal\|ParserAdDecoyTotal" libs/metrics/provider.go` returns ≥ 4 (declaration + Help comment for each).
    - All three new tests pass.
    - Existing `TestParserZeroMatchTotal_IncrementsCorrectly` still passes.
  </done>
</task>

<task type="auto" tdd="true">
  <name>Task 2: Thread `gated bool` through writeSuccess; emit meta.gated on /scraper/stream</name>
  <files>services/scraper/internal/handler/scraper.go, services/scraper/internal/handler/scraper_test.go</files>
  <read_first>
    - services/scraper/internal/handler/scraper.go (entire file — modify writeSuccess + all three call sites: GetEpisodes, GetServers, GetStream)
    - services/scraper/internal/handler/scraper_test.go (find any existing tests that assert on the envelope shape; they MUST keep passing — meta.gated should be optional/omitted in the success envelope OR explicitly false where present)
    - .planning/phases/21-playability-foundation/21-CONTEXT.md D5 ("meta.gated is a top-level meta field, not a per-source field")
    - .planning/milestones/v3.1-REQUIREMENTS.md SCRAPER-HEAL-07 (semantics: present + true when gate ran; absent or false on cache hit)
  </read_first>
  <behavior>
    - Test: `GET /scraper/stream` success response with gated=true emits JSON where `data.meta.gated == true` AND `data.meta.tried` is still present (does not regress Phase 16's tried envelope).
    - Test: `GET /scraper/stream` success response with gated=false emits JSON where `data.meta.gated` field is OMITTED (encoder uses `omitempty`-equivalent — implemented by only setting the key when true). Alternative acceptable: explicit `gated:false`. The frontend (Plan 21-04) treats both undefined and false as "no gate, skip Phase 3".
    - Test: `GET /scraper/episodes` and `GET /scraper/servers` success responses NEVER include `data.meta.gated` (gate is stream-specific).
    - Test: error envelopes still include `meta.tried`; no `meta.gated` field on error responses.
    - Test (regression): Phase 16 envelope shape preserved — `data.meta.tried` still always present as `[]string`.
  </behavior>
  <action>
    1. **Edit services/scraper/internal/handler/scraper.go** — change `writeSuccess` to take a `gated bool` parameter and only emit the field when true:
       ```go
       // writeSuccess writes 200 with envelope {success:true,
       // data:{<fields>, meta:{tried:[...], gated?:true}}}. The gated field is
       // emitted only when true (cache miss / cold path) so cache-hit responses
       // stay byte-identical to Phase 16's shape and don't churn the FE diffs.
       //
       // SCRAPER-HEAL-07.
       func (h *ScraperHandler) writeSuccess(w http.ResponseWriter, data map[string]any, tried []string, gated bool) {
           if tried == nil {
               tried = []string{}
           }
           meta := map[string]any{"tried": tried}
           if gated {
               meta["gated"] = true
           }
           data["meta"] = meta
           httputil.OK(w, data)
       }
       ```
    2. **Update all three call sites in scraper.go**:
       - `GetEpisodes`: `h.writeSuccess(w, map[string]any{"episodes": eps}, tried, false)` — gated is always false (episodes path doesn't run the gate).
       - `GetServers`: `h.writeSuccess(w, map[string]any{"servers": srvs}, tried, false)` — same.
       - `GetStream`: currently `h.writeSuccess(w, map[string]any{"stream": stream}, tried)` — change to `h.writeSuccess(w, map[string]any{"stream": stream}, tried, false)` for now. **NOTE TO PLAN 21-03**: 21-03 will replace this with a value sourced from a new `(*Stream, gated bool, error)` orchestrator return signature. For Wave 1 we ship `gated=false` literally so the build is green; 21-03 wires the real bool.
    3. **Edit services/scraper/internal/handler/scraper_test.go** (or create if absent):
       - Add `TestGetStream_MetaGatedAbsentByDefault`: post a successful stream response, parse the body, assert `data.meta.tried` is present AND `data.meta.gated` key is NOT present in the JSON map.
       - Add `TestWriteSuccess_GatedTrueEmitsField` (or refactor into a direct unit test on writeSuccess): construct a ScraperHandler with a stub orchestrator, call writeSuccess with gated=true, parse the response, assert `meta.gated == true` AND `meta.tried` present.
       - Add `TestWriteSuccess_GatedFalseOmitsField`: same as above but gated=false; assert `meta.gated` absent.
       - Verify existing tests still pass — most likely existing tests assert `data.meta.tried` present; they should not need changes if writeSuccess's signature widening is the only diff. If any test fails on the new signature, update its call to add `, false` at the end.
    4. **Run tests**: `cd services/scraper && go test ./internal/handler/... -count=1`.
  </action>
  <verify>
    <automated>cd /data/animeenigma/services/scraper && go test ./internal/handler/... -count=1 -run "TestGetStream_MetaGated|TestWriteSuccess_Gated|TestGetEpisodes|TestGetServers|TestGetStream"</automated>
  </verify>
  <done>
    - `writeSuccess(w, data, tried, gated bool)` signature in services/scraper/internal/handler/scraper.go.
    - All three call sites (GetEpisodes, GetServers, GetStream) pass `false` for `gated` (real `true` wiring lands in Plan 21-03).
    - `grep -c "writeSuccess" services/scraper/internal/handler/scraper.go` returns ≥ 4 (1 declaration + 3 call sites).
    - `grep -c "meta\[\"gated\"\]" services/scraper/internal/handler/scraper.go` returns ≥ 1.
    - Existing handler tests pass.
    - New gated-emission tests pass.
    - Phase 16 envelope regression: `data.meta.tried` always present.
  </done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| Prometheus scrape | /metrics endpoint exposed inside docker network; not internet-exposed |
| handler.writeSuccess | data + tried + gated under our control; no user input flows into label values |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-21-06 | I (Info disclosure) | reason label values | mitigate | Reason values are a closed enum (7 strings) — no user input flows into the label. Cardinality bounded at 7 × |providers| × |servers| ≈ 7 × 3 × 5 = ~100 series. |
| T-21-07 | T (Tampering) | meta.gated misset | accept | gated is set inside our handler; only Plan 21-03 wires `true` from orchestrator. The FE treats undefined === false === "skip Phase 3", so a missetting at worst hides one loader phase. No security impact. |
| T-21-08 | D (DoS) | metric label cardinality bomb | mitigate | server label values are normalized embed names (vibeplayer, streamhg, earnvids) from the embed registry — bounded set, NOT raw URLs. Provider label same. |
</threat_model>

<verification>
- `cd /data/animeenigma/libs/metrics && go test ./... -count=1` passes.
- `cd /data/animeenigma/services/scraper && go test ./internal/handler/... -count=1` passes.
- `grep -c "parser_unplayable_total" libs/metrics/provider.go` returns 1+.
- `grep -c "parser_ad_decoy_total" libs/metrics/provider.go` returns 1+.
- `grep -c "ParserUnplayableTotal\\|ParserAdDecoyTotal" libs/metrics/provider.go` returns ≥ 2.
- `grep -c "gated" services/scraper/internal/handler/scraper.go` returns ≥ 3 (parameter + envelope + comment).
- Phase 16 regression check: existing handler integration tests still pass.
</verification>

<success_criteria>
- SCRAPER-HEAL-06: `libs/metrics/provider.go` declares `ParserUnplayableTotal` (labels: provider, server, reason) + `ParserAdDecoyTotal` (labels: provider, server). Counters are registered via promauto and visible on /metrics by virtue of being a CounterVec in the standard registry.
- SCRAPER-HEAL-07: `services/scraper/internal/handler/scraper.go` `writeSuccess` signature accepts `gated bool`; `GET /scraper/stream` JSON response includes `meta.gated:true` when gated, omits the field when false. Phase 16 envelope (`meta.tried`) preserved.
- Unit tests cover the counter delta semantics + handler emission semantics in isolation. End-to-end "gated:true after a real gate ran" lands in Plan 21-03.
</success_criteria>

<output>
After completion, create `.planning/phases/21-playability-foundation/21-21-02-SUMMARY.md` documenting the counter names + label sets + the writeSuccess signature change. Note explicitly that GetStream still passes `gated=false` here (placeholder) and 21-03 wires the real bool.
</output>
