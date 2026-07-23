---
id: PLAYER-mark-probe-unreachable-sources
title: Mark aePlayer sources as "not working" when the playback probe rates them unreachable
captured_at: 2026-07-23
captured_during: admin TODO capture (Telegram, @tNeymik)
target_milestone: unscheduled / TBD
deferred_from: N/A (net-new request)
status: backlog
depends_on: unified playback probe ([[project_unified_probe_live_first_run]]), Source-panel truth/ranking initiative ([[project_source_panel_truth_and_ranking]])
---

# Mark aePlayer sources as "not working" when the probe rates them unreachable

## Original request

Admin TODO («Туду: помечать как не рабит в аеПлеере то что проба оценила как
анричибл») — in the aePlayer Source panel, visually mark combos/providers as
"not working" when the analytics playback probe has evaluated them as
unreachable.

## Current state (context, not yet verified against live code at time of capture)

The unified playback probe (analytics service, `services/analytics/internal/probe/`)
already periodically evaluates providers and records a `reason` per tuple —
`cdn_unreachable` is one of the existing `probe_runs_total{provider,slot,server,
result,reason}` / `probe_provider_status{provider,status,reason}` values (see
[[project_unified_probe_live_first_run]], [[project_honest_per_provider_availability_shipped]]).
That data already drives:
- The Grafana "Provider Roster & Playability" dashboard panel (operator-facing only).
- Catalog's `policy`/`health` state machine ([[project_health_driven_policy_and_recovery_gate]],
  [[project_provider_health_hysteresis]]) — down/degraded providers get tinted or
  dropped from the capability feed already (Source-panel Phase A, "tint absent
  providers" — [[project_source_panel_truth_and_ranking]]).
- Phase B's `playability_index` (analytics `GET /internal/playability`), which blends
  `recent_up` (from `probe_runs`) into **ranking/sorting** of the `degraded` bucket in
  the Source panel — but this affects sort order, not an explicit "broken" label.
- A **hacker-mode-only** reactive tooltip (`useProviderAvailability` composable +
  `overlayAvailability` in the "advanceToNextSource" funnel) that shows
  "CDN источника недоступен" — but only fires on-demand when a hacker-mode user
  actually tries that source, and is invisible to normal users.

So there is real infrastructure to build on, but no existing surface shows a
**proactive, all-users** "not working" mark on a source driven directly by the
probe's periodic sweep verdict, ahead of the user clicking it.

## Scope (rough, not yet designed)

- Decide the data path: expose per-provider (or per-provider+anime, if granular
  enough) "probe says unreachable" as a field on the capability feed
  (`GET /api/anime/{id}/capabilities`) rather than a separate fetch — the existing
  `(policy,health,content)` derivation in `services/catalog/internal/parser/.../
  capability/service.go` is the natural place, mirroring how Phase A already tints
  no-content providers.
- Decide what "unreachable" means for marking purposes: a single probe `reason`
  value (`cdn_unreachable`) vs. the broader roster `down` state vs. a fresh
  per-title check — probe runs cover a rotating provider subset per sweep
  ([[project_honest_per_provider_availability_shipped]] dashboard gotcha), so
  staleness/coverage windowing needs the same care the Grafana roster panel took
  (latest-per-provider within a time window, not "last run only").
  Anchor-title probes may not generalize per-anime; likely this is a
  provider-level (not per-title) mark unless per-title `probe_runs` rows are
  joined in.
- Decide the UI treatment for the Source panel: a badge/label distinct from the
  existing "tinted/absent" and "degraded" states (Phase A/B already occupy those),
  visible to ALL users (not hacker-mode gated) so it's actually useful for
  everyday viewers deciding what to click.
- Confirm this doesn't fight the "graduated" degradation posture
  ([[project_graduated_degradation_score]] — "user streams NEVER killed") — this
  is purely an informational/UI mark on NOT-yet-selected sources, not an active
  stream kill, so should be safe, but worth confirming during design.

## Why deferred (not done inline)

- Admin explicitly asked for a TODO capture ("Туду: …"), not an immediate build —
  per the maintenance-bot feedback-store rules, a capture request records future
  work and is recorded as backlog, not implemented; the feedback entry stays a
  single open task.
- This is a genuine feature request (new UI surface + new capability-feed field),
  which the maintenance bot never auto-implements regardless of risk tier.
- The exact granularity (provider-level vs per-title) and UI placement need a
  real design pass against `docs/aeplayer-reference.md` and the Source-panel
  Phase A/B precedent, not a mechanical maintenance-bot edit.

## Cost estimate

| Component | Effort (Fib) | Risk |
|---|---|---|
| Capability-feed field: surface probe-derived "unreachable" per provider (catalog) | 5 | Low — additive field, existing (policy,health) derivation pattern |
| Windowed probe-verdict query (avoid single-last-run gap, mirror Grafana roster fix) | 3 | Low |
| Source panel UI: "not working" badge, all-users-visible | 5 | Low — CSS/markup, DS-lint + build verified |
| Design pass: granularity + interaction with existing tint/degraded/ranking states | 3 | Medium — product decision, avoid overlapping signals |
| **Total** | **~16** | |

## Cross-references

- [[project_unified_probe_live_first_run]] — probe mechanics, `probe_runs_total{reason}`
- [[project_honest_per_provider_availability_shipped]] — existing hacker-mode-only
  reactive unreachable tooltip; dashboard staleness-window gotcha to reuse
- [[project_source_panel_truth_and_ranking]] — Phase A (tint absent providers) and
  Phase B (playability_index ranking) — the two existing "provider quality" signals
  this new mark must not collide with
- [[reference_aeplayer_canonical_doc]] — Source panel canonical reference
- Source feedback: `2026-07-23T15-59-04_tNeymik_telegram`
