# Spec: `/admin/feedback` → "Project Board" notebook reframe

**Date:** 2026-06-24
**Status:** Design — approved in brainstorming, pending spec review
**Route (unchanged):** `/admin/feedback`
**Primary surfaces:** `services/player/` (backend), `frontend/web/src/views/admin/AdminFeedback.vue` (+ new dialog/types/i18n)

---

## 1. Problem

The `/admin/feedback` page is titled **"User Feedback"**, but it has organically become the project's single backlog/notebook. It already physically holds:

- **User feedback** — submitted via the in-site feedback button (`POST /api/users/report`).
- **Telegram-mirrored items** — every human Telegram message, mirrored by the maintenance bot (`POST /internal/feedback`).
- **AI/owner-authored TODOs & ledgers** — `owner-todo`, `repo-todo`, and the `notebook` ledger, written as JSON directly to the `docker_player_reports` volume by agents and `bin/feedback*` tooling.

All of these live as JSON files on one volume, triaged through a 5-state status workflow (`new · in_progress · ai_done · resolved · not_relevant`). But the page's **identity, taxonomy, and capture affordances still say "user feedback only"**:

- No first-class way to distinguish **user feedback** vs **project TODO** vs **idea**.
- No **channel/source** lens (where an item entered the system).
- No in-UI way to **jot a new note** — you write JSON files by hand or route through the bot.
- **Dismissed** (`not_relevant`) items clutter the default view.

## 2. Goal

Reframe the page as **"Project Board"** — the single place for feedback, TODOs, and ideas — by adding first-class **kind** and **source** lenses, **in-UI quick capture**, and **hiding dismissed items by default**. The change is **additive and backward-compatible**: the status workflow, kanban columns, detail modal, attachments, status history, and notification loop are **unchanged**.

## 3. Locked decisions (from brainstorming)

| Decision | Choice |
|---|---|
| Scope | Reframe **+ quick-capture** |
| Taxonomy | **kind above category** (kind = nature; category = sub-type) |
| Source values | `feedback_form · telegram · api · manual`; **`api` = all programmatic** |
| Page name | **"Project Board"** |
| Kind lens UI | **Filter dropdown** (not tabs) |
| Capture form fields | **Kind + Category + Description** (no Title field) |
| Dismissed items | **Hidden by default**, one click to reveal |
| Legacy data | **Read-time derivation**, no file migration |
| Filtering locus | **Server-side** (matches existing `category`/`status`/pagination) |

## 4. Data model (additive)

Two new fields on the report item (`services/player/internal/domain/report.go` `ErrorReport`) and on the list row (`reportMeta` in `admin_reports.go`):

| Field | JSON | Type / enum | Notes |
|---|---|---|---|
| **kind** *(new)* | `kind` | `feedback \| todo \| idea` | the item's nature |
| **source** *(new, normalized)* | `source` | `feedback_form \| telegram \| api \| manual` | normalized channel |
| category *(kept)* | `category` | `bug \| issue \| feature` (optional) | sub-type, mainly for feedback |

- The existing loose `source` free-text (e.g. `telegram`, `owner-todo`) is **normalized** into the enum above. New items store the normalized value directly; the raw provenance, if needed, is preserved in `telegram_meta`/notes — not required for this feature.
- `category` validation stays as-is (already non-strict). `kind`/`source` are validated on the **new** write paths; on read, unknown/empty values fall back through derivation.

## 5. Classification rules

### 5.1 Writers stamp explicitly (going forward)

| Writer | `source` | `kind` |
|---|---|---|
| Site feedback button — `POST /api/users/report` | `feedback_form` | `feedback` |
| Telegram bot — `POST /internal/feedback` | `telegram` | `feedback` (request may override) |
| Agents / `bin/feedback*` / scripts (programmatic) | `api` | `todo` (written JSON may override to `idea`/`feedback`) |
| **+ New note** — `POST /api/admin/reports` *(new)* | `manual` | from form (default `todo`) |

`internalFeedbackCreateRequest` gains an optional `kind` field so agents can post TODO/idea explicitly.

### 5.2 Legacy items — read-time derivation (no migration)

Applied in the admin **read path** (`List` + `Get` → `reportMeta`) when explicit `kind`/`source` are absent. Pure and idempotent:

```
source:
  raw source == "telegram" OR player_type == "telegram"   → telegram
  else raw source present and programmatic
       (owner-todo, repo-todo, agent, …)                  → api
  else                                                     → feedback_form

kind:
  source ∈ {feedback_form, telegram}                       → feedback
  source ∈ {api, manual}                                   → todo
  (no legacy "idea" items; ideas are created going forward)
```

If explicit `kind`/`source` exist on the JSON (new items), they win — derivation is a fallback only.

> **Optional, deferred:** a one-time backfill script to bake derived fields into legacy JSON files. Not required; read-time derivation covers display and filtering.

## 6. Backend changes (`services/player/`)

1. **`domain/report.go`** — add `Kind`, `Source` string fields with JSON tags.
2. **`handler/report.go`** (`SubmitReport` / `saveReportToDisk`) — set `kind=feedback`, `source=feedback_form` before persisting.
3. **`handler/internal_feedback.go`** (`CreateInternal`) — set `source=telegram` (or normalized from request `source`), `kind=feedback` (or request-provided `kind`); add optional `kind` to the request struct.
4. **NEW `POST /api/admin/reports`** (admin-JWT). Routing: the gateway already sends `/api/admin/reports*` to the **player** service via a rule registered *before* the generic `/api/admin/*` → catalog rule (`router_test.go:401-426`), and `GET /api/admin/reports` is already the list. Adding a **POST** to the same collection path needs **no new gateway rule** — just a new POST handler in the player admin router. Body: `{ kind, category?, description }`. Validates `kind ∈ {feedback,todo,idea}`, `category ∈ {bug,issue,feature}` or empty, `description` non-empty. Writes `{ts}_{adminUsername}_manual.json` with `source=manual`, supplied `kind`/`category`, `username`/`user_id` of the admin, `timestamp`, empty diagnostics (`console_logs`/`network_logs="[]"`, `page_html=""`), status defaults `new`. Returns the created `id`.
5. **`handler/admin_reports.go`**:
   - `reportMeta` gains `kind`, `source` (explicit-or-derived).
   - `List` learns three **server-side** filters alongside the existing `category`/`status`:
     - `kind` — exact match.
     - `source` — exact match.
     - `status=active` sentinel — return **all statuses except `not_relevant`** (the default). Existing explicit statuses and `all` keep working.
   - `Get` includes `kind`/`source` (derived if absent) so deep-linked detail shows them.
   - **Deep-link bypass:** `Get` by `id` must keep working for a `not_relevant` item even though the list hides it by default (detail fetch is id-based, independent of the list filter).

## 7. Frontend changes (`frontend/web/`)

### 7.1 `AdminFeedback.vue`
- **Identity:** `title` → "Project Board"; `subtitle` reframed (feedback + TODOs + ideas). Update the top-of-file HTML comment.
- **Filter grid:** add **Kind** dropdown + **Source** dropdown (grid grows 6→8 controls, wraps to two rows). Use the `Select` primitive (consistent height — see the 2026-06-24 DatePicker fix).
- **Status filter:** default value **`active`** (= exclude `not_relevant`). Options: `Active` (default) · `All` · `new` · `in_progress` · `ai_done` · `resolved` · `not_relevant`. Selecting `All` or `Not relevant` reveals dismissed items. Sent as the `status` query param.
- **Kanban:** while the status filter is `active`, **hide the `not_relevant` column**; `All`/`Not relevant` brings it back. (Kanban currently forces `status=all` on entry — change to honor `active` as the default.)
- **Rows & cards:** render a **Kind badge** + **Source badge** (within existing cells — **no new table columns**, to avoid width blow-up). Reuse the `Badge` primitive with variants; brand/provider hues are DS-exempt.
- **Header:** **+ New note** button beside Refresh → opens `NewNoteDialog`.

### 7.2 New `NewNoteDialog.vue`
- Built from `Dialog` + `Select` + `Input`/textarea primitives (no bare native controls).
- Fields: **Kind** (`Select`, default TODO) · **Category** (`Select`, optional) · **Description** (required textarea).
- Submit → `POST /api/admin/reports` → on success, refresh the list (and optionally deep-open the created item). Inline validation: description required.

### 7.3 `types/feedback.ts`
- Add `kind` (`'feedback'|'todo'|'idea'`) and `source` (`'feedback_form'|'telegram'|'api'|'manual'`) to the report row type.
- Add a `NewNotePayload` type.

### 7.4 Filter state / fetch
- The admin-feedback filter state (composable) gains `filterKind`, `filterSource`; the List query builder sends `kind`, `source`, and the `active` default `status`. Keep filtering **server-side** to preserve pagination correctness.

## 8. i18n (en + ru + ja, parity-gated)

New keys under `admin.feedback.*`:
- `title` = "Project Board"; `subtitle` (reframed copy).
- `kind.{feedback,todo,idea}`; `source.{feedback_form,telegram,api,manual}`.
- `filters.kind`, `filters.source`, `filters.allKinds`, `filters.allSources`.
- `status.active` (new option label; keep existing status labels).
- `newNote.{button,title,kindLabel,categoryLabel,descriptionLabel,descriptionPlaceholder,submit,cancel,success,error}`.

All three locales updated together (parity specs + i18n-lint gate the build).

## 9. Testing

- **Backend:** unit tests for the derivation matrix (legacy `source`/`player_type` → `kind`/`source`); the new `POST /api/admin/reports` endpoint (valid create, invalid `kind`, empty `description`, admin-auth required); `report.go`/`internal_feedback.go` stamping the new fields; `List` honoring `kind`/`source`/`status=active`; `Get` deep-link to a `not_relevant` item still resolves.
- **Frontend:** `AdminFeedback.spec` — kind/source filters narrow rows; `active` default hides `not_relevant`; kanban hides the `not_relevant` column by default; kind/source badges render. `NewNoteDialog.spec` — fields, required-description validation, submit payload shape. Locale parity spec for the new keys.
- **Pre-flight:** `/frontend-verify` (DS-lint + i18n parity + real `bun run build`) before ship.

## 10. Out of scope / YAGNI

- Tags/labels, priority, grouping/sections (the "full redesign" option) — **deferred**.
- A separate **Title** field on notes — deferred (first line of description acts as the heading).
- One-time legacy **backfill** script — optional/deferred.
- Any change to the **status workflow**, kanban columns, detail modal, attachments, status history, or the notification loop.

## 11. Risks & mitigations

| Risk | Mitigation |
|---|---|
| `not_relevant` default-hide breaks deep-links to a dismissed item | Detail `Get` is id-based and bypasses the list filter — covered by a test. |
| Pagination skew if filters were client-side | Filtering is **server-side** (verified); add `kind`/`source`/`active` as server query params. |
| DS-lint failures on new dialog/badges | Build from `ui` primitives + semantic tokens; run `/frontend-verify`. |
| i18n parity gate | Add all keys to en/ru/ja together. |
| Normalizing the existing loose `source` strings | Derivation table is explicit + tested; raw value preserved where it matters. |

## 12. Impact metrics

- **UXΔ = +3 (Better)** — the page finally *is* what it's used as; self-serve capture removes the JSON/bot detour; dismissed clutter gone by default.
- **CDI = 0.03 * 13** — additive fields + one new endpoint + a UI lens & dialog; status/kanban/detail untouched (low spread × low shift, moderate effort).
- **MVQ = Griffin 88%/85%** — a familiar triage board re-pointed to its true purpose; coherent, low slop.
