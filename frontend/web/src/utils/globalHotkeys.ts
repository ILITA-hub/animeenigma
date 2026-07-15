/**
 * globalHotkeys — pure predicates for app-wide keyboard shortcuts.
 *
 * Kept as standalone pure functions (mirroring aePlayer/playerHotkeys.ts) so
 * the key contract is unit-tested without mounting App.vue, which owns the
 * actual window listener and routing side effects.
 */

/**
 * `F1` opens the secret tips & hotkeys page (/tips) in a new tab.
 *
 * True only for a bare `F1` press with focus outside any text-entry element.
 * Modified chords are browser/OS commands and never match.
 */
export function isHelpHotkey(e: KeyboardEvent): boolean {
  if (e.key !== 'F1') return false
  if (e.ctrlKey || e.metaKey || e.altKey || e.shiftKey) return false

  const target = e.target as HTMLElement | null
  if (target) {
    const tag = (target.tagName || '').toUpperCase()
    if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT' || target.isContentEditable) {
      return false
    }
  }
  return true
}
