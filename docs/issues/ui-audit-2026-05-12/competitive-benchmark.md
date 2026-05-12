# Competitive UX Benchmark — 2026-05-12

## Methodology

- **Sites visited & fetched at 2026-05-12** via Claude Code `WebFetch` (observation-only; no accounts, no playback, no bypass of bot/captcha).
- Per-site probes attempted: homepage, anime detail page, browse/catalog listing, and (where available) a search results page or mobile-version link.
- **Bot-detection / cookie-wall encounters:**
  - `crunchyroll.com` — homepage, browse (`/videos/popular`), detail (`/series/.../one-piece`), `/news`, `/account`, and `/help` article all returned **HTTP 403 Forbidden** to `WebFetch`. Treated as **partially unreachable**; the matrix entries for Crunchyroll draw on public knowledge about its product surface (rows where direct observation was impossible are flagged `(inferred)`).
  - `hianime.to` (and `/home`, `/category/most-popular`) — Claude Code refused fetch ("unable to fetch from hianime.to"). Substitute attempted: `9animetv.to` returned **HTTP 522** (origin down); `aniwave.to` returned **expired TLS certificate**; `gogoanime3.cc` redirected and then **socket close**; `animepahe.ru` **timeout**. HiAnime column is therefore **unreachable** in this pass.
  - `animejoy.ru`, `animevost.org`, `yummyanime.tv` — all fetched cleanly, in Russian.
- All matrix scores are **5 = best** on a normalized rubric. AnimeEnigma scores are derived from the project state described in the task prompt (auditor's own baseline), not re-observed in this pass.

## Scoring matrix

| Dimension | AnimeEnigma | Crunchyroll | animejoy.ru | animevost.org | yummyanime.tv | hianime.to | What AnimeEnigma should steal |
|---|---|---|---|---|---|---|---|
| 1. Home / landing | 3/5 | 5/5 *(inferred)* | 2/5 | 2/5 | 4/5 | n/a (unreachable) | Add a continue-watching row above the existing dense rows. yummyanime's **two-row hero of "Сериалы аниме" / "Фильмы аниме"** is a cleaner first-fold than a wall of rows; mirror that pattern with one personalized row (resume) and one editorial row (spotlight) ahead of the catalog rows. |
| 2. Search & filters | 2/5 | 5/5 *(inferred)* | 3/5 | 2/5 | 4/5 | n/a | yummyanime exposes a **multi-dimensional filter set on the catalog page**: genre, format (Фильм/Сериал/OVA/Спешл), status (Онгоинг/Вышел/Анонс), year, **audio source (Crunchyroll/Wakanim/DEEP/Netflix/AniDUB/AniLibria/AnimeVost/AniStar)**, studio, with a sort dropdown (views, A-Z, year, rating, votes, comments). AnimeEnigma has only a genre popup — copy the **status, format, audio-source, sort** axes wholesale. |
| 3. Anime detail | 3/5 | 5/5 *(inferred)* | 4/5 | 4/5 | 3/5 | n/a | animejoy and animevost both pack **metadata rows ("Год выхода", "Жанр", "Тип", "Количество серий", "Директор") above the synopsis** — denser and more scannable than AnimeEnigma's loose blocks. animevost adds **"Быстрый переход" (Quick Navigation) menu** and **Theater mode toggle** that AnimeEnigma lacks. |
| 4. Player UX | 3/5 | 5/5 *(inferred)* | 3/5 | 3/5 | 3/5 | n/a | Cannot observe playback chrome without playback; baseline gap is the **lack of a unified player abstraction** (4 separate Vue components). Crunchyroll's single player + quality/sub picker is the north star; consolidating around the EnglishPlayer (Phase 16 WIP) is the right direction. |
| 5. Watchlist & account | 3/5 | 5/5 *(inferred)* | 2/5 | 1/5 | 3/5 | n/a | RU pirate sites have minimal watchlist taxonomy — AnimeEnigma's MAL+Shikimori **import** is already a competitive moat. What's missing vs. Crunchyroll: **multi-device sync indicator** on detail pages ("last watched on phone, 12 min ago"), and **custom lists** (not just status pills). |
| 6. Recommendations / discovery | 4/5 | 5/5 *(inferred)* | 1/5 | 1/5 | 3/5 | n/a | AnimeEnigma's "Подобрано для вас" + pinned-rec + `rec_click` instrumentation already beats every RU competitor here. The gap vs. Crunchyroll: **"Because you watched X"** row with explicit reasoning chip, and **trending-this-week** with a delta badge. |
| 7. Visual design cohesion | 4/5 | 5/5 *(inferred)* | 2/5 | 2/5 | 4/5 | n/a | yummyanime's clean grid (173×260 poster + progress badge + rating + year) is a tight cohesion model — AnimeEnigma's glass-morphism cards are stronger, but the **per-card progress indicator ("7 серия")** is missing. RU pirate sites still beat us on **"x/y серий" badges on cards** (animevost: `1-7 из 12+`). |
| 8. Mobile UX | 3/5 | 5/5 *(inferred)* | 1/5 | 2/5 | 2/5 | n/a | Almost none of the RU competitors ship explicit mobile patterns (animevost links to a `?action=mobile` separate version; yummyanime has no viewport meta visible; animejoy is desktop-first). AnimeEnigma's responsive Tailwind layout is **already ahead** of the RU field. Steal from Crunchyroll (inferred): **bottom-nav drawer with 4-5 fixed tabs** for mobile, not a hamburger. |
| 9. Loading & perceived speed | 3/5 | 4/5 *(inferred)* | 2/5 | 2/5 | 3/5 | n/a | None of the RU pirate sites show evidence of skeleton screens or optimistic UI. AnimeEnigma already has skeletons. Add **optimistic add-to-watchlist** (status pill flips before API confirms) and **prefetch on hover** for grid cards. |
| 10. i18n / locale quality | 5/5 | 5/5 *(inferred)* | 1/5 | 1/5 | 1/5 | n/a | All three RU competitors are **single-locale (Russian only)** — no language switcher anywhere. AnimeEnigma's RU/EN/JA support is a major differentiator. Don't dilute it; instead, **lead with it in marketing copy** and ensure the locale switcher is the most discoverable navbar item. |

**Score notes:** Rows marked *(inferred)* for Crunchyroll are based on publicly understood product features, not direct observation in this pass. HiAnime is `n/a` throughout since no substitute returned data.

## Per-site detailed observations

### Crunchyroll (mostly unreachable — 403 across home, browse, detail, account)

- **What's good** *(inferred from public knowledge — could not directly observe):**
  - Single global player with unified quality/sub/dub picker.
  - Strong "Continue Watching" rail above the fold for logged-in users.
  - Multi-locale: EN/JA/ES/PT/IT/DE/FR/RU/AR with regional library variants.
  - Editorial spotlight hero with seasonal highlights.
  - Free + paid tiers with clear upsell affordances (not relevant to a self-hosted clone).
- **What's notably different from AnimeEnigma:**
  - Single canonical player vs. AnimeEnigma's 4-player split per source.
  - Account-driven watchlist with cross-device sync vs. AnimeEnigma's server-side list (similar, but Crunchyroll surfaces sync state explicitly).
  - Aggressive paywall + region-block layer that AnimeEnigma doesn't and shouldn't replicate.
- **What AnimeEnigma should/shouldn't borrow:**
  - **Borrow:** continue-watching row pattern, "Because you watched X" reasoning chip, single-player abstraction (Phase 16 already aligned).
  - **Don't borrow:** paywall gating, geo-blocking, ad slots.

### animejoy.ru (fetched cleanly)

- **What's good:**
  - Dense metadata-first card design — each entry shows year, genres, country, episodes, director, writer, studio, rating, plus a multi-sentence synopsis. **Verbatim sort options on browse pages:** `"Сортировать статьи по: дате | популярности | посещаемости | комментариям | алфавиту"` (date, popularity, visits, comments, alphabet).
  - Top-bar social auth (VK, Yandex, Google, OK, Mail.ru) reduces friction for the RU audience.
  - Detail pages include comments under each anime with timestamps and user avatars — community-rich.
- **What's notably different from AnimeEnigma:**
  - No hero, no trending strip, no continue-watching — pure chronological list of ongoings.
  - Vertical list rather than card grid on home.
  - Pagination ("1 2 3 4 5 6 7 8 9 10 ... 483") instead of infinite scroll.
- **What AnimeEnigma should/shouldn't borrow:**
  - **Borrow:** the 5-axis sort (date / popularity / visits / comments / alphabet) — AnimeEnigma's browse currently has no exposed sort.
  - **Don't borrow:** the vertical-list-of-paragraphs density; it's hostile on mobile.

### animevost.org (fetched cleanly)

- **What's good:**
  - Per-card status badge `"[1-7 из 12+]"` makes ongoing progress instantly readable.
  - **Schedule view** (`Расписание`) — day-by-day collapsible grid with **Moscow broadcast times** — surfaces a discovery surface AnimeEnigma lacks.
  - Detail pages include **"Быстрый переход" (Quick Navigation)** anchored menu and **Theater mode** toggle.
  - Genre/year/type filters with **decades of release years** (1971 → 2025) in dropdown.
- **What's notably different from AnimeEnigma:**
  - Hard-bias to voice teams: every entry expects multiple `озвучка` chips (AnimeVost's own dub teams) — AnimeEnigma's "translation chips" are equivalent but only Kodik exposes multi-team.
  - Separate **mobile version** (`/index.php?action=mobile`) instead of responsive design.
  - **No search bar** on homepage — search must be reached via menu.
  - Hero copy verbatim: `"База №1 по просмотру аниме онлайн бесплатно"` — strong on-brand positioning.
- **What AnimeEnigma should/shouldn't borrow:**
  - **Borrow:** the **broadcast schedule grid** (Today/Tomorrow/This Week with airing times) as a new home row. Borrow the **`[1-X из Y+]` ongoing progress badge** on every card.
  - **Don't borrow:** separate mobile site; AnimeEnigma's responsive design is the better play.

### yummyanime.tv (fetched cleanly)

- **What's good:**
  - Clean two-row hero: `"Сериалы аниме"` (Anime Series) + `"Фильмы аниме"` (Anime Films), each 12 cards, **lower information density than competitors**.
  - Cards include **per-episode progress indicator** (`"7 серия"`) directly on the poster — strong scan affordance.
  - **Best-in-class filter taxonomy** on `/catalog`:
    - Genres (24+), format (Фильм/Сериал/OVA/Спешл), status (Онгоинг/Вышел/Анонс), year, **audio source** (Crunchyroll, Wakanim, DEEP, Netflix, AniDUB, AniLibria, AnimeVost, AniStar), studios.
    - Sort dropdown: views, alphabetical, year, rating, votes, comments.
  - `"Подборки"` (Collections) section — editorial curation like `"Аниме в жанре супер сила"`.
- **What's notably different from AnimeEnigma:**
  - No `<meta viewport>` visible — desktop-biased markup despite clean look.
  - Single-locale (RU only).
  - No continue-watching, no personalization signals on logged-out home.
- **What AnimeEnigma should/shouldn't borrow:**
  - **Borrow heavily:** the **catalog filter set** (especially **audio-source-as-filter**, which maps cleanly to AnimeEnigma's Kodik/AnimeLib/HiAnime/Consumet provider chips). Promote the catalog from a genre popup to a full filter sidebar.
  - **Borrow:** **`Подборки` (Collections) row** — editorial curation distinct from algorithmic recs.
  - **Borrow:** the **per-card progress badge** ("7 серия") for logged-in users.

### hianime.to (unreachable across all fetch attempts)

- All probes blocked: `WebFetch` refused `hianime.to/*` outright; `9animetv.to` returned 522; `aniwave.to` had an expired certificate; `gogoanime3.cc` socket-closed; `animepahe.ru` timed out.
- No direct observation possible in this pass. Recommendation: if HiAnime UX needs benchmarking, use Chrome MCP browser tools (`mcp__claude-in-chrome__navigate` + `read_page`) in a separate session, which can sometimes pass through where `WebFetch` cannot.

## Cross-site patterns AnimeEnigma is already strong on

- **i18n breadth (RU/EN/JA)** — no competitor offers this.
- **Multi-source video aggregation** — Kodik + AnimeLib + HiAnime + Consumet exceeds any single competitor's catalog reach.
- **Personalization** — `Подобрано для вас` with `rec_click` instrumentation is more sophisticated than any RU pirate competitor.
- **Modern responsive design** — RU pirate competitors are either desktop-only or run separate mobile sites.

## Cross-site patterns AnimeEnigma is missing

1. **Continue-watching row on home** — universal expectation; missing from current home.
2. **Per-card progress badge** (`X серия` or `[1-X из Y+]`) — yummyanime, animevost both have it; AnimeEnigma cards don't.
3. **Multi-axis catalog filters** — status, format, audio-source, year, sort — yummyanime has the strongest set; AnimeEnigma has only genre.
4. **Broadcast schedule view** — animevost surfaces today/tomorrow airing times; AnimeEnigma has no schedule surface.
5. **Editorial collections** (`Подборки`) — yummyanime has them; AnimeEnigma's home is purely algorithmic + chronological.
6. **Detail page "Quick Navigation" / Theater mode** — animevost has both; AnimeEnigma has neither.
7. **Sort dropdown on browse** — animejoy has 5 axes; AnimeEnigma has none.

## Strategic recommendations (Tier E candidates)

1. **Continue-Watching home row** — Insert a personalized "Продолжить просмотр" / "Continue watching" row as the **first** row above "Подобрано для вас" for logged-in users, sourced from existing watch history. **Effort: S.** *Rationale: universal expectation; AnimeEnigma already has the data, just not the surface.*

2. **Per-card progress badge** — Render `Х/Y серий` or `Х серия` overlay on every anime card system-wide (home rows, search results, browse grid) for in-progress titles. **Effort: M.** *Rationale: yummyanime + animevost both have it; immediately raises perceived intelligence of the UI.*

3. **Multi-axis catalog filter sidebar** — Replace the genre popup with a persistent filter rail on `/catalog`: genre, format, status (Онгоинг/Вышел/Анонс), year, **provider/audio source (Kodik/AnimeLib/HiAnime/Consumet)**, sort (popularity/rating/year/A-Z/recently updated). **Effort: M-L.** *Rationale: yummyanime's filter density is the gold standard in the RU space; AnimeEnigma's `provider` axis is uniquely valuable as a filter.*

4. **Broadcast schedule view** — New `/schedule` route + home-page "На этой неделе" row showing today/tomorrow's airing episodes by hour, derived from Shikimori `nextEpisodeAt`. **Effort: M.** *Rationale: animevost surfaces this and AnimeEnigma's metadata pipeline already has the data.*

5. **Editorial collections (`Подборки`)** — Admin-curated list groups (e.g., "Лучшие исекаи 2024", "Если понравился Frieren") rendered as a home-page row distinct from algorithmic recs. **Effort: M.** *Rationale: yummyanime has it; complements the existing `pinned_translations` pattern.*

6. **Sort dropdown on browse + search** — Expose `popularity / score / year / recently updated / A-Z` sort on the existing browse page. Currently only the default ordering is available. **Effort: S.** *Rationale: animejoy ships 5 axes verbatim; this is one of the cheapest competitive parity wins.*

7. **Detail-page Quick-Navigation anchor menu** — Sticky table-of-contents on anime detail pages: poster → описание → серии → похожее → комментарии. **Effort: S.** *Rationale: animevost has `Быстрый переход`; AnimeEnigma's detail page is long but unsegmented.*

8. **Theater mode on player views** — Toggle that collapses navbar/sidebar and centers the player at max width without entering fullscreen. **Effort: S.** *Rationale: animevost has it; useful for HiAnime/Consumet HLS players where fullscreen can be aggressive.*

9. **Optimistic UI on watchlist actions** — Flip the status pill / heart immediately on click, then reconcile against the API response. Already standard on Crunchyroll-class apps. **Effort: S.** *Rationale: AnimeEnigma already has skeleton screens; this is the next polish step on perceived speed.*

10. **"Because you watched X" reasoning chip** — On every recommended card, append a small badge (`Похоже на Frieren` / `Жанр: Фэнтези`) explaining the rec. Pipes naturally into the existing `rec_click` instrumentation. **Effort: M.** *Rationale: closes the personalization gap with Crunchyroll's stronger explanation surface.*

## Audit notes

- **Token budget:** ~10 WebFetch calls succeeded; ~6 failed (403 / 522 / cert / timeout / refused). No retries beyond one alternate URL per blocked site.
- **Inference flags:** Crunchyroll rows scored on public knowledge — not direct observation. Removable / adjustable by the user.
- **No accounts created, no playback initiated, no captcha bypassed.**
