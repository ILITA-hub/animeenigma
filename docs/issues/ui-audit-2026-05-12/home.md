# Home — `/` — per-view findings

**View:** `https://animeenigma.ru/` (Home)
**Account:** `ui_audit_bot` logged in
**Viewports:** desktop 1280×800 ✓ probed, mobile 500×723 (pending)
**axe-core:** 4.10.2 — Pass 41, Viol 2 (1 minor real, 1 moderate is extension-injected `#claude-static-indicator-container` and filtered)

## Desktop findings (1280×800)

### [UA-057] Pinned rec reason line leaks English on Russian locale — Severity 2 (Major) — i18n
**View:** Home desktop + mobile (DOM-confirmed)
**Heuristic:** Nielsen #4 Consistency & standards, plus locale integrity
**Evidence:**
- Pinned card on Home has reason line rendered in `<div class="text-sm text-cyan-300 mb-3">` reading "Because you finished Grand Blue" while `<html lang="ru">` and every other UI string is Russian.
- Confirmed via `document.querySelector('.text-cyan-300.mb-3').textContent` → `"Because you finished Grand Blue"`.
- The Phase 13 backend stores `pin_reason` as a free-form string (recs engine sets it in code without locale awareness).

**Why it matters:**
1. Highly visible — sits prominently above the recommended row in cyan-300 on dark.
2. Breaks i18n contract: the rest of the page (including the cyan badge `ВЫБРАНО`) is translated.
3. Looks unprofessional / draft-quality to RU/JA users — the platform's primary audience.

**Citations:**
- `frontend/web/src/views/Home.vue — found via grep "pin_reason"` (per anchor map)
- Backend pin_reason source: `services/player/internal/...` (admin seeds free-text English currently)

**Proposed fix:** Backend should emit `pin_reason_key` + `pin_reason_data` (i18n key + interpolation values), or frontend should treat `pin_reason` as a key into a known dictionary with fallback to raw text. Quick win: translate the current handful of admin-seeded reasons into RU/EN/JA dictionaries and key by stable hash.

---

### [UA-058] Recs row cards (RecItem) do not use `<h3>` for title — Severity 1 (Minor) — a11y / heading order
**View:** Home desktop
**Heuristic:** Nielsen #6 Recognition rather than recall + WCAG 2.4.6 Headings & Labels
**Evidence:**
- All 20 cards in the "Подобрано для вас" row have `card.querySelector('h3') === null`.
- Other rows on Home (Онгоинги, Топ аниме) use `<h3 class="text-sm font-medium text-white group-hover:text-purple-400">` for the title.
- This makes the recs row's titles invisible to assistive-tech heading-navigation and inconsistent with the page's heading structure.

**Why it matters:**
1. Screen readers can't jump between recs by heading.
2. Inconsistent with the rest of Home (cosmetic but visible to a11y tools).
3. axe `heading-order` doesn't flag this because the cards have NO heading at all — but the regression vs other rows is the real issue.

**Citations:**
- `frontend/web/src/components/RecItem.vue — found via grep "PINNED" / "pinBadge"`
- Compare with `frontend/web/src/views/Home.vue — found via grep "group-hover:text-purple-400"` (the Онгоинги cards use h3)

**Proposed fix:** Change RecItem's title element from `<span>`/`<div>` to `<h3 class="text-sm ...">`.

---

### [UA-059] Pinned card has redundant alt text (image alt = surrounding text) — Severity 1 (Minor) — a11y (axe-confirmed)
**View:** Home desktop
**Heuristic:** WCAG 1.1.1 Non-text Content
**Evidence:**
- axe rule `image-redundant-alt` flagged 1 node: 8th card in `.w-32.md\\:w-40.lg\\:w-48` — image `alt="Ван-Пис"` while the title text "Ван-Пис" sits adjacent.
- Failure summary: "Element contains <img> element with alt text that duplicates existing text".

**Why it matters:** Screen readers announce title twice (image alt + adjacent text).

**Citations:**
- `frontend/web/src/components/RecItem.vue — found via grep ":alt="anime.title"` (suspected; verify)

**Proposed fix:** Either `alt=""` (decorative since title is adjacent) or wrap card in a single accessible name container with `<img alt="">`.

---

### [UA-060] `top_contributor` badge surface absent from currently-rendered recs — Severity 0 (Cosmetic / Data) — observation
**View:** Home desktop
**Heuristic:** N/A — observation only
**Evidence:**
- Phase 13 REC-EVAL-01 added `top_contributor` rendering to RecItem; not visible on any of the 20 cards currently rendered.
- Confirmed: `textContent.toLowerCase().includes('top contributor')` = false on all card subtrees.

**Why it matters:** Either no current recs have top_contributor metadata (data-dependent), or the rendering path is broken. Worth confirming during fix-batch.

**Citations:** `frontend/web/src/components/RecItem.vue — found via grep "top_contributor"`

**Proposed fix:** None — record as "verify data path" item.

---

### [UA-061] Continue-watching row absent for logged-in user despite seeded watch_history — Severity 1 (Minor) — discovery
**View:** Home desktop, logged in as `ui_audit_bot`
**Heuristic:** Nielsen #6 Recognition + #1 System status
**Evidence:**
- `ui_audit_bot` has 3 watch_history rows per CLAUDE.md "UI Audit Test User" section.
- Home shows headings: `Подобрано для вас`, `Онгоинги`, `Топ аниме`, `Анонсы`, `Активность`, `Обновления и новости` — no Continue Watching row found.
- Confirmed: no h2 matches /продолжить|continue|смотрит/i.

**Why it matters:** A "Continue Watching" row is table-stakes for streaming UX. Absent from Home, users must navigate to Profile to resume.

**Citations:** `frontend/web/src/views/Home.vue — found via grep "Продолжить" / "continueWatching"` — anchor map shows no such row in current Home.vue.

**Proposed fix:** Add a Continue-Watching row above or near the top of Home for logged-in users with non-empty watch_history. Lazy-load from /api/users/watch-history.

---

## Carry-over verification (desktop)

| ID | Status | Evidence |
|---|---|---|
| UA-042 (Home `/schedule` icon link aria-label) | ✓ Fixed at desktop | `/schedule` link renders as `icon+text` (text "Расписание"); accessible name comes from visible text. Need mobile check where it was originally icon-only. |
| UA-006 (sr-only h1) | ✓ Holds | `<h1 class="sr-only">AnimeEnigma</h1>` present |
| UA-005 (`<html lang>`) | ✓ Holds | `lang="ru"` |
| UA-040 (localized title) | ✓ Holds | `<title>Главная - AnimeEnigma</title>` |

## Mobile findings (500×723) — pending

(Will be added in mobile pass.)
