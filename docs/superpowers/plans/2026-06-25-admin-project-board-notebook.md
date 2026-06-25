# Admin "Project Board" Notebook Reframe — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Reframe `/admin/feedback` into the project "Project Board" by adding additive `kind` (feedback/todo/idea) + normalized `source` (feedback_form/telegram/api/manual) lenses, in-UI quick-capture (`POST /api/admin/reports`), and hiding `not_relevant` by default — without touching the status workflow, kanban columns, detail modal, attachments, or notifications.

**Architecture:** Backend = player service (Go, chi/v5) gains two derived/stamped fields on report rows + one new admin POST handler; legacy items are classified by pure read-time derivation (no file migration). Frontend = the existing `AdminFeedback.vue` + `useAdminFeedback` composable gain two filter dropdowns, an "Active" default status, kind/source badges, and a `NewNoteDialog`. Filtering stays **server-side** to keep pagination correct.

**Tech Stack:** Go (chi/v5, `httptest`, `t.TempDir`), Vue 3 `<script setup>` + TypeScript, Vitest + `@vue/test-utils`, vue-i18n (en/ru/ja), `bun`/`bunx`.

**Reference spec:** `docs/superpowers/specs/2026-06-24-admin-project-board-notebook-reframe-design.md`

## Global Constraints

- **No time-effort units** (days/hours/sprints). Plans/changelog scored `UXΔ` / `CDI` / `MVQ` (CLAUDE.md).
- **DS-lint is build-enforced.** Bind to semantic tokens; never hardcode colors. Brand/provider hues (`cyan pink orange rose indigo teal lime`) are EXEMPT. Reuse `@/components/ui` primitives. Only `font-medium`/`font-semibold`. Run `/frontend-verify` before ship.
- **i18n parity:** every new key must exist in `en.json`, `ru.json`, AND `ja.json` with matching ICU placeholders, or the parity specs fail the build.
- **Type truth = real build.** `vue-tsc --noEmit` can false-pass from cache; trust only `bun run build`. Import UI types from the `@/components/ui` barrel.
- **Frontend tooling:** `bun` / `bunx` (never npm/npx).
- **Backend:** `go test ./...` from the service dir. JSON responses use the `{success,data}` envelope via `httputil.OK`.
- **Filtering is server-side** (List handler reads query params + paginates); `kind`/`source`/`status=active` are added as server params.
- **Unchanged:** status enum + workflow, kanban status columns, detail modal, attachments, status history, notification loop.
- **Worktree workflow.** All work in a git worktree off fresh `origin/main`; never edit the base tree. Commit co-authors on every commit:
  ```
  Co-Authored-By: Claude Code <noreply@anthropic.com>
  Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
  Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
  ```
- **Canonical values:** kind ∈ `{feedback, todo, idea}`; source ∈ `{feedback_form, telegram, api, manual}`; category ∈ `{bug, issue, feature}` or empty; status ∈ `{new, in_progress, ai_done, resolved, not_relevant}` + the `active` filter sentinel.

---

## File Structure

**Backend (`services/player/`):**
- `internal/domain/report.go` — add `Kind`, `Source` to `ErrorReport`.
- `internal/handler/classify.go` *(new)* — `normalizeSource` + `deriveKind` pure helpers.
- `internal/handler/classify_test.go` *(new)* — derivation matrix tests.
- `internal/handler/admin_reports.go` — `reportMeta.Kind`; `List` derive+filter; `Get` inject; new `CreateNote`.
- `internal/handler/admin_reports_test.go` — List filter + CreateNote tests.
- `internal/handler/report.go` — stamp `kind=feedback`, `source=feedback_form`.
- `internal/handler/internal_feedback.go` — accept optional `kind`; stamp normalized `source`, `kind`.
- `internal/transport/router.go` — register `POST /admin/reports`.

**Frontend (`frontend/web/`):**
- `src/types/feedback.ts` — `FeedbackKind`, `FeedbackSource`, `kind` field.
- `src/api/client.ts` — `listReports` gains `kind`/`source`; new `createNote`.
- `src/composables/useAdminFeedback.ts` — `filterKind`/`filterSource` refs, `active` default, params wiring.
- `src/composables/__tests__/useAdminFeedback.spec.ts` — new-filter assertions.
- `src/components/admin/NewNoteDialog.vue` *(new)* + `__tests__/NewNoteDialog.spec.ts` *(new)*.
- `src/views/admin/AdminFeedback.vue` — title/subtitle, filters, status `active`, badges, kanban hide, + New note button.
- `src/locales/{en,ru,ja}.json` — new `admin.feedback.*` keys.

---

## Task 1: Backend — `kind`/`source` fields + derivation helpers

**Files:**
- Modify: `services/player/internal/domain/report.go` (struct `ErrorReport`)
- Modify: `services/player/internal/handler/admin_reports.go` (struct `reportMeta`)
- Create: `services/player/internal/handler/classify.go`
- Test: `services/player/internal/handler/classify_test.go`

**Interfaces:**
- Produces: `normalizeSource(rawSource, playerType string) string` → one of `feedback_form|telegram|api|manual`. `deriveKind(rawKind, normalizedSource string) string` → one of `feedback|todo|idea`. Used by Tasks 2, 3.
- Produces: `ErrorReport.Kind`, `ErrorReport.Source`, `reportMeta.Kind` (`reportMeta.Source` already exists).

- [ ] **Step 1: Write the failing test** — create `services/player/internal/handler/classify_test.go`:

```go
package handler

import "testing"

func TestNormalizeSource(t *testing.T) {
	cases := []struct {
		raw, playerType, want string
	}{
		{"feedback_form", "feedback", "feedback_form"}, // canonical passes through
		{"manual", "feedback", "manual"},
		{"api", "feedback", "api"},
		{"telegram", "telegram", "telegram"},
		{"", "telegram", "telegram"},      // legacy telegram entry, no source field
		{"", "feedback", "feedback_form"}, // legacy user report, no source field
		{"owner-todo", "feedback", "api"}, // legacy AI/owner ledger
		{"repo-todo", "feedback", "api"},
	}
	for _, c := range cases {
		if got := normalizeSource(c.raw, c.playerType); got != c.want {
			t.Errorf("normalizeSource(%q,%q) = %q, want %q", c.raw, c.playerType, got, c.want)
		}
	}
}

func TestDeriveKind(t *testing.T) {
	cases := []struct {
		rawKind, source, want string
	}{
		{"idea", "manual", "idea"},        // explicit wins
		{"todo", "api", "todo"},           // explicit wins
		{"", "feedback_form", "feedback"}, // user channel → feedback
		{"", "telegram", "feedback"},
		{"", "api", "todo"}, // internal channel → todo
		{"", "manual", "todo"},
		{"bogus", "feedback_form", "feedback"}, // invalid kind ignored
	}
	for _, c := range cases {
		if got := deriveKind(c.rawKind, c.source); got != c.want {
			t.Errorf("deriveKind(%q,%q) = %q, want %q", c.rawKind, c.source, got, c.want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/player && go test ./internal/handler/ -run 'TestNormalizeSource|TestDeriveKind' -v`
Expected: FAIL — `undefined: normalizeSource` / `undefined: deriveKind`.

- [ ] **Step 3: Create the helpers** — create `services/player/internal/handler/classify.go`:

```go
package handler

// normalizeSource maps the historically free-text `source` field (and
// player_type) onto the four canonical Project-Board channels:
//
//	feedback_form — submitted via the in-site feedback button
//	telegram      — mirrored from Telegram by the maintenance bot
//	api           — created programmatically (agents, scripts, bin/feedback*,
//	                legacy owner-todo / repo-todo ledgers)
//	manual        — typed into the admin board via "+ New note"
//
// New code paths write a canonical value, which passes through unchanged;
// legacy items are derived from their raw signals.
func normalizeSource(rawSource, playerType string) string {
	switch rawSource {
	case "feedback_form", "telegram", "api", "manual":
		return rawSource
	}
	if playerType == "telegram" {
		return "telegram"
	}
	if rawSource == "" {
		return "feedback_form"
	}
	// Any other non-empty legacy source (owner-todo, repo-todo, …) was a
	// programmatic write.
	return "api"
}

// deriveKind returns the explicit kind when present and valid, otherwise infers
// it from the (already-normalized) source: user channels → feedback, internal
// channels → todo. There are no legacy "idea" items; ideas are only created
// going forward via "+ New note".
func deriveKind(rawKind, normalizedSource string) string {
	switch rawKind {
	case "feedback", "todo", "idea":
		return rawKind
	}
	switch normalizedSource {
	case "feedback_form", "telegram":
		return "feedback"
	default: // api, manual
		return "todo"
	}
}
```

- [ ] **Step 4: Add the struct fields.** In `services/player/internal/domain/report.go`, inside `ErrorReport`, after the `Category` line (`Category string \`json:"category,omitempty"\``) add:

```go
	Kind   string `json:"kind,omitempty"`   // feedback | todo | idea
	Source string `json:"source,omitempty"` // feedback_form | telegram | api | manual
```

In `services/player/internal/handler/admin_reports.go`, inside `reportMeta`, add a `Kind` field next to the existing `Source` field so the list row carries it:

```go
	Kind          string   `json:"kind,omitempty"`
	Source        string   `json:"source,omitempty"`
```

(The `Source` field already exists — add only the `Kind` line above it.)

- [ ] **Step 5: Run test to verify it passes**

Run: `cd services/player && go test ./internal/handler/ -run 'TestNormalizeSource|TestDeriveKind' -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add services/player/internal/domain/report.go services/player/internal/handler/classify.go services/player/internal/handler/classify_test.go services/player/internal/handler/admin_reports.go
git commit -m "feat(player): add kind/source fields + classification helpers

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Task 2: Backend — `List` derives + filters by kind/source/active

**Files:**
- Modify: `services/player/internal/handler/admin_reports.go` (`List`)
- Test: `services/player/internal/handler/admin_reports_test.go`

**Interfaces:**
- Consumes: `normalizeSource`, `deriveKind` (Task 1).
- Produces: `GET /admin/reports` now accepts `kind`, `source`, and `status=active` query params; each `reportMeta` row carries normalized `kind`/`source`.

- [ ] **Step 1: Write the failing test** — append to `services/player/internal/handler/admin_reports_test.go`:

```go
func TestAdminReports_List_KindSourceActiveFilters(t *testing.T) {
	h, dir := newTestReportsHandler(t)
	// legacy user feedback (no kind/source) → feedback / feedback_form
	idFb := writeReport(t, dir, "2026-06-01T10-00-00", "alice", "feedback", map[string]interface{}{"category": "bug", "description": "user bug"})
	// legacy telegram → feedback / telegram
	writeReport(t, dir, "2026-06-02T10-00-00", "bot", "telegram", map[string]interface{}{"description": "tg msg", "source": "telegram"})
	// legacy AI ledger → todo / api
	writeReport(t, dir, "2026-06-03T10-00-00", "claude", "feedback", map[string]interface{}{"description": "agent todo", "source": "owner-todo"})
	// explicit manual idea
	writeReport(t, dir, "2026-06-04T10-00-00", "neymik", "feedback", map[string]interface{}{"description": "an idea", "kind": "idea", "source": "manual"})

	// mark the user-feedback item not_relevant in the sidecar
	_ = os.WriteFile(filepath.Join(dir, statusFileName),
		[]byte(`{"`+idFb+`":{"status":"not_relevant","updated_at":"x","updated_by":"y"}}`), 0600)

	get := func(qs string) listResp {
		r := httptest.NewRequest(http.MethodGet, "/api/admin/reports?"+qs, nil)
		w := httptest.NewRecorder()
		h.List(w, r)
		var resp listResp
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		return resp
	}

	// kind=todo → only the api ledger item
	if r := get("kind=todo"); r.Data.Total != 1 || r.Data.Items[0].Username != "claude" {
		t.Errorf("kind=todo: total=%d items=%v", r.Data.Total, r.Data.Items)
	}
	// source=telegram → only the tg item, derived kind=feedback
	if r := get("source=telegram"); r.Data.Total != 1 || r.Data.Items[0].Kind != "feedback" {
		t.Errorf("source=telegram: total=%d items=%v", r.Data.Total, r.Data.Items)
	}
	// kind=idea → explicit manual idea
	if r := get("kind=idea"); r.Data.Total != 1 || r.Data.Items[0].Source != "manual" {
		t.Errorf("kind=idea: total=%d items=%v", r.Data.Total, r.Data.Items)
	}
	// status=active → excludes the not_relevant user-feedback item (3 of 4)
	if r := get("status=active"); r.Data.Total != 3 {
		t.Errorf("status=active: total=%d, want 3", r.Data.Total)
	}
	// derived source on the legacy user item is feedback_form (when shown)
	if r := get("source=feedback_form"); r.Data.Total != 1 || r.Data.Items[0].Username != "alice" {
		t.Errorf("source=feedback_form: total=%d items=%v", r.Data.Total, r.Data.Items)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/player && go test ./internal/handler/ -run TestAdminReports_List_KindSourceActiveFilters -v`
Expected: FAIL (kind/source not derived; `active` treated as a literal status → 0 rows).

- [ ] **Step 3: Wire derivation + filters into `List`.** In `services/player/internal/handler/admin_reports.go` `List`, read the two new params alongside the existing ones (just after `fType := q.Get("type")`):

```go
	fKind := q.Get("kind")
	fSource := q.Get("source")
```

Then, inside the per-file loop, AFTER the block that sets `m.Status` (the `m.Status = "new"` / sidecar override) and BEFORE the existing `if fCategory != "" ...` filters, normalize and derive:

```go
		m.Source = normalizeSource(m.Source, m.PlayerType)
		m.Kind = deriveKind(m.Kind, m.Source)
```

Replace the existing status filter line:

```go
		if fStatus != "" && m.Status != fStatus {
			continue
		}
```

with the `active`-aware version:

```go
		switch {
		case fStatus == "active":
			if m.Status == "not_relevant" {
				continue
			}
		case fStatus != "":
			if m.Status != fStatus {
				continue
			}
		}
```

And add the kind/source filters next to the category/type filters (e.g. right after the `fType` filter):

```go
		if fKind != "" && m.Kind != fKind {
			continue
		}
		if fSource != "" && m.Source != fSource {
			continue
		}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd services/player && go test ./internal/handler/ -run 'TestAdminReports_List' -v`
Expected: PASS (both the new test and the existing `TestAdminReports_List_SortFilterPaginate`).

- [ ] **Step 5: Commit**

```bash
git add services/player/internal/handler/admin_reports.go services/player/internal/handler/admin_reports_test.go
git commit -m "feat(player): List derives kind/source + filters incl. status=active

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Task 3: Backend — `Get` injects derived kind/source (deep-link safe)

**Files:**
- Modify: `services/player/internal/handler/admin_reports.go` (`Get`)
- Test: `services/player/internal/handler/admin_reports_test.go`

**Interfaces:**
- Consumes: `normalizeSource`, `deriveKind` (Task 1).
- Produces: `GET /admin/reports/{id}` response map includes normalized `kind`/`source`; a `not_relevant` item still resolves by id (list hides it, detail does not).

- [ ] **Step 1: Write the failing test** — append to `services/player/internal/handler/admin_reports_test.go`:

```go
func TestAdminReports_Get_InjectsKindSource(t *testing.T) {
	h, dir := newTestReportsHandler(t)
	id := writeReport(t, dir, "2026-06-03T10-00-00", "claude", "feedback",
		map[string]interface{}{"description": "agent todo", "source": "owner-todo"})
	// dismiss it — Get must still resolve it (deep-link bypasses the list filter)
	_ = os.WriteFile(filepath.Join(dir, statusFileName),
		[]byte(`{"`+id+`":{"status":"not_relevant","updated_at":"x","updated_by":"y"}}`), 0600)

	r := httptest.NewRequest(http.MethodGet, "/api/admin/reports/"+id, nil)
	r = withURLParam(r, "id", id)
	w := httptest.NewRecorder()
	h.Get(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (deep-link to dismissed item)", w.Code)
	}
	var resp struct {
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Data["source"] != "api" {
		t.Errorf("source = %v, want api", resp.Data["source"])
	}
	if resp.Data["kind"] != "todo" {
		t.Errorf("kind = %v, want todo", resp.Data["kind"])
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/player && go test ./internal/handler/ -run TestAdminReports_Get_InjectsKindSource -v`
Expected: FAIL — `source = owner-todo` (raw, not normalized); `kind = <nil>`.

- [ ] **Step 3: Inject into `Get`.** In `services/player/internal/handler/admin_reports.go` `Get`, after `full["status"] = st` (and before the `status_history` block / final `httputil.OK`), add:

```go
	rawSource, _ := full["source"].(string)
	pt, _ := full["player_type"].(string)
	src := normalizeSource(rawSource, pt)
	full["source"] = src
	rawKind, _ := full["kind"].(string)
	full["kind"] = deriveKind(rawKind, src)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd services/player && go test ./internal/handler/ -run TestAdminReports_Get_InjectsKindSource -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add services/player/internal/handler/admin_reports.go services/player/internal/handler/admin_reports_test.go
git commit -m "feat(player): Get injects normalized kind/source; deep-link to dismissed stays open

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Task 4: Backend — writers stamp explicit kind/source

**Files:**
- Modify: `services/player/internal/handler/report.go` (`saveReportToDisk`)
- Modify: `services/player/internal/handler/internal_feedback.go` (`internalFeedbackCreateRequest`, `CreateInternal`)
- Test: `services/player/internal/handler/internal_feedback_test.go` (or append to `admin_reports_test.go`)

**Interfaces:**
- Produces: feedback-button writes carry `kind=feedback`/`source=feedback_form`; telegram writes carry `source=telegram` and `kind` (request-overridable, default `feedback`).

- [ ] **Step 1: Stamp the site-feedback path.** In `services/player/internal/handler/report.go` `saveReportToDisk`, add two entries to the `fullReport` map (next to `"category": report.Category,`):

```go
		"kind":           "feedback",
		"source":         "feedback_form",
```

- [ ] **Step 2: Accept optional kind on the internal path.** In `services/player/internal/handler/internal_feedback.go`, add a `Kind` field to `internalFeedbackCreateRequest` (after `Category`):

```go
	Kind        string                 `json:"kind,omitempty"`
```

- [ ] **Step 3: Write the failing test** — append to `services/player/internal/handler/internal_feedback_test.go` (create it if absent, mirroring the `package handler` test style):

```go
func TestCreateInternal_StampsTelegramSourceAndKind(t *testing.T) {
	h, dir := newTestReportsHandler(t)
	body := `{"username":"u","user_id":"1","player_type":"telegram","description":"hi","source":"telegram"}`
	r := httptest.NewRequest(http.MethodPost, "/internal/feedback", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.CreateInternal(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	// read the single written file and assert stamped fields
	entries, _ := os.ReadDir(dir)
	var got map[string]interface{}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".json") && !strings.HasPrefix(e.Name(), "_") {
			data, _ := os.ReadFile(filepath.Join(dir, e.Name()))
			_ = json.Unmarshal(data, &got)
		}
	}
	if got["source"] != "telegram" {
		t.Errorf("source = %v, want telegram", got["source"])
	}
	if got["kind"] != "feedback" {
		t.Errorf("kind = %v, want feedback (default)", got["kind"])
	}
}
```

- [ ] **Step 4: Run test to verify it fails**

Run: `cd services/player && go test ./internal/handler/ -run TestCreateInternal_StampsTelegramSourceAndKind -v`
Expected: FAIL — `kind = <nil>` (entry map has no kind; source is raw, here it happens to be "telegram" already, but kind is missing).

- [ ] **Step 5: Stamp the internal path.** In `services/player/internal/handler/internal_feedback.go` `CreateInternal`, replace the `"source": req.Source,` line in the `entry` map and add a `kind`. Compute them just before the `entry :=` literal:

```go
	source := normalizeSource(req.Source, req.PlayerType)
	kind := deriveKind(req.Kind, source)
```

Then in the `entry` map, set:

```go
		"source":      source,
		"kind":        kind,
```

- [ ] **Step 6: Run test to verify it passes**

Run: `cd services/player && go test ./internal/handler/ -run TestCreateInternal_StampsTelegramSourceAndKind -v`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add services/player/internal/handler/report.go services/player/internal/handler/internal_feedback.go services/player/internal/handler/internal_feedback_test.go
git commit -m "feat(player): writers stamp explicit kind/source (feedback_form, telegram)

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Task 5: Backend — `POST /api/admin/reports` quick-capture note

**Files:**
- Modify: `services/player/internal/handler/admin_reports.go` (new `CreateNote` + request struct + validators)
- Modify: `services/player/internal/transport/router.go` (register route)
- Test: `services/player/internal/handler/admin_reports_test.go`

**Interfaces:**
- Consumes: `authz.ClaimsFromContext`, `sanitizeForFilename`, `maxInternalBodySize`, `h.mu`, `h.reportsDir` (all existing in the package).
- Produces: `POST /admin/reports` (admin-JWT) → writes `{ts}_{admin}_manual.json` with `source=manual`, supplied `kind`/`category`, status defaults `new`; returns `{id, status}`.

- [ ] **Step 1: Write the failing test** — append to `services/player/internal/handler/admin_reports_test.go`:

```go
func TestAdminReports_CreateNote(t *testing.T) {
	h, dir := newTestReportsHandler(t)
	claims := &authz.Claims{UserID: "admin1", Username: "neymik"}

	post := func(body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/api/admin/reports", strings.NewReader(body))
		r = r.WithContext(authz.ContextWithClaims(r.Context(), claims))
		w := httptest.NewRecorder()
		h.CreateNote(w, r)
		return w
	}

	// valid idea note
	w := post(`{"kind":"idea","category":"feature","description":"dark mode toggle"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("valid note status = %d, body=%s", w.Code, w.Body.String())
	}
	// it must be listable, normalized to manual/idea
	lr := httptest.NewRequest(http.MethodGet, "/api/admin/reports?kind=idea", nil)
	lw := httptest.NewRecorder()
	h.List(lw, lr)
	var resp listResp
	_ = json.Unmarshal(lw.Body.Bytes(), &resp)
	if resp.Data.Total != 1 || resp.Data.Items[0].Source != "manual" || resp.Data.Items[0].Status != "new" {
		t.Errorf("created note not listed correctly: %+v", resp.Data.Items)
	}

	// invalid kind rejected
	if w := post(`{"kind":"bogus","description":"x"}`); w.Code != http.StatusBadRequest {
		t.Errorf("invalid kind status = %d, want 400", w.Code)
	}
	// empty description rejected
	if w := post(`{"kind":"todo","description":"  "}`); w.Code != http.StatusBadRequest {
		t.Errorf("empty description status = %d, want 400", w.Code)
	}
	_ = dir
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/player && go test ./internal/handler/ -run TestAdminReports_CreateNote -v`
Expected: FAIL — `h.CreateNote undefined`.

- [ ] **Step 3: Add the handler.** In `services/player/internal/handler/admin_reports.go`, add (near `CreateInternal` / the other handlers):

```go
// noteCreateRequest is the admin "+ New note" quick-capture payload.
type noteCreateRequest struct {
	Kind        string `json:"kind"`
	Category    string `json:"category,omitempty"`
	Description string `json:"description"`
}

var validNoteKind = map[string]bool{"feedback": true, "todo": true, "idea": true}
var validNoteCategory = map[string]bool{"": true, "bug": true, "issue": true, "feature": true}

// CreateNote handles POST /api/admin/reports — the admin quick-capture
// "+ New note". It writes a manual notebook item (source=manual) using the same
// on-disk shape the rest of the board reads, so the listing needs no special
// case. Admin-JWT gated by the router.
func (h *AdminReportsHandler) CreateNote(w http.ResponseWriter, r *http.Request) {
	if h.reportsDir == "" {
		httputil.Error(w, errors.Internal("reports dir not configured"))
		return
	}
	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok || claims == nil {
		httputil.Unauthorized(w)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxInternalBodySize)

	var req noteCreateRequest
	if err := httputil.Bind(r, &req); err != nil {
		httputil.BadRequest(w, "invalid request body")
		return
	}
	if !validNoteKind[req.Kind] {
		httputil.BadRequest(w, "invalid kind")
		return
	}
	if !validNoteCategory[req.Category] {
		httputil.BadRequest(w, "invalid category")
		return
	}
	if strings.TrimSpace(req.Description) == "" {
		httputil.BadRequest(w, "description is required")
		return
	}

	username := claims.Username
	if username == "" {
		username = claims.UserID
	}
	username = sanitizeForFilename(username)

	entry := map[string]interface{}{
		"user_id":      claims.UserID,
		"username":     claims.Username,
		"player_type":  "feedback",
		"kind":         req.Kind,
		"source":       "manual",
		"category":     req.Category,
		"description":  req.Description,
		"timestamp":    time.Now().UTC().Format(time.RFC3339),
		"console_logs": json.RawMessage("[]"),
		"network_logs": json.RawMessage("[]"),
		"page_html":    "",
		"attachments":  []string{},
	}

	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		httputil.Error(w, errors.Internal("failed to marshal note"))
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	ts := time.Now().UTC().Format("2006-01-02T15-04-05")
	base := fmt.Sprintf("%s_%s_%s", ts, username, "manual")
	id := base
	for i := 2; ; i++ {
		if _, statErr := os.Stat(filepath.Join(h.reportsDir, id+".json")); os.IsNotExist(statErr) {
			break
		}
		if i > 50 {
			httputil.Error(w, errors.Internal("could not allocate note id"))
			return
		}
		id = fmt.Sprintf("%s-%d", base, i)
	}
	if err := os.WriteFile(filepath.Join(h.reportsDir, id+".json"), data, 0600); err != nil {
		h.log.Errorw("admin note write failed", "id", id, "error", err)
		httputil.Error(w, errors.Internal("failed to persist note"))
		return
	}
	h.log.Infow("admin note created", "id", id, "kind", req.Kind, "username", claims.Username)
	httputil.OK(w, map[string]string{"id": id, "status": "new"})
}
```

> Note: the filename's third segment is the literal `manual`; `player_type` in the JSON is the generic `feedback`. The board distinguishes notes by `source`, not by filename. If `fmt`/`time`/`os`/`json`/`errors`/`authz` are not already imported in this file, they are imported elsewhere in the package and the same import block already lists them — no new import is needed (verify with `goimports` in Step 5).

- [ ] **Step 4: Register the route.** In `services/player/internal/transport/router.go`, in the admin-reports group (the `r.Group` with `AuthMiddleware` + `AdminRoleMiddleware`), add a POST registration next to `r.Get("/admin/reports", ...)`:

```go
		r.Post("/admin/reports", adminReportsHandler.CreateNote)
```

- [ ] **Step 5: Run test + build to verify**

Run: `cd services/player && goimports -w internal/handler/admin_reports.go && go test ./internal/handler/ -run TestAdminReports_CreateNote -v && go build ./...`
Expected: PASS + clean build.

- [ ] **Step 6: Full backend test sweep**

Run: `cd services/player && go test ./... 2>&1 | tail -20`
Expected: all packages PASS (no regression in report/internal_feedback/admin_reports).

- [ ] **Step 7: Commit**

```bash
git add services/player/internal/handler/admin_reports.go services/player/internal/handler/admin_reports_test.go services/player/internal/transport/router.go
git commit -m "feat(player): POST /api/admin/reports quick-capture note (source=manual)

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Task 6: Frontend — types + API client

**Files:**
- Modify: `frontend/web/src/types/feedback.ts`
- Modify: `frontend/web/src/api/client.ts`

**Interfaces:**
- Produces: `FeedbackKind`, `FeedbackSource` types; `FeedbackListItem.kind`; `adminApi.listReports(... kind?, source?)`; `adminApi.createNote({kind, category?, description})`.

- [ ] **Step 1: Extend the types.** In `frontend/web/src/types/feedback.ts`, after the `FeedbackStatus` type, add:

```typescript
// Item nature (Project Board). 'feedback' = inbound from users; 'todo'/'idea'
// are internal notebook items.
export type FeedbackKind = 'feedback' | 'todo' | 'idea'

// Normalized channel the item entered the system through.
export type FeedbackSource = 'feedback_form' | 'telegram' | 'api' | 'manual'
```

In the `FeedbackListItem` interface, add a `kind` field and tighten `source`:

```typescript
  kind?: FeedbackKind
  source?: FeedbackSource
```

(Replace the existing `source?: string` line with `source?: FeedbackSource`; add the `kind?` line above it.)

- [ ] **Step 2: Extend the API client.** In `frontend/web/src/api/client.ts`, update the `FeedbackKind` import line (it currently imports `FeedbackListResponse, FeedbackDetail, FeedbackStatus` — add `FeedbackKind`), then change `listReports` and add `createNote` in the `adminApi` object:

```typescript
  listReports: (params?: { category?: string; status?: string; type?: string; kind?: string; source?: string; username?: string; from?: string; to?: string; page?: number; page_size?: number }) =>
    apiClient.get<FeedbackListResponse | { data: FeedbackListResponse }>('/admin/reports', { params }),
  createNote: (body: { kind: FeedbackKind; category?: string; description: string }) =>
    apiClient.post<{ id: string; status: string } | { data: { id: string; status: string } }>('/admin/reports', body),
```

- [ ] **Step 3: Type-check via real build**

Run: `cd frontend/web && bun run build 2>&1 | tail -8`
Expected: build succeeds (vue-tsc + vite) — confirms the new types/params compile.

- [ ] **Step 4: Commit**

```bash
git add frontend/web/src/types/feedback.ts frontend/web/src/api/client.ts
git commit -m "feat(web): FeedbackKind/Source types + listReports kind/source params + createNote

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Task 7: Frontend — composable filters + `active` default

**Files:**
- Modify: `frontend/web/src/composables/useAdminFeedback.ts`
- Test: `frontend/web/src/composables/__tests__/useAdminFeedback.spec.ts`

**Interfaces:**
- Consumes: `adminApi.listReports` (Task 6).
- Produces: `useAdminFeedback()` now returns `filterKind`, `filterSource` refs; `filterStatus` defaults to `'active'`; `refresh()` sends `kind`/`source` params.

- [ ] **Step 1: Write the failing test** — append a case to `frontend/web/src/composables/__tests__/useAdminFeedback.spec.ts`:

```typescript
it('defaults status to active and sends kind/source params', async () => {
  listSpy.mockResolvedValue(listEnvelope([sampleRow]))
  const fb = useAdminFeedback()
  expect(fb.filterStatus.value).toBe('active')
  fb.filterKind.value = 'todo'
  fb.filterSource.value = 'manual'
  await fb.applyFilters()
  await flushPromises()
  const lastCall = listSpy.mock.calls.at(-1)![0]
  expect(lastCall).toMatchObject({ status: 'active', kind: 'todo', source: 'manual' })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd frontend/web && bunx vitest run src/composables/__tests__/useAdminFeedback.spec.ts`
Expected: FAIL — `fb.filterKind` is undefined; `status` defaults to `'all'`.

- [ ] **Step 3: Update the composable.** In `frontend/web/src/composables/useAdminFeedback.ts`:

Change the status default and add two refs (in the filter-refs block):

```typescript
const filterCategory = ref('all')
const filterStatus = ref('active')
const filterKind = ref('all')
const filterSource = ref('all')
const filterType = ref('all')
const filterUsername = ref('')
const filterDateFrom = ref('')
const filterDateTo = ref('')
```

In `refresh()`, add `kind`/`source` to the `listReports` params (next to `type`):

```typescript
    const res = await adminApi.listReports({
      category: norm(filterCategory.value),
      status: norm(filterStatus.value),
      kind: norm(filterKind.value),
      source: norm(filterSource.value),
      type: norm(filterType.value),
      username: filterUsername.value.trim() || undefined,
      from: dayStartISO(filterDateFrom.value),
      to: dayEndISO(filterDateFrom.value === '' ? filterDateTo.value : filterDateTo.value),
      page: page.value,
      page_size: pageSize.value,
    })
```

(Keep the existing `from`/`to` exactly as they were — only `kind`/`source` are added. If the original `to` line was `to: dayEndISO(filterDateTo.value),` leave it as-is; the line above is illustrative of placement only.)

Add `filterKind` and `filterSource` to the object the composable returns (the `return { ... }` at the end, next to `filterStatus`):

```typescript
    filterCategory, filterStatus, filterKind, filterSource, filterType, filterUsername, filterDateFrom, filterDateTo,
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd frontend/web && bunx vitest run src/composables/__tests__/useAdminFeedback.spec.ts`
Expected: PASS (all cases, including the existing ones).

- [ ] **Step 5: Commit**

```bash
git add frontend/web/src/composables/useAdminFeedback.ts frontend/web/src/composables/__tests__/useAdminFeedback.spec.ts
git commit -m "feat(web): admin-feedback composable gains kind/source filters + active default

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Task 8: Frontend — `NewNoteDialog.vue` quick-capture component

**Files:**
- Create: `frontend/web/src/components/admin/NewNoteDialog.vue`
- Test: `frontend/web/src/components/admin/__tests__/NewNoteDialog.spec.ts`

**Interfaces:**
- Consumes: `adminApi.createNote` (Task 6); `Modal`, `Select`, `Button` from `@/components/ui`.
- Produces: `<NewNoteDialog v-model:open="..." @created="(id) => ..." />`.

- [ ] **Step 1: Write the failing test** — create `frontend/web/src/components/admin/__tests__/NewNoteDialog.spec.ts`:

```typescript
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'

vi.mock('@/api/client', () => ({
  adminApi: { createNote: vi.fn() },
}))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (k: string) => k }) }))

import { adminApi } from '@/api/client'
import NewNoteDialog from '../NewNoteDialog.vue'

const createSpy = adminApi.createNote as ReturnType<typeof vi.fn>

beforeEach(() => vi.clearAllMocks())

describe('NewNoteDialog', () => {
  it('blocks submit when description is empty', async () => {
    const w = mount(NewNoteDialog, { props: { open: true } })
    await (w.vm as unknown as { submit: () => Promise<void> }).submit()
    expect(createSpy).not.toHaveBeenCalled()
  })

  it('posts the note and emits created on success', async () => {
    createSpy.mockResolvedValue({ data: { data: { id: 'note-1', status: 'new' } } })
    const w = mount(NewNoteDialog, { props: { open: true } })
    const vm = w.vm as unknown as { kind: string; description: string; submit: () => Promise<void> }
    vm.kind = 'idea'
    vm.description = 'dark mode'
    await vm.submit()
    await flushPromises()
    expect(createSpy).toHaveBeenCalledWith({ kind: 'idea', category: undefined, description: 'dark mode' })
    expect(w.emitted('created')?.[0]).toEqual(['note-1'])
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd frontend/web && bunx vitest run src/components/admin/__tests__/NewNoteDialog.spec.ts`
Expected: FAIL — cannot resolve `../NewNoteDialog.vue`.

- [ ] **Step 3: Create the component** — create `frontend/web/src/components/admin/NewNoteDialog.vue`:

```vue
<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Modal, Select, Button } from '@/components/ui'
import { adminApi } from '@/api/client'
import type { FeedbackKind } from '@/types/feedback'

const props = defineProps<{ open: boolean }>()
const emit = defineEmits<{ 'update:open': [boolean]; created: [string] }>()

const { t } = useI18n()

const kind = ref<FeedbackKind>('todo')
const category = ref('')
const description = ref('')
const submitting = ref(false)
const errorMsg = ref('')

const kindOptions = [
  { value: 'todo', label: t('admin.feedback.kind.todo') },
  { value: 'idea', label: t('admin.feedback.kind.idea') },
  { value: 'feedback', label: t('admin.feedback.kind.feedback') },
]
const categoryOptions = [
  { value: '', label: t('admin.feedback.newNote.categoryNone') },
  { value: 'bug', label: t('admin.feedback.category.bug') },
  { value: 'issue', label: t('admin.feedback.category.issue') },
  { value: 'feature', label: t('admin.feedback.category.feature') },
]

function reset(): void {
  kind.value = 'todo'
  category.value = ''
  description.value = ''
  errorMsg.value = ''
}

function close(): void {
  emit('update:open', false)
}

function unwrapId(res: unknown): string {
  const d = (res as { data?: unknown }).data
  const inner = (d as { data?: { id?: string }; id?: string })
  return inner?.data?.id ?? inner?.id ?? ''
}

async function submit(): Promise<void> {
  if (!description.value.trim()) {
    errorMsg.value = t('admin.feedback.newNote.error')
    return
  }
  submitting.value = true
  errorMsg.value = ''
  try {
    const res = await adminApi.createNote({
      kind: kind.value,
      category: category.value || undefined,
      description: description.value.trim(),
    })
    emit('created', unwrapId(res))
    reset()
    close()
  } catch {
    errorMsg.value = t('admin.feedback.newNote.error')
  } finally {
    submitting.value = false
  }
}

defineExpose({ kind, category, description, submit })
</script>

<template>
  <Modal
    :model-value="props.open"
    :title="t('admin.feedback.newNote.title')"
    size="sm"
    @update:model-value="(v: boolean) => !v && close()"
  >
    <div class="space-y-4">
      <div>
        <label class="block text-sm font-medium text-white/70 mb-2">{{ t('admin.feedback.newNote.kindLabel') }}</label>
        <Select v-model="kind" size="sm" :options="kindOptions" />
      </div>
      <div>
        <label class="block text-sm font-medium text-white/70 mb-2">{{ t('admin.feedback.newNote.categoryLabel') }}</label>
        <Select v-model="category" size="sm" :options="categoryOptions" />
      </div>
      <div>
        <label class="block text-sm font-medium text-white/70 mb-2">{{ t('admin.feedback.newNote.descriptionLabel') }}</label>
        <textarea
          v-model="description"
          rows="4"
          class="w-full rounded-lg bg-white/5 border border-white/10 px-3 py-2 text-sm text-white placeholder:text-white/30 focus:outline-none focus:ring-2 focus:ring-cyan-500/50 resize-y"
          :placeholder="t('admin.feedback.newNote.descriptionPlaceholder')"
        ></textarea>
      </div>
      <p v-if="errorMsg" class="text-sm text-destructive">{{ errorMsg }}</p>
    </div>

    <template #footer>
      <Button variant="ghost" @click="close">{{ t('admin.feedback.newNote.cancel') }}</Button>
      <Button :disabled="submitting" @click="submit">{{ t('admin.feedback.newNote.submit') }}</Button>
    </template>
  </Modal>
</template>
```

> DS notes: `<textarea>` is NOT a Rule-5-banned control (only `<select>`/`<input type=date|checkbox|radio>` are); it uses token-bound `bg-white/5`/`border-white/10` + brand `cyan` ring (exempt). `Select`/`Button`/`Modal` are reused primitives. Only `font-medium` used.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd frontend/web && bunx vitest run src/components/admin/__tests__/NewNoteDialog.spec.ts`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/web/src/components/admin/NewNoteDialog.vue frontend/web/src/components/admin/__tests__/NewNoteDialog.spec.ts
git commit -m "feat(web): NewNoteDialog quick-capture component

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Task 9: Frontend — wire `AdminFeedback.vue` (filters, badges, kanban, + New note)

**Files:**
- Modify: `frontend/web/src/views/admin/AdminFeedback.vue`

**Interfaces:**
- Consumes: `filterKind`/`filterSource` (Task 7), `NewNoteDialog` (Task 8), the new i18n keys (Task 10).

- [ ] **Step 1: Import + destructure.** Add the dialog import to the `<script setup>` import block:

```typescript
import NewNoteDialog from '@/components/admin/NewNoteDialog.vue'
```

Add `filterKind, filterSource` to the destructure from `useAdminFeedback()`:

```typescript
  filterCategory, filterStatus, filterKind, filterSource, filterType, filterUsername, filterDateFrom, filterDateTo,
```

- [ ] **Step 2: Add option + badge helpers.** After the existing `statusOptions` computed, add `kindOptions`, `sourceOptions`, an `'active'` status option, and label/variant helpers:

In `statusOptions`, prepend an `active` option as the first entry:

```typescript
const statusOptions = computed(() => [
  { value: 'active', label: t('admin.feedback.status.active') },
  { value: 'all', label: t('admin.feedback.filters.allStatuses') },
  { value: 'new', label: statusLabel('new') },
  { value: 'in_progress', label: statusLabel('in_progress') },
  { value: 'ai_done', label: statusLabel('ai_done') },
  { value: 'resolved', label: statusLabel('resolved') },
  { value: 'not_relevant', label: statusLabel('not_relevant') },
])

const KINDS = ['feedback', 'todo', 'idea'] as const
const SOURCES = ['feedback_form', 'telegram', 'api', 'manual'] as const
const kindLabel = (k: string) => t(`admin.feedback.kind.${k}`)
const sourceLabel = (s: string) => t(`admin.feedback.source.${s}`)
const kindOptions = computed(() => [
  { value: 'all', label: t('admin.feedback.filters.allKinds') },
  ...KINDS.map((v) => ({ value: v, label: kindLabel(v) })),
])
const sourceOptions = computed(() => [
  { value: 'all', label: t('admin.feedback.filters.allSources') },
  ...SOURCES.map((v) => ({ value: v, label: sourceLabel(v) })),
])

type KindBadge = 'info' | 'primary' | 'warning'
const KIND_VARIANT: Record<string, KindBadge> = { feedback: 'info', todo: 'primary', idea: 'warning' }
const kindVariant = (k: string): KindBadge => KIND_VARIANT[k] ?? 'info'
```

- [ ] **Step 3: Kanban honors `active`.** Replace the `kanbanColumns` computed and the `filterStatus` force in `setViewMode`:

In `setViewMode`, delete the line `if (m === 'kanban') filterStatus.value = 'all'` (kanban now honors the shared status filter, default `active`).

Replace `kanbanColumns` with:

```typescript
const kanbanColumns = computed(() => {
  let order: FeedbackStatus[] = STATUS_ORDER
  if (filterStatus.value === 'active') {
    order = STATUS_ORDER.filter((s) => s !== 'not_relevant')
  } else if (filterStatus.value !== 'all') {
    order = STATUS_ORDER.filter((s) => s === filterStatus.value)
  }
  return order.map((status) => ({
    status,
    items: items.value.filter((i) => i.status === status),
  }))
})
```

- [ ] **Step 4: Reset uses `active`.** Find the filter-reset (clear) path that sets `filterStatus.value = 'all'` (around the deep-link/clear logic) and change it to `'active'`; also reset the two new filters there:

```typescript
    filterStatus.value = 'active'
    filterKind.value = 'all'
    filterSource.value = 'all'
```

- [ ] **Step 5: Add filter dropdowns + + New note button (template).**

In the header, add a + New note button before the Refresh button:

```vue
        <button
          type="button"
          class="px-4 py-2 rounded-md bg-white/5 hover:bg-white/10 border border-white/10 text-white font-medium text-sm transition"
          @click="showNewNote = true"
        >
          {{ $t('admin.feedback.newNote.button') }}
        </button>
```

Change the filter grid wrapper class from `lg:grid-cols-6` to `lg:grid-cols-4` and add the Kind + Source selects (place them right after the Category select, before the status filter). Also remove the `v-if="viewMode === 'table'"` on the status filter so it shows in both modes:

```vue
        <Select
          v-model="filterKind"
          size="sm"
          :options="kindOptions"
          :label="$t('admin.feedback.filters.kind')"
          @change="applyFilters"
        />
        <Select
          v-model="filterSource"
          size="sm"
          :options="sourceOptions"
          :label="$t('admin.feedback.filters.source')"
          @change="applyFilters"
        />
        <Select
          v-model="filterStatus"
          size="sm"
          :options="statusOptions"
          :label="$t('admin.feedback.filters.status')"
          @change="applyFilters"
        />
```

At the end of the template (sibling to the detail modal), mount the dialog:

```vue
    <NewNoteDialog v-model:open="showNewNote" @created="refresh" />
```

- [ ] **Step 6: Add `showNewNote` ref.** In `<script setup>` (near `viewMode`):

```typescript
const showNewNote = ref(false)
```

- [ ] **Step 7: Render kind/source badges.** In the table row category cell (where the category `Badge` is rendered) and the kanban card header, add a kind badge before the category badge and a muted source badge. Table cell:

```vue
        <Badge size="sm" :variant="kindVariant(r.kind || 'feedback')" class="text-[10px] font-mono uppercase mr-1">
          {{ kindLabel(r.kind || 'feedback') }}
        </Badge>
        <Badge size="sm" :variant="categoryVariant(r.category)" class="text-[10px] font-mono uppercase">
          {{ categoryLabel(r.category) }}
        </Badge>
        <span v-if="r.source" class="ml-2 text-white/40 text-[10px] uppercase">{{ sourceLabel(r.source) }}</span>
```

Kanban card header (the `flex items-center justify-between` block with the category Badge):

```vue
          <div class="flex items-center gap-1.5">
            <Badge size="sm" :variant="kindVariant(r.kind || 'feedback')" class="text-[10px] font-mono uppercase">
              {{ kindLabel(r.kind || 'feedback') }}
            </Badge>
            <Badge size="sm" :variant="categoryVariant(r.category)" class="text-[10px] font-mono uppercase">
              {{ categoryLabel(r.category) }}
            </Badge>
          </div>
```

(Leave the existing date `<span>` in that header block as-is.)

- [ ] **Step 8: Update the header comment.** Change the top-of-file comment from `read + triage user feedback / error reports.` to `Project Board: feedback, TODOs and ideas — read, triage + quick-capture.`

- [ ] **Step 9: DS-lint + build**

Run: `cd frontend/web && bash scripts/design-system-lint.sh && bun run build 2>&1 | tail -8`
Expected: DS-lint PASS (0 violations); build succeeds.

- [ ] **Step 10: Commit**

```bash
git add frontend/web/src/views/admin/AdminFeedback.vue
git commit -m "feat(web): Project Board — kind/source filters+badges, active default, + New note

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Task 10: i18n — en/ru/ja keys (title, kind, source, newNote, status.active)

**Files:**
- Modify: `frontend/web/src/locales/en.json`
- Modify: `frontend/web/src/locales/ru.json`
- Modify: `frontend/web/src/locales/ja.json`

**Interfaces:**
- Produces: all `admin.feedback.*` keys referenced in Tasks 8–9.

- [ ] **Step 1: en.json.** In the `admin.feedback` object: set `title`/`subtitle`, add `kind`/`source`/`newNote` sub-objects, add `filters.kind`/`filters.source`/`filters.allKinds`/`filters.allSources`, and `status.active`:

```json
    "title": "Project Board",
    "subtitle": "Feedback, TODOs and ideas — the whole project backlog in one place.",
```
Add inside `filters`: `"kind": "Kind", "source": "Source", "allKinds": "All kinds", "allSources": "All sources"`.
Add inside `status`: `"active": "Active (hide dismissed)"`.
Add two new sub-objects at the `admin.feedback` level:
```json
    "kind": { "feedback": "Feedback", "todo": "TODO", "idea": "Idea" },
    "source": { "feedback_form": "Feedback form", "telegram": "Telegram", "api": "API", "manual": "Manual" },
    "newNote": {
      "button": "+ New note",
      "title": "New note",
      "kindLabel": "Kind",
      "categoryLabel": "Category",
      "categoryNone": "—",
      "descriptionLabel": "Description",
      "descriptionPlaceholder": "What needs doing? Describe the TODO or idea…",
      "submit": "Add",
      "cancel": "Cancel",
      "success": "Note added",
      "error": "Failed to add note"
    },
```

- [ ] **Step 2: ru.json.** Same structure, Russian values:

```json
    "title": "Доска проекта",
    "subtitle": "Отзывы, задачи и идеи — весь бэклог проекта в одном месте.",
```
`filters`: `"kind": "Тип", "source": "Источник", "allKinds": "Все типы", "allSources": "Все источники"`.
`status`: `"active": "Активные (скрыть нерелевантные)"`.
```json
    "kind": { "feedback": "Отзыв", "todo": "Задача", "idea": "Идея" },
    "source": { "feedback_form": "Форма отзывов", "telegram": "Telegram", "api": "API", "manual": "Вручную" },
    "newNote": {
      "button": "+ Новая заметка",
      "title": "Новая заметка",
      "kindLabel": "Тип",
      "categoryLabel": "Категория",
      "categoryNone": "—",
      "descriptionLabel": "Описание",
      "descriptionPlaceholder": "Что нужно сделать? Опишите задачу или идею…",
      "submit": "Добавить",
      "cancel": "Отмена",
      "success": "Заметка добавлена",
      "error": "Не удалось добавить заметку"
    },
```

- [ ] **Step 3: ja.json.** Same structure, Japanese values:

```json
    "title": "プロジェクトボード",
    "subtitle": "フィードバック・TODO・アイデア — プロジェクトのバックログを一か所に。",
```
`filters`: `"kind": "種別", "source": "ソース", "allKinds": "すべての種別", "allSources": "すべてのソース"`.
`status`: `"active": "アクティブ（非該当を隠す）"`.
```json
    "kind": { "feedback": "フィードバック", "todo": "TODO", "idea": "アイデア" },
    "source": { "feedback_form": "フィードバックフォーム", "telegram": "Telegram", "api": "API", "manual": "手動" },
    "newNote": {
      "button": "＋ 新規メモ",
      "title": "新規メモ",
      "kindLabel": "種別",
      "categoryLabel": "カテゴリ",
      "categoryNone": "—",
      "descriptionLabel": "説明",
      "descriptionPlaceholder": "何をする必要がありますか？TODOやアイデアを記入…",
      "submit": "追加",
      "cancel": "キャンセル",
      "success": "メモを追加しました",
      "error": "メモの追加に失敗しました"
    },
```

- [ ] **Step 4: Validate JSON + parity.**

Run:
```bash
cd frontend/web
node -e "for (const f of ['en','ru','ja']) JSON.parse(require('fs').readFileSync('src/locales/'+f+'.json','utf8')); console.log('json ok')"
bash scripts/i18n-lint.sh; bunx vitest run src/locales/__tests__
```
Expected: `json ok`; i18n-lint clean (retry once if flaky); locale parity specs PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/web/src/locales/en.json frontend/web/src/locales/ru.json frontend/web/src/locales/ja.json
git commit -m "i18n(web): Project Board — kind/source/newNote/status.active in en+ru+ja

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Task 11: Full verification + ship

**Files:** none (verification + deploy)

- [ ] **Step 1: Backend sweep**

Run: `cd services/player && go test ./... 2>&1 | tail -20`
Expected: all PASS.

- [ ] **Step 2: Frontend pre-flight (`/frontend-verify`).** Run DS-lint, i18n parity, and the REAL build, plus the touched specs:

```bash
cd frontend/web
bash scripts/design-system-lint.sh
bash scripts/i18n-lint.sh
bunx vitest run src/composables/__tests__/useAdminFeedback.spec.ts src/components/admin/__tests__/NewNoteDialog.spec.ts src/locales/__tests__
bun run build 2>&1 | tail -8
```
Expected: DS-lint PASS, i18n clean, specs PASS, build succeeds.

- [ ] **Step 3: Ship via `/animeenigma-after-update`.** This change touches `services/player/**` and `frontend/web/**`, so the redeploy set is **`player`** + **`web`**. Run `/animeenigma-after-update`, which will: lint/build, `make redeploy-player` + `make redeploy-web` (from `main` after push), `make health`, prepend a Russian Trump-mode changelog entry, commit, and push. (Per the worktree deploy rule, push these commits to `origin/main` first, then build from the clean worktree.)

- [ ] **Step 4: Manual smoke (optional, opt-in).** On `/admin/feedback`: confirm title reads "Project Board"; Kind + Source dropdowns filter; dismissed (`not_relevant`) items are hidden until you pick All/Not relevant; + New note creates an item that appears as `new` with a Manual source badge.

---

## Self-Review

**Spec coverage:**
- §4 data model (kind/source/category) → Task 1. ✓
- §5.1 writers stamp → Task 4 (report.go, internal_feedback.go) + Task 5 (manual). ✓
- §5.2 legacy read-time derivation → Task 1 helpers, applied in Task 2 (List) + Task 3 (Get). ✓
- §6 backend (struct, endpoint, List/Get) → Tasks 1–5. ✓
- §6 server-side kind/source/active filtering → Task 2. ✓
- §6 deep-link bypass for dismissed → Task 3 (test asserts 200 + normalized fields). ✓
- §7.1 identity/filters/status-active/kanban/badges/+New note → Tasks 9, 10. ✓
- §7.2 NewNoteDialog → Task 8. ✓
- §7.3 types → Task 6. ✓
- §7.4 composable filter state + server params → Task 7. ✓
- §8 i18n en/ru/ja → Task 10. ✓
- §9 testing (derivation, endpoint, filters, deep-link, FE specs, parity) → Tasks 1–10 each carry tests. ✓
- §10 out-of-scope (tags/priority/title/backfill) → not implemented (correct). ✓

**Placeholder scan:** No TBD/TODO/"handle edge cases"; every code step shows real code; the one illustrative note in Task 7 Step 3 explicitly says to keep the original `from`/`to` lines.

**Type consistency:** `normalizeSource`/`deriveKind` signatures identical across Tasks 1–4. `FeedbackKind`/`FeedbackSource` defined in Task 6, consumed in Tasks 7–9. `adminApi.createNote` payload `{kind, category?, description}` matches NewNoteDialog (Task 8) and the backend `noteCreateRequest` (Task 5). `filterKind`/`filterSource` defined in Task 7, consumed in Task 9. Status `active` sentinel: FE option (Task 9) ↔ composable default (Task 7) ↔ backend handling (Task 2). i18n keys referenced in Tasks 8–9 all defined in Task 10.

**Impact:** `UXΔ = +3 (Better)` · `CDI = 0.03 * 13` · `MVQ = Griffin 88%/85%`.
