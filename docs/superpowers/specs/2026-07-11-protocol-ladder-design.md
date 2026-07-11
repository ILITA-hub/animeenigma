# Protocol Ladder — FE-driven HTTP/3 → HTTP/2 → HTTP/1.1 fallback for segment delivery

**Date:** 2026-07-11 · **Status:** Approved (owner, in-chat)
**Metrics:** UXΔ = +3 (Better) · CDI = 0.05 * 13 · MVQ = Phoenix 90%/85%

## 1. Problem

2026-07-10 QUIC/HTTP-3 was enabled on `stream.animeenigma.org` (intended as a JP
long-haul mitigation, AUTO-571). Measured effect on the owner's JP↔DE path the
next day: segment throughput collapsed from **15–20 Mbps (h2+BBR, 07-09)** to
**~2.2 Mbps (h3, 07-11)** on identical-bitrate content (~5.3 Mbps ae RAW 1080p).
Root cause: nginx's QUIC stack has its own congestion controller — the host BBR
sysctls apply to **TCP only** — and on a ~250 ms lossy path it crawls.

The AePlayer made it worse: the silent-stall watchdog fires at 12 s whenever
zero fragments have *completed*, even while the first fragment is downloading
fine at 2 Mbps (needs ~17 s). It aborted the in-flight fragment, re-resolved the
same source, and looped forever — report `2026-07-11T03-23-51_tNeymik_feedback`
shows `segment_000.ts` restarted 3× (16.8 s / 15.9 s / 19.7 s), video never
started, player showed "stale".

Browsers give JS **no direct protocol choice** — protocol is negotiated
per-origin (ALPN + cached Alt-Svc). The FE-controllable lever is *which origin*
a request goes to. So the protocol ladder is an **origin ladder** where each
origin has a different protocol ceiling — the same shape as YouTube/Netflix
QoE-driven endpoint switching.

## 2. The streamX origin group

DNS A records already exist (owner-provisioned). All serve the identical
`/api/streaming/hls-proxy` reverse proxy; CORS is already `*`.

| Origin | Protocol ceiling | nginx shape |
|---|---|---|
| `stream3.animeenigma.org` | **h3** (+h2 fallback) | `listen 443 ssl http2` + `listen 443 quic` (NO `reuseport` — bare `stream.` owns it) + `http3 on` + `Alt-Svc h3` |
| `stream2.animeenigma.org` | **h2** | `listen 443 ssl http2`, no quic, no Alt-Svc |
| `stream1.animeenigma.org` | **http/1.1** | `listen 443 ssl`, no http2/http3 |
| `stream.animeenigma.org` (bare) | legacy | keeps quic reuseport listener; **drop its `Alt-Svc` headers** so legacy clients quietly return to h2 (cached upgrades expire ≤24 h). FE stops using it. |

Host-side work (NOT in git — host nginx configs are host-only; see
`project_streaming_quic_http3_nginx130`): 3 new vhosts cloned from the bare
`stream.` proxy locations + `certbot --nginx -d stream1… -d stream2… -d stream3…`.
Only one `quic reuseport` listener may exist per address — bare `stream.` keeps
it; `stream3` attaches with plain `listen 443 quic`.

**streamX origins provisioned 2026-07-11:** cert lineage
`/etc/letsencrypt/live/stream1.animeenigma.org/` (3 SANs, expires 2026-10-09);
`.pre-ladder-20260711-*.bak` backups beside every touched vhost. The legacy
socket-wide `listen 443 ssl http2` flags (animeenigma.ru, stream.animeenigma.ru)
were converted to per-server `http2 on;` — REQUIRED so stream1 can be
h1.1-only — and every other 443 vhost (animeenigma.org, stream.animeenigma.org,
bidberry, ext) got an explicit `http2 on;` so nothing lost h2 (gotcha: two
vhosts contain a certbot comment matching "listen 443 ssl", which silently
defeated a naive first-match sed — verify with curl per origin, not by echo).
Verified matrix: stream1 200/h1.1 · stream2 200/h2 · stream3 200/h2 TCP +
200/h3 QUIC + Alt-Svc · bare stream 200/h2, Alt-Svc gone · main domains h2.

## 3. Ladder module (`frontend/web/src/utils/protocolLadder.ts`)

Singleton store, no framework deps, unit-testable.

**Tiers** parsed from `VITE_HLS_PROXY_TIERS`
(`"h3=https://stream3.animeenigma.org,h2=https://stream2.animeenigma.org,h1=https://stream1.animeenigma.org"`).
Unset ⇒ single-tier ladder from existing `VITE_HLS_PROXY_BASE` (exact current
behavior — dev / self-hosters unaffected).

**State:** `activeTier` (entry: `h2`, or persisted last-known-good if <24 h
old), per-tier EWMA throughput, switch-trail ring buffer (tier, reason,
timestamp), h3 probe result. Persisted to `localStorage`
(`ae:protocolLadder:v1` = `{tier, ewma, probedH3, ts}`), invalidated after 24 h
or on `navigator.connection` `change` (where supported).

**API:**
- `currentBase(): string` — consumed by `hlsProxyUrl()`; all HLS playlists,
  segments, subtitle tracks, storyboards and the PWA download engine follow
  automatically (they all build URLs through `hlsProxyUrl`). Image proxy stays
  same-origin (unchanged).
- `reportFragment({bytes, ms, mediaDurationS, protocol})` — fed from hls.js
  `FRAG_LOADED` stats; `protocol` from
  `PerformanceResourceTiming.nextHopProtocol`. **Needed bitrate** is derived
  from content, not manifest: `bytes / mediaDurationS` EWMA (our library media
  playlists carry no `BANDWIDTH` attribute, so hls.js level bitrate is
  unreliable here).
- `reportTimeout()` / `reportFirstFragProgress({receivedBytes, elapsedMs, totalBytes})`
  — in-flight progress comes from an `onprogress` listener attached in hls.js
  `config.xhrSetup` (supported public hook; no custom loader).
- `onChange(cb)` — player subscribes; a tier change triggers the existing
  source-swap machinery (same path as edge rotation / server switch) resuming at
  `currentTime`.
- `debugSnapshot()` — everything hacker mode shows.

**Downshift policy** (h3→h2 and h2→h1, same signals):
- EWMA throughput < needed bitrate × **1.2** for **3 consecutive fragments**, or
- **2** fragment-load timeouts on the current tier, or
- first fragment *projected* completion > **8 s** while bytes are demonstrably
  trickling (progress-based, not silence-based).
On downshift: switch base, persist, record reason. h1 is the floor — below it
the normal source-failover chain (existing behavior) takes over.

**h3 upgrade probe** (once per session): after ~30 s of stable playback on h2,
background-fetch one upcoming segment from the h3 tier, measure throughput and
confirm `nextHopProtocol === "h3"`. Accept (switch + persist) only if ≥ **1.1×**
current tier EWMA; otherwise record the measurement (visible in hacker mode) and
re-probe next session. Sessions that persisted `h3` as last-known-good start
there directly.

## 4. Watchdog fix (the actual "stale" bug)

`armPlaybackWatchdog` (AePlayer.vue) currently treats "no fragment *completed*
in 12 s" as a silent stall. New rule: before declaring a stall, consult the
in-flight fragment's progress (via ladder's first-frag progress report). If
bytes are flowing, do **not** abort/re-resolve — defer to the ladder (its
projected-too-slow rule downshifts the tier instead). Only a genuinely dead
fetch (zero bytes) advances to the next source. This kills the observed
abort→re-resolve→restart-seg0 infinite loop.

## 5. Hacker mode (PlaybackSettingsMenu debugStats)

New rows, following the existing EDGE/TRY/ROT pattern — metrics + logic, not
just the decision:

```
PROTO h2 · tier 2/3
NET   4.1 Mbps ewma / need 5.4 ×1.2
LADDR h3→h2 (first-frag projected 17s ×1)
PROBE h3 2.1 Mbps @03:24 — rejected (<1.1× h2)
```

Rows render only when the ladder is multi-tier (prod); single-tier dev shows
nothing new.

## 6. Config & rollout

- FE env: `VITE_HLS_PROXY_TIERS` (new, Dockerfile build arg + compose), keeps
  `VITE_HLS_PROXY_BASE` as single-tier fallback.
- Rollout order: (1) nginx vhosts + certs live and verified (`curl --http3` /
  `--http2` / `--http1.1` per origin), (2) FE ships with tiers env set.
- Instant owner relief on deploy: fresh sessions enter on **h2**.
- Rollback: unset `VITE_HLS_PROXY_TIERS` → prior single-base behavior.

## 7. Testing

- Unit: tier parsing + env fallback; downshift triggers (EWMA margin, timeouts,
  first-frag projection); persistence TTL + connection-change invalidation;
  probe accept/reject; switch-trail bookkeeping.
- Watchdog regression: progressing first fragment must NOT re-resolve (the
  tNeymik loop, encoded as a test).
- debugStats spec: rows present in multi-tier, absent in single-tier.
- `/frontend-verify` before finishing; e2e smoke unchanged.

## 8. Explicitly out of scope

- hls.js custom loader (seamless mid-stream origin swap) — v2 if switch
  re-buffering (~2–6 s, ≤1×/session) ever matters.
- Multi-bitrate ae encodes (the real ABR fix for <5 Mbps paths — separate
  effort).
- Server-side QUIC tuning (nginx exposes no CC knob for QUIC).
- Removing bare `stream.`'s QUIC listener (kept for reuseport ownership +
  future re-use).
