# Degradation management hardening (2026-07-22)

Follow-up hardening pass over the graceful-degradation subsystem
([`2026-07-10-graceful-degradation-design.md`](2026-07-10-graceful-degradation-design.md) +
[`2026-07-21-graduated-degradation-design.md`](2026-07-21-graduated-degradation-design.md)).
Fixes nine weaknesses found in a review of the live mechanism. Scores per
`.planning/CONVENTIONS.md`.

## Governor core

1. **Egress governance (opt-in).** The recording rules already compute the full
   uplink-attribution family (`ae:host_egress:bytes_per_second` etc.) but NONE
   of it fed the level/score — blind to the resource most likely to saturate a
   self-hosted, no-CDN box. The governor now folds egress-as-fraction-of-uplink
   into the verdict when `GOVERNOR_UPLINK_MBPS > 0` (default 0 = disabled, so an
   unknown uplink never false-positives): breach `egress_uplink` at
   `GOVERNOR_EGRESS_ELEVATED_FRAC` (0.75) / `..._CRITICAL_FRAC` (0.90) and a
   parallel score band (0 at half-elevated, 0.5 at elevated, 1.0 at critical),
   `max()`-combined with the PSI score. `UXΔ=+3 (Better)` · `CDI=0.04 * 13` ·
   `MVQ=Griffin 85%/80%`.
2. **Seeding shed (library).** Torrent SEEDING/upload — the most sheddable egress
   consumer — was gated by nothing. Now degradation-gated at level ≥ 1 (the same
   `shedGate` pattern as download/encode/storyboard), so it's the first thing to
   yield uplink to live playback. `UXΔ=+2 (Better)` · `CDI=0.02 * 8` ·
   `MVQ=Griffin 80%/80%`.
3. **One smoothed state.** The discrete hysteresis `Machine` and the continuous
   EWMA `Smoother` were independent state, hand-tuned to agree but structurally
   able to diverge (level-consumers and score-consumers acting on inconsistent
   views); the streak flap-reset could also under-react to noisy-but-real
   escalation. Collapsed to ONE smoothed score; the level is now a Schmitt-trigger
   quantization of it (`Quantizer`, enter/exit thresholds), so level and score
   can never disagree and a jittery ramp still integrates. Published score is
   unchanged, so score-consumers (content-verify, Camoufox) are unaffected.
   `UXΔ=+1 (Ambiguous)` · `CDI=0.06 * 21` · `MVQ=Kraken 80%/70%`.
4. **Staleness guard.** node-exporter→scrape→rule-eval→instant-query lags under
   the very load the governor detects; a slow-but-succeeding Prometheus looked
   healthy. The governor now measures the freshest sample's age
   (`time() - timestamp(ae:pressure_score:preview)`), publishes
   `ae_governor_signal_staleness_seconds`, and when age > `GOVERNOR_STALENESS_MAX`
   (45s) HOLDS the current level (never trusts stale data to LOWER shedding) with
   a `signal_stale` reason. `UXΔ=+1 (Better)` · `CDI=0.02 * 8` · `MVQ=Sprite 80%/85%`.
5. **Override TTL.** `ae:degradation:override` was the one un-TTL'd, fail-CLOSED
   key — forgotten at a shed level, it starves background work forever. The CLI
   now sets it with a default 2h TTL (`--permanent` escape hatch;
   `DEGRADATION_OVERRIDE_TTL` env) and `status` shows the remaining TTL.
   `UXΔ=+1 (Better)` · `CDI=0.01 * 3` · `MVQ=Sprite 85%/90%`.
6. **Held-by-hysteresis reason.** During exit-slow the level is > 0 while
   instantaneous breaches are empty → status showed "level 1, reasons: []".
   Now injects a `held_by_hysteresis` info reason. `UXΔ=+1 (Better)` ·
   `CDI=0.01 * 2` · `MVQ=Sprite 85%/90%`.
7. **mem_available doctrine.** The rules preamble insists PSI-not-static-usage,
   yet `mem_available` was the one static-ratio trigger — can fire on a healthy
   high-cache/high-swap box. Now PSI-corroborated: the mem_available breach AND
   score require `ae:host_psi_mem_full:ratio > 0.02` (real memory stalling).
   Genuine memory distress is still caught independently by the `psi_mem_full`
   signal. `UXΔ=+1 (Better)` · `CDI=0.02 * 5` · `MVQ=Sprite 80%/85%`.

## Request path

8. **Request-tier levers.** Every actuator was background work; a request-driven
   spike had no lever. Added Critical-only (level ≥ 2) suppression of recs
   recompute churn and on-the-fly image resize (serve original/cached instead).
   Egress governance (#1) also makes a playback-driven event surface as
   `egress_uplink` rather than mis-blaming background work — subsuming the
   "effectiveness guard reason" intent. `UXΔ=+1 (Better)` · `CDI=0.03 * 13` ·
   `MVQ=Griffin 75%/75%`.

## Robustness / quality

9. **Curve parity fixture + replica guard.** The Go (`curve.go`) and Python
   (`scaling.py`) score→cap curves duplicate the same floor+epsilon math with
   hand-kept parity — now pinned by a shared test-vector fixture both test suites
   assert against. content-verify's unenforced replicas=1 assumption gets a
   Redis-heartbeat replica-count detector (`content_verify_replicas_detected`
   gauge + WARN) so the landmine is loud. `UXΔ=0 (Ambiguous)` · `CDI=0.01 * 5` ·
   `MVQ=Sprite 85%/90%`.

## Dashboards

New series (`ae_governor_signal_staleness_seconds`,
`ae_governor_egress_uplink_fraction`, `signal_stale`/`held_by_hysteresis`/
`egress_uplink` reasons, `content_verify_replicas_detected`) are added to
`docker/grafana/dashboards/degradation-overview.json` in the same change.
