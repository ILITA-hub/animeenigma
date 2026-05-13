# Scraper Self-Healing — Spec

**Date:** 2026-05-13
**Status:** Draft for review
**Owner:** Scraper service (`services/scraper/`)
**Maintenance owner:** AnimeEnigma Maintenance bot (`services/maintenance/`, prompt at `.claude/maintenance-prompt.md` Patterns 6/7 + "Scraper Playability Regression" alert)

---

## 1. Goal

Make the in-house EnglishPlayer / scraper service **survive upstream-site regressions automatically**. When VibePlayer / StreamHG / Earnvids / anitaku.to changes shape, the system should:

1. Detect the regression within 24h, before users notice
2. Route users around the broken provider transparently
3. Page the maintenance bot with enough context to apply a known-pattern fix (Pattern 6 / 7) — or escalate cleanly if the fix is outside its scope

The system stays Go-native (no Puppeteer sidecar — PoC proved unnecessary; see §3) and is automatically maintained by the existing maintenance bot (no new operator burden).

---

## 2. PoC findings that drive this spec (2026-05-13)

| Provider | What we believed before PoC | What PoC proved |
|---|---|---|
| **VibePlayer** | Fingerprint-poisoned regex → needs Puppeteer | **IP-poisoned**. Real Chromium gets the same ad-decoy m3u8. Only egress rotation (WARP) can defeat. |
| **StreamHG** | 403 from CDN → needs new allowlist/headers | **Already works**. Go regex extracts a valid signed `.m3u8` at `premilkyway.com`. HLS proxy serves real video. |
| **Earnvids** | Same as StreamHG | **Already works**. Valid signed `.m3u8` at `dramiyos-cdn.com`. Real video. |

**Production symptom explanation:** The frontend defaults to the first server in the embed list — always **VibePlayer (HD-1)**. The poisoned m3u8 parses, duration loads, but every segment is a TikTok ad CDN URL → `readyState=0` forever. StreamHG and Earnvids were never tried. Server reorder alone restores playback.

**Bonus finding:** Packed JS for StreamHG/Earnvids exposes **two URLs** — an `hls2` (signed `.m3u8`) and an `hls3` (unsigned `.txt` at a different CDN). Current regex only catches `hls2`. Worth a few extra lines while we're touching the extractor.

---

## 3. What we are NOT building (dropped from earlier draft)

| Dropped component | Reason |
|---|---|
| Puppeteer worker service (`services/extractor/`) | Not needed for any current provider; was solving a non-problem |
| Headless Chromium pool | Same |
| Per-provider browser sidecar | Same — Go regex on packed JS works fine for StreamHG/Earnvids; no real browser helps VibePlayer |

If a future provider genuinely requires JS execution that the packed-JS unpacker can't handle, revisit. For now: keep the architecture Go-native.

---

## 4. What we ARE building

### 4.1 Components

```
┌─ services/scraper/internal/providers/gogoanime/  ───────────────────────┐
│                                                                          │
│  4.1.a  Server priority + per-server fallback in ListServers / GetStream │
│         ─ Priority order (config-driven, default: streamhg, earnvids,    │
│           vibeplayer) instead of source-HTML order                       │
│         ─ GetStream tries each server in priority; on playability-gate   │
│           fail, advances to the next; cache the WINNING server per       │
│           (anime, ep) for 5 min                                          │
│                                                                          │
│  4.1.b  Multi-URL packed-JS extraction (streamhg.go, earnvids.go)        │
│         ─ Capture BOTH "hls2" (.m3u8) AND "hls3" (.txt) URLs             │
│         ─ Return them as multiple sources; orchestrator probes in order  │
└──────────────────────────────────────────────────────────────────────────┘
                                  │
                                  ▼
┌─ libs/streamprobe/  (NEW)  ─────────────────────────────────────────────┐
│                                                                          │
│  4.1.c  Playability gate                                                 │
│         Inputs: master m3u8 URL, Referer, expected-content hints         │
│         Steps:                                                            │
│           1. GET master with caller headers (timeout 4s)                 │
│           2. Assert 200, Content-Type ∈ {video/, application/vnd.apple.  │
│              mpegurl, application/x-mpegurl}, body parses as EXTM3U      │
│           3. Pick first variant, GET it (timeout 4s)                     │
│           4. Walk segment URIs, count hosts                              │
│           5. FAIL if any segment hostname matches the ad-CDN blocklist:  │
│              ibyteimg.com, p16-ad-sg.*, *.ad-site-i18n.*, tiktokcdn.com  │
│           6. FAIL if first-segment HEAD returns non-2xx                  │
│           7. Return Result{playable bool, reason enum, sampled []string} │
│         Reason enum: ad_decoy | zero_match | 403_upstream |              │
│              signed_url_expired | cdn_unreachable | empty_response       │
│         Decision: gate runs on EVERY cold-path stream resolution         │
│           (not just canary). Latency cost ~1-2s on the first server,     │
│           +1-2s per failed-and-retried server. Mitigated by FE loader    │
│           (4.1.d) and by caching the WINNING server per (anime,ep) for   │
│           5 min so subsequent requests skip the gate.                    │
│         Blocklist is hardcoded in v1; see 4.1.c-TODO below.              │
│                                                                          │
│  4.1.c-TODO  Externalize ad-CDN blocklist                                │
│         When the blocklist grows or a new ad-CDN family appears, lift    │
│         the hardcoded slice into Redis (`scraper:streamprobe:blocklist`) │
│         so the maintenance bot can extend the list without redeploying. │
│         Tracked: add a brief stub comment in libs/streamprobe/           │
│         blocklist.go pointing at this spec.                              │
│                                                                          │
│  4.1.d  Frontend loader copy update                                      │
│         The added 1-2s on cold-path resolution must not look like a     │
│         hung player. Update EnglishPlayer.vue loader overlay copy:      │
│           ─ Phase 1 (servers fetch):   "Looking up sources…"            │
│           ─ Phase 2 (stream fetch):    "Connecting to remote stream…"   │
│           ─ Phase 3 (gate validation): "Verifying playback…"            │
│         Localize for RU users: "Подключение к удалённому потоку…" etc.  │
│         Implementation: tie phase to existing `loadingServers` /        │
│         `loadingStream` refs and a new `validatingStream` ref set        │
│         by the scraper response when `meta.gated` is true.              │
│         Used by both: scraper GetStream (4.1.a) and canary cron (4.2.b)  │
└──────────────────────────────────────────────────────────────────────────┘
                                  │
                                  ▼
┌─ services/scheduler/  ──────────────────────────────────────────────────┐
│                                                                          │
│  4.2.a  Canary job: scraper-playability-canary                           │
│         ─ Cron: 03:00 daily                                              │
│         ─ Anime list (5 total, refreshed every run):                     │
│             • 2 fixed anchors: "Frieren: Beyond Journey's End" and       │
│               "One Piece" (long-running and popular — drift here means  │
│               drift in production)                                       │
│             • 3 dynamic from recent watch_history: most recent distinct  │
│               anime across all users in the last 24h, picked at run     │
│               time (catches "what users actually hit").                  │
│             Source: scheduler queries Postgres directly (it already has  │
│             DB access; same connection used by anime_loader.go). Query: │
│               SELECT DISTINCT anime_id FROM watch_history                │
│               WHERE created_at > NOW() - INTERVAL '24h'                  │
│               ORDER BY created_at DESC LIMIT 3                          │
│             Resolve anime_id → MAL id + title via animes table JOIN.    │
│             If watch_history empty, fall back to top 3 from              │
│             anime_list ORDER BY updated_at DESC.                         │
│         ─ Targets: each anime × episode 1 × every server returned by    │
│           ListServers (no provider skipping — exercise the whole list)  │
│         ─ Per run: call /scraper/stream → streamprobe (4.1.c) → record  │
│         ─ Emit metric:                                                   │
│           playability_canary_runs_total{provider,server,result,reason,  │
│                                          anime_slot}                     │
│             anime_slot ∈ {anchor_frieren, anchor_one_piece, recent_1,    │
│                          recent_2, recent_3}                             │
│         ─ Persist run log to player_reports volume for post-mortem       │
└──────────────────────────────────────────────────────────────────────────┘
                                  │
                                  ▼
┌─ infra/grafana/  + Prometheus  ─────────────────────────────────────────┐
│                                                                          │
│  4.3.a  New metrics emitted by scraper + libs/streamprobe                │
│         ─ parser_unplayable_total{provider, server, reason}              │
│         ─ parser_ad_decoy_total{provider, server}  (subset of above)     │
│         ─ playability_canary_runs_total{provider, server, result, reason}│
│         (existing: parser_zero_match_total, proxy_upstream_errors_total) │
│                                                                          │
│  4.3.b  Alert rules                                                      │
│         ─ "Scraper Playability Regression" — fires on any canary FAIL    │
│           in last 25h (catches one failed nightly + handles late starts) │
│         ─ "Scraper Ad-Decoy Surge" — parser_ad_decoy_total rate > 0      │
│           sustained 5 min in prod                                        │
│         ─ "Scraper Unplayable Spike" — parser_unplayable_total rate >    │
│           5% of GetStream calls sustained 5 min in prod                  │
│         ─ Webhook target: existing maintenance svc /api/grafana-webhook  │
│                                                                          │
│  4.3.c  Grafana panel: "Scraper Provider Health"                         │
│         ─ Stacked bar per provider/server: pass/fail counts per 24h      │
│         ─ Reason breakdown when failing                                  │
└──────────────────────────────────────────────────────────────────────────┘
                                  │
                                  ▼
┌─ services/maintenance/  (EXISTING — already in place) ──────────────────┐
│                                                                          │
│  4.4.a  Alert receiver: /api/grafana-webhook (already routes to Claude)  │
│  4.4.b  Updated prompt: .claude/maintenance-prompt.md                    │
│         ─ Patterns 6 (VibePlayer ad-decoy) + 7 (schema drift) added      │
│         ─ "Scraper Playability Regression" alert guidance added          │
│         ─ Reason-enum dispatch (ad_decoy → Pattern 6 fix; zero_match →   │
│           Pattern 7 fix; etc.)                                           │
│         ─ Explicit DO NOT restart scraper for these alerts (would mask   │
│           the real cause)                                                │
│  4.4.c  Tiers stay the same: code-edit fixes are button_fix (admin       │
│         clicks to apply); infra/IP issues escalate                       │
└──────────────────────────────────────────────────────────────────────────┘
```

### 4.2 New files / packages

| Path | What |
|---|---|
| `libs/streamprobe/probe.go` | Public `Probe(ctx, masterURL, headers) Result` |
| `libs/streamprobe/blocklist.go` | Hardcoded ad-CDN host blocklist (start with: ibyteimg.com, p16-ad-sg, ad-site-i18n, tiktokcdn) |
| `libs/streamprobe/probe_test.go` | Table tests with synthetic m3u8 fixtures for each `Reason` |
| `services/scheduler/internal/jobs/scraper_playability_canary.go` | Cron job — flat file alongside existing `anime_loader.go`, `calendar.go`, `cleanup.go`, etc. Uses `libs/streamprobe` + scraper HTTP client. |
| `infra/grafana/dashboards/scraper-provider-health.json` | New dashboard |
| `infra/grafana/alerts/scraper.yaml` | Three alert rules → maintenance webhook |
| `docs/issues/README.md` | Append **ISS-011: VibePlayer ad-decoy poisoning** entry inline (matches project's inline-numbering convention; highest current is ISS-010). Records the PoC findings as the first incident this self-healing system will catch automatically going forward. |

### 4.3 Modified files

| Path | Change |
|---|---|
| `services/scraper/internal/providers/gogoanime/client.go` | Server-priority list (config) + per-server retry in `GetStream` |
| `services/scraper/internal/embeds/streamhg.go` | Extract both `hls2` and `hls3`; return multi-source `Stream` |
| `services/scraper/internal/embeds/earnvids.go` | Same |
| `services/scraper/internal/config/config.go` | New env: `SCRAPER_SERVER_PRIORITY` (CSV, default `streamhg,earnvids,vibeplayer`) |
| `services/scraper/internal/domain/stream.go` (if it caps to one source) | Allow multiple sources; orchestrator picks first playable |
| `libs/videoutils/proxy.go` | Allowlist already covers `premilkyway.com`, `dramiyos-cdn.com`, `vibeplayer.site`, `cdn.cimovix.store` (verified `libs/videoutils/proxy.go:259-262`). **Must add** `managementadvisory.sbs` and `exoplanethunting.space` before §4.1.b's hls3 multi-URL extraction can ship — those are the NEW CDN hosts PoC unpacked from packed JS and they are NOT currently allowlisted. |
| `frontend/web/src/components/player/EnglishPlayer.vue` | Three-phase loader overlay (4.1.d). New `validatingStream` ref; set from scraper response `meta.gated`. EN/RU copy: "Looking up sources…" / "Connecting to remote stream…" / "Verifying playback…" |
| `services/scraper/internal/handler/scraper.go` | `GetStream` response includes `meta.gated: true` when the playability gate ran (so FE can show the verification phase) |
| `.claude/maintenance-prompt.md` | DONE — Patterns 6/7 + Scraper Playability Regression alert section |

---

## 5. Test plan

| ID | What | Type | Where |
|---|---|---|---|
| T1 | `streamprobe.Probe` returns `ad_decoy` for synthetic m3u8 with ibyteimg.com segments | Unit | `libs/streamprobe/probe_test.go` |
| T2 | `streamprobe.Probe` returns `cdn_unreachable` when first-segment HEAD times out | Unit | same |
| T3 | `streamprobe.Probe` returns `zero_match` for malformed m3u8 (no `#EXTM3U` header) | Unit | same |
| T4 | `streamprobe.Probe` returns `playable: true` for golden good fixture | Unit | same |
| T5 | gogoanime `GetStream` skips a server whose probe fails and tries the next | Unit (fake streamprobe + fake embeds) | `services/scraper/internal/providers/gogoanime/client_test.go` |
| T6 | gogoanime server-priority config honoured (env `SCRAPER_SERVER_PRIORITY=earnvids,streamhg,vibeplayer` → that order tried) | Unit | same |
| T7 | streamhg extractor returns both `hls2` and `hls3` URLs from golden packed-JS | Unit | `services/scraper/internal/embeds/streamhg_test.go` |
| T8 | earnvids extractor same | Unit | `earnvids_test.go` |
| T9 | Canary job records `result: pass/fail` and correct `reason` label per server | Integration | `services/scheduler/internal/jobs/canary_test.go` (testcontainer for Redis + mock scraper) |
| T10 | Scraper /metrics exposes `parser_unplayable_total` and `parser_ad_decoy_total` with correct labels | Integration | `services/scraper/internal/handler/metrics_test.go` (new) |
| T11 | Canary job composes its anime list as `[Frieren, One Piece, recent_1, recent_2, recent_3]` and falls back to anime_list when watch_history is empty | Integration | `services/scheduler/internal/jobs/scraper_playability_canary_test.go` |
| T12 | EnglishPlayer.vue shows correct loader phase text given each combination of `loadingServers` / `loadingStream` / `validatingStream` refs (incl. RU locale) | Component | `frontend/web/src/components/player/__tests__/EnglishPlayer.spec.ts` |

---

## 6. Alert flow → maintenance handoff

```
canary cron fails  ──▶  Prometheus scrape  ──▶  Alert rule fires  ──▶
   ──▶  Alertmanager webhook  ──▶  services/maintenance /api/grafana-webhook
   ──▶  Maintenance bot reads .claude/maintenance-prompt.md
        - Matches alert against Pattern 6 (ad_decoy) or Pattern 7 (zero_match)
        - Tier:
            * Code-edit fix (selector update, regex broaden, allowlist add)
              → button_fix; admin clicks to approve; bot applies via Edit
                + redeploy via `make redeploy-scraper`
            * IP-block / network issue → escalate (out of bot scope)
            * Two providers down simultaneously → escalate (likely network)
        - Reply via Telegram with diagnosis + (if button_fix) the fix_plan
```

The maintenance bot **does not need additional code** — it already receives Grafana webhooks and dispatches to Claude with the prompt file as instructions. The only change is the new Pattern 6 / 7 entries and the alert guidance, which are already in place.

**Out of scope for maintenance bot (escalate):**
- Cloudflare WARP configuration changes
- DNS / egress-IP changes
- Two-provider simultaneous failure
- A fix that requires changing the playability gate's own logic (would be circular)

---

## 7. Fix order (ship sequence)

1. **`libs/streamprobe`** — depends on nothing, lowest risk, unblocks everything else. Includes the §4.1.c-TODO comment stub.
2. **Server priority + per-server fallback in gogoanime** — uses (1); restores production playback.
3. **Multi-URL extraction in streamhg/earnvids** — bonus robustness; allowlist additions for `managementadvisory.sbs` + `exoplanethunting.space` ship with this step.
4. **FE loader phases in EnglishPlayer.vue** — ship together with (2) so the new ~1-2s gate latency never feels like a stuck player. Scraper response gains `meta.gated`.
5. **Metrics emission** (parser_unplayable_total, parser_ad_decoy_total) — minimal code, big observability win.
6. **Canary job in scheduler** — uses (1); 03:00 daily, dynamic anime list.
7. **Grafana dashboard + alert rules → maintenance webhook** — config-only.
8. **Optional next phase: WARP egress for VibePlayer A/B test** — separate spec when we want VibePlayer back.
9. **Future: MinIO archival** — separate spec, depends on (1) being stable.

---

## 8. Acceptance criteria

- [ ] EnglishPlayer plays real video for at least one canary anime end-to-end without VibePlayer (StreamHG or Earnvids does the job)
- [ ] Canary cron runs at 03:00, surfaces a regression within 24h if any server breaks
- [ ] Alert fires → maintenance bot receives → tiers correctly per Pattern 6/7
- [ ] No regressions in existing tests (`go test ./...` in scraper + libs)
- [ ] Maintenance prompt edits live in `.claude/maintenance-prompt.md`
- [ ] VibePlayer ad-decoy incident appended to `docs/issues/README.md` as ISS-011

---

## 9. Decisions (closed 2026-05-13)

1. **Canary anime list** — 2 fixed anchors (Frieren, One Piece) + 3 dynamic from recent global watch_history (picked at run time). Detail in §4.2.a.
2. **Probe budget** — gate runs on every cold-path stream resolution (not just canary). FE loader phases mask the latency: "Looking up sources…" → "Connecting to remote stream…" → "Verifying playback…". Cached winning server per (anime,ep) for 5 min so warm-path skips the gate. Detail in §4.1.c + §4.1.d.
3. **Blocklist evolvability** — hardcoded slice in v1; explicit TODO + comment-stub in `libs/streamprobe/blocklist.go` to lift into Redis once the list grows or the maintenance bot needs to extend it without a redeploy. Detail in §4.1.c-TODO.
4. **Cron frequency** — daily at 03:00 local. Detail in §4.2.a.
