import { adminApi } from '@/api/client'
import type { MaintenanceRoutineWire, MaintenanceRoutinesResponse } from '@/api/client'

// Maintenance tab composable, mirroring useAdminProviders.ts. Responses use the
// standard {success,data} envelope, so we unwrap `res.data?.data ?? res.data`.
export type { MaintenanceRoutineWire }

function unwrap<T>(data: unknown): T {
  const d = data as { data?: T }
  return d && typeof d === 'object' && 'data' in d ? (d.data as T) : (data as T)
}

export function useAdminMaintenance() {
  async function list(): Promise<MaintenanceRoutineWire[]> {
    const res = await adminApi.getMaintenanceRoutines()
    return unwrap<MaintenanceRoutinesResponse>(res.data).routines
  }
  async function setRoutine(
    id: string,
    body: { enabled: boolean; settings: Record<string, unknown> },
  ): Promise<void> {
    await adminApi.setMaintenanceRoutine(id, body)
  }
  return { list, setRoutine }
}
