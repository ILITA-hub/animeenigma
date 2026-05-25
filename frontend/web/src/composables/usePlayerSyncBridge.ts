/**
 * Workstream watch-together — Phase 3 (player-sync) Plan 03.1.
 *
 * STUB — full implementation lands in the GREEN commit of this plan. The
 * named export must already exist so the matching `.spec.ts` can `import` it
 * (and fail at runtime, not at import-resolve time).
 */
import type { Ref } from 'vue'
import type { WatchTogetherRoomHandle } from '@/composables/useWatchTogetherRoom'

export function usePlayerSyncBridge(
  _videoRef: Ref<HTMLVideoElement | null>,
  _room: WatchTogetherRoomHandle,
): void {
  // Intentionally empty — real implementation in the GREEN commit.
}
