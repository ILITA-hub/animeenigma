---
phase: 27-animepahe-revival-via-stealth-chromium-sidecar
verified: 2026-05-19T11:40:18Z
status: passed
score: 5/5
overrides_applied: 0
---

# Phase 27: AnimePahe Revival via Stealth-Chromium Sidecar — Verification Report

**Phase Goal:** A request to `GET /api/anime/{uuid}/scraper/episodes?prefer=animepahe` (gateway → catalog → scraper → animepahe parser → new `animepahe-resolver` sidecar) returns ≥ 28 episodes for Frieren (MAL 52991) with playable Kwik stream URLs end-to-end. A new Node 20 + Fastify + puppeteer-extra stealth-Chromium sidecar at `services/animepahe-resolver/` DDoS-Guard-solves on `animepahe.pw` and proxies search/release/play fetches through an internal `:3000` HTTP API. The Go parser at `services/scraper/internal/providers/animepahe/` is rewritten to call the sidecar (replacing direct upstream fetches + the deleted `ddosguard.go`) and migrates from the stale numeric-MAL-id API contract to the new UUID-session-token contract returned by `m=search`. The sidecar stays under a 500 MB hard memory cap (D5 ship gate). Once verified, `animepahe` comes off the `SCRAPER_DEGRADED_PROVIDERS` compose default, restoring orchestrator failover.

**Verified:** 2026-05-19T11:40:18Z
**Status:** PASSED
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | Sidecar `services/animepahe-resolver/` exists with exact-pinned stealth stack, `/healthz` two-layer probe, four Prometheus counters, and passes 500 MB D5 hard gate at 236 MiB peak over 100-request soak | VERIFIED | All 13 sidecar files present; `package.json` has `"puppeteer-extra": "3.3.6"` + `"puppeteer-extra-plugin-stealth": "2.11.2"`; Dockerfile uses `npm ci --omit=dev` from `ghcr.io/puppeteer/puppeteer:24`; 21/21 offline tests pass via `node --test`; `STEALTH-PINS.md` contains `D5 100-request soak` section documenting 236 MiB peak; Pattern 7 branch in `.claude/maintenance-prompt.md`; `redeploy-animepahe-resolver` in Makefile |
| 2 | Go parser routes all transport through `resolverClient`; `ddosguard.go` deleted; UUID-session contract active; `ANIMEPAHE_BASE_URL` replaced by `SCRAPER_ANIMEPAHE_RESOLVER_URL`; Go tests pass | VERIFIED | `resolver.go` exists with `kwikReferer` constant; `ddosguard.go` + `ddosguard_test.go` absent; `p.baseURL` not found in `client.go`; `getWithDDoSGuard` not found; `ANIMEPAHE_BASE_URL` absent from `config.go`; `SCRAPER_ANIMEPAHE_RESOLVER_URL` present; `go build ./...` passes; `go test ./internal/providers/animepahe/...` passes; session-shape validation (`sessionPattern`, `isSessionShape`) and kwik dual-packer fix (`extractAllPackers`) from Plan 27-04 Rule 1 fixes present |
| 3 | `docker/docker-compose.yml` has `animepahe-resolver` service with `mem_limit: 500m`, `seccomp:unconfined`, healthcheck on `/healthz`; scraper has `depends_on: animepahe-resolver: service_healthy`; `ANIMEPAHE_BASE_URL` removed; `SCRAPER_ANIMEPAHE_RESOLVER_URL` added | VERIFIED | `animepahe-resolver` service block present in compose; `mem_limit: 500m` confirmed; `service_healthy` in depends_on; `ANIMEPAHE_BASE_URL` absent from compose; runtime: `docker inspect animeenigma-animepahe-resolver --format '{{.HostConfig.Memory}}'` returns `524288000`; both containers `Up (healthy)` |
| 4 | `GET /api/anime/{uuid}/scraper/episodes?prefer=animepahe` returns ≥ 28 episodes for Frieren, ≥ 1 kwik server, fetchable HLS m3u8 (HTTP 200/206), all four animepahe health stages `up:true`; verdict log updated `FAIL → PASS` | VERIFIED | Live probe during verification: 28 episodes returned for `f0b40660-6627-4a59-8dcf-7ec8596b3623`; 6 kwik servers returned; `data.stream.sources[0].url = https://vault-08.uwucdn.top/...uwu.m3u8` with `Referer: https://kwik.cx/`; `curl -sI -H "Referer: https://kwik.cx/" <m3u8>` returns HTTP/2 200; `/scraper/health` shows all four animepahe stages `up:true` with recent `last_ok` timestamps; `docs/issues/scraper-provider-verification-2026-05-19.md` contains `Post-ship verification — Phase 27` section and `FAIL → PASS` |
| 5 | `SCRAPER_DEGRADED_PROVIDERS` compose default = `gogoanime` (animepahe removed); scraper boots with animepahe REGISTERED (not SKIPPED); D7 gate (b): ≤ 1 flood line in first 10 minutes; changelog entry present | VERIFIED | `docker/docker-compose.yml` contains `${SCRAPER_DEGRADED_PROVIDERS:-gogoanime}` (no `animepahe` in default); scraper boot log shows `registered provider {"name": "animepahe"}` and `providers: 2`; D7 gate (b) captured in SUMMARY: 0 flood lines; `frontend/web/public/changelog.json` contains `animepahe`; all commits carry co-authors trailer |

**Score:** 5/5 truths verified

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `services/animepahe-resolver/server.js` | Fastify entry with /healthz /search /release /play /metrics | VERIFIED | 220 lines; `fastify.get('/healthz'` present; hardcoded-upstream header comment |
| `services/animepahe-resolver/browser.js` | Singleton Chromium launcher + warm page | VERIFIED | `puppeteer-extra-plugin-stealth` import; Pattern 1 warm browser on default BrowserContext |
| `services/animepahe-resolver/upstream.js` | fetch wrapper + 403-retry + page-recycle | VERIFIED | `hostname` host-allowlist guard; `PAGE_RECYCLE_AT` env; `page.evaluate` calls |
| `services/animepahe-resolver/metrics.js` | prom-client registry with 4 counters | VERIFIED | `stealth_challenge_failures_total`, `stealth_challenge_solves_total`, `page_recycle_total`, `upstream_403_total` all present |
| `services/animepahe-resolver/package.json` | Exact-pinned stealth deps | VERIFIED | `"puppeteer-extra": "3.3.6"`, `"puppeteer-extra-plugin-stealth": "2.11.2"` confirmed |
| `services/animepahe-resolver/package-lock.json` | Committed lockfile | VERIFIED | 2146 lines; used by `npm ci` in Dockerfile |
| `services/animepahe-resolver/Dockerfile` | puppeteer:24 base + npm ci | VERIFIED | `FROM ghcr.io/puppeteer/puppeteer:24`; `npm ci --omit=dev` |
| `services/animepahe-resolver/STEALTH-PINS.md` | Pin doc + refresh procedure + D5 soak | VERIFIED | Contains `puppeteer-extra-plugin-stealth`, `Refresh procedure`, `D5 100-request soak` section |
| `services/animepahe-resolver/server.test.js` | Unit tests | VERIFIED | 15 tests; all pass in node --test |
| `services/animepahe-resolver/upstream.test.js` | Unit tests | VERIFIED | 6 tests; all pass in node --test |
| `services/animepahe-resolver/__fixtures__/intercepted-frieren.json` | Offline test fixtures | VERIFIED | Exists with search/release/play keys |
| `Makefile` | `redeploy-animepahe-resolver` target | VERIFIED | Explicit target + .PHONY entry confirmed |
| `.claude/maintenance-prompt.md` | Pattern 7 animepahe-resolver branch | VERIFIED | Contains `animepahe-resolver` text pointing at STEALTH-PINS.md |
| `services/scraper/internal/providers/animepahe/resolver.go` | resolverClient + kwikReferer constant | VERIFIED | Contains `resolverClient`, `kwikReferer = "https://animepahe.pw/"` |
| `services/scraper/internal/providers/animepahe/client.go` | Rewritten to use resolverClient | VERIFIED | Contains `resolverClient`; no `p.baseURL`; no `getWithDDoSGuard`; session-shape validation present |
| `services/scraper/internal/providers/animepahe/ddosguard.go` | DELETED | VERIFIED | File absent |
| `services/scraper/internal/config/config.go` | `SCRAPER_ANIMEPAHE_RESOLVER_URL` binding | VERIFIED | Contains `AnimepaheResolverURL`; `ANIMEPAHE_BASE_URL` absent |
| `services/scraper/testdata/animepahe/frieren-search.json` | Frieren search golden | VERIFIED | Exists; parses with `data[].session` UUID-shaped |
| `services/scraper/testdata/animepahe/frieren-release.json` | Frieren release golden | VERIFIED | Exists; 5 episode entries with 64-char hex sessions |
| `services/scraper/testdata/animepahe/frieren-play.html` | Frieren play HTML golden | VERIFIED | Exists; contains 5 `data-src` kwik entries |
| `docker/docker-compose.yml` | animepahe-resolver service + scraper wiring | VERIFIED | Service block present; `mem_limit: 500m`; `service_healthy`; `SCRAPER_ANIMEPAHE_RESOLVER_URL`; no `ANIMEPAHE_BASE_URL`; compose default = `gogoanime` only |
| `docs/issues/scraper-provider-verification-2026-05-19.md` | Post-ship Phase 27 section | VERIFIED | Contains `Post-ship verification — Phase 27` and `FAIL → PASS` |
| `frontend/web/public/changelog.json` | User-facing changelog entry | VERIFIED | Contains `animepahe` entries (6 occurrences per SUMMARY self-check) |
| `services/scraper/internal/embeds/kwik.go` | Dual-packer fix | VERIFIED | Contains `extractAllPackers` (Rule 1 fix from Plan 27-04) |

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|-----|--------|---------|
| `services/animepahe-resolver/server.js` | `services/animepahe-resolver/upstream.js` | `require.*upstream` | VERIFIED | `upstream.js` import confirmed in server.js |
| `services/animepahe-resolver/upstream.js` | `services/animepahe-resolver/browser.js` | `page.evaluate` | VERIFIED | `page.evaluate` calls present in upstream.js |
| `services/animepahe-resolver/Dockerfile` | `services/animepahe-resolver/package-lock.json` | `npm ci` | VERIFIED | `npm ci --omit=dev` in Dockerfile |
| `.claude/maintenance-prompt.md` | `services/animepahe-resolver/STEALTH-PINS.md` | refresh procedure cross-link | VERIFIED | `STEALTH-PINS` reference found in maintenance-prompt.md |
| `services/scraper/internal/providers/animepahe/client.go` | `services/scraper/internal/providers/animepahe/resolver.go` | `resolverClient` + `kwikReferer` | VERIFIED | Both symbols found in client.go; resolverClient defined in resolver.go |
| `services/scraper/cmd/scraper-api/main.go` | `services/scraper/internal/providers/animepahe/resolver.go` | `AnimepaheResolverURL` config wiring | VERIFIED | Boot log shows `animepahe_resolver_url=http://animepahe-resolver:3000` |
| `docker/docker-compose.yml animepahe-resolver` | `services/animepahe-resolver/Dockerfile` | `build.dockerfile` | VERIFIED | `services/animepahe-resolver/Dockerfile` in compose service block |
| `docker/docker-compose.yml scraper` | `docker/docker-compose.yml animepahe-resolver` | `depends_on: service_healthy` | VERIFIED | `service_healthy` condition present; cold-start ordering proven empirically |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
|----------|---------------|--------|--------------------|--------|
| Episodes API endpoint | `episodes[]` | scraper → animepahe parser → resolverClient → animepahe-resolver sidecar → animepahe.pw | Yes — 28 episodes from live animepahe.pw via sidecar | FLOWING |
| Servers API endpoint | `servers[]` | scraper → animepahe parser → `ListServers` → `Play` HTML scrape via sidecar | Yes — 6 kwik servers returned live | FLOWING |
| Stream API endpoint | `sources[].url` | scraper → animepahe parser → Kwik extractor (dual-packer) → uwucdn.top m3u8 | Yes — m3u8 returns HTTP 200 live | FLOWING |
| `/scraper/health` animepahe stages | `stages.{episodes,search,servers,stream}.up` | scraper health cache → probe runner | Yes — all 4 stages `up:true` with recent timestamps | FLOWING |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| Frieren ≥ 28 episodes via animepahe | `GET /api/anime/f0b40660.../scraper/episodes?prefer=animepahe` | 28 episodes returned (HTTP 200) | PASS |
| ≥ 1 kwik server for Frieren ep 1 | `GET /api/anime/.../scraper/servers?episode=7bf604ba...` | 6 kwik servers returned | PASS |
| Fetchable m3u8 stream URL | `GET /api/anime/.../scraper/stream?...&prefer=animepahe` then `curl -sI -H "Referer: https://kwik.cx/" <url>` | HTTP/2 200 from vault-08.uwucdn.top | PASS |
| All four animepahe health stages up | `curl http://localhost:8088/scraper/health` | episodes/search/servers/stream all `up:true` | PASS |
| Node sidecar tests | `node --test` in `services/animepahe-resolver/` | 21/21 pass in ~2.4 s | PASS |
| Go parser tests | `go test ./internal/providers/animepahe/...` | All pass (0.023 s) | PASS |
| Go build | `go build ./...` from `services/scraper/` | Clean build | PASS |
| animepahe registered (not SKIPPED) at boot | scraper boot log | `registered provider {"name": "animepahe"}` present; `providers: 2` | PASS |
| 500 MB memory cap enforced at runtime | `docker inspect animeenigma-animepahe-resolver --format '{{.HostConfig.Memory}}'` | `524288000` (500 MiB) | PASS |
| D5 soak PASSED | STEALTH-PINS.md D5 section | 236 MiB peak (47% of cap), page_recycle_total=1, OOMKilled=false | PASS |
| SCRAPER_DEGRADED_PROVIDERS default = gogoanime only | compose config | `${SCRAPER_DEGRADED_PROVIDERS:-gogoanime}` (animepahe removed) | PASS |

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|-------------|-------------|--------|----------|
| SCRAPER-HEAL-29 | 27-01 | Stealth-Chromium sidecar scaffold — Node 20 + Fastify + puppeteer-extra + 500 MB D5 gate | SATISFIED | All sidecar artifacts present; 21 tests pass; D5=236 MiB documented; Pattern 7 wired |
| SCRAPER-HEAL-30 | 27-02 | Go parser rewrite + UUID-session contract + sidecar transport + ddosguard.go deleted | SATISFIED | resolver.go exists; ddosguard.go absent; p.baseURL absent; Go tests pass; session-shape validation + kwik dual-packer from Plan 27-04 Rule 1 fixes operational |
| SCRAPER-HEAL-31 | 27-03 | Compose wiring — animepahe-resolver service + mem_limit + scraper depends_on | SATISFIED | Compose block present; runtime memory cap confirmed; boot-ordering empirically proven |
| SCRAPER-HEAL-32 | 27-04 | End-to-end gate-clear — Frieren ≥ 28 episodes, fetchable m3u8, verdict log FAIL → PASS | SATISFIED | Live verification confirms 28 episodes, 6 kwik servers, HTTP 200 m3u8; verdict log updated |
| SCRAPER-HEAL-33 | 27-05 | SCRAPER_DEGRADED_PROVIDERS default flip + D7 gate + changelog + after-update | SATISFIED | Compose default = `gogoanime` only; animepahe registered; D7 gate (b) 0 flood lines; changelog entry present; co-authors commits present |

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| None | — | — | — | — |

No `TBD`, `FIXME`, `XXX`, or placeholder patterns found in any of the Phase 27-modified files. The sidecar has no stub implementations — all routes are live. No empty handlers, no hardcoded empty returns in the data path.

### Deviations Noted (not blockers)

The following deviations were documented by the execution plans and have no impact on goal achievement:

1. **Frieren goldens in `services/scraper/testdata/animepahe/`** — Plan 27-02 SUMMARY documents these as "placeholder goldens shaped per documented API contract" (captured from CONTEXT.md shapes, not from the live sidecar). The goldens pass `TestDTO_Frieren` (all DTO assertions green). The live gate-clear in Plan 27-04 verified the actual production payloads end-to-end — the test goldens serve their purpose of exercising DTO struct compatibility.

2. **Plan 27-04 Rule 1 fixes** — Two bugs were found during the gate-clear: (a) malsync.moe still publishes legacy numeric IDs for animepahe; session-shape validation (`sessionPattern` / `isSessionShape`) added to `client.go` to detect and fall through to `/search`; (b) Kwik embed pages contain two packed `eval()` blocks; `extractAllPackers` added to `kwik.go` to iterate all blocks. Both fixes are now in production and verified by the live endpoint tests.

3. **`kwikReferer` vs m3u8 Referer** — Plan 27-04 SUMMARY notes that the parser's `kwikReferer = "https://animepahe.pw/"` is the Referer for fetching the kwik.cx embed page (the parent-chain Referer), not the Referer for fetching the m3u8 itself. The m3u8 requires `Referer: https://kwik.cx/` which is returned in `data.stream.headers.Referer` in the response DTO. The live m3u8 check confirmed HTTP 200 with `Referer: https://kwik.cx/`.

### Human Verification Required

None. All success criteria are observable programmatically and were verified against the live production system during this verification run.

---

## Gaps Summary

None. All 5 must-haves are VERIFIED with live production evidence:

- Sidecar is `Up (healthy)` with runtime memory cap at 524288000 bytes
- Parser routes through `resolverClient`, `ddosguard.go` deleted, UUID-session contract active
- Compose wiring in place with `service_healthy` boot ordering proven
- Live Frieren probe returns 28 episodes, 6 kwik servers, fetchable m3u8 (HTTP 200)
- Compose default = `gogoanime` only, animepahe REGISTERED at boot, D7 gate (b) passed

Phase goal achieved: `GET /api/anime/f0b40660-6627-4a59-8dcf-7ec8596b3623/scraper/episodes?prefer=animepahe` returns 28 episodes for Frieren (MAL 52991) with playable Kwik stream URLs end-to-end, sidecar at 236 MiB (47% of 500 MB cap), `animepahe` removed from `SCRAPER_DEGRADED_PROVIDERS` default.

---

_Verified: 2026-05-19T11:40:18Z_
_Verifier: Claude (gsd-verifier)_
