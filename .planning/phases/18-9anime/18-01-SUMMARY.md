---
phase: 18-9anime
plan: 01
subsystem: infra
tags: [scraper, gogoanime, anitaku, fuzzy-match, jaro-winkler, goldens, env-config, red-tdd]

# Dependency graph
requires:
  - phase: 17-observability
    provides: "health stage constants + parser_zero_match_total seam"
  - phase: 16-animepahe
    provides: "animepahe.cache.go jaroWinkler/normalizeTitle source code (verbatim move)"
provides:
  - "REQUIREMENTS.md + ROADMAP.md pivot annotations (9anime → Anitaku/Gogoanime)"
  - "services/scraper/internal/fuzzy/ shared package (NormalizeTitle + JaroWinkler exported)"
  - "8 captured anitaku.to / embed-wrapper / malsync goldens under services/scraper/testdata/gogoanime/"
  - "services/scraper/scripts/capture-gogoanime-goldens.sh + Makefile capture-goldens-gogoanime target"
  - "GogoanimeConfig + SCRAPER_GOGOANIME_BASE_URL env var (default https://anitaku.to)"
  - "RED-state test scaffolds: 14 tests in services/scraper/internal/providers/gogoanime/ + 7 tests in services/scraper/internal/embeds/{vibeplayer,streamhg,earnvids}_test.go (all SKIP, ready for 18-02 + 18-03 to turn GREEN)"
affects:
  - "18-02 (Gogoanime provider impl — uses fuzzy/ + config.Gogoanime.BaseURL + goldens + RED tests in gogoanime/)"
  - "18-03 (3 embed extractors — uses goldens + RED tests in embeds/)"
  - "18-04 (orchestrator wiring — uses GogoanimeConfig.BaseURL + 5-hostname HLS allowlist + EnglishPlayer.vue dropdown)"

# Tech tracking
tech-stack:
  added:
    - "services/scraper/internal/fuzzy package (in-tree shared utility — no new external deps)"
  patterns:
    - "Shared-utility extraction triggered by second consumer (avoid premature abstraction)"
    - "Atomic golden capture + anonymization gate (curl -fsSL + sed strip + grep gate scoped to *.html + *.json)"
    - "RED-state TDD scaffolds (t.Skip with deterministic message) so Wave 2 plans turn each SKIP into PASS"

key-files:
  created:
    - "services/scraper/internal/fuzzy/jarowinkler.go"
    - "services/scraper/internal/fuzzy/normalize.go"
    - "services/scraper/internal/fuzzy/fuzzy_test.go"
    - "services/scraper/internal/providers/gogoanime/doc.go"
    - "services/scraper/internal/providers/gogoanime/client_test.go"
    - "services/scraper/internal/providers/gogoanime/dto_test.go"
    - "services/scraper/internal/providers/gogoanime/malsync_test.go"
    - "services/scraper/internal/providers/gogoanime/cache_test.go"
    - "services/scraper/internal/embeds/vibeplayer_test.go"
    - "services/scraper/internal/embeds/streamhg_test.go"
    - "services/scraper/internal/embeds/earnvids_test.go"
    - "services/scraper/testdata/gogoanime/README.md"
    - "services/scraper/testdata/gogoanime/search_attack_on_titan.html"
    - "services/scraper/testdata/gogoanime/category_one_piece.html"
    - "services/scraper/testdata/gogoanime/category_one_piece_dub.html"
    - "services/scraper/testdata/gogoanime/one_piece_episode_1.html"
    - "services/scraper/testdata/gogoanime/vibeplayer_embed.html"
    - "services/scraper/testdata/gogoanime/streamhg_packed.html"
    - "services/scraper/testdata/gogoanime/earnvids_packed.html"
    - "services/scraper/testdata/gogoanime/malsync_no_gogo.json"
    - "services/scraper/scripts/capture-gogoanime-goldens.sh"
  modified:
    - ".planning/REQUIREMENTS.md"
    - ".planning/ROADMAP.md"
    - "services/scraper/internal/providers/animepahe/cache.go"
    - "services/scraper/internal/providers/animepahe/cache_test.go"
    - "services/scraper/internal/providers/animepahe/client.go"
    - "services/scraper/internal/config/config.go"
    - "services/scraper/internal/config/config_test.go"
    - "Makefile"
    - "docker/.env.example"

key-decisions:
  - "Doc pivot in-place (no requirement IDs renumbered) — SCRAPER-9ANI-01..06 keep their literal prefix; an annotation maps them to the Gogoanime/Anitaku implementation."
  - "Promote fuzzy helpers to services/scraper/internal/fuzzy/ as the second consumer (Gogoanime) joins — pure code-motion, both packages now consume the same source."
  - "Anonymization gate scoped to *.html + *.json (not whole testdata/gogoanime/ tree) so the README can document the forbidden patterns without self-failing the gate."
  - "RED scaffolds use t.Skip (not t.Fatal) so the test suite stays green on main while Wave 2 plans land impl."

patterns-established:
  - "Shared-utility extraction triggered by second consumer: avoid building shared packages on first use (premature abstraction); promote on second consumer."
  - "Atomic golden capture: one script fetches all fixtures + dispatches embed-wrapper fetches by host extracted from the episode page, then re-asserts anonymization invariant."
  - "Upstream-death recovery protocol: curl -fsSL halts on 4xx/5xx; executor stops without committing partial goldens; phase pivots per RESEARCH.md §Mirror Viability."
  - "RED-state scaffold pattern: package compiles, tests SKIP with deterministic message, Wave 2 turns each SKIP into PASS (avoids breaking main between waves)."

requirements-completed:
  - SCRAPER-9ANI-01  # scaffolding only — provider impl arrives in 18-02
  - SCRAPER-9ANI-02  # scaffolding only
  - SCRAPER-9ANI-03  # scaffolding only
  - SCRAPER-9ANI-04  # scaffolding only
  - SCRAPER-9ANI-05  # scaffolding only
  - SCRAPER-9ANI-06  # scaffolding only

# Metrics
duration: ~25min
completed: 2026-05-12
---

# Phase 18 Plan 01: Wave-0 Scaffolding for Anitaku/Gogoanime Pivot — Summary

**Shared fuzzy/ package extracted + 8 anitaku.to goldens captured atomically + GogoanimeConfig env var + 20 RED-state test scaffolds, all on a clean break from the dead 9anime brand.**

## Performance

- **Duration:** ~25 min
- **Started:** 2026-05-12 (UTC)
- **Completed:** 2026-05-12 (UTC)
- **Tasks:** 5
- **Files created:** 21
- **Files modified:** 9

## Accomplishments

- **Documentation pivot landed** — REQUIREMENTS.md annotates SCRAPER-9ANI-01..06 as implemented by Gogoanime/Anitaku (display "Anitaku", backend slug "gogoanime"); ROADMAP.md Phase 18 title renamed to "9anime → Anitaku/Gogoanime (pivoted)" in both the v3.0 summary list and the Phase 18 detail header. Existing Phase 18 plan list (18-01..18-04) reflects the new 3-wave structure.
- **services/scraper/internal/fuzzy/ shared package extracted** — `NormalizeTitle` + `JaroWinkler` exported (pure code-motion from animepahe/cache.go). AnimePahe consumer migrated to import fuzzy and call fuzzy.NormalizeTitle / fuzzy.JaroWinkler. AnimePahe test suite stays green after the move. Two test functions in fuzzy_test.go (TestNormalizeTitle_Cases + TestJaroWinkler_KnownPairs) lock the new public API surface.
- **8 anitaku.to + embed-wrapper + malsync goldens captured atomically** via `services/scraper/scripts/capture-gogoanime-goldens.sh`: the script fetches the 4 anitaku.to pages, greps `data-video` URLs out of `one_piece_episode_1.html`, dispatches each by host to the matching wrapper fetch (vibeplayer.site / otakuhg.site / otakuvid.online), fetches the malsync negative-cache exemplar, runs the sed anonymization sweep, and re-asserts the grep gate. Total upstream fetches: 8. Anonymization invariant verified clean (no Set-Cookie / __ddg2_ / cf_clearance / Bearer in any captured HTML/JSON).
- **GogoanimeConfig + SCRAPER_GOGOANIME_BASE_URL env var wired** — Config.Gogoanime.BaseURL reads the env var (default `https://anitaku.to`), rejects malformed URLs at boot with an error message naming the env var verbatim. 3 sub-tests cover default / override / invalid. docker/.env.example documents the override.
- **20 RED-state test scaffolds added** — 14 tests in `services/scraper/internal/providers/gogoanime/` (across client_test.go, dto_test.go, malsync_test.go, cache_test.go) + 7 tests in `services/scraper/internal/embeds/{vibeplayer,streamhg,earnvids}_test.go`. Every test SKIPs with a deterministic "RED — implementation arrives in Plan 18-0X" message. Each test that needs a fixture pre-checks the golden file path so a missing golden produces a loud `t.Fatal` (not a silent skip).

## Task Commits

Each task was committed atomically:

1. **Task 1: Annotate REQUIREMENTS.md + rename ROADMAP.md Phase 18** — `3460181` (docs)
2. **Task 2: Extract services/scraper/internal/fuzzy/ + migrate animepahe** — `799ca7e` (refactor)
3. **Task 3: Capture gogoanime goldens + capture script + Makefile target** — `016732b` (test)
4. **Task 4: GogoanimeConfig + SCRAPER_GOGOANIME_BASE_URL env var** — `f895c06` (feat)
5. **Task 5: RED-state test scaffolds for gogoanime + 3 embed extractors** — `336abb1` (test)

**Plan metadata + deviation fix:** `bd7f90c` (fix — anonymization gate scope)

## Files Created/Modified

### Created
- `services/scraper/internal/fuzzy/jarowinkler.go` — Exported `JaroWinkler` (verbatim relocation).
- `services/scraper/internal/fuzzy/normalize.go` — Exported `NormalizeTitle` (verbatim relocation).
- `services/scraper/internal/fuzzy/fuzzy_test.go` — Migrated season-fold + JW reference-pair coverage.
- `services/scraper/internal/providers/gogoanime/doc.go` — Package declaration so test scaffolds compile.
- `services/scraper/internal/providers/gogoanime/client_test.go` — 8 RED tests (FindID fuzzy/malsync, ListEpisodes sub/dub merge + cache, ListServers anime_muti_link + Turnstile filter, GetStream registry dispatch + TTL).
- `services/scraper/internal/providers/gogoanime/dto_test.go` — 3 RED tests (search/episode/server golden parse).
- `services/scraper/internal/providers/gogoanime/malsync_test.go` — 1 RED test (negative-cache forward-compat probe).
- `services/scraper/internal/providers/gogoanime/cache_test.go` — 1 RED test (StreamHG &e= TTL math).
- `services/scraper/internal/embeds/vibeplayer_test.go` — 2 RED tests (SSRF gate + regex extract).
- `services/scraper/internal/embeds/streamhg_test.go` — 3 RED tests (SSRF gate + packer extract + TTL).
- `services/scraper/internal/embeds/earnvids_test.go` — 2 RED tests (SSRF gate + packer extract).
- `services/scraper/testdata/gogoanime/{search_attack_on_titan,category_one_piece,category_one_piece_dub,one_piece_episode_1,vibeplayer_embed,streamhg_packed,earnvids_packed}.html` — 7 anitaku.to + embed-wrapper captures (2026-05-12, anonymized).
- `services/scraper/testdata/gogoanime/malsync_no_gogo.json` — negative-cache exemplar (no Gogoanime/Anitaku key).
- `services/scraper/testdata/gogoanime/README.md` — fixture catalog + anonymization invariant + refresh + upstream-death recovery protocol.
- `services/scraper/scripts/capture-gogoanime-goldens.sh` — atomic 8-fixture refresh.

### Modified
- `.planning/REQUIREMENTS.md` — Phase 18 SCRAPER-9ANI-* implementation-note annotation.
- `.planning/ROADMAP.md` — Phase 18 title rename in v3.0 summary list + detail header.
- `services/scraper/internal/providers/animepahe/cache.go` — Deleted local `jaroWinkler` + `normalizeTitle` bodies; kept `computeStreamTTL` + TTL constants.
- `services/scraper/internal/providers/animepahe/cache_test.go` — Removed `TestJaroWinkler` + `TestNormalizeTitle` (migrated to fuzzy_test.go).
- `services/scraper/internal/providers/animepahe/client.go` — Added `internal/fuzzy` import; swapped call sites to `fuzzy.NormalizeTitle` / `fuzzy.JaroWinkler`.
- `services/scraper/internal/config/config.go` — New `GogoanimeConfig` struct + `Config.Gogoanime` field + `SCRAPER_GOGOANIME_BASE_URL` env binding + URL validation.
- `services/scraper/internal/config/config_test.go` — `TestLoad_GogoanimeConfig_DefaultsAndOverride` (3 sub-tests).
- `Makefile` — New `capture-goldens-gogoanime` PHONY target.
- `docker/.env.example` — Documented Phase 18 `SCRAPER_GOGOANIME_BASE_URL` override.

## Decisions Made

- **Doc pivot in-place** (no requirement IDs renumbered) — SCRAPER-9ANI-01..06 keep their literal prefix; an annotation in REQUIREMENTS.md maps them to the Gogoanime/Anitaku implementation. This avoids cascading renames across STATE.md / probe-runner test golden pools / observability dashboards.
- **Promote fuzzy helpers to services/scraper/internal/fuzzy/** as the second consumer (Gogoanime) joins — pure code-motion, both packages now consume the same source. Avoids the alternative of duplicating Jaro-Winkler inside the gogoanime package.
- **Anonymization gate scoped to *.html + *.json** (not the whole testdata/gogoanime/ tree) so the README can document the forbidden patterns without self-failing the gate. The sed strip only touches *.html anyway.
- **RED scaffolds use t.Skip (not t.Fatal)** so the test suite stays green on main while Wave 2 plans land impl. Tests reference goldens via filepath.Join helpers; missing-golden fail loudly with `t.Fatal`.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Anonymization gate falsely matched the README that documents it**
- **Found during:** Plan-wide verification (after Task 5)
- **Issue:** The Task 3 spec defines the anonymization gate as `grep -rE '(Set-Cookie|__ddg2_|cf_clearance|Bearer )' services/scraper/testdata/gogoanime/` — recursive over the whole directory tree. The README.md (also under that tree) documents those forbidden patterns in plain text + code blocks, which causes the recursive grep to match the documentation itself.
- **Fix:** Scope the gate to `*.html` + `*.json` (matching what the sed strip operates on). Updated both the Makefile target and the capture script.
- **Files modified:** `Makefile`, `services/scraper/scripts/capture-gogoanime-goldens.sh`
- **Verification:** `grep -E '(Set-Cookie|__ddg2_|cf_clearance|Bearer )' services/scraper/testdata/gogoanime/*.html services/scraper/testdata/gogoanime/*.json` exits non-zero (no matches).
- **Committed in:** `bd7f90c` (separate fix commit after the 5 task commits)

---

**Total deviations:** 1 auto-fixed (Rule 1 — bug)
**Impact on plan:** Minor — the gate is now scoped correctly to data files, which matches the strip's actual operation. README documentation of the forbidden patterns is preserved. No scope creep.

## Issues Encountered

- None. All upstream hosts (anitaku.to, vibeplayer.site, otakuhg.site, otakuvid.online, api.malsync.moe) returned 2xx for the capture run on 2026-05-12 — no upstream-death pivot triggered.

## User Setup Required

None — `SCRAPER_GOGOANIME_BASE_URL` defaults to `https://anitaku.to` (the canonical mirror per 2026-05-12 research); no operator action required for Phase 18 to compile/run.

## Next Phase Readiness

### Ready for Wave 2 (18-02 + 18-03 parallel)
- **18-02 (Gogoanime provider package):** consumes `services/scraper/internal/fuzzy.{NormalizeTitle,JaroWinkler}`, `config.Gogoanime.BaseURL`, all 8 goldens, and the 14 RED tests in `services/scraper/internal/providers/gogoanime/`.
- **18-03 (3 embed extractors):** consumes 3 wrapper goldens + the 7 RED tests in `services/scraper/internal/embeds/{vibeplayer,streamhg,earnvids}_test.go`.

### Test gate state
- `go build ./services/scraper/...` — green.
- `go test ./services/scraper/... -count=1 -timeout=120s` — all packages PASS (20 SKIPs in the RED scaffolds are intentional and counted as PASS at the package level).
- `make capture-goldens-gogoanime` — operator-runnable, refreshes all 8 fixtures atomically, enforces anonymization gate.

### Wave 2 invariants exported by this plan
- `fuzzy.JaroWinkler(a, b string) float64` — Jaro-Winkler [0,1] similarity.
- `fuzzy.NormalizeTitle(s string) string` — season-fold + punctuation collapse.
- `config.GogoanimeConfig{BaseURL string}` — populated from `SCRAPER_GOGOANIME_BASE_URL` (default `https://anitaku.to`).
- 8 golden fixtures + capture script + Makefile target — refresh procedure documented.
- 20 SKIP-state tests — Wave 2 turns each into a real PASS-state assertion.

## Self-Check: PASSED

- All 21 created files exist on disk:
  - `services/scraper/internal/fuzzy/{jarowinkler,normalize,fuzzy_test}.go` — FOUND
  - `services/scraper/internal/providers/gogoanime/{doc,client_test,dto_test,malsync_test,cache_test}.go` — FOUND
  - `services/scraper/internal/embeds/{vibeplayer,streamhg,earnvids}_test.go` — FOUND
  - `services/scraper/testdata/gogoanime/{README.md,search_attack_on_titan.html,category_one_piece.html,category_one_piece_dub.html,one_piece_episode_1.html,vibeplayer_embed.html,streamhg_packed.html,earnvids_packed.html,malsync_no_gogo.json}` — FOUND
  - `services/scraper/scripts/capture-gogoanime-goldens.sh` — FOUND
- All 6 commits exist in `git log --oneline`: `3460181`, `799ca7e`, `016732b`, `f895c06`, `336abb1`, `bd7f90c` — all FOUND.

---
*Phase: 18-9anime*
*Completed: 2026-05-12*
