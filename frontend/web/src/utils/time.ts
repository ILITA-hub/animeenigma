/**
 * Locale-aware relative-time formatting for spotlight cards.
 *
 * `formatAgo` was extracted from LatestNewsCard's inline `formatEntryDate`
 * (v1.1-polish Phase 07) so both LatestNewsCard and NotTimeYetCard
 * (Phase 09, HSB-V11-NT-01) share one implementation. Uses the native
 * `Intl.RelativeTimeFormat` (zero dependencies, localized per the supplied
 * locale).
 *
 * Thresholds (mirrors the Phase 07 behaviour):
 *   - |Δ| < 1 day  → "today" (rtf.format(0, 'day'))
 *   - |Δ| < 7 days → N days ago
 *   - |Δ| < 30 day → N weeks ago
 *   - else         → absolute medium date in the supplied locale
 *
 * Defensive: returns the raw ISO string when the input is unparseable and
 * never throws (the Intl constructor can throw on exotic locale strings).
 */

/**
 * Format an ISO-8601 timestamp as a localized relative string.
 *
 * @param iso    ISO-8601 timestamp (e.g. `"2026-05-10T08:30:00Z"`).
 * @param locale BCP-47 locale tag — typically 'en' | 'ru' | 'ja'. Other
 *               strings are passed through to Intl, which degrades
 *               gracefully.
 * @returns A localized relative string ("2 weeks ago", "2 недели назад",
 *          "2週間前"), or the original ISO if it cannot be parsed.
 */
export function formatAgo(iso: string, locale: string): string {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  const now = new Date()
  const diffMs = d.getTime() - now.getTime()
  const diffDays = Math.round(diffMs / 86_400_000)
  try {
    const rtf = new Intl.RelativeTimeFormat(locale, { numeric: 'auto' })
    if (Math.abs(diffDays) < 1) return rtf.format(0, 'day') // "today"
    if (Math.abs(diffDays) < 7) return rtf.format(diffDays, 'day')
    if (Math.abs(diffDays) < 30) return rtf.format(Math.round(diffDays / 7), 'week')
    return new Intl.DateTimeFormat(locale, { dateStyle: 'medium' }).format(d)
  } catch {
    return iso
  }
}
