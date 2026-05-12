# Profile — `/user/:public_id` — per-view findings

**View:** `https://animeenigma.ru/user/ui-audit-bot`
**Account:** `ui_audit_bot` (own profile)
**Viewports:** desktop 1280×800 ✓ probed (Мои списки tab + Настройки tab)
**axe-core:** "Мои списки" 1 viol (color-contrast), Settings tab 2 viol (1 critical button-name, 6-node color-contrast)

## Desktop findings (1280×800)

### [UA-065] Copy-API-key button has no accessible name — Severity 3 (Catastrophic) — a11y
**View:** Profile → Настройки tab → API key card
**Heuristic:** WCAG 4.1.2 + axe `button-name` (critical)
**Evidence:**
- `<button class="ml-auto p-1.5 rounded hover:bg-white/10 text-white/60 hover:text-white transition-colors">` with SVG copy icon inside, NO `aria-label`, NO `title`, NO visible text.
- Sibling structure: `<div class="flex items-center gap-2 p-3 bg-white/5 rounded-lg"><svg></svg>{api key value}<button class="ml-auto">{copy icon}</button></div>`
- Screen-reader users encounter "button" with no name. They can't tell what it does.

**Why it matters:**
1. API key copy is the **only** way for a user to grab their key once generated — this button is the entire UX of the API key feature for assistive-tech users.
2. axe rates this critical; matches Nielsen 3 (catastrophic — feature unreachable by keyboard/AT).

**Citations:**
- `frontend/web/src/views/Profile.vue — found via grep "ml-auto p-1.5 rounded"` (anchor: clipboard-copy icon block)

**Proposed fix:** `aria-label="Скопировать API-ключ"` (RU) + i18n key `profile.apiKey.copy`. ~1 LOC.

---

### [UA-066] Import-help description text fails contrast at 3.77:1 — Severity 2 (Major) — a11y
**View:** Profile → Настройки tab → "Импорт списка" section
**Heuristic:** WCAG 1.4.3
**Evidence:**
- axe `color-contrast` 6 nodes on `.mt-2.text-xs.text-white/40`. Sample: "Импортирует ваш список аниме с MyAnimeList. Ваш профиль долж…" at 3.77:1 (fg `#78787c` on bg `#0f0f14`).
- The text is **important contextual help** (explains the requirement for public-profile setting on MAL/Shikimori) — not decorative.

**Citations:**
- `frontend/web/src/views/Profile.vue — found via grep "text-white/40 text-xs mt-2"`

**Proposed fix:** Bump to `text-white/60` (4.7:1 ish) or `text-white/70` for help text. Same `text-white/40` pattern shows up in Anime (UA-052) — consider a global Tailwind utility audit.

---

### [UA-067] Profile-import inputs lack visible URL-acceptance hint despite backend accepting URLs — Severity 1 (Minor) — discoverability
**View:** Profile → Настройки → Импорт списка
**Heuristic:** Nielsen #6 Recognition + #1 System status
**Evidence:**
- Commit `8d16aaa feat(profile/import): accept profile URLs and surface human-readable failures` — backend accepts profile URLs.
- Placeholders read "Введите имя пользователя MAL" / "Введите никнейм Shikimori" — they only mention username, no URL.
- No visible hint text mentions URL acceptance.

**Why it matters:** Users with the MAL profile URL handy will type/paste the URL but expect "username" per the placeholder. They have to discover URL acceptance by trial-and-error.

**Citations:**
- `frontend/web/src/views/Profile.vue — found via grep "profile.import.malPlaceholder"`
- Locale files: `frontend/web/src/locales/ru.json` (search `malPlaceholder`)

**Proposed fix:** Update placeholders to "Имя пользователя или URL профиля MAL" + locale equivalents. ≤ 6 LOC across 3 locale files.

---

### [UA-068] Profile `<title>` doesn't include username — Severity 1 (Minor) — i18n / SEO
**View:** Profile desktop
**Heuristic:** Nielsen #1 + WCAG 2.4.2 Page Titled
**Evidence:** `<title>Профиль - AnimeEnigma</title>` — generic, doesn't include "ui_audit_bot". Browser tab / history entry doesn't disambiguate between users.
**Citations:** `frontend/web/src/router/index.ts — found via grep "Профиль"` + title-management composable
**Proposed fix:** Set `<title>{username} - AnimeEnigma</title>` via the dynamic-title composable (same fix pattern as UA-051 will likely apply here).

---

### [UA-069] Profile tabs have no `aria-controls` linking tab to panel — Severity 1 (Minor) — a11y
**View:** Profile desktop
**Heuristic:** WAI-ARIA Tabs pattern
**Evidence:** "Мои списки" and "Настройки" tabs have `role="tab"` + `aria-selected` but `aria-controls: null`. The connection to the panel they switch isn't programmatically defined.
**Proposed fix:** Add `aria-controls` on each tab pointing to the panel id; add matching `id` to panel + `role="tabpanel"`.

---

### Carry-over verification

| Prior ID | Status | Evidence |
|---|---|---|
| **UA-018** Profile pills aria-pressed | ✓ Holds | 6 status pills with aria-pressed (1 true, 5 false) |
| **UA-037** Profile opacity nodes contrast | ✓ Partial | 12 opacity-* nodes still present but axe doesn't flag them; replaced by `text-white/40` contrast issues (UA-066) |
| **UA-038** Avatar FAB aria-label | ✓ Holds | `aria-label="Загрузить аватар"` |
| **UA-040** localized title | ✓ Holds (RU) | "Профиль - AnimeEnigma" present (but missing username — see UA-068) |
| **UA-041** Profile heading-order h1→h2 | ✓ Holds | h1 "ui_audit_bot" present |

## Mobile findings (500×723) — pending in mobile sweep
