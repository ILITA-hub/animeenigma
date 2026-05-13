# Phase 21: Playability Foundation — Context

**Gathered:** 2026-05-13
**Status:** Ready for planning (`/gsd-plan-phase --phase 21`)
**Milestone:** v3.1 Scraper Self-Healing
**Spec:** `docs/plans/2026-05-13-scraper-self-healing-spec.md`

<domain>
## Phase Boundary

A playback request that would have hit an ad-poisoned server transparently rolls forward to the next server and plays real video, while the user sees a calm three-phase loader instead of a stuck spinner.

**Concretely, this phase delivers:**

1. A new shared package `libs/streamprobe/` with `Probe(ctx, masterURL, headers) Result`. Walks master m3u8 → first variant → first segment HEAD; classifies the outcome into one of seven reason enums (`playable, ad_decoy, zero_match, status_403, signed_url_expired, cdn_unreachable, empty_response`).
2. A hardcoded ad-CDN host-suffix blocklist (`ibyteimg.com`, `p16-ad-sg`, `ad-site-i18n`, `tiktokcdn.com`) inside `libs/streamprobe/blocklist.go`. Leads with a `// TODO:` block pointing at the spec's §4.1.c-TODO — when the list grows past a handful of entries OR the maintenance bot needs to extend it without a redeploy, lift into Redis (`scraper:streamprobe:blocklist`).
3. Server-priority list in `services/scraper/internal/providers/gogoanime/`: env `SCRAPER_SERVER_PRIORITY` (CSV, default `streamhg,earnvids,vibeplayer`). `ListServers` sorts results before returning; unknown servers appended in source-HTML order.
4. Per-server fallback in `gogoanime.GetStream`: iterate priority list, run streamprobe on each candidate, return the first `Result.Playable == true`. Total in-call budget ≤ 8 s.
5. Redis cache `scraper:winning_server:<provider>:<anime>:<ep>` (TTL 5 min) — subsequent calls for the same (anime,ep) skip iteration and the gate.
6. Two new Prometheus counters in scraper service: `parser_unplayable_total{provider, server, reason}` and `parser_ad_decoy_total{provider, server}`.
7. `GET /scraper/stream` response JSON gains `meta.gated: bool` — `true` whenever the gate ran on this call (used by FE to show Phase 3 of the loader).
8. `frontend/web/src/components/player/EnglishPlayer.vue` shows a three-phase loader: "Looking up sources…" / "Connecting to remote stream…" / "Verifying playback…" (EN + RU). Phase 3 visible only when scraper response carries `meta.gated: true`.

**Out of scope (deferred to later phases):**
- Multi-URL extraction (`hls2` + `hls3`) inside individual embed extractors → Phase 22.
- HLS proxy allowlist additions for `managementadvisory.sbs` / `exoplanethunting.space` → Phase 22.
- Canary cron job, Grafana dashboard, alert rules → Phase 23.
- WARP egress sidecar to revive VibePlayer → out of v3.1.
- MinIO segment archival → out of v3.1.

**Requirements covered:**
- SCRAPER-HEAL-01 (streamprobe Probe)
- SCRAPER-HEAL-02 (hardcoded blocklist + Redis-lift TODO)
- SCRAPER-HEAL-03 (server-priority env)
- SCRAPER-HEAL-04 (per-server fallback using gate)
- SCRAPER-HEAL-05 (winning-server Redis cache)
- SCRAPER-HEAL-06 (parser_unplayable_total + parser_ad_decoy_total)
- SCRAPER-HEAL-07 (meta.gated response field)
- SCRAPER-HEAL-08 (EnglishPlayer three-phase loader EN + RU)

</domain>

<decisions>
## Implementation Decisions

### D1 — Gate lives in `libs/streamprobe/`, not inside the scraper service

Reason: the scheduler canary in Phase 23 also needs to call the gate, and it lives in a different service. A shared lib avoids duplicating the blocklist and probe logic, and keeps test fixtures in one place. Same pattern as `libs/videoutils/` (which both streaming and scraper consume).

### D2 — Gate runs on every cold-path stream resolution, not just canary

Reason: the user is the most important regression detector. If a server breaks between canary runs, the user's first attempt should still surface a working stream — not 24 hours later. The latency cost (1-2 s per probed server, up to 3 servers) is masked by FE loader Phase 3 ("Verifying playback…").

Trade-off accepted: warm-path latency is unchanged via SCRAPER-HEAL-05 cache; cold-path is bounded at ≤ 8 s. If a future provider's HEAD request is reliably slow, the per-step timeout (4 s) can be tuned per-provider, not globally.

### D3 — Server-priority list is env-driven, not DB-driven

Reason: changing priority is a config-flip, not a data migration. Env default (`streamhg,earnvids,vibeplayer`) reflects PoC 2026-05-13 findings. The maintenance bot's Pattern 6 fix-path can update the env via `make redeploy-scraper` after a button-fix admin click — fits the existing tier model.

Alternative considered: DB column on `providers` table — rejected as overweight for a 3-server list that's expected to change rarely (only on provider rotations).

### D4 — Winning-server cache is Redis (not in-memory)

Reason: the scraper service horizontally scales; an in-memory cache would create per-instance hot/cold-server skew. Redis keeps every instance reading the same winning server. TTL 5 min matches existing `scraper:stream:*` cache convention.

### D5 — `meta.gated` is a top-level meta field, not a per-source field

Reason: the gate determines whether the WHOLE response carries provenance signals — not just a single source. The FE loader cares about "did the backend verify this?", not "which specific source URL was verified". Keeps the schema small.

### D6 — Three-phase loader copy is hard-coded in the .vue component, not pulled from i18n

Reason: existing EnglishPlayer.vue patterns hardcode loader text inline (verified in `frontend/web/src/components/player/EnglishPlayer.vue`); introducing i18n keys for just three new strings would be inconsistent. RU copy lives in the same component file as a small switch on locale. If the project adopts full i18n later, this is a 6-line migration.

### D7 — Reason enum lives in `libs/streamprobe/reason.go` as a typed string

Reason: shared between probe, scraper metrics, and maintenance prompt. Single source of truth. Tests assert that adding a new reason value requires updating the maintenance prompt (compile-time check via a sentinel const + a comment-block reference).

</decisions>

<open_questions>
None — all four spec-level open questions closed 2026-05-13. See `docs/plans/2026-05-13-scraper-self-healing-spec.md §9 Decisions`.
</open_questions>

<risks>
## Risks specific to this phase

- **Probe budget overshoot**: if a CDN times out at 4 s × 3 servers, GetStream sits at 12 s — over the soft budget. Mitigation: probe runs in parallel for the top-2 priority servers, sequential for the rest. Confirm in plan-checker phase.
- **`meta.gated` not used by FE = invisible**: must check that the FE loader rendering tests in Phase 21 cover the case where `meta.gated == false` (skip Phase 3) AND `meta.gated == true` (show Phase 3) — single-checkbox bugs here look like loader bugs.
- **Server-priority env typo silently demotes a real server**: if `SCRAPER_SERVER_PRIORITY=streamg,earnvids,vibeplayer` (typo in `streamhg`), the typo'd server is sorted last but no error is logged. Mitigation: at scraper startup, validate every priority-list entry against the known-extractor registry; fail-fast with a clear error on unknown names.
</risks>
