---
status: partial
phase: 02-be-egress-recorder
source: [02-VERIFICATION.md, 02-REVIEW.md]
started: 2026-06-05T07:30:00Z
updated: 2026-06-05T07:30:00Z
---

## Current Test

[awaiting human decision on the two items below]

## Tests

### 1. MegaplayExtractor egress blind spot (WR-07 / verifier human_needed)
expected: Every third-party hop emits an egress effect row. The MegaplayExtractor
(`services/scraper/internal/embeds/megaplay.go:93-97`) builds its own
`&http.Client{Timeout: 15s}` with the default (unrecorded) transport, so its hop to
`megaplay.buzz` (on the nineanime/9anime last-resort path) emits no row. Kodik solved the
identical leaf-extractor problem via `kodikextract.NewRecordingClient(wrap)`.
result: confirmed gap — live ClickHouse query shows `my.1anime.site` (1 row) and
`cdn.mewstream.buzz` (1 row) present (recorded via the wrapped scraper client + HLS proxy),
but ZERO `megaplay.buzz` rows. Not one of the 5 named clients in this phase's scope, but
contrary to the "every third-party request" goal narrative. DECISION NEEDED: accept as a
documented follow-up, or fix now.

### 2. CR-01 — recording RoundTripper emits effect only on resp.Body.Close()
expected: Exactly one egress effect per outbound request, robust to caller patterns.
`libs/tracing/client.go` emits the Effect on `resp.Body.Close()`; a caller that gets a 2xx
but skips Close on an early-return path would drop the row AND leak the connection.
result: latent gap, not a current goal failure — all wired callers in this phase use
`defer resp.Body.Close()`, and live `tracing_effects_dropped_total=0` confirms no drops in
production today. Verifier + reviewer both recommend fixing before Phase 3 adds more clients.

## Summary

total: 2
passed: 0
issues: 0
pending: 2
skipped: 0
blocked: 0

## Gaps

- WR-07 MegaplayExtractor unrecorded hop (megaplay.buzz) — confirmed via live ClickHouse query
- CR-01 effect-on-Close robustness gap — latent, no production impact today
