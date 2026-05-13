# Phase 12 Plan: AdminRecs SPA quality

**Status:** Active
**Plan #:** 1
**Created:** 2026-05-13

Scope: 2 view files + 1 composable + 3 locale files. Closes UA-090, UA-091, UA-092, UA-093, UA-094, UA-095, UA-096, UA-097, UA-098, UA-100, UA-101.

## Tasks

### AdminRecs.vue table a11y (UX-25)

- [ ] Add `<caption class="sr-only">{{ $t('admin.recs.tableCaption') }}</caption>` as first child of `<table>`. Add `:aria-label="$t('admin.recs.tableCaption')"` on `<table>`. Closes **UA-094**.
- [ ] Add `scope="col"` to each `<th>` in the thead. Closes **UA-094**.
- [ ] Each clickable `<tr>` gains `tabindex="0"`, `role="button"`, `:aria-expanded="expandedRowIds.has(row.rank)"`, `:aria-controls="`detail-${row.rank}`"`, `@keydown.enter.prevent="toggleRow(row.rank)"`, `@keydown.space.prevent="toggleRow(row.rank)"`, and `focus:outline-none focus-visible:ring-2 focus-visible:ring-cyan-400 focus-visible:ring-inset`. Closes **UA-095**.
- [ ] Each detail `<tr>` gets `:id="`detail-${row.rank}`"` for aria-controls linkage.
- [ ] Add `:title="$t('admin.recs.s1Title')"` etc. on S1-S5 column `<th>`s (5 keys). Closes **UA-093**.

### AdminRecsPicker.vue a11y + UX (UX-26)

- [ ] Search input: `ref="searchInputRef"` and `onMounted(() => searchInputRef.value?.focus())` to autofocus. Add `focus-visible:ring-2 focus-visible:ring-cyan-400` ring. Closes **UA-091**, **UA-101**.
- [ ] Container gets `role="listbox"` + `:aria-label="$t('admin.recs.picker.listboxLabel')"`. Each result row gets `role="option"`, `tabindex="0"`, focus-visible ring.
- [ ] Loading indicator: small spinner inside (or adjacent to) the search input when `isSearching=true`. Closes **UA-092**.
- [ ] Filter out (or mark "Вы") the current admin from results: read `authStore.user.id` in `<script setup>`, in the result loop add a "Вы (You)" badge if `result.id === authStore.user.id`. Closes **UA-090**.
- [ ] Empty state: when search returns 0 hits, render `<p>{{ $t('admin.recs.picker.empty') }}</p>` below the input. Closes **UA-097**.
- [ ] Mobile horizontal-scroll hint on AdminRecs.vue table wrapper: `<div class="md:hidden absolute right-0 top-0 bottom-0 w-8 bg-gradient-to-l from-black/40 pointer-events-none" />` inside the scroll container. Closes **UA-098**.

### Admin guard + error mapping (UX-26 cont'd)

- [ ] Router or `AdminRecs.vue` onMounted: if user is not admin, show toast `admin.errors.notAdmin` BEFORE router push to home. Use existing toast helper. Closes **UA-100**.
- [ ] `useAdminRecs.ts`: extend error catch block to map HTTP status to friendly i18n keys (401 → `admin.errors.unauthorized`, 403 → `admin.errors.forbidden`, 500 → `admin.errors.serverError`, timeout/abort → `admin.errors.timeout`). Closes **UA-096**.

### i18n (en/ru/ja)

- [ ] Add to each locale file:
  - `admin.recs.tableCaption`
  - `admin.recs.s1Title`, `s2Title`, `s3Title`, `s4Title`, `s5Title`
  - `admin.recs.picker.listboxLabel`
  - `admin.recs.picker.empty`
  - `admin.recs.picker.youBadge`
  - `admin.errors.notAdmin`, `unauthorized`, `forbidden`, `serverError`, `timeout`
  - Total: 13 keys × 3 locales = 39 entries.

  Strings:
  - tableCaption: EN "Recommendation candidates for user, ranked by final score" / RU "Кандидаты рекомендаций пользователя, отсортированные по итоговому баллу" / JA "ユーザーに対する推薦候補（最終スコア順）"
  - s1Title: EN "Top-list similarity" / RU "Сходство с топ-листом" / JA "トップリスト類似度"
  - s2Title: EN "Genre affinity" / RU "Жанровое соответствие" / JA "ジャンル親和性"
  - s3Title: EN "Global trending" / RU "Глобальный тренд" / JA "グローバルトレンド"
  - s4Title: EN "Rating boost" / RU "Рейтинговый буст" / JA "評価ブースト"
  - s5Title: EN "Seasonal freshness" / RU "Сезонная новизна" / JA "季節の新鮮さ"
  - picker.listboxLabel: EN "User search results" / RU "Результаты поиска пользователей" / JA "ユーザー検索結果"
  - picker.empty: EN "No users found" / RU "Пользователи не найдены" / JA "ユーザーが見つかりません"
  - picker.youBadge: EN "You" / RU "Вы" / JA "あなた"
  - errors.notAdmin: EN "Admin access required" / RU "Требуется доступ администратора" / JA "管理者権限が必要です"
  - errors.unauthorized: EN "Please sign in" / RU "Пожалуйста, войдите" / JA "サインインしてください"
  - errors.forbidden: EN "You don't have permission" / RU "Нет доступа" / JA "アクセス権限がありません"
  - errors.serverError: EN "Server error, try again" / RU "Ошибка сервера, попробуйте снова" / JA "サーバーエラー、再試行してください"
  - errors.timeout: EN "Request timed out" / RU "Превышено время ожидания" / JA "リクエストがタイムアウトしました"

### Verification

- [ ] `cd frontend/web && bunx vue-tsc --noEmit` clean.
- [ ] `bash scripts/i18n-lint.sh` clean (39 new keys).
- [ ] `make redeploy-web` succeeds.
- [ ] `grep -n 'role="button"\|aria-expanded\|@keydown.enter\|@keydown.space' frontend/web/src/views/admin/AdminRecs.vue` confirms 4+ attribute matches.
- [ ] `grep -n 'role="listbox"\|role="option"\|aria-label' frontend/web/src/views/admin/AdminRecsPicker.vue` confirms 2+ matches.
- [ ] `grep -n 'admin.errors\.' frontend/web/src/composables/useAdminRecs.ts` confirms error mapping.
- [ ] Manual smoke: load `/admin/recs` as `ui_audit_bot`. Tab focuses through rows, Enter expands, ESC collapses. Picker accepts a user search.

## Files touched

```
frontend/web/src/views/admin/AdminRecs.vue           (a11y + scroll hint)
frontend/web/src/views/admin/AdminRecsPicker.vue     (listbox + autofocus + empty + youBadge)
frontend/web/src/composables/useAdminRecs.ts         (error mapping)
frontend/web/src/locales/en.json                     (+13 keys)
frontend/web/src/locales/ru.json                     (+13 keys)
frontend/web/src/locales/ja.json                     (+13 keys)
.planning/workstreams/ui-ux-audit/phases/12-adminrecs-quality/
  12-CONTEXT.md
  12-PLAN.md
  12-SUMMARY.md       (written at execute end)
  12-VERIFICATION.md  (written at execute end)
```

## Closes

| Finding | Surface | Mechanism |
|---|---|---|
| UA-090 | AdminRecsPicker | Self-row marked with "Вы / You" badge |
| UA-091 | AdminRecsPicker | Autofocus search input on mount |
| UA-092 | AdminRecsPicker | Spinner during search request |
| UA-093 | AdminRecs table | `title=` tooltips on S1-S5 column headers |
| UA-094 | AdminRecs table | `<caption>` + aria-label + scope="col" |
| UA-095 | AdminRecs table | Expandable row keyboard handlers + aria-expanded + aria-controls |
| UA-096 | useAdminRecs.ts | HTTP status → i18n error keys |
| UA-097 | AdminRecsPicker | Empty-state help text |
| UA-098 | AdminRecs table | Mobile scroll-fade affordance |
| UA-100 | Admin guard | Toast before redirect |
| UA-101 | AdminRecsPicker | Focus rings on input + results |
