# Anime detail — `/anime/:id` — per-view findings

**View:** `https://animeenigma.ru/anime/8dd0c714-…` (Ван-Пис / One Piece — picked because it's the pinned rec)
**Account:** `ui_audit_bot` logged in
**Viewports:** desktop 1280×800 ✓ probed
**axe-core:** 4.10.2 — Viol 1 (4 nodes — `color-contrast` on episode-list `text-white/40`)

## Desktop findings (1280×800)

### [UA-062] RU/EN language switch has no group semantics or pressed state — Severity 2 (Major) — a11y (NEW Phase 16 surface)
**View:** Anime detail
**Heuristic:** WCAG 4.1.2 Name/Role/Value
**Evidence:**
- The RU/EN toggle renders as two adjacent `<button>` siblings inside a `<div>` with NO `role="group"`, NO `role="tablist"`, NO `aria-label`.
- Buttons themselves have `aria-pressed: null`, `aria-selected: null`, `role: null`.
- The "selected" state is communicated only by visual CSS classes (`bg-cyan-500/20 text-cyan` vs `text-white/50`).
- Screen-reader users cannot perceive the group, the toggle relationship, or which language is active.

**Why it matters:**
1. Phase 16 brand-new surface — flagging now before it ossifies.
2. Critical control: switching language switches which player loads, which subtitles, which translations.
3. Adjacent provider chips (Kodik, etc.) have the same problem — same fix can land both.

**Citations:**
- `frontend/web/src/views/Anime.vue — found via grep "switchLanguage"` / `grep "videoProvider === 'english'"` (per anchor map)

**Proposed fix:**
- Wrap with `<div role="group" aria-label="Язык озвучки">`.
- Add `aria-pressed="true"|"false"` to each button (toggle-button pattern), OR convert to `role="tab"` + `role="tablist"` + `aria-selected` if the panels swap below.

---

### [UA-063] Video provider chips (Kodik, AnimeLib, etc) — same group/pressed gap — Severity 2 (Major) — a11y (carries from earlier audits but never numbered)
**View:** Anime detail
**Heuristic:** WCAG 4.1.2
**Evidence:** "Kodik" chip uses `bg-cyan-500/20 text-cyan` for visual-selected; no `aria-pressed`, no parent `role="group"`.

**Why it matters:** Same as UA-062 — provider switching is a critical control with no programmatic state.

**Proposed fix:** Same pattern.

---

### Carry-over verification

| Prior ID | Status | Evidence |
|---|---|---|
| **UA-013** Watch CTA above fold | ✓ Holds | "Смотреть" at y≈381, fold at y=800 |
| **UA-015** Status menu aria-haspopup | ✓ Holds | `aria-haspopup="menu"`, `aria-controls="watchlist-status-menu"`, `aria-expanded="false"` |
| **UA-016** Rating 10 stars unlabeled | ✓ Holds | `role="radiogroup"` `aria-label="Ваша оценка"` + 10 `role="radio"` with per-star labels ("Оценить на 1 из 10" etc.) |
| **UA-050** Anime error state localization | ✓ Holds (no error path observed; "Failed to fetch anime" not present in DOM on happy path) |
| **UA-051** Anime detail dynamic `<title>` | ✗ **Open** | `<title>Детали аниме - AnimeEnigma</title>` — still generic; should include the anime name. h1 is correct ("Ван-Пис") so the data exists. |
| **UA-052** `text-white/40` sweep | ✗ **Open** | 91 elements with `text-white/40`; axe `color-contrast` flags 4 nodes at 3.83:1 (e.g. episode-tile label "Shikimori" / "AnimeEnigma (2)" / "Общий персонаж") |

### [UA-064] Resume banner not observable on a fresh anime — Severity 0 (Info) — data-dependent
**View:** Anime detail
**Evidence:** Resume banner is gated by user watch_history for THIS anime; the seeded watch_history doesn't include One Piece. Verified the four-state machine code path exists in anchors (`anime.resume.watching/finished/notYetAired/currentlyAiring`) but couldn't probe rendering on this anime.
**Action:** Re-test on `/anime/<id>` for the 3 seeded watch_history anime IDs. Flagging as a probe gap, not a finding.

## Mobile findings (500×723) — pending in mobile sweep
