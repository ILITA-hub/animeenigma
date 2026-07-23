# Staggered degradation actuators (2026-07-23)

**Status:** implemented.
**Extends:** `2026-07-21-graduated-degradation-design.md`.

## Problem

The governor already publishes one smoothed pressure score, but the remaining
binary library consumers all watched the same Elevated level edge. Their
`ae_degradation_shed` series therefore changed together and the host lost
several background capabilities in one cliff.

## Actuator order

Each consumer now owns a score threshold or concurrency curve. The score is the
governor's fast-rise/slow-decay EWMA; level 2 remains a hard backstop.

| Score | Actuation |
|---|---|
| `0.20` | Pause torrent seeding (pure background egress). |
| `0.30` | Pause storyboard backfill (lowest-priority disk/CPU work). |
| `0.40 → 0.80` | Reduce library encode concurrency one slot at a time, then pause. |
| `0.40 → 0.80` | Existing content-verify and Camoufox score curves keep stepping down. |
| `0.55 → 0.90` | Reduce library download concurrency one slot at a time, then pause. |
| Level 2 | Hard-pause new library work and refuse new Camoufox work. |

The cap curve is `floor(max_workers × remaining_band_fraction)`, clamped to one
worker until the pause threshold. A falling cap never cancels running work; it
only prevents surplus workers from claiming the next queued job.

## Observability

`ae_degradation_shed{subsystem}` reports actuator state rather than copying the
governor level:

- `0`: full admission;
- `1`: reduced concurrency or paused deferrable admission;
- `2`: fully paused/refusing.

Camoufox derives this gauge from `pool_target < STEALTH_POOL_SIZE`, so the
dashboard shows its graduated pool reduction before Critical.

## Safety

- Missing Redis/governor score still fails open to full speed.
- Running downloads, transcodes, and user streams are never killed.
- Critical level overrides a missing/low score and stops new heavy admission.
- Recovery follows the same thresholds in reverse over the governor's
  slow-decaying score, preventing an all-at-once restart surge.
