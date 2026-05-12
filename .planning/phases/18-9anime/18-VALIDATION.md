---
phase: 18
slug: 9anime
status: ready
nyquist_compliant: true
wave_0_complete: true
created: 2026-05-12
revised: 2026-05-12
---

# Phase 18 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.
> Revised after planner checker feedback (12 warnings — see CHECKER-REPORT.md).

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test (services/scraper), bunx tsc + eslint (frontend/web) |
| **Config file** | services/scraper/go.mod, frontend/web/package.json |
| **Quick run command** | `cd /data/animeenigma && go test ./services/scraper/... -count=1 -timeout=60s` |
| **Full suite command** | `cd /data/animeenigma && go test ./services/scraper/... ./libs/videoutils/... -count=1 -race -timeout=180s && cd frontend/web && bunx tsc --noEmit && bunx eslint src/components/player/EnglishPlayer.vue` |
| **Estimated runtime** | ~90 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test ./services/scraper/...` for backend tasks; `bunx tsc --noEmit` for frontend tasks.
- **After every plan wave:** Run full suite command.
- **Before `/gsd-verify-work`:** Full suite must be green.
- **Max feedback latency:** 90 seconds.

---

## Per-Task Verification Map

> One row per `<task>` in plans 18-01..18-04 (15 tasks total — 14 autonomous + 1 human-verify checkpoint).
> Every autonomous task has an `<automated>` verify command (Nyquist compliance).

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 18-01-T1 | 18-01 | 1 | All 9ANI-* | T-18-04 | Doc pivot annotations | grep | `grep -q "SCRAPER-9ANI-01..06 are implemented by the Gogoanime/Anitaku" .planning/REQUIREMENTS.md && grep -q "9anime → Anitaku/Gogoanime" .planning/ROADMAP.md` | ✅ REQUIREMENTS.md + ROADMAP.md | ⬜ pending |
| 18-01-T2 | 18-01 | 1 | 9ANI-01 | T-18-02 | fuzzy/ refactor pure code-motion | go build + go test | `cd /data/animeenigma && go build ./services/scraper/... && go test ./services/scraper/internal/fuzzy/... -count=1 -timeout=30s && go test ./services/scraper/internal/providers/animepahe/... -count=1 -timeout=60s` | ✅ services/scraper/internal/fuzzy/*.go | ⬜ pending |
| 18-01-T3 | 18-01 | 1 | 9ANI-01..04 | T-18-01, T-18-03 | Goldens captured + anonymized | filesystem + grep | `test -s services/scraper/testdata/gogoanime/one_piece_episode_1.html && ! grep -rqE '(Set-Cookie\|__ddg2_\|cf_clearance\|Bearer )' services/scraper/testdata/gogoanime/ && grep -q '^capture-goldens-gogoanime:' Makefile` | ✅ 8 fixtures + README + script | ⬜ pending |
| 18-01-T4 | 18-01 | 1 | 9ANI-01 | T-18-06 | URL validation at boot | go test | `cd /data/animeenigma && go test ./services/scraper/internal/config/... -run TestLoad_GogoanimeConfig -count=1 -v -timeout=30s` | ✅ services/scraper/internal/config/config.go | ⬜ pending |
| 18-01-T5 | 18-01 | 1 | 9ANI-01..04 | T-18-05 | RED test scaffolds compile + SKIP | go test | `cd /data/animeenigma && go build ./services/scraper/... && go test ./services/scraper/internal/providers/gogoanime/... ./services/scraper/internal/embeds/... -count=1 -timeout=30s -v 2>&1 \| grep -E '^(--- SKIP\|PASS\|ok)'` | ✅ 7 *_test.go files | ⬜ pending |
| 18-02-T1 | 18-02 | 2 | 9ANI-01, -02 | T-18-09, T-18-11 | dto/cache/malsync GREEN against goldens; **exported NewMalSyncClient + MalSyncOption** | go test | `cd /data/animeenigma && go test ./services/scraper/internal/providers/gogoanime/... -run 'TestSearchResult_GoldenParse\|TestEpisodeRow_GoldenParse\|TestServerRow_GoldenParse\|TestComputeStreamTTL_StreamHGSignedURL\|TestMalSync_NegativeCacheForGogoanime' -count=1 -v -timeout=60s` | ✅ doc.go/dto.go/cache.go/malsync.go | ⬜ pending |
| 18-02-T2 | 18-02 | 2 | 9ANI-01, -02, -06 | T-18-07..T-18-13 | Provider implements domain.Provider end-to-end; **exported Deps + New(d Deps)** | go test + go vet | `cd /data/animeenigma && go build ./services/scraper/... && go vet ./services/scraper/... && go test ./services/scraper/internal/providers/gogoanime/... -count=1 -race -v -timeout=120s` | ✅ services/scraper/internal/providers/gogoanime/client.go | ⬜ pending |
| 18-03-T1 | 18-03 | 2 | 9ANI-03, -04 | T-18-14..T-18-21 | Shared packedExtractor base type; **runGoja lifted to package-level (shared with kwik.go)** | go test | `cd /data/animeenigma && go test ./services/scraper/internal/embeds/... -run 'TestPackedExtractor_Matches_RejectsSubdomainImposters\|TestPackedExtractor_Extract_FromGolden\|TestKwik' -count=1 -v -timeout=60s` | ✅ services/scraper/internal/embeds/packed_common.go | ⬜ pending |
| 18-03-T2 | 18-03 | 2 | 9ANI-03, -04 | T-18-14..T-18-21 | 3 extractors implement domain.EmbedExtractor; **rewriteToSrv RoundTripper enforces real host + offline transport** | go test + go race | `cd /data/animeenigma && go test ./services/scraper/internal/embeds/... -count=1 -race -v -timeout=120s` | ✅ services/scraper/internal/embeds/{vibeplayer,streamhg,earnvids}.go | ⬜ pending |
| 18-04-T1 | 18-04 | 3 | 9ANI-03, -04, -06 | T-18-27 | main.go boot invariant: 2 providers registered | go build + go vet + grep | `cd /data/animeenigma && go build ./services/scraper/... && go vet ./services/scraper/... && grep -q 'gogoanime.New(gogoanime.Deps' services/scraper/cmd/scraper-api/main.go && grep -q 'orchestrator.Register(gogoanimeProvider)' services/scraper/cmd/scraper-api/main.go` | ✅ services/scraper/cmd/scraper-api/main.go | ⬜ pending |
| 18-04-T1b | 18-04 | 3 | 9ANI-06 | T-18-09, T-18-13 | **End-to-end orchestrator failover offline test** (animepahe unhealthy → gogoanime serves; parser_fallback_total increments) | go test | `cd /data/animeenigma && go test ./services/scraper/internal/service/... -run TestOrchestrator_AnimePaheToGogoanimeFailover -count=1 -v -timeout=60s` | ✅ services/scraper/internal/service/orchestrator_phase18_test.go | ⬜ pending |
| 18-04-T2 | 18-04 | 3 | 9ANI-05 | T-18-23, T-18-25 | HLS proxy allowlist append; Phase 16 regression-locked; rotating subdomains match | go test | `cd /data/animeenigma && go test ./libs/videoutils/... -run 'TestHLSProxyAllowedDomains_Phase18Additions\|TestHLSProxyAllowedDomains_Phase16RegressionLocked\|TestIsHLSDomainAllowed_RotatingSubdomains' -count=1 -v -timeout=30s` | ✅ libs/videoutils/proxy.go + proxy_test.go | ⬜ pending |
| 18-04-T3 | 18-04 | 3 | 9ANI-03..06 | T-18-26 | EnglishPlayer.vue: multi-option dropdown + capitalizeProvider branch + switchProvider + **2 new locale keys (sourceSwitchedAnnouncement + sourceOfflineSuffix in en/ru/ja)** | bunx tsc + bunx eslint + grep | `cd /data/animeenigma/frontend/web && bunx tsc --noEmit && bunx eslint src/components/player/EnglishPlayer.vue && grep -q "if (slug === 'gogoanime') return 'Anitaku'" src/components/player/EnglishPlayer.vue && grep -q 'sourceSwitchedAnnouncement' src/locales/en.json src/locales/ru.json src/locales/ja.json` | ✅ EnglishPlayer.vue + 3 locale files | ⬜ pending |
| 18-04-T4 | 18-04 | 3 | 9ANI-06 | T-18-24 | changelog.json valid + contains Anitaku entry | python json + grep | `cd /data/animeenigma && python3 -c "import json; json.load(open('frontend/web/public/changelog.json'))" && grep -q 'Anitaku' frontend/web/public/changelog.json` | ✅ frontend/web/public/changelog.json | ⬜ pending |
| 18-04-T5 | 18-04 | 3 | 9ANI-06 | T-18-22, T-18-28 | **HUMAN-VERIFY**: deploy + live failover smoke (forced animepahe down → gogoanime serves; parser_fallback_total++ in /metrics) | curl + manual | `curl -fsS http://localhost:8088/scraper/health \| jq -e '.providers \| has("gogoanime") and has("animepahe")' >/dev/null && curl -fsS http://localhost:8088/metrics \| grep -q '^provider_health_up{provider="gogoanime"'` | n/a — runtime gate | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements (closure tracker)

All Wave-0 gaps from RESEARCH.md §Wave 0 Gaps are now covered by 18-01 tasks plus the new 18-04-T1b integration test (Issue 10 resolution):

- [x] `services/scraper/internal/providers/gogoanime/*_test.go` — RED-state scaffolds (Plan 18-01 Task 5) — turn GREEN in Plan 18-02
- [x] `services/scraper/internal/embeds/{vibeplayer,streamhg,earnvids}_test.go` — RED-state scaffolds (Plan 18-01 Task 5) — turn GREEN in Plan 18-03
- [x] `services/scraper/internal/service/orchestrator_phase18_test.go` — End-to-end failover integration (Plan 18-04 Task 1b — Issue 10 closure)
- [x] `services/scraper/testdata/gogoanime/*` — 8 captured goldens + README + capture script (Plan 18-01 Task 3 — Issue 5 closure: capture-goldens-gogoanime refreshes all 8 atomically)
- [x] `frontend/web/tests/e2e/english-player-multi-provider.spec.ts` — Deferred to Phase 18-04 Task 5 manual verification; UI changes are exercised via tsc + eslint + manual smoke. A dedicated Playwright spec is not Phase 18 scope (deferred per CONTEXT.md S2 — "live probe + offline goldens is sufficient; e2e is v3.1+").

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Visual: source dropdown opens/closes, item state visible, accent on selected | SCRAPER-9ANI-04 (UI) | Visual styling per UI-SPEC | Open english player; click source chip; verify 2 options visible; verify accent on selected; verify (offline) suffix renders when a provider is unhealthy |
| End-to-end stream playback via gogoanime after a real-world failover | SCRAPER-9ANI-06 | Requires live AnimePahe failure or operator-forced unhealthy state | Force animepahe health to 0 (env override or admin endpoint); reload watch page; verify HLS playback for ≥30s; verify network panel shows traffic to vibeplayer.site/otakuhg.site/otakuvid.online |
| `parser_fallback_total{from="animepahe",to="gogoanime"}` increments live | SCRAPER-9ANI-06 | Prometheus scrape against running container | After forced-down test above: `curl -fsS http://localhost:8088/metrics \| grep parser_fallback_total` — assert >= 1 |

Note: the OFFLINE failover invariant is fully covered by the autonomous test 18-04-T1b — manual verification only confirms the wiring works against the LIVE upstream, which the offline test cannot validate.

---

## Validation Sign-Off

- [x] All autonomous tasks have `<automated>` verify commands (Nyquist compliance — 14/14 autonomous tasks)
- [x] Sampling continuity: no 3 consecutive tasks without automated verify (every task has its own command)
- [x] Wave 0 covers all MISSING references (orchestrator integration test added per Issue 10)
- [x] No watch-mode flags
- [x] Feedback latency < 90s (full suite ≈ 90s)
- [x] `nyquist_compliant: true` set in frontmatter

**Approval:** validated 2026-05-12 (revision pass — 12 checker warnings addressed)
