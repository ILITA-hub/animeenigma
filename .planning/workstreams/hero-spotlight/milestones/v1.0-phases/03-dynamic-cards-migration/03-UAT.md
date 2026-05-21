---
status: partial
phase: 03-dynamic-cards-migration
workstream: hero-spotlight
milestone: v1.0
source:
  - 03-01-SUMMARY.md
  - 03-02-SUMMARY.md
  - 03-03-SUMMARY.md
  - 03-04-SUMMARY.md
  - 03-05-SUMMARY.md
  - 03-06-SUMMARY.md
  - 03-07-SUMMARY.md
started: 2026-05-21T09:40:18Z
updated: 2026-05-21T09:45:00Z
---

## Current Test

[user halted UAT — directive: "EACH card looks poor — refactor them one by one"]
[remaining tests skipped pending visual refactor]

## Tests

### 1. 9-card spotlight loads on home page (logged in)
expected: Visit https://animeenigma.ru/ logged in. Single HeroSpotlightBlock visible. Up to 9 rotating card types (anime_of_day, random_tail, platform_stats, latest_news, personal_pick, telegram_news, now_watching, not_time_yet, continue_watching_new) — some may be absent if you have no eligible data.
result: issue
reported: "I can only see 7 dots, also UI design is bad - refactor it. EACH card looks poor - refactor them one by one"
severity: major
notes: |
  Two failures combined:
  (a) **7 dots not 9.** Per 03-VERIFICATION.md, ui_audit_bot currently
      returns 7 cards (anime_of_day, latest_news, personal_pick,
      platform_stats, random_tail, telegram_news, continue_watching_new)
      — `now_watching` and `not_time_yet` are absent because of data
      eligibility (no other users active in 5min window; user has no
      anime in planned/postponed). This is the documented "up to 9"
      contract but it surprises users — there's no UX signal that
      cards are conditional. Treat as design gap: either backfill the
      slots (anon-fallback content) or surface the eligibility rule.
  (b) **All cards look poor visually.** Cosmetic + layout quality is
      below user's bar across the board — needs full card-by-card
      refactor pass. See per-card gaps below.

### 2. Legacy trending row is gone
expected: On the home page, there is NO second "Trending Now" / "Up Next for you" row of recommendations. The old multi-row carousel (`.recs` with pin badges) is removed. The HeroSpotlightBlock is the only spotlight surface.
result: skipped
reason: user halted UAT to focus on visual refactor

### 3. Anonymous user sees fewer cards (login-only types absent)
expected: Open https://animeenigma.ru/ in a private/incognito window (no login). Spotlight still renders, but `not_time_yet` and `continue_watching_new` cards never appear regardless of how many times you cycle. Anon should show 4–6 cards typically.
result: skipped
reason: user halted UAT to focus on visual refactor

### 4. PersonalPickCard renders 1–3 posters with title swap
expected: When the spotlight rotates to `personal_pick`, you see 1 to 3 anime posters in a grid. Logged-in title differs from anon title (anon = "Up Next" / "Trending Now"-style; login = personalized recs phrasing). On mobile (<768px), only 1 poster shows + a "+ N more →" link footer; on desktop the full 3-poster grid is visible.
result: issue
reported: "EACH card looks poor - refactor"
severity: major

### 5. TelegramNewsCard renders post excerpts safely
expected: When the spotlight rotates to `telegram_news`, you see 1–3 Telegram post excerpts (h4 title + truncated p body, possibly date). Any link in the card opens in a NEW tab (target="_blank") and the link does not break your session (i.e. it's marked rel="noopener noreferrer" — verifiable in DevTools).
result: issue
reported: "EACH card looks poor - refactor"
severity: major

### 6. NowWatchingCard shows live user-watching rows
expected: When the spotlight rotates to `now_watching` (only appears if other users are watching anime in the last 5 minutes), each row has a pulsing green dot, optional poster thumbnail, session text ("username watching anime, ep N"), and a LIVE badge. Clicking a row navigates to `/anime/{anime_id}`. Only public columns (`username`, `public_id`) appear — no private user data.
result: issue
reported: "EACH card looks poor - refactor" (card not currently visible — no eligible session, but visual quality must still be addressed)
severity: major

### 7. NotTimeYetCard appears for logged-in users with planned/postponed items
expected: If you have at least one anime in your list with status "planned" or "postponed", the `not_time_yet` card eventually appears in rotation. It shows one poster (left on desktop, stacked on mobile) and a subtitle that swaps based on the status ("planned" vs "postponed"). The "Watch" CTA links to `/anime/{id}`.
result: issue
reported: "EACH card looks poor - refactor" (card not currently visible — no planned/postponed entries for the test user, but visual quality must still be addressed)
severity: major

### 8. ContinueWatchingNewCard appears with new-episode badge
expected: If you have at least one anime in your watchlist where `episodes_aired > last_watched_episode + 1` (a new episode dropped while you weren't watching), the `continue_watching_new` card appears. It shows a poster with a purple "New episode N!" badge overlaid, the last watched episode number in the meta column, and a "Resume" CTA linking to `/anime/{id}`.
result: issue
reported: "EACH card looks poor - refactor"
severity: major

### 9. Carousel chevron + arrow-key navigation cycles all cards
expected: The HeroSpotlightBlock has left/right chevron buttons and shows ~9 dot indicators (one per active card). Clicking the right chevron advances; left reverses. Keyboard ArrowRight/ArrowLeft also navigates. Pressing chevron 9+ times cycles back to the first card. No card is "stuck" or skipped.
result: skipped
reason: user halted UAT to focus on visual refactor

### 10. Page latency feels snappy on repeat loads
expected: Reload the home page 2-3 times. After the first load, the spotlight cards appear quickly (under ~400ms perceived). No visible "loading…" placeholders flickering for seconds on cached responses.
result: skipped
reason: user halted UAT to focus on visual refactor

### 11. Internal player endpoint is NOT reachable from outside
expected: `curl https://animeenigma.ru/internal/users/anyid/list?status=watching` returns 404. The gateway must NOT proxy `/internal/*` paths — they are Docker-network-only.
result: skipped
reason: user halted UAT to focus on visual refactor

### 12. Cold start: full stack boots and serves spotlight
expected: Kill the catalog + player containers (or run `make redeploy-catalog redeploy-player`), wait for healthchecks. Then load https://animeenigma.ru/ — the spotlight populates within ~5 seconds with no console errors, and `make health` reports green.
result: skipped
reason: user halted UAT to focus on visual refactor

## Summary

total: 12
passed: 0
issues: 6
pending: 0
skipped: 6
blocked: 0

## Gaps

- truth: "Hero spotlight presents 9 distinct cards on a logged-in user with realistic data"
  status: failed
  reason: "User sees 7 dots; `now_watching` and `not_time_yet` correctly absent due to data eligibility but UX gives no signal. Treat as design gap — either backfill or surface the eligibility rule."
  severity: major
  test: 1
  artifacts: [HeroSpotlightBlock.vue, services/catalog/internal/handler/spotlight.go]
  missing: ["Eligibility transparency / fallback content for now_watching + not_time_yet"]

- truth: "Each spotlight card has production-grade visual design"
  status: failed
  reason: "User: 'EACH card looks poor - refactor them one by one'. Applies to all 9 card components: AnimeOfDayCard, RandomTailCard, PlatformStatsCard, LatestNewsCard, PersonalPickCard, TelegramNewsCard, NowWatchingCard, NotTimeYetCard, ContinueWatchingNewCard."
  severity: major
  test: 4,5,6,7,8
  artifacts:
    - frontend/web/src/components/home/spotlight/cards/AnimeOfDayCard.vue
    - frontend/web/src/components/home/spotlight/cards/RandomTailCard.vue
    - frontend/web/src/components/home/spotlight/cards/PlatformStatsCard.vue
    - frontend/web/src/components/home/spotlight/cards/LatestNewsCard.vue
    - frontend/web/src/components/home/spotlight/cards/PersonalPickCard.vue
    - frontend/web/src/components/home/spotlight/cards/TelegramNewsCard.vue
    - frontend/web/src/components/home/spotlight/cards/NowWatchingCard.vue
    - frontend/web/src/components/home/spotlight/cards/NotTimeYetCard.vue
    - frontend/web/src/components/home/spotlight/cards/ContinueWatchingNewCard.vue
  missing: ["Per-card visual refactor proposals + execution plans (one per card)"]
