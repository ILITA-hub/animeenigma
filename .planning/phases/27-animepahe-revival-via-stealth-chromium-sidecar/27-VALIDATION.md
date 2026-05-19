---
phase: 27
slug: animepahe-revival-via-stealth-chromium-sidecar
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-05-19
---

# Phase 27 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution. Sourced from `27-RESEARCH.md` § Validation Architecture (HIGH confidence).

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework (Go scraper side)** | Go stdlib `testing` + `httptest` + `prometheus/client_golang/prometheus/testutil` (already in `services/scraper/`) |
| **Framework (Node sidecar)** | Node `node:test` (stdlib) — matches existing `services/megacloud-extractor/` precedent |
| **Config (Go)** | none — `go test` defaults |
| **Config (Node)** | `services/animepahe-resolver/package.json` `"test"` script |
| **Quick run command (Go)** | `cd services/scraper && go test -count=1 -run 'AnimePahe' ./internal/providers/animepahe/...` |
| **Quick run command (Node)** | `cd services/animepahe-resolver && npm test` |
| **Full suite command** | `make redeploy-scraper && make redeploy-animepahe-resolver && make health && bash <(SCRAPER-HEAL-20 curl pipeline)` |
| **Estimated runtime (quick Go)** | ~5 seconds |
| **Estimated runtime (quick Node)** | ~10 seconds |
| **Estimated runtime (full)** | ~3 minutes (rebuild + redeploy + curl pipeline) |

---

## Sampling Rate

- **After every task commit:** Run the quick command for the layer touched (Go for parser changes; Node for sidecar changes).
- **After every plan wave:** Run both quick suites + `make health` + per-provider scraper health probe (`curl localhost:8088/scraper/health`).
- **Before `/gsd-verify-work`:** Full suite + Frieren curl pipeline must return ≥ 28 episodes through gateway with `prefer=animepahe`.
- **Max feedback latency:** 10 seconds for quick runs; ~3 minutes for full.

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 27-01-01 | 01 | 1 | SCRAPER-HEAL-29 | V5 input-validation | Fastify schema validation rejects non-UUID `session` params | unit (Node) | `cd services/animepahe-resolver && npm test -- validation` | ❌ W0 | ⬜ pending |
| 27-01-02 | 01 | 1 | SCRAPER-HEAL-29 | V4 host-allowlist | Upstream URL hardcoded to `animepahe.pw` regardless of param | unit (Node) | `cd services/animepahe-resolver && npm test -- host-allowlist` | ❌ W0 | ⬜ pending |
| 27-01-03 | 01 | 1 | SCRAPER-HEAL-29 | — | `/healthz` two-layer probe (HTTP + `page.evaluate(() => 1)`) returns 200/503 correctly | unit (Node) | `cd services/animepahe-resolver && npm test -- healthz` | ❌ W0 | ⬜ pending |
| 27-01-04 | 01 | 1 | SCRAPER-HEAL-29 | — | `/search` returns verbatim m=search JSON; 502 on 2nd challenge fail | unit (Node, with intercepted upstream) | `cd services/animepahe-resolver && npm test -- search` | ❌ W0 | ⬜ pending |
| 27-01-05 | 01 | 1 | SCRAPER-HEAL-29 | — | `/release` paginated passthrough | unit (Node) | `cd services/animepahe-resolver && npm test -- release` | ❌ W0 | ⬜ pending |
| 27-01-06 | 01 | 1 | SCRAPER-HEAL-29 | — | `/play` HTML passthrough | unit (Node) | `cd services/animepahe-resolver && npm test -- play` | ❌ W0 | ⬜ pending |
| 27-01-07 | 01 | 1 | SCRAPER-HEAL-29 | DoS | 100-request soak stays ≤ 500 MB RSS; page-recycle at N=100 verified | smoke (manual gate) | `for i in {1..100}; do curl -s http://localhost:3000/search?q=test; done & docker stats --no-stream animepahe-resolver` | ❌ W0 (Plan 27-01 final task) | ⬜ pending |
| 27-02-01 | 02 | 1 | SCRAPER-HEAL-30 | — | Resolver HTTP client maps 502 → ErrProviderDown, 404 → ErrNotFound | unit (Go) | `go test -count=1 -run TestResolverClient_ErrorMapping ./internal/providers/animepahe/...` | ❌ W0 | ⬜ pending |
| 27-02-02 | 02 | 1 | SCRAPER-HEAL-30 | — | DTO parses fresh-from-resolver Frieren goldens | unit (Go) | `go test -count=1 -run TestDTO_Frieren ./internal/providers/animepahe/...` | ❌ W0 (goldens captured 27-02) | ⬜ pending |
| 27-02-03 | 02 | 1 | SCRAPER-HEAL-30 | — | `FindID` via resolver/search with fuzzy fallback | unit (Go) | `go test -count=1 -run TestProvider_FindID ./internal/providers/animepahe/...` | ✅ exists (rewire) | ⬜ pending |
| 27-02-04 | 02 | 1 | SCRAPER-HEAL-30 | — | `ListEpisodes` via resolver/release multi-page | unit (Go) | `go test -count=1 -run TestProvider_ListEpisodes ./internal/providers/animepahe/...` | ✅ exists (rewire) | ⬜ pending |
| 27-02-05 | 02 | 1 | SCRAPER-HEAL-30 | — | `ListServers` via resolver/play HTML scraping | unit (Go) | `go test -count=1 -run TestProvider_ListServers ./internal/providers/animepahe/...` | ✅ exists (rewire) | ⬜ pending |
| 27-02-06 | 02 | 1 | SCRAPER-HEAL-30 | — | `GetStream` caches Kwik extraction (unchanged downstream) | unit (Go) | `go test -count=1 -run TestProvider_GetStream ./internal/providers/animepahe/...` | ✅ exists (unchanged) | ⬜ pending |
| 27-02-07 | 02 | 1 | SCRAPER-HEAL-30 | — | MalSync single-strike invalidation on `/release` 404 (A9) | unit (Go) | `go test -count=1 -run TestProvider_MalSyncInvalidationOn404 ./internal/providers/animepahe/...` | ❌ W0 | ⬜ pending |
| 27-02-08 | 02 | 1 | SCRAPER-HEAL-30 | — | `ddosguard.go` + `ddosguard_test.go` removed; package still compiles | unit (Go) | `go build ./...` | passes after deletion | ⬜ pending |
| 27-03-01 | 03 | 2 | SCRAPER-HEAL-31 | DoS | `mem_limit: 500m` enforced on resolver container | manual (smoke) | `docker inspect animepahe-resolver --format '{{.HostConfig.Memory}}'` returns 524288000 | manual gate | ⬜ pending |
| 27-03-02 | 03 | 2 | SCRAPER-HEAL-31 | — | scraper `depends_on: animepahe-resolver: condition: service_healthy` enforced | manual (smoke) | `docker compose up animepahe-resolver scraper && docker compose ps` (scraper does not start until resolver healthy) | manual gate | ⬜ pending |
| 27-03-03 | 03 | 2 | SCRAPER-HEAL-31 | — | scraper sees `SCRAPER_ANIMEPAHE_RESOLVER_URL` env at boot | unit (Go) | `go test -count=1 -run TestConfig_AnimepaheResolverURL ./internal/config/...` | ❌ W0 | ⬜ pending |
| 27-04-01 | 04 | 3 | SCRAPER-HEAL-32 | — | Frieren curl pipeline returns ≥ 28 episodes through gateway with `prefer=animepahe` | integration (manual gate, NOT in CI) | `BASE=http://localhost:8000 ANIME_ID=f0b40660-6627-4a59-8dcf-7ec8596b3623 ; curl -sS "$BASE/api/anime/$ANIME_ID/scraper/episodes?prefer=animepahe"` | manual gate | ⬜ pending |
| 27-04-02 | 04 | 3 | SCRAPER-HEAL-32 | — | Stream URL from server-list+stream chain is fetchable (HTTP 200/206) with Referer header from response | integration (manual gate) | curl pipeline final stage from `docs/issues/scraper-provider-verification-2026-05-19.md` | manual gate | ⬜ pending |
| 27-04-03 | 04 | 3 | SCRAPER-HEAL-32 | — | Phase 24 verdict log animepahe row flips FAIL → PASS | doc update | grep-confirm `docs/issues/scraper-provider-verification-2026-05-19.md` post-ship section exists with PASS | manual | ⬜ pending |
| 27-04-04 | 04 | 3 | SCRAPER-HEAL-32 | — | Per-provider health gauge: animepahe stages search/episodes/servers/stream all `up:true` 5 min after redeploy | observability | `curl http://localhost:8088/scraper/health \| jq '.providers.animepahe.stages \| to_entries[] \| select(.value.up==false)'` is empty | manual | ⬜ pending |
| 27-05-01 | 05 | 3 | SCRAPER-HEAL-33 | — | `SCRAPER_DEGRADED_PROVIDERS` compose default no longer contains `animepahe` | git verification | `git diff docker/docker-compose.yml` shows `gogoanime,animepahe` → `gogoanime` | manual | ⬜ pending |
| 27-05-02 | 05 | 3 | SCRAPER-HEAL-33 | — | No continuous 403/timeout pattern in scraper logs first 10 min post-redeploy (D7) | observability | `make logs-scraper \| head -200 \| grep -E '403\|context deadline' \| wc -l` ≤ 1 | manual | ⬜ pending |
| 27-05-03 | 05 | 3 | SCRAPER-HEAL-33 | — | `/animeenigma-after-update` skill runs cleanly; changelog entry committed | meta | inspect HEAD commit message + changelog.json diff | manual | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

Wave 0 = the test/fixture files that must EXIST before the plans can reference them in `<automated>` blocks.

- [ ] `services/animepahe-resolver/server.test.js` — covers SCRAPER-HEAL-29 (healthz, search, release, play, validation, host-allowlist)
- [ ] `services/animepahe-resolver/upstream.test.js` — covers the 403-retry + page-recycle logic
- [ ] `services/animepahe-resolver/__fixtures__/intercepted-frieren.json` — captured upstream payloads for offline test runs (Node)
- [ ] `services/scraper/testdata/animepahe/frieren-search.json` — captured fresh in 27-02 against the deployed sidecar per D4
- [ ] `services/scraper/testdata/animepahe/frieren-release.json` — captured fresh in 27-02 per D4
- [ ] `services/scraper/testdata/animepahe/frieren-play.html` — captured fresh in 27-02 per D4
- [ ] `services/scraper/internal/providers/animepahe/resolver.go` — new HTTP client for the sidecar
- [ ] `services/scraper/internal/providers/animepahe/resolver_test.go` — covers error-shape mapping (502 → ErrProviderDown, 404 → ErrNotFound)
- [ ] `services/scraper/internal/providers/animepahe/malsync_invalidation_test.go` — covers single-strike invalidation on /release 404 (A9)
- [ ] `services/scraper/internal/config/config_test.go` — extend with `TestConfig_AnimepaheResolverURL` (or add file if missing)

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| 500 MB memory ceiling under 100-request steady-state soak | SCRAPER-HEAL-29 | Empirical — Chromium memory behavior is build/kernel-dependent; can't be asserted reliably in CI | Plan 27-01 final task: run `for i in {1..100}; do curl -s http://localhost:3000/search?q=Frieren; done`; watch `docker stats animepahe-resolver` for max RSS; gate is `≤ 500m`. If exceeds, add page-recycle-every-N=100 and retry. |
| Live Frieren end-to-end curl pipeline | SCRAPER-HEAL-32 | Touches live upstream; not CI-stable | Plan 27-04: re-run the SCRAPER-HEAL-20 pipeline from `docs/issues/scraper-provider-verification-2026-05-19.md` verbatim; confirm episodes/servers/stream all PASS. |
| 10-minute no-403-flood log tail post-redeploy | SCRAPER-HEAL-33 | Time-windowed observation | Plan 27-05: after `make redeploy-scraper`, wait 10 minutes, inspect `make logs-scraper \| head -200`; ≤ 1 line matching `403\|context deadline`. |
| `depends_on: service_healthy` actually blocks scraper boot | SCRAPER-HEAL-31 | Compose runtime behavior; require live observation | Plan 27-03: `docker compose down && docker compose up animepahe-resolver scraper` and observe scraper stays in `Created` state until resolver healthcheck flips green. |

---

## Validation Sign-Off

- [ ] All tasks have an `<automated>` verify or Wave 0 dependency
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING file references
- [ ] No watch-mode flags in commands
- [ ] Feedback latency < 10 seconds for quick suites
- [ ] `nyquist_compliant: true` set in frontmatter (after planner approval)

**Approval:** pending — to be approved by `gsd-plan-checker` in Step 10.
