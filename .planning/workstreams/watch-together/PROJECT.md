# Project: AnimeEnigma — `watch-together` workstream

**Parent project:** AnimeEnigma (see `/data/animeenigma/.planning/PROJECT.md`)
**Workstream:** watch-together
**Created:** 2026-05-25
**Lifecycle:** Independent of v3.x scraper work, parallel to other workstreams (`notifications`, `raw-jp`, `social`, `ui-ux-audit`, `hero-spotlight`)
**Source design doc:** `/data/animeenigma/docs/superpowers/specs/2026-05-25-watch-together-design.md` (Approved for planning, 2026-05-25)

## Scope of this workstream

Build **Watch Together** — a synchronized co-watch experience where 2–10 logged-in friends share an invite link, land in the same room, and watch the same anime in lock-step. Every play, pause, seek, episode switch, player switch, and translation switch mirrors across all members in real time. Text chat and emoji reactions run alongside the player.

> **Two friends in different time zones click a link, watch the same Frieren episode together with sub-second sync, chat about it, and never have to coordinate "1, 2, 3, play" over voice again.**

The feature is **uniformly syncable across all 5 players** — including Kodik, despite its iframe nature, thanks to the undocumented inbound `kodik_player_api` postMessage RPC discovered 2026-05-25 via player bundle disassembly (see [reference_kodik_inbound_postmessage_api.md] in user memory).

## Core value

Co-watching is a category-defining feature for anime communities. It removes the need for voice-coordinated "3, 2, 1, play" and turns shared episodes into a real shared experience — reactions, chat, instant pause to discuss a scene, all without a separate Discord screen-share. AnimeEnigma's multi-player architecture (RU + EN + JP + Hanime) makes this especially powerful for international friend groups, though v1 commits to "one TV, multiple chairs" — the per-user-player evolution is deferred to v2.

**UXΔ = +4 (Better)** | **CDI = 0.04 * 55** | **MVQ = Griffin 88%/75%**

## Out of scope for this workstream (v1.0)

| Excluded | Reason |
|---|---|
| Public room directory / lobby browsing | Private friend rooms only. Public discoverability adds moderation surface (reports, NSFW gating at room level, kick/ban, capacity caps for larger rooms). Defer to v2. |
| Persistent named rooms ("Saturday Anime Night") | Ephemeral keeps state in Redis only — no Postgres tables, no chat retention, no rejoin-tomorrow UX. Defer to v2. |
| Voice or video chat | Discord / Telegram already do this well. WebRTC SFU + TURN + codecs is a separate-product effort. |
| Per-user player choice (RU friend on Kodik, EN friend on OurEnglish, both synced) | Genuinely useful and AnimeEnigma is uniquely positioned to do it. v1 commits to "everything shared"; defer to v2. |
| Guest access | Login required. WebSocket auth uses the existing gateway JWT. |
| In-platform invite notifications | Shareable link only. Friends already have a chat channel. |
| Hanime room-level age gate | Treats all 5 players uniformly. Relies on the user-level 18+ acknowledgement. |
| Host promotion / kick / mute | "Anyone can control" — control is democratic. No host privileges in v1. |
| Watch-history attribution per member | Watch history is written per-user as if solo. Co-watch attribution = future schema decision. |
| Horizontal scaling (multi-instance fanout) | v1 runs one instance. Redis pubsub channel scheme is wired so v2 doesn't need a protocol change. |

## Active milestone

⏳ **v1.0 Watch Together Foundation** — 5 phases, ephemeral private friend rooms, all 5 players syncable.

| Phase | Layer | Independently demoable end-state |
|---|---|---|
| 1 | Backend Foundation | New `services/watch-together/` on :8091. REST `POST /rooms` + WebSocket `/ws`. `curl` + `wscat` create a room, join, exchange messages. Gateway routes done. |
| 2 | Frontend Shell + Chat | `WatchTogetherView.vue` route, `useWatchTogetherRoom` composable, sidebar with members/chat/reactions. "Invite to Watch Together" button on WatchView creates room + copies link. Two browsers can join, exchange chat/reactions. (No player sync yet.) |
| 3 | Player Sync (all 5) | HTML5 adapter (AniLib / OurEnglish / Hanime / Raw) + Kodik adapter via `kodik_player_api` RPC + boot-time smoke probe + drift correction. Two browsers play/pause/seek in sync on any of the 5 players. |
| 4 | State Switching | Episode / player / translation switching propagates to all members. Players cleanly re-mount on state change. |
| 5 | Polish | Reaction burst animations, reconnect grace, mobile bottom-sheet, i18n (en + ru), capacity-full UX, room-expired redirect, Grafana panel. Production-shippable. |

Each phase is independently demoable and atomically committable.

## Planned milestones (post-v1.0)

- **v1.1 Per-User Player** — The killer feature. Let mixed-language friend groups watch in their own language while sharing the timeline. Adds: per-member player + translation state, language-aware seek translation (some providers have different timings), UI to switch your own player without switching the room's. Largest single v2 lift; will need its own brainstorm.
- **v1.2 Persistent Named Rooms** — "Saturday Anime Night" rooms that survive past empty state. Adds: `rooms` Postgres table, chat retention policy, room settings (name, visibility, auto-resume), invite-link revocation. Mostly schema and lifecycle work; protocol unchanged.
- **v1.3 Voice piggyback** — Optional WebRTC voice layer alongside text chat. May not happen if Discord remains the default; revisit after v1.0+v1.1 usage data.

## Active requirements (v1.0)

See `REQUIREMENTS.md` for `WT-FOUND-*`, `WT-SHELL-*`, `WT-SYNC-*`, `WT-STATE-*`, `WT-POLISH-*`, `WT-NF-*` IDs.

## Context (v1.0 surface area)

**Backend touches:**
- New: `services/watch-together/` (new microservice, port 8091)
- New: `services/watch-together/Dockerfile`
- New: `services/watch-together/go.mod` (joined to `go.work`; require + replace for every `libs/*` used)
- New: `go.work` extended for `./services/watch-together`
- Modified: `docker/docker-compose.yml` (new service block + depends_on redis)
- Modified: `docker/.env.example` (`WATCH_TOGETHER_*` env vars; `WATCH_TOGETHER_SERVICE_URL` for gateway)
- Modified: `services/gateway/internal/config/config.go` (new `WatchTogetherURL` field)
- Modified: `services/gateway/internal/router/routes.go` (new `/api/watch-together/*` HTTP proxy + WebSocket proxy under `authMiddleware`)
- Modified: `Makefile` (`make redeploy-watch-together`, `make logs-watch-together`, `make restart-watch-together`)
- Modified: `CLAUDE.md` (Service Ports table + Gateway Routing table)

**Frontend touches:**
- New: `frontend/web/src/views/WatchTogetherView.vue`
- New: `frontend/web/src/composables/useWatchTogetherRoom.ts`
- New: `frontend/web/src/components/watch-together/RoomSidebar.vue`
- New: `frontend/web/src/components/watch-together/MemberList.vue`
- New: `frontend/web/src/components/watch-together/ChatPanel.vue`
- New: `frontend/web/src/components/watch-together/ReactionPalette.vue`
- New: `frontend/web/src/components/watch-together/ReactionBurstOverlay.vue`
- New: `frontend/web/src/components/watch-together/InviteButton.vue`
- New: `frontend/web/src/api/watch-together.ts`
- New: `frontend/web/src/router/index.ts` (add `/watch/room/:roomId` route)
- Modified: `frontend/web/src/components/player/{Kodik,AnimeLib,OurEnglish,Hanime,Raw}Player.vue` (add `room?` prop + adapter wiring)
- Modified: `frontend/web/src/views/WatchView.vue` (mount the `<InviteButton>`)
- Modified: `frontend/web/src/locales/{en,ru}.json` (new `watch_together.*` namespace)

**Operational touches:**
- Grafana dashboard panel for `watch_together_*` metrics (active rooms, members, message rate, drift correction frequency, Kodik probe success rate). Deferred to Phase 5.
