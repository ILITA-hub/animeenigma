# Лудка — Phase 6: Variant-C UI + backdrop/back images

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:subagent-driven-development. Visual source of truth: `.brainstorm/content/gacha-banner-select-v21.html` (playable mock approved by owner — port it 1:1).

**Goal:** Replace the minimal Phase-5 player UI with the approved Variant-C experience, and add the two new image slots (banner backdrop, card back).

## Locked design decisions (from the v21 iteration)
1. `/gacha` = ALL-IN-ONE: hero banner slider `clamp(380px,62vh,560px)` (arrows+dots, per-banner art) + spin dock (pity bar «До гаранта SSR: n/90», «🃏 Выпадения», Крутить ×1/×10) + daily-claim card in page header. **`/gacha/:id` route is removed** (redirect → `/gacha?banner=:id`, slider preselects).
2. Banner NEW FIELD `backdrop_path` (separately uploaded image; admin dialog gets a second upload slot, kind=`banners`). Slider slide background = backdrop (cover + bottom scrim `rgba(8,8,15,.15→.92)`).
3. Card NEW FIELD `back_path` (optional; admin card dialog third upload slot, kind=`cards`). 3D viewer card back = uploaded image, else the default branded back (◆ emblem, breathing rings, diamond lattice, ANIMEENIGMA wordmark — CSS from v21).
4. «Выпадения» modal: pool grouped SSR→N with tier-rate headers («шанс тира: 1% · гарант на 90-й», SR «· минимум 1 в ×10»); unowned cards VISIBLE (saturate(.45) brightness(.75) + dashed border), counts «выбито X из Y».
5. Pull flow: gem ceremony (spiral sparks→charge→crack seams→burst; rarity-tease color, SSR gold + ~3s vs ~2s, skip) → white flash → sequential fullscreen 3D viewer (fly-in from depth w/ spin; shockwave+particles tinted by tier; rarity radial bg, rotating godrays SSR; drag-rotate via rAF, cos-damped vertical, spring-back; sin-based holo shine no-repeat; NEW/×N badges backface-hidden; counter, «Дальше ›»/«К результатам ›», «Пропустить всё ››») → summary grid 5 cols (×10 = 5×2; mobile 3 cols), SSR pinned-visible fix (`opacity:1` with shake class).
6. Profile collection tab: **owned cards ONLY** (no unowned, no silhouettes) — per-rarity sections of owned cards with ×N.

## Tasks
**T1 (backend, small):** `services/gacha/` — add `BackdropPath string` to `domain.Banner` + `BackPath string` to `domain.Card` (gorm size:512, json `backdrop_path`/`back_path`); thread through Create/Update requests in content service + admin handlers (AutoMigrate adds columns). Tests: round-trip in existing repo tests. Path-scoped commit; redeploy-gacha (controller).
**T2 (frontend api/types):** add the two fields to TS types + create/update payloads in `src/api/gacha.ts`; admin dialogs (AdminGacha.vue): banner dialog gets «Задник» upload slot, card dialog gets «Рубашка (опционально)» upload slot — both reuse the existing file-or-URL upload flow.
**T3 (frontend, the big port):** rebuild `src/views/Gacha.vue` as Variant C (merge in current GachaBanner.vue logic; delete the separate view + its route; redirect `/gacha/:id`→`/gacha?banner=:id`); new components `src/components/gacha/` — `GachaSlider.vue`, `SpinDock.vue`, `DropsModal.vue`, `GemCeremony.vue`, `CardViewer3D.vue` (port v21 CSS/JS 1:1 into Vue; scoped styles; rarity hues = exempt list cyan/teal/indigo/orange; respects `prefers-reduced-motion` → skip straight to summary), `PullSummary.vue`. Collection tab: owned-only filter. i18n en/ru/ja for all new strings. Specs: ceremony skip → summary; viewer next/skip; drops modal grouping; slider preselect from query.
**T4 (controller):** gates (vue-tsc, vitest, ds-lint, i18n-lint), redeploy-gacha + redeploy-web, owner smoke, push, memory.

## Notes
- DS lint: animations/colors must use exempt hues or semantic tokens; the v21 hex values map: cyan #00d4ff→`--brand-cyan` token/`cyan-400`, teal/indigo/orange via Tailwind exempt classes; any raw hex left in scoped CSS needs the allowlist file (prefer tokens).
- The mock's `TEASE`/`RATE` data come from the API (`pity_threshold`, weights are display-only — hardcode tier % labels from config defaults or add them to the banners payload; simplest: show static 69/22/8/1 labels for now, matching prod config).
