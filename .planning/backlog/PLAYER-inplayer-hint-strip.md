---
id: PLAYER-inplayer-hint-strip
title: Rotating tinted hint strip under the video player (discoverability tips)
captured_at: 2026-07-14
captured_during: admin TODO request (@tNeymik, Telegram)
deferred_from: n/a — new idea, not deferred from prior work
status: backlog
---

# Rotating tinted hint strip under the video player

## Context

Admin (`@tNeymik`) asked to add a TODO for a small, tinted, randomized hint strip
displayed under the player, cycling through short discoverability tips. Examples given:

- "You can download episodes in the app"
- "Leave feedback via the footer button (bottom)"
- "Use `z`/`x` for custom subtitle timing offset"
- "What's your favorite secret feature?" (playful/teaser copy)
- "…and so on" — the admin implied more tips beyond the examples, an open-ended set.

## The idea (what to build if/when picked up)

1. A small, low-emphasis, tinted (Neon-Tokyo token-bound, not hardcoded) text strip
   rendered under `AePlayer.vue`'s chrome — not overlaid on video, doesn't block
   controls.
2. A pool of short tip strings, each optionally routable (e.g. the footer-feedback tip
   could deep-link/scroll to the feedback button; the hotkey tips could reuse copy from
   `composables/aePlayer/playerHotkeys.ts` / `components/tips/HotkeyRows.vue` so the
   in-player strip and the `/tips` secret page share one source of truth instead of two
   copies drifting apart — see `[[project_secret_tips_page.md]]`).
3. Rotation: randomized pick on mount / periodic interval (need a UX call on cadence —
   too fast is distracting, too slow never gets seen); should not persist an annoying
   re-shown tip if the user already dismissed/acted on it (localStorage-backed
   "seen" set, similar in spirit to other one-time-nudge patterns in the player).
4. i18n: en/ru/ja — every tip string needs all three locales (gate:
   `[[feedback_i18n_three_locales_gate]]`).
5. Respect the mobile redesign's pseudo-fullscreen layout
   (`[[project_mobile_player_redesign_season_downloads]]`) — under-player space is
   tighter on mobile; may need to hide/collapse there.

## Why deferred (not done inline)

- Admin explicitly asked for a TODO capture ("Добавь туду"), not an immediate build —
  per the maintenance-bot feedback-store rules, a capture request is recorded as
  backlog, not implemented.
- Underspecified: exact tip copy set, rotation cadence, dismiss/seen behavior, and
  whether tips deep-link anywhere are all open UX questions worth a real design pass
  (brainstorming skill) rather than a maintenance-bot guess.
- Touches the player chrome, a heavily-audited surface
  (`[[reference_aeplayer_canonical_doc]]`) — any visual change there should go through
  the design-prototyping sandbox per `CLAUDE.md`, not a quick inline edit.

## Cost estimate

| Component | Effort (Fib) | Risk |
|---|---|---|
| Tip pool + rotation logic + seen-state | 3 | Low |
| Under-player chrome placement (desktop) + DS tokens | 2 | Low |
| Mobile pseudo-FS layout accommodation | 2 | Low |
| i18n (en/ru/ja) for full tip set | 1 | Low |
| Design-prototyping sandbox pass before Vue | 3 | Low |

## Cross-references

- Existing tip surface: `frontend/web/src/views/TipsPage.vue` +
  `frontend/web/src/components/tips/HotkeyRows.vue` (secret `/tips` page, mirrors
  `composables/aePlayer/playerHotkeys.ts`) — `[[project_secret_tips_page.md]]`
- Player reference: `docs/aeplayer-reference.md`, `[[reference_aeplayer_canonical_doc]]`
- Source: admin message from `@tNeymik`, Telegram, feedback entry
  `2026-07-14T15-40-22_tNeymik_telegram`
