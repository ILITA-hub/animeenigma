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
- **Dead code (leave untouched, out of scope):** `useSubtitleTimingOffset.ts` (persisted
  singleton) and `SubtitleSettingsMenu.vue` — not mounted in the live path.

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
}
```

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

**VAD.** `AnalyserNode` (`fftSize` ~2048) polled ~50 Hz (rAF-gated, only while playing):
- `getByteFrequencyData` → **speech-band energy** (≈300–3400 Hz) over total energy.
  Speech-band weighting rejects BGM/SFX/silence false positives better than raw RMS.
- Adaptive noise floor (running percentile) → boolean `speaking` per frame, timestamped by
  `videoElement.currentTime` (the *media* clock — robust to pauses/seeks/playback-rate).
- Append to a speech-activity timeline (sparse run-length intervals; bounded memory).

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
- Once a stable lock holds, drop to the low-rate monitor (CPU saver).
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

### 3.3 `components/player/aePlayer/SubtitlesMenu.vue` (touched) — the UI

- Add an **"Auto-sync subtitles"** toggle using the DS `Switch` primitive, placed **above**
  the existing Timing-offset row. Props in: `autoSync: boolean`; emits
  `update:autoSync(boolean)`.
- When auto-sync is ON, relabel the manual offset row to read as a **fine-tune** (copy only)
  and show the live detected offset under hacker mode (3.5).
- New i18n keys in **en / ru / ja** (parity-gated): `player.aePlayer.subs.autoSync` (label),
  `…subs.autoSyncHint`, plus hacker-mode readout strings (3.5). No off-palette colors / DS
  rule compliance (Switch is a DS primitive ⇒ no Rule-5 native-control violation).

### 3.4 `components/player/aePlayer/AePlayer.vue` (touched) — wiring

- **Parse cues once for alignment.** Add a small reactive `subtitleCues` derived from
  `chosenSubUrl` + `chosenSubFormat` via `subtitle-parser.ts` (fetch is browser-cached, the
  overlay already fetched it). Additive — `SubtitleOverlay.vue` stays untouched (it keeps
  parsing internally for render). If duplicate fetch/parse proves wasteful later, lift parse
  into a shared `useSubtitleCues` composable consumed by both; not required for v1.
- Build `episodeKey` as `"{anime.uuid}:{selectedEpisode.number}"`.
- `const pref = useSubtitleAutoSyncPref(episodeKey)`.
- `const sync = useSubtitleAutoSync({ videoElement, cues: subtitleCues, enabled: pref.enabled, episodeKey })`.
- **Apply:** pass `:offset="effectiveOffset"` where
  `effectiveOffset = computed(() => sync.autoOffset.value + state.subOffset.value)`.
  Manual slider remains a delta on top of the auto result.
- Wire `SubtitlesMenu` `:auto-sync="pref.enabled.value"` / `@update:auto-sync="pref.setEnabled"`.

### 3.5 Hacker-mode debug readout

When `state.hackerMode` is on, the Subtitles menu shows: `status`, `autoOffset` (s), and
`confidence` (%). Lets the owner verify the engine is locking. Pure display; gated; no
effect when hacker mode is off.

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
- Dead `useSubtitleTimingOffset` / `SubtitleSettingsMenu`: **left alone**.
- Manual-offset persistence (currently ephemeral): **out of scope** (separate concern).

## 6. Risks & mitigations

1. **`createMediaElementSource` reroutes audio → volume/mute break.** Mitigation: GainNode
   mirror of `videoElement.volume`/`muted` (3.1). Verified by a unit test on the gain sync.
2. **HLS (hls.js / MediaSource blob) tap.** blob: URL is same-origin ⇒ analyzable. If a
   future provider serves cross-origin un-proxied media ⇒ tainted ⇒ `unsupported` no-op.
3. **Warm-up latency** (realtime tap learns from played audio). Accepted for v1; a future
   offline-decode head-start (range-fetch + `OfflineAudioContext`) can shorten it.
4. **Mis-sync risk** on low-dialogue/musical openings. Mitigation: confidence gate + speech
   minimum ⇒ no-op rather than a wrong jump.
5. **CPU.** ~50 Hz FFT + bounded correlation is light; drop to low-rate monitor after lock.

## 7. Testing (Vitest; audio graph stubbed)

- **Pref:** default `true` when missing; default `true` when expired (>24 h); per-episode
  isolation (disabling ep A leaves ep B default-on); 24 h TTL boundary; SSR/quota fallback.
- **Correlation:** recovers a known constant offset from synthetic speech+cue timelines;
  sign convention (subs-early ⇒ positive offset); confidence gate ⇒ no-op below threshold;
  warm-up gate ⇒ no-op below `MIN_SPEECH`.
- **Step re-sync:** a synthetic mid-timeline jump is detected and the segment offset adopted.
- **Volume/mute mirror:** gain tracks `videoElement.volume`/`muted`.
- **Apply path:** `effectiveOffset = autoOffset + subOffset`; auto OFF ⇒ equals `subOffset`.
- **Menu:** Switch reflects + emits pref; hacker readout shows status/offset/confidence.

Gates (per project): `bunx vitest run`, `bunx vue-tsc --noEmit`, DS-lint (0 errors),
real `bun run build`, i18n en/ru/ja parity — all via `/frontend-verify`.

## 8. Metrics (project convention)

- **UXΔ = +3 (Better)** — meaningful QoL for subtitle watchers; tempered by warm-up +
  mis-sync risk.
- **CDI = 0.03 * 13** — localized, mostly-additive spread (2 new composables + small edits
  to `AePlayer.vue` / `SubtitlesMenu.vue` + i18n + tests); the VAD/correlation engine is the
  real effort (Fibonacci 13). Not pre-multiplied.
- **MVQ = Griffin 85%/80%** — precise, self-contained signal-processing unit grafted cleanly
  onto the existing offset path.

## 9. Out of scope / future (phase C seam)

- Backend `ffsubsync`-style precompute feeding `autoOffset` (exact fps-scale + full-episode
  segments), cached per (stream, subtitle). The `useSubtitleAutoSync` output contract
  (`autoOffset`/`status`/`confidence`) is the seam it would plug into.
- Offline-decode head-start to remove warm-up latency.
- Porting auto-sync to any future non-aePlayer subtitle surface.
