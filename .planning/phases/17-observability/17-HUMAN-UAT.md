---
status: partial
phase: 17-observability
source: [17-VERIFICATION.md]
started: 2026-05-12T12:25:00Z
updated: 2026-05-12T12:25:00Z
---

## Current Test

[awaiting human testing]

## Tests

### 1. End-to-end Telegram alert delivery (SCRAPER-OBS-04 — live runtime)
expected: A real Telegram message arrives in the existing admin chat (`TELEGRAM_ADMIN_CHAT_ID`) when the `provider-health-stream-segment-down` Grafana alert fires after `provider_health_up{stage="stream_segment"} == 0 for 15m`.
how-to-test:
  1. Inject a controlled failure into the stream_segment stage (e.g., temporarily rename the Kwik referer header to garbage so `fetchSegment` returns 4xx on every probe tick) — easiest path: docker exec into scraper container and `sed` the referer header in a copy of the binary, or set `SCRAPER_PROBE_INITIAL_DELAY_OVERRIDE_SECONDS=5` to force fast ticking and break upstream URL via env override.
  2. Wait ≥15 min for the alert rule to fire (the `for: 15m` field).
  3. Confirm a message lands in the admin Telegram chat.
  4. Revert the change.
result: [pending]

### 2. Controlled 3-of-15-min flip experiment against live AnimePahe (SCRAPER-OBS-02 — live runtime)
expected: A stage flips from up=1 to up=0 after exactly 3 consecutive probe failures within a 15-min window, verified by observing the `provider_health_up{provider="animepahe",stage=<X>}` gauge transition in Prometheus.
how-to-test:
  1. Force 3 consecutive failures in the chosen stage (e.g., set the probe's golden anime ID to one that AnimePahe rejects, or temporarily blackhole the AnimePahe DNS for the scraper container).
  2. Wait through 3 probe ticks (~45 min default cadence, or set `SCRAPER_PROBE_INITIAL_DELAY_OVERRIDE_SECONDS=5` + reduce probe interval to drive 3 ticks in ~5 min for fast UAT).
  3. Confirm `provider_health_up{provider="animepahe",stage=<X>}` reads 0 in Prometheus.
  4. Revert; confirm it flips back to 1 on the next successful probe tick.
result: [pending]

## Summary

total: 2
passed: 0
issues: 0
pending: 2
skipped: 0
blocked: 0

## Gaps

(none — both items are scheduled human-UAT, not bugs)

## Notes

Static + unit-test verification has already confirmed:
- 3-of-15-min sliding-window threshold logic (window_test.go)
- Alert rule YAML structure (`for: 15m`, `severity: critical`, contact-point=Telegram)
- Probe writes to cache + cache.IsHealthy semantics
- Gauge transitions on stage flip (verified by unit tests via `metrics.ProviderHealthUp.WithLabelValues(...).Set(0)` assertions)

These 2 items exist only because the ROADMAP success criteria explicitly call out "verified end-to-end with a test alert" / "verified by intentionally breaking the AnimePahe stage in a controlled test" — they require live infrastructure + wall-clock time and were deferred from the autonomous execution loop.
