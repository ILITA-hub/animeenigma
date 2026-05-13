# Phase 10 Plan: Recommendations polish — reasoning chip + Top-10 visual

**Status:** Active
**Plan #:** 1
**Created:** 2026-05-13

Scope: frontend-only polish on `frontend/web/src/views/Home.vue` (trending row + Top row) + 3 locale files. Closes UX-19, UX-20. Verifies UA-060.

## Tasks

### Reasoning chip (UX-19)

- [ ] In `Home.vue` `<script setup>`, compute `dominantSignalKey` from `trendingRecs`:
  ```ts
  const dominantSignalKey = computed(() => {
    const counts = trendingRecs.value
      .filter(r => !r.pinned && r.top_contributor)
      .reduce<Record<string, number>>((acc, r) => {
        const k = r.top_contributor!
        acc[k] = (acc[k] || 0) + 1
        return acc
      }, {})
    const top = Object.entries(counts).sort((a, b) => b[1] - a[1])[0]
    return top ? top[0] : null
  })
  ```
- [ ] Add `reasonI18nKey` computed: `dominantSignalKey.value ? `recs.reason.${dominantSignalKey.value}` : null`.
- [ ] Render chip BELOW the row label (`{{ t(rowLabelKey) }}` at line 34) and ABOVE pin-reason line. Use:
  ```html
  <p v-if="reasonI18nKey && !trendingRecs[0]?.pinned"
     class="text-xs text-cyan-400/80 mb-2">
    {{ t(reasonI18nKey) }}
  </p>
  ```
  Hidden when first item is pinned (existing pin_reason renders for that case).
- [ ] Add `recs.reason.s1`..`recs.reason.s5` keys to en.json, ru.json, ja.json (5 keys × 3 locales = 15 entries) per CONTEXT.md.

### Top-10 numeral visual (UX-20)

- [ ] Locate Top row in Home.vue. The `v-for="anime in topAnime"` block (~line 239+). Identify the outermost `<router-link>` or wrapping div for each item.
- [ ] Wrap each item in a `relative pl-12 md:pl-16` parent (or add classes to existing). Add immediately inside the wrapper:
  ```html
  <span
    v-if="index < 10"
    class="absolute -left-2 md:-left-4 top-1/2 -translate-y-1/2 text-[80px] md:text-[120px] lg:text-[160px] font-black leading-none text-cyan-400/10 z-0 pointer-events-none select-none"
    aria-hidden="true"
  >{{ index + 1 }}</span>
  ```
  Ensure the existing poster wrapper has `relative z-10` so it sits above the numeral.
- [ ] `topAnime.length > 10`: items beyond 10 render without numeral (the `v-if="index < 10"` guard handles this).

### Verification

- [ ] `cd frontend/web && bunx vue-tsc --noEmit` clean.
- [ ] `bash scripts/i18n-lint.sh` PASS (3 locales × 5 new keys; no collisions).
- [ ] `make redeploy-web` succeeds.
- [ ] `grep -n "recs.reason.s" frontend/web/src/locales/en.json` returns 5 lines.
- [ ] `grep -n "reasonI18nKey\|dominantSignalKey" frontend/web/src/views/Home.vue` returns at least 2 matches.
- [ ] `grep -n "index + 1\|index < 10" frontend/web/src/views/Home.vue` confirms numeral block.
- [ ] Manual: signed in as `ui_audit_bot`, load `/`. Top row shows 1, 2, 3 numerals behind first three posters. Reasoning chip renders below trending row label (only when not pinned).

## Files touched

```
frontend/web/src/views/Home.vue                (chip + dominant signal + Top numeral)
frontend/web/src/locales/en.json               (+5 recs.reason.* keys)
frontend/web/src/locales/ru.json               (+5 recs.reason.* keys)
frontend/web/src/locales/ja.json               (+5 recs.reason.* keys)
.planning/workstreams/ui-ux-audit/phases/10-recs-polish/
  10-CONTEXT.md
  10-PLAN.md
  10-SUMMARY.md       (written at execute end)
  10-VERIFICATION.md  (written at execute end)
```

## Closes

| Req | Surface | Mechanism |
|---|---|---|
| UX-19 | Home trending row | Per-row dominant-signal chip via `top_contributor` mapping → `recs.reason.s{N}` |
| UA-060 | Home trending row | Validates `top_contributor` is consumed by frontend (rendered as chip) |
| UX-20 | Home Top row | `index + 1` giant cyan-400/10 numeral behind each of the first 10 posters |
