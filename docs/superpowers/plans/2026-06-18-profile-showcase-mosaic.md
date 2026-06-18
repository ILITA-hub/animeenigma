# Profile Showcase Mosaic — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Turn the flat vertical showcase stack into a user-arrangeable bento mosaic with per-block resize, drag-to-swap (size exchange), per-variant size limits, and showcase-as-first-tab.

**Architecture:** Backend `services/player` stays a pure config store — `Block` gains `w`/`h`, validated/clamped against a per-(type,variant) size table. Frontend `ProfileShowcase.vue` renders a CSS-grid bento (`grid-auto-flow:dense`) driven by `w`/`h`; `ShowcaseEditor.vue` renders that same grid in edit mode with a corner resize handle, drag-to-swap, an add-block picker, and a per-block config modal. `Profile.vue` moves the showcase into the tab bar as the first tab.

**Tech Stack:** Go (player service, GORM, libs/errors) · Vue 3 + TypeScript + Tailwind v4 · vuedraggable (existing) · Vitest + Playwright · i18n (en/ru/ja).

**Reference implementation:** the approved interactive mock `.brainstorm/content/profile-prod-v1.html` is a working vanilla-JS reference for every editor interaction (resize pointer math in `wire()`, `swapTiles`, `con`/`VC` size table, `renderBody` per-variant layouts, add picker, config modal). Port its logic; don't reinvent.

**Design spec:** `docs/superpowers/specs/2026-06-18-profile-showcase-mosaic-design.md` (§7 = size matrix, authoritative).

## Global Constraints

- **No time units.** Any scoring uses UXΔ / CDI / MVQ (`.planning/CONVENTIONS.md`). This feature: `UXΔ = +3 (Better)`, `CDI = 0.05 * 21`, `MVQ = Griffin 85%/80%`.
- **No new frontend dependency.** Reuse vuedraggable + a small custom pointer handler for resize.
- **DS-lint (build-enforced).** Grid spans must be **static class strings** (Rule 1). Inline `style` may carry only layout (`grid-column`/`grid-row`/`grid-auto-rows`), never color (Rule 8). No off-palette colors, no arbitrary spacing outside the 4px scale (Rule 6). Run `bash frontend/web/scripts/design-system-lint.sh` clean.
- **i18n parity.** Every new key added to ALL THREE locales `en.json`/`ru.json`/`ja.json`; `frontend/web/scripts/i18n-lint.sh` is a hard `make redeploy-web` gate.
- **TS↔Go parity.** `VARIANT_SIZE` (TS) ↔ `VariantSizeAllowlist` (Go) and `SHOWCASE_VARIANTS` ↔ `VariantAllowlist` must match exactly. A divergence test fails CI.
- **Limits:** `MaxBlocks = 12`, `MaxBlockItems = 12` (unchanged). Width ∈ [1,4], Height ∈ [1,3].
- **Backend leniency:** out-of-range / absent `w`/`h` are **clamped / defaulted**, never 400 (a stale client must always be able to save).
- **Dark-ship:** stays behind `VITE_PROFILE_WALL_ADMIN_ONLY`.
- **Deploy from a CLEAN `origin/main` worktree** (copy `docker/.env`, compose project stays `docker`); never `make redeploy-*` from the shared dirty tree. Commit path-scoped (`git commit <pathspec>`), then push; if push is rejected, land on `main` via a worktree cherry-pick.

---

## File Structure

- `services/player/internal/domain/showcase.go` — `Block.Width/Height`; `SizeBound` + `VariantSizeAllowlist`; `clampBlockSize`; `ValidateBlocks` extended. (+ `showcase_test.go`)
- `frontend/web/src/types/showcase.ts` — `w`/`h` on `ShowcaseBlock`; `SizeBound`; `VARIANT_SIZE`; `sizeFor`; `spanClasses`. (+ `src/types/__tests__/showcase-size.spec.ts`)
- `frontend/web/src/components/profile/showcase/ProfileShowcase.vue` — bento grid + span mapping + `@loaded(count)`. (+ existing spec updated)
- `frontend/web/src/components/profile/showcase/ShowcaseEditor.vue` — grid render, resize handle, drag-swap with size exchange, add-block picker, per-block config modal. (+ existing spec updated)
- `frontend/web/src/components/profile/showcase/ShowcaseConfigDialog.vue` — NEW: per-block config modal (variant + content). (+ spec)
- `frontend/web/src/views/Profile.vue` — showcase as first tab + default-tab logic.
- `frontend/web/src/locales/{en,ru,ja}.json` — new keys.

---

### Task 1: Backend — Block size fields, allowlist, validation

**Files:**
- Modify: `services/player/internal/domain/showcase.go`
- Test: `services/player/internal/domain/showcase_test.go`

**Interfaces:**
- Produces: `Block.Width int`, `Block.Height int` (json `w`/`h`); `type SizeBound struct{ MinW, MaxW, MinH, MaxH, DefW, DefH int }`; `var VariantSizeAllowlist map[string]map[string]SizeBound`; `func SizeFor(blockType, variant string) SizeBound`; `func clampBlockSize(b *Block)`. `ValidateBlocks` now clamps `w`/`h`.

- [ ] **Step 1: Write the failing test** — append to `showcase_test.go`:

```go
func TestValidateBlocks_ClampsSize(t *testing.T) {
	// favorite_anime/row bounds: W2..4, H1..1, default 4x1
	blocks := []Block{{Type: BlockFavoriteAnime, Variant: "row", Width: 1, Height: 3,
		Config: cfg(t, map[string][]string{"anime_ids": {"a"}})}}
	if err := ValidateBlocks(blocks); err != nil {
		t.Fatalf("expected clamp, got error: %v", err)
	}
	if blocks[0].Width != 2 || blocks[0].Height != 1 {
		t.Fatalf("want 2x1 after clamp, got %dx%d", blocks[0].Width, blocks[0].Height)
	}
}

func TestValidateBlocks_BackfillsDefaultSize(t *testing.T) {
	blocks := []Block{{Type: BlockStats, Variant: "tiles"}} // w/h absent
	if err := ValidateBlocks(blocks); err != nil {
		t.Fatal(err)
	}
	if blocks[0].Width != 2 || blocks[0].Height != 1 {
		t.Fatalf("want default 2x1, got %dx%d", blocks[0].Width, blocks[0].Height)
	}
}

func TestSizeFor_FallsBackToDefaultVariant(t *testing.T) {
	got := SizeFor(BlockAbout, "") // empty -> default variant "quote"
	if got.DefW != 2 || got.DefH != 1 {
		t.Fatalf("about default want 2x1, got %dx%d", got.DefW, got.DefH)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/player && go test ./internal/domain/ -run 'TestValidateBlocks_Clamps|Backfill|SizeFor' -v`
Expected: FAIL — `Width`/`SizeFor`/`VariantSizeAllowlist` undefined.

- [ ] **Step 3: Add fields + allowlist + helpers** in `showcase.go`.

Add to `Block` (after `Order`):

```go
	Width   int             `json:"w,omitempty"`
	Height  int             `json:"h,omitempty"`
```

Add after `VariantAllowlist`:

```go
// SizeBound is the grid-cell size range for one (block type, variant).
type SizeBound struct{ MinW, MaxW, MinH, MaxH, DefW, DefH int }

// VariantSizeAllowlist maps block type → variant → size bounds (grid cells,
// W 1..4, H 1..3). Keep in sync with frontend src/types/showcase.ts VARIANT_SIZE.
var VariantSizeAllowlist = map[string]map[string]SizeBound{
	BlockAbout: {
		"quote": {2, 4, 1, 2, 2, 1}, "bio": {2, 4, 1, 2, 2, 2}, "terminal": {2, 4, 1, 2, 2, 2},
		"minimal": {2, 4, 1, 2, 2, 1}, "vn": {2, 2, 1, 1, 2, 1},
	},
	BlockFavoriteAnime: {
		"row": {2, 4, 1, 1, 4, 1}, "podium": {2, 2, 2, 2, 2, 2}, "grid": {2, 4, 1, 3, 4, 2},
		"list": {2, 2, 1, 3, 2, 2}, "banner": {2, 4, 1, 3, 2, 2},
	},
	BlockFavoriteCharacter: {
		"circles": {1, 4, 1, 3, 2, 1}, "portraits": {2, 4, 1, 3, 2, 2},
		"hero": {2, 4, 1, 3, 2, 2}, "hex": {1, 4, 1, 3, 2, 2},
	},
	BlockCardCollection: {
		"row": {2, 4, 1, 1, 2, 1}, "fan": {2, 4, 2, 3, 2, 2}, "grid": {2, 4, 1, 3, 2, 2},
		"hero": {2, 4, 1, 2, 2, 2}, "tilt3d": {2, 4, 2, 3, 3, 2},
	},
	BlockStats: {
		"tiles": {2, 2, 1, 1, 2, 1}, "rings": {2, 2, 1, 1, 2, 1},
		"bars": {2, 2, 1, 1, 2, 1}, "strip": {2, 2, 1, 1, 2, 1},
	},
	BlockContinueWatching: {"cards": {2, 4, 1, 3, 2, 2}},
	BlockOpEd:             {"grid": {2, 4, 1, 3, 2, 2}},
	BlockAnimeDNA:         {"bars": {1, 2, 1, 3, 1, 2}},
	BlockCompatibility:    {"ring": {1, 2, 1, 1, 2, 1}},
}

// SizeFor returns the bound for (type, variant); empty/unknown variant falls
// back to the type's default variant (VariantAllowlist[type][0]).
func SizeFor(blockType, variant string) SizeBound {
	byVariant := VariantSizeAllowlist[blockType]
	if b, ok := byVariant[variant]; ok {
		return b
	}
	if defaults := VariantAllowlist[blockType]; len(defaults) > 0 {
		return byVariant[defaults[0]]
	}
	return SizeBound{1, 4, 1, 3, 2, 1}
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// clampBlockSize backfills absent w/h to the variant default and clamps any
// provided value into the variant's range. Never rejects.
func clampBlockSize(b *Block) {
	sb := SizeFor(b.Type, b.Variant)
	if b.Width == 0 {
		b.Width = sb.DefW
	}
	if b.Height == 0 {
		b.Height = sb.DefH
	}
	b.Width = clampInt(b.Width, sb.MinW, sb.MaxW)
	b.Height = clampInt(b.Height, sb.MinH, sb.MaxH)
}
```

In `ValidateBlocks`, inside the `for _, b := range blocks` loop, the loop var is a copy — switch to index so clamping persists. Change the loop header and add the clamp after the variant check:

```go
	for i := range blocks {
		b := &blocks[i]
		if _, known := VariantAllowlist[b.Type]; !known {
			return errors.InvalidInput("unknown showcase block type")
		}
		if !variantAllowed(b.Type, b.Variant) {
			return errors.InvalidInput("unknown variant for block type")
		}
		clampBlockSize(b)
		switch b.Type {
		// ... existing config validation unchanged, using b.Config ...
		}
	}
```

(Keep the existing per-type config validation body; only the loop header and the `clampBlockSize(b)` line are new.)

- [ ] **Step 4: Run test to verify it passes**

Run: `cd services/player && go test ./internal/domain/ -v`
Expected: PASS (new + existing tests).

- [ ] **Step 5: Commit**

```bash
git commit services/player/internal/domain/showcase.go services/player/internal/domain/showcase_test.go \
  -m "feat(showcase): per-variant block size bounds + clamp/backfill in ValidateBlocks"
```

---

### Task 2: Frontend types — w/h, VARIANT_SIZE, sizeFor, spanClasses

**Files:**
- Modify: `frontend/web/src/types/showcase.ts`
- Test: `frontend/web/src/types/__tests__/showcase-size.spec.ts` (create)

**Interfaces:**
- Consumes: `SHOWCASE_VARIANTS`, `ShowcaseBlockType` (existing).
- Produces: `ShowcaseBlock.w?: number`, `ShowcaseBlock.h?: number`; `interface SizeBound { minW; maxW; minH; maxH; defW; defH }`; `VARIANT_SIZE: Record<ShowcaseBlockType, Record<string, SizeBound>>`; `sizeFor(t, variant?): SizeBound`; `spanClasses(w, h): string`.

- [ ] **Step 1: Write the failing test** — create `src/types/__tests__/showcase-size.spec.ts`:

```ts
import { describe, it, expect } from 'vitest'
import { SHOWCASE_VARIANTS, VARIANT_SIZE, sizeFor, spanClasses } from '@/types/showcase'

describe('VARIANT_SIZE parity', () => {
  it('has a bound for every (type, variant) in SHOWCASE_VARIANTS', () => {
    for (const t of Object.keys(SHOWCASE_VARIANTS) as Array<keyof typeof SHOWCASE_VARIANTS>) {
      for (const v of SHOWCASE_VARIANTS[t]) {
        expect(VARIANT_SIZE[t]?.[v], `${t}.${v}`).toBeDefined()
      }
    }
  })
})

describe('sizeFor', () => {
  it('falls back to the default variant when variant missing', () => {
    expect(sizeFor('about')).toEqual(VARIANT_SIZE.about.quote)
    expect(sizeFor('about', 'nope')).toEqual(VARIANT_SIZE.about.quote)
  })
  it('stats tiles are fixed 2x1', () => {
    const b = sizeFor('stats', 'tiles')
    expect([b.minW, b.maxW, b.minH, b.maxH]).toEqual([2, 2, 1, 1])
  })
})

describe('spanClasses', () => {
  it('maps width/height to static tailwind span classes', () => {
    expect(spanClasses(4, 1)).toBe('col-span-2 md:col-span-4 row-span-1')
    expect(spanClasses(1, 2)).toBe('col-span-1 md:col-span-1 row-span-2')
    expect(spanClasses(3, 3)).toBe('col-span-2 md:col-span-3 row-span-3')
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd frontend/web && bunx vitest run src/types/__tests__/showcase-size.spec.ts`
Expected: FAIL — `VARIANT_SIZE`/`sizeFor`/`spanClasses` not exported.

- [ ] **Step 3: Implement in `showcase.ts`.**

Add `w`/`h` to the `ShowcaseBlock` interface:

```ts
export interface ShowcaseBlock {
  type: ShowcaseBlockType
  variant?: string
  order: number
  w?: number
  h?: number
  config: AboutConfig | FavoriteAnimeConfig | FavoriteCharacterConfig
        | CardCollectionConfig | StatsConfig | OpEdConfig | AutoConfig
}
```

Append:

```ts
export interface SizeBound { minW: number; maxW: number; minH: number; maxH: number; defW: number; defH: number }
const sb = (minW: number, maxW: number, minH: number, maxH: number, defW: number, defH: number): SizeBound =>
  ({ minW, maxW, minH, maxH, defW, defH })

// MUST mirror Go domain.VariantSizeAllowlist (services/player/internal/domain/showcase.go).
export const VARIANT_SIZE: Record<ShowcaseBlockType, Record<string, SizeBound>> = {
  about: { quote: sb(2,4,1,2,2,1), bio: sb(2,4,1,2,2,2), terminal: sb(2,4,1,2,2,2), minimal: sb(2,4,1,2,2,1), vn: sb(2,2,1,1,2,1) },
  favorite_anime: { row: sb(2,4,1,1,4,1), podium: sb(2,2,2,2,2,2), grid: sb(2,4,1,3,4,2), list: sb(2,2,1,3,2,2), banner: sb(2,4,1,3,2,2) },
  favorite_character: { circles: sb(1,4,1,3,2,1), portraits: sb(2,4,1,3,2,2), hero: sb(2,4,1,3,2,2), hex: sb(1,4,1,3,2,2) },
  card_collection: { row: sb(2,4,1,1,2,1), fan: sb(2,4,2,3,2,2), grid: sb(2,4,1,3,2,2), hero: sb(2,4,1,2,2,2), tilt3d: sb(2,4,2,3,3,2) },
  stats: { tiles: sb(2,2,1,1,2,1), rings: sb(2,2,1,1,2,1), bars: sb(2,2,1,1,2,1), strip: sb(2,2,1,1,2,1) },
  continue_watching: { cards: sb(2,4,1,3,2,2) },
  op_ed: { grid: sb(2,4,1,3,2,2) },
  anime_dna: { bars: sb(1,2,1,3,1,2) },
  compatibility: { ring: sb(1,2,1,1,2,1) },
}

export const sizeFor = (t: ShowcaseBlockType, variant?: string): SizeBound =>
  VARIANT_SIZE[t][variant ?? ''] ?? VARIANT_SIZE[t][SHOWCASE_VARIANTS[t][0]]

const W_CLASS: Record<number, string> = {
  1: 'col-span-1 md:col-span-1', 2: 'col-span-2 md:col-span-2',
  3: 'col-span-2 md:col-span-3', 4: 'col-span-2 md:col-span-4',
}
const H_CLASS: Record<number, string> = { 1: 'row-span-1', 2: 'row-span-2', 3: 'row-span-3' }
export const spanClasses = (w: number, h: number): string =>
  `${W_CLASS[w] ?? W_CLASS[2]} ${H_CLASS[h] ?? H_CLASS[1]}`

export const clampSize = (t: ShowcaseBlockType, variant: string | undefined, w: number, h: number) => {
  const b = sizeFor(t, variant)
  const c = (v: number, lo: number, hi: number) => Math.max(lo, Math.min(hi, v))
  return { w: c(w || b.defW, b.minW, b.maxW), h: c(h || b.defH, b.minH, b.maxH) }
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd frontend/web && bunx vitest run src/types/__tests__/showcase-size.spec.ts && bunx tsc --noEmit`
Expected: PASS + no type errors.

- [ ] **Step 5: Commit**

```bash
git commit frontend/web/src/types/showcase.ts frontend/web/src/types/__tests__/showcase-size.spec.ts \
  -m "feat(showcase): VARIANT_SIZE matrix + sizeFor/spanClasses/clampSize (Go parity)"
```

---

### Task 3: View — ProfileShowcase bento grid

**Files:**
- Modify: `frontend/web/src/components/profile/showcase/ProfileShowcase.vue`
- Test: `frontend/web/src/components/profile/showcase/__tests__/ProfileShowcase.spec.ts` (create if absent)

**Interfaces:**
- Consumes: `spanClasses`, `sizeFor` (Task 2).
- Produces: emits `loaded` with `(count: number)`; renders each block inside a grid cell with `spanClasses(w,h)`.

- [ ] **Step 1: Write the failing test:**

```ts
import { mount } from '@vue/test-utils'
import { describe, it, expect, vi } from 'vitest'
import ProfileShowcase from '../ProfileShowcase.vue'

vi.mock('@/api/client', () => ({
  showcaseApi: { getShowcase: vi.fn().mockResolvedValue({ data: { blocks: [
    { type: 'favorite_anime', variant: 'row', order: 0, w: 4, h: 1, config: { anime_ids: [] } },
    { type: 'stats', variant: 'tiles', order: 1, config: {} }, // w/h absent -> default 2x1
  ] } }) },
}))

describe('ProfileShowcase grid', () => {
  it('wraps each block in a cell with span classes and emits loaded', async () => {
    const wrapper = mount(ProfileShowcase, { props: { userId: 'u1', isOwner: false },
      global: { stubs: { FavoriteAnimeBlock: true, StatsBlock: true } } })
    await new Promise((r) => setTimeout(r))
    const cells = wrapper.findAll('[data-showcase-cell]')
    expect(cells).toHaveLength(2)
    expect(cells[0].classes()).toContain('md:col-span-4')
    expect(cells[1].classes()).toContain('md:col-span-2') // stats default 2x1
    expect(wrapper.emitted('loaded')?.[0]).toEqual([2])
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd frontend/web && bunx vitest run src/components/profile/showcase/__tests__/ProfileShowcase.spec.ts`
Expected: FAIL — no grid cells / no `loaded` emit.

- [ ] **Step 3: Implement.** In `ProfileShowcase.vue`:

Add to `<script setup>`: import `{ spanClasses, sizeFor }` from `@/types/showcase`; add `const emit = defineEmits<{ loaded: [number] }>()`; in `load()` after `blocks.value = data.blocks ?? []` add `emit('loaded', blocks.value.length)`. Add a helper:

```ts
function cellClass(b: ShowcaseBlock): string {
  const s = sizeFor(b.type, b.variant)
  return spanClasses(b.w || s.defW, b.h || s.defH)
}
```

Replace the view-mode block list wrapper. Change the `<template v-else>` body so the blocks render inside a grid; wrap each block in a cell `<div data-showcase-cell :class="['h-full', cellClass(b)]">`:

```html
<template v-else>
  <p v-if="!loading && !blocks.length" class="text-sm text-muted-foreground">{{ $t('showcase.empty') }}</p>
  <div v-else class="grid grid-cols-2 md:grid-cols-4 gap-3 [grid-auto-flow:dense] [grid-auto-rows:165px] md:[grid-auto-rows:190px]">
    <template v-for="(b, i) in blocks" :key="i">
      <div :data-showcase-cell="b.type" :class="['h-full', cellClass(b)]">
        <AboutBlock v-if="b.type === 'about'" :config="b.config as never" :variant="b.variant" />
        <FavoriteAnimeBlock v-else-if="b.type === 'favorite_anime'" :config="b.config as never" :variant="b.variant" :user-id="userId" />
        <StatsBlock v-else-if="b.type === 'stats'" :user-id="userId" :variant="b.variant" />
        <FavoriteCharacterBlock v-else-if="b.type === 'favorite_character'" :config="b.config as never" :variant="b.variant" />
        <CardCollectionBlock v-else-if="b.type === 'card_collection'" :config="b.config as never" :user-id="userId" :variant="b.variant" />
        <ContinueWatchingBlock v-else-if="b.type === 'continue_watching'" :user-id="userId" :variant="b.variant" />
        <OpEdBlock v-else-if="b.type === 'op_ed'" :config="b.config as never" :variant="b.variant" />
        <AnimeDnaBlock v-else-if="b.type === 'anime_dna'" :user-id="userId" :variant="b.variant" />
        <CompatibilityBlock v-else-if="b.type === 'compatibility'" :user-id="userId" :is-owner="isOwner" :variant="b.variant" />
      </div>
    </template>
  </div>
</template>
```

Each block component's root must fill its cell — add `h-full` (and `flex flex-col` where content should stretch) to the block component roots whose default already used `rounded-xl border bg-card`. (Audit the 9 block roots; add `h-full`.) The arbitrary `[grid-auto-rows:…]` / `[grid-auto-flow:dense]` carry layout only — DS-lint Rule 6 exempts non-spacing arbitrary values; verify with the lint self-test in Task 9.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd frontend/web && bunx vitest run src/components/profile/showcase/__tests__/ProfileShowcase.spec.ts && bunx tsc --noEmit`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git commit frontend/web/src/components/profile/showcase/ProfileShowcase.vue \
  frontend/web/src/components/profile/showcase/blocks/ \
  frontend/web/src/components/profile/showcase/__tests__/ProfileShowcase.spec.ts \
  -m "feat(showcase): render blocks as a w/h-driven bento grid + loaded emit"
```

---

### Task 4: Tab integration — showcase as first profile tab

**Files:**
- Modify: `frontend/web/src/views/Profile.vue`
- Modify: `frontend/web/src/locales/{en,ru,ja}.json`
- Test: extend the Profile tabs test if one exists, else add a focused spec for the tabs computed.

**Interfaces:**
- Consumes: `ProfileShowcase` `@loaded` (Task 3); `profileWallVisible` (existing `useProfileWallVisible`).
- Produces: a `showcase` tab value; default-tab selection.

- [ ] **Step 1: Add i18n key (all three locales).** In each of `en.json`/`ru.json`/`ja.json`, under `profile.tabs`, add:
  - en: `"showcase": "Showcase"`
  - ru: `"showcase": "Витрина"`
  - ja: `"showcase": "ショーケース"`

- [ ] **Step 2: Wire the tab.** In `Profile.vue`:

Change `const activeTab = ref('watchlist')` → `const activeTab = ref(profileWallVisible.value ? 'showcase' : 'watchlist')`.

Prepend the showcase tab in the `tabs` computed (before `watchlist`), gated by visibility:

```ts
const tabs = computed(() => {
  const baseTabs: Array<{ value: string; label: string }> = []
  if (profileWallVisible.value) baseTabs.push({ value: 'showcase', label: t('profile.tabs.showcase') })
  baseTabs.push({ value: 'watchlist', label: t('profile.tabs.watchlist') })
  if (isOwnProfile.value && gachaVisible.value) baseTabs.push({ value: 'collection', label: t('gacha.collection_tab') })
  if (isOwnProfile.value) baseTabs.push({ value: 'settings', label: t('profile.tabs.settings') })
  return baseTabs
})
```

Remove the standalone `<ProfileShowcase … class="mt-6" />` block (lines ~72–78) and add a panel slot inside `<Tabs>`:

```html
<template #showcase>
  <ProfileShowcase
    v-if="profileUser?.id"
    :user-id="profileUser.id"
    :is-owner="!!isOwnProfile"
    @loaded="onShowcaseLoaded"
  />
</template>
```

Add the empty-wall fallback (visitor shouldn't land on an empty showcase):

```ts
function onShowcaseLoaded(count: number) {
  if (count === 0 && !isOwnProfile.value && activeTab.value === 'showcase') {
    activeTab.value = 'watchlist'
  }
}
```

- [ ] **Step 3: Run checks**

Run: `cd frontend/web && bash scripts/i18n-lint.sh && bunx tsc --noEmit && bunx vitest run src/views`
Expected: i18n parity OK, no type errors, view tests pass.

- [ ] **Step 4: Commit**

```bash
git commit frontend/web/src/views/Profile.vue frontend/web/src/locales/en.json \
  frontend/web/src/locales/ru.json frontend/web/src/locales/ja.json \
  -m "feat(showcase): move showcase into the profile tab bar as the first tab"
```

---

### Task 5: Editor — render the bento grid with drag-to-swap (size exchange)

**Files:**
- Modify: `frontend/web/src/components/profile/showcase/ShowcaseEditor.vue`
- Test: `frontend/web/src/components/profile/showcase/__tests__/ShowcaseEditor.spec.ts`

**Interfaces:**
- Consumes: `sizeFor`, `clampSize`, `spanClasses` (Task 2); `defaultVariant`, `SHOWCASE_VARIANTS` (existing).
- Produces: `function swapBlocks(i: number, j: number)` — swaps `local[i]`/`local[j]` order positions and exchanges their `w/h`, each re-clamped via `clampSize`. New blocks created with default-variant default size.

- [ ] **Step 1: Write the failing test** for the swap logic (unit-level via exposed function or simulated):

```ts
import { mount } from '@vue/test-utils'
import { describe, it, expect } from 'vitest'
import ShowcaseEditor from '../ShowcaseEditor.vue'

const blocks = [
  { type: 'favorite_anime', variant: 'row', order: 0, w: 4, h: 1, config: { anime_ids: [] } },
  { type: 'compatibility', variant: 'ring', order: 1, w: 2, h: 1, config: {} },
]

describe('ShowcaseEditor swap', () => {
  it('swaps order and exchanges sizes clamped to each block\'s variant', async () => {
    const wrapper = mount(ShowcaseEditor, { props: { userId: 'u1', modelValue: blocks },
      global: { stubs: { draggable: true } } })
    ;(wrapper.vm as any).swapBlocks(0, 1)
    const local = (wrapper.vm as any).local as typeof blocks
    // anime gets compatibility's 2x1 -> clamped to anime/row (W2..4,H1) = 2x1
    // compatibility gets anime's 4x1 -> clamped to ring (W1..2,H1) = 2x1
    const anime = local.find((b) => b.type === 'favorite_anime')!
    const compat = local.find((b) => b.type === 'compatibility')!
    expect([anime.w, anime.h]).toEqual([2, 1])
    expect([compat.w, compat.h]).toEqual([2, 1])
  })
})
```

(Expose `swapBlocks` and `local` via `defineExpose({ swapBlocks, local })`.)

- [ ] **Step 2: Run test to verify it fails**

Run: `cd frontend/web && bunx vitest run src/components/profile/showcase/__tests__/ShowcaseEditor.spec.ts -t swap`
Expected: FAIL — `swapBlocks` undefined.

- [ ] **Step 3: Implement.** In `ShowcaseEditor.vue`:

Seed sizes when adding (replace the push in `addBlock`):

```ts
const variant = defaultVariant(type)
const s = sizeFor(type, variant)
local.value.push({ type, order: local.value.length, variant, w: s.defW, h: s.defH, config })
```

Add the swap (port of mock `swapTiles`):

```ts
import { sizeFor, clampSize, spanClasses } from '@/types/showcase'

function swapBlocks(i: number, j: number) {
  const a = local.value[i], b = local.value[j]
  if (!a || !b) return
  const ca = clampSize(a.type, a.variant, b.w ?? 0, b.h ?? 0)
  const cb = clampSize(b.type, b.variant, a.w ?? 0, a.h ?? 0)
  a.w = ca.w; a.h = ca.h
  b.w = cb.w; b.h = cb.h
  local.value[i] = b; local.value[j] = a
}
defineExpose({ swapBlocks, local })
```

Convert the `<draggable>` list to the bento grid. Replace the draggable wrapper `class` and item template wrapper so items render in the grid with `spanClasses`. Use vuedraggable's `swap` semantics: configure `:swap="true"` via the SortableJS Swap plugin OR, simpler and dependency-free, keep `draggable` for reordering and additionally bind a drop-to-swap handler mirroring the mock (`@drop`/`@dragover` per item calling `swapBlocks`). Set the draggable container classes to:

```
class="grid grid-cols-2 md:grid-cols-4 gap-3 [grid-auto-flow:dense] [grid-auto-rows:190px]"
```

and each item wrapper to `:class="['relative', spanClasses(element.w || sizeFor(element.type, element.variant).defW, element.h || sizeFor(element.type, element.variant).defH)]"`. The block content inside each item renders via the same 9-way dispatch as `ProfileShowcase` (extract a shared `ShowcaseBlockView.vue` to avoid duplicating the dispatch — see note) so the editor previews real variants. Keep the existing per-block inline config OR move it to the dialog in Task 8; for this task just render the grid + swap + size.

> **Decomposition note:** extract the 9-way block dispatch from `ProfileShowcase.vue` into a small `ShowcaseBlockView.vue` (props `block`, `userId`, `isOwner`) and reuse it in both view and editor. Do this as the first edit of Task 5 and update Task 3's `ProfileShowcase` to use it (run Task 3 tests again — they should still pass).

- [ ] **Step 4: Run test to verify it passes**

Run: `cd frontend/web && bunx vitest run src/components/profile/showcase/__tests__/ShowcaseEditor.spec.ts && bunx tsc --noEmit`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git commit frontend/web/src/components/profile/showcase/ \
  -m "feat(showcase): editor renders bento grid + drag-to-swap with size exchange"
```

---

### Task 6: Editor — corner resize handle (snap, clamp, fixed/touch hiding)

**Files:**
- Modify: `frontend/web/src/components/profile/showcase/ShowcaseEditor.vue`
- Test: `ShowcaseEditor.spec.ts` (extend)

**Interfaces:**
- Consumes: `clampSize`, `sizeFor` (Task 2).
- Produces: `function applyResize(i: number, dCols: number, dRows: number)` — adjusts `local[i].w/h` by deltas, clamped to the block's variant bounds. `function isFixed(b): boolean`.

- [ ] **Step 1: Write the failing test:**

```ts
it('resize clamps to the variant bounds', () => {
  const wrapper = mount(ShowcaseEditor, { props: { userId: 'u1', modelValue: [
    { type: 'favorite_anime', variant: 'row', order: 0, w: 4, h: 1, config: { anime_ids: [] } },
  ] }, global: { stubs: { draggable: true } } })
  ;(wrapper.vm as any).applyResize(0, -5, +2) // try to shrink below min / grow past max-h(1)
  const b = (wrapper.vm as any).local[0]
  expect([b.w, b.h]).toEqual([2, 1]) // row: W2..4, H fixed 1
})

it('reports fixed-size variants', () => {
  const wrapper = mount(ShowcaseEditor, { props: { userId: 'u1', modelValue: [
    { type: 'stats', variant: 'tiles', order: 0, w: 2, h: 1, config: {} },
  ] }, global: { stubs: { draggable: true } } })
  expect((wrapper.vm as any).isFixed((wrapper.vm as any).local[0])).toBe(true)
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd frontend/web && bunx vitest run src/components/profile/showcase/__tests__/ShowcaseEditor.spec.ts -t resize`
Expected: FAIL — `applyResize`/`isFixed` undefined.

- [ ] **Step 3: Implement** (port mock `wire()` resize handler + `con`). Add:

```ts
function isFixed(b: ShowcaseBlock): boolean {
  const s = sizeFor(b.type, b.variant)
  return s.minW === s.maxW && s.minH === s.maxH
}
function applyResize(i: number, dCols: number, dRows: number) {
  const b = local.value[i]; if (!b) return
  const c = clampSize(b.type, b.variant, (b.w ?? 0) + dCols, (b.h ?? 0) + dRows)
  b.w = c.w; b.h = c.h
}
defineExpose({ swapBlocks, applyResize, isFixed, local })
```

Add the pointer-driven handle to each grid item (hidden when `isFixed(element)` or on coarse pointers via `@media (pointer: coarse)`):

```html
<button
  v-if="!isFixed(element)"
  type="button"
  class="showcase-resize absolute bottom-1 right-1 grid h-6 w-6 place-items-center rounded-lg border border-border text-brand-cyan cursor-nwse-resize touch-none"
  :data-test="`showcase-resize-${index}`"
  @pointerdown="startResize($event, index)"
>◢</button>
```

```ts
function startResize(e: PointerEvent, i: number) {
  e.preventDefault(); e.stopPropagation()
  const grid = (e.currentTarget as HTMLElement).closest('[data-showcase-grid]') as HTMLElement
  const cols = window.innerWidth < 768 ? 2 : 4
  const gap = 12
  const cellW = (grid.clientWidth - (cols - 1) * gap) / cols
  const rowH = window.innerWidth < 768 ? 165 : 190
  const sx = e.clientX, sy = e.clientY
  const sw = local.value[i].w ?? 0, sh = local.value[i].h ?? 0
  ;(e.target as HTMLElement).setPointerCapture(e.pointerId)
  const move = (ev: PointerEvent) => {
    applyResizeAbsolute(i, sw + Math.round((ev.clientX - sx) / (cellW + gap)),
                            sh + Math.round((ev.clientY - sy) / (rowH + gap)))
  }
  const up = () => { document.removeEventListener('pointermove', move); document.removeEventListener('pointerup', up) }
  document.addEventListener('pointermove', move); document.addEventListener('pointerup', up)
}
function applyResizeAbsolute(i: number, w: number, h: number) {
  const b = local.value[i]; if (!b) return
  const c = clampSize(b.type, b.variant, w, h); b.w = c.w; b.h = c.h
}
```

Hide the handle on touch with a scoped style: `@media (pointer: coarse) { .showcase-resize { display: none } }`. Add `data-showcase-grid` to the grid container. Use `text-brand-cyan` / `border-border` tokens (DS-safe). `.touch-none` maps to `touch-action: none`.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd frontend/web && bunx vitest run src/components/profile/showcase/__tests__/ShowcaseEditor.spec.ts && bunx tsc --noEmit`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git commit frontend/web/src/components/profile/showcase/ShowcaseEditor.vue \
  frontend/web/src/components/profile/showcase/__tests__/ShowcaseEditor.spec.ts \
  -m "feat(showcase): corner resize handle with per-variant clamp + fixed/touch hiding"
```

---

### Task 7: Editor — add-block picker

**Files:**
- Modify: `frontend/web/src/components/profile/showcase/ShowcaseEditor.vue`
- Modify: `frontend/web/src/locales/{en,ru,ja}.json`
- Test: `ShowcaseEditor.spec.ts` (extend)

**Interfaces:**
- Consumes: `addBlock` (existing), `MAX_SHOWCASE_BLOCKS`, `ADDABLE` (existing).
- Produces: `const pickerOpen: Ref<boolean>`; `function usedTypes(): Set<string>` (one block per type).

- [ ] **Step 1: Write the failing test:**

```ts
it('disables already-present types in the picker', async () => {
  const wrapper = mount(ShowcaseEditor, { props: { userId: 'u1', modelValue: [
    { type: 'about', variant: 'bio', order: 0, w: 2, h: 2, config: { title: '', text: '' } },
  ] }, global: { stubs: { draggable: true } } })
  ;(wrapper.vm as any).pickerOpen = true
  await wrapper.vm.$nextTick()
  const aboutBtn = wrapper.find('[data-test="picker-about"]')
  expect(aboutBtn.attributes('disabled')).toBeDefined()
  expect(wrapper.find('[data-test="picker-stats"]').attributes('disabled')).toBeUndefined()
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd frontend/web && bunx vitest run src/components/profile/showcase/__tests__/ShowcaseEditor.spec.ts -t picker`
Expected: FAIL — no picker markup.

- [ ] **Step 3: Implement.** Replace the row of `+` buttons with a single "Add block" button that opens a picker (reuse `@/components/ui/Dialog.vue` if present, else a simple overlay). Add:

```ts
import { ref } from 'vue'
const pickerOpen = ref(false)
function usedTypes(): Set<string> { return new Set(local.value.map((b) => b.type)) }
function pick(type: ShowcaseBlockType) { addBlock(type); pickerOpen.value = false }
defineExpose({ swapBlocks, applyResize, isFixed, pickerOpen, usedTypes, local })
```

Picker markup (each option disabled when present):

```html
<button type="button" data-test="showcase-open-picker"
  class="rounded-lg border border-border px-3 py-1 text-sm font-medium text-foreground hover:bg-accent"
  @click="pickerOpen = true">+ {{ $t('showcase.add_block') }}</button>

<div v-if="pickerOpen" class="..."> <!-- overlay; reuse Dialog if available -->
  <button v-for="type in ADDABLE" :key="type" type="button"
    :data-test="`picker-${type}`" :disabled="usedTypes().has(type)"
    class="... disabled:opacity-40 disabled:pointer-events-none"
    @click="pick(type)">
    {{ $t(`showcase.block.${type}`) }}
  </button>
</div>
```

Add i18n keys (all three locales): `showcase.add_block` (en `"Add block"`, ru `"Добавить блок"`, ja `"ブロックを追加"`), `showcase.add_block_title` (en `"Add a block"`, ru `"Добавить блок"`, ja `"ブロックを追加"`).

- [ ] **Step 4: Run test to verify it passes**

Run: `cd frontend/web && bunx vitest run src/components/profile/showcase/__tests__/ShowcaseEditor.spec.ts && bash scripts/i18n-lint.sh && bunx tsc --noEmit`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git commit frontend/web/src/components/profile/showcase/ShowcaseEditor.vue \
  frontend/web/src/components/profile/showcase/__tests__/ShowcaseEditor.spec.ts \
  frontend/web/src/locales/en.json frontend/web/src/locales/ru.json frontend/web/src/locales/ja.json \
  -m "feat(showcase): add-block picker (one block per type, present types disabled)"
```

---

### Task 8: Editor — per-block config dialog (variant + content, re-clamp on variant change)

**Files:**
- Create: `frontend/web/src/components/profile/showcase/ShowcaseConfigDialog.vue`
- Modify: `frontend/web/src/components/profile/showcase/ShowcaseEditor.vue`
- Test: `frontend/web/src/components/profile/showcase/__tests__/ShowcaseConfigDialog.spec.ts`

**Interfaces:**
- Consumes: `SHOWCASE_VARIANTS`, `clampSize`, existing config editors (about fields, op_ed list, auto-fill).
- Produces: `ShowcaseConfigDialog` with props `{ block: ShowcaseBlock; userId: string }`, emits `update:block` (mutated copy) and `close`. On variant change it re-clamps `block.w/h` via `clampSize`.

- [ ] **Step 1: Write the failing test:**

```ts
import { mount } from '@vue/test-utils'
import { describe, it, expect } from 'vitest'
import ShowcaseConfigDialog from '../ShowcaseConfigDialog.vue'

describe('ShowcaseConfigDialog', () => {
  it('re-clamps size when switching to a smaller-bound variant', async () => {
    const block = { type: 'favorite_anime', variant: 'grid', order: 0, w: 4, h: 3, config: { anime_ids: [] } }
    const wrapper = mount(ShowcaseConfigDialog, { props: { block, userId: 'u1' } })
    await wrapper.find('[data-test="variant-podium"]').trigger('click') // podium fixed 2x2
    const updated = wrapper.emitted('update:block')!.at(-1)![0] as typeof block
    expect([updated.variant, updated.w, updated.h]).toEqual(['podium', 2, 2])
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd frontend/web && bunx vitest run src/components/profile/showcase/__tests__/ShowcaseConfigDialog.spec.ts`
Expected: FAIL — component missing.

- [ ] **Step 3: Implement `ShowcaseConfigDialog.vue`.** Move the existing per-block inline config (about title/text, favorite_anime/cards Auto buttons + hint, op_ed theme list, auto-block info, variant Select) out of `ShowcaseEditor.vue` into this dialog. Operate on a local copy of the block; emit `update:block` on every change. Variant picker as chips:

```ts
import { SHOWCASE_VARIANTS, clampSize } from '@/types/showcase'
const draft = reactive({ ...props.block, config: { ...props.block.config } })
function setVariant(v: string) {
  draft.variant = v
  const c = clampSize(draft.type, v, draft.w ?? 0, draft.h ?? 0)
  draft.w = c.w; draft.h = c.h
  emit('update:block', { ...draft })
}
```

```html
<button v-for="v in SHOWCASE_VARIANTS[block.type]" :key="v"
  type="button" :data-test="`variant-${v}`"
  :class="['rounded-lg border px-3 py-1 text-sm', draft.variant === v ? 'border-brand-cyan text-brand-cyan' : 'border-border text-muted-foreground']"
  @click="setVariant(v)">{{ $t(`showcase.variant.${block.type}.${v}`) }}</button>
```

(Add `showcase.variant.<type>.<variant>` labels to all three locales — or reuse the raw variant key as label if a full label set is out of scope; the parity test only checks key existence, so if you add the namespace, add it to all three. Keeping the raw key label is acceptable for v1.)

Content sections: reuse the exact markup currently in `ShowcaseEditor.vue` (about inputs, favorite_anime/cards Auto + hint, op_ed list, auto info). Seed from `props.block.config` (NOT defaults) so edits persist — this fixes the persistence bug surfaced in the mock.

In `ShowcaseEditor.vue`: add a ⚙ button per grid item that opens the dialog for that block; on `update:block`, replace `local.value[index]` with the emitted block.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd frontend/web && bunx vitest run src/components/profile/showcase/ && bash scripts/i18n-lint.sh && bunx tsc --noEmit`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git commit frontend/web/src/components/profile/showcase/ frontend/web/src/locales/ \
  -m "feat(showcase): per-block config dialog with variant picker + size re-clamp"
```

---

### Task 9: QA, lint gates, e2e, deploy

**Files:**
- Modify: `frontend/web/e2e/spotlight.spec.ts` neighbour — add/extend a showcase e2e spec.
- No source changes unless a gate fails.

- [ ] **Step 1: Lint + type + unit gates**

Run:
```bash
cd frontend/web
bash scripts/design-system-lint.sh           # Rules 1/6/8 — must be ERRORS=0
bash scripts/design-system-lint.sh --selftest # prove the gate works
bash scripts/i18n-lint.sh
bunx tsc --noEmit
bunx vitest run src/components/profile/showcase/ src/types/__tests__/showcase-size.spec.ts
cd ../../services/player && go test ./internal/... -count=1
```
Expected: all green.

- [ ] **Step 2: e2e** — extend a showcase Playwright spec: as `ui_audit_bot`, open own profile → showcase tab is first + active → enter edit → resize a non-fixed block (drag the `[data-test^="showcase-resize"]` handle) → open ⚙, switch a variant, add a favorite → save → reload → assert the block size, variant, and favorites persisted, and that switching to a fixed-size variant hid the handle.

Run: `cd frontend/web && bunx playwright test showcase`
Expected: PASS.

- [ ] **Step 3: Manual smoke (optional, owner opt-in per DS-NF-06).** Offer a Chrome checkup of the live mosaic (cascade-sensitive: Tailwind v4 unlayered classes can beat utilities — jsdom won't catch it). Only run if the owner asks.

- [ ] **Step 4: Deploy via `/animeenigma-after-update`** — lints + builds, redeploys `player` + `web` **from a clean `origin/main` worktree** (copy `docker/.env`), health check, changelog entry (Russian Trump-mode), commit + push. If push is rejected, land on `main` via worktree cherry-pick (do not reset --hard the shared tree).

- [ ] **Step 5: Commit** (e2e spec)

```bash
git commit frontend/web/e2e/ -m "test(showcase): e2e mosaic resize/swap/config persistence"
```

---

## Self-Review

**Spec coverage:** D1 bento grid → Task 3/5; D2 resize handle → Task 6; D3 mobile 2-col → Task 3 (`grid-cols-2`) + Task 6 (cols=2 math); D4 first tab → Task 4; D5 swap+size → Task 5; D6 per-variant bounds → Task 1/2; D7 additive/backfill → Task 1. §5 model → Task 1/2. §6 view → Task 3. §7 matrix → Task 1/2 (verbatim). §8 tabs → Task 4. §9 editor (resize/swap/picker/config) → Task 5/6/7/8. §10 validation/parity/tests → Task 1/2/9. §11 files → all. §12 rollout → Task 9.

**Placeholder scan:** no TBD/TODO; every code step has concrete code; the one "raw key label acceptable for v1" is an explicit, justified scope decision, not a placeholder.

**Type consistency:** `SizeBound`/`sizeFor`/`clampSize`/`spanClasses` defined in Task 2 and used unchanged in Tasks 3/5/6/8; Go `SizeFor`/`clampBlockSize` Task 1; `swapBlocks`/`applyResize`/`isFixed`/`pickerOpen`/`usedTypes` exposed consistently across Tasks 5–7.

**Open follow-up:** Task 5's `ShowcaseBlockView.vue` extraction is required before the editor can preview real variants; it is folded into Task 5 step 3 and re-runs Task 3 tests.
