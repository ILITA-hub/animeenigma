// frontend/web/src/composables/schedule/format.ts
/** HH:MM in Europe/Moscow (project standard, mirrors the old Schedule.vue). */
export function formatAirTime(date: Date): string {
  return date.toLocaleTimeString('ru-RU', { hour: '2-digit', minute: '2-digit', timeZone: 'Europe/Moscow' })
}

type T = (key: string, named?: Record<string, unknown>) => string

/** e.g. "Суббота, 13 июня" — weekday + day + genitive month from i18n. */
export function formatDayTitle(date: Date, t: T): string {
  const dowIdx = (date.getDay() + 6) % 7 // Mon=0
  const dowKeys = ['monday', 'tuesday', 'wednesday', 'thursday', 'friday', 'saturday', 'sunday']
  const monKeys = ['jan', 'feb', 'mar', 'apr', 'may', 'jun', 'jul', 'aug', 'sep', 'oct', 'nov', 'dec']
  const weekday = t(`schedule.days.${dowKeys[dowIdx]}`)
  const month = t(`schedule.monthsGenitive.${monKeys[date.getMonth()]}`)
  return `${weekday}, ${date.getDate()} ${month}`
}
