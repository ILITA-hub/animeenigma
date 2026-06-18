# Retire Legacy Player Surfaces — Implementation Plan (Plan B)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax. Execute in an ISOLATED worktree off origin/main (the shared tree is stale + churned); re-anchor line numbers at execution (Anime.vue churns).

**Goal:** Collapse the anime watch surface to **AePlayer as the default player**, with **KodikPlayer (iframe) kept as a "Classic Kodik" RU fallback**, and DELETE the other 6 legacy player surfaces (KodikAdFree, AnimeLib, OurEnglish, Hanime, Anime18, Raw) — components, their tabs/UI, i18n namespaces, DS-allowlist lines, and feature flags. Drop Hanime + AniLib content (18+ now served by AePlayer's `18anime` source).

**Architecture:** Add-only Plan A already made AePlayer a synced WatchTogether player. Plan B is the **removal** wave. AePlayer's resolver already consumes every surviving source (`ae`, `kodik`-HLS, EN `scraper` chain, `raw`, `18anime`), so deleting the standalone player components loses no content except Hanime (dropped per scope D1) and AniLib (dead upstream). KodikPlayer's iframe stays as the bulletproof RU fallback (D2). Two build gates shipped 2026-06-18 ENFORCE cleanup completeness: the DS-allowlist **path-integrity** check fails the build if a deleted player's allowlist line lingers, and the **locale-parity** spec fails if an i18n namespace is removed asymmetrically across en/ru/ja.

**Tech Stack:** Vue 3 + TS (Anime.vue, WatchTogetherView.vue, player components), Go (catalog `stream_providers` roster), vitest, vue-tsc.

---

## Locked decisions (from scope `2026-06-18-player-surface-retirement-scope-design.md`)

- **Survivors:** AePlayer (default surface) + `KodikPlayer.vue` (iframe "Classic Kodik" fallback).
- **Delete:** `KodikAdFreePlayer`, `AnimeLibPlayer`, `OurEnglishPlayer`, `HanimePlayer`, `Anime18Player`, `RawPlayer` (+ specs).
- **Content dropped:** Hanime (18+ → `18anime` source in AePlayer), AniLib (dead). All other sources stay (AePlayer consumes them).
- **Backend roster:** set ONLY `hanime` + `animelib` → `status=disabled` in `stream_providers`. Leave `ae`/`kodik`/`raw`/`18anime`/scraper-chain enabled.
- **Non-negotiable:** player-surface deletion ≠ provider/source removal. Do NOT touch `useProviderResolver.ts` adapters, `api/client.ts` source endpoints, or the scraper/catalog source plumbing.

## Current-state anchors (origin/main `7f6009d2`; re-verify at execution — Anime.vue churns)

- `frontend/web/src/views/Anime.vue`:
  - Player async imports ~1094-1111 (KodikPlayer 1094 = KEEP; the other 6 = delete refs).
  - Flag reads ~1098-1132: `kodikAdfreeEnabled`(1098), `rawProviderEnabled`(1107), `ourEnglishEnabled`(1112), `animeLibEnabled`(1121), `aePlayerEnabled`(1132, `VITE_AE_PLAYER_ENABLED`). NO kodik flag.
  - Tab UI `<div v-if="!notReleasedYet" class="...player-tabs">` ~371-522 (language tabs RU/EN/18+/RAW + `aeSelected` "AnimeEnigma" toggle ~425 + provider sub-tabs).
  - Legacy mount chain `<div class="glass-card..." v-if="!aeSelected">` ~525-652 (KodikPlayer 565=KEEP; KodikAdFree 580, AnimeLib 595, OurEnglish 609, Hanime 619, Anime18 631, Raw 644 = delete).
  - AePlayer mount ~656 (`v-if="aeSelected && aePlayerEnabled"`).
  - Refs/logic: `videoLanguage`, `videoProvider`, `aeSelected`, `switchLanguage`, `onUserPickedProvider`, `playerActivated`(1237), `notReleasedYet`(1814).
  - localStorage: `preferred_video_provider`(1392,2513), `preferred_video_language`(1373,2522), `unified_player_selected`(1382-1385).
- `frontend/web/src/views/WatchTogetherView.vue`: legacy imports 88-93 (KodikPlayer 88 = KEEP; KodikAdFree 89, AnimeLib 90, OurEnglish 91, Hanime 92, Raw 93 = delete); branches kodik-adfree 564, animelib 571, ourenglish 578, hanime 585, raw 592 (+ kodik KEEP + aeplayer KEEP from Plan A).
- DS allowlist `frontend/web/scripts/design-system-allowlist.txt`: per-player accent lines — `OurEnglishPlayer.vue:#22d3ee`, `RawPlayer.vue:#22d3ee`, `KodikAdFreePlayer.vue:#06b6d4`, `AnimeLibPlayer.vue:#f97316`, `HanimePlayer.vue:#ec4899`, `Anime18Player.vue:#f43f5e` = REMOVE. `KodikPlayer.vue:#06b6d4` = KEEP. `SubtitleOverlay.vue` lines = KEEP (shared).
- i18n namespaces (en/ru/ja): `player.kodikAdfree`, `player.ourenglish`, `player.raw`, `player.anime18` = REMOVE symmetrically. KEEP `player.aePlayer`/`player.unified`, `player.sources`, `player.scraperProviders`, shared keys.
- No external importers of the 6 doomed players (verified) — only Anime.vue + WatchTogetherView + own specs.

---

### Task B1: Backend roster — disable hanime + animelib

**Files:**
- `services/catalog/internal/service/scraperprovider/seed.go` (seed status)
- a guarded one-time DB update (mirror `BackfillScraperOperated` pattern) OR document the live `UPDATE`
- Test: `seed_test.go` / migrate test

**Context:** `stream_providers` is DB-authoritative; the seed inserts-if-absent and preserves operator edits, so changing the seed alone won't flip EXISTING rows. Mirror the existing guarded-migration helper to set `status='disabled'` for `hanime` + `animelib` once.

- [ ] **Step 1:** Write a failing test asserting the seed/migration yields `status=disabled` for `hanime` and `animelib` (and leaves `ae`/`kodik`/`raw`/`18anime`/scraper rows untouched). Run → fail.
- [ ] **Step 2:** Set seed default `status: disabled` for the `hanime` + `animelib` rows; add a guarded idempotent boot update (e.g. `RetireHanimeAnimelib(db)`) that flips those two rows to disabled exactly once (guard like the existing backfill so re-runs are no-ops and operator re-enables aren't clobbered every boot — match the established pattern; if the pattern always-forces, follow it but document). Run → pass.
- [ ] **Step 3:** `cd services/catalog && go test ./internal/service/... -count=1 && go build ./...` → green.
- [ ] **Step 4:** Commit: `feat(catalog): retire hanime+animelib stream-provider rows (status=disabled)`.

> Note: independent of the FE tasks; can run any time. Lower risk.

---

### Task B2: Anime.vue — AePlayer default + "Classic Kodik" fallback; remove tabs + legacy chain

**This is the centerpiece and the riskiest task. Re-anchor every line number against the live file.**

**Files:** `frontend/web/src/views/Anime.vue` (+ its spec if one exists; add/adjust tests)

**Target UI:** AePlayer is THE player (mounts by default). A single small "Classic Kodik" toggle/button mounts `KodikPlayer` (iframe) instead — the RU fallback. Remove the RU/EN/18+/RAW language tabs, ALL provider sub-tabs, and the entire legacy mount chain except KodikPlayer. AePlayer's SourcePanel owns language/provider/source selection internally.

- [ ] **Step 1: Introduce the fallback toggle state.** Replace the `aeSelected` (ae is opt-in) model with: AePlayer default + `classicKodik` ref (default `false`). Persist as `classic_kodik_selected` (boolean). Remove `videoLanguage`, `videoProvider`, `switchLanguage`, `onUserPickedProvider` and their watchers/handlers. Keep `notReleasedYet`, `playerActivated` (if it gates AePlayer too; otherwise simplify).

- [ ] **Step 2: localStorage normalization.** On read: migrate legacy `unified_player_selected`/`preferred_video_provider`/`preferred_video_language` → the new model. Any saved provider that named a deleted player resolves to AePlayer default; `kodik` (iframe) maps to `classicKodik=true`. Delete writes to the obsolete keys. Add a unit test for the normalization function (extract it to a pure helper for testability).

- [ ] **Step 3: Collapse the template.** Remove the `player-tabs` block (language tabs + provider sub-tabs). Replace with: AePlayer mounted by default, plus a compact "Classic Kodik" toggle (a single Button, i18n key `player.classicKodik` — add to en/ru/ja). Mount logic:
  - `<AePlayer v-if="!classicKodik && aePlayerEnabled" :anime-id :anime :theater :is-hentai :initial-episode :mal-id @toggle-theater />`
  - `<KodikPlayer v-else ... />` (the iframe fallback; keep its existing props).
  - Keep the `notReleasedYet` premiere notice.
  - Remove the `<div v-if="!aeSelected">` legacy chain and all 6 deleted-player tags (KodikAdFree/AnimeLib/OurEnglish/Hanime/Anime18/Raw). KodikPlayer moves into the fallback branch.

- [ ] **Step 4: Remove the 6 deleted-player async imports** (lines ~1097-1111; KEEP KodikPlayer 1094, KEEP AePlayer). Remove the now-unused flag reads `kodikAdfreeEnabled`/`rawProviderEnabled`/`ourEnglishEnabled`/`animeLibEnabled` (and any `anime18Enabled`). Keep `aePlayerEnabled`.

- [ ] **Step 5: WT create payload.** The Plan A `wtInvitePayload`/`aeWtSeed` path keyed on `aeSelected` must now key on `!classicKodik` (AePlayer active by default). When `classicKodik` is on, fall back to the legacy kodik create payload. Update accordingly; keep the seed working.

- [ ] **Step 6: Tests + gates.** Update/author `Anime.vue` tests for: AePlayer mounts by default; "Classic Kodik" toggle mounts KodikPlayer; localStorage normalization. Run `bunx vitest run src/views/__tests__/` (adjust) + `bunx vue-tsc --noEmit`. Run `bash scripts/design-system-lint.sh` (the tab removal changes classes — keep it green).

- [ ] **Step 7: Commit** (may split Steps 1-2 / 3-5 into 2 commits): `feat(web): aePlayer is the default player; Classic Kodik fallback; remove legacy player tabs`.

> Risk: this is a large template+logic edit in a churned 2000+-line file. Re-anchor constantly. Do NOT remove the AniSkip/resume/episode plumbing AePlayer relies on. Keep `isHentai` flowing to AePlayer (18+ via its `18anime` source).

---

### Task B3: WatchTogetherView — remove deleted-player branches + imports

**Files:** `frontend/web/src/views/WatchTogetherView.vue` (+ spec)

- [ ] **Step 1:** Remove the async imports for KodikAdFree(89)/AnimeLib(90)/OurEnglish(91)/Hanime(92)/Raw(93). KEEP KodikPlayer(88) + AePlayer. Remove the `v-else-if="livePlayer === '...'"` branches for `kodik-adfree`/`animelib`/`ourenglish`/`hanime`/`raw`. KEEP `kodik`, `aeplayer`, and the forward-compat empty-state `v-else` (now catches retired kinds gracefully — an old room on a deleted player shows the empty state instead of crashing).
- [ ] **Step 2:** If `PlayerTabBar`'s hidden/visible set enumerates the removed players, prune to `aeplayer | kodik` (+ whatever remains). Keep the aeplayer tab.
- [ ] **Step 3:** Update the WT view spec(s) for the reduced branch set. Run `bunx vitest run src/views/__tests__/ src/components/watch-together/` + `bunx vue-tsc --noEmit`.
- [ ] **Step 4: Commit:** `feat(wt): drop retired legacy players from WatchTogetherView (keep kodik + aeplayer)`.

> After B2 + B3, the 6 player components have ZERO references (verified no other importers). Build stays green (files still exist, just unused).

---

### Task B4: Delete the 6 player components + specs

**Files (delete):** `frontend/web/src/components/player/{KodikAdFreePlayer,AnimeLibPlayer,OurEnglishPlayer,HanimePlayer,Anime18Player,RawPlayer}.vue` + their co-located `.spec.ts` / `__tests__` entries.

- [ ] **Step 1:** Before deleting, re-grep to CONFIRM zero remaining references: `grep -rnE "(KodikAdFreePlayer|AnimeLibPlayer|OurEnglishPlayer|HanimePlayer|Anime18Player|RawPlayer)" frontend/web/src --include=*.vue --include=*.ts | grep -v '\.spec\.'` → must be empty (the files' own definitions aside). If anything remains, fix the referrer first.
- [ ] **Step 2:** `git rm` the 6 `.vue` files + their specs.
- [ ] **Step 3:** Run `bunx vue-tsc --noEmit` (clean — no dangling imports) + `bunx vitest run` (the deleted specs are gone; no failures from missing files). The DS-allowlist path-integrity check will now FAIL (orphaned allowlist lines) — that's expected; B7 fixes it. Note it; do not "fix" by reverting.
- [ ] **Step 4: Commit:** `feat(player): delete retired player components (KodikAdFree/AnimeLib/OurEnglish/Hanime/Anime18/Raw)`.

---

### Task B5: Remove feature flags

**Files:** `frontend/web/.env`, `frontend/web/.env.example`, any remaining read sites.

- [ ] **Step 1:** Remove `VITE_KODIK_ADFREE_ENABLED`, `VITE_ANIMELIB_ENABLED`, `VITE_OURENGLISH_ENABLED`, `VITE_RAW_PROVIDER_ENABLED`, `VITE_ANIME18_ENABLED` from `.env` + `.env.example`. KEEP `VITE_AE_PLAYER_ENABLED` (kill-switch). Grep to confirm no remaining reads (Anime.vue reads were removed in B2; WT view read of `VITE_ANIMELIB_ENABLED` ~line 97 in Plan A's discovery — remove it).
- [ ] **Step 2:** `grep -rnE "VITE_(KODIK_ADFREE|ANIMELIB|OURENGLISH|RAW_PROVIDER|ANIME18)_ENABLED" frontend/web/src` → empty. `bunx vue-tsc --noEmit` clean.
- [ ] **Step 3: Commit:** `chore(web): remove retired-player feature flags`.

---

### Task B6: i18n — remove deleted-player namespaces (symmetric en/ru/ja)

**Files:** `frontend/web/src/locales/{en,ru,ja}.json`

- [ ] **Step 1:** Remove `player.kodikAdfree`, `player.ourenglish`, `player.raw`, `player.anime18` (and any hanime/animelib keys) from ALL THREE locales. KEEP `player.aePlayer`/`player.unified`, `player.sources`, `player.scraperProviders`, shared keys, and the `player.classicKodik` key added in B2.
- [ ] **Step 2:** `bunx vitest run src/locales/__tests__/locale-parity.spec.ts` (key + placeholder parity — fails if asymmetric) + `bash scripts/i18n-lint.sh` (fails on missing keys / orphaned references). If i18n-lint flags an UNUSED key warning for a removed namespace's leftover reference, fix the reference.
- [ ] **Step 3: Commit:** `chore(i18n): remove retired-player namespaces (en/ru/ja)`.

---

### Task B7: DS allowlist — remove deleted-player accent lines

**Files:** `frontend/web/scripts/design-system-allowlist.txt`

- [ ] **Step 1:** Remove the per-player accent lines for the 6 deleted files (`OurEnglishPlayer`/`RawPlayer`/`KodikAdFreePlayer`/`AnimeLibPlayer`/`HanimePlayer`/`Anime18Player`). KEEP `KodikPlayer.vue:#06b6d4` and the shared `SubtitleOverlay.vue` lines.
- [ ] **Step 2:** `bash scripts/design-system-lint.sh` → exit 0 (path-integrity now green: no orphaned lines; B4's deletions are accounted for). `--selftest` still passes.
- [ ] **Step 3: Commit:** `chore(ds): drop allowlist accent lines for deleted players`.

---

### Task B8: 18+ verification + final gates + whole-branch review

- [ ] **Step 1: 18+ smoke (code-level).** Confirm AePlayer offers the `18anime` source for a hentai title: trace that `isHentai` → `content: 'hentai'` (AePlayer.vue:522) → capabilities/resolver surface the `18anime` provider, and the age gate that previously wrapped the 18+ tab is preserved (AePlayer mounts only with the same age/`isHentai` context Anime.vue already enforces). If the age gate lived ONLY in the removed tab UI, re-add an equivalent gate around AePlayer's 18+ exposure. Document the finding.
- [ ] **Step 2: Full gate suite** (in the worktree):
  - `cd services/catalog && go test ./internal/service/... -count=1`
  - `cd frontend/web && bunx vue-tsc --noEmit`
  - `bunx vitest run` (whole FE; note the pre-existing `InviteButton.spec.ts` load failure is unrelated)
  - `bash scripts/i18n-lint.sh` ; `bash scripts/design-system-lint.sh`
- [ ] **Step 3: Whole-branch review** (dispatch a final reviewer): no dangling refs to deleted players anywhere (grep src + e2e/); AePlayer reachable as default; Classic Kodik fallback works; WT reduced cleanly; no source/provider plumbing touched; gates green.
- [ ] **Step 4:** Update any e2e specs (`frontend/web/e2e/`) that reference the removed players/tabs (player.spec, watch-together specs). The Kodik-canary spec stays (Kodik survives).

---

## Risks & mitigations
1. **Anime.vue collapse (B2)** — biggest risk; large edit in a churned file. Mitigate: re-anchor constantly, keep AniSkip/resume/episode/isHentai plumbing, test default-mount + fallback + localStorage.
2. **18+ regression** — AePlayer's `18anime` adapter exists; verify it surfaces for hentai titles + age gate preserved (B8.1). If broken, 18+ is lost (not just Hanime).
3. **Kodik fallback UX** — KodikPlayer iframe must stand alone without the removed videoProvider machinery (it's self-contained; verify props).
4. **Build-gate enforcement is your friend** — B4 intentionally leaves the DS-allowlist red until B7; locale-parity guards B6. Do not bypass.
5. **Active legacy WT rooms** — a room created pre-deploy on a now-deleted player hits the forward-compat empty state (graceful), not a crash. Acceptable.
6. **Deploy breadth** — `make redeploy-web` ships all concurrent FE work; coordinate the deploy (owner call), and redeploy catalog for B1.

## Out of scope
- Touching `useProviderResolver.ts` adapters / `api/client.ts` source endpoints / scraper plumbing (sources stay).
- Removing KodikPlayer (kept as fallback).
- aePlayer-in-WatchTogether sync (shipped in Plan A).
