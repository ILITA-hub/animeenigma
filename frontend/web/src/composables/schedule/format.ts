// frontend/web/src/composables/schedule/format.ts
/** HH:MM in Europe/Moscow (project standard, mirrors the old Schedule.vue). */
export function formatAirTime(date: Date): string {
  return date.toLocaleTimeString('ru-RU', { hour: '2-digit', minute: '2-digit', timeZone: 'Europe/Moscow' })
}
