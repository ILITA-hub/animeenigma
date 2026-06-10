// frontend/web/src/composables/useTimezonePref.ts
// User-selectable display timezone ('auto' = browser zone). Persisted in
// localStorage under the established pref:* pattern; module-level singleton
// so Schedule.vue, DayModal and Anime.vue all react to one source of truth.
import { ref, computed } from 'vue'

const STORAGE_KEY = 'pref:timezone'

/** Curated picker list: full RU offset coverage + common world zones. */
export const TIMEZONE_CHOICES: ReadonlyArray<{ value: string; cityKey: string }> = [
  { value: 'UTC', cityKey: 'utc' },
  { value: 'Europe/London', cityKey: 'london' },
  { value: 'Europe/Berlin', cityKey: 'berlin' },
  { value: 'Europe/Kaliningrad', cityKey: 'kaliningrad' },
  { value: 'Europe/Kyiv', cityKey: 'kyiv' },
  { value: 'Europe/Minsk', cityKey: 'minsk' },
  { value: 'Europe/Moscow', cityKey: 'moscow' },
  { value: 'Europe/Samara', cityKey: 'samara' },
  { value: 'Asia/Yekaterinburg', cityKey: 'yekaterinburg' },
  { value: 'Asia/Almaty', cityKey: 'almaty' },
  { value: 'Asia/Omsk', cityKey: 'omsk' },
  { value: 'Asia/Krasnoyarsk', cityKey: 'krasnoyarsk' },
  { value: 'Asia/Irkutsk', cityKey: 'irkutsk' },
  { value: 'Asia/Yakutsk', cityKey: 'yakutsk' },
  { value: 'Asia/Tokyo', cityKey: 'tokyo' },
  { value: 'Asia/Vladivostok', cityKey: 'vladivostok' },
  { value: 'Asia/Magadan', cityKey: 'magadan' },
  { value: 'Asia/Kamchatka', cityKey: 'kamchatka' },
  { value: 'America/New_York', cityKey: 'newYork' },
  { value: 'America/Los_Angeles', cityKey: 'losAngeles' },
]

export function isValidTz(tz: string): boolean {
  try {
    new Intl.DateTimeFormat('en-US', { timeZone: tz })
    return true
  } catch {
    return false
  }
}

export const browserTimezone: string = (() => {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC'
  } catch {
    return 'UTC'
  }
})()

function readStored(): string {
  try {
    const v = localStorage.getItem(STORAGE_KEY)
    if (v && (v === 'auto' || isValidTz(v))) return v
  } catch {
    // storage blocked — fall through to auto
  }
  return 'auto'
}

const pref = ref<string>(readStored())

export function useTimezonePref() {
  const timezone = computed(() => (pref.value === 'auto' ? browserTimezone : pref.value))

  function setPref(v: string) {
    pref.value = v === 'auto' || isValidTz(v) ? v : 'auto'
    try {
      localStorage.setItem(STORAGE_KEY, pref.value)
    } catch {
      // storage blocked — selection still applies for this session
    }
  }

  return { pref, timezone, browserTimezone, setPref }
}
