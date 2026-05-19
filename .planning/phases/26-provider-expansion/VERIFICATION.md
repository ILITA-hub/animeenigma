# Phase 26 — Provider Expansion: Verification

**Date:** 2026-05-19
**Status:** human_needed
**Milestone:** v3.1 Scraper Self-Healing (REOPENED 2026-05-19)
**Requirements:** SCRAPER-HEAL-25, SCRAPER-HEAL-26, SCRAPER-HEAL-27, SCRAPER-HEAL-28

## Plan dispositions

| Plan | Wave | Status | Notes |
|---|---|---|---|
| 26-01 — AllAnime lift | 1 | COMPLETED | Live in scraper failover; 28 Frieren episodes returned end-to-end. |
| 26-02 — has_english + filter | 1 | COMPLETED | Column auto-migrated, lazy backfill confirmed, filter narrows listing. |
| 26-03 — 2026 EN survival sweep | 2 | PARTIAL | Verdict matrix + PoC sketches complete; **decision gate awaiting operator input**. |
| 26-04 — Survivor #1 impl | 3a | DEFERRED | Gated on 26-03 Decision Gate operator pick. |
| 26-05 — Survivor #2 impl | 3b | DEFERRED | Gated on 26-03 Decision Gate operator pick. |
| 26-06 — AnimeKai recovery | 4 | DEFERRED | 7-day R&D budget; not run in this autonomous batch (CONTEXT.md D3 escape-hatch acceptable). |
| 26-07 — Dropdown polish | 5 | SKIPPED | `EnglishPlayer.vue` missing — cross-phase dependency on Phase 24. |

## Wave 1 verification (COMPLETED)

### AllAnime registered + reachable

```
$ curl -sf http://localhost:8088/scraper/health | jq '.data.providers.allanime.provider'
"allanime"

$ curl -sf http://localhost:8088/scraper/health | jq '.data.providers.allanime.stages | keys'
[ "episodes", "search", "servers", "stream", "stream_segment" ]
```

All 5 canonical stages present. End-to-end smoke against Frieren
(UUID `f0b40660-6627-4a59-8dcf-7ec8596b3623`):

```
$ curl -s 'http://localhost:8000/api/anime/f0b40660-.../scraper/episodes?provider=allanime' | jq '.data.episodes | length'
28
```

### has_english column + browse filter

```
$ docker compose exec -T postgres psql -U postgres -d animeenigma -c '\d animes' | grep has_english
has_english | boolean | | | false
idx_animes_has_english | btree (has_english)

$ # After one curl to allanime episodes:
$ docker compose exec -T postgres psql -U postgres -d animeenigma -c "SELECT has_english FROM animes WHERE shikimori_id='52991';"
 has_english
-------------
 t

$ curl -s 'http://localhost:8000/api/anime?providers=english&page_size=5' | jq '.data | length'
1
```

Filter narrows the listing correctly.

## Wave 2 (26-03) — verdict matrix

| Candidate | Status | Recommendation |
|---|---|---|
| Miruro | live | needs-deeper-PoC (HIGH RISK — obfuscation reverse-engineering required) |
| AnimeOwl | dead | not-worth (explainer page; legacy domains 404/parking) |
| AnimeFever | live | needs-deeper-PoC (HTML scraping, PHP backend; low risk, high maintenance burden) |
| AniWatchTV | dead | not-worth (Cloudflare 404 from prod; consistent with USTR shutdown claim) |
| HiAnime | dead | not-worth (7 mirror domains evaluated; literal "goodbye" body on .nz) |
| Crunchyroll-Free | live | not-worth (CF challenge + login wall + legal disqualifier) |

Sweep summary: 0 worth-implementing / 2 needs-deeper-PoC / 4 not-worth.

**Operator decision gate:** UNFILLED. See
`.planning/research/2026-05-19-en-source-survival.md` § Decision Gate.

## Wave 5 (26-07) — dependency check

`frontend/web/src/components/player/EnglishPlayer.vue` does not exist on
disk. Phase 24 (SCRAPER-HEAL-17) owns its restoration. Phase 26 Wave 1
delivered AllAnime as the recovery provider that unblocks Phase 24, but
restoring EnglishPlayer.vue is itself Phase 24 work. 26-07 is SKIPPED
cleanly; SCRAPER-HEAL-28 stays open as BLOCKED-ON-PHASE-24.

## Deferred plans

- **26-04 / 26-05** — Conditional on the 26-03 Decision Gate. If operator
  picks one survivor, only 26-04 executes; if two, 26-05 also executes;
  if `research-only`, both are SKIPPED.
- **26-06** — Has a 7-day R&D budget that exceeds this autonomous batch.
  Per CONTEXT.md D3, AnimeKai recovery is allowed a second escape-hatch:
  if R&D doesn't converge, SCRAPER-HEAL-27 stays open and AnimeKai
  remains flag-default-off. Not a Phase 26 failure.

## Requirements coverage

| Requirement | Status | Plan |
|---|---|---|
| SCRAPER-HEAL-25 | DELIVERED | 26-01, 26-02 |
| SCRAPER-HEAL-26 | PARTIAL (research + gate) | 26-03 |
| SCRAPER-HEAL-27 | DEFERRED | 26-06 |
| SCRAPER-HEAL-28 | BLOCKED-ON-PHASE-24 | 26-07 |

## Commits on `main`

```
$ git log --oneline -3
f53f30a docs(research): 2026 EN-source survival sweep (Phase 26 SCRAPER-HEAL-26)
4ee73ed feat(browse): add has_english column + English provider filter
99c80c5 feat(scraper): lift AllAnime into failover pool as third EN provider
```

## Why status = `human_needed`

Per the autonomous-run plan, this batch STOPS at the 26-03 Decision Gate
(CONTEXT.md D2 — hard operator gate before survey-candidate implementation).
Without operator selection, Wave 3a/3b cannot proceed. This is the
documented expected outcome.

Operator next steps:

1. Read `.planning/research/2026-05-19-en-source-survival.md`.
2. Pick 0, 1, or 2 survivors (Miruro and/or AnimeFever, or neither).
3. Fill the Decision Gate section + commit.
4. Re-invoke `/gsd-execute-phase 26 --plan 26-04` (and 26-05 if two picks),
   OR close SCRAPER-HEAL-26 as `research-only`.

Optionally, queue 26-06 separately when a 7-day R&D window opens.
