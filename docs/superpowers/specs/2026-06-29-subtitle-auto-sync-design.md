# Auto-sync subtitles (frontend Web Audio) — Design

- **Date:** 2026-06-29
- **Owner request (feedback `2026-06-25T03-55-10_tNeymik_manual`):** «Нужно сделать чтобы субтитры автоматически подстраивались под плейбек» — subtitles should automatically adjust to the playback.
- **Status:** Approved (design), spec written for plan.
- **Surface:** Frontend only (`frontend/web`). No backend, no new service, no env vars.

## 1. Problem & intent

External subtitle files (Jimaku, OpenSubtitles, provider-bundled) are timed for a
specific encode. The streamed source (gogoanime / animepahe / allanime / miruro /
nineanime / …) is frequently a *different* encode, so the subtitles drift from the
spoken audio. Today the only remedy is the **manual timing-offset** slider
(`usePlayerState.subOffset`, applied by `SubtitleOverlay.vue` as `currentTime - offset`).

The owner wants this to happen **automatically**.

### Drift model (from owner)

1. **Constant offset** — *dominant case.* The whole file is shifted by a fixed amount
   (different intro/encode start). A single nudge fixes the whole episode.
2. **Step / eyecatch discontinuity** — a commercial bumper / eyecatch present in the
   subtitle file but not the video (or vice-versa) inserts a mid-episode jump. Subs are
   in sync, then off by the bumper length after the boundary. Either direction.
3. **Linear framerate drift** (23.976 vs 25 fps) — theoretically possible, unconfirmed,
   **out of v1 scope** (only approximated by periodic re-anchoring; an exact scale solve
   is a possible future backend phase).

### Chosen approach

**Approach A — frontend, progressive Web Audio.** The streamed audio is same-origin
(proxied through the backend HLS proxy), so the browser can tap the playing `<video>`
audio via the Web Audio API without CORS tainting. We derive a Voice-Activity-Detection
(VAD) "someone is speaking" timeline from the audio, cross-correlate it against the
subtitle cue timeline, and apply the resulting offset automatically — on top of the
existing manual offset, which remains a manual override.

Rejected for v1: **B (backend ffsubsync-style)** — most accurate (full-episode decode,
solves fps drift) but heavy (per-episode ffmpeg decode, new pipeline, slow first run);
**C (hybrid)** — best end state but largest scope. The engine is designed so a backend
precompute (C) can later feed the same apply path without rework.

## 2. Current wiring (as-is)

- `SubtitleOverlay.vue` — the **only** live subtitle renderer; rendered solely by
  `AePlayer.vue`. Time-synced to `videoElement.currentTime` via `requestAnimationFrame`;
  applies `:offset` as `t = currentTime - offset` (positive offset ⇒ subs shown later).
  aePlayer is also the RAW/JP surface, so this covers EN/RU/JP subtitle cuts.
- `usePlayerState.subOffset` — an **ephemeral** `ref(0)` (not persisted). Manual slider in
  `aePlayer/SubtitlesMenu.vue` emits `update:sub-offset`; `AePlayer.vue` writes
  `state.subOffset.value`; passed to overlay as `:offset="state.subOffset.value"`.
- Subtitle state: `state.subLang` (`off` disables), `chosenSubUrl`, `chosenSubFormat`,
  `subtitleTracks` (merged provider-bundled + aggregated). Cues are parsed *inside*
  `SubtitleOverlay.vue` via `subtitle-parser.ts`.
- Episode identity: `selectedEpisode.number` + `anime` uuid.
- **Retired follow-up:** the unmounted `useSubtitleTimingOffset.ts` singleton and
  `SubtitleSettingsMenu.vue` were removed in the 2026-07-15 cleanup wave.

## 3. Units (new + touched)

### 3.1 `composables/aePlayer/useSubtitleAutoSync.ts` (new) — the engine

The self-contained align unit. No Vue-component coupling; testable with synthetic
fixtures.

**Input (reactive):**
```ts
useSubtitleAutoSync({
  videoElement: Ref<HTMLVideoElement | null>,
  cues: Ref<SubtitleCue[]>,        // parsed cues for the active track (see 3.4)
  enabled: Ref<boolean>,           // from the per-episode pref (see 3.2)
  episodeKey: Ref<string>,         // changes => reset accumulation
})
```
**Output:**
```ts
{
  autoOffset: Ref<number>,         // seconds, same sign convention as manual
  status: Ref<'idle' | 'listening' | 'locked' | 'unsupported'>,
  confidence: Ref<number>,         // 0..1, peak prominence of the chosen offset
  syncEvents: Ref<SyncEvent[]>,    // bounded change-log (last N, e.g. 10), newest-first
}

type SyncEvent = {
  delta: number,                   // offset change applied (s), signed
  confidence: number,              // 0..1 of the chosen offset
  windowStart: number,             // analyzed speech-window interval (s) that produced it
  windowEnd: number,
  reason: 'lock' | 'resync',       // initial lock vs. step/eyecatch re-sync
}
```
> Trimmed to what the hacker-mode log actually surfaces (delta + interval + confidence).
> Earlier drafts also carried `atMediaTime`/`fromOffset`/`toOffset`; `atMediaTime` always
> equalled `windowEnd` and the absolute offsets were never read (the current absolute value
> shows in the live state line), so they were dropped.

A `SyncEvent` is pushed on every `autoOffset` change — the initial lock (`reason='lock'`)
and each step/eyecatch re-sync (`reason='resync'`). The list is bounded (newest-first,
~10) and reset on `episodeKey` change. This is the data behind the hacker-mode log (3.5).

**Audio tap.** On first enable with a usable `videoElement`:
- `const ctx = new AudioContext()`
- `const src = ctx.createMediaElementSource(videoElement)` (once per element; the engine
  is the single owner)
- **Preserve player volume/mute** (#1 integration risk — `createMediaElementSource`
  reroutes the element's output): `src → gain → ctx.destination`, where `gain.gain` mirrors
  `videoElement.muted ? 0 : videoElement.volume` (kept in sync via the element's
  `volumechange` event).
- `src → analyser` (parallel branch) for analysis.
- If `createMediaElementSource` throws / `AudioContext` unavailable / media tainted ⇒
  `status='unsupported'`, **no-op** (subs unchanged).

**VAD.** `AnalyserNode` (`fftSize` ~2048), rAF-driven but **throttled to ~20 Hz** (VAD
segments are ≫50 ms, so 20 Hz is ample and ~3× cheaper than display-rate; only while
playing & not seeking):
- The frame decision is a **pure, unit-tested** function `classifyFrame(freq, sampleRate,
  fftSize)` in `subtitleAlign.ts` (next to the math), so the thresholds that determine what
  counts as speech are testable — not buried in the impure tap. It does a **single pass** over
  the bins computing both mean energy and the **speech-band ratio** (≈300–3400 Hz), and
  returns `speaking` when mean ≥ floor AND ratio ≥ `VAD_RATIO`. Speech-band weighting rejects
  BGM/SFX/silence better than raw RMS. The tap (`subtitleAudioTap.ts`) is then thin Web-Audio
  I/O glue that calls it.
- **v1 uses a fixed energy floor** (a deliberate simplification of the spec's earlier
  "adaptive running-percentile floor"); a later adaptive floor can drop into the pure layer
  without touching the tap. Each `speaking` frame is timestamped by `videoElement.currentTime`
  (the *media* clock — robust to pauses/seeks/playback-rate).
- Append to a speech-activity timeline (run-length intervals), kept to a **sliding recent
  window** (e.g. ~120 s) so per-evaluation cost and memory stay bounded over a long episode
  (alignment is local; old history adds nothing).

**Correlation.**
- Build a "subtitle active" boolean timeline from `cues` (`[start,end]` intervals).
- For candidate offsets δ ∈ [−30, +30]s @ 0.1 s step: score = temporal overlap between the
  speech timeline and the subtitle timeline shifted by δ (intersection-over-union of active
  intervals on the analyzed window).
- `bestδ = argmax(score)`. **Confidence** = `(peak − secondBest) / peak` style prominence.
- **Lock gate:** apply `autoOffset = bestδ` only when (a) accumulated speech ≥ `MIN_SPEECH`
  (warm-up, e.g. ~20 s of detected speech) AND (b) `confidence ≥ CONF_MIN`. Otherwise leave
  `autoOffset = 0` (no-op) and `status='listening'`.

**Sign convention.** `autoOffset` uses the overlay convention: positive ⇒ subs shown
later. If subtitles currently appear *before* the speech, `bestδ` is positive. Pinned by a
unit test against synthetic timelines.

**Step / eyecatch handling.** After lock, keep a low-rate rolling alignment-quality monitor
at the current offset. On persistent degradation (recent cues consistently mis-aligned by a
similar new delta over a trailing window), re-run correlation on that trailing window; if a
new high-confidence offset emerges, adopt it as the current segment offset (piecewise).
This serves the bumper case without a global model.

**Lifecycle / safety.**
- `episodeKey` change ⇒ reset timeline + `autoOffset=0` + `status='idle'`; re-arm.
- `enabled=false` ⇒ stop polling, `autoOffset=0` (manual-only behavior returns).
- CPU is bounded by the ~20 Hz tick throttle + the sliding speech window (a per-lock dynamic
  downshift was considered and dropped — it needed an engine→tap rate channel for marginal
  gain once the window already caps sweep cost).
- `onUnmounted` / `enabled=false` ⇒ disconnect nodes, `ctx.close()`.
- **Never makes subs worse:** any failure/uncertainty ⇒ `autoOffset` stays `0`.

### 3.2 `composables/aePlayer/useSubtitleAutoSyncPref.ts` (new) — the toggle

Per-episode persisted preference (owner spec: localStorage, episode-only, 24 h TTL,
default `true`).
```ts
useSubtitleAutoSyncPref(episodeKey: Ref<string>)
  -> { enabled: Ref<boolean>, setEnabled(v: boolean): void }
```
- Storage key: `aenigma_subautosync_{animeId}_{ep}`.
- Stored value: `{ value: boolean, expiresAt: number }` (epoch ms; `expiresAt = now + 24h`).
- Read: missing OR `now > expiresAt` ⇒ **default `true`**. `setEnabled` rewrites `expiresAt`.
- SSR-guard (`typeof window`), try/catch quota/disabled storage ⇒ falls back to in-memory
  default `true`.
- Reactive to `episodeKey` (re-read on episode switch).
- The TTL is **read-time only** (expired keys aren't evicted — they're tiny and harmless).
  This is the **third** subtitle-pref persistence model alongside the dead global-sticky
  the former persisted timing singleton and the live ephemeral `usePlayerState.subOffset`; the per-episode
  + decaying-opt-out shape is deliberate, so add a one-line comment saying so.

### 3.3 `components/player/aePlayer/SubtitlesMenu.vue` (touched) — the UI

- Add an **"Auto-sync subtitles"** toggle using the DS `Switch` primitive, placed **above**
  the existing Timing-offset row. Props in: `autoSync: boolean`; emits
  `update:autoSync(boolean)`. The manual offset stays as-is (a fine-tune delta on top of the
  auto result) — no relabel; it already works unchanged.
- New i18n keys in **en / ru / ja** (parity-gated): `player.aePlayer.subs.autoSync` (label),
  `…subs.autoSyncHint`, plus hacker-mode readout strings (3.5). No off-palette colors / DS
  rule compliance (Switch is a DS primitive ⇒ no Rule-5 native-control violation).

### 3.4 Cue fetch/parse (shared) + `AePlayer.vue` wiring (touched)

- **One cue fetch/parse path, not two.** Extract `fetchAndParseCues(url, format, signal?)`
  into `@/utils/subtitle-parser.ts` (it already owns `parseASS/parseSRT/parseVTT` + the
  format auto-detect; it gains the `hlsProxyUrl` URL rule from `@/utils/streaming`).
  `SubtitleOverlay.loadSubtitles` is refactored to **delegate** to it (keeping its own
  `emit('loading'|'error')` + `AbortController` wrapper — behavior-preserving), and the new
  `useSubtitleCues(url, format)` composable calls the same helper. This kills the
  fetch/format-sniff fork (the real risk is the two copies drifting so the engine aligns a
  *different* cue set than the overlay renders). aePlayer is the overlay's **only** consumer,
  so this has no cross-player blast radius.
- In `AePlayer.vue`: `const { cues: subtitleCues } = useSubtitleCues(chosenSubUrl, chosenSubFormat)`.
- Reuse the **existing** `subEpisode` computed for episode identity:
  `episodeKey = computed(() => `${props.animeId}:${subEpisode.value}`)` (don't re-derive
  `selectedEpisode.number ?? anime.ep`).
- `const pref = useSubtitleAutoSyncPref(episodeKey)`.
- `const sync = useSubtitleAutoSync({ videoElement, cues: subtitleCues, enabled: computed(() => pref.enabled.value && subsOn.value), episodeKey })`.
- **Apply:** pass `:offset="effectiveOffset"` where
  `effectiveOffset = computed(() => sync.autoOffset.value + state.subOffset.value)`.
  Manual slider remains a delta on top of the auto result.
- Wire `SubtitlesMenu` `:auto-sync="pref.enabled.value"` / `@update:auto-sync="pref.setEnabled"`.

### 3.5 Hacker-mode debug readout

When `state.hackerMode` is on, the Subtitles menu shows a debug panel:

- **Current state line:** `status` · current **auto-offset** (s, signed) · `confidence` (%).
- **VAD change-log:** the `syncEvents` list (newest-first), one row per change, e.g.
  `Changed by +1.2s (VAD) @ 10:00–12:30` — i.e. `delta` (signed), the `reason` (VAD
  lock/resync), and the analyzed `windowStart–windowEnd` interval as `mm:ss`. Showing the
  interval makes step/eyecatch re-syncs legible (you see *when* and *over what window* the
  correction was decided). Confidence per row shown as a trailing `(NN%)`.

Pure display; gated on `hackerMode`; no effect when hacker mode is off. Empty log while
`listening`/`idle`. i18n keys under `player.aePlayer.subs.autoSyncDebug.*` (en/ru/ja parity).

## 4. Manual-offset interaction (explicit)

| Auto-sync | Effective offset | Behavior |
|-----------|------------------|----------|
| OFF       | `subOffset`      | Exactly today's behavior (manual only). |
| ON, listening / unsupported | `0 + subOffset` | No auto correction yet; manual still works. |
| ON, locked | `autoOffset + subOffset` | Auto drives; manual is a fine-tune delta on top. |

Turning auto-sync OFF instantly drops `autoOffset` to 0 (manual value preserved).

## 5. Scope guards (YAGNI)

- **Frontend only.** No backend, service, DB, or env var. Deploy = `make redeploy-web`.
- **aePlayer only** (the sole live `SubtitleOverlay` consumer).
- Linear-fps exact solve: **deferred** (approximated by re-anchoring).
- `SubtitleOverlay.vue`: **only** its fetch/parse is refactored to call the shared
  `fetchAndParseCues` helper (behavior-preserving) — no render/teleport/offset changes.
- Dead persisted timing singleton/menu: **retired 2026-07-15**.
- Manual-offset persistence (currently ephemeral): **out of scope** (separate concern).
- Post-lock dynamic tick downshift / `bestOffset` window-bounding: **deferred** — the fixed
  ~20 Hz throttle + sliding speech window already bound cost; both add cross-layer/dynamic
  complexity for marginal gain.

## 6. Risks & mitigations

1. **`createMediaElementSource` reroutes audio → volume/mute break.** Mitigation: GainNode
   mirror of `videoElement.volume`/`muted` (3.1). Verified by a unit test on the gain sync.
2. **HLS (hls.js / MediaSource blob) tap.** blob: URL is same-origin ⇒ analyzable. If a
   future provider serves cross-origin un-proxied media ⇒ tainted ⇒ `unsupported` no-op.
3. **Warm-up latency** (realtime tap learns from played audio). Accepted for v1; a future
   offline-decode head-start (range-fetch + `OfflineAudioContext`) can shorten it.
4. **Mis-sync risk** on low-dialogue/musical openings. Mitigation: confidence gate + speech
   minimum ⇒ no-op rather than a wrong jump.
5. **CPU.** ~20 Hz FFT (throttled) + correlation over a sliding speech window (not the whole
   episode) keeps both per-frame and per-evaluation cost bounded and flat over time.

## 7. Testing (Vitest; audio graph stubbed)

- **Pref:** default `true` when missing; default `true` when expired (>24 h); per-episode
  isolation (disabling ep A leaves ep B default-on); 24 h TTL boundary; SSR/quota fallback.
- **Correlation:** recovers a known constant offset from synthetic speech+cue timelines;
  sign convention (subs-early ⇒ positive offset); confidence gate ⇒ no-op below threshold;
  warm-up gate ⇒ no-op below `MIN_SPEECH`.
- **Classifier (`classifyFrame`):** high speech-band energy ⇒ `speaking=true`; out-of-band
  or sub-floor energy ⇒ `false` (the thresholds that drive every produced offset are tested).
- **Step re-sync:** a synthetic mid-timeline jump is detected and the segment offset adopted.
- **Change-log:** `syncEvents` records an entry on initial lock (`reason='lock'`) and on
  step re-sync (`reason='resync'`) with correct `delta`/`windowStart..End`; list is bounded
  (newest-first) and reset on `episodeKey` change.
- **Volume/mute mirror:** gain tracks `videoElement.volume`/`muted`.
- **Apply path:** `effectiveOffset = autoOffset + subOffset`; auto OFF ⇒ equals `subOffset`.
- **Menu:** Switch reflects + emits pref; hacker readout shows status/offset/confidence.

Gates (per project): `bunx vitest run`, `bunx vue-tsc --noEmit`, DS-lint (0 errors),
real `bun run build`, i18n en/ru/ja parity — all via `/frontend-verify`.

## 8. Metrics (project convention)

- **UXΔ = +3 (Better)** — meaningful QoL for subtitle watchers; tempered by warm-up +
  mis-sync risk.
- **CDI = 0.03 * 13** — localized, mostly-additive spread (pure-math module + Web-Audio tap +
  3 composables + a shared `fetchAndParseCues` helper + small edits to `AePlayer.vue` /
  `SubtitlesMenu.vue` / `SubtitleOverlay.vue` delegation + i18n + tests); the VAD/correlation
  engine is the real effort (Fibonacci 13). Not pre-multiplied.
- **MVQ = Griffin 85%/80%** — precise, self-contained signal-processing unit grafted cleanly
  onto the existing offset path.

## 9. Out of scope / future (phase C seam)

- Backend `ffsubsync`-style precompute feeding `autoOffset` (exact fps-scale + full-episode
  segments), cached per (stream, subtitle). The `useSubtitleAutoSync` output contract
  (`autoOffset`/`status`/`confidence`) is the seam it would plug into.
- Offline-decode head-start to remove warm-up latency.
- Porting auto-sync to any future non-aePlayer subtitle surface.
