---
phase: 22-provider-robustness
plan: "02"
subsystem: infra
tags: [hls-proxy, allowlist, ssrf, ad-decoy, vibeplayer, iss-011, integration, scraper]

# Dependency graph
requires:
  - phase: 21-playability-foundation
    provides: streamprobe playability gate + per-server priority sort (mitigation for ISS-011)
  - phase: 22-provider-robustness (Plan 22-01)
    provides: multi-URL extraction in streamhg / earnvids embeds (consumes the allowlist this plan adds)
provides:
  - HLSProxyAllowedDomains extended with two hls3 CDN eTLD+1 hosts (managementadvisory.sbs, exoplanethunting.space)
  - regression-lock test (TestHLSProxyAllowedDomains_HasStreamhgHls3Hosts) + behavior test (TestIsHLSDomainAllowed_Hls3Hosts) pinning the new entries
  - handler-level integration smoke (TestGetStream_MultiSource_BothHostsAllowlisted) proving both Sources survive the JSON round-trip and pass the allowlist gate
  - ISS-011 inline incident entry in docs/issues/README.md (Active Issues, status Mitigated)
  - User-facing changelog entry on 2026-05-13 announcing the backup CDN path + ISS-011 documentation
affects: [phase-23-canary, phase-24-warp-egress, future maintenance Pattern 7 hls4 rotations]

# Tech tracking
tech-stack:
  added: [libs/videoutils as a require+replace in services/scraper/go.mod]
  patterns:
    - "Plain string literals in HLSProxyAllowedDomains (per locked D2) so the maintenance bot's Pattern 7 fix-path finds them"
    - "Mirror-helper pattern: test-local re-derivation of package-private libs/videoutils.isHLSDomainAllowed to keep the libs surface minimal (// MIRROR: comment marks it)"
    - "Inline incident convention preserved: ISS-NNN appended to docs/issues/README.md, not split out to a separate file"

key-files:
  created:
    - .planning/phases/22-provider-robustness/22-02-SUMMARY.md
    - .planning/phases/22-provider-robustness/deferred-items.md
  modified:
    - libs/videoutils/proxy.go
    - libs/videoutils/proxy_test.go
    - services/scraper/internal/handler/scraper_test.go
    - services/scraper/go.mod
    - services/scraper/go.sum
    - docs/issues/README.md
    - frontend/web/public/changelog.json

key-decisions:
  - "D2 honored: hls3 hosts added as plain string literals (matches existing convention; maintenance bot Pattern 7 expects this layout)"
  - "D3 honored: ISS-011 entry inline in docs/issues/README.md, not a separate file"
  - "D4 honored: ISS-011 status = Mitigated (NOT Resolved); appended before the ## Resolved Issues heading so it stays in Active Issues until WARP egress ships"
  - "Mirror-helper (videoutilsHLSDomainAllowed) in scraper_test.go instead of exporting isHLSDomainAllowed from libs/videoutils — keeps libs surface minimal"

patterns-established:
  - "Per-phase section comment in HLSProxyAllowedDomains: 'Phase N — ...' + a SCRAPER-HEAL-NN traceability tag, so future audits can grep allowlist entries back to the requirement that motivated them"
  - "Handler-level integration smoke as the architectural closure of cross-package coupling: when libs/X depends on services/Y's output shape, drop a test in Y that imports X and asserts the contract"

requirements-completed: [SCRAPER-HEAL-10, SCRAPER-HEAL-11]

# Metrics
duration: 7min
completed: 2026-05-13
---

# Phase 22 Plan 02: HLS Proxy Allowlist + ISS-011 Inline Incident Entry Summary

**Closed the architectural loop on Phase 22's multi-URL fallback by adding the two hls3 CDN eTLD+1 hosts to HLSProxyAllowedDomains, locked them with a regression + behavior test pair, proved end-to-end survival via a handler-level integration smoke, and documented ISS-011 (VibePlayer ad-decoy poisoning) inline as the v3.1 motivating incident — status Mitigated pending WARP egress.**

## Performance

- **Duration:** ~7 min
- **Started:** 2026-05-13T06:26:28Z
- **Completed:** 2026-05-13T06:33:05Z
- **Tasks:** 3
- **Files modified:** 7 (5 source + 2 docs)

## Accomplishments

- HLSProxyAllowedDomains gains `managementadvisory.sbs` (StreamHG hls3) and `exoplanethunting.space` (Earnvids hls3) with a section comment that ties the entries back to SCRAPER-HEAL-10 and the PoC spec.
- Regression-lock + behavior tests pin the new entries: `TestHLSProxyAllowedDomains_HasStreamhgHls3Hosts` + `TestIsHLSDomainAllowed_Hls3Hosts` (the latter exercises exact match, subdomain match via HasSuffix-on-"."+allowed, port-stripping, and impostor rejection — `evilmanagementadvisory.sbs` is correctly rejected).
- Handler-level integration smoke `TestGetStream_MultiSource_BothHostsAllowlisted` constructs a fake provider returning a two-source Stream (premilkyway + managementadvisory hosts), drives the GetStream handler, and asserts both Source URLs survive the JSON round-trip AND both hostnames satisfy a test-local mirror of `isHLSDomainAllowed`. This is the explicit cross-package contract between Plan 22-01's extractor and Plan 22-02's allowlist.
- ISS-011 inline entry in `docs/issues/README.md`: full incident schema (Date / Severity / Affected / Symptom / Root cause / Why Grafana didn't catch it / Bonus discovery / Fix applied / Remaining work / Key files / Lesson learned / Status), placed in Active Issues directly before the `## Resolved Issues` heading, with status `Mitigated (2026-05-13)`. References Phase 21 Plan 03 server-priority deprioritization + libs/streamprobe ad-CDN blocklist as the applied mitigation; flags WARP egress as the path to Resolved.
- Production smoke post-redeploy: `/scraper/health` returns 200; `make health` shows all 8 services green; `/scraper/stream` for Frieren ep1 returns a 2-source Stream end-to-end (multi-URL path live in prod). Russian-language changelog entry shipped at the top of 2026-05-13 announcing the backup CDN path + ISS-011 docs.

## Task Commits

1. **Task 1 (TDD RED): failing tests for hls3 CDN allowlist** — `84ce48b` (test)
2. **Task 1 (TDD GREEN): allowlist hls3 CDN hosts** — `1545316` (feat)
3. **Task 1 (integration smoke): multi-source Stream both hosts allowlisted** — `e813cea` (test)
4. **Task 2: ISS-011 inline entry — VibePlayer ad-decoy poisoning (Mitigated)** — `92e9ab4` (docs)
5. **Task 3: changelog entry — hls3 backup CDN path + ISS-011** — `9d62d36` (docs)
6. **Task 3 (deferred items): DEF-22-01 non-hls3 CDN hosts unreachable** — `15f8aec` (docs)

Plan metadata (this SUMMARY + STATE/ROADMAP touches) will follow as a separate commit after this file lands.

## Files Created/Modified

- **libs/videoutils/proxy.go** — appended Phase 22 section comment + two literals `managementadvisory.sbs` / `exoplanethunting.space` in HLSProxyAllowedDomains.
- **libs/videoutils/proxy_test.go** — added `TestHLSProxyAllowedDomains_HasStreamhgHls3Hosts` (slice membership lock) + `TestIsHLSDomainAllowed_Hls3Hosts` (11-case behavior matrix including impostor rejection).
- **services/scraper/internal/handler/scraper_test.go** — added `videoutilsHLSDomainAllowed` test-local MIRROR + `TestGetStream_MultiSource_BothHostsAllowlisted` integration smoke; added `net/url` + `libs/videoutils` imports.
- **services/scraper/go.mod / go.sum** — added `libs/videoutils` as a require + replace; `go work sync` synced indirect deps.
- **docs/issues/README.md** — inserted ISS-011 entry before `## Resolved Issues` heading (28 lines, full incident schema, status Mitigated).
- **frontend/web/public/changelog.json** — prepended Russian-language `feature` entry for 2026-05-13 announcing hls3 backup CDN path + ISS-011 documentation.
- **.planning/phases/22-provider-robustness/deferred-items.md** — created with DEF-22-01 logging the discovery that `cdn-centaurus.com` + `goldenridgeproduction.shop` are not in HLSProxyAllowedDomains (out-of-scope; pre-existing).

## Decisions Made

- Honored locked decision D2 from 22-CONTEXT.md: hls3 hosts added as plain string literals (not wrapped in a helper) to match the existing convention and the maintenance bot's Pattern 7 fix-path expectations.
- Honored D3: ISS-011 inline in docs/issues/README.md (not a separate file).
- Honored D4: status = Mitigated, not Fixed/Resolved. Appended to Active Issues right before the `## Resolved Issues` heading so the section boundary stays correct.
- Chose the mirror-helper pattern over exporting `isHLSDomainAllowed` from libs/videoutils. Keeps the libs surface minimal; the regression-lock tests in libs/videoutils/proxy_test.go pin the source behavior, so the mirror only needs to share the same rules at the call site (annotated with a `// MIRROR:` comment).
- Did not push to remote — spawn context explicitly instructed "NO push — leave to user."

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Added libs/videoutils as a dependency of services/scraper**
- **Found during:** Task 1 (integration smoke compile attempt)
- **Issue:** The plan's Step 3 action requires the scraper handler test to import `libs/videoutils` for `HLSProxyAllowedDomains`, but services/scraper/go.mod did not list libs/videoutils as a require + replace. Compile would have failed.
- **Fix:** Added `github.com/ILITA-hub/animeenigma/libs/videoutils v0.0.0-00010101000000-000000000000` to the first `require` block and `replace github.com/ILITA-hub/animeenigma/libs/videoutils => ../../libs/videoutils` as a single-line replace; ran `go work sync` to refresh indirect deps in go.sum.
- **Files modified:** services/scraper/go.mod, services/scraper/go.sum
- **Verification:** `go build ./...` + `go vet ./...` + the new integration test all pass.
- **Committed in:** e813cea (Task 1 integration smoke commit)

**2. [Logged not fixed - DEF-22-01] cdn-centaurus.com + goldenridgeproduction.shop not in HLSProxyAllowedDomains**
- **Found during:** Task 3 production smoke (post-redeploy)
- **Issue:** The Frieren ep1 cold-path winner returned source hostnames `in1rhjc5cqhz.cdn-centaurus.com` + `in1rhjc5cqhz.goldenridgeproduction.shop` that are NOT in HLSProxyAllowedDomains. Streaming proxy would 403 those URLs on real playback.
- **Why not fixed inline:** Out of scope. These are different CDN families than the `hls2`/`hls3` pair the plan targets (premilkyway / dramiyos-cdn vs managementadvisory.sbs / exoplanethunting.space). The 403 is a pre-existing condition not introduced by Plan 22-02; my changes did not regress this surface. Auto-fixing would expand scope beyond the locked plan.
- **Logged as:** `.planning/phases/22-provider-robustness/deferred-items.md` DEF-22-01 — recommends a Phase 23 canary detection or a follow-up plan to audit all distinct CDN host families and reconcile the allowlist to a complete set.
- **Committed in:** 15f8aec

---

**Total deviations:** 1 auto-fix (Rule 3 blocking) + 1 deferred item logged (out-of-scope production-smoke finding).
**Impact on plan:** Rule 3 auto-fix was a pure dependency-graph correctness step — no scope expansion. DEF-22-01 is documentation, not action — surfaces a known reachable-CDN gap for a future plan without expanding 22-02.

## Issues Encountered

- **Parallel-wave file noise:** Plan 22-01's executor (running in Wave 1 alongside this plan) modified files in `services/scraper/internal/embeds/`, `services/scraper/internal/providers/gogoanime/`, and `services/scraper/testdata/gogoanime/`. Per spawn-context instructions ("file scopes do not overlap — DO NOT touch ..."), I only staged my own files in each commit (using explicit `git add <path>` rather than `git add -A`). No collision; final commit list shows clean separation.
- **UI-UX audit working-tree noise:** The pre-existing untracked `frontend/web/src/components/browse/` + `useBrowseFilters.ts` files and several committed `feat(ui-ux-audit/phase-15)` commits coexist on main during this plan's execution. I left them alone per spawn context.
- **make redeploy-web type-check trap:** Anticipated per spawn context. Avoided by going direct via `docker compose -f docker/docker-compose.yml build web && docker compose ... up -d web` — clean build, no type-check failures (the changelog.json edit doesn't touch TS).
- **Web port confusion during smoke:** Initial curl hit `localhost:80` and 404'd; container actually publishes on `127.0.0.1:3003`. Re-hit `http://localhost:3003/changelog.json` and confirmed deployment.

## Production Smoke Evidence

| Check | Command | Result |
|---|---|---|
| Scraper health | `curl -sf http://localhost:8088/scraper/health` | HTTP 200 |
| All services | `make health` | 8/8 green (gateway, auth, catalog, streaming, player, rooms, scheduler, scraper) |
| Multi-source live | `curl /scraper/stream?mal_id=52991&episode=frieren-...` | `sources_count: 2` |
| Allowlist literals | `grep -E "managementadvisory\|exoplanethunting" libs/videoutils/proxy.go` | both present with Phase 22 comment header |
| ISS-011 inline | `grep -c "ISS-011" docs/issues/README.md` + `awk '/^## Resolved/{exit} /^### ISS-011/{found=1} END{exit !found}'` | 1 occurrence; placement = Active Issues (before Resolved heading) |
| Status = Mitigated | `grep "Status.*Mitigated" docs/issues/README.md` | matches `Mitigated (2026-05-13) — ...` |
| Changelog deployed | `curl -s http://localhost:3003/changelog.json \| jq '.[0].entries[0].type'` | `"feature"` (top of 2026-05-13 entries) |
| No secrets leaked | `grep -E "([0-9]{1,3}\.){3}[0-9]{1,3}\|ak_[a-f0-9]{60,}\|API_KEY=\|password=" docs/issues/README.md` | no matches in ISS-011 block |
| Commits ahead of origin | `git log origin/main..HEAD --oneline \| wc -l` | 34 (6 mine; rest are pre-existing ui-ux-audit + plan-22-01 work) |

## Push Status

**NOT pushed.** Per spawn context: "NO push — leave to user." Local commits remain on `main` ahead of `origin/main`. The 6 Plan 22-02 commits sit alongside Wave 1 partner work and earlier ui-ux-audit work, all locally committed but unpushed.

## Self-Check

### Files claimed created

- `.planning/phases/22-provider-robustness/22-02-SUMMARY.md` — this file (PENDING write completion + commit)
- `.planning/phases/22-provider-robustness/deferred-items.md` — verified via `[ -f ... ] && echo FOUND`: present.

### Commits claimed exist

| Hash | Subject | Verified |
|---|---|---|
| 84ce48b | test(22-02): add failing tests for hls3 CDN allowlist | git log shows present |
| 1545316 | feat(22-02): allowlist hls3 CDN hosts | git log shows present |
| e813cea | test(22-02): integration smoke — multi-source Stream both hosts allowlisted | git log shows present |
| 92e9ab4 | docs(22-02): ISS-011 inline entry | git log shows present |
| 9d62d36 | docs(22-02): changelog entry — hls3 backup CDN path + ISS-011 | git log shows present |
| 15f8aec | docs(22-02): log DEF-22-01 — non-hls3 CDN hosts unreachable | git log shows present |

## Self-Check: PASSED

## Threat Flags

None — no new security-relevant surface introduced beyond the two narrowly-scoped allowlist additions. T-22-06 (SSRF expansion from allowlist) is structurally mitigated by the existing HasSuffix-on-"."+allowed rule (verified by `TestIsHLSDomainAllowed_Hls3Hosts` impostor-rejection cases). T-22-07 (doc leak in ISS-011) verified clean via the grep-based secret-leak check.

## Next Phase Readiness

- **Phase 22 closure:** With 22-01 (multi-URL extraction) + 22-02 (allowlist + ISS-011 docs) shipped and live in production, the v3.1 self-healing milestone's "per-server fallback URL family" rail is functionally complete.
- **Phase 23 canary:** The `parser_unplayable_total{reason="cdn_unreachable"}` + `parser_ad_decoy_total` metrics that this plan's mitigations depend on are emitting; Phase 23's job is to ingest them and fire alerts.
- **Phase 24 WARP egress:** Reserved future work; ISS-011 will move to Resolved only after that phase ships and 30-day flat-line on `parser_ad_decoy_total{server="vibeplayer"}`.
- **DEF-22-01 follow-up:** The `cdn-centaurus.com` + `goldenridgeproduction.shop` reachability gap surfaced during Task 3 smoke is the highest-priority defer to address in any next Phase 22 plan or via the maintenance Pattern 7 fix-path.
- **User push:** The 6 plan commits + 1 SUMMARY commit (pending) need to be pushed by the user to land on origin/main.

---

*Phase: 22-provider-robustness*
*Completed: 2026-05-13*
