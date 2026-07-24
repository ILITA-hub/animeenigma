# Gacha Bulk Card Upload + Bulk Editing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let admins mass-upload card cover images (each file → disabled draft card) and mass-edit cards (selection + bulk actions + inline cell editing) in the `/admin/gacha` Cards tab.

**Architecture:** One new partial-update bulk endpoint pair in `services/gacha` (`PATCH /api/gacha/admin/cards/bulk`, `POST /api/gacha/admin/cards/bulk-delete`); the FE reuses existing upload/create endpoints for the batch-upload dialog (new `GachaBulkUpload.vue`) and uses the bulk endpoint for both multi-select actions AND single-cell inline edits (partial semantics — no full-replace risk). Spec: `docs/superpowers/specs/2026-07-24-gacha-bulk-cards-design.md`.

**Tech Stack:** Go (chi + GORM + sqlite tests), Vue 3 `<script setup>` + vitest, existing UI primitives (Modal/Button/Select/Checkbox/Input/Spinner).

## Global Constraints

- Worktree: `/tmp/ae-gacha-bulk` (branch `gacha-bulk-cards`). ALL edits there, never in `/data/animeenigma`.
- Commits: conventional style, pathspec-only (`git commit -m "…" -- <paths>`), NEVER `git add -A`. Co-authors on every commit:
  `Co-Authored-By: Claude Code <noreply@anthropic.com>` + `Co-Authored-By: 0neymik0 <0neymik0@gmail.com>` + `Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>`.
- i18n: every new key goes to **en.json, ru.json AND ja.json** (all three — lint gate).
- DS-lint: no bare native form controls (use `Input`/`Select`/`Checkbox` primitives), no raw hex/rgba, semantic tokens only. `teal-400`/`text-destructive`/`border-white/*` idioms already used in `AdminGacha.vue` are fine.
- Frontend tooling: `bun` / `bunx` only (never npm/npx). Go: plain `go test`, never `gofmt -w`.
- Drafts must stay invisible to gameplay: created cards get `enabled=false` — pull pools and public surfaces already filter on `enabled`.

---

### Task 1: Backend — repo + service bulk operations (TDD)

**Files:**
- Modify: `services/gacha/internal/repo/card.go` (append after `GroupCardIDs`)
- Modify: `services/gacha/internal/service/content.go` (append after `ListCards`, ~line 121)
- Test: `services/gacha/internal/service/content_test.go` (append)

**Interfaces:**
- Produces (used by Task 2):
  - `repo.CardBulkSet{Name, SourceTitle *string; Rarity *domain.Rarity; Enabled *bool}`
  - `(*repo.ContentRepository) BulkUpdateCards(ctx, ids []string, set CardBulkSet) (int64, error)`
  - `(*repo.ContentRepository) BulkDeleteCards(ctx, ids []string) (int64, error)`
  - `service.BulkCardSet` (JSON: `name`, `source_title`, `rarity`, `enabled` — all optional pointers)
  - `service.BulkUpdateCardsRequest{IDs []string `json:"ids"`; Set BulkCardSet `json:"set"`}`
  - `(*service.ContentService) BulkUpdateCards(ctx, req BulkUpdateCardsRequest) (int64, error)`
  - `(*service.ContentService) BulkDeleteCards(ctx, ids []string) (int64, error)`

- [ ] **Step 1: Write the failing tests** — append to `content_test.go`:

```go
func TestBulkUpdateCards_ValidationAndApply(t *testing.T) {
	svc := newContentService(t)
	ctx := context.Background()

	c1, err := svc.CreateCard(ctx, CreateCardRequest{Name: "A", Rarity: domain.RarityN, ImagePath: "cards/a.webp"})
	if err != nil {
		t.Fatalf("create c1: %v", err)
	}
	c2, err := svc.CreateCard(ctx, CreateCardRequest{Name: "B", Rarity: domain.RarityN, ImagePath: "cards/b.webp"})
	if err != nil {
		t.Fatalf("create c2: %v", err)
	}

	// Empty ids → InvalidInput
	if _, err := svc.BulkUpdateCards(ctx, BulkUpdateCardsRequest{}); !isInvalidInput(err) {
		t.Errorf("empty ids: want InvalidInput, got %v", err)
	}

	// Empty set → InvalidInput
	if _, err := svc.BulkUpdateCards(ctx, BulkUpdateCardsRequest{IDs: []string{c1.ID}}); !isInvalidInput(err) {
		t.Errorf("empty set: want InvalidInput, got %v", err)
	}

	// Empty-string name → InvalidInput (source_title MAY be blanked, name may not)
	empty := ""
	if _, err := svc.BulkUpdateCards(ctx, BulkUpdateCardsRequest{
		IDs: []string{c1.ID}, Set: BulkCardSet{Name: &empty},
	}); !isInvalidInput(err) {
		t.Errorf("empty name: want InvalidInput, got %v", err)
	}

	// Bad rarity → InvalidInput
	bad := domain.Rarity("XX")
	if _, err := svc.BulkUpdateCards(ctx, BulkUpdateCardsRequest{
		IDs: []string{c1.ID}, Set: BulkCardSet{Rarity: &bad},
	}); !isInvalidInput(err) {
		t.Errorf("bad rarity: want InvalidInput, got %v", err)
	}

	// Valid partial update: rarity+enabled+blank source on both cards; a
	// nonexistent id is silently skipped (affected count reports reality).
	sr := domain.RaritySR
	on := true
	blank := ""
	n, err := svc.BulkUpdateCards(ctx, BulkUpdateCardsRequest{
		IDs: []string{c1.ID, c2.ID, "00000000-0000-0000-0000-000000000000"},
		Set: BulkCardSet{Rarity: &sr, Enabled: &on, SourceTitle: &blank},
	})
	if err != nil {
		t.Fatalf("bulk update: %v", err)
	}
	if n != 2 {
		t.Errorf("want 2 rows affected (missing id skipped), got %d", n)
	}

	got, err := svc.GetCard(ctx, c1.ID)
	if err != nil {
		t.Fatalf("get c1: %v", err)
	}
	if got.Rarity != domain.RaritySR || !got.Enabled {
		t.Errorf("c1 not updated: %+v", got)
	}
	if got.Name != "A" {
		t.Errorf("untouched field must survive: name = %q, want A", got.Name)
	}
}

func TestBulkDeleteCards(t *testing.T) {
	svc := newContentService(t)
	ctx := context.Background()

	c1, err := svc.CreateCard(ctx, CreateCardRequest{Name: "A", Rarity: domain.RarityN, ImagePath: "cards/a.webp"})
	if err != nil {
		t.Fatalf("create c1: %v", err)
	}
	c2, err := svc.CreateCard(ctx, CreateCardRequest{Name: "B", Rarity: domain.RarityN, ImagePath: "cards/b.webp"})
	if err != nil {
		t.Fatalf("create c2: %v", err)
	}

	if _, err := svc.BulkDeleteCards(ctx, nil); !isInvalidInput(err) {
		t.Errorf("empty ids: want InvalidInput, got %v", err)
	}

	n, err := svc.BulkDeleteCards(ctx, []string{c1.ID, c2.ID})
	if err != nil {
		t.Fatalf("bulk delete: %v", err)
	}
	if n != 2 {
		t.Errorf("want 2 deleted, got %d", n)
	}

	if _, err := svc.GetCard(ctx, c1.ID); err == nil {
		t.Error("c1 must be soft-deleted (GetCard should return NotFound)")
	}
	list, err := svc.ListCards(ctx, repo.CardFilter{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("want empty list after bulk delete, got %d", len(list))
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /tmp/ae-gacha-bulk/services/gacha && go test ./internal/service/ -run 'TestBulk' -v`
Expected: compile error — `BulkUpdateCardsRequest`/`BulkCardSet` undefined.

- [ ] **Step 3: Implement repo layer** — append to `services/gacha/internal/repo/card.go`:

```go
// CardBulkSet carries the optional fields BulkUpdateCards applies.
// nil pointer = leave that column unchanged.
type CardBulkSet struct {
	Name        *string
	SourceTitle *string
	Rarity      *domain.Rarity
	Enabled     *bool
}

// BulkUpdateCards applies the non-nil fields of set to every card in ids.
// Soft-deleted cards are excluded by GORM's default scope; returns rows affected.
func (r *ContentRepository) BulkUpdateCards(ctx context.Context, ids []string, set CardBulkSet) (int64, error) {
	updates := map[string]any{}
	if set.Name != nil {
		updates["name"] = *set.Name
	}
	if set.SourceTitle != nil {
		updates["source_title"] = *set.SourceTitle
	}
	if set.Rarity != nil {
		updates["rarity"] = *set.Rarity
	}
	if set.Enabled != nil {
		updates["enabled"] = *set.Enabled
	}
	res := r.db.WithContext(ctx).Model(&domain.Card{}).Where("id IN ?", ids).Updates(updates)
	return res.RowsAffected, res.Error
}

// BulkDeleteCards soft-deletes every card in ids. Same semantics as DeleteCard:
// group/banner join rows stay in place — every join query already filters on
// gacha_cards.deleted_at. Returns rows affected.
func (r *ContentRepository) BulkDeleteCards(ctx context.Context, ids []string) (int64, error) {
	res := r.db.WithContext(ctx).Delete(&domain.Card{}, "id IN ?", ids)
	return res.RowsAffected, res.Error
}
```

- [ ] **Step 4: Implement service layer** — insert into `services/gacha/internal/service/content.go` right after the `ListCards` method (before the `── Groups ──` divider):

```go
// BulkCardSet mirrors repo.CardBulkSet with JSON pointer semantics — keys
// absent from the request body are left unchanged on every card.
type BulkCardSet struct {
	Name        *string        `json:"name"`
	SourceTitle *string        `json:"source_title"`
	Rarity      *domain.Rarity `json:"rarity"`
	Enabled     *bool          `json:"enabled"`
}

// BulkUpdateCardsRequest is the PATCH /api/gacha/admin/cards/bulk payload.
type BulkUpdateCardsRequest struct {
	IDs []string    `json:"ids"`
	Set BulkCardSet `json:"set"`
}

// BulkUpdateCards validates and applies a partial update to a set of cards,
// returning the number of cards actually updated (missing/soft-deleted ids
// are skipped, not errors — the caller refetches the list anyway).
func (s *ContentService) BulkUpdateCards(ctx context.Context, req BulkUpdateCardsRequest) (int64, error) {
	if len(req.IDs) == 0 {
		return 0, apperrors.InvalidInput("ids is required")
	}
	set := req.Set
	if set.Name == nil && set.SourceTitle == nil && set.Rarity == nil && set.Enabled == nil {
		return 0, apperrors.InvalidInput("set must contain at least one field")
	}
	if set.Name != nil && *set.Name == "" {
		return 0, apperrors.InvalidInput("card name cannot be empty")
	}
	if set.Rarity != nil && !domain.ValidRarity(*set.Rarity) {
		return 0, apperrors.InvalidInput("invalid rarity: must be N, R, SR, or SSR")
	}
	return s.cards.BulkUpdateCards(ctx, req.IDs, repo.CardBulkSet{
		Name:        set.Name,
		SourceTitle: set.SourceTitle,
		Rarity:      set.Rarity,
		Enabled:     set.Enabled,
	})
}

// BulkDeleteCards soft-deletes a set of cards, returning how many were deleted.
func (s *ContentService) BulkDeleteCards(ctx context.Context, ids []string) (int64, error) {
	if len(ids) == 0 {
		return 0, apperrors.InvalidInput("ids is required")
	}
	return s.cards.BulkDeleteCards(ctx, ids)
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd /tmp/ae-gacha-bulk/services/gacha && go test ./... 2>&1 | tail -20`
Expected: all packages `ok`, both new tests PASS.

- [ ] **Step 6: Commit**

```bash
cd /tmp/ae-gacha-bulk
git commit -m "feat(gacha): bulk update/delete card operations (repo+service)" \
  -m "Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>" \
  -- services/gacha/internal/repo/card.go services/gacha/internal/service/content.go services/gacha/internal/service/content_test.go
```

---

### Task 2: Backend — handler + routes

**Files:**
- Modify: `services/gacha/internal/handler/admin.go` (append to the Cards section, after `DeleteCard` ~line 126)
- Modify: `services/gacha/internal/transport/router.go` (Cards block, ~line 108)
- Test: `services/gacha/internal/transport/router_test.go` (append)

**Interfaces:**
- Consumes: `service.BulkUpdateCardsRequest`, `(*ContentService).BulkUpdateCards`, `(*ContentService).BulkDeleteCards` (Task 1).
- Produces (used by Task 3 FE client):
  - `PATCH /api/gacha/admin/cards/bulk` → `{"data": {"updated": <n>}}`
  - `POST /api/gacha/admin/cards/bulk-delete` → `{"data": {"deleted": <n>}}`

- [ ] **Step 1: Write the failing router test** — append to `router_test.go`:

```go
// TestRouter_AdminBulk_RequiresAuth asserts the bulk card routes exist and are
// auth-gated (401 without token — NOT 404/405, which would mean a routing bug).
func TestRouter_AdminBulk_RequiresAuth(t *testing.T) {
	r := getTestRouter(t)
	for _, tc := range []struct{ method, path string }{
		{http.MethodPatch, "/api/gacha/admin/cards/bulk"},
		{http.MethodPost, "/api/gacha/admin/cards/bulk-delete"},
	} {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("%s %s: expected 401 without token, got %d", tc.method, tc.path, rr.Code)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /tmp/ae-gacha-bulk/services/gacha && go test ./internal/transport/ -run TestRouter_AdminBulk -v`
Expected: FAIL — `PATCH /cards/bulk` currently falls into `PATCH /cards/{id}` with id="bulk" → handler-level `isUUID` check → 400, not 401? No: auth middleware runs BEFORE the handler, so the param route also 401s. The test still fails before Step 3 for the `bulk-delete` path (405: no POST on `/cards/{id}`). Either way expect at least one subtest failing.

- [ ] **Step 3: Implement handlers** — append to the Cards section of `admin.go` (after `DeleteCard`):

```go
// BulkUpdateCards handles PATCH /api/gacha/admin/cards/bulk — applies the
// present fields of `set` to every card in `ids` (partial semantics).
func (h *AdminHandler) BulkUpdateCards(w http.ResponseWriter, r *http.Request) {
	var req service.BulkUpdateCardsRequest
	if err := httputil.Bind(r, &req); err != nil {
		httputil.Error(w, err)
		return
	}
	for _, id := range req.IDs {
		if !isUUID(id) {
			httputil.Error(w, apperrors.InvalidInput("invalid id: "+id))
			return
		}
	}
	n, err := h.content.BulkUpdateCards(r.Context(), req)
	if err != nil {
		h.log.Errorw("bulk update cards", "count", len(req.IDs), "error", err)
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, map[string]int64{"updated": n})
}

type bulkDeleteRequest struct {
	IDs []string `json:"ids"`
}

// BulkDeleteCards handles POST /api/gacha/admin/cards/bulk-delete.
func (h *AdminHandler) BulkDeleteCards(w http.ResponseWriter, r *http.Request) {
	var req bulkDeleteRequest
	if err := httputil.Bind(r, &req); err != nil {
		httputil.Error(w, err)
		return
	}
	for _, id := range req.IDs {
		if !isUUID(id) {
			httputil.Error(w, apperrors.InvalidInput("invalid id: "+id))
			return
		}
	}
	n, err := h.content.BulkDeleteCards(r.Context(), req.IDs)
	if err != nil {
		h.log.Errorw("bulk delete cards", "count", len(req.IDs), "error", err)
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, map[string]int64{"deleted": n})
}
```

- [ ] **Step 4: Register routes** — in `router.go`, inside the `// Cards` block, add ABOVE the `{id}` routes (chi gives static segments priority anyway; ordering is for the human reader):

```go
			// Cards
			r.Post("/cards", adminHandler.CreateCard)
			r.Get("/cards", adminHandler.ListCards)
			// Bulk ops — static segments route before /cards/{id}.
			r.Patch("/cards/bulk", adminHandler.BulkUpdateCards)
			r.Post("/cards/bulk-delete", adminHandler.BulkDeleteCards)
			r.Get("/cards/{id}", adminHandler.GetCard)
			r.Patch("/cards/{id}", adminHandler.UpdateCard)
			r.Delete("/cards/{id}", adminHandler.DeleteCard)
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd /tmp/ae-gacha-bulk/services/gacha && go test ./... 2>&1 | tail -10`
Expected: all `ok`.

- [ ] **Step 6: Commit**

```bash
cd /tmp/ae-gacha-bulk
git commit -m "feat(gacha): bulk card endpoints — PATCH /cards/bulk + POST /cards/bulk-delete" \
  -m "Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>" \
  -- services/gacha/internal/handler/admin.go services/gacha/internal/transport/router.go services/gacha/internal/transport/router_test.go
```

---

### Task 3: Frontend — API client additions

**Files:**
- Modify: `frontend/web/src/api/gacha.ts`

**Interfaces:**
- Consumes: Task 2 endpoints.
- Produces (used by Tasks 4–5):
  - `export interface BulkCardSet { name?: string; source_title?: string; rarity?: Rarity; enabled?: boolean }`
  - `gachaAdminApi.bulkUpdateCards(ids: string[], set: BulkCardSet)`
  - `gachaAdminApi.bulkDeleteCards(ids: string[])`

- [ ] **Step 1: Add the type** — near the other exported admin types in `gacha.ts` (e.g. right above the `gachaAdminApi` object):

```ts
/** Partial field set for bulk card updates — absent keys are left unchanged. */
export interface BulkCardSet {
  name?: string
  source_title?: string
  rarity?: Rarity
  enabled?: boolean
}
```

- [ ] **Step 2: Add the methods** — inside `gachaAdminApi`, directly after `deleteCard` (~line 259):

```ts
  bulkUpdateCards: (ids: string[], set: BulkCardSet) =>
    apiClient.patch<{ data: { updated: number } }>('/gacha/admin/cards/bulk', { ids, set }),

  bulkDeleteCards: (ids: string[]) =>
    apiClient.post<{ data: { deleted: number } }>('/gacha/admin/cards/bulk-delete', { ids }),
```

- [ ] **Step 3: Type-check**

Run: `cd /tmp/ae-gacha-bulk/frontend/web && bunx vue-tsc --noEmit 2>&1 | tail -5`
Expected: no errors (note: `bun install` first if `node_modules` missing in the worktree).

- [ ] **Step 4: Commit**

```bash
cd /tmp/ae-gacha-bulk
git commit -m "feat(web): gacha admin API — bulkUpdateCards/bulkDeleteCards" \
  -m "Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>" \
  -- frontend/web/src/api/gacha.ts
```

---

### Task 4: Frontend — `GachaBulkUpload.vue` dialog + spec + its i18n keys

**Files:**
- Create: `frontend/web/src/components/admin/GachaBulkUpload.vue`
- Test: `frontend/web/src/components/admin/GachaBulkUpload.spec.ts`
- Modify: `frontend/web/src/locales/en.json`, `ru.json`, `ja.json` (keys under `gacha.admin`)

**Interfaces:**
- Consumes: existing `gachaAdminApi.uploadFile(file, 'cards')` and `gachaAdminApi.createCard(payload)`.
- Produces (used by Task 5): component with `modelValue: boolean` v-model, no other props/emits. Parent refetches cards when the dialog closes.

- [ ] **Step 1: Add i18n keys** — to `gacha.admin` in ALL THREE locale files:

en.json:
```json
"bulk_upload_btn": "Bulk upload",
"bulk_upload_title": "Bulk cover upload",
"bulk_upload_hint": "Drop images here or click to pick files — each becomes a disabled draft card (rarity N, name from the file name)",
"bulk_upload_progress": "{done} of {total} uploaded",
"bulk_upload_retry": "Retry failed ({n})",
"bulk_upload_close": "Close",
"bulk_unnamed": "Unnamed"
```

ru.json:
```json
"bulk_upload_btn": "Загрузить пачку",
"bulk_upload_title": "Массовая загрузка обложек",
"bulk_upload_hint": "Перетащите изображения или кликните для выбора — каждая станет карточкой-черновиком (редкость N, имя из файла)",
"bulk_upload_progress": "{done} из {total} загружено",
"bulk_upload_retry": "Повторить неудачные ({n})",
"bulk_upload_close": "Закрыть",
"bulk_unnamed": "Без имени"
```

ja.json:
```json
"bulk_upload_btn": "一括アップロード",
"bulk_upload_title": "カバー一括アップロード",
"bulk_upload_hint": "画像をドロップまたはクリックで選択 — 各画像が下書きカードになります（レアリティN、名前はファイル名）",
"bulk_upload_progress": "{done} / {total} 完了",
"bulk_upload_retry": "失敗を再試行 ({n})",
"bulk_upload_close": "閉じる",
"bulk_unnamed": "名称未設定"
```

- [ ] **Step 2: Write the failing component spec** — `GachaBulkUpload.spec.ts`:

```ts
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import en from '@/locales/en.json'
import ru from '@/locales/ru.json'
import ja from '@/locales/ja.json'
import GachaBulkUpload from './GachaBulkUpload.vue'

vi.mock('@/api/gacha', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api/gacha')>()
  return {
    ...actual,
    gachaAdminApi: {
      uploadFile: vi.fn(),
      createCard: vi.fn(),
    },
  }
})

import { gachaAdminApi } from '@/api/gacha'

const i18n = createI18n({ locale: 'en', legacy: false, messages: { en, ru, ja } })

function mountDialog() {
  return mount(GachaBulkUpload, {
    props: { modelValue: true },
    global: {
      plugins: [i18n],
      stubs: { teleport: true },
    },
  })
}

/** jsdom file inputs are read-only — override `files` to simulate a pick. */
async function pickFiles(wrapper: ReturnType<typeof mountDialog>, files: File[]) {
  const input = wrapper.find('[data-testid="bulk-file-input"]')
  Object.defineProperty(input.element, 'files', { value: files, configurable: true })
  await input.trigger('change')
  await flushPromises()
}

describe('GachaBulkUpload', () => {
  beforeEach(() => {
    vi.mocked(gachaAdminApi.uploadFile).mockReset()
    vi.mocked(gachaAdminApi.createCard).mockReset()
  })

  it('uploads each picked file and creates a disabled draft card per file', async () => {
    vi.mocked(gachaAdminApi.uploadFile).mockResolvedValue({
      data: { data: { image_path: 'cards/x.png', image_url: '/api/gacha/images/cards/x.png' } },
    } as never)
    vi.mocked(gachaAdminApi.createCard).mockResolvedValue({ data: { data: {} } } as never)

    const wrapper = mountDialog()
    await pickFiles(wrapper, [
      new File(['a'], 'Emilia.png', { type: 'image/png' }),
      new File(['b'], 'Rem.webp', { type: 'image/webp' }),
    ])

    expect(gachaAdminApi.uploadFile).toHaveBeenCalledTimes(2)
    expect(gachaAdminApi.uploadFile).toHaveBeenCalledWith(expect.any(File), 'cards')
    expect(gachaAdminApi.createCard).toHaveBeenCalledTimes(2)
    expect(gachaAdminApi.createCard).toHaveBeenCalledWith({
      name: 'Emilia',
      source_title: '',
      rarity: 'N',
      enabled: false,
      image_path: 'cards/x.png',
      back_path: '',
      group_ids: [],
    })
  })

  it('falls back to the unnamed label when the file stem is empty', async () => {
    vi.mocked(gachaAdminApi.uploadFile).mockResolvedValue({
      data: { data: { image_path: 'cards/y.png', image_url: '' } },
    } as never)
    vi.mocked(gachaAdminApi.createCard).mockResolvedValue({ data: { data: {} } } as never)

    const wrapper = mountDialog()
    await pickFiles(wrapper, [new File(['a'], '.png', { type: 'image/png' })])

    expect(gachaAdminApi.createCard).toHaveBeenCalledWith(
      expect.objectContaining({ name: 'Unnamed' })
    )
  })

  it('marks failed files and re-runs them via the retry button', async () => {
    vi.mocked(gachaAdminApi.uploadFile).mockRejectedValueOnce(new Error('boom'))
    const wrapper = mountDialog()
    await pickFiles(wrapper, [new File(['a'], 'Fail.png', { type: 'image/png' })])

    expect(gachaAdminApi.createCard).not.toHaveBeenCalled()
    const retry = wrapper.find('[data-testid="bulk-retry-btn"]')
    expect(retry.exists()).toBe(true)

    vi.mocked(gachaAdminApi.uploadFile).mockResolvedValue({
      data: { data: { image_path: 'cards/z.png', image_url: '' } },
    } as never)
    vi.mocked(gachaAdminApi.createCard).mockResolvedValue({ data: { data: {} } } as never)
    await retry.trigger('click')
    await flushPromises()

    expect(gachaAdminApi.createCard).toHaveBeenCalledTimes(1)
  })
})
```

- [ ] **Step 3: Run spec to verify it fails**

Run: `cd /tmp/ae-gacha-bulk/frontend/web && bunx vitest run src/components/admin/GachaBulkUpload.spec.ts`
Expected: FAIL — component file does not exist.

- [ ] **Step 4: Implement the component** — `GachaBulkUpload.vue` (repo idiom: explicit props/emits, no `defineModel` — no existing usage in the codebase):

```vue
<template>
  <!-- Bulk cover upload: every picked image becomes a DISABLED draft card
       (rarity N, name = file stem) via the existing upload+create endpoints. -->
  <Modal
    :model-value="modelValue"
    :title="$t('gacha.admin.bulk_upload_title')"
    closable
    @update:model-value="v => emit('update:modelValue', v)"
  >
    <div
      class="border-2 border-dashed border-white/20 rounded-lg p-6 text-center cursor-pointer transition-colors"
      :class="dragOver ? 'border-white/60 bg-white/5' : 'hover:border-white/40'"
      data-testid="bulk-drop-zone"
      @click="fileInput?.click()"
      @dragover.prevent="dragOver = true"
      @dragleave.prevent="dragOver = false"
      @drop.prevent="onDrop"
    >
      <Upload class="size-8 mx-auto mb-2 text-white/40" />
      <p class="text-white/70 text-sm">{{ $t('gacha.admin.bulk_upload_hint') }}</p>
      <input
        ref="fileInput"
        type="file"
        accept="image/*"
        multiple
        class="hidden"
        data-testid="bulk-file-input"
        @change="onPick"
      />
    </div>

    <div v-if="items.length > 0" class="mt-4">
      <p class="text-white/70 text-sm mb-2">
        {{ $t('gacha.admin.bulk_upload_progress', { done: doneCount, total: items.length }) }}
      </p>
      <ul class="max-h-48 overflow-y-auto space-y-1 text-sm">
        <li v-for="(item, i) in items" :key="i" class="flex items-center gap-2">
          <Spinner v-if="item.status === 'uploading'" class="size-3" />
          <span v-else-if="item.status === 'done'" class="text-teal-400">✓</span>
          <span v-else-if="item.status === 'error'" class="text-destructive">✗</span>
          <span v-else class="text-white/40">•</span>
          <span class="truncate text-white/80">{{ item.file.name }}</span>
        </li>
      </ul>
    </div>

    <template #footer>
      <div class="flex justify-end gap-2">
        <Button
          v-if="errorCount > 0 && !running"
          variant="outline"
          data-testid="bulk-retry-btn"
          @click="retryFailed"
        >
          {{ $t('gacha.admin.bulk_upload_retry', { n: errorCount }) }}
        </Button>
        <Button variant="outline" :disabled="running" @click="emit('update:modelValue', false)">
          {{ $t('gacha.admin.bulk_upload_close') }}
        </Button>
      </div>
    </template>
  </Modal>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { Upload } from 'lucide-vue-next'
import { useI18n } from 'vue-i18n'
import { gachaAdminApi } from '@/api/gacha'
import Modal from '@/components/ui/Modal.vue'
import Button from '@/components/ui/Button.vue'
import Spinner from '@/components/ui/Spinner.vue'

const props = defineProps<{ modelValue: boolean }>()
const emit = defineEmits<{ 'update:modelValue': [value: boolean] }>()

const { t } = useI18n()

type ItemStatus = 'pending' | 'uploading' | 'done' | 'error'
interface UploadItem {
  file: File
  status: ItemStatus
}

const items = ref<UploadItem[]>([])
const running = ref(false)
const dragOver = ref(false)
const fileInput = ref<HTMLInputElement | null>(null)

const doneCount = computed(() => items.value.filter(i => i.status === 'done').length)
const errorCount = computed(() => items.value.filter(i => i.status === 'error').length)

// Fresh queue on every reopen.
watch(() => props.modelValue, open => {
  if (open) items.value = []
})

function onPick(e: Event) {
  const input = e.target as HTMLInputElement
  if (input.files) addFiles(Array.from(input.files))
  input.value = ''
}

function onDrop(e: DragEvent) {
  dragOver.value = false
  const files = e.dataTransfer?.files
  if (files) addFiles(Array.from(files).filter(f => f.type.startsWith('image/')))
}

function addFiles(files: File[]) {
  if (files.length === 0) return
  items.value.push(...files.map(file => ({ file, status: 'pending' as ItemStatus })))
  void run()
}

/** Card name = file stem; backend rejects empty names, so fall back. */
function nameFromFile(file: File): string {
  const stem = file.name.replace(/\.[^.]+$/, '').trim()
  return stem || t('gacha.admin.bulk_unnamed')
}

async function processItem(item: UploadItem) {
  // Status flips to 'uploading' synchronously — that is the claim that stops
  // a sibling worker from picking the same item (single-threaded JS: no await
  // between a worker's find() and this line).
  item.status = 'uploading'
  try {
    const res = await gachaAdminApi.uploadFile(item.file, 'cards')
    const data = (res as { data?: { data?: { image_path?: string } } }).data
    const imagePath = data?.data?.image_path ?? ''
    if (!imagePath) throw new Error('empty image_path')
    await gachaAdminApi.createCard({
      name: nameFromFile(item.file),
      source_title: '',
      rarity: 'N',
      enabled: false,
      image_path: imagePath,
      back_path: '',
      group_ids: [],
    })
    item.status = 'done'
  } catch {
    item.status = 'error'
  }
}

/** Drain all pending items with at most 3 concurrent workers. */
async function run() {
  if (running.value) return
  running.value = true
  try {
    const workers = Array.from({ length: 3 }, async () => {
      for (;;) {
        const next = items.value.find(i => i.status === 'pending')
        if (!next) return
        await processItem(next)
      }
    })
    await Promise.all(workers)
  } finally {
    running.value = false
  }
}

function retryFailed() {
  for (const item of items.value) {
    if (item.status === 'error') item.status = 'pending'
  }
  void run()
}
</script>
```

Note: `createCard` payload type — check the existing `gachaAdminApi.createCard` signature in `gacha.ts` and match it exactly (AdminGacha.vue `saveCard` builds the same shape).

- [ ] **Step 5: Run spec to verify it passes**

Run: `cd /tmp/ae-gacha-bulk/frontend/web && bunx vitest run src/components/admin/GachaBulkUpload.spec.ts`
Expected: 3 tests PASS. If Modal teleport breaks rendering, keep `stubs: { teleport: true }` and check how `AdminGacha.spec.ts` mounts dialogs for the working idiom.

- [ ] **Step 6: Commit**

```bash
cd /tmp/ae-gacha-bulk
git commit -m "feat(web): gacha bulk cover upload dialog — files → disabled draft cards" \
  -m "Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>" \
  -- frontend/web/src/components/admin/GachaBulkUpload.vue frontend/web/src/components/admin/GachaBulkUpload.spec.ts frontend/web/src/locales/en.json frontend/web/src/locales/ru.json frontend/web/src/locales/ja.json
```

---

### Task 5: Frontend — Cards table: selection, bulk bar, inline editing, wiring

**Files:**
- Modify: `frontend/web/src/views/admin/AdminGacha.vue`
- Modify: `frontend/web/src/locales/en.json`, `ru.json`, `ja.json`
- Test: `frontend/web/src/views/admin/AdminGacha.spec.ts` (append)

**Interfaces:**
- Consumes: `gachaAdminApi.bulkUpdateCards/bulkDeleteCards` + `BulkCardSet` type (Task 3), `GachaBulkUpload.vue` (Task 4), existing `deleteTarget`/`runDelete`/`loadCards`/`extractMessage` machinery in AdminGacha.vue.
- Produces: final user-facing feature; nothing downstream.

- [ ] **Step 1: Add i18n keys** — `gacha.admin` in ALL THREE locales:

en.json:
```json
"bulk_selected": "Selected: {n}",
"bulk_set_rarity": "Set rarity…",
"bulk_set_name_placeholder": "New name…",
"bulk_set_source_placeholder": "New source title…",
"bulk_apply": "Apply",
"bulk_add_to_group": "Add to group…",
"bulk_enable": "Enable",
"bulk_disable": "Disable",
"bulk_delete": "Delete",
"bulk_delete_label": "Bulk delete",
"bulk_delete_confirm": "Delete {n} selected cards?",
"draft_badge": "draft"
```

ru.json:
```json
"bulk_selected": "Выбрано: {n}",
"bulk_set_rarity": "Редкость…",
"bulk_set_name_placeholder": "Новое имя…",
"bulk_set_source_placeholder": "Новый источник…",
"bulk_apply": "Применить",
"bulk_add_to_group": "В группу…",
"bulk_enable": "Включить",
"bulk_disable": "Выключить",
"bulk_delete": "Удалить",
"bulk_delete_label": "Массовое удаление",
"bulk_delete_confirm": "Удалить выбранные карточки: {n}?",
"draft_badge": "черновик"
```

ja.json:
```json
"bulk_selected": "選択中: {n}",
"bulk_set_rarity": "レアリティ…",
"bulk_set_name_placeholder": "新しい名前…",
"bulk_set_source_placeholder": "新しい出典…",
"bulk_apply": "適用",
"bulk_add_to_group": "グループへ…",
"bulk_enable": "有効化",
"bulk_disable": "無効化",
"bulk_delete": "削除",
"bulk_delete_label": "一括削除",
"bulk_delete_confirm": "選択した{n}枚のカードを削除しますか？",
"draft_badge": "下書き"
```

- [ ] **Step 2: Script additions** in `AdminGacha.vue` `<script setup>` (place after the Cards-state block; import `GachaBulkUpload` next to the `GachaCardPicker` import, add `Upload` to the existing lucide import, and add `BulkCardSet` to the `@/api/gacha` type imports):

```ts
// ── Bulk upload dialog ────────────────────────────────────────────────────────
const showBulkUpload = ref(false)
// Refetch on close — the dialog created draft cards behind the table's back.
watch(showBulkUpload, open => {
  if (!open) void loadCards()
})

// ── Bulk selection + actions ──────────────────────────────────────────────────
const selectedIds = ref<Set<string>>(new Set())
const bulkBusy = ref(false)
const bulkName = ref('')
const bulkSource = ref('')
const bulkRarity = ref('')
const bulkGroup = ref('')

const allSelected = computed(() =>
  filteredCards.value.length > 0 && filteredCards.value.every(c => selectedIds.value.has(c.id)))

function toggleSelect(id: string) {
  const next = new Set(selectedIds.value)
  if (next.has(id)) next.delete(id)
  else next.add(id)
  selectedIds.value = next
}

function toggleSelectAll() {
  selectedIds.value = allSelected.value ? new Set() : new Set(filteredCards.value.map(c => c.id))
}

// Selection references filtered rows; changing filters invalidates it.
watch([cardFilterRarity, cardFilterGroup, cardFilterEnabled], () => {
  selectedIds.value = new Set()
})

async function applyBulk(set: BulkCardSet) {
  if (selectedIds.value.size === 0) return
  bulkBusy.value = true
  pageError.value = null
  try {
    await gachaAdminApi.bulkUpdateCards(Array.from(selectedIds.value), set)
    bulkName.value = ''
    bulkSource.value = ''
    await loadCards()
  } catch (err: unknown) {
    pageError.value = extractMessage(err)
  } finally {
    bulkBusy.value = false
  }
}

// Selects apply immediately on pick, then reset to '' so the placeholder returns.
watch(bulkRarity, v => {
  if (!v) return
  void applyBulk({ rarity: v as Rarity }).finally(() => { bulkRarity.value = '' })
})

watch(bulkGroup, v => {
  if (!v || selectedIds.value.size === 0) {
    if (v) bulkGroup.value = ''
    return
  }
  bulkBusy.value = true
  gachaAdminApi.addCardsToGroup(v, Array.from(selectedIds.value))
    .catch((err: unknown) => { pageError.value = extractMessage(err) })
    .finally(() => { bulkBusy.value = false; bulkGroup.value = '' })
})

const bulkGroupOptions = computed(() => groups.value.map(g => ({ value: g.id, label: g.name })))
const rarityEditOptions = [
  { value: 'N', label: 'N' },
  { value: 'R', label: 'R' },
  { value: 'SR', label: 'SR' },
  { value: 'SSR', label: 'SSR' },
]

function confirmBulkDelete() {
  const ids = Array.from(selectedIds.value)
  if (ids.length === 0) return
  deleteTarget.value = {
    label: t('gacha.admin.bulk_delete_label'),
    confirmMsg: t('gacha.admin.bulk_delete_confirm', { n: ids.length }),
    action: async () => {
      await gachaAdminApi.bulkDeleteCards(ids)
      selectedIds.value = new Set()
      await loadCards()
    },
  }
  showDeleteDialog.value = true
}

// ── Inline cell editing ───────────────────────────────────────────────────────
// Single-cell edits go through the bulk endpoint with one id: partial
// semantics — unlike updateCard (full replace), nothing else can be clobbered.
const inlineEdit = ref<{ id: string; field: 'name' | 'source_title' } | null>(null)
const inlineValue = ref('')

function isEditing(id: string, field: 'name' | 'source_title') {
  return inlineEdit.value?.id === id && inlineEdit.value?.field === field
}

function startInlineEdit(card: GachaCard, field: 'name' | 'source_title') {
  inlineEdit.value = { id: card.id, field }
  inlineValue.value = field === 'name' ? card.name : card.source_title
}

function cancelInlineEdit() {
  inlineEdit.value = null
}

async function commitInlineEdit() {
  const edit = inlineEdit.value
  if (!edit) return
  inlineEdit.value = null
  const card = cards.value.find(c => c.id === edit.id)
  if (!card) return
  const value = inlineValue.value.trim()
  if (edit.field === 'name' && !value) return // backend rejects empty names
  if (value === (edit.field === 'name' ? card.name : card.source_title)) return
  try {
    await gachaAdminApi.bulkUpdateCards([card.id], { [edit.field]: value })
    if (edit.field === 'name') card.name = value
    else card.source_title = value
  } catch (err: unknown) {
    pageError.value = extractMessage(err)
  }
}

async function onInlineRarity(card: GachaCard, rarity: Rarity) {
  if (rarity === card.rarity) return
  try {
    await gachaAdminApi.bulkUpdateCards([card.id], { rarity })
    card.rarity = rarity
  } catch (err: unknown) {
    pageError.value = extractMessage(err)
  }
}

async function onInlineEnabled(card: GachaCard, enabled: boolean) {
  try {
    await gachaAdminApi.bulkUpdateCards([card.id], { enabled })
    card.enabled = enabled
  } catch (err: unknown) {
    pageError.value = extractMessage(err)
  }
}
```

- [ ] **Step 3: Template changes** in the Cards tab:

3a. Header buttons — replace the single create Button with:

```html
            <div class="flex gap-2">
              <Button size="sm" variant="outline" data-testid="bulk-upload-btn" @click="showBulkUpload = true">
                <Upload class="size-4 mr-1" />
                {{ $t('gacha.admin.bulk_upload_btn') }}
              </Button>
              <Button size="sm" @click="openCardCreate">
                + {{ $t('gacha.admin.card_create') }}
              </Button>
            </div>
```

3b. Bulk actions bar — insert between the filter row and the loading `v-if`:

```html
          <!-- Bulk actions bar (visible while a selection exists) -->
          <div
            v-if="selectedIds.size > 0"
            class="glass-card flex flex-wrap items-center gap-2 px-3 py-2 mb-3"
            data-testid="bulk-actions-bar"
          >
            <span class="text-sm text-white/80 font-medium">
              {{ $t('gacha.admin.bulk_selected', { n: selectedIds.size }) }}
            </span>
            <Select
              v-model="bulkRarity"
              :options="rarityEditOptions"
              :placeholder="$t('gacha.admin.bulk_set_rarity')"
              size="sm"
              class="w-28"
              data-testid="bulk-rarity-select"
            />
            <div class="flex items-center gap-1">
              <Input v-model="bulkName" size="sm" :placeholder="$t('gacha.admin.bulk_set_name_placeholder')" class="w-36" />
              <Button size="sm" variant="outline" :disabled="!bulkName.trim() || bulkBusy" data-testid="bulk-name-apply" @click="applyBulk({ name: bulkName.trim() })">
                {{ $t('gacha.admin.bulk_apply') }}
              </Button>
            </div>
            <div class="flex items-center gap-1">
              <Input v-model="bulkSource" size="sm" :placeholder="$t('gacha.admin.bulk_set_source_placeholder')" class="w-36" />
              <Button size="sm" variant="outline" :disabled="!bulkSource.trim() || bulkBusy" data-testid="bulk-source-apply" @click="applyBulk({ source_title: bulkSource.trim() })">
                {{ $t('gacha.admin.bulk_apply') }}
              </Button>
            </div>
            <Select
              v-model="bulkGroup"
              :options="bulkGroupOptions"
              :placeholder="$t('gacha.admin.bulk_add_to_group')"
              size="sm"
              class="w-36"
              data-testid="bulk-group-select"
            />
            <Button size="sm" variant="outline" :disabled="bulkBusy" data-testid="bulk-enable-btn" @click="applyBulk({ enabled: true })">
              {{ $t('gacha.admin.bulk_enable') }}
            </Button>
            <Button size="sm" variant="outline" :disabled="bulkBusy" data-testid="bulk-disable-btn" @click="applyBulk({ enabled: false })">
              {{ $t('gacha.admin.bulk_disable') }}
            </Button>
            <Button size="sm" variant="destructive" :disabled="bulkBusy" data-testid="bulk-delete-btn" @click="confirmBulkDelete">
              {{ $t('gacha.admin.bulk_delete') }}
            </Button>
          </div>
```

3c. Table head — add a checkbox column as the FIRST `<th>`:

```html
                  <th class="px-3 py-2 w-8">
                    <Checkbox :model-value="allSelected" data-testid="select-all" @update:model-value="toggleSelectAll" />
                  </th>
```

3d. Table row — add the row checkbox as the FIRST `<td>`:

```html
                  <td class="px-3 py-2">
                    <Checkbox :model-value="selectedIds.has(card.id)" :data-testid="`row-select-${card.id}`" @update:model-value="toggleSelect(card.id)" />
                  </td>
```

3e. Name cell — replace `<td class="px-3 py-2 font-medium">{{ card.name }}</td>` with inline editing + draft badge (Input primitive, NOT a bare `<input>` — DS-lint Rule 5):

```html
                  <td class="px-3 py-2 font-medium cursor-pointer" @click="startInlineEdit(card, 'name')">
                    <Input
                      v-if="isEditing(card.id, 'name')"
                      v-model="inlineValue"
                      size="sm"
                      autofocus
                      @keyup.enter="commitInlineEdit"
                      @keyup.esc="cancelInlineEdit"
                      @blur="commitInlineEdit"
                      @click.stop
                    />
                    <template v-else>
                      {{ card.name }}
                      <span
                        v-if="!card.enabled"
                        class="ml-1.5 text-[10px] uppercase tracking-wide text-white/50 border border-white/20 rounded px-1 py-px align-middle"
                      >
                        {{ $t('gacha.admin.draft_badge') }}
                      </span>
                    </template>
                  </td>
```

(If the `Input` primitive has no `autofocus` prop, drop the attribute — it falls through to the native input anyway.)

3f. Source cell — same pattern for `source_title`:

```html
                  <td class="px-3 py-2 text-white/60 text-xs cursor-pointer" @click="startInlineEdit(card, 'source_title')">
                    <Input
                      v-if="isEditing(card.id, 'source_title')"
                      v-model="inlineValue"
                      size="sm"
                      autofocus
                      @keyup.enter="commitInlineEdit"
                      @keyup.esc="cancelInlineEdit"
                      @blur="commitInlineEdit"
                      @click.stop
                    />
                    <template v-else>{{ card.source_title }}</template>
                  </td>
```

3g. Rarity cell — replace the badge `<span>` with an inline Select (keep `rarityBadgeClass` helper — it stays in use if referenced elsewhere; delete it only if this was the last usage and the linter flags it):

```html
                  <td class="px-3 py-2">
                    <Select
                      :model-value="card.rarity"
                      :options="rarityEditOptions"
                      size="sm"
                      class="w-20"
                      @update:model-value="v => onInlineRarity(card, v as Rarity)"
                    />
                  </td>
```

3h. Enabled cell — replace the ✓/– `<span>` with a Checkbox:

```html
                  <td class="px-3 py-2 text-center">
                    <Checkbox :model-value="card.enabled" @update:model-value="v => onInlineEnabled(card, v)" />
                  </td>
```

3i. Mount the dialog — at the end of the template, next to the other modals:

```html
  <!-- ─── BULK UPLOAD DIALOG ────────────────────────────────────────────── -->
  <GachaBulkUpload v-model="showBulkUpload" />
```

- [ ] **Step 4: Extend `AdminGacha.spec.ts`** — add `bulkUpdateCards: vi.fn(),` and `bulkDeleteCards: vi.fn(),` to the `gachaAdminApi` mock object, stub `GachaBulkUpload` (add `GachaBulkUpload: { template: '<div />' }` to the mount `stubs` if dialogs are stubbed there; otherwise it renders inert), then append tests:

```ts
  it('row checkbox selection shows the bulk bar and Enable calls bulkUpdateCards', async () => {
    vi.mocked(gachaAdminApi.listCards).mockResolvedValue({
      data: { data: [makeCard({ id: 'c1' }), makeCard({ id: 'c2', name: 'Second' })] },
    } as never)
    vi.mocked(gachaAdminApi.bulkUpdateCards).mockResolvedValue({ data: { data: { updated: 1 } } } as never)
    const wrapper = mountPage()          // ← use this spec's existing mount helper name
    await flushPromises()

    expect(wrapper.find('[data-testid="bulk-actions-bar"]').exists()).toBe(false)
    await wrapper.find('[data-testid="row-select-c1"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="bulk-actions-bar"]').exists()).toBe(true)

    await wrapper.find('[data-testid="bulk-enable-btn"]').trigger('click')
    await flushPromises()
    expect(gachaAdminApi.bulkUpdateCards).toHaveBeenCalledWith(['c1'], { enabled: true })
  })

  it('select-all selects every filtered card', async () => {
    vi.mocked(gachaAdminApi.listCards).mockResolvedValue({
      data: { data: [makeCard({ id: 'c1' }), makeCard({ id: 'c2', name: 'Second' })] },
    } as never)
    vi.mocked(gachaAdminApi.bulkUpdateCards).mockResolvedValue({ data: { data: { updated: 2 } } } as never)
    const wrapper = mountPage()
    await flushPromises()

    await wrapper.find('[data-testid="select-all"]').trigger('click')
    await flushPromises()
    await wrapper.find('[data-testid="bulk-disable-btn"]').trigger('click')
    await flushPromises()
    const [ids, set] = vi.mocked(gachaAdminApi.bulkUpdateCards).mock.calls[0]
    expect([...ids].sort()).toEqual(['c1', 'c2'])
    expect(set).toEqual({ enabled: false })
  })

  it('bulk delete goes through the confirm dialog then calls bulkDeleteCards', async () => {
    vi.mocked(gachaAdminApi.listCards).mockResolvedValue({
      data: { data: [makeCard({ id: 'c1' })] },
    } as never)
    vi.mocked(gachaAdminApi.bulkDeleteCards).mockResolvedValue({ data: { data: { deleted: 1 } } } as never)
    const wrapper = mountPage()
    await flushPromises()

    await wrapper.find('[data-testid="row-select-c1"]').trigger('click')
    await wrapper.find('[data-testid="bulk-delete-btn"]').trigger('click')
    await flushPromises()
    expect(gachaAdminApi.bulkDeleteCards).not.toHaveBeenCalled()  // confirm first

    const vm = wrapper.vm as unknown as { runDelete: () => Promise<void> }
    await vm.runDelete()
    expect(gachaAdminApi.bulkDeleteCards).toHaveBeenCalledWith(['c1'])
  })
```

Adapt selector interaction to the Checkbox primitive: if `.trigger('click')` on the wrapper element doesn't flip it, find the inner control the way existing specs in the repo interact with `Checkbox` (check `Checkbox.vue` and any spec using it; worst case call the handler via emitted `update:model-value`).

- [ ] **Step 5: Run the spec + full frontend gates**

Run: `cd /tmp/ae-gacha-bulk/frontend/web && bunx vitest run src/views/admin/AdminGacha.spec.ts src/components/admin/GachaBulkUpload.spec.ts`
Expected: all PASS (existing tests must not regress).
Run: `bunx vue-tsc --noEmit` → clean. (`vue-tsc`, not `tsc` — plain tsc false-passes SFCs.)

- [ ] **Step 6: Commit**

```bash
cd /tmp/ae-gacha-bulk
git commit -m "feat(web): gacha cards table — selection, bulk actions bar, inline editing, draft badge" \
  -m "Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>" \
  -- frontend/web/src/views/admin/AdminGacha.vue frontend/web/src/views/admin/AdminGacha.spec.ts frontend/web/src/locales/en.json frontend/web/src/locales/ru.json frontend/web/src/locales/ja.json
```

---

### Task 6: Full verification gates

**Files:** none (verification only).

- [ ] **Step 1: Backend** — `cd /tmp/ae-gacha-bulk/services/gacha && go test ./...` → all `ok`.
- [ ] **Step 2: Frontend unit** — `cd /tmp/ae-gacha-bulk/frontend/web && bunx vitest run` → all PASS.
- [ ] **Step 3: Types** — `bunx vue-tsc --noEmit` → clean.
- [ ] **Step 4: DS-lint** — `bash scripts/design-system-lint.sh` (from `frontend/web/`) → `ERRORS=0`.
- [ ] **Step 5: i18n parity** — `cd /tmp/ae-gacha-bulk && make i18n-lint` (or the underlying script if make target unavailable in worktree) → en/ru/ja all pass.
- [ ] **Step 6: Build** — `cd frontend/web && bun run build` → success.
- [ ] **Step 7: Report** any failure verbatim; do NOT mark done until all six are green.

---

## After the plan (main session, not a task)

1. `/frontend-verify` formal pass, then pull-rebase-push to `main`.
2. `/animeenigma-after-update` — redeploys `gacha` + `web`, health checks, Trump-mode changelog, final commit/push.
3. `git worktree remove /tmp/ae-gacha-bulk && git worktree prune` once green.
