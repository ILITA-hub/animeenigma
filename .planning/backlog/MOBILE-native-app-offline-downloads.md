---
id: MOBILE-native-app-offline-downloads
title: Native mobile app with offline anime downloads
captured_at: 2026-07-01
captured_during: admin TODO capture (Telegram, @tNeymik)
target_milestone: unscheduled / TBD
deferred_from: N/A (net-new request)
status: backlog
---

# Native mobile app — download anime and watch offline

## Original request

Admin TODO («Туду: Сделать мобильное приложение чтобы можно было скачать аниме и
смотреть оффлайн») — build a mobile app that lets users download anime episodes and
watch them offline.

## Scope

This is a new platform, not an extension of the existing Vue 3 web frontend:

1. **Client**: native or cross-platform mobile app (React Native / Flutter / separate
   Swift+Kotlin) — none of the current frontend code (`frontend/web/`) is directly
   reusable; the player UI, source-selection logic, and design system would need a
   from-scratch mobile port.
2. **Download manager**: background download queue, per-episode progress, storage
   quota/eviction policy, resumable downloads, Wi-Fi-only toggle.
3. **Local playback engine**: offline HLS/MP4 playback (no proxy/CDN in the loop once
   downloaded), local subtitle rendering (would need to port `SubtitleOverlay.vue`'s
   ASS/SRT/VTT logic, or the parsing at least, to the mobile stack).
4. **Backend surface**: new or extended endpoints to fetch a downloadable asset (vs.
   today's streaming-only model — HLS proxy, signed short-TTL URLs, 1h video-URL
   cache — see CLAUDE.md "No storing external video files (stream directly)" and
   "No caching video URLs >1h"). A persistent local copy is a different trust/legal
   model than the current stream-through-proxy one.
5. **Sync**: watch progress / watchlist reconciliation between mobile-offline state and
   the existing player service (`services/player`).

## Why this needs an explicit decision before any planning starts

- **Project target deployment is "small self-hosted groups (no CDN)"** (CLAUDE.md). A
  native mobile app is a materially larger commitment (two more client platforms, app
  store distribution/review, ongoing OS-version maintenance) than the rest of the
  project's footprint.
- **Content provenance**: today's sources are third-party scrapers/aggregators (Kodik,
  AniLib, EN scraper chain, AllAnime raw, Hanime, animejoy) consumed via short-lived
  signed streaming URLs that are deliberately never cached >1h. Persisting full
  episodes to a user's device for offline playback is a different exposure profile
  than transient streaming — worth the admin weighing explicitly, not something to
  wave through as a routine feature build.
- **Platform choice** (React Native vs. Flutter vs. native) has large downstream
  effects on team velocity and is itself a decision point, not something to default.

## Cost estimate

| Component | Effort (Fib) | Risk |
|---|---|---|
| Mobile client shell (platform choice, nav, auth) | 55 | Medium — new stack, no reuse from `frontend/web/` |
| Player port (source selection, HLS/MP4 playback, subtitles) | 55 | High — `AePlayer.vue`'s capability-feed model + subtitle overlay has no mobile analog yet |
| Download manager + local storage/eviction | 34 | Medium |
| Backend: downloadable-asset endpoints + auth/signing model for persisted files | 21 | High — conflicts with current "stream, don't store" convention; needs a deliberate exception |
| Sync (watch progress / watchlist ↔ mobile) | 13 | Low — `services/player` API mostly reusable |
| App store distribution (iOS + Android review, signing, releases) | 21 | Medium — ongoing, not one-time |
| **Total** | **233+ (should be split into its own milestone/roadmap, not a single phase)** | |

## Status

Captured only — no implementation, no architecture decision made. Needs an explicit
scoping/brainstorm pass (platform choice, content-persistence policy) before it becomes
a milestone. Feature requests are never auto-implemented by the maintenance bot; this
awaits owner review.

## Cross-references

- Player architecture (would need a mobile analog): `docs/aeplayer-reference.md`
- Streaming model conventions ("no storing external video files", "no caching video
  URLs >1h"): `CLAUDE.md` → "Don't Do"
- Source feedback: `2026-07-01T01-30-04_tNeymik_telegram`
