# Phase 14: Marketing-surface polish - Context

**Gathered:** 2026-05-13
**Status:** Ready for planning
**Mode:** Auto-generated (autonomous run, three Tier-E quick wins on marketing/discovery surface)

<domain>
## Phase Boundary

Three small Tier-E wins on the marketing/discovery surface:

- **UX-28** — Soft social proof: follower count on detail page derived from `anime_list` rows with `status='watching'`. Tier E #18.
- **UX-29** — Search-scope clarity: placeholder text disambiguates ("Поиск: название или жанр"). Tier E #19. Update i18n only.
- **UX-30** — FAQ accordion on a public marketing surface (`/о-сервисе` or `/about`). Tier E #20.

</domain>

<decisions>
## Implementation Decisions

### Claude's Discretion (autonomous mode)

**UX-28 — Follower count:**
- New backend endpoint: `GET /api/anime/:id/watchers-count` (catalog or player — choose player since `anime_list` lives there). Returns `{ count: int }`. Public, no auth.
- Implementation: simple `SELECT COUNT(*) FROM anime_list WHERE anime_id = ? AND status = 'watching'`. Cache 15 minutes via the existing cache layer.
- Frontend: `Anime.vue` calls the endpoint after anime data loads; renders small chip near the score: `<Badge variant="default">👥 {{ count }}</Badge>` (using existing Badge component).
- Hidden when count < 5 to avoid embarrassingly empty signals on new/niche titles.
- i18n: `anime.watchersCount` with `{count}` placeholder.

**UX-29 — Search placeholder clarity:**
- Update existing i18n key `search.placeholder` across en/ru/ja:
  - EN: "Search: title or genre" (was "Search anime...")
  - RU: "Поиск: название или жанр"
  - JA: "検索: タイトルまたはジャンル"
- Backend genre matching: leave as-is. Search backend already matches genre names via the existing search query (Shikimori passes through). If verification shows it doesn't, defer the backend update to Phase 20 polish — UX-29 only commits to the UI clarity claim.

**UX-30 — FAQ accordion:**
- New view: `frontend/web/src/views/About.vue` mounted at `/about`. Optional aliased route `/о-сервисе` redirects to `/about`.
- Content: 6-8 FAQs curated for AnimeEnigma users. Topics:
  1. What is AnimeEnigma? (one-paragraph overview)
  2. Is it free? Are there ads? (free, no ads, self-hosted notes)
  3. Where do the videos come from? (Kodik / AnimeLib / HiAnime / Consumet)
  4. How do recommendations work? (S1-S5 signals overview, no PII)
  5. Can I import my MAL list? (Yes, via Profile)
  6. Is there a mobile app? (No, mobile-responsive web)
  7. How do I report a broken player? (use the in-player Report button)
  8. Who runs this site? (small self-hosted group)
- Each FAQ uses a native `<details>` element (no JS required for accordion):
  ```html
  <details class="border-b border-white/10 py-3">
    <summary class="cursor-pointer text-lg font-medium text-white">{{ q }}</summary>
    <p class="mt-2 text-white/70">{{ a }}</p>
  </details>
  ```
- Page header: localized title + 1-line subtitle.
- Navbar link: add "О сервисе / About" to the Navbar (desktop + mobile drawer). Optional — alternatively, link only from Footer. Pick footer to keep the navbar minimal.
- i18n: `about.title`, `about.subtitle`, `about.faqs.{q1..q8}.q`, `about.faqs.{q1..q8}.a`. (1 + 1 + 16 = 18 keys × 3 locales = 54 entries.)

### Locked from ROADMAP

- Three items batched cleanly. No dependencies.

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets

- `services/player/internal/repo/list.go` (if exists) — anime_list repo where watchers count query lives.
- `services/player/internal/handler/list.go` — pattern for adding public endpoints.
- `frontend/web/src/components/ui/SearchAutocomplete.vue` — placeholder is `$t('search.placeholder')`. Just update i18n.
- `frontend/web/src/router/index.ts` — register new `/about` route.

### Established Patterns

- Public endpoint pattern: gateway proxies `/api/anime/*` to catalog by default. Need to add a player route for anime-list aggregates — alternative: register `/api/anime/:id/watchers-count` under catalog gateway routing and have catalog query the player DB (same Postgres, different service). Simpler: register a new gateway route `/api/anime/:id/watchers-count` that proxies to player.
- Use existing `Badge` component for the chip.

### Integration Points

- No new tables.
- Gateway routing change required for the new endpoint.
- Footer component for the about link — check `frontend/web/src/components/layout/Footer.vue` if it exists.

</code_context>

<specifics>
## Specific Ideas

- The follower count is "soft social proof" — render hidden below 5, render visible above. Use number-formatting via `Intl.NumberFormat` to render "1.2K" / "1,234".
- FAQ uses native `<details>` for zero-JS accordion behavior. Keyboard accessible by default. SEO-friendly (content always in DOM).

</specifics>

<deferred>
## Deferred Ideas

- Footer redesign — keep existing Footer pattern; just add the About link.
- Search backend explicit genre matching — Phase 20 if verification shows it's needed.
- Animated FAQ open/close — native `<details>` is good enough; CSS transitions on `<details[open]>` can be added in Phase 20.

</deferred>
