// Third subtitle-pref persistence model in the player, by design: the dead global-sticky
// useSubtitleTimingOffset and the live ephemeral usePlayerState.subOffset are the others.
// Per-episode + 24h TTL = a "decaying opt-out": a local disable (auto-sync misfired on one
// episode) self-heals after a day rather than permanently killing a default-good feature.
// Expiry is read-time only (expired keys aren't evicted — they're tiny).
import { ref, watch, type Ref } from 'vue'

export const DAY_MS = 24 * 60 * 60 * 1000
const PREFIX = 'aenigma_subautosync_'

function read(key: string): boolean {
  if (typeof window === 'undefined') return true
  try {
    const raw = localStorage.getItem(PREFIX + key)
    if (!raw) return true
    const parsed = JSON.parse(raw) as { value?: unknown; expiresAt?: unknown }
    if (typeof parsed.expiresAt !== 'number' || Date.now() > parsed.expiresAt) return true
    return parsed.value !== false   // anything but an explicit false → on
  } catch { return true }
}

function write(key: string, value: boolean): void {
  if (typeof window === 'undefined') return
  try { localStorage.setItem(PREFIX + key, JSON.stringify({ value, expiresAt: Date.now() + DAY_MS })) }
  catch { /* quota / disabled storage — in-memory only */ }
}

export function useSubtitleAutoSyncPref(episodeKey: Ref<string>) {
  const enabled = ref<boolean>(read(episodeKey.value))
  watch(episodeKey, (k) => { enabled.value = read(k) })
  function setEnabled(v: boolean): void { enabled.value = v; write(episodeKey.value, v) }
  return { enabled, setEnabled }
}
