# Plan 26-03 — 2026 EN-Source Survival Sweep

**Status:** PARTIAL (probes + verdict matrix + PoC sketches complete;
operator decision gate awaiting input)
**Requirement:** SCRAPER-HEAL-26
**Wave:** 2 (autonomous: false)
**Date:** 2026-05-19

## What shipped

Research artifact at `.planning/research/2026-05-19-en-source-survival.md`
covering 6 EN-source candidates with curl probe evidence, anti-bot
posture summary, recommendation, and PoC sketches for the two
`needs-deeper-PoC` candidates.

## Verdict highlights

- **Miruro** — frontend live but documented API gone; backend is
  obfuscation-routed through `pro.ultracloud.cc`. `needs-deeper-PoC`,
  5–7 day estimate, **HIGH RISK** (obfuscation reverse-engineering may
  not converge).
- **AnimeFever** — HTML-scraping path; PHP backend with session cookies;
  no JS challenge from prod IP. `needs-deeper-PoC`, 4–6 day estimate,
  **LOW RISK but high maintenance burden**.
- **AnimeOwl** — dead (`.cc` is explainer page, others 404/parking).
- **AniWatchTV** — dead (timeout / Cloudflare 404 from prod, consistent
  with the March 2026 USTR-related shutdown rumour).
- **HiAnime** — dead across all 7 known mirror domains (literal "goodbye"
  body on `.nz`; squatter/aggregator on `.io`; placeholders elsewhere).
- **Crunchyroll-Free** — `not-worth` for both technical (CF challenge +
  login wall) and legal (ToS) reasons.

## Decision gate

**HALTED**. Per the autonomous-run plan, the agent did NOT autonomously
fill the gate — Wave 3 plans (26-04, 26-05) wait for the operator to
pick 0–2 survivors. Default-on-7-day-silence per CONTEXT.md D2:
`research-only`.

## Files modified

- `.planning/research/2026-05-19-en-source-survival.md` — new file, full
  sweep + Decision Gate template.
- `.planning/phases/26-provider-expansion/26-03-VERIFICATION.md` — new
  file, probe inventory + sweep summary + compliance checklist.

## Verification

See `26-03-VERIFICATION.md`. SCRAPER-HEAL-26 acceptance: ships when
the operator either picks survivors (one of {Miruro, AnimeFever, both})
OR picks `research-only`.

## Not done

- Decision Gate not filled (operator gate, by design).
- 26-04 and 26-05 plans NOT executed (gated on operator's pick).
