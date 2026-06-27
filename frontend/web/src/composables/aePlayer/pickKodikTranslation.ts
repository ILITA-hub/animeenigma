import type { AudioKind } from '@/types/aePlayer'

/** Minimal shape of a Kodik translation needed to choose one. */
export interface KodikTranslationLike {
  title: string
  type: string // 'voice' = dub, anything else = sub
}

/** Kodik tags dubs as type 'voice'; everything else is a sub translation. */
function isDub(t: KodikTranslationLike): boolean {
  return t.type === 'voice'
}

/**
 * Choose the Kodik translation to play.
 *
 *  1. An explicit team pick (`team`) wins when it still exists.
 *  2. Otherwise fall back to the first translation matching the requested AUDIO
 *     (sub vs dub) — NOT blindly `translations[0]`, which is frequently a DUB
 *     voice and would override a user who selected SUB.
 *  3. Only if no translation matches the audio at all, fall back to the first
 *     translation so a one-sided title still plays something.
 *
 * `team` is reset to null on every sub↔dub toggle (usePlayerState.setAudio), so
 * rule 2 is the common path and must respect `audio`.
 */
export function pickKodikTranslation<T extends KodikTranslationLike>(
  translations: T[],
  opts: { team: string | null; audio: AudioKind },
): T | undefined {
  if (translations.length === 0) return undefined
  const wantDub = opts.audio === 'dub'
  const pinned = opts.team ? translations.find((t) => t.title === opts.team) : undefined
  return (
    pinned ??
    translations.find((t) => isDub(t) === wantDub) ??
    translations[0]
  )
}
