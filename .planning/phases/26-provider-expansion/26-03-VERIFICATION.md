# 26-03 Verification — 2026 EN-Source Survival Sweep

**Date:** 2026-05-19
**SCRAPER-HEAL-26** — Survey + decision gate.

## Artifact

`.planning/research/2026-05-19-en-source-survival.md` — committed alongside
this verification log.

## Probe inventory

| Candidate | Probe HTTP status | Verdict |
|---|---|---|
| Miruro (.tv/.to) | 200 (frontend), `{"error":"Gone"}` on API | live → needs-deeper-PoC |
| AnimeOwl (.cc/.live + 4 others) | mix (200 explainer, 404, 000) | dead → not-worth |
| AnimeFever (.cc) | 200 (HTML) | live → needs-deeper-PoC |
| AniWatchTV (.to/.tv) | timeout / 404 | dead → not-worth |
| HiAnime (.nz/.io/.so/.to/.tv/.pe/.bz) | mix (200 goodbye-message / 200 unrelated / 404 / 000) | dead → not-worth |
| Crunchyroll-Free | 403 + CF challenge | live but legally + technically not-worth |

## Sweep summary

- 6 candidates evaluated
- 0 `worth-implementing`
- 2 `needs-deeper-PoC` (Miruro, AnimeFever)
- 4 `not-worth`

## Status

**HALTED at operator decision gate.** Per the autonomous-run plan, this
agent stopped at Task 4 of 26-03-PLAN.md without filling the Decision
Gate. Wave 3a / 3b (26-04, 26-05) **DO NOT EXECUTE** until the operator:

1. Reads `.planning/research/2026-05-19-en-source-survival.md` end-to-end.
2. Fills the **Decision Gate** section with one of:
   - Pick Miruro (spike-gated; high risk)
   - Pick AnimeFever (HTML-scraping-tolerable)
   - Pick both
   - Pick neither (`research-only`)
3. Commits the filled gate.

## Compliance

- [x] Artifact location: `.planning/research/` (NOT under phase dir)
- [x] Methodology section: curl-based, no anti-bot tooling
- [x] Each candidate has probe evidence (command + HTTP status + body excerpt)
- [x] AniWatchTV verdict references the rumoured March 2026 USTR claim
- [x] HiAnime verdict covers each known mirror domain (.nz/.io/.so/.to/.tv/.pe/.bz)
- [x] Decision Gate template present with operator-input placeholders
- [x] Sweep summary numbers visible inline
