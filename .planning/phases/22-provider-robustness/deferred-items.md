# Phase 22 — Deferred Items

Out-of-scope discoveries logged during executor work.

## DEF-22-01: cdn-centaurus.com / goldenridgeproduction.shop not in HLSProxyAllowedDomains

**Discovered during:** Plan 22-02 Task 3 production smoke (post-redeploy).
**Symptom:** `/scraper/stream?mal_id=52991&episode=frieren...` returned two sources whose hostnames (`in1rhjc5cqhz.cdn-centaurus.com` + `in1rhjc5cqhz.goldenridgeproduction.shop`) are NOT in `HLSProxyAllowedDomains`. Streaming proxy would 403 both on a real playback attempt.
**Scope rationale:** Pre-existing condition — these are different CDN families than the `hls2`/`hls3` pair this plan targets (premilkyway / dramiyos-cdn vs managementadvisory.sbs / exoplanethunting.space). My changes did not introduce the 403; they would have happened identically before my changes. The Frieren cold-path winner at smoke time happened to be a server I do not own.
**Recommendation:** Either:
  (a) Phase 23 canary detects via `parser_unplayable_total{reason="cdn_unreachable"}` and the maintenance Pattern 7 fix-path adds them in a separate PR; or
  (b) A follow-up plan in Phase 22 (e.g. 22-03) audits ALL distinct CDN host families currently reachable from each provider's extractor and reconciles `HLSProxyAllowedDomains` to a complete set.
**Severity:** Low for v3.1 milestone (multi-URL fallback path between hls2/hls3 — the documented v3.1 scope — works end-to-end as designed). Higher for general operability since these particular CDN families are unreachable for users right now.
**Action:** Logged here, not auto-fixed; would expand scope beyond locked plan files.
