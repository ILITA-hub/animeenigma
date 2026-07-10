# Playback resilience for constrained browsers — design

**Date:** 2026-07-10
**Branch:** `worktree-autoplay-blocked-fix` (single branch: the already-built autoplay-overlay fix + this bundle ship together)
**Status:** design approved (forks resolved 2026-07-10), pending spec review → implementation plan

## 1. Context & problem

User `@gerahertz` filed three "stream unavailable" reports (2026-06-22, 2026-07-10 ×2) for streams that were provably healthy server-side — every `/api/streaming/hls-proxy` request during his sessions returned 200/206 with real bytes, and the same episode played for other users. Diagnosis was a multi-hour server-log archaeology session because we had **no client-side signal**. Root cause was client-side: media downloaded for every source in the failover chain but playback never started; his console showed the useless `[Unhandled Promise Rejection] {}` (a DOMException JSON-stringifies to `{}`).

That surfaced a class of structural weaknesses in our **self-hosted** player that ordinary iframe-embedding anime sites don't share:

1. `video.play()` is sometimes called async, outside the user-gesture window → autoplay veto (`NotAllowedError`), previously swallowed.
2. `crossorigin="anonymous"` on `<video>` (needed for VAD auto-sync's capture-stream audio tap) forces CORS-mode media fetches; a CORS-stripping middlebox/antivirus/extension makes all our media unreadable while ordinary no-cors `<video>` plays fine.
3. Proxy URLs embed the CDN hostname and a `proxy?url=` shape (`hls-proxy?url=https%3A%2F%2Fp12.solodcdn.com…`) — a bullseye for uBlock-style network filters, silently, for **all** users. (Gera's `/api/analytics/collect` requests were all status 0 — `/analytics/` is on every filter list.)
4. Browser privacy hardening (Firefox strict ETP, `resistFingerprinting`, MSE/Web-Audio-breaking extensions) hits self-hosted MSE players harder than iframe embeds.

**Prime directive:** playback must never fail because an *enhancement* failed. Enhancements degrade individually; core video is sacred.

## 2. Scope

**In scope**
- The already-built autoplay-overlay fix (documented in §7, folded into this branch).
- Track A — opaque HLS proxy tokens (always-on, all users).
- Track B — detect-then-act resilience for constrained browsers (probe → gate → graceful-disable → notify), reactive `crossorigin` fallback, and masked analytics fallback.
- Telemetry to make the next report self-diagnosing.

**Out of scope**
- Muted-autoplay start ("no shenanigans" — the click-to-play overlay already covers autoplay veto).
- A "compatibility mode" toggle — only graceful fallbacks.
- Rich cause-specific wording on the terminal error panel ("autoplay blocked / extensions block our video / network to host slow") — **nice-to-have**, layered later on Track B's signals.

**Activation rule (core constraint):** every Track B *action* fires **only for users where a problem is detected**. The probe runs for everyone (it's how we find them) and emits one compact capability event per session (for a first-party denominator), but the behavioural changes — feature disable, popup, reactive crossorigin, masked endpoint — only trigger on an actual failure/blocked signal. **The only unconditional-for-all change is Track A (opaque HLS tokens).**

## 3. Track A — opaque HLS proxy tokens (always-on)

### Goal
Remove the CDN hostname and the `proxy?url=` query shape from every media URL so static filter lists cannot match them. Applies to all users.

### Token
`{ upstream_url, referer, exp }` → **authenticated encryption** (AES-GCM with a server-side secret derived from an existing env secret; random nonce prepended) → URL-safe base64 → single opaque token. Opaque (hostname unreadable by the client, DOM, or a filter list) **and** unforgeable/tamper-evident (supersedes today's `exp`+`sig` query pair). "Encoded with a secret", per the directive.

### URL shape (path-shaped, no bait words)
```
/api/streaming/m/<token>/<leaf>
```
`<leaf>` = the last path segment of the upstream (e.g. `seg-001.ts`, `manifest.m3u8`, `mon.key`) — preserved only so player heuristics keying off the extension keep working; it carries no hostname and the token is authoritative. No `hls-proxy`, no `url=`, no query-string shape → defeats path-based filters too.

### Backend changes
- **`libs/videoutils/streamtoken.go`** (new): `Encode(payload) → token`, `Decode(token) → payload` (validates the GCM tag + `exp`). Pure, unit-tested (round-trip, tamper, expiry).
- **`libs/videoutils/proxy.go`**: new handler for `/m/<token>/<leaf>` — decode → the existing fetch/referer/rewrite pipeline unchanged. The m3u8 rewriter (`rewriteM3U8URLs`/`rewriteHLSURL`/`rewriteURIAttribute`) emits the path-shaped token form for every segment, key, and child playlist. **Dual-accept:** keep the legacy `?url=&referer=&exp=&sig=` handler alive during rollout; existing signed URLs are cached ≤1h, so after ~1h everything is new-form. Remove the legacy handler in a follow-up after a safe window.
- **`services/catalog` streamsign**: mint the path-shaped token where it currently signs `sources[].url` / subtitle `tracks[].file`.

### Notes
- This is `libs/videoutils` → per repo rules, redeploy **all** Go services that import it, and sweep every `go.work` Dockerfile that COPYs the lib. No `go work sync`.
- The static `HLSProxyAllowedDomains` allowlist is unaffected (the token path is first-party and self-authorizing).

## 4. Track B — detect-then-act (constrained browsers only)

### B1. Capability probe (`capabilityProbe.ts`, pure + testable)
On player mount, cheaply probe the enhancement prerequisites:
- **MSE** — `MediaSource`/`ManagedMediaSource` constructible + `isTypeSupported` for our codecs.
- **Web Audio** — `AudioContext` constructible + resumable (VAD auto-sync prerequisite).
- **captureStream** — `video.captureStream`/`mozCaptureStream` present (VAD audio tap prerequisite).
- **canvas 2D** — storyboard/scrub-preview rendering.
- **analytics reachability** — a tiny fetch to a normal `/api/analytics/*` probe; failure ⇒ analytics blocked (doubles as adblock detection; drives B5).
- (nice-to-have) network-to-video-host latency sample.

Returns a `CapabilityReport`. **Emits one compact all-capabilities event per session** (chosen fork) for a first-party denominator; every probe result is boolean, no payload bloat.

### B2. Gate + graceful disable (`useClientCapabilities.ts`)
Each enhancement binds to its capability **and** is try/caught so a runtime throw also flips the capability off (belt + suspenders):
- **VAD auto-sync** needs Web Audio + captureStream. Off ⇒ subtitles still render, only auto-alignment is disabled.
- **Storyboard scrub previews** need canvas (+ SW cache). Off ⇒ scrubbing still works, no thumbnails.
- **`preload`** — if the browser/policy rejects it ⇒ drop to `metadata`/`none`.
- **MSE** — core, not an "enhancement". Off is extremely rare; the probe feeds the diagnostic banner and biases source selection toward natively-playable sources, but HLS-without-MSE playback is only possible where native HLS exists (Safari). No compat shim.

Every caught failure emits a `degrade` telemetry event (feature + reason) through the analytics client (which uses the B5 masked fallback when blocked).

### B3. Reactive `crossorigin` fallback
Keep `crossorigin="anonymous"` as the default (VAD needs it on native-MP4 sources). On a `<video>` media error consistent with CORS taint/strip **on a native-MP4 source**, reload the element **without** `crossorigin`, restore the playhead (reuse the existing `restorePlayhead`/`capturePlayhead`). Playback recovers; VAD self-disables via B2 and raises a B4 notice. This is the reactive form (not proactive per-type), per the directive.

### B4. "Because of N we disabled M" notice (`PlayerDegradeNotice.vue`)
One small, dismissible in-player banner that **aggregates** every disabled feature with its cause ("Subtitle auto-sync is off because your browser blocks Web Audio"). i18n'd (en/ru/ja). Dismissal persisted per session (localStorage) so it never nags. Covers only user-facing feature disables (VAD/storyboards/subs-via-crossorigin) — analytics blocking is invisible to the user and needs no popup.

### B5. Masked analytics fallback (time-bucketed HMAC path)
When B1 detects `/api/analytics/*` is blocked, the **shared analytics client** (used by `playerTelemetry.ts`, `feErrorLog.ts`, and the clickstream collector) switches **for that session** to a masked endpoint:
- The path segment is `HMAC(server_secret, current_time_bucket)`, so it rotates on its own and cannot be pinned by a static filter rule.
- The FE cannot hold the secret, so the gateway **hands the current masked path to the client** by attaching it to an existing, non-blocked bootstrap API response (a field/header on a normal `/api/...` call the SPA already makes). No extra request, no client secret.
- The gateway validates incoming masked requests by recomputing the HMAC for the **current and previous** buckets (clock-skew / session-straddle tolerance) and routes matches to the analytics service (same handler, additional mount).
- Normal (unblocked) users keep hitting `/api/analytics/*`, keeping the masked path low-profile.

## 5. Telemetry additions

Building on the already-shipped `playback_start_rejected` kind (§7):
- FE `PlayerEvent.kind` gains `capability` (once-per-session compact probe summary) and `degrade` (feature + reason on a caught failure / reactive-crossorigin trip).
- `services/analytics` handler maps them to `effect_kind` `player_capability` / `player_degrade` (mirrors the existing `player_resolve`/`player_stall` switch). Provider whitelist unaffected (these are client-state, not source-ranking, rows — keep `provider` optional / a sentinel).
- Result: the next gera-class report is answerable in one ClickHouse query — "what did this user's browser support and what did we disable" — instead of server-log archaeology.

## 6. Isolation & units

Each unit has one purpose and is independently testable:

| Unit | Responsibility |
|---|---|
| `libs/videoutils/streamtoken.go` | opaque token encode/decode (pure) |
| `libs/videoutils/proxy.go` (path handler + rewriter) | serve `/m/<token>/<leaf>`, emit token URLs |
| catalog `streamsign` | mint token URLs at the source |
| gateway masked-path route + validator | HMAC bucket validation, hand path to client |
| `capabilityProbe.ts` | run probes → `CapabilityReport` (pure) |
| `useClientCapabilities.ts` | reactive gates + once-per-session emit |
| analytics client endpoint resolver | primary → masked fallback (shared by 3 callers) |
| `PlayerDegradeNotice.vue` | aggregated dismissible notice |
| AePlayer glue | reactive `crossorigin`, wire gates to VAD/storyboards/preload |

## 7. Already-built (folded into this branch)

The autoplay-overlay fix is complete + verified (FE gates + Go tests green), landing in the same branch:
- All `video.play()` calls funnel through `attemptPlay()`; `NotAllowedError` raises a dedicated `playbackBlocked` state + click-to-play overlay (`autoplayBlocked` / `autoplayBlockedHint`, en/ru/ja), never counts as a dead source, and stands the silent-stall watchdog down.
- `usePlayerSyncBridge` forwards remote-play rejections via an `onPlayRejected` callback.
- `playerTelemetry` + analytics handler gained the `playback_start_rejected` kind (DOMException name in `error_kind`).
- `diagnostics.ts` serializes `Error`/`DOMException` as `name: message` (kills the `{}`); `main.ts` includes the error name in unhandled-rejection reports.

## 8. Testing strategy

- **Go:** `streamtoken` round-trip + tamper + expiry; proxy `/m/<token>/<leaf>` decode + rewrite; gateway masked-path HMAC validation across current/previous bucket + rejection of a stale/forged segment; analytics handler `capability`/`degrade` kinds.
- **FE (vitest):** `capabilityProbe` per-probe (mock missing APIs); `useClientCapabilities` (emits once, gates flip on throw); `PlayerDegradeNotice` render/aggregate/dismiss-persist; reactive-crossorigin on a simulated CORS media error → reload without attribute + playhead restore; analytics client switches to masked endpoint when primary is blocked; autoplay overlay specs (done).
- **Manual/verify:** Firefox with Autoplay=Block → overlay; a uBlock filter for `solodcdn`/`hls-proxy` → media still loads via token path; block `/api/analytics/*` → events arrive on masked path.

## 9. Rollout & sequencing

Single branch. Order within it: (a) Track A token scheme + dual-accept, verify existing playback unaffected; (b) telemetry kinds; (c) B1 probe + B5 masked analytics (so we get signal); (d) B2/B3/B4 graceful-disable + notice + reactive crossorigin. Redeploy: all Go services (videoutils touch) + gateway + analytics + web. Then `/animeenigma-after-update`. Remove the legacy `?url=` proxy handler in a later branch after a safe window.

## 10. Effort scoring (repo convention — no time units)

- **UXΔ = +2 (Better)** — constrained-browser users go from a dead player to working video + an honest reason; everyone gets un-blockable media URLs.
- **CDI = 0.06 * 34** — moderate spread (proxy, gateway, analytics, player), low per-site shift, Effort_Fib 34 (token scheme + FE capability layer + masked route).
- **MVQ = Griffin 85%/80%** — sturdy, defensive, composed of independent guarded parts.
