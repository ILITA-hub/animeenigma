---
id: 21-04
phase: 21
plan: 04
type: execute
wave: 2
depends_on:
  - 21-02
files_modified:
  - frontend/web/src/components/player/EnglishPlayer.vue
  - frontend/web/src/components/player/__tests__/EnglishPlayer.spec.ts
  - frontend/web/public/changelog.json
autonomous: false
requirements:
  - SCRAPER-HEAL-08
user_setup: []
must_haves:
  truths:
    - "EnglishPlayer.vue renders three sequential loader phases driven by loadingServers / loadingStream / validatingStream refs"
    - "Phase 1 copy: EN 'Looking up sources…' / RU 'Поиск источников…'"
    - "Phase 2 copy: EN 'Connecting to remote stream…' / RU 'Подключение к удалённому потоку…'"
    - "Phase 3 copy: EN 'Verifying playback…' / RU 'Проверка воспроизведения…'"
    - "Phase 3 only renders when scraper response carries meta.gated:true"
    - "Locale detection follows the existing component pattern (hardcoded inline switch, not i18n keys per CONTEXT.md D6)"
    - "Vitest component test exercises all three phases + both locales + the meta.gated:false skip-phase-3 case"
  artifacts:
    - path: "frontend/web/src/components/player/EnglishPlayer.vue"
      provides: "validatingStream ref + three-phase loader overlay + RU/EN copy"
      contains: "validatingStream"
    - path: "frontend/web/src/components/player/__tests__/EnglishPlayer.spec.ts"
      provides: "Vitest spec covering loader phases × locale × meta.gated"
      contains: "Verifying playback"
  key_links:
    - from: "frontend/web/src/components/player/EnglishPlayer.vue (fetchStream block)"
      to: "scraper response meta.gated field"
      via: "set validatingStream.value = true when meta.gated === true before HLS init"
      pattern: "meta.gated"
    - from: "EnglishPlayer.vue loader overlay template"
      to: "validatingStream ref"
      via: "v-if + nested phase conditions"
      pattern: "validatingStream"
---

<objective>
Ship the three-phase loader in EnglishPlayer.vue. Phase 3 ("Verifying playback…") renders only when the scraper response carries `meta.gated:true` — masking the 1-2s gate latency that Plan 21-03 added to the cold path. EN + RU copy locked per spec; hardcoded inline switch on locale (CONTEXT.md D6 — no i18n key extraction). SCRAPER-HEAL-08.

Purpose: The user-facing payoff of Phase 21. Without this loader, the 1-2s gate adds dead-air silence that LOOKS like a stuck player. With this loader, the user sees a calm "Verifying playback…" message and understands the system is working.

Output: A working three-phase loader + Vitest component spec covering 6 cases (3 phases × 2 locales) + a 7th case asserting Phase 3 is skipped when meta.gated is absent.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/PROJECT.md
@.planning/ROADMAP.md
@.planning/STATE.md
@.planning/phases/21-playability-foundation/21-CONTEXT.md
@docs/plans/2026-05-13-scraper-self-healing-spec.md
@.planning/phases/21-playability-foundation/21-21-02-SUMMARY.md
@CLAUDE.md

<interfaces>
<!-- Plan 21-02 outputs (handler): -->

`GET /scraper/stream` JSON response:
```json
{
  "success": true,
  "data": {
    "stream": { "sources": [...], "tracks": [...], "headers": {...} },
    "meta": { "tried": ["gogoanime"], "gated": true }
  }
}
```
`meta.gated` is OMITTED when false (warm path / cache hit). The FE treats undefined as false.

<!-- Existing EnglishPlayer.vue refs (verified via grep): -->

- `const loadingEpisodes = ref(false)` (line 611)
- `const loadingServers = ref(false)` (line 612)
- `const loadingStream = ref(false)` (line 613)

Existing loader overlay (lines 27-35 of EnglishPlayer.vue):
```vue
<div
  v-if="loadingStream || (loadingServers && selectedEpisode)"
  class="absolute inset-0 z-10 flex items-center justify-center bg-black/80"
>
  <div class="text-center">
    <div class="w-10 h-10 border-2 accent-border border-t-transparent rounded-full animate-spin mx-auto mb-3" />
    <p class="text-white/60 text-sm">{{ $t('player.loadingEpisode', { n: selectedEpisode?.number }) }}</p>
  </div>
</div>
```

Existing fetchStream body sets streamUrl after HLS init. The new flow:
1. `loadingServers.value = true` (Phase 1) — already exists
2. `loadingStream.value = true` (Phase 2) — already exists
3. **NEW**: `validatingStream.value = true` (Phase 3) — set ONLY when `meta.gated === true`, BEFORE the `initPlayer(...)` call. Cleared after initPlayer completes OR errors.

<!-- Locale detection — per CONTEXT.md D6, do not introduce i18n keys; use the
component's existing inline locale switch. The component already has `t()` for
existing strings (vue-i18n based), so we have two options:
  A) Hardcode the three new strings inline (`locale.value === 'ru' ? '...' : '...'`)
  B) Add three new keys to ru/en/ja locale JSON

CONTEXT.md D6 explicitly says option A. Follow it. -->

<!-- The component already imports `t` from vue-i18n at line 498-area. We will
ALSO need access to `locale` from useI18n() — likely already destructured;
verify and import if not. -->
</interfaces>
</context>

<tasks>

<task type="auto" tdd="true">
  <name>Task 1: Add validatingStream ref + three-phase loader template + EN/RU inline copy</name>
  <files>frontend/web/src/components/player/EnglishPlayer.vue</files>
  <read_first>
    - frontend/web/src/components/player/EnglishPlayer.vue lines 1-100 (template — find the existing loader overlay around lines 27-35)
    - frontend/web/src/components/player/EnglishPlayer.vue lines 480-620 (script setup — find ref declarations + the useI18n() destructuring around `import { useI18n } from 'vue-i18n'`)
    - frontend/web/src/components/player/EnglishPlayer.vue lines 1080-1165 (fetchStream — find the response parsing where data.meta.gated will be read)
    - .planning/phases/21-playability-foundation/21-CONTEXT.md D6 (hardcoded inline locale switch)
    - .planning/milestones/v3.1-REQUIREMENTS.md SCRAPER-HEAL-08 (exact copy strings)
  </read_first>
  <behavior>
    - With `loadingServers=true, loadingStream=false, validatingStream=false, locale='en'` → overlay shows "Looking up sources…".
    - With `loadingServers=false, loadingStream=true, validatingStream=false, locale='en'` → overlay shows "Connecting to remote stream…".
    - With `loadingServers=false, loadingStream=false, validatingStream=true, locale='en'` → overlay shows "Verifying playback…".
    - Same three states with `locale='ru'` show: "Поиск источников…", "Подключение к удалённому потоку…", "Проверка воспроизведения…".
    - Phase precedence: when MULTIPLE refs are true (rare during transitions), the LATER phase wins (validatingStream > loadingStream > loadingServers). Implementation: nest the conditions.
    - The existing overlay shape (spinner + centered text + black/80 backdrop) preserved — only the text content changes.
    - When meta.gated is true in the response, validatingStream.value is set to true between the stream-URL receipt and the `await nextTick()` + initPlayer call, then cleared once initPlayer returns (or in the `finally`).
    - When meta.gated is false/absent, validatingStream stays false; Phase 3 NEVER renders.
  </behavior>
  <action>
    1. **Edit EnglishPlayer.vue script setup** — declare the new ref right after `loadingStream`:
       ```ts
       const loadingStream = ref(false)
       const validatingStream = ref(false) // SCRAPER-HEAL-08: Phase 3 (gate validation). True only when scraper response meta.gated === true.
       ```
       Verify `useI18n()` is destructured to include `locale` (e.g. `const { t, locale } = useI18n()`). If only `t` is destructured, add `locale`. The existing import line should already pull from `vue-i18n`.
    2. **Edit EnglishPlayer.vue template** — replace the existing loader overlay (the `<div v-if="loadingStream || (loadingServers && selectedEpisode)" ...>` block at lines 27-35) with the three-phase variant:
       ```vue
       <!-- Three-phase loader overlay (SCRAPER-HEAL-08).
            Phase precedence: validatingStream (3) > loadingStream (2) > loadingServers (1).
            Phase 3 renders only when scraper response carried meta.gated === true.
            Locale switch is hardcoded inline per CONTEXT.md D6 (i18n keys are not
            introduced for three strings; if full i18n is adopted later, this is
            a 6-line migration). -->
       <div
         v-if="validatingStream || loadingStream || (loadingServers && selectedEpisode)"
         class="absolute inset-0 z-10 flex items-center justify-center bg-black/80"
       >
         <div class="text-center">
           <div class="w-10 h-10 border-2 accent-border border-t-transparent rounded-full animate-spin mx-auto mb-3" />
           <p class="text-white/60 text-sm">
             <template v-if="validatingStream">
               {{ locale === 'ru' ? 'Проверка воспроизведения…' : 'Verifying playback…' }}
             </template>
             <template v-else-if="loadingStream">
               {{ locale === 'ru' ? 'Подключение к удалённому потоку…' : 'Connecting to remote stream…' }}
             </template>
             <template v-else>
               {{ locale === 'ru' ? 'Поиск источников…' : 'Looking up sources…' }}
             </template>
           </p>
         </div>
       </div>
       ```
       Note: this REPLACES the existing `$t('player.loadingEpisode', { n: ... })` line. The old "Episode N" interpolation is removed — the spec replaces it with the three-phase copy. If the UX team wants the episode number back in Phase 3, that's a follow-up.
       Other loader spinner blocks in the file (e.g. line 5 "loading episodes" — a separate spinner before any episode is picked) are NOT touched.
    3. **Edit EnglishPlayer.vue fetchStream function** — around line 1099 (after `updateTriedChain(response.data)`), read the gated bool from the envelope and toggle the ref. Find the JSON envelope parsing block (line 1099-1110):
       ```ts
       updateTriedChain(response.data)
       // SCRAPER-HEAL-08: surface the playability-gate phase if the scraper
       // ran the gate on this call. Type the envelope's meta field defensively
       // — meta.gated is optional + omitted when false (cache-hit path).
       const env = response.data as {
         data?: {
           stream?: ScraperStream
           meta?: { tried?: string[]; gated?: boolean }
         }
       } | undefined
       const data: ScraperStream | undefined = env?.data?.stream
       const gated = env?.data?.meta?.gated === true
       if (gated) {
         validatingStream.value = true
       }
       if (!data || !Array.isArray(data.sources) || data.sources.length === 0) {
         throw new Error('scraper stream response missing data.stream.sources')
       }
       ```
       Add `validatingStream.value = false` to BOTH the success path (after `initPlayer(...)` completes) AND the `finally` block. The simplest correct location is the existing `finally` block where `loadingStream.value = false`:
       ```ts
       } finally {
         loadingStream.value = false
         validatingStream.value = false // SCRAPER-HEAL-08
       }
       ```
       Verify the previously-typed `env` destructuring (line 1101) is not duplicated — replace the existing `as { data?: { stream?: ScraperStream } }` cast with the wider type above. If a similar cast exists elsewhere for the same response, keep ONE consistent shape.
    4. **Visual self-check (Vitest in Task 2 also covers this)**:
       - Run `cd frontend/web && bunx tsc --noEmit` to catch TypeScript errors.
       - Run `cd frontend/web && bunx eslint src/components/player/EnglishPlayer.vue` and fix any new lint warnings.
  </action>
  <verify>
    <automated>cd /data/animeenigma/frontend/web && bunx tsc --noEmit && bunx eslint src/components/player/EnglishPlayer.vue</automated>
  </verify>
  <done>
    - `frontend/web/src/components/player/EnglishPlayer.vue` declares `const validatingStream = ref(false)`.
    - Template overlay contains all three EN strings literally: "Looking up sources…", "Connecting to remote stream…", "Verifying playback…".
    - Template overlay contains all three RU strings literally: "Поиск источников…", "Подключение к удалённому потоку…", "Проверка воспроизведения…".
    - fetchStream sets validatingStream=true when `env?.data?.meta?.gated === true`.
    - validatingStream is cleared in the `finally` block.
    - `grep -c "validatingStream" frontend/web/src/components/player/EnglishPlayer.vue` returns ≥ 4 (declaration + 3 template uses + set/clear in fetchStream).
    - `grep -c "Verifying playback" frontend/web/src/components/player/EnglishPlayer.vue` returns 1+.
    - `grep -c "Проверка воспроизведения" frontend/web/src/components/player/EnglishPlayer.vue` returns 1+.
    - `bunx tsc --noEmit` passes for the file.
  </done>
</task>

<task type="auto" tdd="true">
  <name>Task 2: Vitest component spec covering 3 phases × 2 locales + meta.gated:false skip case</name>
  <files>frontend/web/src/components/player/__tests__/EnglishPlayer.spec.ts</files>
  <read_first>
    - frontend/web/src/components/player/EnglishPlayer.vue (Task 1 of THIS plan — for template structure + refs)
    - frontend/web/vite.config.ts or vitest.config.ts (to confirm test runner + jsdom setup; create the __tests__ directory if absent)
    - frontend/web/package.json (verify @vue/test-utils + vitest are dependencies; if not, abort and document — this plan needs them already in place)
    - frontend/web/src/api/client.ts (scraperApi exports — needed to mock in the spec)
  </read_first>
  <behavior>
    - Test 1 (en-Phase1): mount EnglishPlayer with loadingServers=true + selectedEpisode set + locale='en' → component renders text "Looking up sources…".
    - Test 2 (en-Phase2): loadingStream=true + locale='en' → text "Connecting to remote stream…".
    - Test 3 (en-Phase3): validatingStream=true + locale='en' → text "Verifying playback…".
    - Test 4-6: same three phases with locale='ru' → respective RU strings.
    - Test 7 (gated-false skip): drive fetchStream with a mocked scraperApi.getStream that returns `data:{stream:{sources:[...]}, meta:{tried:["gogoanime"]}}` (NO gated key) → assert that during the fetchStream call's lifetime, validatingStream is NEVER toggled to true. (Spy on the ref via `vi.spyOn` or track via a watcher.)
    - Test 8 (gated-true sets): same mock but with `meta:{tried:[...], gated:true}` → assert validatingStream is set to true before initPlayer is called (mocked) and cleared after.
    - Test 9 (phase precedence): when ALL three refs are true (synthetic) → the rendered text is the Phase 3 string ("Verifying playback…").
  </behavior>
  <action>
    1. **Verify test infra**: `cd /data/animeenigma/frontend/web && cat package.json | grep -E '"vitest"|"@vue/test-utils"'`. If either is missing, install: `bun add -D vitest @vue/test-utils @vitejs/plugin-vue jsdom`. (vite already has the Vue plugin from the dev config; verify before installing.)
    2. **Ensure vitest config**: if `vitest.config.ts` does not exist, create:
       ```ts
       import { defineConfig } from 'vitest/config'
       import vue from '@vitejs/plugin-vue'
       import { resolve } from 'node:path'

       export default defineConfig({
         plugins: [vue()],
         test: {
           environment: 'jsdom',
           globals: true,
         },
         resolve: {
           alias: { '@': resolve(__dirname, 'src') },
         },
       })
       ```
       If `vite.config.ts` already has a `test:` block with jsdom, reuse it instead of creating a separate vitest.config.
    3. **Create frontend/web/src/components/player/__tests__/EnglishPlayer.spec.ts**:
       ```ts
       import { describe, it, expect, vi, beforeEach } from 'vitest'
       import { mount, flushPromises } from '@vue/test-utils'
       import { createI18n } from 'vue-i18n'
       import { nextTick } from 'vue'
       import EnglishPlayer from '../EnglishPlayer.vue'

       // Minimal i18n shim — the component uses t() for non-loader strings;
       // we don't care about those values, only that the locale matches and
       // t() doesn't throw.
       function makeI18n(locale: 'en' | 'ru') {
         return createI18n({
           legacy: false,
           locale,
           fallbackLocale: 'en',
           messages: {
             en: { player: {} },
             ru: { player: {} },
           },
           // missing-key handler returns the key so t('player.foo') == 'player.foo'
           missing: (_l, key) => key,
         })
       }

       // Mock scraperApi to control fetchStream's response in Tests 7+8.
       vi.mock('@/api/client', () => ({
         scraperApi: {
           getEpisodes: vi.fn().mockResolvedValue({ data: { data: { episodes: [], meta: { tried: [] } } } }),
           getServers:  vi.fn().mockResolvedValue({ data: { data: { servers: [],  meta: { tried: [] } } } }),
           getStream:   vi.fn(),
           getHealth:   vi.fn().mockResolvedValue({ data: { providers: {} } }),
         },
         jimakuApi: { lookupEntries: vi.fn().mockResolvedValue({ data: [] }) },
         userApi: {},
       }))

       const baseProps = {
         animeId: 'test-anime-id',
         malId: '52991',
         shikimoriId: '52991',
         title: 'Frieren',
         posterUrl: '',
       } as Record<string, unknown>

       async function mountAt(locale: 'en' | 'ru') {
         const i18n = makeI18n(locale)
         const wrapper = mount(EnglishPlayer, {
           global: { plugins: [i18n] },
           props: baseProps as never,
         })
         await flushPromises()
         await nextTick()
         return wrapper
       }

       describe('EnglishPlayer three-phase loader', () => {
         beforeEach(() => { vi.clearAllMocks() })

         it.each([
           ['en', 'Looking up sources…',         { loadingServers: true,  selectedEpisode: { number: 1, id: 'ep1' } }],
           ['en', 'Connecting to remote stream…', { loadingStream:  true }],
           ['en', 'Verifying playback…',          { validatingStream: true }],
           ['ru', 'Поиск источников…',           { loadingServers: true,  selectedEpisode: { number: 1, id: 'ep1' } }],
           ['ru', 'Подключение к удалённому потоку…', { loadingStream: true }],
           ['ru', 'Проверка воспроизведения…',    { validatingStream: true }],
         ])('locale=%s renders %s', async (locale, expected, refState) => {
           const wrapper = await mountAt(locale as 'en' | 'ru')
           // The component exposes its loader refs via setup; we set them via vm-ref access.
           const vm = wrapper.vm as unknown as Record<string, { value: unknown }>
           Object.entries(refState).forEach(([k, v]) => { (vm[k] as { value: unknown }).value = v })
           await nextTick()
           expect(wrapper.text()).toContain(expected)
         })

         it('precedence: validatingStream wins over loadingStream + loadingServers', async () => {
           const wrapper = await mountAt('en')
           const vm = wrapper.vm as unknown as Record<string, { value: unknown }>
           ;(vm.loadingServers as { value: boolean }).value = true
           ;(vm.loadingStream as { value: boolean }).value = true
           ;(vm.validatingStream as { value: boolean }).value = true
           ;(vm.selectedEpisode as { value: unknown }).value = { number: 1, id: 'ep1' }
           await nextTick()
           expect(wrapper.text()).toContain('Verifying playback…')
           expect(wrapper.text()).not.toContain('Connecting to remote stream…')
           expect(wrapper.text()).not.toContain('Looking up sources…')
         })

         it('meta.gated=true sets validatingStream during fetchStream', async () => {
           const { scraperApi } = await import('@/api/client')
           // Stub a stream response with meta.gated=true
           ;(scraperApi.getStream as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
             data: { data: {
               stream: { sources: [{ url: 'https://example.com/x.m3u8', type: 'hls' }], tracks: [], headers: {} },
               meta: { tried: ['gogoanime'], gated: true },
             }},
           })
           const wrapper = await mountAt('en')
           const vm = wrapper.vm as unknown as Record<string, { value: unknown }>
           ;(vm.selectedEpisode as { value: unknown }).value = { number: 1, id: 'ep1' }
           ;(vm.selectedServer as { value: unknown }).value = { id: 'https://otakuhg.site/foo', name: 'StreamHG', type: 'sub' }
           // Drive fetchStream by calling the exposed method (or by toggling
           // selectedServer if a watcher triggers it). The component's
           // setup() return shape exposes fetchStream — assert via reflection.
           const fn = (vm as unknown as { fetchStream?: () => Promise<void> }).fetchStream
           if (typeof fn === 'function') {
             const p = fn()
             // immediately after kick-off, validatingStream should toggle once the
             // response resolves; await flushPromises waits for both microtasks.
             await flushPromises()
             expect((vm.validatingStream as { value: boolean }).value).toBe(false) // cleared in finally
             await p
           }
         })

         it('meta.gated absent does NOT toggle validatingStream', async () => {
           const { scraperApi } = await import('@/api/client')
           ;(scraperApi.getStream as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
             data: { data: {
               stream: { sources: [{ url: 'https://example.com/x.m3u8', type: 'hls' }], tracks: [], headers: {} },
               meta: { tried: ['gogoanime'] }, // no gated field
             }},
           })
           const wrapper = await mountAt('en')
           const vm = wrapper.vm as unknown as Record<string, { value: unknown }>
           const seenTrue: boolean[] = []
           // Watch the ref directly via the Vue reactivity system.
           const w = wrapper.vm
           const stop = (await import('vue')).watch(
             () => (vm.validatingStream as { value: boolean }).value,
             (v) => { if (v) seenTrue.push(true) },
           )
           ;(vm.selectedEpisode as { value: unknown }).value = { number: 1, id: 'ep1' }
           ;(vm.selectedServer as { value: unknown }).value = { id: 'https://otakuhg.site/foo', name: 'StreamHG', type: 'sub' }
           const fn = (vm as unknown as { fetchStream?: () => Promise<void> }).fetchStream
           if (typeof fn === 'function') {
             await fn()
           }
           stop()
           expect(seenTrue).toHaveLength(0)
         })
       })
       ```
       NOTE: The component's setup() return shape may not expose `fetchStream` directly. If the spec can't drive fetchStream via the exposed surface, two acceptable simplifications:
       (a) Drop Tests 8/9 from automated coverage; manually verify in Task 3 (the human checkpoint).
       (b) Refactor a tiny portion of EnglishPlayer.vue to `defineExpose({ fetchStream, loadingServers, loadingStream, validatingStream, selectedEpisode, selectedServer })` for test access. **Recommended:** do (b) — add a `defineExpose` block at the bottom of `<script setup>` exposing the refs + fetchStream by name. This is a 4-line change and unlocks per-ref test access cleanly.
    4. **Run the spec**: `cd frontend/web && bunx vitest run src/components/player/__tests__/EnglishPlayer.spec.ts`.
  </action>
  <verify>
    <automated>cd /data/animeenigma/frontend/web && bunx vitest run src/components/player/__tests__/EnglishPlayer.spec.ts --reporter=basic</automated>
  </verify>
  <done>
    - `frontend/web/src/components/player/__tests__/EnglishPlayer.spec.ts` exists with 6 phase×locale cases + precedence test + 2 meta.gated cases.
    - All tests pass.
    - `defineExpose` (if added) only exposes refs needed by tests + fetchStream; does NOT widen the public component API beyond what tests need.
    - `grep -c "Verifying playback\|Проверка воспроизведения\|Connecting to remote stream\|Подключение к удалённому потоку\|Looking up sources\|Поиск источников" frontend/web/src/components/player/__tests__/EnglishPlayer.spec.ts` returns ≥ 6.
  </done>
</task>

<task type="checkpoint:human-verify" gate="blocking">
  <name>Task 3: Manual smoke — three-phase loader visible on real production cold path</name>
  <what-built>
    EnglishPlayer.vue now renders three sequential loader phases. Phase 3 ("Verifying playback…") appears only when the scraper response carries meta.gated:true (cold-path resolution from Plan 21-03).
  </what-built>
  <how-to-verify>
    1. After Plan 21-03 Task 7 has signed off (production scraper is deployed with the gate), rebuild the frontend: `cd /data/animeenigma/frontend/web && bun run build`. Then redeploy the web container via `make redeploy-web` OR `make restart-web` if the build artefact is volume-mounted.
    2. Open https://animeenigma.ru in an EN-locale browser tab (cleared cache; use Incognito to guarantee no warm scraper cache from prior testing).
    3. Search for an anime that has NOT been played in the last 5 minutes (cold path required — Frieren or One Piece are good anchors).
    4. Click episode 1.
    5. **Observe the loader phases in order**:
       a. "Looking up sources…" appears briefly during ListServers fetch.
       b. "Connecting to remote stream…" appears during the GetStream API call.
       c. "Verifying playback…" appears for ~1-2s while the gate runs.
       d. Player starts playing real video.
    6. Switch to RU locale (language toggle in the header) and repeat steps 3-5. Confirm the RU strings render: "Поиск источников…", "Подключение к удалённому потоку…", "Проверка воспроизведения…".
    7. Re-click the same episode within 5 minutes — confirm Phase 3 does NOT appear (warm cache: meta.gated absent → no Phase 3).
    8. Open browser devtools → Network tab → filter for `/scraper/stream`. Inspect the JSON response body. Confirm `data.meta.gated: true` is PRESENT on cold call, ABSENT on warm call (within 5 min TTL).
    9. Optional regression check: switch to a Russian-locale browser tab WITHOUT any new gating happening (e.g. an already-cached anime). Phase 3 should not flash even briefly.
  </how-to-verify>
  <resume-signal>
    Reply "approved" if all three phases render in order on cold path, both locales render correct strings, and Phase 3 is skipped on warm path. Reply with screenshots/details if any assertion fails.
  </resume-signal>
</task>

<task type="auto">
  <name>Task 4: Update changelog.json + invoke /animeenigma-after-update</name>
  <files>frontend/web/public/changelog.json</files>
  <read_first>
    - frontend/web/public/changelog.json (find the latest entry; copy the structure/format)
    - CLAUDE.md "After-Update Skill (MUST USE)" section
    - .claude/skills/animeenigma-after-update/SKILL.md if it exists at the listed path; otherwise treat the skill as a slash command `/animeenigma-after-update` invoked at the end
  </read_first>
  <behavior>
    - changelog.json has a new entry dated 2026-05-13 (or today's date if different) announcing the Phase 21 user-facing change.
    - Entry uses the existing structure (likely { version, date, items: ["..."] } — match exactly by reading the latest entry).
    - Tone: informative + enthusiastic with emojis per CLAUDE.md after-update guidance.
    - /animeenigma-after-update completes: lint passes, scraper builds, redeploy succeeds, health passes, changelog updated, commits + push.
  </behavior>
  <action>
    1. **Read frontend/web/public/changelog.json** — find the latest entry's structure.
    2. **Add a new entry at the top** with content like:
       ```json
       {
         "version": "v3.1-phase-21",
         "date": "2026-05-13",
         "items": [
           "🎬 English playback is back: the player now routes around ad-poisoned servers automatically",
           "✅ New 'Verifying playback…' loader phase shows when the system is checking that a stream really plays",
           "⚡ Once a working server is found, future plays in the next 5 minutes skip the check for instant load",
           "🛡️ Self-healing groundwork: if a server goes bad, the system notices on the very next play, not days later"
         ]
       }
       ```
       Adjust shape to match the actual schema (read the file first).
    3. **Invoke /animeenigma-after-update** — this is a slash command from the project skill set; the executor runs it as the last step. The skill handles:
       - `bun run build` in frontend/web
       - `make redeploy-scraper` (since the scraper has new code from Plan 21-03)
       - `make redeploy-web` (since EnglishPlayer.vue changed)
       - `make health` post-deploy
       - Stages + commits all changes with co-authors (per memory: Claude Opus 4.6 + 0neymik0 + NANDIorg)
       - Pushes to remote
    4. If the skill is unavailable, do the equivalent steps manually:
       - `cd /data/animeenigma/frontend/web && bun run build`
       - `cd /data/animeenigma && make redeploy-scraper`
       - `cd /data/animeenigma && make redeploy-web`
       - `cd /data/animeenigma && make health`
       - `git add -p` and commit with `git commit -m "feat(scraper-21): ship Phase 21 — playability gate + three-phase loader\n\nCo-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>\nCo-Authored-By: 0neymik0 <0neymik0@gmail.com>\nCo-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"`
       - `git push origin main`
  </action>
  <verify>
    <automated>cd /data/animeenigma && make health</automated>
  </verify>
  <done>
    - changelog.json has a new entry for Phase 21 with emojis.
    - All services pass `make health` post-deploy.
    - Commit + push complete (or skill invocation succeeded).
    - `grep -c "v3.1-phase-21\\|2026-05-13" frontend/web/public/changelog.json` returns 1+.
  </done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| scraper response.meta.gated → FE ref | trusted (our backend), but defensive typing required because a malformed response should not throw |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-21-14 | I (Info disclosure) | hardcoded RU/EN strings in component | accept | strings are user-facing UI copy; no security-sensitive content. |
| T-21-15 | T (Tampering) | locale detection via useI18n().locale | accept | locale is browser-detected + user-toggled via existing locale switcher; no security context (only changes the visible string). |
| T-21-16 | D (DoS) | validatingStream stuck-true bug = permanent overlay | mitigate | `finally` block in fetchStream clears validatingStream regardless of error path. Vitest spec asserts state after the promise resolves. |
| T-21-17 | R (Repudiation) | regression goes unnoticed if loader copy diverges from spec | mitigate | Task 2 Vitest spec asserts each EN + RU string literal — any future code change that mistypes a string fails CI. |
</threat_model>

<verification>
- `cd /data/animeenigma/frontend/web && bunx tsc --noEmit` exits 0.
- `cd /data/animeenigma/frontend/web && bunx eslint src/components/player/EnglishPlayer.vue` exits 0.
- `cd /data/animeenigma/frontend/web && bunx vitest run src/components/player/__tests__/EnglishPlayer.spec.ts` passes all tests (≥ 8 cases).
- Task 3 manual smoke confirms all three phases visible in both locales + Phase 3 skipped on warm path.
- `grep -c "validatingStream" frontend/web/src/components/player/EnglishPlayer.vue` returns ≥ 4.
- `make health` post-deploy returns 9/9 services up.
</verification>

<success_criteria>
- SCRAPER-HEAL-08: EnglishPlayer.vue renders three sequential loader phases driven by `loadingServers` / `loadingStream` / `validatingStream` refs. EN + RU copy locked per spec. Phase 3 visible only when scraper response carries `meta.gated: true`.
- Vitest component spec covers each phase × each locale + meta.gated:false skip case.
- Production smoke confirms real cold-path UX is calm and informative.
- Phase 21 ships end-to-end via /animeenigma-after-update.
</success_criteria>

<output>
After completion, create `.planning/phases/21-playability-foundation/21-21-04-SUMMARY.md` documenting:
- The validatingStream ref + fetchStream gating logic
- The locale switch pattern (inline vs i18n keys per D6)
- Vitest spec coverage matrix (3 phases × 2 locales = 6 cases + precedence + 2 meta.gated cases = 9 cases)
- Production smoke result (whether the three-phase loader was visible on real cold path)
- Phase 21 shipped — note that this is the LAST plan in Phase 21 and the after-update skill ran here
</output>
