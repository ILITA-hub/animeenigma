---
phase: 16-animepahe-and-new-englishplayer
fixed_at: 2026-05-12T05:39:32Z
review_path: .planning/phases/16-animepahe-and-new-englishplayer/16-REVIEW.md
iteration: 1
findings_in_scope: 16
fixed: 16
skipped: 0
status: all_fixed
---

# Phase 16: Code Review Fix Report

**Fixed at:** 2026-05-12T05:39:32Z
**Source review:** `.planning/phases/16-animepahe-and-new-englishplayer/16-REVIEW.md`
**Iteration:** 1

**Summary:**
- Findings in scope: 16 (5 Critical + 11 Warning)
- Fixed: 16
- Skipped: 0

All five BLOCKER-class defects and all eleven WARNING-class defects from
`16-REVIEW.md` were fixed and committed atomically. Critical issues were
prioritized first; each commit includes a regression test (or contract test)
where the original defect was non-trivially testable. Info-level findings
(7) are out of scope for this iteration per `fix_scope: critical_warning`.

Go test suites for `services/scraper/...` and `services/catalog/...` pass.
Frontend `bunx tsc --noEmit` clean. ESLint on every modified Vue / TS file
clean.

## Fixed Issues

### CR-01: Host port 8088 is bound twice in docker-compose.yml

**Files modified:** `docker/docker-compose.yml`
**Commit:** 92f6ea5
**Applied fix:** Moved `admin-nginx` from `127.0.0.1:8088:80` to
`127.0.0.1:8089:80` so the scraper retains the documented 8088 port
referenced by `make health` and the gateway env var `SCRAPER_API_URL`.
Added inline comment pointing back to CR-01 and CLAUDE.md.

### CR-02: AnimePahe Server entries lack `Type` (sub/dub) — frontend filter always empty

**Files modified:** `services/scraper/internal/domain/provider.go`,
`services/scraper/internal/providers/animepahe/client.go`,
`services/scraper/internal/providers/animepahe/client_test.go`,
`services/scraper/internal/handler/scraper_test.go`
**Commit:** 32ec4cc
**Applied fix:** Added `Type Category` to `domain.Server` and populated it
in AnimePahe's `ListServers` by reading the `data-audio` attribute on
each `button[data-src]`. Defaults to `CategorySub` when the attribute is
missing or unknown (the dominant case on AnimePahe). Added
`TestProvider_ListServers_DubAudio` and tightened the happy-path test to
count sub-tagged entries — both serve as regression anchors against the
frontend's empty `subServers`/`dubServers` lists.

### CR-03: scraperApi never sends `mal_id`; every scraper request returns 400

**Files modified:** `services/catalog/internal/handler/scraper_test.go`
**Commit:** 209a816
**Applied fix:** The production code path already injects `mal_id` correctly
(catalog `scraperOps.resolveMALID` → real `scraper.Client.GetEpisodes`
builds `?mal_id=N`, verified independently by client + service unit tests).
What was missing was an end-to-end CONTRACT test that fails loudly if any
future refactor breaks the chain. Added
`TestCatalogHandler_ScraperPipeline_InjectsMalID` — fires a frontend-style
request through the catalog handler + a fake `scraperServiceAPI` that
forwards to a real `httptest` scraper server, captures the upstream URL,
and asserts `mal_id=12345` appears.

### CR-04: ReportButton's dynamic Tailwind classes are unstyled at build time

**Files modified:** `frontend/web/src/components/player/ReportButton.vue`
**Commit:** 4c9869a
**Applied fix:** Replaced the JIT-incompatible template-string
`bg-[${accentColor}]/20 text-[${accentColor}] hover:bg-[${accentColor}]/30`
with an inline `:style` binding using `color-mix(in srgb, ${accentColor}
20%, transparent)` for the background, the accent color directly for the
text, and a scoped CSS rule that lifts the background to 30% on hover via
a `--report-accent` CSS custom property. Works on any Tailwind version
and is robust against future accentColor values (EnglishPlayer passes
`#00d4ff`).

### CR-05: DDoS-Guard cookie-name match fails — real cookies are `__ddg2_<suffix>`, not exactly `__ddg2_`

**Files modified:** `services/scraper/internal/providers/animepahe/ddosguard.go`,
`services/scraper/internal/providers/animepahe/ddosguard_test.go`
**Commit:** 920fd6e
**Applied fix:** Renamed `ddosCookieName` → `ddosCookieNamePrefix` and
switched both jar checks (the idempotency short-circuit and the
post-bypass jar re-check) from `c.Name == ddosCookieName` to
`strings.HasPrefix(c.Name, ddosCookieNamePrefix)`. Updated existing tests
to use real-shape cookie names (`__ddg2_BvHvjMmh`) and added
`TestEnsureDDoSCookie_PrefixMatch_NoHTTP` as a dedicated regression
anchor that asserts a pre-populated jar short-circuits with zero HTTP
calls.

### WR-01: Scraper handler's `prefer` parameter is forwarded unbounded

**Files modified:** `services/scraper/internal/handler/scraper.go`,
`services/scraper/internal/handler/scraper_test.go`
**Commit:** 5a0daaf
**Applied fix:** Introduced `maxPreferLength = 64` and truncated the
`prefer` query parameter at parse time (after TrimSpace). Provider names
are short identifiers ("animepahe", "9anime") so legitimate callers are
nowhere near the cap. Added `TestParseQuery_PreferLengthCap` with a
1024-char input asserting the result is exactly 64 chars.

### WR-02: `handleFullscreenChange` operator precedence bug

**Files modified:** `frontend/web/src/components/player/EnglishPlayer.vue`
**Commit:** 0e479c5
**Applied fix:** Restructured the predicate with an early return for the
no-fullscreen-element case and a single explicit add-class predicate so
the intent (`fsEl != null AND (contains || equals container)`) matches
the implementation. Added a code comment explaining the precedence trap.

### WR-03: `deactivateSubtitle` sets `activeSubtitleUrl` to empty string, not null

**Files modified:** `frontend/web/src/components/player/EnglishPlayer.vue`
**Commit:** 6dea617
**Applied fix:** Changed `activeSubtitleUrl.value = ''` to
`activeSubtitleUrl.value = null` so the typing (`ref<string | null>`),
the watcher check (`!activeSubtitleUrl`), and the value all agree.
Eliminates the risk of `SubtitleOverlay` doing a load-of-empty-url
network call.

### WR-04: `resumeStartEpisode` typed as `number | undefined`, 0 is silently swallowed

**Files modified:** `frontend/web/src/components/player/EnglishPlayer.vue`
**Commit:** b8b9093
**Applied fix:** Replaced the truthy check
`props.initialEpisode ? find : first` with an explicit
`(requested != null && requested > 0) ? find : first` so a stray 0
(which `parseInt("0")` upstream could produce) doesn't silently fall
through to "use the first one" — instead it's treated as "no specific
episode requested," which is the correct contract.

### WR-05: `kwikHosts` matcher accepts non-http(s) schemes

**Files modified:** `services/scraper/internal/embeds/kwik.go`,
`services/scraper/internal/embeds/kwik_test.go`,
`services/scraper/internal/providers/animepahe/client.go`
**Commit:** 6137854
**Applied fix:** Added an explicit scheme check
(`u.Scheme != "http" && u.Scheme != "https" → false`) to both
`KwikExtractor.Matches` and AnimePahe's `ListServers` host filter. Added
three rejection cases to `TestKwik_Matches` (`kwik://`, `ftp://`,
`file://`) so the SSRF-style URLs never propagate to the orchestrator's
extract step where they'd produce an unhelpful ErrProviderDown.

### WR-06: AnimePahe `GetStream` does not honor the requested `category` parameter

**Files modified:** `services/scraper/internal/providers/animepahe/client.go`
**Commit:** d7bf144
**Applied fix:** Documented the contract explicitly. After CR-02, sub/dub
selection happens at `ListServers` time (each kwik URL is tagged with
its `Server.Type` derived from `data-audio`), so per-stream filtering on
category is redundant. Added a `_ = category` blank assignment with a
function-level comment explaining the parameter is informational rather
than a missed feature, so future readers do not "fix" it by adding
spurious branching.

### WR-07: malsync provider can pick a non-deterministic entry from a map iteration

**Files modified:** `services/scraper/internal/providers/animepahe/malsync.go`
**Commit:** b3a0324
**Applied fix:** Sort the `site` map's keys lexicographically before
iterating so the chosen identifier is deterministic across processes
and restarts. Added `sort` to the imports. The cache value is now stable
per `(mal_id, provider)` key.

### WR-08: Diagnostics console interception holds references to large argument objects forever

**Files modified:** `frontend/web/src/utils/diagnostics.ts`
**Commit:** 0ce653f
**Applied fix:** Extracted a `safeStringify` helper that short-circuits
primitives, uses a `WeakSet`-based replacer to break circular references
(`[Circular]` placeholder), and caps each argument at `maxLen` (default
2000) so a single deep object can't blow up the synchronous stringify
walk. The existing `.slice(0, 2000)` on the joined message is preserved,
so the total stored message is still bounded.

### WR-09: e2e test depends on a magic UUID that may not exist in the seed

**Files modified:** `frontend/web/e2e/english-player.spec.ts`
**Commit:** e9bb958
**Applied fix:** Replaced the hardcoded UUID with a `resolveTestAnimeID()`
helper that hits `GET /api/anime/search?q=Frieren` from inside the page
and picks the first result. Renamed the old constant to
`FALLBACK_TEST_ANIME_ID` and made it inactive — the helper throws an
explicit error when zero hits are returned rather than falling back to
the constant, so seed drift produces a loud failure instead of a silent
false-pass.

### WR-10: `parseInt(ep)` in localStorage parse missing radix

**Files modified:** `frontend/web/src/views/Anime.vue`
**Commit:** f610c32
**Applied fix:** Changed `parseInt(ep)` to `parseInt(ep, 10)` and added a
WR-10 reference comment. Defends against the historic octal-on-leading-
zero foot-gun and satisfies ESLint's `radix` rule.

### WR-11: `Provider.New` panics on missing dependencies rather than failing at construction

**Files modified:** `services/scraper/internal/providers/animepahe/client.go`,
`services/scraper/internal/providers/animepahe/client_test.go`,
`services/scraper/cmd/scraper-api/main.go`
**Commit:** 2d0841e
**Applied fix:** Changed `New(Deps) *Provider` to
`New(Deps) (*Provider, error)`. Added explicit nil checks for `HTTP`,
`Embeds`, `MalSync`, and `Cache` that return descriptive errors. `Log`
falls back to `logger.Default()` when omitted. Main.go now fatals on the
error so misconfiguration is caught at boot rather than via a runtime
nil-pointer dereference minutes after deploy. Added
`TestNew_RequiresDependencies` (4 missing-dep cases + a "Log optional"
happy path) and updated `newTestProvider` in the existing test helper
to consume the new error return.

## Skipped Issues

None — all in-scope findings were fixed.

---

_Fixed: 2026-05-12T05:39:32Z_
_Fixer: Claude (gsd-code-fixer)_
_Iteration: 1_
