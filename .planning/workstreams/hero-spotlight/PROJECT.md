# Project: AnimeEnigma — `hero-spotlight` workstream

**Parent project:** AnimeEnigma (see `/data/animeenigma/.planning/PROJECT.md`)
**Workstream:** hero-spotlight
**Created:** 2026-05-21
**Lifecycle:** Independent of v3.x scraper work, parallel to `notifications`, `raw-jp`, `social`, `ui-ux-audit`.
**Source design doc:** `/data/animeenigma/docs/superpowers/specs/2026-05-21-hero-spotlight-block-design.md` (Approved 2026-05-21)

## Scope of this workstream

Build a **rotating hero spotlight block** at the top of the home page that
surfaces nine different content types in a single auto-cycling carousel:
anime of the day, personal picks, latest changelog, platform stats, live
"now watching" sessions, random discovery from the catalog tail, Telegram
channel news, "is it time yet?" backlog reminders, and "continue watching"
nudges for shows with newly-aired episodes.

The block is **eligibility-aware**: cards with no data are excluded from the
rotation pool entirely (so a logged-out anon doesn't see an empty "continue
watching" slide; a quiet night doesn't show an empty "now watching" card).

The existing `trendingRecs` row at the top of `Home.vue` is removed; its #1
recommendation surfaces as the `personal_pick` card inside the spotlight.

## Core value

> Заходя на главную, пользователь сразу видит **одну точку входа в
> ежедневный контент**: что сегодня смотреть, кто прямо сейчас на сайте, что
> нового на платформе, чего он сам когда-то отложил. Один блок, девять
> разных способов «зацепить» на просмотр — без визуального шума от пустых
> подборок.

Anonymous users: discovery + social signals (5 cards eligible).
Logged-in users: discovery + social signals + personal nudges (up to 9 cards).

## Out of scope for this workstream (v1.0)

| Excluded | Reason |
|---|---|
| Personalization of slide ORDER (only of card content) | Cards returned in fixed type-order, client randomizes starting index. Re-ordering deferred to v1.1 if data shows it matters. |
| Admin-curated "editorial pick" card type | Already covered by CollectionsRow lower on the page; no need to double up in v1. |
| Server-driven A/B testing of card sets | Adds infra debt without a clear short-term payoff. |
| Persisting "seen" cards per user to avoid repetition | Day-cache already rotates content daily; repetition pressure is low. |
| A "tip / feature highlight" card | Static config-driven cards offer diminishing return vs. dynamic content. Reconsider in v1.1. |
| Server-rendered fragment cache (nginx ESI) | Personalization + live cards make edge-caching the whole block infeasible. Per-card Redis is the right granularity. |

## Future iterations (v1.1+)

- Editorial-pick admin form (curated weekly slot)
- Per-user "do not show this card type again" preference
- WebSocket-driven now_watching updates (replace 10s Redis polling)
- Slide-order personalization per user history
