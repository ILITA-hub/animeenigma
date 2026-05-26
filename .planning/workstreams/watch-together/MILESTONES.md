# Milestones — `watch-together` workstream

## v1.0 Watch Together Foundation (✅ COMPLETE)

**Status:** ✅ Shipped — 5/5 phases delivered
**Started:** 2026-05-25
**Closed:** 2026-05-26
**Plans landed:** 41 (Phase 1: 9, Phase 2: 10, Phase 3: 7, Phase 4: 6, Phase 5: 9)
**Requirements covered:** 51 (WT-FOUND-01..10 + WT-SHELL-01..08 + WT-SYNC-01..10 + WT-STATE-01..05 + WT-POLISH-01..08 + WT-NF-01..07)
**Phase summaries:** `phases/{01..05}-*/0[1-5]-PHASE-SUMMARY.md`
**Source design doc:** `/data/animeenigma/docs/superpowers/specs/2026-05-25-watch-together-design.md`
**Closure summary:** `phases/05-polish/05-PHASE-SUMMARY.md`

**Scope:** A new `services/watch-together/` Go microservice on port 8091 (Redis-only, no Postgres) + WebSocket protocol for playback/state/chat/reaction sync + frontend `WatchTogetherView` with `useWatchTogetherRoom` composable + player-adapter wiring across all 5 players (Kodik via undocumented `kodik_player_api` RPC, HTML5 four via native `<video>`) + ephemeral private friend rooms (2–10 members, dies when empty + 5min grace) + invite-link only + login-required + text chat + emoji reactions + Grafana panel.

The feature ships as a self-contained workstream; the only existing files modified are gateway routing config, docker-compose, Makefile, CLAUDE.md tables, and the 5 player components (additive `room?` prop only).

**Phases (planned):**

1. Backend Foundation (`WT-FOUND-01..10` + `WT-NF-01..03`)
2. Frontend Shell + Chat (`WT-SHELL-01..08` + `WT-NF-04`)
3. Player Sync — All 5 (`WT-SYNC-01..10`)
4. State Switching (`WT-STATE-01..05`)
5. Polish + Production-Ship (`WT-POLISH-01..08` + `WT-NF-05..07`)

See `REQUIREMENTS.md` for full requirement text and `ROADMAP.md` for the phase / requirement map + success criteria.

---

## v1.1 Per-User Player (planned — not yet scoped)

The killer feature deferred from v1. Mixed-language friend groups watch in their own language while sharing the timeline. Will need its own brainstorm (per-member state shape, language-aware seek translation for providers with different timings, UI for "switch your player without switching the room's"). Largest single v2 lift.

## v1.2 Persistent Named Rooms (planned — conditional)

"Saturday Anime Night" rooms that survive past empty state. Adds Postgres tables (`rooms`, `room_messages`, `room_members_history`), chat retention policy, room settings (name, visibility, auto-resume), invite-link revocation, host-only delete. Mostly schema + lifecycle work; the v1 WebSocket protocol stays the same.

## v1.3 Voice Piggyback (planned — conditional)

Optional WebRTC voice layer alongside text chat. May not happen if Discord remains the default coordination channel. Revisit after v1.0+v1.1 usage data shows whether built-in voice would actually be used.
