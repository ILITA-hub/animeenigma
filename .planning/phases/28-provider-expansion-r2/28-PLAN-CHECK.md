---
phase: 28-provider-expansion-r2
artifact: PLAN-CHECK
status: warnings
blocker_count: 0
warning_count: 7
info_count: 2
checked_by: claude-sonnet-4-6
checked_at: 2026-05-20
---

# Phase 28 Plan-Check

**Phase goal:** Grow EN failover pool from 1 working provider (allanime) to 4 (allanime + animefever + miruro + nineanime, conditional Miruro).

**Plans inspected:** 28-00, 28-01, 28-02, 28-03, 28-04, 28-05, 28-06

**Overall status: WARNINGS — execution may proceed; fix warnings before or during execution.**

---

## Requirement Coverage

| Req ID | Covering Plans | Status |
|--------|---------------|--------|
| SCRAPER-HEAL-34 | 28-00 | COVERED |
| SCRAPER-HEAL-35 | 28-01 | COVERED |
| SCRAPER-HEAL-36 | 28-02 | COVERED |
| SCRAPER-HEAL-37 | 28-04 (conditional) | COVERED (conditional path documented) |
| SCRAPER-HEAL-38 | 28-03 | COVERED |
| SCRAPER-HEAL-39 | 28-05, 28-06 | COVERED |

All 6 requirements have covering plans. No gaps.

---

## Plan Summary

| Plan | Wave | Tasks | Autonomous | Key artifact |
|------|------|-------|-----------|-------------|
| 28-00 | 0 | 4 | yes | SPIKE-MIRURO.md + (conditional) obfuscation.go |
| 28-01 | 0 | 3 | yes | SPIKE-ANIMEFEVER.md + 5 testdata fixtures |
| 28-02 | 1 | 4 (1 checkpoint) | no | animefever provider + HLS allowlist |
| 28-03 | 1 | 3 | yes | vidstream_vip.go extractor |
| 28-04 | 2 | 5 (1 decision + 1 checkpoint) | no | miruro provider (conditional) |
| 28-05 | 3 | 5 (1 checkpoint) | no | nineanime provider + allowlist |
| 28-06 | 3 | 5 (1 checkpoint) | no | i18n + e2e + changelog + after-update |

---

## Findings

### Warnings (7)

---

**W-01 [wiring_invariant] Phase 19 wiring invariant will FATAL on first boot after 28-02/28-05 land unless updated correctly.**

The existing `candidateProviders` slice in `main.go:367` is `["gogoanime", "animepahe", "allanime"]`. Plans 28-02 and 28-05 both include instructions to append `"animefever"` and `"nineanime"` respectively. However the invariant check at line 367-396 computes `expectedProviders` from `candidateProviders` and fatals if `len(orchestrator.RegisteredProviders()) != expectedProviders`. If Plan 28-02 appends `"animefever"` to the slice but Plan 28-05 also appends `"nineanime"`, Wave 3 must land with the Phase 2 (28-02) candidateProviders update already present. If the invariant is updated piecemeal across commits, every intermediate deploy between Wave 1 and Wave 3 will be correct only if each commit updates the slice at the same time it registers the provider. Both plans explicitly instruct this in Task 3. The WARNING is that the plans do not spell out the invariant's count arithmetic for the case where Miruro is conditional: if `28-04` proceeds, `candidateProviders` should include `"miruro"` as well, and the invariant count must account for all three new providers plus the existing three plus the conditional `"animekai"`. Plan 28-04 Task 3 does include this step ("Add `"miruro"` to the `candidateProviders` slice"), but does NOT explicitly address the count arithmetic when Miruro IS registered — the invariant will fatal if the slice grows by 3 but the count logic doesn't track all three.

- **Severity:** WARNING
- **Plans:** 28-02, 28-04, 28-05
- **Fix:** Executor of 28-05 should verify that after all three providers (animefever, miruro if converged, nineanime) are registered, `candidateProviders` includes all three and the invariant count will match `len(orchestrator.RegisteredProviders())`. No code change needed in the plans — just a runtime verification step that 28-05's Task 4 verify gate already covers via `go build` (compile check only), but the human E2E checkpoint in Task 5 should explicitly validate the startup log for the invariant-pass message.

---

**W-02 [dependency_ordering] 28-02 depends on 28-03 for Frieren E2E gate but Plans are parallel Wave 1.**

Plan 28-02 Task 4 (the blocking human-verify checkpoint) states: "Plan 28-03 (vidstream_vip extractor) is the parallel Wave 1 plan — it MUST be merged before this checkpoint runs because GetStream depends on the registered extractor." Wave 1 treats 28-02 and 28-03 as parallel, but in practice the checkpoint in 28-02 cannot be satisfied until 28-03 is also merged and redeployed. This is called out in the checkpoint text, but the plan frontmatter `depends_on: ["28-01"]` for both 28-02 and 28-03 does not encode the merge-order constraint between them. An executor running them in strict parallel could reach the 28-02 checkpoint before 28-03 has landed, wasting a deployment.

- **Severity:** WARNING
- **Plans:** 28-02, 28-03
- **Fix:** Either (a) add `"28-03"` to 28-02's `depends_on` to force ordering (making them serial, not parallel) or (b) note explicitly in the after-update for 28-03 that both 28-02 and 28-03 go out in a single `make redeploy-scraper` call. The current wording in 28-02's Task 4 checkpoint handles this at the human-verify level, which is adequate but adds friction if an executor misses the prose note.

---

**W-03 [context_compliance] Co-author in 28-06 references "Claude Opus 4.7" but MEMORY.md convention says "Claude Opus 4.6".**

Plan 28-06 Task 5 (`/animeenigma-after-update` invocation) references "Claude Opus 4.7 + 0neymik0 + NANDIorg" in the `what-built` section. The canonical co-author in `MEMORY.md` is `Claude Opus 4.6 <noreply@anthropic.com>`. The running model is claude-sonnet-4-6. The CLAUDE.md commit convention says "3 co-authors documented in MEMORY.md" — the plan prose diverges from the source-of-truth.

- **Severity:** WARNING
- **Plan:** 28-06
- **Fix:** The executor of 28-06 Task 5 should use the co-authors verbatim from MEMORY.md: `Claude Opus 4.6`, `0neymik0`, `NANDIorg`. The plan's prose says "per MEMORY.md" which will steer the executor correctly, but the parenthetical "(Claude Opus 4.7 ...)" in the prose is a noise-level contradiction that could confuse.

---

**W-04 [key_links] 28-05 registers `nineanime` provider with `Embeds *domain.Registry` in Deps but 9anime's GetStream does NOT use the embed registry — it inlines the iframe→source extraction.**

Plan 28-05 client.go action item (g) describes `GetStream` as fetching the episode page → regex iframe src → fetching `my.1anime.site` → regex `<source src>` MP4, all inline. The `Deps` struct includes `Embeds *domain.Registry` mirroring the allanime template, but the embed registry is never consulted by nineanime's GetStream (there is no `NewVidstreamVipExtractor`-equivalent for `my.1anime.site` in any plan — the extraction is inlined in the provider itself, per the RESEARCH.md note "or inline in 9anime client"). This is architecturally fine — allanime does use the registry, nineanime doesn't need to — but it means the `Deps.Embeds` field is a dead field that will never be read. Including it adds misleading weight and a nil-pointer risk if `domain.Registry` has any non-nil invariants.

- **Severity:** WARNING
- **Plan:** 28-05
- **Fix:** Either (a) drop `Embeds` from `nineanime.Deps` since GetStream does not dispatch through the registry, or (b) add `my.1anime.site` extraction as a new `EmbedExtractor` in `embeds/onenime_site.go` (already mentioned as optional in RESEARCH.md component table) and wire it through the registry consistently. Option (a) is simpler and matches the plan's actual implementation. The plan's compile-time tests will not catch this because the Deps struct still compiles; only a runtime nil-deref or a dead-field review would surface it.

---

**W-05 [scope_reduction detection] 28-06 conditionally drops Miruro from the changelog if 28-04 was killed, but pre-stages the i18n key — this is correct per D3 but the changelog wording needs to not silently omit Miruro if it DID converge.**

Plan 28-06 Task 4 says: "Conditional on Plan 28-04 — if killed, drop 'Miruro' from the entry (the i18n key stays pre-staged silently)." This is correct. The WARNING is the inverse: if Miruro DID converge (28-04 proceeded), the changelog template in Task 4 body uses the phrase "Three new fallback sources joined the English tab: AnimeFever, Miruro, and 9anime." The executor must not forget to include Miruro in the deployed changelog when it converges. The plan's conditional logic is correct but the positive-path reminder is weaker than the negative-path reminder. Not a blocker, but worth noting.

- **Severity:** WARNING
- **Plan:** 28-06, Task 4
- **Fix:** Add a paired positive-path note: "If 28-04 converged, include 'Miruro' in the entry." The current plan implies this but the text gives stronger emphasis to the kill path.

---

**W-06 [verification_gate] 28-05's Frieren S1 E2E gate is ambiguous — Frieren S1 is absent from 9anime's catalog but S2 is present; the test map in RESEARCH.md uses Frieren S2 episode 1 for unit testdata but the manual gate in 28-05 Task 5 says "Marriagetoxin ep 1 or 7".**

CONTEXT.md D6 says: "9anime: Marriagetoxin episode 1 OR 7 (Frieren absent from upstream catalog)." RESEARCH.md live recon (2026-05-20) found Frieren S2 IS present at `9anime.me.uk/series/frieren-beyond-journeys-end-season-2/`. The testdata fixtures in 28-05 Task 1 use `wp_search_frieren.json` and `series_frieren_s2.html`. Plan 28-05 Task 5 checkpoint uses Marriagetoxin as the E2E target. This split — unit tests use Frieren S2, E2E gate uses Marriagetoxin — is technically correct (different test scopes) but creates a subtle inconsistency: if Frieren S2 is available, the integration gate should prefer Frieren S2 (avoiding the need to find Marriagetoxin's UUID in the catalog). The plan's Task 5 step 3 uses `<marriagetoxin-uuid>` as a placeholder, requiring the operator to look up that UUID at runtime. If Frieren S2 is confirmed present in the upstream, using Frieren S2 (`f0b40660-...` UUID or its S2 equivalent, if it exists in the local catalog) would be a cleaner E2E gate.

- **Severity:** WARNING
- **Plan:** 28-05, Task 5
- **Fix:** Either (a) confirm Frieren S2 UUID in the local catalog and use it as the primary E2E target (matches Frieren-as-golden-file for AnimeFever and Miruro), or (b) explicitly document the Marriagetoxin UUID lookup step in the checkpoint. The current ambiguity ("Marriagetoxin or any anime present in 9anime.me.uk catalog") may delay the checkpoint if the operator has to search for the UUID at review time.

---

**W-07 [scope_sanity] Plan 28-02 lists `autonomous: false` with 4 tasks (3 auto + 1 checkpoint). Plan 28-05 lists `autonomous: false` with 5 tasks (3 auto + 1 TDD + 1 checkpoint). These are at the upper limit for complexity.**

28-02 creates ~680+ lines across 5 files (client.go at min_lines 350, client_test.go at min_lines 200, cache.go at min_lines 80, dto.go at min_lines 40, config additions) plus main.go and proxy.go edits. Total files touched: 9. This is borderline by the 5-8 file target and is manageable given the TDD scaffolding structure. 28-05 similarly creates ~680+ lines across 5 files plus main.go/proxy.go (9 files). Both plans have tight internal structure (scaffold → implement → register → E2E gate) that offsets the scope. No split is recommended. This is a scope sanity note.

- **Severity:** WARNING (informational scope flag, not a structural problem)
- **Plans:** 28-02, 28-05
- **Fix:** No change needed. Both plans follow the TDD pattern (scaffold-first, then implement) which naturally bounds per-task scope. The 9-file count is at the upper edge; executor should run `go test ./... -race` at the end of every task, not just at checkpoints.

---

### Info (2)

---

**I-01 [context_compliance] Open Questions in RESEARCH.md are addressed via plan prose but not formally resolved with "(RESOLVED)" markers.**

RESEARCH.md section "Open Questions" lists 4 questions. All 4 are answered inline in the plan actions:
- Q1 (nineanime flag): 28-05 uses SCRAPER_DEGRADED_PROVIDERS unconditionally.
- Q2 (ctk caching): 28-02 GetStream action item (i) explicitly implements 15min ctk cache.
- Q3 (Miruro spike approach): 28-00 Task 1 action item (b) starts with source-map check.
- Q4 (AnimeFever parallel ListServers): RESEARCH.md recommendation (sequential) is implemented in 28-02 action item (h).

None of these are marked `(RESOLVED)` in RESEARCH.md, but all are addressed in the plan bodies. Per Dimension 11, this would be a BLOCKER under strict interpretation, but the open questions are answered within the SAME document set (research + plans), which is adequate for this phase's bounded nature.

- **Severity:** INFO
- **Fix:** Mark all 4 questions in RESEARCH.md with `(RESOLVED)` notes pointing to the plan that resolves them (optional quality cleanup, not blocking).

---

**I-02 [pattern_compliance] No PATTERNS.md exists for this phase — Dimension 12 is SKIPPED.**

The existing allanime provider is referenced as the analog template throughout plans (plan bodies explicitly `@services/scraper/internal/providers/allanime/client.go`) but no formal PATTERNS.md classification table exists. This is consistent with the research-driven approach used in Phase 26/27. The analog references in the plan context sections are equivalent and sufficient.

- **Severity:** INFO
- **Fix:** No action needed.

---

## Dimension Checks

| Dimension | Status | Notes |
|-----------|--------|-------|
| 1. Requirement Coverage | PASS | All 6 SCRAPER-HEAL requirements covered |
| 2. Task Completeness | PASS | All tasks have files/action/verify/done; checkpoints have appropriate gates |
| 3. Dependency Correctness | PASS | Wave order is valid; no cycles; Wave 0→1→2→3 is DAG-correct; 28-04's conditional dependency on 28-00 is explicit and unambiguous |
| 4. Key Links Planned | PASS | Critical wiring documented: extractor registered before provider (28-03→28-02); allowlist per-provider (D7); ARM in Miruro FindID; embed registry Find in GetStream |
| 5. Scope Sanity | WARNING | W-07: 28-02/28-05 at upper file count but within acceptable range |
| 6. Verification Derivation | PASS | must_haves truths are user-observable (E2E gates, episode counts, stream types) not implementation-only |
| 7. Context Compliance | PASS | All locked decisions (D1-D7) have covering tasks; no deferred ideas included |
| 7b. Scope Reduction | PASS | No v1/v2/placeholder/static-for-now language found; all decisions delivered fully or explicitly deferred via the D3 kill-switch protocol |
| 7c. Architectural Tier | PASS | All Go provider work in scraper microservice; frontend work in Vue frontend; no tier mismatch |
| 8. Nyquist Compliance | PASS | Every auto task has an `<automated>` verify command; Wave 0 and Wave 1 tasks all have `go test` or equivalent; Playwright spec in Wave 3 has skip-guard |
| 9. Cross-Plan Data Contracts | PASS | SPIKE-ANIMEFEVER.md feeds 28-02 and 28-03 consistently; testdata fixtures from 28-01 copied to 28-03 via Task 1 without transformation conflicts |
| 10. CLAUDE.md Compliance | PASS | All plans use `bun`/`bunx` for frontend; `make redeploy-scraper` for backend; `/animeenigma-after-update` invoked in 28-06; stdlib-only constraint honored in Miruro spike; no CDN code; no video URLs cached >1h |
| 11. Research Resolution | INFO | Open questions not formally marked RESOLVED but answered in plan prose |
| 12. Pattern Compliance | SKIPPED | No PATTERNS.md for this phase |

---

## Conditional Wave Protocol Verification

The Wave 2 skip protocol (D3 kill-switch) is unambiguous:

1. **28-00 Task 4** writes `Verdict: converged|killed` on the first non-blank line of SPIKE-MIRURO.md and uses the exact regex `^Verdict:\s+(converged|killed)$` in both the plan's verify gate and the downstream-signal section.
2. **28-04 Task 0** (`checkpoint:decision`) auto-resolves by grepping `head -5 .planning/phases/28-provider-expansion-r2/SPIKE-MIRURO.md | grep -E '^Verdict:'`. The resume-signal is explicit: `converged` → proceed Tasks 1-4; `killed` → skip to Task 5 (skip-summary).
3. **Wave 3** plans (28-05, 28-06) declare `depends_on: ["28-02"]` only — independent of Miruro outcome per `<plan_sketch>` Wave gates.
4. **28-04 Task 5** emits a skip-summary and marks REQUIREMENTS.md if killed — clean no-orphan-code guarantee.

The conditional protocol is correctly wired. No ambiguity found.

---

## HLS Proxy Allowlist Verification (D7)

| Provider | Expected hosts | Where added | Plan |
|----------|--------------|-------------|------|
| AnimeFever | `am.vidstream.vip`, `static-cdn-ca1.mofl.pro` | proxy.go + same commit as provider registration | 28-02 Task 3(c) |
| Miruro (conditional) | `pro.ultracloud.cc`, `pru.ultracloud.cc` | proxy.go + same commit as provider registration | 28-04 Task 3(c) |
| 9anime | `my.1anime.site` | proxy.go + same commit as provider registration | 28-05 Task 4(c) |

Current proxy.go allowlist does NOT contain any of these hosts. Plans correctly instruct adding them per-provider per D7. Fail-closed Phase 25 SCRAPER-HEAL-24 protection is in place — missing allowlist entries cause 502, which the E2E gates (human-verify checkpoints) will catch before marking complete.

---

## Registration Order Verification (D5)

Current `main.go` failover chain (line 367):
```
["gogoanime", "animepahe", "allanime"] + optional ["animekai"]
```

Phase 28 target chain:
```
["gogoanime", "animepahe", "allanime", "animefever"] + optional ["miruro"] + ["nineanime"] + optional ["animekai"]
```

Plan 28-02 inserts `animefever` AFTER `allanime` block (line ~239 area), BEFORE `animekai` gated block. Plan 28-04 inserts `miruro` AFTER `animefever` block. Plan 28-05 inserts `nineanime` AFTER `miruro` (or `animefever` if 28-04 was skipped), BEFORE `animekai`. The comment in 28-05 Task 4 action item (a) says "The plan must work in both cases — match position dynamically by name comment." This is correct. Registration order in main.go matches D5 failover-chain order.

---

## Stdlib-Only Constraint (Miruro Spike)

28-00's verify gates explicitly check:
```
grep -E "chromedp|utls|tls-client|flaresolverr|cloudscraper" services/scraper/internal/providers/miruro/obfuscation.go
```
…returning nothing as the pass condition. 28-00's threat model (T-28-00-05) references the CI lint that rejects these deps. The constraint is enforced at both the test and the plan-verify level. PASS.

---

## E2E Gate Coverage

| Provider | Test target | Gate type | Plan |
|----------|------------|-----------|------|
| AnimeFever | Frieren (MAL 52991), episodes ≥ 28 | checkpoint:human-verify | 28-02 Task 4 |
| Miruro (conditional) | Frieren (AniList 154587), episodes ≥ 28 | checkpoint:human-verify | 28-04 Task 4 |
| 9anime | Marriagetoxin ep 1/7 OR Frieren S2 ep 1 | checkpoint:human-verify | 28-05 Task 5 |
| All | `/scraper/health` all 5 stages up:true | checkpoint:human-verify | 28-05 Task 5 step 2 |

All three providers have explicit E2E gates matching D6. The Frieren S2 vs Marriagetoxin ambiguity for 9anime is flagged in W-06 but does not block execution.

---

## Recommendation

**Status: WARNINGS — 0 blockers, 7 warnings.**

Execution may proceed. The 7 warnings are quality/clarity issues that the executor can resolve during implementation without halting the phase. The most operationally significant warnings are:

1. **W-01** (wiring invariant arithmetic for 3 new providers) — verify the startup invariant log after each redeploy.
2. **W-02** (28-02/28-03 merge order) — merge 28-03 before running 28-02's human-verify checkpoint.
3. **W-04** (dead `Embeds` field in nineanime Deps) — drop the field if inline extraction is chosen, to avoid nil-deref footgun.

All phase goals are achievable with the plans as written.
