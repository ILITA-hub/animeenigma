import { describe, it, expect } from 'vitest'
import { MAINTENANCE_ROUTINES, routineDescriptor } from '@/config/maintenanceRoutines'

describe('maintenanceRoutines registry', () => {
  it('covers the 9 seeded routine ids', () => {
    const ids = MAINTENANCE_ROUTINES.map((d) => d.id).sort()
    expect(ids).toEqual([
      'build_cache_prune', 'disk_prune', 'git_autosync', 'maintenance_bot',
      'playability_canary', 'provider_recovery', 'provider_self_heal',
      'shikimori_sync', 'subtitle_probe',
    ])
  })
  it('assigns every routine to a known group', () => {
    for (const d of MAINTENANCE_ROUTINES) expect(['host', 'cluster']).toContain(d.group)
  })
  it('resolves a descriptor by id and returns undefined for unknown', () => {
    expect(routineDescriptor('maintenance_bot')?.group).toBe('host')
    expect(routineDescriptor('nope')).toBeUndefined()
  })
  it('select knobs carry non-empty literal option lists', () => {
    for (const d of MAINTENANCE_ROUTINES) {
      for (const k of d.knobs) {
        if (k.type === 'select') expect(k.options.length).toBeGreaterThan(0)
      }
    }
  })
})
