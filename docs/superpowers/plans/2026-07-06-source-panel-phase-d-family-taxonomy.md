# Source Panel — Phase D: Family Taxonomy Refactor — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Collapse the capability feed's wire `family` field to the three-value taxonomy `{"18+", "others", "aeProvider"}` and migrate the player's one functional use of `family` onto the already-load-bearing `group` field — a behavior-preserving refactor.

**Architecture:** The frontend stops reading `family` entirely (it reads `group`, which the feed already emits correctly), then the backend regroups the assembled per-source families into three merged wire families. `group` (`en`/`ru`/`adult`/`firstparty`) is unchanged and remains the functional language/relevance facet.

**Tech Stack:** Go (catalog service, GORM/capability service), Vue 3 + TypeScript (aePlayer), Vitest, `bun`/`bunx`.

## Global Constraints

- Work happens in the existing worktree `source-panel-truth-and-ranking` (already checked out). Never edit the base tree at `/data/animeenigma`.
- `group` values are fixed: `'en' | 'ru' | 'adult' | 'firstparty'` — do NOT change them. Only `family` changes.
- New wire `family` values (exact strings): `"18+"`, `"others"`, `"aeProvider"`.
- `group === 'en'` uniquely identifies the EN scraper chain (the only providers with group `en`). This — NOT `family === 'others'` — is what replaces the old `family === 'ourenglish'` check. `"others"` also contains kodik/animelib/animejoy (group `ru`), so keying EN routing off the family label would misroute them.
- **DEPLOY-ORDER HAZARD:** the frontend migration (Task 1) MUST be built/deployed before the backend relabel (Task 2). A backend-first deploy would leave a live old frontend still checking `family === 'ourenglish'` against the new `"others"` label → EN routing breaks. Committing Task 1 before Task 2 (and letting `/animeenigma-after-update` redeploy web ahead of catalog, or together) satisfies this. After Task 1 the frontend ignores `family` entirely, so it is robust to either label set.
- Frontend gate: run `/frontend-verify` before finishing any `frontend/web/` change. No new i18n strings are introduced in Phase D.
- Commit trailers on every commit:
  ```
  Co-Authored-By: Claude Code <noreply@anthropic.com>
  Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
  Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
  ```

---

### Task 1: Frontend — migrate the player off `family` onto `group`

Atomic FE migration: `providerToLegacyPlayer`, `comboToWatchCombo`, the resolver's EN-chain dispatch, and the feed helper all move from `family` to `group`, and all call sites in `AePlayer.vue` pass `group`. This lands as one commit so every intermediate commit keeps the app working. After this task the frontend reads zero `family` values functionally.

**Files:**
- Modify: `frontend/web/src/composables/aePlayer/comboMapping.ts`
- Modify: `frontend/web/src/composables/aePlayer/useProviderResolver.ts`
- Modify: `frontend/web/src/composables/aePlayer/useProviderFeed.ts`
- Modify: `frontend/web/src/components/player/aePlayer/AePlayer.vue` (lines ~476, ~585, ~976, ~1146)
- Modify: `frontend/web/src/types/capabilities.ts`
- Test: `frontend/web/src/composables/aePlayer/comboMapping.spec.ts`
- Test: `frontend/web/src/composables/aePlayer/useProviderFeed.spec.ts`

**Interfaces:**
- Produces: `providerToLegacyPlayer(providerId: string, group?: string): LegacyPlayer | null` — EN chain via `group === 'en'`.
- Produces: `comboToWatchCombo(combo: Combo, group?: string): WatchCombo | null`.
- Produces: `groupOfProvider(report: CapabilityReport | null, providerId: string): string | undefined` (renamed from `familyOfProvider`; returns the provider cap's `group`).
- Produces: `ResolverDeps.groupOf?: (providerId: string) => string | undefined` (renamed from `familyOf`); `useProviderResolver(groupOf?: (id: string) => string | undefined)`.

- [ ] **Step 1: Update the unit tests to assert group-based behavior (write the failing tests)**

In `frontend/web/src/composables/aePlayer/comboMapping.spec.ts`, replace the two `'ourenglish'` usages with `'en'`:

```ts
describe('providerToLegacyPlayer', () => {
  it('maps en-group providers to english', () => {
    for (const id of ['gogoanime', 'allanime-okru', 'miruro']) {
      expect(providerToLegacyPlayer(id, 'en')).toBe('english')
    }
  })
  it('maps single-provider ids by id', () => {
    expect(providerToLegacyPlayer('kodik')).toBe('kodik')
    expect(providerToLegacyPlayer('ae')).toBe('ae')
    expect(providerToLegacyPlayer('18anime')).toBe('hanime')
    expect(providerToLegacyPlayer('animelib')).toBe('animelib')
  })
  it('returns null for an unknown provider', () => {
    expect(providerToLegacyPlayer('nope')).toBeNull()
  })
})
```

And the `comboToWatchCombo` case (was line ~33-34):

```ts
  it('threads the en group through to english', () => {
    expect(comboToWatchCombo({ audio: 'sub', lang: 'en', provider: 'allanime-okru', server: '', team: 'SubsPlease' }, 'en'))
      .toMatchObject({ player: 'english' })
  })
```

In `frontend/web/src/composables/aePlayer/useProviderFeed.spec.ts`, change the import and replace the entire `familyOfProvider` describe block (bottom of file) with a `groupOfProvider` one:

```ts
import { rowsFromReport, groupOfProvider } from '@/composables/aePlayer/useProviderFeed'
```

```ts
const groupReport = {
  anime_id: 'x',
  families: [
    { family: 'others', providers: [
      { provider: 'gogoanime', group: 'en' }, { provider: 'allanime-okru', group: 'en' },
      { provider: 'kodik', group: 'ru' },
    ] },
  ],
} as unknown as CapabilityReport

describe('groupOfProvider', () => {
  it('returns the group for a known provider', () => {
    expect(groupOfProvider(groupReport, 'allanime-okru')).toBe('en')
    expect(groupOfProvider(groupReport, 'kodik')).toBe('ru')
  })
  it('returns undefined for an unknown provider or null report', () => {
    expect(groupOfProvider(groupReport, 'nope')).toBeUndefined()
    expect(groupOfProvider(null, 'gogoanime')).toBeUndefined()
  })
})
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd frontend/web && bunx vitest run src/composables/aePlayer/comboMapping.spec.ts src/composables/aePlayer/useProviderFeed.spec.ts`
Expected: FAIL — `groupOfProvider` is not exported; `providerToLegacyPlayer(id, 'en')` returns `null` (old code checks `'ourenglish'`).

- [ ] **Step 3: Migrate `comboMapping.ts`**

Change `providerToLegacyPlayer` and `comboToWatchCombo` from `family` to `group`, and update the doc comment:

```ts
/** Map a granular unified provider id -> coarse legacy WatchCombo.player (or null).
 *  EN-chain membership is backend-driven: pass the provider's capability `group`
 *  (from groupOfProvider) — group 'en' ⇒ 'english'. The remaining single-provider
 *  families stay keyed on id. */
export function providerToLegacyPlayer(providerId: string, group?: string): LegacyPlayer | null {
  if (group === 'en') return 'english'
  switch (providerId) {
    case 'kodik': return 'kodik'
    case 'ae': return 'ae'
    case '18anime': return 'hanime'
    case 'hanime': return 'hanime'
    case 'animelib': return 'animelib'
    default: return null
  }
}
```

```ts
/** Map a unified Combo -> legacy WatchCombo for persistence/resolve. Null if provider unmappable. */
export function comboToWatchCombo(combo: Combo, group?: string): WatchCombo | null {
  const player = providerToLegacyPlayer(combo.provider, group)
  if (!player) return null
  return {
    player,
    language: langToLanguage[combo.lang],
    watch_type: combo.audio,
    translation_id: '',
    translation_title: combo.team ?? '',
  }
}
```

- [ ] **Step 4: Migrate `useProviderResolver.ts`**

Rename the dep and the EN-chain dispatch. In `ResolverDeps`:

```ts
  /** provider id → backend group (from the capability feed). Feed-driven EN-chain routing. */
  groupOf?: (providerId: string) => string | undefined
```

In `makeResolver`'s `getAdapter`, replace the dispatch line:

```ts
    if (deps.groupOf?.(provider) === 'en') {
      if (!deps.scraperApi) {
        throw new NotAvailableError(provider, 'not available (scraperApi dep missing)')
      }
      return makeScraperAdapter(deps.scraperApi, provider)
    }
```

Update the `makeResolver` doc line `- familyOf(provider) === 'ourenglish' → scraperAdapter` to `- groupOf(provider) === 'en' → scraperAdapter`. Update the composable:

```ts
export function useProviderResolver(groupOf?: (id: string) => string | undefined): ProviderResolver {
  return makeResolver({ scraperApi, anime18Api, kodikApi, aeApi, hanimeApi, animejoyApi, groupOf })
}
```

- [ ] **Step 5: Migrate `useProviderFeed.ts`**

Rename `familyOfProvider` → `groupOfProvider`, returning the cap's `group`:

```ts
/** Provider id → its backend `group` ('en' | 'ru' | 'adult' | 'firstparty'), read
 *  straight from the capability feed. Single source of truth for "which language
 *  facet / backend serves this provider" — the FE no longer reads the coarse
 *  `family` label. */
export function groupOfProvider(report: CapabilityReport | null, providerId: string): string | undefined {
  if (!report || !Array.isArray(report.families)) return undefined
  for (const fam of report.families) {
    for (const cap of fam.providers ?? []) {
      if (cap.provider === providerId) return cap.group
    }
  }
  return undefined
}
```

- [ ] **Step 6: Update the three `AePlayer.vue` call sites + import**

Line ~476 import: `import { rowsFromReport, groupOfProvider } from '@/composables/aePlayer/useProviderFeed'`.

Line ~585:

```ts
const resolver = props.offline ? makeOfflineResolver(props.offline) : useProviderResolver((id) => groupOfProvider(report.value, id))
```

Line ~976 (inside `buildAvailable` — use the per-provider `cap.group`, not the family):

```ts
      const player = providerToLegacyPlayer(cap.provider, cap.group)
```

Line ~1146:

```ts
  () => comboToWatchCombo(state.combo.value, groupOfProvider(report.value, state.combo.value.provider)),
```

- [ ] **Step 7: Update the `family` type in `capabilities.ts`**

```ts
export interface SourceFamily {
  family: '18+' | 'others' | 'aeProvider'
  providers: ProviderCap[]
}
```

- [ ] **Step 8: Run the tests to verify they pass**

Run: `cd frontend/web && bunx vitest run src/composables/aePlayer/comboMapping.spec.ts src/composables/aePlayer/useProviderFeed.spec.ts`
Expected: PASS.

- [ ] **Step 9: Type-check (catches any missed `family` reference / fixture label that no longer satisfies the narrowed type)**

Run: `cd frontend/web && bunx tsc --noEmit`
Expected: no errors. If `useCapabilities.spec.ts` (or any fixture) fails because a literal `family: 'ourenglish'`/`'kodik'` no longer satisfies the narrowed union, change those fixture labels to `'others'` (their tests assert the provider map, not the family string, so behavior is unchanged).

- [ ] **Step 10: Frontend pre-flight**

Run: `/frontend-verify`
Expected: DS-lint clean, i18n parity unchanged, `bun run build` succeeds.

- [ ] **Step 11: Commit**

```bash
git add frontend/web/src/composables/aePlayer/comboMapping.ts \
  frontend/web/src/composables/aePlayer/comboMapping.spec.ts \
  frontend/web/src/composables/aePlayer/useProviderResolver.ts \
  frontend/web/src/composables/aePlayer/useProviderFeed.ts \
  frontend/web/src/composables/aePlayer/useProviderFeed.spec.ts \
  frontend/web/src/components/player/aePlayer/AePlayer.vue \
  frontend/web/src/types/capabilities.ts \
  frontend/web/src/composables/aePlayer/useCapabilities.spec.ts
git commit -m "$(cat <<'EOF'
refactor(player): migrate source dispatch from capability `family` to `group`

The FE no longer reads the coarse `family` label; EN-chain routing keys off
`group === 'en'`. Prepares the backend `family` collapse (Phase D) and removes
`family`/`group` redundancy.

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
)"
```

---

### Task 2: Backend — regroup wire families to `{18+, others, aeProvider}`

A pure `regroupFamilies` function buckets the assembled per-source families by label and merges them; `buildFamilies` calls it as the last step. `BuildENFamily` keeps its internal `"ourenglish"` label (its direct test stays green) — only the assembled report is regrouped.

**Files:**
- Modify: `services/catalog/internal/service/capability/service.go` (add `familyLabel` + `regroupFamilies`; call in `buildFamilies`)
- Modify: `services/catalog/internal/domain/capability.go` (update `SourceFamily.Family` doc comment)
- Test: `services/catalog/internal/service/capability/service_test.go` (add `TestRegroupFamilies`)

**Interfaces:**
- Consumes: `domain.SourceFamily{Family string; Providers []domain.ProviderCap}`, `domain.ProviderCap{Provider string; …}`.
- Produces: `regroupFamilies(in []domain.SourceFamily) []domain.SourceFamily` (pure); `buildFamilies` now returns regrouped families with `Family ∈ {"18+","others","aeProvider"}`.

- [ ] **Step 1: Write the failing test**

Add to `services/catalog/internal/service/capability/service_test.go` (ensure `reflect` is imported):

```go
func TestRegroupFamilies(t *testing.T) {
	in := []domain.SourceFamily{
		{Family: "ae", Providers: []domain.ProviderCap{{Provider: "ae"}}},
		{Family: "ourenglish", Providers: []domain.ProviderCap{{Provider: "gogoanime"}, {Provider: "okru"}}},
		{Family: "adult", Providers: []domain.ProviderCap{{Provider: "18anime"}}},
		{Family: "kodik", Providers: []domain.ProviderCap{{Provider: "kodik"}}},
		{Family: "hanime", Providers: []domain.ProviderCap{{Provider: "hanime"}}},
		{Family: "animejoy-sibnet", Providers: []domain.ProviderCap{{Provider: "animejoy-sibnet"}}},
	}
	out := regroupFamilies(in)

	if len(out) != 3 {
		t.Fatalf("want 3 wire families, got %d", len(out))
	}
	// First-seen label order: ae→aeProvider, ourenglish→others, adult→18+.
	if out[0].Family != "aeProvider" || out[1].Family != "others" || out[2].Family != "18+" {
		t.Fatalf("labels/order = %q, %q, %q", out[0].Family, out[1].Family, out[2].Family)
	}
	// "others" collects EN chain + kodik + animejoy, in input order.
	var others []string
	for _, p := range out[1].Providers {
		others = append(others, p.Provider)
	}
	if want := []string{"gogoanime", "okru", "kodik", "animejoy-sibnet"}; !reflect.DeepEqual(others, want) {
		t.Fatalf("others = %v, want %v", others, want)
	}
	// "18+" collects adult + hanime.
	if len(out[2].Providers) != 2 {
		t.Fatalf("18+ providers = %d, want 2", len(out[2].Providers))
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd services/catalog && go test ./internal/service/capability/ -run TestRegroupFamilies`
Expected: FAIL — `undefined: regroupFamilies`.

- [ ] **Step 3: Implement `familyLabel` + `regroupFamilies` and call from `buildFamilies`**

Add to `services/catalog/internal/service/capability/service.go`:

```go
// familyLabel maps an internally-assembled family string to the collapsed wire
// taxonomy: "18+" (adult sources), "aeProvider" (first-party standalone), or
// "others" (every language provider — EN chain, kodik, animelib, animejoy legs).
func familyLabel(internal string) string {
	switch internal {
	case "hanime", "adult":
		return "18+"
	case "ae":
		return "aeProvider"
	default:
		return "others"
	}
}

// regroupFamilies collapses the internally-assembled per-source families into the
// three wire families {aeProvider, others, 18+}. Providers are bucketed by
// familyLabel, preserving input order within a bucket; buckets are emitted in
// first-seen order (deterministic). PURE — the FE re-sorts by state/order, so the
// merged intra-family order is not display-authoritative.
func regroupFamilies(in []domain.SourceFamily) []domain.SourceFamily {
	order := []string{}
	byLabel := map[string][]domain.ProviderCap{}
	for _, fam := range in {
		label := familyLabel(fam.Family)
		if _, seen := byLabel[label]; !seen {
			order = append(order, label)
		}
		byLabel[label] = append(byLabel[label], fam.Providers...)
	}
	out := make([]domain.SourceFamily, 0, len(order))
	for _, label := range order {
		out = append(out, domain.SourceFamily{Family: label, Providers: byLabel[label]})
	}
	return out
}
```

In `buildFamilies`, change the final `return families, nil` to:

```go
	return regroupFamilies(families), nil
```

- [ ] **Step 4: Update the wire doc comment**

In `services/catalog/internal/domain/capability.go`, change the `SourceFamily.Family` comment:

```go
type SourceFamily struct {
	Family    string        `json:"family"` // "18+" | "others" | "aeProvider"
	Providers []ProviderCap `json:"providers"`
}
```

- [ ] **Step 5: Run the capability package tests**

Run: `cd services/catalog && go test ./internal/service/capability/ -count=1`
Expected: PASS (including the existing `BuildENFamily` test asserting the internal `"ourenglish"` label, which is unchanged).

- [ ] **Step 6: Commit**

```bash
git add services/catalog/internal/service/capability/service.go \
  services/catalog/internal/service/capability/service_test.go \
  services/catalog/internal/domain/capability.go
git commit -m "$(cat <<'EOF'
refactor(catalog): collapse wire capability `family` to {18+, others, aeProvider}

regroupFamilies buckets the assembled per-source families into the three-value
wire taxonomy. `group` (the functional facet) is unchanged; the FE already reads
`group`, so this is behavior-preserving.

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
)"
```

---

### Task 3: Docs — record the taxonomy + kodik-iframe standalone

**Files:**
- Modify: `docs/aeplayer-reference.md`

- [ ] **Step 1: Add the taxonomy note**

In `docs/aeplayer-reference.md`, in (or near) the source-families section, add a subsection documenting: the wire `family` field is one of `"18+"` (hanime, 18anime), `"others"` (EN scraper chain, kodik HLS, animelib, animejoy-sibnet/allvideo), or `"aeProvider"` (self-hosted, standalone — audio/lang per-title); `group` (`en`/`ru`/`adult`/`firstparty`) is the functional language/relevance facet and EN-chain routing keys off `group === 'en'`; and the **Classic Kodik iframe** (`KodikPlayer.vue`) is a separate standalone surface on the anime page — it is NOT part of aePlayer and never appears in the capability feed (distinct from the in-aePlayer Kodik HLS provider, which lives in `"others"`).

- [ ] **Step 2: Commit**

```bash
git add docs/aeplayer-reference.md
git commit -m "$(cat <<'EOF'
docs(player): record collapsed family taxonomy + kodik-iframe standalone

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
)"
```

---

## Self-Review

**Spec coverage (Phase D section of the design):**
- Wire `family` → `{"18+","others","aeProvider"}` → Task 2 (`regroupFamilies` + `familyLabel`).
- Migrate combo-language shortcut `family === 'ourenglish'` → `group === 'en'` → Task 1 (`providerToLegacyPlayer`).
- Migrate resolver EN-chain dispatch → Task 1 (`useProviderResolver`/`makeResolver`).
- `familyOfProvider` → `groupOfProvider` + FE type update → Task 1.
- kodik-iframe documented standalone; both Kodik surfaces kept → Task 3 (and `group ru` kodik stays in `others` via Task 2).
- Behavior-preserving + deploy-order hazard → Global Constraints; verified by unchanged `rowsFromReport` tests (group-driven) and the green `BuildENFamily` test.

**Placeholder scan:** none — every step carries exact code/paths/commands.

**Type consistency:** `providerToLegacyPlayer(id, group)`, `comboToWatchCombo(combo, group)`, `groupOfProvider(report, id)`, `ResolverDeps.groupOf`, `useProviderResolver(groupOf)`, `regroupFamilies([]SourceFamily) []SourceFamily`, `familyLabel(string) string`, and the `SourceFamily.family` union `'18+'|'others'|'aeProvider'` are used consistently across tasks.

## Notes for execution

- After all three tasks, run `/animeenigma-after-update` (redeploys web + catalog, updates changelog, pushes). Given the deploy-order hazard, ensure the web bundle (Task 1) is deployed no later than catalog (Task 2) — deploying together is safe; catalog-first is not.
- Manual smoke after deploy: open a known EN title, confirm an EN provider still resolves/plays (EN-chain routing intact), and confirm the feed's `family` values are now `18+`/`others`/`aeProvider` (`curl .../capabilities`).
