# Pitfalls Research

**Domain:** Self-hosted Go scraping service for anime piracy / aggregator mirrors (v3.0 Universal Anime Scraper)
**Researched:** 2026-05-11
**Confidence:** HIGH (most claims grounded in our own production incidents ISS-001/006/007/008/009/010, the dead-aniwatch/dead-consumet triage on 2026-05-09, and inspection of the current `hianime`, `consumet`, `kodik` parsers + `megacloud-extractor`)

This document lists pitfalls that are specific to:

1. Scraping piracy-grade anime mirrors (Cloudflare, fingerprint redirects, rotating embeds, encrypted HLS keys)
2. Doing so from a Go monorepo with ~10 active users where the dev server **is** production (`/data/animeenigma/`)
3. Doing so in a way that does not silently mutate UX semantics (the AnimeLib→Kodik fallback was just removed in commit `9347143` because it violated user expectations — we will not re-introduce that class of bug under a different name)

Generic web-scraping advice ("use a real User-Agent", "respect robots.txt") is intentionally omitted; this document only covers traps that actually bit AnimeEnigma or are realistic recurrences during v3.0.

---

## Critical Pitfalls

### Pitfall 1: Silent provider fallback that changes player technology under the user's feet

**What goes wrong:**
The user clicks the "HiAnime" (or future "AnimeKai") tab to get HLS + JP subs + quality switching. The orchestrator's primary provider returns no episodes, so it falls back to a different provider — or worse, to a Kodik iframe — and the player surface receives an `iframe_url` instead of a `sources[]` array. The frontend either renders a Kodik iframe inside English-player chrome (no quality picker, no JP subs, no tracking, no time scrubbing in the way the user picked) or renders nothing.

**Why it happens:**
"Fallback" feels like a safety feature. It is, for *the data layer* (DB row not found → try external). It is **not** safe for *playback technology*, because the four players are not interchangeable: KodikPlayer = iframe (no events), AnimeLibPlayer = HTML5 video, HiAnimePlayer/ConsumetPlayer = Video.js/HLS.js with subtitle overlay. Cross-tier fallback breaks the implicit contract that "this tab = this technology".

We have already shipped this bug twice:
- **ISS-008** (2026-03-23): AnimeLib silently fell back to Kodik iframe; users with Kodik-only translations got the wrong UX. Was disabled in commit `9347143`.
- **AnimeLib-no-Kodik-fallback feedback** (memory `feedback_animelib_no_kodik_fallback.md`, 2026-05-09): User explicitly corrected the assistant — "AniLib should mean AniLib." Empty state is correct UX; cross-tech substitution is dishonest UX.

**How to avoid:**
- Architectural rule: the v3.0 scraper service exposes one DTO shape per *player technology*, not one DTO shape per *provider*. The English-source endpoint (`/api/scrape/en/...`) returns `{sources[], tracks[], intro, outro}` and never returns `iframe_url`. If no provider can produce HLS, return HTTP 404 with `{reason: "no_english_source_available", tried: ["animekai", "animepahe", "anitaku"]}` and let the frontend show empty state.
- Fallback IS allowed inside the English tier (provider A → provider B → provider C all producing HLS) because the player tech is the same.
- Fallback IS NOT allowed across tiers (English HLS → Kodik iframe, AnimeLib MP4 → Kodik iframe).
- Test acceptance criteria: an integration test that asks the English scraper for an anime that exists only in Kodik MUST receive a 404, not a Kodik iframe URL.

**Warning signs:**
- Any DTO field named `iframe_url` returned from the English scraper service — refuse it at the type boundary, do not even define the field on the response struct.
- Any orchestrator code that calls `kodik.Client.GetEpisodeLink(...)` from the English scraper path.
- A Grafana spike in "HiAnime player tab loaded" while "HiAnime sources returned" stayed flat — means we are sending something other than HLS.

**Phase to address:**
Phase 1 (Foundation: interfaces, DTOs, orchestrator skeleton). The DTO shape is the architectural enforcement. Once the type is wrong, every downstream phase is contaminated.

---

### Pitfall 2: Dead upstream that "looks UP" because the health check tests the wrong layer

**What goes wrong:**
The aniwatch container's `/health` endpoint returned 200 (Node.js was alive). The scraper inside it returned 500 on every actual request because `hianime.to` had been dead for 9 days. Grafana showed the service green. Users got "no episodes" for every anime. We didn't know until a user complained.

**Why it happens:**
Default health checks test "is the process running" (TCP listen, HTTP 200 on `/health`). They do NOT test "does the dependency this process exists to wrap still work." For a scraping service, the dependency *is the upstream HTML page* — and that's the only thing the user actually cares about.

We have shipped this bug at least twice:
- **ISS-007** (2026-03-22): HiAnime player DOWN for 9 days, Grafana green the whole time because aniwatch `/health` only tested Node liveness.
- **ISS-009** (2026-03-23): HiAnime Go client used dead `hianime.to` for `Search`/`GetEpisodes`/`GetServers`, but the health checker tested only `GetStream` via the aniwatch sidecar — separate code path, separate blind spot.

The current `health_checker.go` was already upgraded to test the full chain (search → episodes → streams) after ISS-009. v3.0 must inherit and extend that pattern, not regress to liveness checks.

**How to avoid:**
- Per-provider health checks must run the full user-facing pipeline against a **stable, well-known anime** every N minutes: search for "Naruto" or "One Piece" → fetch episode list → fetch stream → assert the stream URL passes an `HEAD` request and returns `Content-Type: application/vnd.apple.mpegurl` or `video/*`.
- Use a **golden anime** that is not the user's reported anime — "verify with the user's actual broken anime" is for incident response (memory `feedback_verify_streams.md`), but for synthetic health monitoring you need a fixed test case so the metric is comparable over time.
- Expose `provider_health_up{provider, stage}` where `stage ∈ {search, episodes, servers, stream, stream_segment}` — five gauges, not one — so we can see *which layer* died.
- Alert on `provider_health_up{stage="stream_segment"} == 0 for 15m`, not on container liveness.

**Warning signs:**
- A provider's container has been UP for > 7 days but its `parser_requests_total{provider=X, status="success"}` counter has been flat-zero for > 24 hours.
- The "aniwatch was running successfully for 7 weeks with zero successful searches" pattern from the v3.0 driver brief — synthesize this as a Grafana alert: `rate(parser_requests_total{status="success"}[1d]) == 0 AND up == 1`.

**Phase to address:**
Phase 2 (Per-provider health + observability). Must ship in the same phase that ships the first provider, otherwise we accumulate undetectable rot.

---

### Pitfall 3: Caching encrypted/short-lived stream URLs longer than their actual expiry

**What goes wrong:**
The orchestrator caches the m3u8 URL for 1 hour. The upstream signs it with `?token=...&expires=1715459200` and the token dies after 60 seconds. For 59 of every 60 minutes, every user gets an HTTP 403 from the CDN. HLS.js retries, fails, retries — the player enters the infinite reload loop we already documented in ISS-001.

**Why it happens:**
"Cache for an hour" is a sensible default for *most* HTTP responses. It is wrong for *signed CDN URLs*. Each provider signs its URLs with different TTLs:

| Upstream | Observed expiry window | Source |
|---|---|---|
| `owocdn.top` / `uwucdn.top` (Consumet → Kwik) | "short-lived, expire quickly" (ISS-001) | our own incident |
| Megacloud (`.blog`) `getSources` payload | embedded `?_k=` ephemeral client key, valid for the lifetime of the embed page (~minutes) | `docker/megacloud-extractor/server.js` |
| AnimePahe / Kwik | typically 15-30 min based on community reports | LOW confidence, verify per provider |
| AnimeKai | unknown, must measure | unverified |

There is no way to know an URL's expiry without parsing it. Generic 1-hour cache → broken playback. No cache → upstream rate-limit storm (Pitfall 6).

**How to avoid:**
- Cache the **resolution work** (the search hit, the episode list, the server list), not the final stream URL. Stream URL must be resolved on every `GET /api/scrape/.../stream` request unless an `expires_at` is explicitly parsed from the URL.
- For each provider, write a small `parseExpiry(url string) time.Time` helper that extracts the signed expiry from the URL/JWT and caches with `min(parsedExpiry - 30s, 5min)`.
- If no expiry can be parsed, default to **30 second** cache (de-dup burst, not request-coalesce hours) — far below the most aggressive observed window.
- Use Redis SETNX with a small TTL as a single-flight guard so 5 concurrent users hitting the same episode → one upstream fetch + 4 cache reads.

**Warning signs:**
- Spike in `proxy_upstream_errors_total{status="403"}` (the metric added in ISS-001's fix).
- Users reporting "the player worked an hour ago" — classic stale-cached-URL signature.
- The same m3u8 URL appearing in the `video_sources` cache for > 5 minutes.

**Phase to address:**
Phase 3 (Stream extraction + caching). Document the per-provider expiry table in the provider's parser file at the top.

---

### Pitfall 4: Brittle decryption that patches *into a third-party package's internals*

**What goes wrong:**
We currently patch `aniwatch/dist/index.js`'s `extract5` method via Node string-substitution on container start (`docker/megacloud-extractor/patch-aniwatch.sh`). This works today. It will break the next time aniwatch ships a release that renames `extract5`, restructures braces, or moves the embed extraction code into a different file. We cannot pin to a known-good version because the upstream repo has been deleted.

The patch script literally counts braces to find a function body. That is the kind of fragility you ship at 2 AM, not the kind you build a milestone around.

**Why it happens:**
"Reuse the upstream extractor" feels efficient when the upstream still exists. The cost is hidden — until the upstream dies, then you find out your "small patch" is your only access to the decryption.

**How to avoid:**
- For v3.0, **port the extraction logic into our own Go (or our own tightly-scoped Node)**. The megacloud-extractor *service* (~250 LOC `server.js`) is the right level of containment: it is a service we own, with one entry point we control, and we can replace it line-by-line without restoring a deleted repo.
- The aniwatch `extract5`-patch path must die: when we delete `services/catalog/internal/parser/hianime/`, we also delete `docker/megacloud-extractor/patch-aniwatch.sh` and remove the `aniwatch` container from `docker/docker-compose.yml`.
- For each provider's decryption (AnimeKai's embed, AnimePahe's Kwik decryption, etc.), put the logic in a single Go file with golden-file tests (Pitfall 5). When the upstream changes encoding, exactly one file changes.
- **Never** modify a vendored npm package on container boot. If we need to extend a Node lib, fork it and pin to our fork, or rewrite the function we need.

**Warning signs:**
- Any `.sh` script under `docker/` that calls `node -e "..."` to mutate `node_modules/`.
- A unit test that imports an internal symbol of a third-party package.
- A README sentence containing "monkey-patch."

**Phase to address:**
Phase 1 (Foundation) — the decision to own decryption logic must be in the v3.0 architecture, not retrofitted. Phase 4 (per-provider work) implements per-provider extractors against this contract.

---

### Pitfall 5: HTML scraping brittleness with no golden-file safety net

**What goes wrong:**
A provider tweaks a CSS class from `.episode-link` to `.ep__link_v2`, or moves the embed `<iframe>` from `[data-id]` to `[data-src]`. All parser calls silently return empty arrays — the parser returns `[]Episode{}` and `nil` error. The orchestrator interprets "no episodes" as "no source available" → user sees empty state with no clue why. This is exactly the ISS-009 failure mode again, one layer down.

**Why it happens:**
- Scraping code calls `.Find(".some-class").Each(...)`; on selector miss, `Each` simply runs zero times. No error. No metric.
- Mocked-HTML tests use the markup *the test author saw* on the day they wrote the test. The upstream changes; the mock does not.
- "Just run the real scrape in CI" doesn't work for piracy mirrors: they rate-limit, region-block, and disappear.

**How to avoid:**
- **Golden-file pattern (mandatory for v3.0):** every provider gets `testdata/{provider}/search_naruto.html`, `episodes_naruto.html`, `embed_naruto_ep1.html` captured from a real fetch. Parser tests run against these files offline.
- Refresh discipline: a CI cron (weekly) runs a `refresh-goldens` make target that fetches fresh HTML and writes to `testdata/` with a date suffix. If the new file differs structurally from the old, CI fails and a human reviews. **Do not auto-merge golden refreshes** — that defeats the test.
- **Sentinel assertions in parsers:** if `Search()` finds zero elements matching the selector, it must return `errors.New("scraper: selector matched 0 elements; upstream may have changed markup")`, not `[]SearchResult{}, nil`. Empty result is a valid state (no anime matches the query); selector miss is a defect that must surface as an error.
- Track `parser_zero_match_total{provider, selector}` so we see selector-miss rate climbing before it hits 100%.

**Warning signs:**
- Two consecutive weeks of "no results for any query" → selectors moved.
- `parser_zero_match_total` rising while `parser_requests_total{status="success"}` stays flat.
- A maintainer updating the parser without running `make refresh-goldens` against a real upstream.

**Phase to address:**
Phase 1 (golden-file harness + sentinel error contract) — must exist before the first scraper is written, or the first scraper will be written without it.

---

### Pitfall 6: Parallel fan-out to N providers multiplying load → tripping per-host bans

**What goes wrong:**
The orchestrator implements "try AnimeKai, AnimePahe, Anitaku in parallel, first-success wins" because parallel feels fast. Three providers × ~10 concurrent users browsing search results × 3 mirrors per provider = 90 concurrent connections to piracy sites that rate-limit at 1-3 RPS. Within 5 minutes the server IP is on a soft ban list from one or more providers; within an hour, two of three providers return 403/Cloudflare challenge to every request from our IP.

The "fastest" architecture creates the longest outage.

**Why it happens:**
- Go's `goroutine + sync.WaitGroup` makes parallel-fan-out feel free.
- The previous `consumet` client (`services/catalog/internal/parser/consumet/client.go:24`) defines `FallbackProviders = []string{"animekai", "hianime", "animepahe"}` but executes sequentially. The temptation in v3.0 is "let's parallelize this." Don't.
- ISS-005 documented the inverse problem: *sequential* hits on Jikan + 3 HiAnime variants compounded to 11s+ latency, fixed by parallelization. The lesson is "parallelize independent inputs (different name variants for *one* provider)" not "parallelize across providers."

**How to avoid:**
- **Sequential-with-fallback by default.** Try provider 1 with a 5s timeout. If it returns sources, stop. If it errors or times out, try provider 2. This matches our 10-user load profile and respects rate limits.
- Per-host semaphore: `golang.org/x/sync/semaphore` with weight 2 for each upstream host. No more than 2 in-flight requests per provider hostname *globally* across the service.
- Per-host rate limiter: `golang.org/x/time/rate.NewLimiter(rate.Every(500*time.Millisecond), 1)` per host → ≤2 RPS sustained.
- Exponential backoff on 429/403 with jitter: 1s, 2s, 4s, 8s, give up. After give-up, mark the provider DOWN in a circuit-breaker for 5 minutes; the orchestrator skips it.
- **Do not** add proxy rotation, residential IPs, or per-request IP-cycling. That is Pitfall 11 territory and is out of scope for a 10-user self-hosted service.

**Concrete numbers (starting points, must be refined per provider in Phase 2):**

| Provider | Max RPS | Concurrency cap | Backoff on 429 | Circuit-break duration |
|---|---|---|---|---|
| AnimeKai | 1 | 2 | 1s, 2s, 4s | 5 min |
| AnimePahe | 2 | 2 | 1s, 2s, 4s | 5 min |
| Anitaku / Gogoanime | 1 | 2 | 1s, 2s, 4s | 5 min |

**Warning signs:**
- `parser_requests_total{provider=X, status="error"}` jumps to >50% within minutes of a deploy → we tripped a per-host ban.
- HTTP 403 with `cf-ray` headers in the response → Cloudflare flagged us.
- A provider's success rate is fine in isolation but degrades when multiple are queried simultaneously → we are running them in parallel; revert to sequential.

**Phase to address:**
Phase 1 (orchestrator shape: sequential, per-host limiter primitives) — wrong here propagates everywhere.

---

### Pitfall 7: Subtitle/episode-number misalignment between provider and Jimaku

**What goes wrong:**
The user is watching episode 12 of "Mushoku Tensei". The English provider has 23 episodes (the broadcast order with two recaps included). Jimaku.cc has 21 entries because they used the BD order (recaps omitted). The Japanese subtitle for "episode 12" arrives ~2 episodes behind plot. Users notice within 30 seconds and lose trust in the JP subtitle feature.

**Why it happens:**
- "Episode number" is not a stable identifier. Different mirrors use:
  - Broadcast order (includes recaps, specials inline)
  - BD/home video order (recaps may be separate)
  - Streaming-platform order (Crunchyroll-style numbering)
  - Absolute count across cours (e.g., "S2E25" rendered as "E37")
- Our existing `subtitle-parser.ts` + ARM AniList ID resolution (Phase 2 of JP subs, complete) assumes "episode N from the player" = "episode N from Jimaku." That holds for some series and breaks silently for others.
- Recap detection is also unreliable: a provider's `isFiller: true` flag is set inconsistently (HiAnime had it, AnimePahe may not).

**How to avoid:**
- **Surface the misalignment** rather than hide it: when a user opens episode N and the JP subtitle file for episode N has a duration that differs from the video duration by > 90 seconds (or > 20% of the shorter), log a `subtitle_drift_warning` event with `provider, anime_id, episode, video_duration, subtitle_duration` and show a small UI notice "Subtitles may be misaligned for this episode."
- Map provider episode → canonical AniList episode number where the metadata allows (ARM gives us the AniList ID; AniList's `streamingEpisodes` field is sometimes annotated with relative numbers). LOW confidence on AniList being authoritative for fillers/recaps; verify in Phase 4 per anime.
- For known-problem series (Mushoku Tensei, JoJo, Naruto Shippuden's filler-heavy stretches), allow a manual override table — admin-managed `episode_offset_map(provider, anime_id, episode_no) → jimaku_episode_no`.

**Warning signs:**
- User reports "subtitles are out of sync" but the audio sync is fine → wrong file, not bad timing.
- The `subtitle_drift_warning` event count > 5% of "subtitle loaded" events.

**Phase to address:**
Phase 4 (provider implementations) or a dedicated Phase 5 (subtitle alignment hardening) if Phase 4 reveals broad inconsistency.

---

### Pitfall 8: "Page-not-found" served as HTTP 200 OK

**What goes wrong:**
The provider's CDN swaps `anime/{slug}` for an "anime not found" template page at HTTP 200. The Go HTML parser happily runs every selector against this page, finds zero matches, and returns `[]Episode{}, nil`. From the orchestrator's perspective: "valid response with no episodes" — indistinguishable from a real anime with no episodes (a movie pre-airing, for instance).

**Why it happens:**
- Piracy mirrors deliberately serve soft-404s (200 OK with a "back to home" page) to keep bot scrapers from short-circuiting on status code.
- ISS-001 showed the same pattern at the CDN layer: Cloudflare 403 challenge page returned with `Content-Type: application/vnd.apple.mpegurl`, causing HLS.js to parse HTML as a manifest.

**How to avoid:**
- Every provider parser includes a `verifyPageShape(html string) error` helper that checks for at least one **structural sentinel** specific to a real anime page (e.g., `<meta property="og:type" content="video">` or `<h1 class="anime-title">`). Returns `errors.New("scraper: anime page sentinel missing; likely a 404 template")` if absent.
- The HLS proxy (`libs/videoutils/proxy.go`) already detects upstream 4xx/5xx (per ISS-001 fix). Extend the same idea to *body inspection*: if the response body starts with `<!DOCTYPE html` and `Content-Type` claims to be HLS, treat it as a 502 from us.
- Sentinel check is part of the parser's success criteria, not optional.

**Warning signs:**
- `parser_zero_match_total{provider}` rising sharply for a single provider.
- HLS proxy returns 502 with `domain=X` and `upstream_status=200` → 200-but-HTML pattern.

**Phase to address:**
Phase 1 (parser contract: sentinel check is required) + Phase 3 (extend HLS proxy body inspection).

---

### Pitfall 9: Cutover bugs from deleting dead infrastructure before the replacement is verified

**What goes wrong:**
We are excited about the new scraper, so we:
1. Delete `services/catalog/internal/parser/hianime/`
2. Remove `aniwatch` and `consumet-api` containers from `docker/docker-compose.yml`
3. Update `HiAnimePlayer.vue` to point at the new endpoints
4. Push to main → production breaks for the 10-30% of anime where the new provider doesn't yet have coverage but the old aniwatch container *was* still serving a useful response despite the GitHub repo being deleted.

Or, the inverse: we leave the dead `aniwatch` container running because "it's there, we'll get to it later." It quietly logs errors at 1 RPS forever, polluting `parser_requests_total` and obscuring the new providers' actual success rates.

**Why it happens:**
- This server **is** production (`project_deployment.md`). There is no staging environment. "Deploy and see" lands on real users.
- The frontend has two players that consume the existing HLS-source DTO. Both must be wired before either old endpoint is removed.

**How to avoid:**
- **Parallel-running, dark-cutover:** ship the new scraper alongside the old paths. New endpoint is `/api/scrape/en/...`; old endpoint is `/api/anime/{id}/hianime/...`. Frontend continues to call the old. Backend logs both old and new responses for the same query and emits `cutover_diff_total{provider, agree, disagree}`.
- After two weeks of `agree >> disagree` for the anime our users actually open, flip one player (HiAnime first because it is more deeply broken) to the new endpoint and watch error rate for 48 hours.
- Only after both players are on the new endpoint *and* error rate is flat for a week do we delete the old parsers, containers, and patch script.
- Use a feature flag (env var or DB row) to switch providers per-user, starting with `ui_audit_bot` and the admin user, then 25% rollout, then full.

**Warning signs:**
- A PR titled "remove dead aniwatch container" before the new endpoint has live traffic.
- The new scraper has been in production for < 7 days when someone proposes deleting the old code.
- "I'll just disable it" — see Pitfall 1's ISS-008. Disabling without an alternative IS the bug.

**Phase to address:**
Phase 5 or 6 (cutover) — explicit phase, must not be folded into a parser-implementation phase.

---

### Pitfall 10: Over-engineering for coverage we don't need

**What goes wrong:**
We try to support 100% of mainstream + niche + sub-only + dub-only anime by orchestrating across 4 providers in parallel, building a candidate-merging algorithm to pick the best stream, and adding a translation-quality-scoring heuristic to rank dubs. This takes 4 weeks. The result handles edge cases that affect 2 users 1% of the time. The orchestration code is 800 lines and the most common bug is "wrong provider picked for a popular anime."

**Why it happens:**
- We see all the edge cases (a fan asking for a specific 1998 OVA, a user with EN-dub preferences for a series only subbed) during planning. Each one feels solvable. Solved together, the complexity compounds.
- 10 active users + on-demand catalog → the realistic working set is "the 200 anime currently airing + the 50 most-watched classics." Provider X is enough for that.

**How to avoid:**
- **Phase 4 target: one alive provider, end-to-end, for the top 100 anime by current watch_history.** Ship it. Measure how many users it covers.
- If user reports say "still missing X" after 2 weeks of live data, add provider 2.
- If user reports persist, add provider 3. Do not add provider 3 preemptively.
- The `Provider` interface (REQ from Phase 1) makes adding provider N+1 cheap; that is the architectural payoff that justifies *not* implementing all of them now.
- Sequential fallback (Pitfall 6) over parallel fan-out is the architectural expression of this principle.

**Warning signs:**
- A phase plan that introduces 3+ providers in one phase.
- A roadmap with a "provider quality scoring" task before the first provider has shipped.
- A diff that adds a "smart provider selection" service before there is a per-provider error-rate metric to base "smart" on.

**Phase to address:**
Roadmap structure decision (before Phase 1 even starts). State it as a non-goal in PROJECT.md if it isn't already.

---

### Pitfall 11: Anti-bot detection scope creep (Cloudflare, JS challenge, residential proxies)

**What goes wrong:**
AnimeKai serves a JS challenge page. The team adds Puppeteer to solve it. Now there is a headless Chrome process consuming 500MB. AnimePahe rate-limits more aggressively. The team adds a residential proxy provider for $50/month. The provider's challenge page now requires fingerprint randomization. The team adds curl_cffi (or in Go, a TLS-fingerprint-spoofing library). The ops surface area doubles for every step, and our 10-user, self-hosted, no-CDN posture goes from "small Go service" to "complex anti-bot pipeline that needs maintenance."

**Why it happens:**
- "We hit a wall, we add the next tool" is locally rational and globally a disaster.
- Each tool has community support that says "this is the standard solution." It is — for a 1000-RPS commercial scraping operation. Not for ~10 users requesting ~100 episodes a day.

**How to avoid:**
- **Hard rules, written into the architecture doc and enforced at PR review:**
  1. We use `net/http` with sensible headers (User-Agent, Accept-Language, Referer matching the provider's own pattern).
  2. We may add **rotating User-Agents** from a small list of real browser strings.
  3. We may add **cookie persistence** (one cookie jar per provider, refreshed on session-expired errors).
  4. We DO NOT add Puppeteer, Playwright, headless Chrome, or any browser automation.
  5. We DO NOT add proxy rotation, residential proxies, or paid IP services.
  6. We DO NOT add TLS fingerprint spoofing (curl_cffi, utls).
- Tripping rules 4-6 means "this provider is too aggressive for our profile; pick a different provider."
- If all alive English providers reach rules-4-6 levels of difficulty simultaneously, this is the moment to revisit project scope, not to escalate tooling.

**Warning signs:**
- A PR that adds a Chromium dependency to a Go service.
- A budget line item for "residential proxies."
- A phase plan with a "stealth scraper" objective.

**Phase to address:**
Phase 1 — written into the orchestrator contract as explicit non-goals.

---

### Pitfall 12: Test data that doesn't reflect what the upstream actually serves

**What goes wrong:**
A developer writes a parser test against a hand-written HTML snippet that "looks like what AnimeKai returns." It works. CI passes. Production fails because real AnimeKai responses include:
- A 700-line `<head>` with Cloudflare's `__cf_bm` cookie-setting script
- Lazy-loaded episode tiles that aren't in the initial HTML (require a follow-up JSON fetch)
- Random class-name suffixes from a build hash (`.episode-card_a8f2`, changes every deploy)
- A `<noscript>` redirect to a captcha page when no JS-runtime cookie is present

None of which were in the hand-written snippet.

**Why it happens:**
- The author hadn't seen a real response or wrote the test from memory of what they thought they saw.
- The test passed; the assumption felt validated.
- This is a special case of Pitfall 5 with a specific failure mode at the test-authoring step.

**How to avoid:**
- **Golden-capture script:** `make capture-goldens` runs a real `curl` (or, when needed, a one-shot Playwright fetch with the cookies our scraper would use) against each provider for the same fixed anime list, and writes the response bodies (raw, untouched, including headers) to `testdata/{provider}/`. Headers are captured separately as `*.headers.txt`. Parser tests load these files.
- Goldens include the cookie-setting `Set-Cookie` headers so we can test our cookie-jar behavior offline.
- Goldens include at least one "anime not found" response per provider (for Pitfall 8 sentinel testing).
- Goldens include at least one "rate-limited" response per provider (HTTP 429 with Cloudflare body) for testing our backoff logic.
- **Anti-pattern:** "I wrote a representative HTML fragment" comments in tests. Either it's a real capture or it's a bug.

**Warning signs:**
- A test fixture file < 5KB for a real provider page (real pages are 100KB+).
- Multiple test fixtures with identical structural skeletons (developer wrote one, copy-pasted).
- A fixture file that has not been refreshed in > 90 days while the provider has shipped UI changes (detectable via mtime + `curl -I` to a real page).

**Phase to address:**
Phase 1 (golden capture script as part of the test harness) + Phase 4 enforcement (each provider PR must include captured goldens).

---

## Technical Debt Patterns

Shortcuts that look reasonable in the moment, with their real long-term cost in this domain.

| Shortcut | Immediate Benefit | Long-term Cost | When Acceptable |
|---|---|---|---|
| Hard-code an upstream domain in a parser (`https://hianime.to`) | One less env var, one less config layer | When the domain dies (it WILL — ISS-007), every parser PR is a domain hunt. Configure via env, with a `*_BASE_URL` per provider. | Never (we have already burned this). |
| Use a single env var for "the provider URL" when the provider has multiple known mirrors | Simple | When mirror A dies, every user hits errors until a redeploy. Mirrors must be a list, tried in order with circuit-breaker. | Phase 1 MVP only if there is exactly one known mirror; convert to list before phase exit. |
| Cache the orchestrator's final response (DTO) instead of intermediate calls | Fewer cache keys | Stream URL inside the DTO expires mid-cache → broken playback (Pitfall 3). Cache intermediate calls only. | Never for the stream URL itself. |
| Patch a third-party npm/Go package on container boot | "Quick fix" without forking | Upstream version pin becomes impossible; future maintainer cannot reproduce build (megacloud-extractor patch is already this). | Only as a documented stopgap with an explicit "remove by phase X" tag. |
| Return `[]T{}, nil` when a selector matches nothing | Empty result feels safe | Indistinguishable from "upstream broke" (Pitfall 5). | Never for required selectors. |
| Reuse aniwatch / consumet container from upstream | "Free" implementation | Both have died unannounced. Reuse only when the upstream has > 6 months of activity AND we have a fallback plan AND we monitor the upstream repo for archival. | Only for narrow, swap-out-able adapters (e.g., the megacloud-extractor microservice pattern), not for the whole scraper. |
| Test against a hand-written HTML snippet | Test loads in 1ms | False confidence; production fails on real response shape (Pitfall 12). | Never. |
| Add a provider in parallel orchestration "for redundancy" | Looks faster, looks more reliable | Triples rate-limit pressure, masks per-provider error rates, complicates debugging (Pitfall 6). | Never without per-host limiters and a measured proof that one provider is insufficient. |

---

## Integration Gotchas

Specific mistakes when wiring to each external service we already touch or will touch in v3.0.

| Integration | Common Mistake | Correct Approach |
|---|---|---|
| Shikimori (already integrated) | Hard-code `shikimori.one` — they migrated to `.io` and serve a 301 redirect that strips the POST body (ISS-010). | Configure via `SHIKIMORI_GRAPHQL_URL` env. Add to startup health-check that the configured URL returns JSON, not HTML, for a known query. |
| Cloudflare-protected mirrors (all of AnimeKai/AnimePahe/Anitaku candidates) | Treat `Content-Type: text/html` on a stream URL as a real response (ISS-001). | Body inspection in HLS proxy: if `body[0:15] == "<!DOCTYPE html"` regardless of Content-Type, return 502 to client. |
| Jimaku.cc (already integrated, JP subs Phase 1-3 complete) | Assume `provider episode N == jimaku episode N` (Pitfall 7). | Surface drift to user UI when video and subtitle durations diverge > 90s. |
| ARM (`arm.haglund.dev`, already integrated) | Forget that Shikimori IDs == MAL IDs, query `?source=shikimori` (it doesn't exist). | `?source=myanimelist&id={shikimori_id}` (per `MEMORY.md`). |
| MegaCloud `getSources` | Cache the `_k` client key (it's per-page-load, dies in minutes). | Re-fetch the embed page on every stream resolution. The `_k` is ephemeral, not the source URL. |
| Per-provider cookie jars | Share one global cookie jar across providers. | One `http.CookieJar` per provider, persisted to Redis with a 24h TTL, refreshed on any 403/429. Cross-provider cookie leakage can de-anonymize our scraper across mirrors. |
| HLS proxy allowed domains (`libs/videoutils/proxy.go:230`) | Add a new provider's CDN to allowed domains as part of a "small fix." | New CDN domains require an explicit PR, code review, and a metric (`hls_proxy_new_domain_seen_total`). The 2026-02-27 incident where `uwucdn.top` was missing (ISS-002) was caused by exactly this. |
| Aniwatch / Consumet legacy containers | Leave them running "just in case" after cutover. | Phase 6 explicit deletion: remove from `docker-compose.yml`, remove parser dir, remove `megacloud-extractor/patch-aniwatch.sh`, remove `aniwatch` health check, redeploy. Verify `docker ps` shows neither container. |

---

## Performance Traps

Patterns that work at small scale but fail as our 10 users grow to 50 or as a few users get aggressive about pre-loading next episodes.

| Trap | Symptoms | Prevention | When It Breaks |
|---|---|---|---|
| One worker pool per provider with unbounded queue | Latency creeps up; eventually provider returns 429 storms | Per-provider bounded semaphore (weight 2) + queue with bounded size (e.g., 10) + reject-fast when full | First time 5+ users open episode pages within a few seconds (browsing the catalog row) |
| Synchronous extractor calls on the search-list request | Search latency spikes to seconds when we resolve N stream URLs eagerly | Resolve stream URLs lazily, only on `/stream` endpoint. Search returns episode lists, not streams. | Day 1, immediately, because search is a hot path |
| Caching the final stream URL globally instead of per-user-or-anonymous | Two users mid-episode → one URL expires → second user blocked by first's expired cache | Single-flight by `(episode_id, provider, server)`; do not key cache by user | Whenever two users watch the same episode within the URL's TTL |
| Letting HLS.js retry indefinitely on 403 | Client floods our proxy → proxy floods upstream → ban (ISS-001) | Proxy returns 502 on upstream non-2xx (already implemented per ISS-001); frontend HLS.js error handler must surface user-friendly error and stop retrying (already pending per ISS-001 remaining work) | Mobile Safari especially (ISS-006), where retry loops are most aggressive |
| Health-check synthetic queries hit the same anime/episode every 60s | Provider sees suspicious uniform traffic from our IP | Rotate health-check anime among a small (5-10) pool; randomize interval ±20%. The synthetic must look like a low-volume browse, not a heartbeat. | First time a provider deploys behavioral fingerprinting (Cloudflare bot management defaults to this) |

---

## Security Mistakes

Beyond OWASP basics — domain-specific issues for piracy-grade scraping.

| Mistake | Risk | Prevention |
|---|---|---|
| Leak our server IP to upstream piracy mirrors that get DMCA'd → IP appears in takedown filings | LOW probability, real reputational/operational risk | Outbound traffic from the scraper service must go through an explicit egress allow-list. Document the IP that touches piracy sites; if it becomes problematic, the egress can be moved (but do not preemptively route through a paid proxy per Pitfall 11). |
| User-supplied search queries pass straight into URL path | URL injection into upstream (HTTP request smuggling), or upstream-side XSS if the response includes the query in HTML and we render it | Always `url.QueryEscape` user input (the existing parsers do — verify in code review). Reject queries containing `\r`, `\n`, raw `&`, `?` in unexpected positions. |
| Log full stream URLs at INFO level | Stream URLs contain signed tokens that grant access; logs persist longer than the token does, but Loki retention (168h per CLAUDE.md) is short enough that this is low-risk; still avoid as a habit | Log the host + episode_id + provider, not the query string. |
| Trust the `Referer` headers we set on upstream requests (or trust the user-supplied Referer in any way) | An attacker shaping the `Referer` via the API could exfiltrate our internal Referer policy or fingerprint our scraper | The scraper service constructs Referer internally per provider. The gateway must strip incoming Referer headers before forwarding to the scraper. |
| Serve the megacloud-extractor `/extract?url=...` endpoint without an SSRF guard | Any internal caller can be coerced into fetching arbitrary URLs through our infra | The extractor currently accepts any URL — restrict to `megacloud.blog` and other known embed hosts. Validate upstream host against an allow-list before fetching. |
| Persist user-supplied watch URLs anywhere they can be retrieved unauthenticated | Same SSRF class | The scraper does not accept user-supplied URLs end-to-end. User input is anime_id + episode_no; URLs are constructed internally. |

---

## UX Pitfalls

Domain-specific UX mistakes when stream sources are unreliable.

| Pitfall | User Impact | Better Approach |
|---|---|---|
| Showing "no source available" without explaining which providers were tried | User thinks the system is broken; will report it repeatedly | "We checked AnimeKai, AnimePahe, Anitaku — no English source for this anime. Try the Russian tab (Kodik) which has wider catalog coverage." Be specific. |
| Auto-falling-back from English tab to Russian tab on no-source (the Kodik trap, Pitfall 1) | User watching the wrong language without realizing | Never. Empty state with a CTA to the Russian tab — the user chooses to switch. |
| Hiding the source's name from the user | When playback fails, the user can't tell us *which* source failed | Each player shows the source name and a "report this stream" button (already exists per `MEMORY.md`'s ReportButton notes). v3.0 must preserve this. |
| Switching providers mid-episode without warning | Time-position is lost; subtitle alignment may break (Pitfall 7) | If a provider fails mid-stream, surface "this server died; click to try another server." Do not auto-switch. |
| Stale Redis `search:*` cache after a fix (ISS-010 fallout) | Users keep seeing the broken state for the cache TTL even after deploy | Cache bust on each scraper deploy. `make redeploy-catalog` (and the new scraper service) must include a Redis SCAN-and-DEL for `search:*`, `episodes:*`, `stream:*` keys, OR cache keys must include a deploy SHA prefix. |

---

## "Looks Done But Isn't" Checklist

Things that appear complete but routinely ship with critical pieces missing.

- [ ] **Provider parser:** Has integration tests against captured `testdata/{provider}/*.html` goldens — verify by running `make test-parsers-offline`. Mocked-HTML strings inside `_test.go` files do not count.
- [ ] **Provider parser:** Has a `verifyPageShape()` sentinel that returns an error on missing `<meta og:type>` or equivalent — verify by feeding a captured 404-template golden and asserting the error fires.
- [ ] **Provider parser:** Returns explicit error (not `nil, []`) when required selectors match zero — verify by running the parser against `testdata/{provider}/empty_page.html`.
- [ ] **Provider parser:** Reads `*_BASE_URL` env var with a sensible default; does NOT hard-code a domain — verify with `grep -r "https://" services/scraper/internal/parser/{provider}/ | grep -v '_test.go'` should return nothing or only the default.
- [ ] **Health check:** Tests the full pipeline (search → episodes → servers → stream → HEAD of stream URL), not just `GET /health` — verify by reading the health checker code.
- [ ] **Health check:** Emits `provider_health_up{provider, stage}` per stage — verify by `curl /metrics` and seeing all 5 stages.
- [ ] **Orchestrator:** Returns 404 (not Kodik iframe URL, not empty `sources[]` with `iframe_url` set) when no English source exists — verify with an integration test using an anime known to be Kodik-only.
- [ ] **HLS proxy:** Inspects body for `<!DOCTYPE html` regardless of Content-Type — verify by mocking an upstream that returns HTML with `Content-Type: application/vnd.apple.mpegurl`.
- [ ] **HLS proxy:** Has the new provider's CDN domains in `HLSProxyAllowedDomains` — verify by hitting the stream URL through the proxy and watching logs for "domain not allowed."
- [ ] **Stream cache:** TTL is per-provider, based on parsed expiry, capped at 5 min, never indefinite — verify via Redis `TTL stream:*` and observe values < 300s.
- [ ] **Cutover:** Old `aniwatch` + `consumet-api` containers deleted from `docker/docker-compose.yml` — verify with `grep -E "aniwatch|consumet-api" docker/docker-compose.yml` returning nothing.
- [ ] **Cutover:** Old parser dirs deleted — verify with `ls services/catalog/internal/parser/hianime services/catalog/internal/parser/consumet 2>/dev/null` returning nothing.
- [ ] **Cutover:** `megacloud-extractor/patch-aniwatch.sh` deleted — verify with `test ! -f docker/megacloud-extractor/patch-aniwatch.sh`.
- [ ] **Frontend cutover:** Both `HiAnimePlayer.vue` and `ConsumetPlayer.vue` either point at new endpoints or are merged into a single `EnglishPlayer.vue` — verify by grepping for the old endpoint paths in `frontend/web/src/`.
- [ ] **Redis cache bust:** Deploy procedure includes `redis-cli SCAN ... DEL` for `search:*`, `stream:*`, `episodes:*` on cutover — verify in the deploy runbook.

---

## Recovery Strategies

When pitfalls happen anyway, how to recover with bounded cost.

| Pitfall | Recovery Cost | Recovery Steps |
|---|---|---|
| Pitfall 1 (silent fallback) | LOW | Diff frontend → revert the fallback path or harden the DTO type to reject `iframe_url`. Same playbook as commit `9347143`. Re-deploy via `make redeploy-catalog` (or new scraper service). |
| Pitfall 2 (dead upstream looks UP) | LOW | Switch the affected provider's circuit-breaker to OPEN manually via admin endpoint. Add the missing stage to the health check. |
| Pitfall 3 (cache expired URLs) | LOW | Redis: `SCAN 0 MATCH stream:* COUNT 1000 | xargs DEL`. Reduce TTL config. Roll forward. |
| Pitfall 4 (decryption brittle) | MEDIUM | Port the broken extractor function to Go in the megacloud-extractor microservice; do not patch back into a third-party package. |
| Pitfall 5 (HTML selectors moved) | LOW-MEDIUM | Re-capture goldens via `make capture-goldens {provider}`. Diff against committed goldens. Update selectors. Add sentinel for the new structure. |
| Pitfall 6 (per-host ban from parallel fan-out) | MEDIUM | Stop sending traffic from our IP for 30 min; revert to sequential orchestration; lower per-host RPS cap. May require manual unban request from provider (ToS-fragile). |
| Pitfall 7 (subtitle drift) | LOW per anime, MEDIUM systemic | Manual `episode_offset_map` row for the affected series; surface drift warning in UI until mapped. |
| Pitfall 8 (200 OK soft-404) | LOW | Add the sentinel selector for the soft-404 template; emit error. |
| Pitfall 9 (cutover blew up) | HIGH | Revert the frontend to the old endpoint via env flag (must exist by design); re-enable old container; investigate; re-cutover with feature flag at 10% next time. |
| Pitfall 10 (over-engineered orchestration) | HIGH | Delete the smart-selection code; replace with sequential; rebuild metrics; lose 2 weeks. Prevention is cheaper than recovery here. |
| Pitfall 11 (anti-bot scope creep) | VERY HIGH | The ops surface area cannot be un-added easily once a Chromium dep ships. Treat any PR that adds it as a STOP point — re-evaluate provider choice instead. |
| Pitfall 12 (fake test data) | LOW per parser, MEDIUM systemic | Re-capture goldens, re-run tests, fix the parsers that now fail. |

---

## Pitfall-to-Phase Mapping

The roadmapper should use this mapping to allocate prevention to specific phases. Pitfalls without a clear phase home become guardrails enforced at PR-review.

| Pitfall | Prevention Phase | Verification (in that phase's exit criteria) |
|---|---|---|
| 1. Silent provider fallback / Kodik trap | Phase 1 (Foundation: DTOs + orchestrator interface) | Integration test: ask English scraper for a Kodik-only anime → expect HTTP 404, no `iframe_url` field on the response struct |
| 2. Dead upstream looks UP | Phase 2 (Per-provider health + observability) | `provider_health_up{provider, stage}` gauges exist for all 5 stages; alert rule fires within 15 min of synthetic failure |
| 3. Stream URL freshness / cache TTL | Phase 3 (Stream extraction + caching) | Per-provider expiry parser exists; `stream:*` keys in Redis observed with TTL ≤ 300s |
| 4. Decryption fragility / npm patching | Phase 1 (Foundation: decision documented) + Phase 4 (per-provider decryptors in Go) | `docker/megacloud-extractor/patch-aniwatch.sh` is deleted by end of Phase 6; no `node -e` patch scripts in docker/ |
| 5. HTML scraping brittleness | Phase 1 (golden-file harness + sentinel contract) | `make test-parsers-offline` exists and runs against `testdata/`; CI runs it |
| 6. Concurrency / per-host bans | Phase 1 (orchestrator: sequential + per-host semaphore primitives) | Code review: no `goroutine` fan-out across providers; per-host `rate.Limiter` configured |
| 7. Subtitle / episode-number misalignment | Phase 4 (per-provider impl) or Phase 5 (alignment hardening if Phase 4 reveals broad mismatch) | `subtitle_drift_warning` event emitted when video and subtitle durations diverge > 90s |
| 8. 200 OK soft-404 | Phase 1 (parser contract: sentinel required) + Phase 3 (HLS proxy body inspection) | Golden file `testdata/{provider}/not_found.html` exists; parser returns error on it |
| 9. Cutover bugs | Phase 5 or 6 (dedicated cutover phase with feature flag) | Old containers deleted via PR-checklist; cutover diff metric `cutover_diff_total` was emitted for ≥ 14 days before deletion |
| 10. Over-engineering for coverage | Roadmap structure (pre-Phase 1 decision) | Phase 4 ships exactly one provider; provider 2 only added after a documented user-coverage gap is observed |
| 11. Anti-bot scope creep | Phase 1 (non-goals written into architecture doc) | Code review enforces no Chromium / no Puppeteer / no residential proxy deps; CI lint check on `go.mod` |
| 12. Test data realism | Phase 1 (capture-goldens script) + Phase 4 enforcement (per-provider PR checklist) | Each provider PR must commit `testdata/{provider}/*.html` files ≥ 50KB each; PR template includes "captured fresh via `make capture-goldens`" checkbox |

---

## Sources

Grounded in our own production incidents and code, with confidence levels:

- **HIGH confidence** — derived from documented AnimeEnigma incidents:
  - `docs/issues/README.md` ISS-001 (Cloudflare 403 served as HLS), ISS-002 (missing CDN domain in proxy allow-list), ISS-005 (sequential search latency, fixed via parallelization within one provider), ISS-006 (mobile Safari HLS buffer errors), ISS-007 (HiAnime domain migration silent failure), ISS-008 (AnimeLib Kodik-fallback silent UX swap), ISS-009 (Go client and health check used different paths), ISS-010 (Shikimori `.one` → `.io` migration with HTML response).
  - User feedback memories: `feedback_animelib_no_kodik_fallback.md` (2026-05-09, explicit "AniLib must mean AniLib"); `feedback_verify_streams.md` (2026-04, "verify with the user's actual broken anime").
  - Project memory: `project_deployment.md` ("this server IS production").
  - Existing code: `services/catalog/internal/parser/hianime/client.go` (retry + server-fallback shape), `services/catalog/internal/parser/consumet/client.go` (`FallbackProviders` sequential pattern), `services/catalog/internal/parser/kodik/client.go` (token-refresh pattern), `docker/megacloud-extractor/server.js` (microservice containment shape we want to preserve), `docker/megacloud-extractor/patch-aniwatch.sh` (anti-pattern we want to delete), `libs/videoutils/proxy.go` (HLS proxy + allowed-domain list), `libs/metrics/parser.go` (existing parser metrics — note `parser_fallback_total` exists, no per-stage-health gauge does).
  - v3.0 driver triage 2026-05-09: documented in `.planning/STATE.md` (`hianime.to`/`hianime.nz`/`aniwatch-api`/`aniwatchtv.to` all dead; `riimuru/consumet-api` 5-months-stale calling `enc-dec.app` with wrong shape; verified alive: AnimeKai, AnimePahe, Anitaku, AniZone).
  - v2.0 milestone audit `.planning/milestones/v2.0-MILESTONE-AUDIT.md` (recent example of phase structure, exit criteria, tech-debt logging — Pitfall 9 cutover strategy mirrors v2.0's per-phase production verification).

- **MEDIUM confidence** — informed by piracy-mirror ecosystem patterns observed across community discussions:
  - HiAnime domain lineage (`zoro.to → aniwatch.to → hianime.to → hianime.nz → dead`) — recorded in ISS-007.
  - General observations about Kwik/MegaCloud short-lived signed URLs (15-30 min typical) — needs per-provider verification in Phase 3.
  - Cloudflare bot management default behaviors (TLS fingerprinting, JS challenge) — Pitfall 11's hard rules are derived from "what we will not do regardless of how prevalent these defenses become."

- **LOW confidence** — needs verification in Phase 4 against the actually-chosen providers:
  - Concrete RPS limits per provider (the Pitfall 6 table is a starting point; refine after observation in Phase 2).
  - AniList `streamingEpisodes` field as a canonical episode mapping (Pitfall 7) — has not been verified in our codebase.
  - Whether AniZone, Anitaku, or AnimeKai use any of the megacloud/Kwik decryption schemes our existing extractor knows about — Phase 4 will determine.

---

*Pitfalls research for: v3.0 Universal Anime Scraper — anime piracy mirror scraping in a small self-hosted Go monorepo.*
*Researched: 2026-05-11*
