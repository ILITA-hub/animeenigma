# Auth — `/auth` — per-view findings

**View:** `https://animeenigma.ru/auth`
**Account:** Anonymous (logged out)
**Viewports:** desktop 1280×800 ✓ probed
**axe-core:** 4.10.2 — Pass clean, **0 violations** beyond extension noise

## Desktop findings (1280×800)

### [UA-070] Auth page has no `<h1>` — Severity 2 (Major) — a11y / SEO
**View:** Auth desktop (and likely mobile)
**Heuristic:** WCAG 2.4.6 + 1.3.1
**Evidence:**
- Page contains only `<h2>Войти через Telegram</h2>`. No `<h1>` (sr-only or otherwise).
- Home and Profile both correctly carry an h1 (Home is sr-only since UA-006 fix; Profile uses username); Auth is the regression.

**Why it matters:**
- Screen-reader users navigating by heading land on h2 with no orienting top-level.
- SEO: `/auth` is a legitimate landing page (we drive users to it from external Telegram OAuth flows).

**Citations:**
- `frontend/web/src/views/Auth.vue — found via grep "auth.telegramLogin"`

**Proposed fix:** Add `<h1 class="sr-only">{$t('auth.title')}</h1>` (e.g. "Вход в AnimeEnigma" / "Sign in to AnimeEnigma").

---

### [UA-071] QR canvas has no accessible name or alternative — Severity 2 (Major) — a11y
**View:** Auth desktop (QR section)
**Heuristic:** WCAG 1.1.1 Non-text Content
**Evidence:**
- `<canvas width="200" height="200">` (the rendered QR), no `aria-label`, no `role`, no adjacent visually-hidden description.
- AT users get "canvas" with no context — they can't know it's a QR they should scan with their phone.
- Alternative login path (the tg:// link / t.me link) is below, but screen readers should still announce what the canvas is.

**Citations:** `frontend/web/src/views/Auth.vue — found via grep "QRCode.toCanvas" / "qrCanvas"`

**Proposed fix:** `<canvas role="img" aria-label="{$t('auth.qrAlt')}">` where `auth.qrAlt = "QR-код для входа через Telegram-приложение"` (RU). For users who can't scan, mention that the buttons below serve the same purpose.

---

### [UA-072] "Используете Telegram Web?" summary uses `text-white/40` (suspected contrast fail) — Severity 1 (Minor) — a11y
**View:** Auth desktop
**Heuristic:** WCAG 1.4.3
**Evidence:**
- `<summary class="cursor-pointer text-white/40 hover:text-white/70 transition-colors select-none text-center">Используете Telegram Web?</summary>`
- Same `text-white/40` pattern that fails contrast elsewhere (UA-052, UA-066). axe didn't flag this specific node (possibly because it's centered on solid bg with luminance close-but-passing) — verify with explicit ratio probe.

**Citations:** `frontend/web/src/views/Auth.vue — found via grep "Используете Telegram Web"`

**Proposed fix:** Bump to `text-white/60` to be safe. Global `text-white/40 → /60` sweep would clean up UA-052, UA-066, UA-072 in one shot.

---

### [UA-073] Locale switcher button label is just "ru" — Severity 1 (Minor) — a11y / clarity
**View:** Auth (and all views — global navbar)
**Heuristic:** WCAG 2.4.4 Link Purpose / Nielsen #6
**Evidence:**
- Navbar has `<button>ru</button>` (or `<button>en</button>`) with no `aria-label`. Screen readers announce just "ru, button" with no context that this is a language switcher.

**Citations:** `frontend/web/src/components/layout/Navbar.vue — found via grep "i18n" / "locale"`

**Proposed fix:** `aria-label="Сменить язык"` + visible text continues to show current locale code.

---

### Holds (positive verification)

| Item | Status | Evidence |
|---|---|---|
| Vue I18n `@` escaping (commit f7a6aad) | ✓ Works | Page renders "@animeenigma_auth_bot" as plain text in two places without Vue I18n message-syntax errors. |
| tg:// deeplink | ✓ Present | `<a href="tg://resolve?domain=animeenigma_auth_bot">Открыть в Telegram</a>` |
| t.me fallback for non-app users | ✓ Present | `<a href="https://t.me/animeenigma_auth_bot?start={token}">Нет приложения? Открыть в браузере</a>` |
| tg-web fallback details (manual /start) | ✓ Present, collapsed by default | Summary "Используете Telegram Web?" → contains `/start {token}` text + "Копировать" button |
| Token expiry indicator | ✓ Present | "Истекает через 252 сек" (Nielsen #1 System status) |
| Copy-to-clipboard button | ✓ Has visible text "Копировать" (unlike UA-065 on Profile) |

### Scenario E5 (Auth QR + deeplink + fallback) — observations

- **Happy path (desktop):** QR + tg:// button + t.me-web fallback all render. Expiry countdown visible.
- **The 3 paths each work** for their respective user contexts. Good UX foundation; just needs the a11y polish above.
- **Driven probe limit:** Couldn't drive the actual Telegram-side authentication completion (would need a real Telegram account or test-mode bot) — verified the FRONTEND surface only, not the round-trip auth.

## Mobile findings (500×723) — pending in mobile sweep
