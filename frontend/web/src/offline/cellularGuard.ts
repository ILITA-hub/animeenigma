// src/offline/cellularGuard.ts
// Wi-Fi-only default, part 2: reacts to connectivity-type changes. The engine
// dynamic-imports this at module scope, so this module MUST stay
// dependency-light: a static resolver/api-client chain would drag
// router+i18n into every test suite that imports the engine. Everything
// heavy is lazy-imported inside the functions.
import { onConnectionChange, isCellular, allowCellularThisSession, setAllowCellularThisSession } from './network'
import { listDownloads } from './registry'

let installed = false
/** Idempotent — the engine installs it on first load. */
export function ensureCellularGuard(): void {
  if (installed) return
  installed = true
  onConnectionChange(() => {
    if (isCellular()) {
      if (!allowCellularThisSession()) void import('./downloadEngine').then((m) => m.pauseAllForCellular()).catch(() => {})
    } else if (navigator.onLine) {
      void resumeNetworkPaused().catch(() => {})
    }
  })
}

/** Re-enqueue every record the guard parked (pausedBy:'network'), rebuilding
 *  the resolve closures from the persisted combo/subPref — the exact recipe
 *  of the store's manual resume. Returns how many were released.
 *  Known self-healing edges (manual resume recovers both): a cellular blip
 *  shorter than the in-flight segment can leave the active id parked with no
 *  later trigger; a worker's final paused-write can theoretically drop a
 *  concurrent pausedBy stamp. */
export async function resumeNetworkPaused(): Promise<number> {
  if (!navigator.onLine) return 0
  const [{ enqueueDownload, isEngineWorking }, { makeExternalSubResolver }, { useProviderResolver }] = await Promise.all([
    import('./downloadEngine'),
    import('./externalSubs'),
    import('@/composables/aePlayer/useProviderResolver'),
  ])
  const resolver = useProviderResolver()
  let n = 0
  for (const d of await listDownloads()) {
    if (d.pausedBy !== 'network' || d.state !== 'paused' || isEngineWorking(d.id)) continue
    await enqueueDownload({
      animeId: d.animeId, animeTitle: d.animeTitle, episode: d.episode, combo: d.combo,
      quality: d.quality, subPref: d.subPref,
      resolve: () => resolver.resolveStream(d.combo.provider, d.animeId, d.episode, d.combo),
      resolveSubs: makeExternalSubResolver(d.animeId, d.subPref)?.(d.episode),
    })
    n++
  }
  return n
}

/** «Качать по мобильным данным» — set the session override and release parked records. */
export async function allowCellularAndResume(): Promise<void> {
  setAllowCellularThisSession(true)
  await resumeNetworkPaused()
}
