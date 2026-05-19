# Phase 28: Provider Expansion Round 2 — Context

**Gathered:** 2026-05-19
**Status:** Ready for planning (`/gsd-plan-phase --phase 28`)
**Milestone:** v3.1 Scraper Self-Healing (REOPENED 2026-05-19; Phase 28 added 2026-05-19 per operator request)
**Spec:** `.planning/milestones/v3.1-REQUIREMENTS.md` (SCRAPER-HEAL-34..39)
**Origin:** Operator request "as many providers as possible" — adds three sources surfaced in the 2026-05-19 EN-source survival sweep (`.planning/research/2026-05-19-en-source-survival.md`) plus 9anime.me.uk surfaced ad-hoc this session (not in original sweep — separate recon log below).

<domain>
## Phase Boundary

Grow the EN failover pool from 1 working provider (allanime, shipped Phase 26 Wave 1) to 4. Three new providers in priority order by reliability ceiling:

1. **AnimeFever** (`animefever.cc`) — clean HTML scrape against a PHP+Cloudflare-passive backend. Failover slot 4. Reliability MEDIUM.
2. **Miruro** (`miruro.tv`) — Vite SPA over an obfuscated proxy (`pro.ultracloud.cc`). Failover slot 5, gated on a 4-agent-session obfuscation reverse-engineering spike. Reliability MEDIUM if spike converges; DROPPED if it doesn't.
3. **9anime.me.uk** — WordPress 6.9.4 + dramastream theme; brand-jack of the dead aniwave ecosystem. Failover slot 6 (last-resort). Reliability LOW (~6-month half-life expected). MP4-only, no canonical ID mapping, title-fuzzy-match only.

**Concretely, this phase delivers:**

1. **AnimeFever lift (SCRAPER-HEAL-36)** — new `services/scraper/internal/providers/animefever/` package implementing the `domain.Provider` interface against `animefever.cc`. FindID via title-search-with-MalSync-fallback (no canonical ID exposed). ListEpisodes/ListServers scrape server-rendered HTML. GetStream delegates to embed-extractor registry (extractors added in SCRAPER-HEAL-38 if missing). Cookie-jar handled by existing `domain.BaseHTTPClient`.
2. **Miruro lift (SCRAPER-HEAL-37, conditional)** — new `services/scraper/internal/providers/miruro/` package. FindID via AniList-ID-direct (ARM lookup MAL → AniList because Miruro URL is `/anime/<anilist_id>`). All other methods route through the deobfuscated `pro.ultracloud.cc` JSON proxy. **Hard kill-switch on SCRAPER-HEAL-34 spike**: if the obfuscation transform can't be ported to pure Go inside 4 agent-sessions, Miruro is dropped from Phase 28 and SCRAPER-HEAL-37 rolls to v3.2.
3. **9anime.me.uk lift (SCRAPER-HEAL-39)** — new `services/scraper/internal/providers/nineanime/` package. FindID via title-fuzzy-match only (no MAL/AniList in URLs; no series-level entity — episodes are individual WP posts). Per-episode WP slug walking for episode enumeration. Stream extraction: iframe-then-MP4-source from `my.1anime.site/index.php?action=play&file=...`. HLS proxy allowlist update in `libs/videoutils/proxy.go` adds `my.1anime.site`. Documented as last-resort tier.
4. **Embed extractors (SCRAPER-HEAL-38)** — add the embed-host extractors that AnimeFever's recon (SCRAPER-HEAL-35) reveals are missing. Typical 2026 candidates: `streamwish.go`, `filelions.go`, `doodstream.go`. Each templated from `embeds/streamhg.go`. Golden-file tested.
5. **Spikes as artifacts (SCRAPER-HEAL-34, -35)** — both spikes produce written `SPIKE-*.md` artifacts (one per spike) with verdicts and either ship the work or kill the dependent plan. Spikes are NOT throwaway — the SPIKE files become permanent decision logs.
6. **Source dropdown polish (no new requirement, folded into Phase 28 housekeeping)** — `capitalizeProvider` branches for `animefever`/`miruro`/`nineanime`, i18n labels in en/ru/ja, Playwright e2e for source-switching mid-episode. The dropdown infrastructure shipped in Phase 24 SCRAPER-HEAL-17 (EnglishPlayer.vue restore); Phase 26 Wave 5 was supposed to activate it for AllAnime but skipped because Phase 24 didn't ship. Phase 28's housekeeping wave does it for all 4 providers at once.

**Out of scope:**

- Backfill of `has_english` column for never-touched anime (v3.2 polish item — opportunistic-only stays per Phase 26 D5).
- WARP egress sidecar (revives VibePlayer; v3.2+ separate spec).
- MinIO segment archival (v3.2+ separate spec).
- Reviving gogoanime, animepahe, animekai (separate phases: 24, 27, 26-06 respectively).
- 9anime's broken WP search — we do not attempt to fix or work around the upstream's broken `?s=` results. Title-fuzzy is implemented against the WP REST API (`/wp-json/wp/v2/search`) instead; if that endpoint also returns garbage, 9anime degrades to "manually-curated slug map" — operator override expected.

**Requirements covered:** SCRAPER-HEAL-34, SCRAPER-HEAL-35, SCRAPER-HEAL-36, SCRAPER-HEAL-37, SCRAPER-HEAL-38, SCRAPER-HEAL-39.

</domain>

<decisions>
## Implementation Decisions

### D1 — Ship-order by reliability ceiling, not by alphabetical or by upstream-popularity

AnimeFever → Miruro → 9anime. Reasoning: each plan that ships independently raises floor reliability of the EN player; the LOWEST-reliability source slot should land LAST so we have a clean rollback story ("kill Phase 28's last commit") if 9anime causes operational pain. This also matches the failover-chain ordering: `gogoanime → animepahe → allanime → animefever → miruro → nineanime → animekai`. Higher-slot positions get probed less often in the steady state, which protects the noisier upstreams from generating noise during normal operation.

### D2 — 9anime.me.uk is explicitly accepted as a low-quality, last-resort source

Recon evidence:
- Brand-jacking WordPress (not original 9anime/aniwave ecosystem)
- No `anime`/`series`/`episode` custom post types in WP REST API — only stock `post,page,attachment`
- Search broken (`?s=frieren` returns 19 irrelevant "episode 7" stubs)
- Canonical test target Frieren is absent from upstream catalog (HTTP 404)
- Episode-7 page found embedding episode-6 MP4 file — data quality is mid
- Player extraction works but routes through a separate brand-jack domain (`my.1anime.site`)

**Operator policy** ("as many providers as possible") explicitly overrides the natural "not-worth" verdict. The trade-off is captured here so a future maintainer understands why a brittle, brand-jacking source was accepted. The operational mitigation is failover-chain slot 6 (LAST) — 9anime is probed only when all 5 prior providers fail. When 9anime breaks (estimated 6-month half-life), the operator response is `SCRAPER_DEGRADED_PROVIDERS=nineanime` and no replanning is needed.

Alternate test target for 9anime: Marriagetoxin episode 1 (`/marriagetoxin-episode-1-english-subbed/` if it exists, else episode 7 which is confirmed present). Frieren is not usable for 9anime canary because the upstream doesn't carry it.

### D3 — Miruro spike has a hard 4-agent-session kill-switch; failure does NOT block the phase

The obfuscation transform reverse-engineering is bounded effort (E=34 on the Fibonacci scale per `.planning/CONVENTIONS.md`). If 4 agent-sessions of focused work don't yield a clean Go port (i.e., no `utls`/`chromedp` dependency surfaces required, and `pro.ultracloud.cc` returns playable HLS from the prod IP), then SCRAPER-HEAL-37 is marked `killed` in the spike artifact and rolls to v3.2. The remaining Phase 28 work (AnimeFever, 9anime, extractors, dropdown polish) is wholly independent and proceeds.

The kill-switch is workflow time-box, NOT an effort estimate — per project convention, plan-level effort lives in CDI's `E` factor (right side of the `*`).

Spike convergence criteria (gate to advance to SCRAPER-HEAL-37):
1. The transform from `(endpoint, OBF_KEY)` → obfuscated URL is implementable in pure Go using only `crypto/hmac`, `crypto/sha256`, `crypto/aes`, or `encoding/base64` (i.e., no TLS-fingerprinting required).
2. A test call against `pro.ultracloud.cc/<transformed-url>` from the production server returns HTTP 200 with parseable HLS / JSON.
3. The obfuscation keys (`VITE_PROXY_OBF_KEY=a54d389c18527d9fd3e7f0643e27edbe`, `VITE_PIPE_OBF_KEY=71951034f8fbcf53d89db52ceb3dc22c`) appear stable across at least 3 sequential page-load fetches of `env2.js` (i.e., not rotated per session).
4. A spot-check against the Frieren AniList ID (154587) returns a non-error episode listing.

Any gate fails → spike `killed`, SCRAPER-HEAL-37 rolls.

### D4 — AnimeFever's embed-extractor recon (SCRAPER-HEAL-35) ships its own spike artifact

We do not pre-implement embed extractors. The recon spike fetches one Frieren episode page on AnimeFever, identifies the embed iframe host(s), and emits an ordered list of `embeds/<host>.go` files to write in SCRAPER-HEAL-38. This keeps the implementation tightly scoped — we don't speculatively write a `streamwish.go` if AnimeFever proxies to a host we already have.

If AnimeFever's embeds are 100% covered by existing extractors (earnvids/kwik/megacloud/packed_common/streamhg/vibeplayer), SCRAPER-HEAL-38 is empty and trivially completes.

### D5 — Failover-chain ordering is locked in CONTEXT.md and enforced in `main.go` Register order

The orchestrator probes providers in registration order. Per D1:

```
1. gogoanime          (degraded — kept as recovery target)
2. animepahe          (degraded — kept as recovery target; revived in Phase 27)
3. allanime           ★ working — primary EN (shipped Phase 26 Wave 1)
4. animefever         ← NEW (Phase 28) — primary fallback
5. miruro             ← NEW (Phase 28) — secondary fallback (conditional)
6. nineanime          ← NEW (Phase 28) — last-resort
7. animekai           (gated stub; flipped on if Phase 26 SCRAPER-HEAL-27 converges)
```

`SCRAPER_SERVER_PRIORITY` env override can re-order without redeploying; default is the chain above.

### D6 — Test targets per provider follow `feedback_verify_streams.md`

Test the user's ACTUAL working data, not just synthetic fixtures:
- AnimeFever: Frieren (MAL 52991, AniList 154587)
- Miruro: Frieren (AniList 154587)
- 9anime: Marriagetoxin episode 1 OR 7 (Frieren absent from upstream catalog)
- All providers also exercised through the Phase 23 daily canary cron once shipped

Each provider's `Frieren E2E gate` is a hard ship gate — the plan does NOT mark complete until the curl pipeline against the canary target passes through the gateway → catalog → scraper → provider stack.

### D7 — HLS proxy allowlist additions land per-provider, not in a single batch

`libs/videoutils/proxy.go` allowlist grows in the same commit that ships the provider it serves. Reason: rollback is per-provider. If 9anime is killed via `SCRAPER_DEGRADED_PROVIDERS`, the allowlist entries it added stay (they're harmless — Phase 25 SCRAPER-HEAL-24 makes "domain not allowed" return HTTP 502 anyway), but they're traceable to the provider that needed them. Per-provider PRs make audit clean.

New allowlist entries expected:
- AnimeFever: TBD per SCRAPER-HEAL-35 recon (likely `streamwish.com`, `filelions.to`, `dood.li`)
- Miruro: `pro.ultracloud.cc`, `pru.ultracloud.cc` (proxy hosts; HLS CDN hosts behind them surface at runtime per stream)
- 9anime: `my.1anime.site` (iframe + MP4 host)

</decisions>

<open_questions>
- **Is 4 agent-sessions a tight timebox for the Miruro spike?** Spike CDI is `0.02 * 34` — the E=34 anchor calibrates against "research + significant phase-of-work." Workflow timebox sized accordingly; bump to 6 sessions if operator wants more rope (override D3 to read "6 agent-sessions").
- **Should 9anime's title-fuzzy include a Shikimori/MAL pre-resolution step?** Default: yes — the `AnimeRef` already carries Shikimori ID, and we fetch the Russian + English titles from catalog to feed the fuzzy match. Implementation overhead is minimal.
- **Source dropdown order in UI**: should the order in the dropdown match the failover-chain order (allanime → animefever → miruro → nineanime), or alphabetical, or by user preference (last-used first)? Default: failover-chain order; matches user expectation of "primary first."
</open_questions>

<risks>
## Risks specific to this phase

- **Miruro spike doesn't converge** — explicit kill-switch in D3 makes this a controlled outcome, not a phase blocker. Cost: spike CDI `0.02 * 34` worth of work absorbed without shipping the dependent plan; Phase 28 ships with 2 new providers instead of 3 (UXΔ ceiling drops from `+3` to `+2`).
- **AnimeFever rebrands its DOM selectors** — HTML scraping is fragile. Mitigation: per CLAUDE.md and Phase 23 canary infrastructure, the daily canary catches selector breakage within 24h and the maintenance bot Pattern 7 dispatch handles known-class fixes (Phase 25 SCRAPER-HEAL-21 ships this loop). For DOM changes the bot can't classify, Telegram fires an alert and operator intervenes.
- **9anime.me.uk DMCAs / migrates / rebrands** — high-probability event (~6-month half-life). Mitigation: D2's documented trade-off + operator kill via `SCRAPER_DEGRADED_PROVIDERS=nineanime`. No replanning needed; 9anime stays in the codebase as inert provider until a future cleanup phase removes it.
- **Embed extractor recon (SCRAPER-HEAL-35) reveals a host we can't extract** (e.g., heavily-obfuscated JS player) — Mitigation: the recon spike's verdict says either "implementable as `<host>.go`" or "blocked — falls back to remaining hosts AnimeFever proxies to." If ALL of AnimeFever's hosts are blocked, SCRAPER-HEAL-36 ships with degraded coverage (search/list/servers work, GetStream fails on un-extractable hosts). Frieren E2E gate would catch this and force a re-spike.
- **9anime title-fuzzy false positives** (matches wrong anime) — Mitigation: include MAL/Shikimori metadata (year, episode count) in the fuzzy-match scoring; reject matches below confidence threshold. If catalog has both EN and JP titles, score both and take the higher. Documented uncertainty as a CONTEXT.md side-note in the 9anime provider's `doc.go`.
- **Failover chain length increases per-request latency** — orchestrator probes providers in order; longer chain = more wall-clock when early providers fail. Mitigation: Phase 21's hard ≤8s budget is per-server, not per-provider; each provider's first-server probe is bounded; chain length doesn't blow the budget for ≤8 providers each with first-server-probe ≤1s.
- **9anime's `my.1anime.site` MP4 host has no HLS** — frontend `EnglishPlayer.vue` assumes HLS-first; MP4 path needs verification. Pre-existing Phase 16 AnimePahe path uses MP4-via-Kwik so the player already supports MP4; the path just needs exercise. Documented as Plan 28-05 verification step.
</risks>

<dependencies>
## Phase Dependencies

- **Hard dependency on:** Phase 15 (scraper microservice + Provider interface + embed registry; v3.0 shipped). Phase 26 Wave 1 (AllAnime provider template — Phase 28's three providers all clone its shape, so the template MUST be live).
- **Soft dependency on:** Phase 24 (EnglishPlayer.vue restored — only needed for the source-dropdown UI polish in 28-06). If Phase 24 hasn't shipped by the time 28-06 lands, the backend providers are still operational and the dropdown polish stays pending — does NOT block Phase 28 closure.
- **Soft dependency on:** Phase 25 SCRAPER-HEAL-24 (HLS proxy "domain not allowed" → 502 mapping). Phase 28 adds allowlist entries; the 502 mapping ensures un-allowlisted hosts fail loudly. Already shipped 2026-05-19.
- **Soft dependency on:** Phase 25 SCRAPER-HEAL-21 (maintenance-bot Pattern 7 allowlist self-heal). The bot's coverage extends to Phase 28's new allowlist entries as soon as they land; no Phase 28 work needed to wire that in.
- **No dependency on:** Phase 27 (AnimePahe revival) — both phases run independently in v3.1.
- **Blocks:** v3.1 final milestone audit (`/gsd-audit-milestone`). Once Phase 28 ships (with or without Miruro converging), all 39 SCRAPER-HEAL-* requirements are accounted for.

</dependencies>

<plan_sketch>
## Plan Sketch (for `/gsd-plan-phase` to flesh out)

**Wave 0 — Spikes (parallel)**

- `28-00-PLAN.md` — Miruro obfuscation spike (SCRAPER-HEAL-34). Workflow timebox: 4 agent-sessions kill-switch. Output: `.planning/phases/28-provider-expansion-r2/SPIKE-MIRURO.md` verdict (`converged` | `killed`) + Go reference implementation of the OBF transform if converged.
  - `UXΔ = 0 (Ambiguous)` · `CDI = 0.02 * 34` · `MVQ = Basilisk 75%/90%`
- `28-01-PLAN.md` — AnimeFever embed-extractor recon (SCRAPER-HEAL-35). Output: `.planning/phases/28-provider-expansion-r2/SPIKE-ANIMEFEVER.md` with ordered embed-host list and `existing-registry | needs-new-extractor` classification per host.
  - `UXΔ = 0 (Ambiguous)` · `CDI = 0.01 * 3` · `MVQ = Sprite 60%/85%`

**Wave 1 — AnimeFever (parallel)**

- `28-02-PLAN.md` — AnimeFever provider lift (SCRAPER-HEAL-36). New `services/scraper/internal/providers/animefever/` package. FindID via title-search-with-MalSync-fallback. ListEpisodes/ListServers via HTML scrape. GetStream delegates to embed registry. Cookie-jar handled by existing `BaseHTTPClient`. Registered in `main.go` failover slot 4. Frieren E2E gate.
  - `UXΔ = +2 (Better)` · `CDI = 0.02 * 13` · `MVQ = Griffin 85%/80%`
- `28-03-PLAN.md` — New embed extractors (SCRAPER-HEAL-38). Conditional on 28-01's recon. Each `embeds/<host>.go` templated from `embeds/streamhg.go`; golden-file tested. If 28-01 returns empty list (all hosts already covered), 28-03 ships as a no-op.
  - `UXΔ = +1 (Better)` · `CDI = 0.015 * 8` · `MVQ = Sprite 70%/80%`

**Wave 2 — Miruro (conditional on Wave 0 verdict)**

- `28-04-PLAN.md` — Miruro provider lift (SCRAPER-HEAL-37). Conditional on `28-00` verdict `converged`. New `services/scraper/internal/providers/miruro/` package. FindID via AniList-ID-direct (ARM lookup). All upstream calls route through the deobfuscated `pro.ultracloud.cc`. May add `services/scraper/internal/embeds/ultracloud.go` if stream-extraction requires it. Registered in `main.go` failover slot 5. Frieren E2E gate.
  - `UXΔ = +2 (Better)` · `CDI = 0.04 * 21` · `MVQ = Phoenix 70%/85%`

**Wave 3 — 9anime + Polish (parallel)**

- `28-05-PLAN.md` — 9anime.me.uk provider lift (SCRAPER-HEAL-39). New `services/scraper/internal/providers/nineanime/` package. FindID via title-fuzzy-match using Shikimori/MAL metadata for scoring. ListEpisodes walks per-episode WP slugs. GetStream extracts iframe → MP4 source from `my.1anime.site`. Allowlist update in `libs/videoutils/proxy.go` adds `my.1anime.site`. Registered LAST in failover chain (slot 6). Alternate test target: Marriagetoxin episode 1 or 7.
  - `UXΔ = 0 (Ambiguous)` · `CDI = 0.075 * 13` · `MVQ = Basilisk 40%/30%`
- `28-06-PLAN.md` — Source dropdown polish + Playwright e2e + `/animeenigma-after-update`. `capitalizeProvider` branches for animefever/miruro/nineanime in `EnglishPlayer.vue`. i18n keys in `frontend/web/src/locales/{en,ru,ja}.json`. Playwright e2e for source switching mid-episode. After-update skill ships changelog entry + commit (3 co-authors) + push.
  - `UXΔ = +3 (Better)` · `CDI = 0.015 * 5` · `MVQ = Sprite 88%/92%`

**Phase-level aggregate** (Miruro converges): `UXΔ ≈ +1.4 (Better, weighted by Wave 3 user-facing plans)` · `CDI cumulative ≈ 0.195 across 6 plans, weighted-E ≈ 13` · `MVQ-mix = Basilisk + Sprite + Griffin + Phoenix — a coherent "transformation under tension" zoo`.

**Wave gates:**

- Wave 0 → Wave 1: trivial — recon spikes only feed Wave 1's 28-03; Wave 1's 28-02 can start in parallel with Wave 0.
- Wave 1 → Wave 2: not a gate — Wave 2's only dependency is on Wave 0's `28-00` verdict.
- Wave 2 → Wave 3: not a gate — Wave 3 is independent of Miruro outcome.
- Wave 3 → Phase complete: all-or-nothing on the 6 plans (with 28-04 SKIPPED-clean acceptable per D3 kill-switch).

</plan_sketch>

<verification_targets>
## Frieren E2E gate per provider

(per `feedback_verify_streams.md` — test the actual production target, not just goldens)

| Provider | Test target | Curl command (through gateway) |
|----------|-------------|--------------------------------|
| animefever | Frieren MAL 52991 | `curl -s 'http://localhost:8000/api/anime/f0b40660-6627-4a59-8dcf-7ec8596b3623/scraper/episodes?provider=animefever' \| jq '. \| length'` → `≥ 28` |
| miruro (conditional) | Frieren AniList 154587 | `curl -s 'http://localhost:8000/api/anime/f0b40660-6627-4a59-8dcf-7ec8596b3623/scraper/episodes?provider=miruro' \| jq '. \| length'` → `≥ 28` |
| nineanime | Marriagetoxin ep 1 | `curl -s 'http://localhost:8000/api/anime/<marriagetoxin-uuid>/scraper/episodes?provider=nineanime' \| jq '. \| length'` → `≥ 1` |

`/scraper/health` shows `up:true` across all 5 stages (search, episodes, servers, stream, stream_segment) for each shipped provider within 5 minutes of redeploy.

</verification_targets>
