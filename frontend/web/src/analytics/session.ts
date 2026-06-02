// Session id management. A session rotates after 30 min of inactivity or on
// a new UTC day. Stored in localStorage alongside its last-activity stamp.
const SID_KEY = 'aenig_analytics_sid'
const SID_TS_KEY = 'aenig_analytics_sid_ts'
const SID_DAY_KEY = 'aenig_analytics_sid_day'
const IDLE_MS = 30 * 60 * 1000

function dayOf(now: number): string {
  return new Date(now).toISOString().slice(0, 10)
}

// getSessionId returns the current session id, rotating it when the idle
// window has elapsed or the UTC day changed. Each call refreshes the
// last-activity stamp. `now` is injectable for testing.
export function getSessionId(now: number = Date.now()): string {
  try {
    const sid = localStorage.getItem(SID_KEY)
    const ts = Number(localStorage.getItem(SID_TS_KEY) || '0')
    const day = localStorage.getItem(SID_DAY_KEY)
    const expired = !sid || now - ts > IDLE_MS || day !== dayOf(now)
    if (expired) {
      const fresh = crypto.randomUUID()
      localStorage.setItem(SID_KEY, fresh)
      localStorage.setItem(SID_DAY_KEY, dayOf(now))
      localStorage.setItem(SID_TS_KEY, String(now))
      return fresh
    }
    localStorage.setItem(SID_TS_KEY, String(now))
    return sid as string
  } catch {
    return crypto.randomUUID()
  }
}
