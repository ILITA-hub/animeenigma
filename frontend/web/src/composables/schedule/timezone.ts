// frontend/web/src/composables/schedule/timezone.ts
// Pure IANA-timezone helpers built on Intl (no date libraries in this project).

const dtfCache = new Map<string, Intl.DateTimeFormat>()

function dtf(tz: string): Intl.DateTimeFormat {
  let f = dtfCache.get(tz)
  if (!f) {
    f = new Intl.DateTimeFormat('en-US', {
      timeZone: tz,
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
      hourCycle: 'h23',
    })
    dtfCache.set(tz, f)
  }
  return f
}

/**
 * Re-express an instant as a Date whose LOCAL fields equal the wall-clock
 * time in `tz` — so existing local-field consumers (day grouping via
 * isSameDay, getDay/getDate titles, HH:MM formatting) read the chosen zone.
 * The result is a display value, not the original instant. No tz → as-is.
 */
export function wallClockDate(date: Date, tz?: string): Date {
  if (!tz) return date
  try {
    const p: Record<string, number> = {}
    for (const part of dtf(tz).formatToParts(date)) {
      if (part.type !== 'literal') p[part.type] = Number(part.value)
    }
    return new Date(p.year, p.month - 1, p.day, p.hour, p.minute, p.second, date.getMilliseconds())
  } catch {
    return date // invalid/unsupported tz (e.g. stale localStorage) — degrade to browser-local
  }
}

/** Current UTC offset of `tz` in minutes (DST-aware via `at`). */
export function tzOffsetMinutes(tz: string, at: Date = new Date()): number {
  return Math.round((wallClockDate(at, tz).getTime() - wallClockDate(at, 'UTC').getTime()) / 60000)
}

/** Compact offset label: "UTC+3", "UTC-4", "UTC+5:30". */
export function formatUtcOffset(tz: string, at: Date = new Date()): string {
  const m = tzOffsetMinutes(tz, at)
  const sign = m < 0 ? '-' : '+'
  const abs = Math.abs(m)
  const h = Math.floor(abs / 60)
  const mm = abs % 60
  return `UTC${sign}${h}${mm ? ':' + String(mm).padStart(2, '0') : ''}`
}
