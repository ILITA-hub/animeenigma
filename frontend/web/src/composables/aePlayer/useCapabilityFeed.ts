import { computed, type ComputedRef, type Ref } from 'vue'
import { useCapabilities, flattenCapabilities } from '@/composables/aePlayer/useCapabilities'
import { rowsFromReport } from '@/composables/aePlayer/useProviderFeed'
import { offlineCapabilityReport } from '@/offline/offlineAdapter'
import type { OfflinePlayback } from '@/offline/offlineAdapter'
import type { PlayerState } from '@/composables/aePlayer/usePlayerState'
import type { CapabilityReport, ProviderCap } from '@/types/capabilities'
import type { ProviderRow } from '@/types/aePlayer'
import type { VerifyReport } from '@/types/contentVerify'

// ── The capability feed (backend single source of truth) ────────────────────
// The backend capability feed (/api/anime/{id}/capabilities) is the single
// source of truth for which providers the player may use/show: state, order,
// audios, group, selectable/hacker-only all arrive from it. `rows` is a pure
// derivation of the report + the active audio/lang/content filter — no FE-side
// registry, health poll, or availability probe. Disabled providers are omitted
// backend-side; `ae` with no local copy arrives as state:'no_content'.

export interface CapabilityFeedDeps {
  animeIdRef: ComputedRef<string> | Ref<string>
  /** Reactive offline-bundle read (the useCapabilities guard below samples it
   *  once at setup; the report/capMap computeds track it live — both matching
   *  the original inline wiring). */
  getOffline: () => OfflinePlayback | null | undefined
  isHentai: () => boolean
  state: PlayerState
  /** Content-verify probe report (Task 13/14) gating which audios each
   *  non-firstparty row may claim — see `verifiedCaps.ts`. A null return
   *  (no report yet, or poll inactive) treats every non-firstparty row as
   *  unverified/RAW-only, per `effectiveAudios`. */
  getVerify: () => VerifyReport | null
}

export function useCapabilityFeed(deps: CapabilityFeedDeps) {
  const { getOffline, state } = deps

  const filter = computed(() => ({
    audio: state.combo.value.audio,
    lang: state.combo.value.lang,
    content: (deps.isHentai() ? 'hentai' : 'common') as 'hentai' | 'common',
  }))

  // Guard point 2 — capability feed. Offline: a synthetic one-provider report
  // ('offline'), and NO network fetch/poll fires — useCapabilities (whose
  // immediate watch triggers the /capabilities GET) is never constructed. Live
  // (every existing usage): identical to before — useCapabilities runs with the
  // same immediate fetch, and `report`/`capMap` transparently forward its refs
  // (same object identity, same reactivity timing).
  const cap = getOffline() ? null : useCapabilities(deps.animeIdRef)
  const report = computed<CapabilityReport | null>(() => {
    const offline = getOffline()
    return offline ? offlineCapabilityReport(offline) : (cap?.report.value ?? null)
  })
  const capMap = computed<Map<string, ProviderCap>>(() =>
    getOffline() ? flattenCapabilities(report.value) : (cap?.capMap.value ?? new Map()),
  )
  const rows = computed<ProviderRow[]>(() =>
    rowsFromReport(report.value, filter.value, deps.getVerify()),
  )

  // ── Active provider display info ──────────────────────────────────────────
  // Name from the capability feed (display_name) → row label → raw id. Cosmetics
  // are state-driven, not per-provider: the active dot is always brand cyan.

  const activeProviderName = computed(() => {
    const id = state.combo.value.provider
    return capMap.value.get(id)?.display_name ?? rows.value.find((r) => r.id === id)?.label ?? id ?? ''
  })

  const activeProviderHue = computed(() => 'var(--brand-cyan)')

  return { report, capMap, rows, activeProviderName, activeProviderHue }
}
