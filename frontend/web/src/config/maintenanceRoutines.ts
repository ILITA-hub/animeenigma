/**
 * Static registry mapping each maintenance routine id (services/policy
 * MaintenanceRoutine.ID) to its display group, i18n name key, staleness
 * threshold, and the safe knobs the admin may tune. The Maintenance tab renders
 * cards from the BACKEND list, using this registry for labels + knob controls;
 * a backend row with no descriptor still renders (enable toggle only).
 *
 * Keep the ids in sync with domain.SeedRoutines(). Select-knob option values are
 * literal tokens (model names, durations, risk levels) shown verbatim — NOT i18n.
 */
export type MaintenanceKnob =
  | { key: string; type: 'select'; labelKey: string; options: string[] }
  | { key: string; type: 'number'; labelKey: string; min?: number; max?: number }
  | { key: string; type: 'chips'; labelKey: string; placeholderKey: string }

export interface MaintenanceRoutineDescriptor {
  id: string
  group: 'host' | 'cluster'
  nameKey: string
  /** ms after lastRunAt beyond which the status badge reads "stale"; omit to disable. */
  staleAfterMs?: number
  knobs: MaintenanceKnob[]
}

const HOUR = 3_600_000
const DAY = 24 * HOUR

export const MAINTENANCE_ROUTINES: MaintenanceRoutineDescriptor[] = [
  {
    id: 'maintenance_bot',
    group: 'host',
    nameKey: 'admin.policy.maintenance.routines.maintenance_bot',
    knobs: [
      { key: 'auto_apply_max_risk', type: 'select', labelKey: 'admin.policy.maintenance.knobs.autoApplyMaxRisk', options: ['none', 'low', 'medium'] },
      { key: 'suppressed_alerts', type: 'chips', labelKey: 'admin.policy.maintenance.knobs.suppressedAlerts', placeholderKey: 'admin.policy.maintenance.knobs.suppressedAlertsPlaceholder' },
    ],
  },
  {
    id: 'provider_recovery',
    group: 'host',
    nameKey: 'admin.policy.maintenance.routines.provider_recovery',
    staleAfterMs: 2 * DAY,
    knobs: [{ key: 'model', type: 'select', labelKey: 'admin.policy.maintenance.knobs.model', options: ['sonnet', 'opus'] }],
  },
  { id: 'git_autosync', group: 'host', nameKey: 'admin.policy.maintenance.routines.git_autosync', staleAfterMs: HOUR, knobs: [] },
  {
    id: 'disk_prune',
    group: 'host',
    nameKey: 'admin.policy.maintenance.routines.disk_prune',
    staleAfterMs: 2 * DAY,
    knobs: [{ key: 'high_water_pct', type: 'number', labelKey: 'admin.policy.maintenance.knobs.highWaterPct', min: 50, max: 95 }],
  },
  { id: 'build_cache_prune', group: 'host', nameKey: 'admin.policy.maintenance.routines.build_cache_prune', staleAfterMs: 8 * DAY, knobs: [] },
  { id: 'subtitle_probe', group: 'cluster', nameKey: 'admin.policy.maintenance.routines.subtitle_probe', staleAfterMs: HOUR, knobs: [] },
  { id: 'shikimori_sync', group: 'cluster', nameKey: 'admin.policy.maintenance.routines.shikimori_sync', staleAfterMs: 2 * DAY, knobs: [] },
  { id: 'playability_canary', group: 'cluster', nameKey: 'admin.policy.maintenance.routines.playability_canary', staleAfterMs: 2 * DAY, knobs: [] },
  {
    id: 'provider_self_heal',
    group: 'cluster',
    nameKey: 'admin.policy.maintenance.routines.provider_self_heal',
    knobs: [
      { key: 'promote_after', type: 'select', labelKey: 'admin.policy.maintenance.knobs.promoteAfter', options: ['12h', '24h', '48h'] },
      { key: 'probe_every', type: 'select', labelKey: 'admin.policy.maintenance.knobs.probeEvery', options: ['3h', '6h', '12h'] },
    ],
  },
]

const BY_ID = new Map(MAINTENANCE_ROUTINES.map((d) => [d.id, d]))
export function routineDescriptor(id: string): MaintenanceRoutineDescriptor | undefined {
  return BY_ID.get(id)
}
