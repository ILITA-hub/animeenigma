/**
 * Tiny relative-time formatter for the notifications dropdown + card.
 *
 * Uses `Intl.RelativeTimeFormat` (built into every supported browser; zero
 * new dependencies). Returns localized strings like "5 minutes ago",
 * "вчера", "3時間前" — handled natively by the Intl API per locale.
 *
 * Thresholds (per Phase 3 plan D-UI-08):
 *   - < 60 s     → "just now" (caller supplies the localized label)
 *   - < 60 min   → N minutes ago
 *   - < 24 h     → N hours ago
 *   - < 30 d     → N days ago
 *   - else       → absolute date in the supplied locale
 *
 * Workstream: notifications, Phase 3.
 */

export type SupportedLocale = 'en' | 'ru' | 'ja'

/**
 * Format an ISO-8601 timestamp as a localized relative-time string.
 *
 * @param iso         ISO-8601 timestamp (e.g. `"2026-05-21T12:34:56Z"`).
 * @param locale      'en' | 'ru' | 'ja'. Other strings are accepted and
 *                    passed through to Intl, which falls back gracefully.
 * @param justNowLabel Already-localized "just now" label (the i18n layer
 *                    holds the canonical translations; this helper stays
 *                    pure so it's unit-testable without vue-i18n).
 * @returns A localized human-readable relative string. Returns the
 *          original ISO if parsing fails (defensive — never throws).
 */
export function formatRelativeTime(
  iso: string,
  locale: SupportedLocale | string,
  justNowLabel: string,
): string {
  const ts = Date.parse(iso)
  if (!Number.isFinite(ts)) {
    return iso
  }

  const now = Date.now()
  const deltaMs = ts - now // negative for past
  const deltaSec = Math.round(deltaMs / 1000)
  const absSec = Math.abs(deltaSec)

  if (absSec < 60) {
    return justNowLabel
  }

  const rtf = new Intl.RelativeTimeFormat(locale, { numeric: 'auto' })

  if (absSec < 3600) {
    const minutes = Math.round(deltaSec / 60)
    return rtf.format(minutes, 'minute')
  }
  if (absSec < 86400) {
    const hours = Math.round(deltaSec / 3600)
    return rtf.format(hours, 'hour')
  }
  if (absSec < 2592000) {
    // 30 days
    const days = Math.round(deltaSec / 86400)
    return rtf.format(days, 'day')
  }

  // Beyond 30 days: absolute date, locale-formatted.
  return new Intl.DateTimeFormat(locale, {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
  }).format(new Date(ts))
}
