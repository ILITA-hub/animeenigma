// frontend/web/src/composables/useUserTimezone.ts
// ACCOUNT-level display timezone: set once at sign-up (browser-detected),
// changeable only in Profile → Settings. Drives the anime page next-episode
// line and the home rail chip. Distinct from useTimezonePref, which is the
// schedule page's OWN localStorage-backed selector.
import { computed } from 'vue'
import { useAuthStore } from '@/stores/auth'
import { browserTimezone, isValidTz } from './useTimezonePref'

export function useUserTimezone() {
  // Tolerate mounts without an active Pinia (component unit tests).
  let auth: ReturnType<typeof useAuthStore> | null = null
  try {
    auth = useAuthStore()
  } catch {
    auth = null
  }

  const timezone = computed(() => {
    const tz = auth?.user?.timezone
    return tz && isValidTz(tz) ? tz : browserTimezone
  })

  return { timezone }
}
