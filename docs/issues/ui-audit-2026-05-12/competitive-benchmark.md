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

---

## Supplementary deep-dive — 2026-05-12 follow-up

Per user direction: skip HiAnime; add **Crunchyroll, 9anime, Netflix, FOD**. Where direct browser navigation was blocked, use publicly available designs and wireframes.

### Reachability this pass

| Site | Probe method | Status |
|---|---|---|
| **Crunchyroll** | Browser MCP | **Geo-blocked**: redirects to `/currently-unavailable-in-your-location?source=japan` (audit host is in JP). Filled in via [Crunchyroll Help](https://help.crunchyroll.com) docs + [Kurt Henderson UX case study](https://www.kurthenderson.com/blog/crunchyroll-redesign) + [Jeffrey Li UX case study](https://uxdesign.cc/how-i-reimagined-crunchyrolls-homepage-a-ux-case-study-f349ec04450) + [Crunchyroll Skip-Intro help](https://help.crunchyroll.com/hc/en-us/articles/20369940738708-What-is-the-Skip-Intro-feature) |
| **9anime** | Browser MCP (mirror `9anime.org.lv`) | **Reachable** — fully observed |
| **Netflix** (jp) | Browser MCP | **Reachable** logged-out (home + top-10 row visible; content gated behind login) |
| **FOD** (Fuji TV) | Browser MCP | **Reachable** logged-out (home + category nav visible; cards mostly login-gated) |

### Per-site observations (live or reference-derived)

#### Crunchyroll — derived from official help docs + 2026 UX case studies

- **Native player features (confirmed via help.crunchyroll.com):**
  - Skip Intro button — appears at intro start; user toggle
  - Subtitle language picker — gear icon in player; multiple languages selectable
  - "More Details" panel under show description — exhaustive list of available audio + subtitle languages per show
  - Auto-quality (no manual quality selector in official UI — a long-standing complaint per [Improve Crunchyroll extension](https://chromewebstore.google.com/detail/improve-crunchyroll/elmhfjhlecffodalffipmgpploaihjgh) which restores manual selection)
- **Homepage features (per 2026 UX case studies):** Continue Watching row, Watchlist, Top 10, episode progress indicators, queue/add-to-list, ratings, community comments, publisher metadata.
- **Pain points called out by external UX writers:** "feels like a glorified forum"; manual list-management UX is annoying ("tired of updating my list of animes 'watched' / 'need to watch' / 'watching'"); no manual quality picker.
- **Widely-relied-on browser extensions:** "Improve Crunchyroll", "Crunchyroll With Better Seasons" — adding theater mode, skip intros/outros, mark-watched shortcuts, playback-speed shortcuts, PiP, UI tweaks. **Heavy extension dependence is itself a UX signal** — features users want are missing or hidden in official UI.

**What AnimeEnigma should steal:**
1. **Skip Intro / Skip Outro detection** — Crunchyroll uses pre-marked timestamps; we could ingest from open intro-skip datasets (e.g. aniskip.com) and surface "Пропустить опенинг" CTA at the right offset on HiAnime/Consumet players (Phase 16+ candidate).
2. **"More Details" exhaustive language list** under detail page — every dub/sub language available, surfaced once instead of buried in player.
3. **Don't replicate the pain point.** AnimeEnigma's status pills (UA-018 verified) carry the same "tired of updating my list" risk. Mitigations: auto-progress (already partial via watch_progress), one-tap status change from any card surface, undo-toast pattern.

#### 9anime (mirror `9anime.org.lv`) — directly observed

- **Hero/featured area:** large numbered "Top 10" + multiple "Watch Now" CTAs (Iruma S4, One Piece, Rent-a-Girlfriend S5, Beginning After the End S2, Agents of the Four Seasons).
- **"Popular Today" + "Latest episodes" sections** with **episode-level granularity** in card titles (e.g., "One Piece Episode 1161", "Naruto: Shippuuden (Dub) Episode 500", "Witch Hat Atelier Episode 7").
- **Sub/Dub explicitly labeled** in episode card titles ("(Dub)") and per-episode rows.
- **Anime detail page** is metadata-rich: Status, Studio, Released date, Season ("Spring 2026"), Type ("TV"), Episodes ("24"), Censor flag ("Censored"), Director, Released on, Updated on, Genre tags ("Comedy Fantasy School Shounen"), full character + Japanese VA list, episode list with Sub/Dub flag + release date columns.
- **Bookmark + Follower count** ("Followed 264 people") — quiet social proof on every detail page.
- **Quality / format mentioned in body copy:** "720P 360P 240P 480P MP4/MKV hardsub softsub" — not a player-side picker but mentioned in the SEO-driven body text.
- **Comment form at page bottom** with no moderation visible (anonymous "Leave a Reply"); WordPress-flavored UX.
- **Recommended Series** row at detail page bottom — same pattern as our "Связанные" section but flatter.

**What AnimeEnigma should steal:**
4. **Episode-level granularity in "Latest episodes" row** — instead of "Witch Hat Atelier" linking to detail page, surface the specific new episode ("Эпизод 7") with a direct link to the player at that episode. We have the data (Phase 14 recs has per-episode timestamps).
5. **Sub/Dub indicator on episode cards/rows** — would directly support our multi-translation Kodik provider data which currently isn't exposed at episode-card granularity.
6. **Follower count on detail page** — soft social proof; combine with existing watchlist data (count `anime_list` rows by anime_id with status='watching').
7. **Don't borrow:** the WordPress-flavored comment form (no moderation, no avatars, no threading) is below AnimeEnigma's current bar.

#### Netflix (`netflix.com/jp`) — directly observed (logged-out)

- **Big numeric Top-10 row** — list items use the **giant 3D number behind the poster** (iconic Netflix UX motif). 11 items observed in the sample; clear `1, 2, 3, …` ranking.
- **Locale switcher** prominent in nav: 日本語 / English; one click to swap.
- **Email-first signup**: single email input + "今すぐ始める" CTA; lowest-friction conversion path.
- **Pricing row** explicit: ¥890/mo standard, ad-supported plan mentioned with "詳しくはこちら" link.
- **FAQ accordion** answering the most common questions inline on the page (Netflixとは? / 利用料金は? — "What is Netflix?" / "What's the price?") — handles objections without leaving the page.
- **Footer with phone number** (0120-996-012) + IR/careers/help — full enterprise content. Trust signal.
- **Cookie Privacy preferences** as a separate section — GDPR/local-law compliance is visible UX.

**What AnimeEnigma should steal:**
8. **Numbered Top-10 row with oversized rank typography** — visually distinctive, instantly readable. AnimeEnigma's "Топ аниме" row is great content, but the rank is buried as a small badge inside the card. Borrow the **giant-numeral-behind-poster** treatment. Effort: S.
9. **FAQ accordion on the public landing/marketing surface** — for a self-hosted streaming platform, this could be /о-сервисе with "What is AnimeEnigma?", "Free or paid?", "Что такое OP/ED?", "How do I sync with MAL/Shikimori?". Reduces support-Telegram-bot load.
10. **Don't borrow:** Netflix's logged-out home is mostly conversion-funnel for paid subscriptions — irrelevant to AnimeEnigma's free model. The **Top-10 numeric treatment is the only directly applicable UX**.

#### FOD (`fod.fujitv.co.jp`) — directly observed (logged-out)

- **Persistent top nav with 11 categories**: 国内ドラマ (domestic drama), 国内映画 (domestic film), アニメ (anime), バラエティ (variety), アジアドラマ (Asian drama), アジア映画 (Asian films), 海外映画 (foreign films), 海外ドラマ (foreign drama), ドキュメンタリー (documentary), 音楽・舞台・スポーツ (music/stage/sports), キッズ・ファミリー (kids/family). **Always visible** — not buried in a popup.
- **Dated お知らせ (Notices) section** at top of home: "2026.05.12【障害復旧のお知らせ】…" — system status integrated into the main UI surface, not a separate `/status` link.
- **配信カレンダー (broadcast calendar)** — dedicated route for when content drops; same role as AnimeEnigma's `/schedule`.
- **F1 plan as a separate top-level nav** ("F1™ プラン") — commercial-tier integration. Shows users which content is in which tier without separate exploration.
- **Multi-media platform:** 動画 (video) + マンガ (manga) + 雑誌読み放題 (magazine subscription) + レンタル・PPV (rental/PPV) all reachable from one nav. AnimeEnigma is anime-only — not a direct comparison, but the **clear tier/category labeling** is the UX lesson.
- **psearch ("番組タイトル・出演者")** — search scope explicitly named "show title or cast" so users know what's searchable. Compare AnimeEnigma's `Поиск аниме...` placeholder.
- **Logged-out home shows mostly "Popular作品" + "Popular genres"** — same row pattern as Russian competitors; loaded content cards are largely login-gated.

**What AnimeEnigma should steal:**
11. **System-status integrated as お知らせ row on home** — currently AnimeEnigma has a "Статус системы" link in footer. Promote any active incident or planned maintenance to a dismissible banner on home (only when active). Pairs naturally with the AUTO-NNN issue-tracking infrastructure.
12. **Search-scope clarity** — replace "Поиск аниме..." with something like "Поиск: название или жанр" (or full Shikimori-style hint) so users know they can search beyond title. Effort: S (placeholder text change + maybe small hint).
13. **Always-visible category nav** — when AnimeEnigma adds the multi-axis catalog filter sidebar (Tier E #3), don't hide the categories behind a popup — surface them as persistent top-nav like FOD does.

### Updated cross-site patterns (rolled into existing Tier E)

The original 10 Tier-E recommendations from the first pass all hold. New observations support them and add three more:

**Tier E additions:**
11. **Skip-Intro detection** — Crunchyroll-class UX; aniskip.com or self-hosted timestamps. Effort: M (Phase 16+ candidate). Pairs with player consolidation.
12. **Numbered Top-10 rank treatment (giant numeral)** — Netflix-iconic; small visual lift for an existing row. Effort: S.
13. **System-status banner on home (only when active)** — FOD-style integration of incident state with main UI. Effort: S. Pairs with AUTO-NNN infrastructure.

### Updated matrix (with new sites)

| Dimension | AnimeEnigma | Crunchyroll | 9anime | Netflix (JP) | FOD | animejoy.ru | animevost.org | yummyanime.tv |
|---|---|---|---|---|---|---|---|---|
| 1. Home / landing | 3/5 | 4/5 *(case-study + help docs)* | 3/5 | 5/5 *(numbered Top-10 + email funnel)* | 3/5 | 2/5 | 2/5 | 4/5 |
| 2. Search & filters | 2/5 | 4/5 *(inferred — extensions reveal gaps)* | 3/5 | 4/5 *(inferred, full app)* | 3/5 *(scope-labeled search)* | 3/5 | 2/5 | 4/5 |
| 3. Anime detail | 3/5 | 4/5 *(case-study)* | 4/5 *(metadata-rich)* | 4/5 *(inferred)* | n/a *(login-gated)* | 4/5 | 4/5 | 3/5 |
| 4. Player UX | 3/5 | 4/5 *(skip-intro + sub picker; no manual quality)* | 3/5 *(inferred)* | 5/5 *(inferred — industry leader)* | 4/5 *(inferred)* | 3/5 | 3/5 | 3/5 |
| 5. Watchlist & account | 3/5 | 3/5 *(case-study notes UX friction)* | 2/5 *(bookmark + follower count only)* | 5/5 *(inferred)* | 4/5 *(inferred)* | 2/5 | 1/5 | 3/5 |
| 6. Recommendations / discovery | 4/5 | 5/5 *(inferred)* | 3/5 *(Popular Today + Recommended Series)* | 5/5 *(inferred — content recs are core)* | 3/5 | 1/5 | 1/5 | 3/5 |
| 7. Visual design cohesion | 4/5 | 4/5 *(inferred, polished)* | 2/5 *(WordPress-flavored)* | 5/5 *(industry gold standard)* | 4/5 *(clean JP design)* | 2/5 | 2/5 | 4/5 |
| 8. Mobile UX | 3/5 | 4/5 *(inferred)* | 3/5 | 5/5 *(inferred)* | 4/5 *(inferred)* | 1/5 | 2/5 | 2/5 |
| 9. Loading & perceived speed | 3/5 | 4/5 *(inferred)* | 3/5 | 5/5 *(inferred)* | 4/5 *(inferred)* | 2/5 | 2/5 | 3/5 |
| 10. i18n / locale quality | 5/5 | 5/5 | 1/5 *(EN only)* | 5/5 *(prominent JP/EN switch)* | 2/5 *(JP only)* | 1/5 | 1/5 | 1/5 |

**Updated cross-pattern observations:**
- **Numbered Top-10 row is universal** at the polished tier (Netflix overt; Crunchyroll has it; AnimeEnigma has the data but underplayed visual treatment).
- **Skip-Intro is now table-stakes** on EN streaming (Crunchyroll native; HiAnime/Consumet expected). AnimeEnigma doesn't have this; not blocking but a competitive gap on the EN side.
- **Logged-out home for paid platforms** = funnel; for free platforms (us, 9anime, RU pirate sites) = catalog. Different UX rules. Don't copy Netflix's logged-out home wholesale.
- **9anime's metadata-rich detail page** is what AnimeEnigma should match, not Crunchyroll's slicker one — same audience (free, EN-pirate-adjacent), same content-discovery patterns.

## Audit notes

- **Token budget:** ~10 WebFetch calls succeeded; ~6 failed (403 / 522 / cert / timeout / refused). Browser MCP added: 9anime ✓ live, Netflix ✓ live (logged-out), FOD ✓ live (logged-out), Crunchyroll ✗ geo-block. ~25k additional tokens spent in supplementary pass.
- **Inference flags:** Crunchyroll rows scored on public-knowledge UX case studies — not direct observation. Removable / adjustable.
- **No accounts created, no playback initiated, no captcha bypassed.**

Sources for the Crunchyroll inferences:
- [Crunchyroll UI Redesign — Kurt Henderson](https://www.kurthenderson.com/blog/crunchyroll-redesign)
- [How I reimagined Crunchyroll's homepage — Jeffrey Li (UX Collective)](https://uxdesign.cc/how-i-reimagined-crunchyrolls-homepage-a-ux-case-study-f349ec04450)
- [Crunchyroll Skip-Intro help](https://help.crunchyroll.com/hc/en-us/articles/20369940738708-What-is-the-Skip-Intro-feature)
- [How to change the subtitle language — Crunchyroll Help](https://help.crunchyroll.com/hc/en-us/articles/22934571555476-How-do-I-change-the-subtitle-language)
- [Improve Crunchyroll Chrome extension](https://chromewebstore.google.com/detail/improve-crunchyroll/elmhfjhlecffodalffipmgpploaihjgh)
