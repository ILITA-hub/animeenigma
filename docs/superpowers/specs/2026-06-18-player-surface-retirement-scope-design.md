# Player-Surface Retirement — Scope

**Date:** 2026-06-18
**Status:** Scoping (decisions locked; ready to decompose into implementation plans)
**Author:** Claude (with owner)
**Parent direction:** `project_retire_all_players_except_aeplayer` (owner, 2026-06-17)

## Goal

Collapse the frontend video-player surface from **7 player components** down to
**aePlayer as the single primary surface**, with the **Kodik iframe kept as a
permanent RU fallback**. This is the user-facing half of the player-retirement
direction; the backend `stream_providers` roster unification (Phases 1–2) is
already shipped.

## Locked decisions

| # | Decision | Choice |
|---|----------|--------|
| D1 | Hanime 18+ gap (aePlayer can't play hanime) | **Retire HanimePlayer, drop Hanime content.** 18+ served by the `anime18` source aePlayer already resolves. |
| D2 | Kodik (aePlayer uses fragile HLS; legacy is bulletproof iframe) | **Keep `KodikPlayer.vue` iframe permanently** as an aePlayer fallback ("Classic Kodik" escape hatch). |
| D3 | Sequencing | **Direct** — flag-off + delete in one wave (no gradual default-soak), ordered tasks. |
| D4 | WatchTogether (fully coupled to legacy players, no aePlayer) | **Integrate aePlayer into WatchTogether as part of this wave.** |

### Survivors vs retired

- **Survive:** `aePlayer` (unified, becomes the default surface) · `KodikPlayer.vue` (iframe fallback).
- **Deleted player components:** `KodikAdFreePlayer.vue`, `AnimeLibPlayer.vue`,
  `OurEnglishPlayer.vue`, `HanimePlayer.vue`, `Anime18Player.vue`, `RawPlayer.vue`.

### Critical distinction — player surface ≠ provider/source

Deleting a **player component** is NOT the same as retiring the **provider** it
used. aePlayer's resolver (`src/composables/unifiedPlayer/useProviderResolver.ts:540`)
keeps consuming these **sources** after the surfaces are gone:

- `ae` (first-party MinIO HLS) · `kodik` (RU, ad-free **HLS** path) ·
  `scraper` (the EN chain: gogoanime→animepahe→…) · `raw` (AllAnime JP) ·
  `anime18` (18+).

So the only **content actually dropped** is **Hanime** (D1) and **AniLib**
(already dead — upstream went Kodik-only, `useProviderResolver.ts:26,462`). The
EN/raw/18+/Kodik *content* all remain, just behind aePlayer instead of dedicated
player tabs.

**Backend roster impact:** set only `hanime` + `animelib` to `status=disabled`
in `stream_providers`. Leave `ae`/`kodik`/`raw`/`anime18`/the scraper chain
`enabled` — aePlayer still resolves them.

---

## Workstreams & change-set

References use the architecture map gathered 2026-06-18 (current `Anime.vue` line
anchors; will drift — re-anchor at plan time).

### WS1 — `Anime.vue` surface collapse
- **Remove the two-row tab UI** (`src/views/Anime.vue:371–522`): the
  RU/EN/18+/RAW language strip + all provider sub-tabs. aePlayer handles
  language/provider internally via its SourcePanel.
- **Replace** with: aePlayer as the default mounted surface + a single "Classic
  Kodik" toggle that mounts `KodikPlayer.vue` (the iframe fallback).
- **Remove the legacy player chain** (`Anime.vue:525–652`) except the
  `KodikPlayer` branch.
- **Remove async imports** for the 6 deleted players (`Anime.vue:1091–1128`,
  keeping Kodik + Unified).
- **Remove flags** (see WS-FLAGS).
- **localStorage normalization** (`Anime.vue:1369,1378,1388,2465,2474`):
  `preferred_video_provider` values that name a removed provider
  (`ourenglish|hanime|raw|anime18|kodik-adfree|animelib`) must migrate to the
  aePlayer default on read; `kodik` keeps meaning the iframe fallback.
  `unified_player_selected` becomes effectively always-true (aePlayer default).

### WS2 — aePlayer becomes the default
- aePlayer mounts by default (drop the `unifiedSelected` opt-in gate; keep
  `VITE_UNIFIED_PLAYER_ENABLED` as an emergency kill-switch OR remove — decide at
  plan time).
- **Notification deep-links:** align with the existing
  `2026-06-18-notification-deeplink-aeplayer-provider-design.md` spec — removed
  provider ids in deep-links resolve onto aePlayer's source selection
  (`providerRegistry.ts`), not a now-deleted player.
- **Adult (18+) gating:** the age/`isHentai` gate currently wrapping the Hanime/
  Anime18 tabs must move to gate aePlayer's `anime18` source exposure.

### WS3 — Delete components, tests, locales, allowlist, flags
- **Delete** the 6 `.vue` files + their co-located `.spec.ts`.
- **i18n:** remove `player.anime18`, `player.kodikAdfree`, `player.ourenglish`,
  `player.raw` (+ any hanime/animelib keys) from **en/ru/ja** symmetrically.
  Keep `player.unified`, `player.sources`, `player.scraperProviders`, and shared
  keys. ⚠️ The new `locale-parity.spec.ts` (shipped `098d0c41`) will FAIL the
  build if removal isn't symmetric across all three locales — this is a feature,
  it enforces the cleanup.
- **DS allowlist** (`scripts/design-system-allowlist.txt`): remove the
  per-player accent lines for the deleted files — **lines 18 (OurEnglish), 19
  (Raw), 21 (kodik-adfree), 22 (animelib), 23 (hanime), 24 (anime18)**. **KEEP
  line 20 (KodikPlayer `#06b6d4`)** — it survives. ⚠️ The new allowlist
  path-integrity check (shipped `098d0c41`) will FAIL the build if a deleted
  file's allowlist line is left behind — again, it enforces the cleanup.
- **SubtitleOverlay** (allowlist lines 28–29) is shared and survives (aePlayer
  uses it) — keep.

### WS-FLAGS — flag removal
Remove `VITE_ANIMELIB_ENABLED`, `VITE_KODIK_ADFREE_ENABLED`,
`VITE_OURENGLISH_ENABLED`, `VITE_RAW_PROVIDER_ENABLED`, `VITE_ANIME18_ENABLED`
from `.env`, `.env.example`, and all read sites (`Anime.vue`,
`WatchTogetherView.vue:97`). No Kodik flag exists (iframe stays always-on).

### WS4 — aePlayer in WatchTogether (the long pole)
`WatchTogetherView.vue` mounts legacy players by `livePlayer` (room.player) at
`:482–517` (kodik / kodik-adfree / animelib / ourenglish / hanime / raw) and has
**no aePlayer wiring**. WT sync keys on a single fixed player-type per room;
aePlayer is multi-source, so its "what's playing" is `(provider, episode, audio,
team, server, time)` — a richer sync payload.
- **Design needed:** aePlayer broadcasts its **resolved source** so room members
  converge on the same provider/audio/team, plus the existing time-sync.
- **Backend** `services/watch-together`: room-state model likely needs an
  `aeplayer` player type + the resolved-source fields in the synced state
  (verify against `docs/watch-together-reference.md` + the WS `wt:` Redis state).
- **Reduce** WT player branches to `aePlayer` + `KodikPlayer` iframe; delete the
  others.
- This is a **sizable sub-project** and the dependency gate for WS3's deletions
  (can't delete the players WT still mounts until WT no longer mounts them).

### WS5 — Backend roster
- `stream_providers`: set `hanime` + `animelib` → `status=disabled` (content
  dropped). Roster is already DB-driven + emitter-unified (Phases 1–2), so this
  is an operator/seed edit, not new infra. Confirm aePlayer-consumed providers
  stay `enabled`.

### WS6 — Verification gates
- `bunx vue-tsc --noEmit` (NOT plain tsc — catches `.vue` template breakage),
  `bunx vitest run`, `make redeploy-web` gates (DS-lint allowlist path-integrity,
  i18n-lint + locale-parity, eslint).
- e2e: player + WatchTogether specs; the daily **Kodik-canary** WT runbook
  (`docs/watch-together-reference.md`).
- Manual smoke: RU (aePlayer Kodik-HLS + Classic Kodik fallback), EN, raw, 18+
  (anime18 behind age gate), co-watch room.

---

## Risks & dependencies

1. **WT-aePlayer sync (WS4)** — highest effort + risk; gates the deletions. The
   multi-source sync payload is a genuine protocol change.
2. **Kodik-HLS reliability** — mitigated by keeping the `KodikPlayer` iframe
   (D2); RU never loses the bulletproof path.
3. **Dangling localStorage prefs** — users with a saved removed-provider pref
   need on-read normalization (WS1) or they hit a dead provider.
4. **Notification deep-links** — must resolve removed providers to aePlayer
   (align with the existing deeplink spec).
5. **18+ catalog shrink** — Hanime-only titles become unavailable (accepted per
   D1); anime18 is a different catalog.
6. **The two gates shipped today help** — allowlist path-integrity + locale
   parity will *catch* incomplete deletion cleanup as build failures, turning
   "forgot a line" into a red build instead of silent rot.

## Recommended decomposition (two implementation plans)

WS4 is separable and is the long pole, so split rather than one mega-plan:

- **Plan A — "aePlayer in WatchTogether"** (WS4): the sync-protocol sub-project.
  Must land first (deletion depends on it). Own spec/plan; touches
  `services/watch-together`, `WatchTogetherView.vue`, aePlayer broadcast.
- **Plan B — "Retire legacy player surfaces"** (WS1–3, WS5, WS-FLAGS, WS6):
  the FE collapse + deletions + roster edit + verification. Depends on Plan A.

Each plan produces independently testable, shippable software. Plan B's deletions
are guarded by the new build gates, so an incomplete cleanup fails CI.

## Out of scope
- Building a hanime adapter for aePlayer (D1 drops Hanime instead).
- Replacing the Kodik iframe (D2 keeps it).
- The backend roster *infra* (already shipped, Phases 1–2) — only the
  hanime/animelib status edit (WS5) is in scope.
