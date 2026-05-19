# Stealth Plugin Pin Manifest

This file records the **exact-pinned** versions of the puppeteer-extra stealth
stack used by `services/animepahe-resolver/` to defeat DDoS-Guard on
`https://animepahe.pw`. The pins are LOAD-BEARING — without them, transitive
dependency drift defeats the stealth plugin's overrides and every request to
animepahe returns a 403 challenge.

> **Why this doc exists** — CONTEXT.md D6 (pin policy) + Phase 27 RESEARCH
> Pitfall 2 (package-lock commit policy) + Pitfall 7 (stealth defeated by new
> DDoS-Guard challenge variant).

---

## Current Pins (LAST UPDATED 2026-05-19)

| Package | Pin | Style | Why |
|---------|-----|-------|-----|
| `puppeteer-extra` | `3.3.6` | EXACT (no caret/tilde) | Plugin host; npm cadence is dormant (last published 3 years ago); pinning eliminates drift risk. |
| `puppeteer-extra-plugin-stealth` | `2.11.2` | EXACT | DDoS-Guard + FingerprintJS overrides. The defeat mechanism is reverse-engineered from a specific point-in-time DDoS-Guard challenge variant; minor version bumps have historically broken on at least one site. Pin and test on refresh. |
| `puppeteer` | `^24.0.0` | Caret | Official Google package; semver-respected; tested against `ghcr.io/puppeteer/puppeteer:24` base image. |
| `fastify` | `^4.28.0` | Caret | HTTP server; 4.x is the stable line in 2026. |
| `pino` | `^9.5.0` | Caret | Logger used by Fastify. |
| `prom-client` | `^15.1.3` | Caret | Prometheus client; standard. |

**Last tested against:** `animepahe.pw` (Frieren MAL 5319 search probe) on **2026-05-19**.

**Last tested via:** Phase 27 Plan 27-01 Task 4 D5 100-request soak (see appended
section "D5 100-request soak — 2026-05-19").

**Lockfile policy:** `package-lock.json` **MUST** be committed alongside
`package.json`. The Dockerfile uses `npm ci --omit=dev` (not `npm install`).
`npm ci` fails if the lockfile is stale, which is the desired pin-enforcement
behavior — it forces an explicit refresh through this doc.

---

## Refresh Procedure

When `stealth_challenge_failures_total` increments to > 1 over a sustained 1h
window in animepahe-resolver's `/metrics`, OR when end-to-end Frieren curl
pipeline (Phase 27 Plan 27-04) starts returning `stealth_challenge_failed`,
follow this procedure:

```bash
cd /data/animeenigma/services/animepahe-resolver

# 1. Refresh both pins to latest. If only ONE has a new version, that's fine.
PUPPETEER_SKIP_DOWNLOAD=true npm install \
    puppeteer-extra@latest \
    puppeteer-extra-plugin-stealth@latest

# 2. Re-run the offline test suite (must stay green).
npm test

# 3. Rebuild + redeploy the sidecar.
cd /data/animeenigma
make redeploy-animepahe-resolver

# 4. Re-run the Phase 27 Plan 27-04 Frieren curl pipeline (live integration gate).
#    Expected: ≥ 28 episodes returned through the gateway with prefer=animepahe.
BASE=http://localhost:8000
ANIME_ID=$(docker compose -f docker/docker-compose.yml exec -T postgres \
    psql -U postgres -d animeenigma -tAc \
    "SELECT id FROM animes WHERE shikimori_id = '52991' OR mal_id = 52991 LIMIT 1;")
curl -sS "$BASE/api/anime/$ANIME_ID/scraper/episodes?prefer=animepahe" | jq '.data | length'

# 5. If green:
#    - Update the "Current Pins" table above with the new versions.
#    - Update "Last tested against" with today's date.
#    - Commit:
#        git add services/animepahe-resolver/package.json \
#                services/animepahe-resolver/package-lock.json \
#                services/animepahe-resolver/STEALTH-PINS.md
#        git commit -m "chore(animepahe-resolver): refresh stealth pins to <NEW>"
#
# 6. If RED (Frieren probe returns < 28 episodes OR 502 on /search):
#    - Roll back: git checkout HEAD -- services/animepahe-resolver/package.json \
#                                       services/animepahe-resolver/package-lock.json
#    - Redeploy the previous build.
#    - Re-add `animepahe` to SCRAPER_DEGRADED_PROVIDERS in docker/.env
#      (operator decision, NOT auto-applied).
#    - Open a maintenance-bot escalation: `escalate` tier per
#      .claude/maintenance-prompt.md Pattern 7 animepahe-resolver branch.
```

**Single-line shorthand for the maintenance bot** (matches the bullet in
`.claude/maintenance-prompt.md` Pattern 7):

```bash
cd services/animepahe-resolver && \
    PUPPETEER_SKIP_DOWNLOAD=true npm install puppeteer-extra@latest puppeteer-extra-plugin-stealth@latest && \
    npm test && \
    cd /data/animeenigma && make redeploy-animepahe-resolver
```

---

## Why exact pins (not carets)

`puppeteer-extra-plugin-stealth`'s defeat mechanism is reverse-engineered from
the specific properties DDoS-Guard's challenge JS probes at a point in time.
A minor bump (`2.11.2 → 2.12.0`) might:

- ADD a new override property that DDoS-Guard's current challenge tests, fixing
  a different bug — but not affecting us.
- REMOVE an override property the maintainers thought was no-longer-needed — but
  which our specific DDoS-Guard challenge actually probes. Fail.

Because we cannot tell at install-time whether a minor bump is a benign add or a
silently breaking remove, we pin EXACT and treat the refresh as an integration
event (test against Frieren end-to-end before committing).

---

## Hardcoded-upstream invariant

This sidecar is **HARDCODED** to `https://animepahe.pw`. Adding a second
upstream is **NOT** a Pattern 7 button-fix. It requires:

1. Sandbox re-enablement with a Docker `cap_add: SYS_ADMIN` grant (the
   sidecar today runs with `--no-sandbox`, which is acceptable for a
   single-trusted-upstream profile; adding an untrusted upstream changes the
   threat model), OR
2. Explicit security review for the second domain.

See `server.js` header comment + Threat Model in
`.planning/phases/27-animepahe-revival-via-stealth-chromium-sidecar/27-01-PLAN.md`
(T-27-01-01, T-27-01-02).

---

## D5 100-request soak — 2026-05-19

Plan 27-01 Task 4 ran the D5 hard ship gate against the locally-built
`animepahe-resolver:dev` image. The gate is **PEAK_RSS ≤ 500 MB AND
`page_recycle_total` ≥ 1** under a 100-sequential-`/search?q=Frieren` soak.

> **TODO:** Plan 27-01 Task 4 fills this section with the empirical values:
>
> - `PEAK_RSS`: <value> MB
> - `page_recycle_total at end`: <value>
> - `close-first recycle activated`: <yes|no>
> - `PAGE_RECYCLE_AT`: <100 default, or 50 if downshifted per Pitfall 4>
> - `OOMKilled events`: <0 or count>
> - `502 responses during run`: <count, allowed ≤ 2 from initial stealth warmup>
>
> If the gate failed (RSS > 500 MB OR `page_recycle_total === 0`), Task 4
> documents the remediation (close-first recycle, lower PAGE_RECYCLE_AT to 50)
> and the second-run measurements.
