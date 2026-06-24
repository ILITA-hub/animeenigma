/**
 * Pure, non-reactive helpers extracted out of `views/Profile.vue`'s
 * `<script setup>` (simplify pass 2026-06-24). None of these touch component
 * reactive state — i18n functions are passed in as explicit parameters so the
 * helpers stay pure and unit-testable. Behaviour is byte-for-byte identical to
 * the inline originals.
 */

// Loose i18n function shapes. We avoid importing vue-i18n's `ComposerTranslation`
// type to keep these helpers decoupled from the i18n internals — the call sites
// pass `t` / `te` from `useI18n()` directly.
type TFn = (key: string, named?: Record<string, unknown>) => string
type TeFn = (key: string) => boolean

/** Shape of the structured error body the import endpoints return. */
export interface ApiError {
  response?: {
    status?: number
    data?: {
      message?: string
      error?:
        | string
        | {
            code?: string
            message?: string
            details?: Record<string, string>
          }
    }
  }
}

/**
 * Relative-time string for the import card's "last synced" line.
 *
 * Distinct from `utils/time.ts#formatAgo` — this variant has minute/hour
 * granularity and is fully localized through the supplied `t`, matching the
 * `profile.import.*` keys. Unparseable input → "just now".
 */
export function timeAgo(dateStr: string, t: TFn): string {
  const date = new Date(dateStr)
  if (isNaN(date.getTime())) return t('profile.import.justNow')
  const now = new Date()
  const diffMs = now.getTime() - date.getTime()
  const diffMin = Math.floor(diffMs / 60000)
  const diffHours = Math.floor(diffMs / 3600000)
  const diffDays = Math.floor(diffMs / 86400000)

  if (diffMin < 1) return t('profile.import.justNow')
  if (diffMin < 60) return t('profile.import.minutesAgo', { n: diffMin })
  if (diffHours < 24) return t('profile.import.hoursAgo', { n: diffHours })
  return t('profile.import.daysAgo', { n: diffDays })
}

/**
 * Map a server error response into a friendly, localized message for the
 * import card. The backend tags each failure with `error.details.reason`
 * (see services/player/internal/handler/import_input.go and the
 * fetch{MAL,Shikimori}Page functions); we look up that reason in i18n and
 * fall back to the server's English message, then a generic translation.
 */
export function importErrorMessage(
  err: ApiError,
  source: 'mal' | 'shikimori',
  t: TFn,
  te: TeFn,
): string {
  const errBody = err.response?.data?.error
  const structured = typeof errBody === 'object' ? errBody : undefined
  const reason = structured?.details?.reason
  const host = structured?.details?.host
  const username = structured?.details?.username || structured?.details?.nickname

  if (reason) {
    const key = `profile.import.errors.${reason}`
    if (te(key)) return t(key, { host: host ?? '', username: username ?? '', source })
  }

  // Fall back to the server-provided message, then a generic localized one.
  const serverMsg = structured?.message
    || (typeof errBody === 'string' ? errBody : undefined)
    || err.response?.data?.message
  if (serverMsg) return serverMsg
  return t('profile.import.errors.generic', { source })
}

/**
 * Resize an avatar image File to a 256x256 center-cropped JPEG data URL.
 *
 * Pure (no component state): resolves with the data URL, or `null` when the
 * file exceeds 2 MB (the inline original silently bailed in that case). Runs
 * entirely in the browser via canvas — caller owns the resulting refs.
 */
export function resizeAvatarToDataUrl(file: File): Promise<string | null> {
  if (file.size > 2 * 1024 * 1024) {
    return Promise.resolve(null)
  }

  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => {
      const img = new Image()
      img.onload = () => {
        // Resize to 256x256 center-crop
        const canvas = document.createElement('canvas')
        canvas.width = 256
        canvas.height = 256
        const ctx = canvas.getContext('2d')!

        // Center crop to square
        const size = Math.min(img.width, img.height)
        const sx = (img.width - size) / 2
        const sy = (img.height - size) / 2

        ctx.drawImage(img, sx, sy, size, size, 0, 0, 256, 256)

        resolve(canvas.toDataURL('image/jpeg', 0.85))
      }
      img.onerror = reject
      img.src = reader.result as string
    }
    reader.onerror = reject
    reader.readAsDataURL(file)
  })
}
