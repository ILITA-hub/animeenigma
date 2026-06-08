// frontend/web/src/composables/schedule/calendarGrid.ts

export function startOfDay(d: Date): Date {
  return new Date(d.getFullYear(), d.getMonth(), d.getDate())
}

export function isSameDay(a: Date, b: Date): boolean {
  return a.getFullYear() === b.getFullYear() && a.getMonth() === b.getMonth() && a.getDate() === b.getDate()
}

/** Monday-of-week (Monday-first calendar). */
export function weekStart(d: Date): Date {
  const off = (d.getDay() + 6) % 7 // Mon=0 … Sun=6
  return new Date(d.getFullYear(), d.getMonth(), d.getDate() - off)
}

/** 7 dates Mon..Sun of the week containing `d`. */
export function weekDays(d: Date): Date[] {
  const s = weekStart(d)
  return Array.from({ length: 7 }, (_, i) => new Date(s.getFullYear(), s.getMonth(), s.getDate() + i))
}

/** All day cells (7 * N) for the month grid containing `viewDate`. */
export function monthGridDays(viewDate: Date): Date[] {
  const y = viewDate.getFullYear()
  const m = viewDate.getMonth()
  const first = new Date(y, m, 1)
  const lead = (first.getDay() + 6) % 7 // Mon-first offset of the 1st
  const daysInMonth = new Date(y, m + 1, 0).getDate()
  const weeks = Math.ceil((lead + daysInMonth) / 7)
  return Array.from({ length: weeks * 7 }, (_, i) => new Date(y, m, 1 - lead + i))
}

/** [start, end) covering the whole month grid (end exclusive). */
export function monthGridRange(viewDate: Date): { start: Date; end: Date } {
  const days = monthGridDays(viewDate)
  const start = startOfDay(days[0])
  const last = days[days.length - 1]
  const end = new Date(last.getFullYear(), last.getMonth(), last.getDate() + 1)
  return { start, end }
}
