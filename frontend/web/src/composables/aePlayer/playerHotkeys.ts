/**
 * playerHotkeys — pure keyboard-shortcut mapping for the unified player.
 *
 * Kept as a standalone pure function so the key→action contract is unit-tested
 * without mounting the player. AePlayer.vue attaches a `keydown` listener
 * scoped to its own root element and dispatches the returned action.
 *
 * Two mirrors depend on this contract — update them when it changes:
 * - utils/globalHotkeys.ts duplicates the chord + text-field guard below;
 * - views/TipsPage.vue renders the key→action table as a user cheatsheet.
 */

export type HotkeyAction =
  | { type: 'play-pause' }
  | { type: 'seek-rel'; value: number }
  | { type: 'vol-rel'; value: number }
  | { type: 'seek-pct'; value: number }
  | { type: 'sub-offset'; value: number }
  | { type: 'mute' }
  | { type: 'fullscreen' }
  | { type: 'subs' }
  | { type: 'pip' }
  // Jump to the next episode. `anytime` (Shift+N) advances whenever a next
  // episode exists; bare `n` is prompt-scoped — the dispatcher only acts on it
  // while an Up-Next prompt (countdown card or end-of-episode chip) is visible.
  | { type: 'next-episode'; anytime: boolean }

const SEEK_STEP = 5
const VOL_STEP = 5
const SUB_OFFSET_STEP = 0.1

/**
 * Translate a keyboard event into a player action, or null if the key is not a
 * shortcut (or focus is in a text field, where typing must not be hijacked).
 */
export function mapKeyToAction(e: KeyboardEvent): HotkeyAction | null {
  // A Ctrl/Cmd/Alt chord is a browser/OS command (copy, paste, cut, select-all,
  // find, print, focus URL bar…), never a bare player hotkey. Let it through so
  // the player never swallows e.g. Ctrl/Cmd+C over selectable subtitle text.
  // Shift is intentionally NOT excluded — it only produces an uppercase letter,
  // which the case-insensitive mapping below still treats as the same hotkey.
  if (e.ctrlKey || e.metaKey || e.altKey) return null

  // Never intercept keystrokes meant for a text field.
  const target = e.target as HTMLElement | null
  if (target) {
    const tag = (target.tagName || '').toUpperCase()
    if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT' || target.isContentEditable) {
      return null
    }
  }

  const key = e.key

  // Digit 0-9 → seek to that decile of the timeline.
  if (key.length === 1 && key >= '0' && key <= '9') {
    return { type: 'seek-pct', value: Number(key) * 10 }
  }

  switch (key) {
    case ' ':
    case 'Spacebar': // legacy Edge/IE
      return { type: 'play-pause' }
    case 'ArrowLeft':
      return { type: 'seek-rel', value: -SEEK_STEP }
    case 'ArrowRight':
      return { type: 'seek-rel', value: SEEK_STEP }
    case 'ArrowUp':
      return { type: 'vol-rel', value: VOL_STEP }
    case 'ArrowDown':
      return { type: 'vol-rel', value: -VOL_STEP }
  }

  switch (key.toLowerCase()) {
    case 'k':
      return { type: 'play-pause' }
    case 'j':
      return { type: 'seek-rel', value: -SEEK_STEP }
    case 'l':
      return { type: 'seek-rel', value: SEEK_STEP }
    case 'm':
      return { type: 'mute' }
    case 'f':
      return { type: 'fullscreen' }
    case 'c':
      return { type: 'subs' }
    case 'p':
      return { type: 'pip' }
    case 'z':
      // Nudge subtitles earlier (show sooner).
      return { type: 'sub-offset', value: -SUB_OFFSET_STEP }
    case 'x':
      // Nudge subtitles later (show further behind).
      return { type: 'sub-offset', value: SUB_OFFSET_STEP }
    case 'n':
      // Next episode. Shift+N advances any time; bare `n` only while an
      // Up-Next prompt is showing (the dispatcher enforces the scope).
      return { type: 'next-episode', anytime: e.shiftKey }
    default:
      return null
  }
}
