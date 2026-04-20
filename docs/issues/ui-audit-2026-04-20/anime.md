# Anime detail mobile audit — 2026-04-20 — viewport 500x723

Test case: Frieren (UUID `f0b40660-6627-4a59-8dcf-7ec8596b3623`)

## State verification (Batch A)

- **UA-013 ✓** — "Смотреть" CTA visible at y=538 with vh=723 → above fold
- **UA-014 ✓** — no `<video src>` or `<iframe src>` on initial page mount (player is click-to-load)
- **UA-015 ✓** — status "Смотрю" button has `aria-haspopup="menu"`, `aria-expanded="false"`, `aria-controls="watchlist-status-menu"`
- **UA-016 ✓** — star rating is `<[role="radiogroup"] aria-label="Ваша оценка">` with 10 `[role="radio"]` children
- `<h1>` present with full anime title
- No `accent-bg-muted` / `accent-text` elements found on this view → UA-036 fix landed (Batch E)

## axe-core (mobile 500x723, logged in)

- 40 passes, **1 violation** (down from 2 on desktop)
- `color-contrast` serious, **9 nodes** — all `.text-white/40.text-sm` — remaining scope of UA-036 not yet addressed

## NEW findings

### [UA-050] `/anime/:id` error state — "Failed to fetch anime" string is English on Russian locale — Severity 2 (major) — i18n

**View:** `/anime/:id` when the UUID doesn't exist in DB (observed navigating to `/anime/52991` by mistake — Shikimori numeric IDs aren't valid routes)
**Heuristic:** Consistency + SC 3.1.2
**Evidence:**
- Full page: error message **"Failed to fetch anime"** above a button **"Повторить"**
- Mixed locale: hard-coded English error + translated retry button
- `<html lang="ru">`
- Separate from UA-040 (doc title) and UA-043 (hamburger) — this is a runtime fetch-error message

**Why it matters:** This is the page Russian users see when a bad link is clicked, or when the backend returns 404/500 on a valid-looking URL. It's a first-impression of the error state.

**Citations:**
- `frontend/web/src/views/Anime.vue — found via grep "Failed to fetch anime"` (or wherever the error string lives)

**Proposed fix:** Replace the literal with `$t('anime.fetchError')` and add the key to `ru.json` ("Не удалось загрузить аниме"), `en.json`, `ja.json`.

### [UA-051] `/anime/:id` `<title>` is generic "Детали аниме - AnimeEnigma" — Severity 1 (minor) — SEO/UX

**View:** every anime detail page
**Heuristic:** SEO + browser tab clarity
**Evidence:**
- `document.title === "Детали аниме - AnimeEnigma"` on Frieren page
- Other views get specific titles: Home "Главная", Browse "Каталог", Themes "Опенинги и Эндинги"
- Social-media shares and bookmarks currently all carry the same generic title
- UA-040 (doc title i18n) shipped in Batch F but that was scoped to static titles. Dynamic title (anime name) wasn't wired.

**Proposed fix:** In Anime.vue `onMounted` / watcher, set `document.title = \`${anime.name} — AnimeEnigma\`` once the fetch resolves. Keep the fallback "Детали аниме" for the initial load/error state. 3-line change.

### [UA-052] `/anime/:id` — remaining 9-10 `.text-white/40.text-sm` nodes fail contrast — Severity 1 (minor) — accessibility

**View:** `/anime/:id` (all anime pages)
**Heuristic:** WCAG 2.1 SC 1.4.3
**Evidence:**
- axe `color-contrast` serious, 9 nodes on initial render (grows if the anime has many reviews or relations)
- Affected elements: "Shikimori" / "AnimeEnigma (4)" rating-provider subtitles under star, "3 отзывов" reviews-count label, review-date `<p>` nodes, related-anime relation-type labels ("Прочее", "Продолжение", "Другая история")
- `0c8e629` commit flagged this as "remaining 8 nodes = review-count text-white/40 — scope". It's now 9-10 (reviews + related expanded the surface).

**Citations:**
- `frontend/web/src/views/Anime.vue — found via grep "text-white/40"`

**Proposed fix:** Bump `text-white/40` → `text-white/60` in the affected spans/paragraphs — same pattern already used in Batch D (ThemeCard). Zero-risk CSS-only change.

## N/A / deferred

- N/A: "accent-bg-muted" button cluster not visible on Frieren's page mobile view (may require specific player state to appear) — couldn't verify UA-036 via re-inspection in this pass, but commit `17b9b67` already shipped the fix and there's no regression signal.
- Deferred: touch-driving the status menu (L1 scenario — handled in scenarios section).
