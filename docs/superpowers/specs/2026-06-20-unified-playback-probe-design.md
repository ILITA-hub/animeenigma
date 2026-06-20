# Unified Playback Probe — Design

**Date:** 2026-06-20
**Status:** Approved (design), pending spec review → plan
**Owner:** @0neymik0
**Constraint:** Do NOT modify scraper provider/extraction logic (`services/scraper/internal/providers/`, `embeds/`). The ONLY authorized scraper-service edit is deleting `services/scraper/internal/health/probe.go` and its boot wiring.

---

## 1. Problem

The playback-health dashboard reports providers GREEN while real playback is dead
(traced live 2026-06-19/20: Frieren → gogoanime → server HD-1 → megaplay CDN
`cdn.mewstream.buzz` is Cloudflare-403 to our datacenter IP; the proxy returns 502;
the sister HD-2 `watching.onl` plays fine but is never the default).

Root cause of the **false green** is two divergent probing systems:

- **Scraper in-process probe** (`services/scraper/internal/health/probe.go`) → emits
  `provider_health_up`, which drives the prominent "Provider × Stage Up" panel. It is
  misleading because it (a) probes a *random* anime per 15-min tick, (b) the `stream`
  stage only checks *extraction success* (megaplay extraction returns a URL fine — the
  403 only happens later at the CDN), and (c) gauges are failure-window smoothed (observed
  `stream_segment Up=1` with `last_ok=0001`, i.e. never actually succeeded).
- **Scheduler playability canary** (`services/scheduler/internal/jobs/scraper_playability_canary.go`)
  → emits `playability_canary_runs_total`. This one is *correct* (anchors + recent, reason
  codes) and DID catch the Frieren 403 — but it fetches segments **direct** (not through our
  proxy), so it mis-reports HD-2 as 403, and it lives in the less-prominent "Top Failing" table.

## 2. Goal

Exactly **one** probe engine. It must exercise the **real user playback path** end-to-end and
produce a truthful per-provider verdict, so a green dashboard means *users can actually watch*.

## 3. Decisions (locked)

| # | Decision |
|---|----------|
| D1 | Single engine lives in the **analytics** service. It calls the live system over HTTP — never imports scraper internals. |
| D2 | **Delete** `services/scraper/internal/health/probe.go` + boot wiring + `provider_health_up` registration. |
| D3 | **Absorb** the scheduler `scraper_playability_canary` job into the analytics engine. Scheduler keeps only a thin daily cron trigger. |
| D4 | Cadence: **daily** (`0 3 * * *`), scheduler-cron-triggered → `POST /internal/probe/run` on analytics (docker-network-only), mirroring the existing `player-ranking/recompute` / `read-threshold` pattern. |
| D5 | Resolve through **catalog's signed scraper path** (`/api/anime/{uuid}/scraper/...`) and validate **through `/api/streaming/hls-proxy`** — the exact player path. This is the fidelity fix vs the old direct-fetch canary. |
| D6 | **Real video validation:** download a few seconds of segments and run **ffprobe**; require ≥1 decodable video stream. Add `ffmpeg` to the analytics image. |
| D7 | Anime set per provider per run = 4 slots: `anchor` (Frieren) + `featured` + `spotlight_random` + `random`. |
| D8 | Dashboard panel "Provider × Stage Up" → **table: Provider \| Stage Status \| Reason**, one row per provider. |
| D9 | The scraper-resolution step is an **independent component** (`Resolver` interface) decoupled from validation, so each can be understood/tested alone. |

## 4. Architecture

```
scheduler (cron 0 3 * * *)
   └─ POST http://analytics:8092/internal/probe/run   (docker-network only)
        └─ analytics ProbeEngine.RunOnce()
             ├─ AnimeSetResolver  → 4 anime UUIDs/run (anchor+featured+spotlight_random+random)
             ├─ for each provider × anime:
             │     Resolver.Resolve(uuid, provider)        [INDEPENDENT COMPONENT — D9]
             │        → catalog /api/anime/{uuid}/scraper/episodes|servers|stream  (SIGNED)
             │        → returns ResolvedStream{ masterURL, exp, sig, referer, server, stage }
             │     Validator.Validate(ResolvedStream)
             │        → fetch master→variant→N segments via /api/streaming/hls-proxy
             │        → ffprobe bytes → Verdict{ reason, playable }
             ├─ Scorer.Rollup(per-anime verdicts) → ProviderVerdict{ status, reason }
             └─ Reporter
                  ├─ Prometheus gauges/counters
                  └─ ClickHouse run rows
```

### Components (each: one purpose, own interface, independently testable)

- **AnimeSetResolver** — produces the 4 slot UUIDs. `anchor` is a constant UUID
  (Frieren `f0b40660-6627-4a59-8dcf-7ec8596b3623`). `featured` + `spotlight_random` come from
  catalog `GET /api/home/spotlight` (cards carry anime refs). `random` is a random catalog title.
  Degrades gracefully: a slot that can't resolve is skipped (logged), the run still proceeds.
- **Resolver (independent — D9)** — given `(animeUUID, provider)`, walks catalog's signed scraper
  endpoints and returns the resolved, signed stream for each server it finds (episode 1).
  Interface: `Resolve(ctx, animeUUID, provider) ([]ResolvedStream, error)`. Knows nothing about
  ffprobe or scoring. The single seam where we talk to the "scraper system".
- **Validator** — given a `ResolvedStream`, fetches master → first variant → first ~2–3 segments
  **through the streaming hls-proxy** (so referer/sign/CDN behavior matches the player), caps bytes
  (≤ ~8 MiB) and wall time (≤ 10s), then runs ffprobe on the downloaded bytes. Returns a typed
  `Verdict`. SSRF guards as in `libs/streamprobe`.
- **Scorer** — folds per-(anime,server) verdicts into one `ProviderVerdict` (see §6).
- **Reporter** — emits Prometheus + ClickHouse (see §7).
- **Engine** — orchestrates the above for all providers; bounded concurrency; per-provider isolation
  (one provider's failure never aborts the run).

## 5. Video validation (D6)

1. Validator downloads the first variant's first 2–3 segments via the proxy. Hard caps:
   ≤ ~6s of media, ≤ ~8 MiB total, ≤ 10s wall.
2. Pipe bytes to **ffprobe** `-v error -show_streams -show_format -print_format json`.
3. Pass (`playable`) requires: ≥1 stream with `codec_type=video`, a known codec, and nonzero
   duration/frame signal. Otherwise classify (`decode_failed` / `invalid_video`).
4. **Dockerfile:** add `ffmpeg` to `services/analytics/Dockerfile` (`apk add --no-cache ffmpeg`).
   Exec is bounded by context timeout; failure is drop-and-classify, never a crash.

## 6. Reason codes & per-provider rollup

Reuse `libs/streamprobe` reasons: `playable`, `ad_decoy`, `zero_match`, `status_403`,
`signed_url_expired`, `cdn_unreachable`, `empty_response`. Add: `decode_failed`, `invalid_video`.

`stage` of a verdict = furthest stage reached: `search → episodes → servers → stream → playback`.

**ProviderVerdict (one row per provider on the dashboard):**

- **up** — ≥1 of the provider's anime reaches `playable` (valid decoded video).
- **degraded** — `stream` stage resolves but playback (segment/ffprobe) fails on all/most anime.
- **down** — no anime reaches a resolvable stream (fails at/-before `stream`).
- **Reason** — dominant failure classification with locus, e.g. `status_403 on megaplay HD-1`.

## 7. Metrics & storage

**Prometheus (analytics):**
- `probe_provider_up{provider}` — `1` up / `0.5` degraded / `0` down (drives the table color).
- `probe_runs_total{provider,slot,server,result,reason}` — counter (replaces `playability_canary_runs_total`).
- `probe_last_run_timestamp` — gauge (replaces the canary last-run signal).

**ClickHouse (analytics store):** one row per `(run_ts, provider, anime_uuid, slot, server, stage, reason, playable)`
for history/trend.

**Retired:** scraper `provider_health_up` (deleted with probe.go); scheduler
`playability_canary_runs_total` (job absorbed).

## 8. Dashboard changes (`docker/grafana/dashboards/playback-health.json`)

- Replace **"Provider × Stage Up"** state-timeline with a **table**:
  `Provider | Stage Status | Reason`, one row per provider, from `probe_provider_up` +
  `probe_runs_total` (reason via a transform/label). Color: green/amber/red on up/degraded/down.
- Repoint **"Canary Last Run (age)"** → `time() - probe_last_run_timestamp`.
- Repoint **"Top Failing (provider, server, reason, slot) Tuples"** →
  `topk(15, sum by (provider, server, reason, slot)(probe_runs_total{result="fail"}))`.
- Leave Roster, Real-User Telemetry, HLS-Proxy sections unchanged.

## 9. animefever comment (#2)

Edit catalog seed `services/catalog/internal/service/scraperprovider/seed.go`: remove the
"Region-walled" / egress-IP-class claims (unverified). Keep only what's observed: segments
302-redirect to an ad CDN (`ibytedtos…`) that 403s for us; provider stays `degraded`, out of
the auto-failover chain. Because the seed is insert-if-absent, ship a **guarded description-update
migration** so the live DB row updates. Update the mirrored text in `CLAUDE.md`.

## 10. Deletions

- `services/scraper/internal/health/probe.go` + its boot wiring in
  `services/scraper/cmd/scraper-api/main.go` + the `provider_health_up` metric registration
  (if scraper-only). *(Only authorized scraper edit.)*
- `services/scheduler/internal/jobs/scraper_playability_canary.go` (+ tests); replace with a thin
  `probe_trigger` job that POSTs to analytics.

## 11. Testing

Table-driven unit tests, fakes only (no live APIs):
- AnimeSetResolver: spotlight payload → 4 slots, slot-skip on resolve failure.
- Resolver: catalog episodes/servers/stream happy path + each error stage; signed-URL passthrough.
- Validator: proxy fetch + ffprobe wrapper faked — playable / 403 / ad_decoy / empty / decode_failed.
- Scorer: up/degraded/down rollup across mixed verdicts.
- Reporter: gauge/counter values; CH row shape.
- Scheduler trigger job: POSTs to the right path (mirror `provider_ranking_test.go`).

## 12. Risks

- ffmpeg image bloat (~80 MiB) — accepted; bounded exec + size cap.
- Analytics must reach `catalog:8081` + `streaming:8082` internally and forward `exp/sig` to the
  proxy. Catalog signs in `GetScraperStream`, so the signed URL is what `/scraper/stream` returns
  via the catalog route.
- Daily-only cadence means up to 24h detection latency for new breakage — acceptable "for now"
  per owner; real-user ClickHouse telemetry remains the fast signal.

## 13. Out of scope (this session)

- Any scraper provider/extraction change (no playback fix for the megaplay 403 here).
- Frontend per-server failover.
- Alerting/notification wiring on probe failure (future).
