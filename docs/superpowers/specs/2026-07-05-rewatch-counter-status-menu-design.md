# Rewatch counter → status-menu row (de-clutter the anime hero)

**Date:** 2026-07-05 · **Status:** approved by owner
**Metrics:** UXΔ = +2 (Better) · CDI = 0.02 × 3 · MVQ = Sprite 90%/85%

## Problem

The editable `RewatchCounter` on the anime page hero rendered an unlabeled
`↻ − 0 +` for **any** list status (even «Запланировано») because
`v-if="editable || count > 0"` overrode the design intent «hidden at 0».
New users had no idea what the widget was (owner screenshot, 2026-07-05).
Same zero-noise appeared on own My List rows.

## Decision (owner picked "move into the status menu" over hide-at-0 /
completed-only gating / labeling in place)

1. **Anime hero:** `RewatchCounter` removed entirely — no stepper, no ghost.
2. **Status dropdown menu:** new row `↻ Пересмотрено: N [− +]` between the
   status options and «Удалить из списка» (same `border-t border-white/10`
   separator). Visible only when it can matter: `currentListStatus ===
   'completed'` **or** `currentRewatchCount > 0` (an active rewatch keeps the
   prior count while status is «Смотрю»). +/− do not close the menu (the
   close path is click-outside / status selection only). `setRewatchCount`
   optimistic-PATCH logic unchanged; − disabled at 0.
3. **`RewatchCounter.vue`:** visibility tightened to `v-if="count > 0"` —
   hidden at 0 even when editable. My List rows now show the stepper only
   after the first rewatch; the first manual bump from 0 lives in the
   anime-page status menu (the auto-bump on a rewatch finale remains the
   primary path).
4. **i18n:** `anime.rewatchLabel` added to en/ru/ja.

## Testing

- `RewatchCounter.spec.ts`: editable-at-0 now asserts hidden; new
  stepper-at-count>0 case.
- `Anime.player.spec.ts`: dropped the now-unused `RewatchCounter` stub.
- `/frontend-verify` (DS-lint, i18n parity, real build) before commit.

## Known accepted edge

A user who wants to record a rewatch of an anime they never marked completed
must first set status «Просмотрено» — matches backend semantics
(`POST …/rewatch` refuses non-completed entries).
