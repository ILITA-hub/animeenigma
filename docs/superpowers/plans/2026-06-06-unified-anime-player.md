# Unified Anime Player ("Neon Tokyo") Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a new unified, fully-branded "Neon Tokyo" video player to AnimeEnigma — one Vue component owning its own `<video>` + `hls.js` core and a custom control bar — wired non-destructively as a new pill in the `RU EN 18+ RAW JP` selector, with providers as swappable stream-backend adapters and live provider-health-driven chip tinting.

**Architecture:** A thin `UnifiedPlayer.vue` orchestrator over four decoupled layers: state (`usePlayerState`), video engine (`useVideoEngine`), source backends (`useProviderResolver` + per-provider adapters), and health (`useProviderHealth`). Presentational components are dumb (props-in/events-out) and reuse existing primitives. Styling binds to design-system tokens; the vendored prototype is the pixel source of truth.

**Tech Stack:** Vue 3 `<script setup lang="ts">`, Vite, `hls.js@~1.5.20`, Vitest + Vue Test Utils, Tailwind v4 + design-system tokens, Go (catalog service) for the one backend endpoint extension, i18n (en/ru/ja).

**Spec:** `docs/superpowers/specs/2026-06-06-unified-anime-player-design.md`
**Pixel source of truth:** `docs/superpowers/specs/assets/unified-player-prototype/` (`Player.jsx`, `player.css`, `WatchPage.jsx`, `Icons.jsx`). Recreate the visual output pixel-perfectly in Vue; do NOT copy the React structure.

---

## Conventions for every task

- Frontend dir: run commands from `frontend/web/`.
- Test a component: `bunx vitest run <path>`. Type-check: `bunx tsc --noEmit`.
- DS lint: `bash frontend/web/scripts/design-system-lint.sh` (fail-path: `--selftest`).
- New player code lives under `frontend/web/src/components/player/unified/`.
- New composables under `frontend/web/src/composables/unifiedPlayer/`.
- Commit after every task with the project co-authors:
  ```
  Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
  Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
  Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
  ```
- Vue template rule: never place a non-conditional element between `v-if`/`v-else-if`; new player mounts AFTER the existing chain.

---

## File Structure

**Create (frontend):**
- `src/types/unifiedPlayer.ts` — shared types (single source).
- `src/components/player/unified/providerRegistry.ts` — static provider definitions.
- `src/composables/unifiedPlayer/useProviderHealth.ts` + `.spec.ts`
- `src/composables/unifiedPlayer/useProviderResolver.ts` + adapters + `.spec.ts`
- `src/composables/unifiedPlayer/usePlayerState.ts` + `.spec.ts`
- `src/composables/unifiedPlayer/useVideoEngine.ts` + `.spec.ts`
- `src/components/player/unified/UnifiedPlayer.vue`
- `src/components/player/unified/PlayerControlBar.vue`
- `src/components/player/unified/PlayerScrubBar.vue` + `.spec.ts`
- `src/components/player/unified/SourcePanel.vue` + `.spec.ts`
- `src/components/player/unified/ProviderChip.vue` + `.spec.ts`
- `src/components/player/unified/PlaybackSettingsMenu.vue`
- `src/components/player/unified/SubtitlesMenu.vue`
- `src/components/player/unified/BrowseSubsModal.vue` + `.spec.ts`
- `src/components/player/unified/overlays/{SkipIntroChip,NextEpisodeCard,BigPlayButton,WatchTogetherButton}.vue`
- `src/components/player/unified/unified-player.css`

**Modify:**
- `src/api/client.ts` — add `scraperApi.getProviders()` (merged rows).
- `src/views/Anime.vue` — new selector pill + mount branch + flag.
- `src/locales/{en,ru,ja}.json` — `player.unified.*` keys.

**Modify (backend, one task):**
- `services/scraper/internal/...` — include registry metadata (enabled/reason/description) in the health payload.
- `services/catalog/internal/handler/...` — passthrough already exists; verify shape.

---

## Task 1: Shared types

**Files:**
- Create: `frontend/web/src/types/unifiedPlayer.ts`

- [ ] **Step 1: Create the types file**

```typescript
// frontend/web/src/types/unifiedPlayer.ts
// Single source of truth for unified-player types. Imported by composables,
// the provider registry, and components — keep names stable across tasks.

export type AudioKind = 'sub' | 'dub'
export type TrackLang = 'en' | 'ru' | 'ja'
export type ContentKind = 'common' | 'hentai'
export type ProviderGroup = 'en' | 'ru' | 'adult' | 'raw' | 'first-party'

/** Static definition of a selectable backend provider. */
export interface ProviderDef {
  id: string                 // 'allanime', 'kodik', 'ae', ...
  name: string               // display label
  hue: string                // identity hue (brand-exempt: cyan/orange/pink/rose)
  group: ProviderGroup
  audios: AudioKind[]        // audio kinds this backend can serve
  langs: TrackLang[]         // track languages this backend serves
  content: ContentKind[]     // which content kinds it serves
  scraper: boolean           // true => live health comes from /scraper/health
  /** Non-scraper backends that are hard-disabled or WIP carry their reason here. */
  staticDisabled?: { reason: string; description: string; wip?: boolean }
}

export type ChipState = 'active' | 'disabled' | 'down' | 'irrelevant' | 'wip'

/** A provider as rendered in the Source panel: definition + computed state. */
export interface ProviderRow {
  def: ProviderDef
  state: ChipState
  /** Hover/tooltip text for non-active states. */
  reason?: string
}

/** Live + registry health for one scraper provider (from the backend). */
export interface ScraperProviderHealth {
  name: string
  enabled: boolean
  up: boolean
  reason?: string
  description?: string
}

/** The user's current source selection. */
export interface Combo {
  audio: AudioKind
  lang: TrackLang
  provider: string
  server: string
  team: string | null
}

/** Normalised stream descriptor returned by a provider adapter. */
export interface StreamResult {
  url: string
  type: 'hls' | 'mp4'
  headers?: Record<string, string>
  qualities?: { label: string; value: number | string }[]
  servers?: { id: string; label: string }[]
}
```

- [ ] **Step 2: Type-check**

Run: `cd frontend/web && bunx tsc --noEmit`
Expected: PASS (no references yet).

- [ ] **Step 3: Commit**

```bash
git add frontend/web/src/types/unifiedPlayer.ts
git commit -m "feat(player): unified-player shared types"
```

---

## Task 2: Provider registry

**Files:**
- Create: `frontend/web/src/components/player/unified/providerRegistry.ts`

Non-scraper backends (kodik/animelib/hanime/raw) are NOT in `docker/scraper-providers.yaml`, so their default state lives here. Scraper backends (allanime/animefever/gogoanime/miruro/nineanime/animepahe/18anime) get live state from the backend (Task 4); their registry entry just provides display + filter metadata.

- [ ] **Step 1: Create the registry**

```typescript
// frontend/web/src/components/player/unified/providerRegistry.ts
import type { ProviderDef } from '@/types/unifiedPlayer'

// Identity hues are the design-system "brand-exempt" hues (NOT the lint-forbidden
// palette): cyan/orange/pink/rose. Keep hex here (allowlisted in DS task).
export const PROVIDER_REGISTRY: ProviderDef[] = [
  // First-party — WIP, always inactive for now.
  { id: 'ae', name: 'AnimeEnigma', hue: '#00d4ff', group: 'first-party',
    audios: ['sub', 'dub'], langs: ['en', 'ru'], content: ['common'], scraper: false,
    staticDisabled: { reason: 'WIP', description: 'We are working on our own hosting', wip: true } },

  // EN scraper providers (live health from backend).
  { id: 'allanime',   name: 'AllAnime',   hue: '#00d4ff', group: 'en', audios: ['sub', 'dub'], langs: ['en'], content: ['common'], scraper: true },
  { id: 'animefever', name: 'AnimeFever', hue: '#00d4ff', group: 'en', audios: ['sub', 'dub'], langs: ['en'], content: ['common'], scraper: true },
  { id: 'gogoanime',  name: 'Gogoanime',  hue: '#00d4ff', group: 'en', audios: ['sub', 'dub'], langs: ['en'], content: ['common'], scraper: true },
  { id: 'miruro',     name: 'Miruro',     hue: '#00d4ff', group: 'en', audios: ['sub', 'dub'], langs: ['en'], content: ['common'], scraper: true },
  { id: 'nineanime',  name: '9anime',     hue: '#00d4ff', group: 'en', audios: ['sub'],        langs: ['en'], content: ['common'], scraper: true },
  { id: 'animepahe',  name: 'AnimePahe',  hue: '#00d4ff', group: 'en', audios: ['sub', 'dub'], langs: ['en'], content: ['common'], scraper: true },

  // RU.
  { id: 'kodik',   name: 'Kodik',  hue: '#22d3ee', group: 'ru', audios: ['dub', 'sub'], langs: ['ru'], content: ['common'], scraper: false },
  { id: 'animelib', name: 'AniLib', hue: '#ff8a3d', group: 'ru', audios: ['sub'], langs: ['ru'], content: ['common'], scraper: false,
    staticDisabled: { reason: 'Unavailable', description: 'AniLib direct streams are not currently working' } },

  // 18+ (adult group). 18anime is a scraper provider but in the adult orchestrator.
  { id: 'hanime',  name: 'Hanime', hue: '#ff4d8d', group: 'adult', audios: ['dub'], langs: ['ru'], content: ['hentai'], scraper: false,
    staticDisabled: { reason: 'Unavailable', description: 'Hanime streams are not currently working' } },
  { id: '18anime', name: '18anime', hue: '#fb7185', group: 'adult', audios: ['sub', 'dub'], langs: ['en'], content: ['hentai'], scraper: true },

  // JP raw.
  { id: 'raw', name: 'Raw', hue: '#fb7185', group: 'raw', audios: ['sub'], langs: ['ja'], content: ['common'], scraper: false },
]

export const providerById = (id: string): ProviderDef | undefined =>
  PROVIDER_REGISTRY.find(p => p.id === id)
```

- [ ] **Step 2: Type-check + commit**

```bash
cd frontend/web && bunx tsc --noEmit
git add frontend/web/src/components/player/unified/providerRegistry.ts
git commit -m "feat(player): unified-player provider registry"
```

---

## Task 3: Backend — include registry metadata in scraper health

The scraper already loads `config.ProvidersConfig` (`ProviderMeta{Name,Enabled,Reason,Description}`) and a live `health` cache. The `/scraper/health` JSON must expose, per provider: `enabled`, `up`, `reason`, `description`. Find the handler that builds the health snapshot.

**Files:**
- Locate: `grep -rn "providers" services/scraper/internal --include=*.go | grep -i "health\|snapshot\|MarshalJSON\|Handle"`
- Modify: the scraper health handler/serializer.
- Test: alongside the modified file.

- [ ] **Step 1: Find the health serializer**

Run: `grep -rnE "func.*Health|\"providers\"|json:\"providers\"" services/scraper/internal/transport services/scraper/internal/service --include=*.go`
Expected: the function assembling the `providers` map in the `/scraper/health` response.

- [ ] **Step 2: Write the failing test**

In the health handler's `_test.go`, assert the JSON for a provider includes registry fields. Adapt names to the real types found in Step 1:

```go
func TestScraperHealth_IncludesRegistryMeta(t *testing.T) {
	// Build the handler with a ProvidersConfig where 'animepahe' is disabled
	// with a reason, and a health cache marking 'allanime' up.
	// ... construct per the file's existing test helpers ...
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/scraper/health", nil)
	router.ServeHTTP(rec, req)

	var body struct {
		Providers map[string]struct {
			Enabled     bool   `json:"enabled"`
			Up          bool   `json:"up"`
			Reason      string `json:"reason"`
			Description string `json:"description"`
		} `json:"providers"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.Providers["animepahe"].Enabled {
		t.Errorf("animepahe must be enabled=false")
	}
	if body.Providers["animepahe"].Reason == "" {
		t.Errorf("animepahe must carry a reason")
	}
	if !body.Providers["allanime"].Up {
		t.Errorf("allanime must be up=true")
	}
}
```

- [ ] **Step 3: Run it — expect FAIL**

Run: `cd services/scraper && go test ./internal/transport/... -run TestScraperHealth_IncludesRegistryMeta`
Expected: FAIL (fields absent / zero).

- [ ] **Step 4: Implement — merge registry meta into the per-provider health row**

In the serializer, for each provider name, read `ProvidersConfig` (`IsEnabled(name)` + the `ProviderMeta` reason/description) and the health cache `up`, and emit all four fields. Keep the existing `stages`/`last_updated` fields intact (additive change).

- [ ] **Step 5: Run it — expect PASS**

Run: `cd services/scraper && go test ./internal/transport/...`
Expected: PASS.

- [ ] **Step 6: Verify catalog passthrough shape unchanged**

Run: `cd services/catalog && go test ./internal/transport/... -run Scraper`
Expected: PASS (catalog forwards the body verbatim).

- [ ] **Step 7: Commit**

```bash
git add services/scraper/internal
git commit -m "feat(scraper): expose provider enabled/reason/description in /scraper/health"
```

---

## Task 4: `useProviderHealth` composable

Polls `/api/anime/_/scraper/health`, merges with `PROVIDER_REGISTRY`, and computes a `ProviderRow` per provider given the active `Combo` + title content kind.

**Files:**
- Create: `frontend/web/src/api/client.ts` already has `scraperApi.getHealth()`. Reuse it.
- Create: `frontend/web/src/composables/unifiedPlayer/useProviderHealth.ts`
- Test: `frontend/web/src/composables/unifiedPlayer/useProviderHealth.spec.ts`

- [ ] **Step 1: Write the failing test (chip-state matrix)**

```typescript
// useProviderHealth.spec.ts
import { describe, it, expect } from 'vitest'
import { computeProviderRows } from './useProviderHealth'
import type { ScraperProviderHealth } from '@/types/unifiedPlayer'

const health = (over: Partial<ScraperProviderHealth> & { name: string }): ScraperProviderHealth =>
  ({ enabled: true, up: true, ...over })

describe('computeProviderRows', () => {
  it('marks a healthy, relevant scraper provider active', () => {
    const rows = computeProviderRows(
      [health({ name: 'allanime' })],
      { audio: 'sub', lang: 'en', content: 'common' },
    )
    expect(rows.find(r => r.def.id === 'allanime')!.state).toBe('active')
  })

  it('marks a registry-disabled scraper provider disabled with its reason', () => {
    const rows = computeProviderRows(
      [health({ name: 'animepahe', enabled: false, reason: 'Cloudflare challenge', description: 'sidecar 0% solve' })],
      { audio: 'sub', lang: 'en', content: 'common' },
    )
    const r = rows.find(r => r.def.id === 'animepahe')!
    expect(r.state).toBe('disabled')
    expect(r.reason).toContain('Cloudflare')
  })

  it('marks an up=false scraper provider down', () => {
    const rows = computeProviderRows(
      [health({ name: 'gogoanime', up: false })],
      { audio: 'sub', lang: 'en', content: 'common' },
    )
    expect(rows.find(r => r.def.id === 'gogoanime')!.state).toBe('down')
  })

  it('marks a non-scraper hard-disabled provider disabled (animelib)', () => {
    const rows = computeProviderRows([], { audio: 'sub', lang: 'ru', content: 'common' })
    expect(rows.find(r => r.def.id === 'animelib')!.state).toBe('disabled')
  })

  it('marks AnimeEnigma wip', () => {
    const rows = computeProviderRows([], { audio: 'sub', lang: 'en', content: 'common' })
    expect(rows.find(r => r.def.id === 'ae')!.state).toBe('wip')
  })

  it('marks 18anime irrelevant on a common title', () => {
    const rows = computeProviderRows(
      [health({ name: '18anime' })],
      { audio: 'sub', lang: 'en', content: 'common' },
    )
    expect(rows.find(r => r.def.id === '18anime')!.state).toBe('irrelevant')
  })

  it('marks a provider irrelevant when audio/lang mismatch (raw on sub/en)', () => {
    const rows = computeProviderRows([], { audio: 'sub', lang: 'en', content: 'common' })
    expect(rows.find(r => r.def.id === 'raw')!.state).toBe('irrelevant')
  })
})
```

- [ ] **Step 2: Run — expect FAIL**

Run: `cd frontend/web && bunx vitest run src/composables/unifiedPlayer/useProviderHealth.spec.ts`
Expected: FAIL (module/function missing).

- [ ] **Step 3: Implement**

```typescript
// useProviderHealth.ts
import { ref, onUnmounted, type Ref } from 'vue'
import { scraperApi } from '@/api/client'
import { PROVIDER_REGISTRY } from '@/components/player/unified/providerRegistry'
import type { ProviderRow, ScraperProviderHealth, AudioKind, TrackLang, ContentKind } from '@/types/unifiedPlayer'

export interface RowFilter { audio: AudioKind; lang: TrackLang; content: ContentKind }

/** Pure: registry + live scraper health + active filter → rendered rows. */
export function computeProviderRows(
  scraperHealth: ScraperProviderHealth[],
  filter: RowFilter,
): ProviderRow[] {
  const byName = new Map(scraperHealth.map(h => [h.name, h]))
  return PROVIDER_REGISTRY.map((def): ProviderRow => {
    // WIP first-party.
    if (def.staticDisabled?.wip) {
      return { def, state: 'wip', reason: def.staticDisabled.description }
    }
    // Non-scraper hard-disabled.
    if (def.staticDisabled) {
      return { def, state: 'disabled', reason: def.staticDisabled.description }
    }
    // Relevance: must serve the active audio, language, and content kind.
    const relevant =
      def.audios.includes(filter.audio) &&
      def.langs.includes(filter.lang) &&
      def.content.includes(filter.content)
    if (!relevant) {
      return { def, state: 'irrelevant', reason: relevanceReason(def, filter) }
    }
    // Scraper providers consult live health.
    if (def.scraper) {
      const h = byName.get(def.id)
      if (h && !h.enabled) return { def, state: 'disabled', reason: h.reason || h.description }
      if (h && !h.up) return { def, state: 'down', reason: 'Temporarily unreachable' }
    }
    return { def, state: 'active' }
  })
}

function relevanceReason(def: { content: ContentKind[] }, f: RowFilter): string {
  if (def.content.includes('hentai') && f.content !== 'hentai') return 'Only for 18+ titles'
  return `No ${f.audio}/${f.lang} stream from this source`
}

/** Live composable: polls health, exposes a setter for the active filter. */
export function useProviderHealth(filter: Ref<RowFilter>, intervalMs = 30_000) {
  const health = ref<ScraperProviderHealth[]>([])
  const rows = ref<ProviderRow[]>([])
  const recompute = () => { rows.value = computeProviderRows(health.value, filter.value) }

  async function poll() {
    try {
      const resp = await scraperApi.getHealth()
      const providers = (resp.data?.providers ?? {}) as Record<string, { enabled?: boolean; up?: boolean; reason?: string; description?: string }>
      health.value = Object.entries(providers).map(([name, v]) => ({
        name, enabled: v.enabled ?? true, up: v.up ?? false, reason: v.reason, description: v.description,
      }))
    } catch {
      // Fail soft: keep last known health; registry defaults still render.
    }
    recompute()
  }

  let timer: ReturnType<typeof setInterval> | null = null
  const start = () => { poll(); timer = setInterval(poll, intervalMs) }
  const stop = () => { if (timer) clearInterval(timer); timer = null }
  onUnmounted(stop)
  return { rows, recompute, start, stop }
}
```

- [ ] **Step 4: Run — expect PASS**

Run: `cd frontend/web && bunx vitest run src/composables/unifiedPlayer/useProviderHealth.spec.ts`
Expected: PASS (7 tests).

- [ ] **Step 5: Commit**

```bash
git add frontend/web/src/composables/unifiedPlayer/useProviderHealth.ts frontend/web/src/composables/unifiedPlayer/useProviderHealth.spec.ts
git commit -m "feat(player): useProviderHealth + chip-state matrix"
```

---

## Task 5: `usePlayerState` composable

Single reactive state + named actions. No DOM.

**Files:**
- Create: `frontend/web/src/composables/unifiedPlayer/usePlayerState.ts`
- Test: `frontend/web/src/composables/unifiedPlayer/usePlayerState.spec.ts`

- [ ] **Step 1: Write the failing test**

```typescript
import { describe, it, expect } from 'vitest'
import { usePlayerState } from './usePlayerState'

describe('usePlayerState', () => {
  it('defaults: paused-progress 0, sub/en, autoplay+autoskip OFF', () => {
    const s = usePlayerState()
    expect(s.progress.value).toBe(0)
    expect(s.combo.value.audio).toBe('sub')
    expect(s.combo.value.lang).toBe('en')
    expect(s.autoNext.value).toBe(false)
    expect(s.autoSkip.value).toBe(false)
  })

  it('setCombo replaces provider+server and keeps audio/lang', () => {
    const s = usePlayerState()
    s.setProvider('kodik', 'Server 1')
    expect(s.combo.value.provider).toBe('kodik')
    expect(s.combo.value.server).toBe('Server 1')
  })

  it('setAudio resets team to null', () => {
    const s = usePlayerState()
    s.setTeam('AniLibria')
    s.setAudio('dub')
    expect(s.combo.value.team).toBeNull()
  })
})
```

- [ ] **Step 2: Run — expect FAIL**

Run: `cd frontend/web && bunx vitest run src/composables/unifiedPlayer/usePlayerState.spec.ts`
Expected: FAIL.

- [ ] **Step 3: Implement**

```typescript
// usePlayerState.ts
import { ref } from 'vue'
import type { AudioKind, TrackLang, Combo } from '@/types/unifiedPlayer'

export function usePlayerState() {
  const playing = ref(false)
  const progress = ref(0)        // 0..100
  const volume = ref(80)
  const muted = ref(false)
  const quality = ref<string>('Auto')
  const speed = ref(1)
  const autoNext = ref(false)
  const autoSkip = ref(false)

  const combo = ref<Combo>({ audio: 'sub', lang: 'en', provider: '', server: '', team: null })

  const setAudio = (a: AudioKind) => { combo.value = { ...combo.value, audio: a, team: null } }
  const setLang = (l: TrackLang) => { combo.value = { ...combo.value, lang: l, team: null } }
  const setProvider = (id: string, server: string) => { combo.value = { ...combo.value, provider: id, server } }
  const setServer = (server: string) => { combo.value = { ...combo.value, server } }
  const setTeam = (team: string | null) => { combo.value = { ...combo.value, team } }

  // subtitle prefs
  const subLang = ref<'off' | TrackLang>('en')
  const subSize = ref(26)
  const subBg = ref(45)
  const subOffset = ref(0)

  return {
    playing, progress, volume, muted, quality, speed, autoNext, autoSkip, combo,
    subLang, subSize, subBg, subOffset,
    setAudio, setLang, setProvider, setServer, setTeam,
  }
}
export type PlayerState = ReturnType<typeof usePlayerState>
```

- [ ] **Step 4: Run — expect PASS. Step 5: Commit**

```bash
cd frontend/web && bunx vitest run src/composables/unifiedPlayer/usePlayerState.spec.ts
git add frontend/web/src/composables/unifiedPlayer/usePlayerState.ts frontend/web/src/composables/unifiedPlayer/usePlayerState.spec.ts
git commit -m "feat(player): usePlayerState"
```

---

## Task 6: Provider resolver + adapters

A registry of adapters that resolve episodes + a stream for a combo, reusing the existing API clients. Default provider for stage 1 = the EN scraper path (`ourenglish`).

**Files:**
- Create: `frontend/web/src/composables/unifiedPlayer/useProviderResolver.ts`
- Test: `frontend/web/src/composables/unifiedPlayer/useProviderResolver.spec.ts`

Inspect the existing clients first: `grep -n "getEpisodes\|getStream\|getServers" frontend/web/src/api/client.ts` (already mapped: `scraperApi`, plus `kodikAdfree`/`raw`/`anime18` clients near lines 660-712).

- [ ] **Step 1: Write the failing test (adapter dispatch + scraper mapping)**

```typescript
import { describe, it, expect, vi } from 'vitest'
import { makeResolver } from './useProviderResolver'

describe('useProviderResolver', () => {
  it('dispatches to the scraper adapter for an EN provider', async () => {
    const scraperApi = {
      getEpisodes: vi.fn().mockResolvedValue({ data: { episodes: [{ id: 'e1', number: 1 }], meta: { provider: 'allanime' } } }),
      getServers: vi.fn().mockResolvedValue({ data: { servers: [{ id: 's1', name: 'Server 1' }] } }),
      getStream: vi.fn().mockResolvedValue({ data: { url: 'http://x/m3u8', type: 'hls' } }),
    }
    const resolver = makeResolver({ scraperApi } as any)
    const eps = await resolver.listEpisodes('allanime', 'anime-uuid')
    expect(eps[0].number).toBe(1)
    const stream = await resolver.resolveStream('allanime', 'anime-uuid', eps[0], { audio: 'sub', lang: 'en', provider: 'allanime', server: 's1', team: null })
    expect(stream.type).toBe('hls')
    expect(scraperApi.getEpisodes).toHaveBeenCalledWith('anime-uuid', 'allanime')
  })

  it('throws a typed error for a disabled/unwired provider', async () => {
    const resolver = makeResolver({} as any)
    await expect(resolver.listEpisodes('animelib', 'x')).rejects.toThrow(/not available/i)
  })
})
```

- [ ] **Step 2: Run — expect FAIL**

Run: `cd frontend/web && bunx vitest run src/composables/unifiedPlayer/useProviderResolver.spec.ts`
Expected: FAIL.

- [ ] **Step 3: Implement (scraper adapter fully; others reuse their clients)**

```typescript
// useProviderResolver.ts
import { scraperApi as defaultScraperApi, rawApi, anime18Api, kodikAdfreeApi } from '@/api/client'
import type { EpisodeOption } from '@/components/player/EpisodeSelector.types'
import type { Combo, StreamResult } from '@/types/unifiedPlayer'

export interface ProviderAdapter {
  listEpisodes(animeId: string): Promise<EpisodeOption[]>
  resolveStream(animeId: string, ep: EpisodeOption, combo: Combo): Promise<StreamResult>
}

// Deps injected for testability.
export interface ResolverDeps {
  scraperApi: typeof defaultScraperApi
  rawApi?: typeof rawApi
  anime18Api?: typeof anime18Api
  kodikAdfreeApi?: typeof kodikAdfreeApi
}

class NotAvailableError extends Error {}

function scraperAdapter(deps: ResolverDeps, prefer: string): ProviderAdapter {
  return {
    async listEpisodes(animeId) {
      const resp = await deps.scraperApi.getEpisodes(animeId, prefer)
      const eps = resp.data?.episodes ?? []
      return eps.map((e: any) => ({ key: e.id, label: e.number, number: e.number }))
    },
    async resolveStream(animeId, ep, combo) {
      const srv = await deps.scraperApi.getServers(animeId, String(ep.key), prefer)
      const serverId = combo.server || srv.data?.servers?.[0]?.id
      const stream = await deps.scraperApi.getStream(animeId, ep.number, Number(serverId) || 0)
      return { url: stream.data?.url, type: (stream.data?.type ?? 'hls') as 'hls' | 'mp4' }
    },
  }
}

export function makeResolver(deps: ResolverDeps) {
  // EN + 18anime go through the scraper; others reuse their own clients.
  const SCRAPER_IDS = new Set(['allanime', 'animefever', 'gogoanime', 'miruro', 'nineanime', 'animepahe', '18anime'])

  function adapterFor(providerId: string): ProviderAdapter {
    if (SCRAPER_IDS.has(providerId)) return scraperAdapter(deps, providerId)
    // Stage 1: kodik(adfree)/raw wired via their existing clients; animelib/hanime/ae unavailable.
    if (providerId === 'raw' && deps.rawApi) return rawAdapter(deps)
    if (providerId === 'kodik' && deps.kodikAdfreeApi) return kodikAdapter(deps)
    throw new NotAvailableError(`Provider "${providerId}" is not available yet`)
  }

  return {
    listEpisodes: (providerId: string, animeId: string) => adapterFor(providerId).listEpisodes(animeId),
    resolveStream: (providerId: string, animeId: string, ep: EpisodeOption, combo: Combo) =>
      adapterFor(providerId).resolveStream(animeId, ep, combo),
  }
}

// raw (AllAnime JP) — reuse rawApi.getEpisodes/getStream (see api/client.ts ~700-712).
function rawAdapter(deps: ResolverDeps): ProviderAdapter {
  return {
    async listEpisodes(animeId) {
      const resp = await deps.rawApi!.getEpisodes(animeId)
      return (resp.data?.episodes ?? []).map((e: any) => ({ key: e.slug ?? e.id, label: e.number, number: e.number }))
    },
    async resolveStream(animeId, ep) {
      const resp = await deps.rawApi!.getStream(animeId, String(ep.key))
      return { url: resp.data?.url, type: (resp.data?.type ?? 'hls') as 'hls' | 'mp4' }
    },
  }
}

// kodik (ad-free HLS via kodikextract) — reuse kodikAdfreeApi.
function kodikAdapter(deps: ResolverDeps): ProviderAdapter {
  return {
    async listEpisodes(animeId) {
      const resp = await deps.kodikAdfreeApi!.getEpisodes(animeId)
      return (resp.data?.episodes ?? []).map((e: any) => ({ key: e.id ?? e.number, label: e.number, number: e.number }))
    },
    async resolveStream(animeId, ep, combo) {
      const resp = await deps.kodikAdfreeApi!.getStream(animeId, ep.number, Number(combo.server) || 0)
      return { url: resp.data?.url, type: (resp.data?.type ?? 'hls') as 'hls' | 'mp4' }
    },
  }
}

export function useProviderResolver() {
  return makeResolver({ scraperApi: defaultScraperApi, rawApi, anime18Api, kodikAdfreeApi })
}
```

NOTE for the implementer: confirm the exact exported client names/shapes in `src/api/client.ts` (lines ~624-712). If `kodikAdfreeApi`/`rawApi`/`anime18Api` are named differently, rename imports to match — do NOT invent endpoints. If a client's method signature differs, adapt the adapter body; the `StreamResult` return shape stays fixed.

- [ ] **Step 4: Run — expect PASS. Step 5: Commit**

```bash
cd frontend/web && bunx vitest run src/composables/unifiedPlayer/useProviderResolver.spec.ts
git add frontend/web/src/composables/unifiedPlayer/useProviderResolver.ts frontend/web/src/composables/unifiedPlayer/useProviderResolver.spec.ts
git commit -m "feat(player): provider resolver + adapters"
```

---

## Task 7: `useVideoEngine` composable

Wraps `<video>` + lazy `hls.js`. Loads a `StreamResult`, exposes levels/buffered, recovers from fatal errors.

**Files:**
- Create: `frontend/web/src/composables/unifiedPlayer/useVideoEngine.ts`
- Test: `frontend/web/src/composables/unifiedPlayer/useVideoEngine.spec.ts`

- [ ] **Step 1: Write the failing test (mp4 vs hls branch selection)**

```typescript
import { describe, it, expect, vi } from 'vitest'
import { chooseLoadStrategy } from './useVideoEngine'

describe('chooseLoadStrategy', () => {
  it('uses native src for mp4', () => {
    expect(chooseLoadStrategy({ url: 'a.mp4', type: 'mp4' }, false)).toBe('native')
  })
  it('uses native src for hls when the browser supports it (Safari)', () => {
    expect(chooseLoadStrategy({ url: 'a.m3u8', type: 'hls' }, true)).toBe('native')
  })
  it('uses hls.js for hls when the browser lacks native HLS', () => {
    expect(chooseLoadStrategy({ url: 'a.m3u8', type: 'hls' }, false)).toBe('hlsjs')
  })
})
```

- [ ] **Step 2: Run — expect FAIL.**

Run: `cd frontend/web && bunx vitest run src/composables/unifiedPlayer/useVideoEngine.spec.ts`

- [ ] **Step 3: Implement**

```typescript
// useVideoEngine.ts
import { ref, onUnmounted, type Ref } from 'vue'
import type { StreamResult } from '@/types/unifiedPlayer'

export type LoadStrategy = 'native' | 'hlsjs'

export function chooseLoadStrategy(stream: StreamResult, nativeHls: boolean): LoadStrategy {
  if (stream.type === 'mp4') return 'native'
  return nativeHls ? 'native' : 'hlsjs'
}

export function useVideoEngine(videoEl: Ref<HTMLVideoElement | null>) {
  const fatal = ref<string | null>(null)
  let hls: any = null

  function nativeHlsSupported(v: HTMLVideoElement): boolean {
    return v.canPlayType('application/vnd.apple.mpegurl') !== ''
  }

  async function load(stream: StreamResult) {
    const v = videoEl.value
    if (!v) return
    fatal.value = null
    destroy()
    const strategy = chooseLoadStrategy(stream, nativeHlsSupported(v))
    if (strategy === 'native') { v.src = stream.url; return }
    const Hls = (await import('hls.js')).default
    if (!Hls.isSupported()) { v.src = stream.url; return }
    hls = new Hls({ enableWorker: true })
    hls.loadSource(stream.url)
    hls.attachMedia(v)
    hls.on(Hls.Events.ERROR, (_e: unknown, data: any) => {
      if (!data?.fatal) return
      if (data.type === 'networkError') hls.startLoad()
      else if (data.type === 'mediaError') hls.recoverMediaError()
      else { fatal.value = 'unrecoverable'; destroy() }
    })
  }

  function destroy() { if (hls) { hls.destroy(); hls = null } }
  onUnmounted(destroy)
  return { fatal, load, destroy }
}
```

- [ ] **Step 4: Run — expect PASS. Step 5: Commit**

```bash
cd frontend/web && bunx vitest run src/composables/unifiedPlayer/useVideoEngine.spec.ts
git add frontend/web/src/composables/unifiedPlayer/useVideoEngine.ts frontend/web/src/composables/unifiedPlayer/useVideoEngine.spec.ts
git commit -m "feat(player): useVideoEngine (lazy hls.js + recovery)"
```

---

## Task 8: `ProviderChip.vue`

Renders one provider with its health state + tooltip. Pure presentational.

**Files:**
- Create: `frontend/web/src/components/player/unified/ProviderChip.vue`
- Test: `frontend/web/src/components/player/unified/ProviderChip.spec.ts`

- [ ] **Step 1: Write the failing spec**

```typescript
import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import ProviderChip from './ProviderChip.vue'
import type { ProviderRow } from '@/types/unifiedPlayer'

const row = (over: Partial<ProviderRow>): ProviderRow => ({
  def: { id: 'allanime', name: 'AllAnime', hue: '#00d4ff', group: 'en', audios: ['sub'], langs: ['en'], content: ['common'], scraper: true },
  state: 'active', ...over,
} as ProviderRow)

describe('ProviderChip', () => {
  it('renders the provider name', () => {
    expect(mount(ProviderChip, { props: { row: row({}) } }).text()).toContain('AllAnime')
  })
  it('emits select when active and clicked', async () => {
    const w = mount(ProviderChip, { props: { row: row({ state: 'active' }) } })
    await w.find('button').trigger('click')
    expect(w.emitted('select')).toBeTruthy()
  })
  it('is disabled and does NOT emit when state is disabled', async () => {
    const w = mount(ProviderChip, { props: { row: row({ state: 'disabled', reason: 'Cloudflare challenge' }) } })
    await w.find('button').trigger('click')
    expect(w.emitted('select')).toBeFalsy()
    expect(w.find('button').attributes('disabled')).toBeDefined()
  })
  it('exposes the reason as a title/tooltip for non-active states', () => {
    const w = mount(ProviderChip, { props: { row: row({ state: 'wip', reason: 'We are working on our own hosting' }) } })
    expect(w.html()).toContain('We are working on our own hosting')
  })
  it('marks the active selection', () => {
    const w = mount(ProviderChip, { props: { row: row({}), selected: true } })
    expect(w.classes().join(' ')).toMatch(/is-active|is-selected/)
  })
})
```

- [ ] **Step 2: Run — expect FAIL.** Run: `bunx vitest run src/components/player/unified/ProviderChip.spec.ts`

- [ ] **Step 3: Implement the SFC**

Template contract (style per prototype `.pl-src-item` in vendored `player.css`):
- A `<button>` with `:disabled="row.state !== 'active'"`, `:title="row.reason"`.
- Identity-hue dot (`:style="{ background: row.def.hue }"`).
- Name span; a `<Tooltip>` (reuse `@/components/ui/Tooltip.vue`) or native `title` carrying `row.reason`.
- Active check icon when `selected`.
- `@click="row.state === 'active' && $emit('select')"`.
- Props: `{ row: ProviderRow; selected?: boolean }`. Emits: `select`.
- Use semantic tokens; tinted state = reduced opacity + `cursor-not-allowed`.

- [ ] **Step 4: Run — expect PASS. Step 5: Commit**

```bash
git add frontend/web/src/components/player/unified/ProviderChip.vue frontend/web/src/components/player/unified/ProviderChip.spec.ts
git commit -m "feat(player): ProviderChip with health states"
```

---

## Task 9: `SourcePanel.vue`

Audio + Language sliders (filters) → Team chips → Provider list (`ProviderChip`) → Server list. Drives `usePlayerState` + consumes `useProviderHealth` rows.

**Files:**
- Create: `frontend/web/src/components/player/unified/SourcePanel.vue`
- Test: `frontend/web/src/components/player/unified/SourcePanel.spec.ts`

- [ ] **Step 1: Write the failing spec**

```typescript
import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import SourcePanel from './SourcePanel.vue'
import type { ProviderRow } from '@/types/unifiedPlayer'

const rows: ProviderRow[] = [
  { def: { id: 'allanime', name: 'AllAnime', hue: '#0df', group: 'en', audios: ['sub'], langs: ['en'], content: ['common'], scraper: true }, state: 'active' },
  { def: { id: 'animepahe', name: 'AnimePahe', hue: '#0df', group: 'en', audios: ['sub'], langs: ['en'], content: ['common'], scraper: true }, state: 'disabled', reason: 'Cloudflare challenge' },
]

const baseProps = {
  rows, audio: 'sub', lang: 'en', team: null, provider: 'allanime', server: 's1',
  servers: [{ id: 's1', label: 'Server 1' }], teams: [] as string[],
}

describe('SourcePanel', () => {
  it('renders a chip per provider row', () => {
    const w = mount(SourcePanel, { props: baseProps as any })
    expect(w.findAll('[data-test="provider-chip"]').length).toBe(2)
  })
  it('emits update:audio when the Dub slider option is clicked', async () => {
    const w = mount(SourcePanel, { props: baseProps as any })
    await w.find('[data-test="audio-dub"]').trigger('click')
    expect(w.emitted('update:audio')?.[0]).toEqual(['dub'])
  })
  it('emits select-provider only for active chips', async () => {
    const w = mount(SourcePanel, { props: baseProps as any })
    await w.find('[data-test="provider-chip"][data-id="allanime"] button').trigger('click')
    expect(w.emitted('select-provider')?.[0]).toEqual(['allanime'])
  })
})
```

- [ ] **Step 2: Run — expect FAIL.**

- [ ] **Step 3: Implement the SFC**

Contract (style per prototype `.pl-srcpanel`, `.pl-bigfilters`, `.pl-slider`, `.pl-team-chips`, `.pl-src-list` in vendored `player.css`):
- Props: `{ rows: ProviderRow[]; audio: AudioKind; lang: TrackLang; team: string|null; provider: string; server: string; servers: {id,label}[]; teams: string[] }`.
- Emits: `update:audio`, `update:lang`, `update:team`, `select-provider`, `select-server`.
- Two sliders: Audio (`sub`/`dub`), Language (`en`/`ru`/`ja`). Each option carries a `data-test` (`audio-sub`, `audio-dub`, `lang-en`…).
- Team chips from `teams` (hidden when empty).
- Provider list: `<ProviderChip v-for="r in rows" :row="r" :selected="r.def.id===provider" data-test="provider-chip" :data-id="r.def.id" @select="$emit('select-provider', r.def.id)" />`.
- Server list: buttons from `servers`, active = `server`.

- [ ] **Step 4: Run — expect PASS. Step 5: Commit**

```bash
git add frontend/web/src/components/player/unified/SourcePanel.vue frontend/web/src/components/player/unified/SourcePanel.spec.ts
git commit -m "feat(player): SourcePanel (audio/lang filters → providers → servers)"
```

---

## Task 10: `PlayerScrubBar.vue`

Scrub track: buffered + fill + hover-preview thumb + intro/outro chapter markers. Markers only render when timings are provided.

**Files:**
- Create: `frontend/web/src/components/player/unified/PlayerScrubBar.vue`
- Test: `frontend/web/src/components/player/unified/PlayerScrubBar.spec.ts`

- [ ] **Step 1: Write the failing spec**

```typescript
import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import PlayerScrubBar from './PlayerScrubBar.vue'

describe('PlayerScrubBar', () => {
  it('renders fill at the given progress', () => {
    const w = mount(PlayerScrubBar, { props: { progress: 40, buffered: 55, durationSec: 1421, chapters: [] } })
    expect(w.find('[data-test="fill"]').attributes('style')).toContain('40%')
  })
  it('renders NO chapter markers when none provided', () => {
    const w = mount(PlayerScrubBar, { props: { progress: 0, buffered: 0, durationSec: 1421, chapters: [] } })
    expect(w.findAll('[data-test="chapter"]').length).toBe(0)
  })
  it('renders chapter markers when provided', () => {
    const w = mount(PlayerScrubBar, { props: { progress: 0, buffered: 0, durationSec: 1421, chapters: [{ kind: 'intro', startPct: 2, widthPct: 5 }] } })
    expect(w.findAll('[data-test="chapter"]').length).toBe(1)
  })
  it('emits seek with a 0..100 pct on click', async () => {
    const w = mount(PlayerScrubBar, { props: { progress: 0, buffered: 0, durationSec: 1421, chapters: [] } })
    await w.find('[data-test="track"]').trigger('click', { clientX: 50 })
    expect(w.emitted('seek')).toBeTruthy()
  })
})
```

- [ ] **Step 2: Run — expect FAIL. Step 3: Implement** (style per `.pl-track/.pl-fill/.pl-buffered/.pl-chapter/.pl-preview`). Props `{ progress, buffered, durationSec, chapters: {kind,startPct,widthPct}[], stillUrl? }`; emits `seek` (0..100) + `hover` (pct|null). Use a single positioned preview node; compute pct from `getBoundingClientRect`.

- [ ] **Step 4: Run — expect PASS. Step 5: Commit**

```bash
git add frontend/web/src/components/player/unified/PlayerScrubBar.vue frontend/web/src/components/player/unified/PlayerScrubBar.spec.ts
git commit -m "feat(player): PlayerScrubBar (buffered/preview/chapters)"
```

---

## Task 11: Menus — `PlaybackSettingsMenu.vue` + `SubtitlesMenu.vue` + `BrowseSubsModal.vue`

**Files:**
- Create: `PlaybackSettingsMenu.vue`, `SubtitlesMenu.vue`, `BrowseSubsModal.vue` (+ `BrowseSubsModal.spec.ts`)

- [ ] **Step 1: Write the failing spec for BrowseSubsModal (search + filter)**

```typescript
import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import BrowseSubsModal from './BrowseSubsModal.vue'

const tracks = [
  { url: 't1', provider: 'Jimaku', lang: 'ja', label: 'Kawaisubs Re:Zero 12', format: 'ass' },
  { url: 't2', provider: 'OpenSubtitles', lang: 'en', label: 'Re:Zero 12 HorribleSubs', format: 'srt' },
]

describe('BrowseSubsModal', () => {
  it('groups tracks by language', () => {
    const w = mount(BrowseSubsModal, { props: { tracks, selectedUrl: null } })
    expect(w.findAll('[data-test="lang-group"]').length).toBe(2)
  })
  it('search narrows the visible tracks', async () => {
    const w = mount(BrowseSubsModal, { props: { tracks, selectedUrl: null } })
    await w.find('[data-test="search"]').setValue('horriblesubs')
    expect(w.findAll('[data-test="track"]').length).toBe(1)
  })
  it('emits select with the track', async () => {
    const w = mount(BrowseSubsModal, { props: { tracks, selectedUrl: null } })
    await w.find('[data-test="track"] [data-test="select"]').trigger('click')
    expect(w.emitted('select')).toBeTruthy()
  })
})
```

- [ ] **Step 2: Run — expect FAIL. Step 3: Implement all three.**
- `PlaybackSettingsMenu.vue` — props `{ quality, qualities, speed, speeds, autoNext, autoSkip }`; emits `update:*`. Rows for Quality/Speed (nested view) + two `Switch` toggles (reuse `@/components/ui/Switch.vue`). Style per `.pl-settings`.
- `SubtitlesMenu.vue` — props `{ subLang, subLangs, subSize, subBg, subOffset }`; emits `update:*` + `open-browse`. Includes the precise typeable offset stepper (`.pl-stepper`/`.pl-offset-input`) and a "Browse all subtitles" row. NO language/color blocks (removed per spec).
- `BrowseSubsModal.vue` — reuse `@/components/ui/Modal.vue`; search input + provider/language filter chips + grouped list. Data source wired by the parent from the existing `OtherSubsPanel` fetch path; the modal itself is presentational over a `tracks` prop.

- [ ] **Step 4: Run — expect PASS. Step 5: Commit**

```bash
cd frontend/web && bunx vitest run src/components/player/unified/BrowseSubsModal.spec.ts
git add frontend/web/src/components/player/unified/PlaybackSettingsMenu.vue frontend/web/src/components/player/unified/SubtitlesMenu.vue frontend/web/src/components/player/unified/BrowseSubsModal.vue frontend/web/src/components/player/unified/BrowseSubsModal.spec.ts
git commit -m "feat(player): playback/subtitles menus + browse-subs modal"
```

---

## Task 12: Overlays + control bar

**Files:**
- Create: `overlays/SkipIntroChip.vue`, `overlays/NextEpisodeCard.vue`, `overlays/BigPlayButton.vue`, `overlays/WatchTogetherButton.vue`, `PlayerControlBar.vue`

- [ ] **Step 1: Implement the small overlays** (style per `.pl-skip/.pl-next/.pl-bigplay`).
  - `SkipIntroChip.vue` — props `{ visible }`; emits `skip`. Parent only renders it when real skip timings exist (see Task 14 — for stage 1 timings are absent, so it stays hidden).
  - `NextEpisodeCard.vue` — props `{ nextEp, title, stillUrl, countdown }`; emits `play`, `cancel`.
  - `BigPlayButton.vue` — props `{ visible }`; emits `play`.
  - `WatchTogetherButton.vue` — **WIP stub**: a top-bar icon button, always `disabled`, with a "WIP" badge + `title="Watch Together — coming soon"`. No panel, no emits.

- [ ] **Step 2: Implement `PlayerControlBar.vue`** — lays out: play/pause, −10s, +10s, hover-volume, spacer, Source pill (`<provider> · <audio> ▾`), Subtitles button, Settings button, PiP, theater toggle, fullscreen. Props pass current state; emits one event per control (`toggle-play`, `seek-rel`, `set-volume`, `toggle-source`, `toggle-subs`, `toggle-settings`, `toggle-theater`, `toggle-fullscreen`, `toggle-pip`). Mobile: trim −10s/PiP/fullscreen via the `.pl-btns` media rule. Reuse `Icon` equivalents from the existing icon set (`grep -rn "name=\"play\"" src/components` to find the project's icon component).

- [ ] **Step 3: Commit**

```bash
git add frontend/web/src/components/player/unified/overlays frontend/web/src/components/player/unified/PlayerControlBar.vue
git commit -m "feat(player): overlays (skip/next/bigplay/WT-wip) + control bar"
```

---

## Task 13: `UnifiedPlayer.vue` orchestrator

Wires composables + components. Owns the `<video>`, the rAF progress loop, and the open-menu state.

**Files:**
- Create: `frontend/web/src/components/player/unified/UnifiedPlayer.vue`
- Create: `frontend/web/src/components/player/unified/unified-player.css`

- [ ] **Step 1: Implement** the SFC with this structure:
  - Props: `{ animeId: string; anime: { title: string; ep: number; eps: number; still?: string }; theater: boolean; initialEpisode?: number }`. Emits: `toggle-theater`, `open-episodes`.
  - Setup: `const state = usePlayerState()`, `const engine = useVideoEngine(videoRef)`, `const resolver = useProviderResolver()`, `const filter = computed(() => ({ audio: state.combo.value.audio, lang: state.combo.value.lang, content: isHentai ? 'hentai' : 'common' }))`, `const { rows, start } = useProviderHealth(filter)`.
  - Pick the default active provider = first `active` row; set it via `state.setProvider`.
  - `watch(() => state.combo.value, resolveAndLoad)` — call `resolver.listEpisodes` + `resolver.resolveStream` then `engine.load(stream)`. Guard against unavailable providers (catch `NotAvailableError` → show inline "source coming soon").
  - rAF loop updates `state.progress` from `videoRef.currentTime/duration` (NOT setInterval re-rendering the tree).
  - Layout: `<video ref="videoRef">` + `<SubtitleOverlay>` (reuse) + overlays + `<PlayerControlBar>` + conditional `<SourcePanel>` / `<PlaybackSettingsMenu>` / `<SubtitlesMenu>` / `<BrowseSubsModal>` + `<ResumePill>` (reuse) + WT-wip button. `chapters` prop to scrub bar = `[]` for stage 1 (no timings backend yet).
  - Style: import `unified-player.css` (recreate prototype `player.css` visuals with DS tokens). Aspect-ratio 16/9 inline; full-height in theater.

- [ ] **Step 2: Type-check**

Run: `cd frontend/web && bunx tsc --noEmit`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add frontend/web/src/components/player/unified/UnifiedPlayer.vue frontend/web/src/components/player/unified/unified-player.css
git commit -m "feat(player): UnifiedPlayer orchestrator"
```

---

## Task 14: i18n keys (en/ru/ja)

**Files:**
- Modify: `frontend/web/src/locales/en.json`, `ru.json`, `ja.json`

- [ ] **Step 1: Add a `player.unified` namespace to ALL THREE** with identical key sets. Minimum keys (add more as components reference them):

```jsonc
// en.json (mirror into ru.json + ja.json with translations)
"player": {
  "unified": {
    "tab": "AnimeEnigma",
    "beta": "Beta",
    "source": "Source & translation",
    "audio": "Audio", "language": "Language", "team": "Team",
    "provider": "Provider", "server": "Server",
    "providersAvailable": "{n} available",
    "wipHosting": "We are working on our own hosting",
    "temporarilyUnreachable": "Temporarily unreachable",
    "only18": "Only for 18+ titles",
    "playback": "Playback", "quality": "Quality", "speed": "Speed",
    "autoplayNext": "Autoplay next", "autoSkipIntro": "Auto-skip intro",
    "subtitles": "Subtitles", "subtitleSettings": "Subtitle settings",
    "textSize": "Text size", "background": "Background", "timingOffset": "Timing offset",
    "browseAll": "Browse all subtitles", "searchSubs": "Search by release or group…",
    "skipIntro": "Skip Intro", "resume": "Resume", "startOver": "Start over",
    "watchTogether": "Watch Together", "wip": "WIP"
  }
}
```

- [ ] **Step 2: Run the locale parity test**

Run: `cd frontend/web && bunx vitest run src/locales`
Expected: PASS (no key present in one file but missing in another).

- [ ] **Step 3: Commit**

```bash
git add frontend/web/src/locales/en.json frontend/web/src/locales/ru.json frontend/web/src/locales/ja.json
git commit -m "feat(player): i18n keys for unified player (en/ru/ja)"
```

---

## Task 15: Wire into `Anime.vue` (new pill + mount + flag)

**Files:**
- Modify: `frontend/web/src/views/Anime.vue`

- [ ] **Step 1: Add the Vite flag** near the other `*Enabled` consts (search `ourEnglishEnabled =`):

```typescript
const unifiedPlayerEnabled = import.meta.env.VITE_UNIFIED_PLAYER_ENABLED !== 'false'
```

- [ ] **Step 2: Extend the selection model.** Find `VALID_LANGUAGES`/`VALID_PROVIDERS` (~lines 1230-1245). Add a dedicated selection rather than overloading `videoLanguage`: introduce `const unifiedSelected = ref(false)`. When true, the unified player mounts and the old language/provider sub-tabs hide.

- [ ] **Step 3: Add the pill** as the LAST button in the language `ButtonGroup` (after the RAW pill, before `</ButtonGroup>`):

```vue
<button
  v-if="unifiedPlayerEnabled"
  @click="unifiedSelected = true"
  :aria-pressed="unifiedSelected"
  class="px-3 py-1.5 rounded-md text-sm font-medium transition-all inline-flex items-center gap-1.5"
  :class="unifiedSelected ? 'bg-white/15 text-white' : 'text-white/50 hover:text-white/70'"
>
  {{ $t('player.unified.tab') }}
  <span class="text-[10px] px-1 rounded bg-cyan-500/20 text-cyan-400">{{ $t('player.unified.beta') }}</span>
</button>
```

When any existing language pill is clicked, set `unifiedSelected = false` (add to `switchLanguage`).

- [ ] **Step 4: Mount the player AFTER the existing `v-if/v-else-if` player chain** (per the Vue template rule — never inside it). Find the end of the chain (after the `Anime18Player`/last `v-else-if`, ~line 647) and add a sibling block:

```vue
<UnifiedPlayer
  v-if="unifiedSelected && unifiedPlayerEnabled"
  :anime-id="anime.id"
  :anime="{ title: anime.russian || anime.name, ep: 1, eps: anime.episodes || 1, still: anime.image }"
  :theater="theaterMode"
  @toggle-theater="theaterMode = !theaterMode"
  @open-episodes="/* existing drawer open handler */"
/>
```

Adapt prop names to the real `anime` shape in this view (`grep -n "anime\." views/Anime.vue | head`). Add the import + register `UnifiedPlayer`. Gate the existing chain's outer wrapper with `v-show="!unifiedSelected"` (or `v-if`) so only one player renders.

- [ ] **Step 5: Type-check + build**

Run: `cd frontend/web && bunx tsc --noEmit && bun run build`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add frontend/web/src/views/Anime.vue
git commit -m "feat(player): mount unified player as a new selector pill (stage 1, flagged)"
```

---

## Task 16: Design-system lint + styling pass

**Files:**
- Modify: `frontend/web/src/components/player/unified/unified-player.css` and any SFC styles.
- Modify (if needed): `frontend/web/scripts/design-system-allowlist.txt`

- [ ] **Step 1: Run the DS lint**

Run: `bash frontend/web/scripts/design-system-lint.sh`
Expected: `ERRORS: 0`. If hex literals are needed for provider identity hues, add justified `path:hex:reason` lines to the allowlist (cyan/orange/pink/rose are brand-exempt for Tailwind classes; raw hex still needs allowlisting).

- [ ] **Step 2: Prove the fail-path still works**

Run: `bash frontend/web/scripts/design-system-lint.sh --selftest`
Expected: non-zero (gate is live).

- [ ] **Step 3: Commit**

```bash
git add frontend/web/src/components/player/unified frontend/web/scripts/design-system-allowlist.txt
git commit -m "style(player): design-system token compliance for unified player"
```

---

## Task 17: In-browser smoke (desktop + mobile)

jsdom can't catch Tailwind-v4 cascade bugs (DS-NF-06) — verify in a real browser.

- [ ] **Step 1: Run the full frontend test + type check**

Run: `cd frontend/web && bunx vitest run src/components/player/unified src/composables/unifiedPlayer src/locales && bunx tsc --noEmit`
Expected: PASS.

- [ ] **Step 2: Deploy & smoke**

Run: `make redeploy-web` then open a real released title's anime page.
Verify (desktop + a 390px mobile viewport):
- The new `AnimeEnigma (Beta)` pill appears in the `RU EN 18+ RAW JP` row; clicking it mounts the unified player and hides the old tabs.
- A healthy EN provider plays (custom control bar, scrub, play/pause).
- Source panel: audio/lang sliders filter the provider list; disabled providers (animepahe/animelib/hanime) render tinted with hover reasons; AnimeEnigma shows "We are working on our own hosting"; 18anime is tinted-irrelevant on a common title.
- Subtitles overlay renders; Browse-all modal opens, searches, selects.
- Gear toggles default OFF; quality/speed switch.
- WT icon is visible but disabled with a "WIP" tooltip.
- Theater toggle widens; no console errors.

- [ ] **Step 3: Final commit (if smoke fixes were needed)**

```bash
git add -A frontend/web/src/components/player/unified frontend/web/src/views/Anime.vue
git commit -m "fix(player): unified player smoke-test corrections"
```

---

## Task 18: After-update

- [ ] Run `/animeenigma-after-update` to lint/build/redeploy, append a Russian-Trump-mode changelog entry, and commit+push. (Per project policy this is the push path; batch the whole feature into one after-update.)

---

## Self-Review notes (coverage map)

- Spec §3.A composables → Tasks 4–7. §3.B components → Tasks 8–13. §3.C styling → Task 16. §3.D perf → Task 13 (rAF, lazy hls.js) + Task 7.
- Spec §4 provider model & health → Tasks 2,3,4 (registry + backend meta + chip-state matrix). Disabled animepahe/animelib/hanime, WIP AnimeEnigma, irrelevant 18anime all covered by the Task 4 test matrix.
- Spec §5 control surfaces → Tasks 8–13. Gear off-by-default → Task 5 test. Browse-all → Task 11. WT WIP → Task 12. Skip markers hidden (no timings) → Task 10 test + Task 13 (`chapters: []`).
- Spec §6 Anime.vue wiring → Task 15. §7 i18n → Task 14. §8 testing → every task + Task 17.
- Deferred items (§9) intentionally not implemented; the skip-markers TODO is encoded as `chapters: []` until a backend exists.
