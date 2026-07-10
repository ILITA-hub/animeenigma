/**
 * globalHotkeys — pure predicates for app-wide keyboard shortcuts.
 *
 * Kept as standalone pure functions (mirroring aePlayer/playerHotkeys.ts) so
 * the key contract is unit-tested without mounting App.vue, which owns the
 * actual window listener and routing side effects.
 */

/**
 * `?` opens the secret tips & hotkeys page (/tips).
 *
 * True only for a bare `?` press (Shift is inherent — `?` is a shifted glyph
 * on most layouts; `e.key` is layout-resolved so RU Shift+7 works too) with
 * focus outside any text-entry element. Ctrl/Cmd/Alt chords are browser/OS
 * commands and never match.
 */
export function isHelpHotkey(e: KeyboardEvent): boolean {
  if (e.key !== '?') return false
  if (e.ctrlKey || e.metaKey || e.altKey) return false

  const target = e.target as HTMLElement | null
  if (target) {
    const tag = (target.tagName || '').toUpperCase()
    if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT' || target.isContentEditable) {
      return false
    }
  }
  return true
}
