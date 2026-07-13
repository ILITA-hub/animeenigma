# Raw-Library File Manager UX Overhaul (①) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Turn the single-page `/admin/raw-library` view into a two-tab surface (Torrent Client / File Manager) with a deep-linkable file browser that shows anime titles for numeric folders and a `../` parent affordance, and delete the "Add torrent by hand" form.

**Architecture:** Split the 860-line `RawLibrary.vue` monolith into a thin tab host + two focused panel components (`TorrentClient.vue`, `FileManager.vue`) reusing the existing DS `Tabs.vue`. The active tab and the File Manager's browsed folder are driven entirely by the route (`/admin/raw-library/file-manager/:backend/:filepath(.*)*`), so reload/share restores state. `FileManager.vue` takes `backend`+`prefix` props and emits `navigate`; the host owns the router. Anime titles for `aeProvider/<shikimori-id>/` folders are resolved client-side via the existing `GET /api/anime/shikimori/{id}` with a module-level cache. Frontend-only — no backend change.

**Tech Stack:** Vue 3 `<script setup>` + TypeScript, Vue Router 4.6, vue-i18n (en/ru/ja), Vitest + `@vue/test-utils`, Tailwind v4 + Neon-Tokyo DS, `bun`.

## Global Constraints

- **Package manager:** `bun` only (never npm/pnpm); `bunx` for CLI (never npx). Worktrees don't share `node_modules` → run `bun install` in the worktree before building.
- **Work location:** the `feat/raw-library-file-manager-ux` worktree at `/data/ae-fm-ux`. NEVER edit the base tree `/data/animeenigma`. All file paths below are **worktree-relative** — always prefix with `/data/ae-fm-ux/`.
- **Design System:** bind semantic tokens; never hardcode colors. The `cyan`/`indigo` provider hues already in this view are DS-EXEMPT brand hues — keep them. A `PostToolUse` DS-lint hook runs on every `.vue`/`.ts` edit; `ERRORS>0` fails the build.
- **i18n parity REQUIRED:** every key added/removed must be mirrored across `en.json`, `ru.json`, `ja.json`. Locale files: `frontend/web/src/locales/{en,ru,ja}.json`.
- **Type-check must stay green:** `bunx vue-tsc --noEmit` (project trap: `vue-tsc --noEmit` can false-pass; still run it). No `any` unless the surrounding code already uses the pattern.
- **Commits:** each task ends with a commit **in the worktree** (do NOT push per-task; the final task lands the branch via `bin/ae-land.sh`, which appends the three required co-authors). Commit messages: conventional-commit `feat(...)`/`refactor(...)`/`test(...)`.
- **Backend is untouched:** no edits under `services/`. The only non-`.vue`/non-locale change is one method added to `frontend/web/src/api/client.ts`.
- **Route param note:** Vue Router 4.6 catch-all uses `(.*)*` (see existing `router/index.ts:330` `/:pathMatch(.*)*`). A `(.*)*` param arrives as `string[]` (segments, no slashes); reconstruct the prefix by `segs.join('/') + '/'` (empty → `''`).

---

## Task 1: Shared helpers module (`rawlibrary/lib.ts`)

Extract the two helpers both panels need (`formatBytes`, `unwrap`) into one module so the split panels don't duplicate them. `formatGB`/`formatPct`/`truncateMagnet` stay with TorrentClient (Task 2); `freshness*`/`keyFor` stay with FileManager (Task 3).

**Files:**
- Create: `frontend/web/src/views/admin/rawlibrary/lib.ts`
- Test: `frontend/web/src/views/admin/rawlibrary/__tests__/lib.spec.ts`

**Interfaces:**
- Produces:
  - `formatBytes(n: number): string` — humanizes a byte count (`0` → `"0 B"`, `1024` → `"1.0 KB"`, `≥100` in a unit drops decimals).
  - `unwrap<T>(resp: { data?: { data?: T } | T }): T | undefined` — unwraps the `{success,data}` httputil envelope.

- [ ] **Step 1: Write the failing test**

Create `frontend/web/src/views/admin/rawlibrary/__tests__/lib.spec.ts`:

```ts
import { describe, it, expect } from 'vitest'
import { formatBytes, unwrap } from '@/views/admin/rawlibrary/lib'

describe('formatBytes', () => {
  it('renders 0 and small/large units', () => {
    expect(formatBytes(0)).toBe('0 B')
    expect(formatBytes(-5)).toBe('0 B')
    expect(formatBytes(1024)).toBe('1.0 KB')
    expect(formatBytes(1024 * 1024)).toBe('1.0 MB')
    expect(formatBytes(150 * 1024 * 1024)).toBe('150 MB') // >=100 drops decimals
  })
})

describe('unwrap', () => {
  it('peels the {data:{data}} envelope, else returns the body', () => {
    expect(unwrap({ data: { data: { a: 1 } } })).toEqual({ a: 1 })
    expect(unwrap({ data: { a: 1 } })).toEqual({ a: 1 })
    expect(unwrap({ data: undefined })).toBeUndefined()
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /data/ae-fm-ux/frontend/web && bunx vitest run src/views/admin/rawlibrary/__tests__/lib.spec.ts`
Expected: FAIL — cannot resolve `@/views/admin/rawlibrary/lib`.

- [ ] **Step 3: Write the module**

Create `frontend/web/src/views/admin/rawlibrary/lib.ts` (bodies copied verbatim from the current `RawLibrary.vue:511-521` and `:567-573`):

```ts
// Shared helpers for the raw-library admin views (TorrentClient + FileManager).
// formatBytes humanizes a byte count; unwrap peels the {success,data} httputil
// envelope every /api/library/* + /api/anime/* response is wrapped in.

export function formatBytes(n: number): string {
  if (!n || n <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let i = 0
  let val = n
  while (val >= 1024 && i < units.length - 1) {
    val /= 1024
    i++
  }
  return val.toFixed(val >= 100 ? 0 : 1) + ' ' + units[i]
}

export function unwrap<T>(resp: { data?: { data?: T } | T }): T | undefined {
  const body = resp.data as { data?: T } | T | undefined
  if (body && typeof body === 'object' && 'data' in (body as Record<string, unknown>)) {
    return (body as { data: T }).data
  }
  return body as T | undefined
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /data/ae-fm-ux/frontend/web && bunx vitest run src/views/admin/rawlibrary/__tests__/lib.spec.ts`
Expected: PASS (5 assertions).

- [ ] **Step 5: Commit**

```bash
cd /data/ae-fm-ux
git add frontend/web/src/views/admin/rawlibrary/lib.ts frontend/web/src/views/admin/rawlibrary/__tests__/lib.spec.ts
git commit -m "refactor(raw-library): extract shared formatBytes/unwrap helpers"
```

---

## Task 2: Extract `TorrentClient.vue` (stats + search + jobs)

Move the stats strip, search panel, and jobs panel (today's `RawLibrary.vue` sections 1–3) into a self-contained component. Behavior-preserving. This panel keeps its own `errorBanner` and its own polling lifecycle.

**Files:**
- Create: `frontend/web/src/views/admin/rawlibrary/TorrentClient.vue`
- Reference (do not delete yet): `frontend/web/src/views/admin/RawLibrary.vue` (source of the moved code)
- Test: `frontend/web/src/views/admin/rawlibrary/__tests__/TorrentClient.spec.ts`

**Interfaces:**
- Consumes: `formatBytes`, `unwrap` from `@/views/admin/rawlibrary/lib` (Task 1).
- Produces: `<TorrentClient />` — takes no props, emits nothing. Self-contained (owns its fetch + polling).

- [ ] **Step 1: Create the component by moving sections 1–3 verbatim**

Create `frontend/web/src/views/admin/rawlibrary/TorrentClient.vue`. Assemble it from the current `RawLibrary.vue`:

- **`<template>`**: a single root `<div>` containing, in order, the **Error banner** (`RawLibrary.vue:9-12`), **Stats strip** (`:14-42`), **Search panel** (`:44-132`), **Jobs panel** (`:134-281`). Do NOT include the page `min-h-screen`/`container` wrapper or the `<h1>` header (those move to the host in Task 4) — start the template at the error banner.
- **`<script setup lang="ts">`**: copy these from `RawLibrary.vue` verbatim, EXCEPT replace the local `formatBytes`/`unwrap` definitions with an import:
  - imports (`:395-408`) — but change the `formatBytes`/`unwrap` source: add `import { formatBytes, unwrap } from '@/views/admin/rawlibrary/lib'` and DROP the local `formatBytes` (`:511-521`) and `unwrap` (`:567-573`) function bodies. Keep the `library` types import but trim it to only the types used here: `Job, JobStatus, Release, LibraryHealth, CreateJobPayload, StorageBackend`.
  - `useI18n`/`useConfirm` (`:410-411`)
  - `AnimeSearchResult` interface (`:418-425`)
  - refs: `health`, `errorBanner` (`:428-429`); search refs (`:431-434`); `selectedStorage` (`:438`); jobs refs (`:440-445`); interval handles (`:459-461`); `pendingLinkDebounces` (`:464`)
  - computed: `totalActiveJobs` (`:467-470`); `storageOptions` (`:472-475`)
  - helpers: `storageLabel`/`storageBadgeClass` (`:490-501`); `providerBadgeVariant` (`:505-509`); `formatGB` (`:523-525`); `formatPct` (`:527-530`); `truncateMagnet` (`:532-535`); `statusVariant` (`:537-556`)
  - fetchers: `fetchHealth`, `fetchActiveJobs`, `fetchFailedJobs`, `fetchPendingLinkJobs` (`:576-615`)
  - `handleSearch` (`:618-639`); `queueJob`, `cancelJob`, `retryJob`, `onPendingLinkInput`, `linkJob` (`:642-738`)
  - lifecycle `onMounted`/`onUnmounted` (`:838-860`) but DROP the `void loadFiles('')` call from `onMounted` (files are the FileManager's job now).

Do NOT copy: `fileDomain`/`filePrefix`/`fileEntries`/`fileBreadcrumb`/`filesLoading`/`magnet*` refs, `fileDomainOptions`, `freshnessVariant`/`freshnessLabel`, `loadFiles`/`openEntry`/`crumbTo`/`changeDomain`/`keyFor`/`downloadEntry`/`deleteEntry`/`addMagnet`. Those go to FileManager (Task 3) or are deleted.

- [ ] **Step 2: Write the smoke test**

Create `frontend/web/src/views/admin/rawlibrary/__tests__/TorrentClient.spec.ts`:

```ts
import { describe, it, expect, vi, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import TorrentClient from '@/views/admin/rawlibrary/TorrentClient.vue'

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return { ...actual, useI18n: () => ({ t: (k: string) => k }) }
})
vi.mock('@/composables/useConfirm', () => ({ useConfirm: () => ({ confirm: vi.fn() }) }))
vi.mock('@/api/client', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api/client')>()
  return {
    ...actual,
    adminLibraryApi: {
      ...actual.adminLibraryApi,
      healthExtended: vi.fn().mockResolvedValue({ data: { success: true, data: {
        disk_free_bytes: 500, disk_total_bytes: 1000, active_torrents: 2, active_jobs_by_status: { downloading: 1 },
      } } }),
      listJobs: vi.fn().mockResolvedValue({ data: { success: true, data: { jobs: [] } } }),
    },
  }
})

afterEach(() => vi.clearAllMocks())

describe('TorrentClient.vue', () => {
  it('renders the stats strip from healthExtended', async () => {
    const wrapper = mount(TorrentClient)
    await flushPromises()
    // 500/1000 disk free = 50.0%
    expect(wrapper.text()).toContain('50.0')
    expect(wrapper.text()).toContain('player.adminLibrary.stats.activeTorrents')
    wrapper.unmount()
  })
})
```

- [ ] **Step 3: Run test to verify it passes**

Run: `cd /data/ae-fm-ux/frontend/web && bunx vitest run src/views/admin/rawlibrary/__tests__/TorrentClient.spec.ts`
Expected: PASS. If it fails on a missing helper/type import, fix the import list in `TorrentClient.vue` per Step 1.

- [ ] **Step 4: Type-check the new component**

Run: `cd /data/ae-fm-ux/frontend/web && bunx vue-tsc --noEmit 2>&1 | grep -i "TorrentClient\|rawlibrary" || echo "no TorrentClient type errors"`
Expected: `no TorrentClient type errors` (RawLibrary.vue may still reference moved code — that's fixed in Task 4; ignore RawLibrary.vue errors here).

- [ ] **Step 5: Commit**

```bash
cd /data/ae-fm-ux
git add frontend/web/src/views/admin/rawlibrary/TorrentClient.vue frontend/web/src/views/admin/rawlibrary/__tests__/TorrentClient.spec.ts
git commit -m "refactor(raw-library): extract TorrentClient panel (stats+search+jobs)"
```

---

## Task 3: Extract `FileManager.vue` (props/emits) + remove "Add torrent by hand"

Move the Files panel (today's section 4) into a component driven by `backend`+`prefix` props, emitting `navigate` for every folder/backend change. **Delete the add-torrent magnet form** and its i18n keys.

**Files:**
- Create: `frontend/web/src/views/admin/rawlibrary/FileManager.vue`
- Create: `frontend/web/src/views/admin/rawlibrary/__tests__/FileManager.spec.ts`
- Delete: `frontend/web/src/views/__tests__/RawLibrary.files.spec.ts` (superseded by the FileManager spec)
- Modify: `frontend/web/src/locales/{en,ru,ja}.json` (remove `player.adminLibrary.files.add`)

**Interfaces:**
- Consumes: `formatBytes`, `unwrap` from `@/views/admin/rawlibrary/lib`; `adminLibraryApi.browseFiles/downloadFile/deleteFile`; types `FileDomain, FileEntry, BrowseResponse, StorageBackend` from `@/types/library`.
- Produces: `<FileManager :backend :prefix @navigate />`
  - Props: `backend: FileDomain` (`'work'|'minio'|'s3'`), `prefix: string` (bucket-relative, trailing-slash or empty).
  - Emits: `navigate: (payload: { backend: FileDomain; prefix: string }) => void` — the ONLY way this component changes location; the host translates it to `router.push`.

- [ ] **Step 1: Write the FileManager component**

Create `frontend/web/src/views/admin/rawlibrary/FileManager.vue`. Template (root `<div>`), in order:

1. **Domain switch** — copy `RawLibrary.vue:289-300`, but change `@click="changeDomain(o.value)"` to emit and mark active from the `backend` prop:
```html
<div class="flex gap-2 mb-3">
  <Button
    v-for="o in fileDomainOptions"
    :key="o.value"
    :variant="props.backend === o.value ? 'default' : 'outline'"
    size="sm"
    @click="switchBackend(o.value)"
  >
    {{ o.label }}
  </Button>
</div>
```
2. **Error banner** (local): 
```html
<div v-if="errorBanner" class="glass-card p-4 mb-4 border border-destructive/40">
  <p class="text-destructive">{{ errorBanner }}</p>
</div>
```
3. **Breadcrumb** — copy `RawLibrary.vue:344-353`, but the root button and each crumb emit navigate:
```html
<nav class="flex items-center flex-wrap gap-1 text-sm text-white/60 mb-2">
  <button type="button" class="hover:text-white transition" @click="goTo('')">
    {{ $t('player.adminLibrary.files.root') }}
  </button>
  <template v-for="(c, i) in fileBreadcrumb" :key="i">
    <span>/</span>
    <button type="button" class="hover:text-white transition" @click="crumbTo(i)">{{ c }}</button>
  </template>
</nav>
```
4. **List** — copy `RawLibrary.vue:355-388` verbatim (spinner + `<ul>` of entries with the `openEntry`/`downloadEntry`/`deleteEntry` per-row actions). (The `../` row and transcript come in Tasks 5 & 6 — leave the list as-is here.)

Do NOT include the "Add torrent by hand" form (`RawLibrary.vue:302-342`).

`<script setup lang="ts">`:

```ts
import { ref, computed, watch, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminLibraryApi } from '@/api/client'
import type { FileDomain, FileEntry, BrowseResponse } from '@/types/library'
import Badge from '@/components/ui/Badge.vue'
import Button from '@/components/ui/Button.vue'
import { Spinner } from '@/components/ui'
import { useConfirm } from '@/composables/useConfirm'
import { formatBytes, unwrap } from '@/views/admin/rawlibrary/lib'

const props = defineProps<{ backend: FileDomain; prefix: string }>()
const emit = defineEmits<{ navigate: [payload: { backend: FileDomain; prefix: string }] }>()

const { t } = useI18n()
const { confirm } = useConfirm()

const fileEntries = ref<FileEntry[]>([])
const fileBreadcrumb = ref<string[]>([])
const filesLoading = ref(false)
const errorBanner = ref<string | null>(null)

const fileDomainOptions = computed<{ value: FileDomain; label: string }[]>(() => [
  { value: 'work', label: t('player.adminLibrary.files.domain.work') },
  { value: 'minio', label: t('player.adminLibrary.files.domain.minio') },
  { value: 's3', label: t('player.adminLibrary.files.domain.s3') },
])

function freshnessVariant(freshness: 'fresh' | 'stale'): 'default' | 'secondary' {
  return freshness === 'fresh' ? 'default' : 'secondary'
}
function freshnessLabel(freshness: 'fresh' | 'stale'): string {
  return freshness === 'fresh' ? t('player.adminLibrary.files.fresh') : t('player.adminLibrary.files.stale')
}

async function loadFiles() {
  filesLoading.value = true
  errorBanner.value = null
  try {
    const resp = await adminLibraryApi.browseFiles(props.backend, props.prefix)
    const body = unwrap<BrowseResponse>(resp)
    if (body) {
      fileEntries.value = body.entries
      fileBreadcrumb.value = body.breadcrumb
    }
  } catch (err) {
    console.warn('loadFiles failed', err)
  } finally {
    filesLoading.value = false
  }
}

// The host drives backend+prefix via the route; reload whenever either changes.
watch(() => [props.backend, props.prefix], loadFiles)
onMounted(loadFiles)

function goTo(prefix: string) {
  emit('navigate', { backend: props.backend, prefix })
}
function switchBackend(backend: FileDomain) {
  emit('navigate', { backend, prefix: '' })
}
function openEntry(e: FileEntry) {
  if (e.kind === 'dir') goTo(props.prefix + e.name + '/')
}
function crumbTo(idx: number) {
  goTo(fileBreadcrumb.value.slice(0, idx + 1).join('/') + '/')
}

// The work domain never sets FileEntry.key; reconstruct it from prefix + name.
function keyFor(e: FileEntry): string {
  return e.key ?? (props.prefix + e.name)
}

async function downloadEntry(e: FileEntry) {
  const key = keyFor(e)
  const { data } = await adminLibraryApi.downloadFile(props.backend, key)
  const url = URL.createObjectURL(data as Blob)
  const a = document.createElement('a')
  a.href = url
  a.download = e.name
  a.click()
  URL.revokeObjectURL(url)
}

async function deleteEntry(e: FileEntry) {
  const key = keyFor(e)
  const msgKey = props.backend === 'work'
    ? 'player.adminLibrary.files.confirm.work'
    : e.episode ? 'player.adminLibrary.files.confirm.episode'
      : 'player.adminLibrary.files.confirm.orphan'
  if (!(await confirm({ description: t(msgKey, { name: e.name }), variant: 'destructive' }))) return
  try {
    await adminLibraryApi.deleteFile(props.backend, key, !e.episode)
    await loadFiles()
  } catch (err) {
    const status = (err as { response?: { status?: number } })?.response?.status
    if (status === 409) {
      const reason = (err as { response?: { data?: { reason?: string } } })?.response?.data?.reason
      if (reason === 'torrent_active') errorBanner.value = t('player.adminLibrary.files.error.active')
      else if (reason === 'episode_member') errorBanner.value = t('player.adminLibrary.files.error.episodeMember')
      else errorBanner.value = t('player.adminLibrary.files.error.deleteFailed')
    } else {
      console.warn('file delete failed', err)
      errorBanner.value = t('player.adminLibrary.files.error.deleteFailed')
    }
  }
}
```

Note: `openEntry`/`downloadEntry`/`deleteEntry` are referenced by the template copied from `RawLibrary.vue:355-388`; keep those `@click` handlers exactly as in the source.

- [ ] **Step 2: Remove the add-torrent i18n keys (all three locales)**

In `frontend/web/src/locales/en.json`, `ru.json`, `ja.json`, delete the `"add": { ... }` object inside `player.adminLibrary.files` (en.json line ~421). Leave a valid JSON object (mind the trailing comma on the preceding `"stale"` line).

- [ ] **Step 3: Write the FileManager spec (refit from the old files spec)**

Delete the old spec and create the new one:

```bash
cd /data/ae-fm-ux && git rm frontend/web/src/views/__tests__/RawLibrary.files.spec.ts
```

Create `frontend/web/src/views/admin/rawlibrary/__tests__/FileManager.spec.ts`:

```ts
import { describe, it, expect, vi, afterEach } from 'vitest'
import { mount, flushPromises, type VueWrapper } from '@vue/test-utils'
import FileManager from '@/views/admin/rawlibrary/FileManager.vue'
import { adminLibraryApi } from '@/api/client'
import type { BrowseResponse } from '@/types/library'

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return { ...actual, useI18n: () => ({ t: (k: string) => k }) }
})
const { confirmMock } = vi.hoisted(() => ({ confirmMock: vi.fn() }))
vi.mock('@/composables/useConfirm', () => ({ useConfirm: () => ({ confirm: confirmMock }) }))
vi.mock('@/api/client', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api/client')>()
  return {
    ...actual,
    adminLibraryApi: {
      ...actual.adminLibraryApi,
      browseFiles: vi.fn(),
      deleteFile: vi.fn().mockResolvedValue({ data: { success: true, data: { deleted: true } } }),
      downloadFile: vi.fn(),
    },
    animeApi: { ...actual.animeApi, resolveShikimori: vi.fn() },
  }
})

function browseResponse(overrides: Partial<BrowseResponse> = {}) {
  return { data: { success: true, data: {
    domain: 'minio', prefix: '', breadcrumb: [], entries: [
      { name: 'season1', kind: 'dir', size: 2048 },
      { name: 'episode01.mp4', kind: 'file', size: 1024, key: 'episode01.mp4' },
    ], ...overrides,
  } } }
}

afterEach(() => { vi.clearAllMocks(); confirmMock.mockReset() })

async function mountFM(prefix = ''): Promise<VueWrapper> {
  ;(adminLibraryApi.browseFiles as ReturnType<typeof vi.fn>).mockResolvedValue(browseResponse({ prefix }))
  const wrapper = mount(FileManager, { props: { backend: 'minio', prefix } })
  await flushPromises()
  return wrapper
}

describe('FileManager.vue', () => {
  it('browses the backend/prefix from props', async () => {
    const wrapper = await mountFM('')
    expect(adminLibraryApi.browseFiles).toHaveBeenCalledWith('minio', '')
    expect(wrapper.text()).toContain('season1')
    expect(wrapper.text()).toContain('episode01.mp4')
    wrapper.unmount()
  })

  it('emits navigate when opening a folder', async () => {
    const wrapper = await mountFM('')
    await wrapper.findAll('button').find((b) => b.text().includes('season1'))!.trigger('click')
    expect(wrapper.emitted('navigate')![0]).toEqual([{ backend: 'minio', prefix: 'season1/' }])
    wrapper.unmount()
  })

  it('has NO add-torrent-by-hand form', async () => {
    const wrapper = await mountFM('')
    expect(wrapper.text()).not.toContain('player.adminLibrary.files.add')
    expect(wrapper.find('input[placeholder*="magnet" i]').exists()).toBe(false)
    wrapper.unmount()
  })
})
```

- [ ] **Step 4: Run the spec**

Run: `cd /data/ae-fm-ux/frontend/web && bunx vitest run src/views/admin/rawlibrary/__tests__/FileManager.spec.ts`
Expected: PASS (3 tests).

- [ ] **Step 5: Commit**

```bash
cd /data/ae-fm-ux
git add frontend/web/src/views/admin/rawlibrary/FileManager.vue \
        frontend/web/src/views/admin/rawlibrary/__tests__/FileManager.spec.ts \
        frontend/web/src/locales/en.json frontend/web/src/locales/ru.json frontend/web/src/locales/ja.json
git rm --cached frontend/web/src/views/__tests__/RawLibrary.files.spec.ts 2>/dev/null || true
git commit -m "refactor(raw-library): extract FileManager panel, drop add-torrent-by-hand"
```

---

## Task 4: Rebuild `RawLibrary.vue` as tab host + wire routes (deep-linking)

Replace `RawLibrary.vue`'s body with a thin tab host that renders `<Tabs>` + the two panels, derives the active tab from the route, and translates FileManager's `navigate` emits into `router.push`. Add the file-manager route.

**Files:**
- Modify (replace body): `frontend/web/src/views/admin/RawLibrary.vue`
- Modify: `frontend/web/src/router/index.ts` (add the file-manager route)
- Modify: `frontend/web/src/locales/{en,ru,ja}.json` (add tab labels; relabel s3)
- Test: `frontend/web/src/views/admin/rawlibrary/__tests__/RawLibrary.host.spec.ts`

**Interfaces:**
- Consumes: `<TorrentClient />` (Task 2), `<FileManager :backend :prefix @navigate />` (Task 3), `Tabs` from `@/components/ui`, `useRoute`/`useRouter` from `vue-router`.
- Produces: the two named routes `admin-raw-library` and `admin-raw-library-files`.

- [ ] **Step 1: Add the file-manager route**

In `frontend/web/src/router/index.ts`, the existing block (`:257-265`) keeps `path: '/admin/raw-library'`, `name: 'admin-raw-library'`, `component: () => import('@/views/admin/RawLibrary.vue')`. Immediately AFTER that object, add a second record (same component):

```ts
  {
    // ① File Manager deep-link: backend (work|minio|s3) + catch-all folder path.
    // Same component as admin-raw-library; the host derives the active tab from
    // route.name. Vue Router 4 catch-all `(.*)*` → filepath arrives as string[].
    path: '/admin/raw-library/file-manager/:backend/:filepath(.*)*',
    name: 'admin-raw-library-files',
    component: () => import('@/views/admin/RawLibrary.vue'),
    meta: { titleKey: 'player.adminLibrary.title', requiresAuth: true, requiresAdmin: true },
  },
```

- [ ] **Step 2: Replace `RawLibrary.vue` with the tab host**

Overwrite `frontend/web/src/views/admin/RawLibrary.vue` entirely:

```vue
<template>
  <div class="min-h-screen bg-base">
    <div class="container mx-auto px-4 py-8 max-w-7xl">
      <div class="flex flex-wrap items-center justify-between gap-3 mb-6">
        <h1 class="text-3xl font-semibold text-white">{{ $t('player.adminLibrary.title') }}</h1>
      </div>

      <Tabs :model-value="activeTab" :tabs="tabDefs" variant="underline" @update:model-value="onTabChange">
        <template #torrent-client>
          <TorrentClient />
        </template>
        <template #file-manager>
          <FileManager :backend="backend" :prefix="prefix" @navigate="onNavigate" />
        </template>
      </Tabs>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { Tabs } from '@/components/ui'
import TorrentClient from '@/views/admin/rawlibrary/TorrentClient.vue'
import FileManager from '@/views/admin/rawlibrary/FileManager.vue'
import type { FileDomain } from '@/types/library'

const route = useRoute()
const router = useRouter()
const { t } = useI18n()

const VALID_BACKENDS: FileDomain[] = ['work', 'minio', 's3']

const tabDefs = computed(() => [
  { value: 'torrent-client', label: t('player.adminLibrary.tabs.torrentClient') },
  { value: 'file-manager', label: t('player.adminLibrary.tabs.fileManager') },
])

const activeTab = computed(() =>
  route.name === 'admin-raw-library-files' ? 'file-manager' : 'torrent-client',
)

// Normalize the route's :backend (default/invalid → minio).
const backend = computed<FileDomain>(() => {
  const b = route.params.backend as string | undefined
  return VALID_BACKENDS.includes(b as FileDomain) ? (b as FileDomain) : 'minio'
})

// Catch-all `:filepath(.*)*` arrives as string[] (segments). Rebuild the
// bucket-relative prefix (trailing slash, or '' at root).
const prefix = computed(() => {
  const fp = route.params.filepath as string[] | string | undefined
  const segs = Array.isArray(fp) ? fp : fp ? [fp] : []
  return segs.length ? segs.join('/') + '/' : ''
})

function onTabChange(value: string) {
  if (value === 'file-manager') {
    router.push({ name: 'admin-raw-library-files', params: { backend: 'minio', filepath: [] } })
  } else {
    router.push({ name: 'admin-raw-library' })
  }
}

function onNavigate(payload: { backend: FileDomain; prefix: string }) {
  const segs = payload.prefix.split('/').filter(Boolean)
  router.push({ name: 'admin-raw-library-files', params: { backend: payload.backend, filepath: segs } })
}
</script>
```

- [ ] **Step 3: Add tab-label i18n keys + relabel s3 (all three locales)**

In each of `en.json`/`ru.json`/`ja.json`, inside `player.adminLibrary`, add a `tabs` object and update the `files.domain.s3` label:

en.json — add after `"title": "Raw Library",`:
```json
      "tabs": { "torrentClient": "Torrent Client", "fileManager": "File Manager" },
```
and change `"domain": { "work": "Torrent work dir", "minio": "MinIO", "s3": "S3 cloud" }` → `"s3": "S3 · firstvds"`.

ru.json — `"tabs": { "torrentClient": "Торрент-клиент", "fileManager": "Файловый менеджер" },` and `"s3": "S3 · firstvds"`.

ja.json — `"tabs": { "torrentClient": "トレントクライアント", "fileManager": "ファイルマネージャー" },` and `"s3": "S3 · firstvds"`.

- [ ] **Step 4: Write the host route-sync test**

Create `frontend/web/src/views/admin/rawlibrary/__tests__/RawLibrary.host.spec.ts`:

```ts
import { describe, it, expect, vi, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createRouter, createMemoryHistory, type Router } from 'vue-router'
import RawLibrary from '@/views/admin/RawLibrary.vue'

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return { ...actual, useI18n: () => ({ t: (k: string) => k }) }
})
// Stub both panels so the host test doesn't pull their api/polling.
vi.mock('@/views/admin/rawlibrary/TorrentClient.vue', () => ({
  default: { name: 'TorrentClient', template: '<div class="tc-stub" />' },
}))
vi.mock('@/views/admin/rawlibrary/FileManager.vue', () => ({
  default: {
    name: 'FileManager',
    props: ['backend', 'prefix'],
    emits: ['navigate'],
    template: '<div class="fm-stub">{{ backend }}|{{ prefix }}</div>',
  },
}))

function makeRouter(): Router {
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/admin/raw-library', name: 'admin-raw-library', component: RawLibrary },
      { path: '/admin/raw-library/file-manager/:backend/:filepath(.*)*', name: 'admin-raw-library-files', component: RawLibrary },
    ],
  })
}

afterEach(() => vi.clearAllMocks())

describe('RawLibrary host', () => {
  it('defaults to the Torrent Client tab at the base route', async () => {
    const router = makeRouter()
    router.push('/admin/raw-library'); await router.isReady()
    const wrapper = mount(RawLibrary, { global: { plugins: [router] } })
    await flushPromises()
    expect(wrapper.find('.tc-stub').exists()).toBe(true)
    expect(wrapper.find('.fm-stub').exists()).toBe(false)
  })

  it('deep-links a folder into the File Manager tab', async () => {
    const router = makeRouter()
    router.push('/admin/raw-library/file-manager/minio/aeProvider/11981/RAW'); await router.isReady()
    const wrapper = mount(RawLibrary, { global: { plugins: [router] } })
    await flushPromises()
    expect(wrapper.find('.fm-stub').text()).toBe('minio|aeProvider/11981/RAW/')
  })

  it('pushes the file-manager route when a FileManager navigate is emitted', async () => {
    const router = makeRouter()
    router.push('/admin/raw-library/file-manager/minio'); await router.isReady()
    const wrapper = mount(RawLibrary, { global: { plugins: [router] } })
    await flushPromises()
    wrapper.findComponent({ name: 'FileManager' }).vm.$emit('navigate', { backend: 's3', prefix: 'aeProvider/4901/' })
    await flushPromises()
    expect(router.currentRoute.value.fullPath).toBe('/admin/raw-library/file-manager/s3/aeProvider/4901')
  })
})
```

- [ ] **Step 5: Run tests + typecheck**

Run: `cd /data/ae-fm-ux/frontend/web && bunx vitest run src/views/admin/rawlibrary/__tests__/RawLibrary.host.spec.ts && bunx vue-tsc --noEmit 2>&1 | grep -iE "rawlibrary|RawLibrary" || echo "typecheck clean for raw-library"`
Expected: host spec PASS (3 tests); `typecheck clean for raw-library`.

- [ ] **Step 6: Commit**

```bash
cd /data/ae-fm-ux
git add frontend/web/src/views/admin/RawLibrary.vue frontend/web/src/router/index.ts \
        frontend/web/src/locales/en.json frontend/web/src/locales/ru.json frontend/web/src/locales/ja.json \
        frontend/web/src/views/admin/rawlibrary/__tests__/RawLibrary.host.spec.ts
git commit -m "feat(raw-library): tabbed host with deep-linked file-manager route"
```

---

## Task 5: `../` parent-folder row

Add a synthetic `../` entry at the top of the FileManager list whenever not at root; clicking it navigates up one path segment.

**Files:**
- Modify: `frontend/web/src/views/admin/rawlibrary/FileManager.vue`
- Modify: `frontend/web/src/views/admin/rawlibrary/__tests__/FileManager.spec.ts`
- Modify: `frontend/web/src/locales/{en,ru,ja}.json` (add `files.parent`)

**Interfaces:**
- Consumes: `emit('navigate')`, `props.prefix` (Task 3).
- Produces: `parentPrefix(): string` helper (prefix one level up).

- [ ] **Step 1: Add the failing test**

Append to `describe('FileManager.vue', ...)` in `FileManager.spec.ts`:

```ts
  it('shows ../ only when not at root and navigates up one level', async () => {
    const atRoot = await mountFM('')
    expect(atRoot.text()).not.toContain('player.adminLibrary.files.parent')
    atRoot.unmount()

    const nested = await mountFM('aeProvider/11981/RAW/')
    const up = nested.findAll('button').find((b) => b.text().includes('player.adminLibrary.files.parent'))
    expect(up).toBeTruthy()
    await up!.trigger('click')
    expect(nested.emitted('navigate')![0]).toEqual([{ backend: 'minio', prefix: 'aeProvider/11981/' }])
    nested.unmount()
  })
```

- [ ] **Step 2: Run it to verify failure**

Run: `cd /data/ae-fm-ux/frontend/web && bunx vitest run src/views/admin/rawlibrary/__tests__/FileManager.spec.ts -t "navigates up"`
Expected: FAIL — no `../` row rendered.

- [ ] **Step 3: Implement the parent row**

In `FileManager.vue` script, add:
```ts
function parentPrefix(): string {
  const segs = props.prefix.split('/').filter(Boolean)
  segs.pop()
  return segs.length ? segs.join('/') + '/' : ''
}
```
In the template, immediately INSIDE the `<ul>` (before the `v-if="fileEntries.length === 0"` empty `<li>` and the `v-for` list), add:
```html
<li v-if="prefix !== ''" class="flex items-center gap-2 p-3">
  <button
    type="button"
    class="flex items-center gap-2 min-w-0 text-left text-white"
    :aria-label="$t('player.adminLibrary.files.parent')"
    @click="goTo(parentPrefix())"
  >
    <span class="truncate">📁 {{ $t('player.adminLibrary.files.parent') }}</span>
  </button>
</li>
```
Also guard the empty-state `<li>` so `../` isn't swallowed by it: change its condition to `v-if="fileEntries.length === 0 && prefix === ''"` (a non-root empty dir still shows `../`).

- [ ] **Step 4: Add the `files.parent` key (all three locales)**

Inside `player.adminLibrary.files`, add `"parent": "..",` in `en.json`/`ru.json`/`ja.json` (the label is the literal `..` in every locale; the key exists so the aria-label routes through i18n and passes the i18n-parity gate).

- [ ] **Step 5: Run tests to verify pass**

Run: `cd /data/ae-fm-ux/frontend/web && bunx vitest run src/views/admin/rawlibrary/__tests__/FileManager.spec.ts`
Expected: PASS (4 tests).

- [ ] **Step 6: Commit**

```bash
cd /data/ae-fm-ux
git add frontend/web/src/views/admin/rawlibrary/FileManager.vue \
        frontend/web/src/views/admin/rawlibrary/__tests__/FileManager.spec.ts \
        frontend/web/src/locales/en.json frontend/web/src/locales/ru.json frontend/web/src/locales/ja.json
git commit -m "feat(raw-library): add ../ parent-folder navigation row"
```

---

## Task 6: Anime-title "transcript" for `aeProvider/<id>/` folders

When browsing `aeProvider/` on an object store, resolve each numeric child folder's Shikimori ID to a title (module-cached) and render `<id> — <title>`.

**Files:**
- Modify: `frontend/web/src/api/client.ts` (add `animeApi.resolveShikimori`)
- Modify: `frontend/web/src/views/admin/rawlibrary/FileManager.vue`
- Modify: `frontend/web/src/views/admin/rawlibrary/__tests__/FileManager.spec.ts`

**Interfaces:**
- Consumes: `GET /api/anime/shikimori/{id}` → envelope `{ data: { name?, name_ru?, ... } }`.
- Produces: `animeApi.resolveShikimori(id: string)`; reactive `titleFor(id: string): string | undefined` in FileManager.

- [ ] **Step 1: Add the API client method**

In `frontend/web/src/api/client.ts`, inside the `animeApi` object (after `search`, `:383`), add:
```ts
  resolveShikimori: (shikimoriId: string) => apiClient.get(`/anime/shikimori/${shikimoriId}`),
```

- [ ] **Step 2: Add the failing test**

In `FileManager.spec.ts`, extend the api/client mock's `animeApi.resolveShikimori` (already stubbed in Task 3's mock) and append:

```ts
  it('resolves anime titles for numeric folders under aeProvider/', async () => {
    ;(adminLibraryApi.browseFiles as ReturnType<typeof vi.fn>).mockResolvedValue({
      data: { success: true, data: { domain: 'minio', prefix: 'aeProvider/', breadcrumb: ['aeProvider'],
        entries: [{ name: '11981', kind: 'dir', size: 42 }, { name: 'pending', kind: 'dir', size: 1 }] } },
    })
    const { animeApi } = await import('@/api/client')
    ;(animeApi.resolveShikimori as ReturnType<typeof vi.fn>).mockResolvedValue({
      data: { success: true, data: { name: 'Frieren', name_ru: 'Фрирен' } },
    })
    const wrapper = mount(FileManager, { props: { backend: 'minio', prefix: 'aeProvider/' } })
    await flushPromises(); await flushPromises()
    expect(animeApi.resolveShikimori).toHaveBeenCalledWith('11981')
    expect(animeApi.resolveShikimori).not.toHaveBeenCalledWith('pending') // non-numeric skipped
    expect(wrapper.text()).toContain('Frieren')
    wrapper.unmount()
  })
```

- [ ] **Step 3: Run it to verify failure**

Run: `cd /data/ae-fm-ux/frontend/web && bunx vitest run src/views/admin/rawlibrary/__tests__/FileManager.spec.ts -t "resolves anime titles"`
Expected: FAIL — `resolveShikimori` not called / title not rendered.

- [ ] **Step 4: Implement the transcript cache + render**

In `FileManager.vue`, add the import and a MODULE-LEVEL cache (outside `<script setup>`'s component scope so it survives remounts — declare it at top-level of the module, before `defineProps`):

```ts
import { animeApi } from '@/api/client'
import { reactive } from 'vue'

// Module-level Shikimori-id → display title cache, shared across FileManager
// remounts within the session. Value '' = resolved-but-untitled/404 (don't refetch).
const titleCache = reactive<Record<string, string>>({})
```
(Adjust the existing `import { ref, computed, watch, onMounted } from 'vue'` to also import `reactive`, or add the separate import line above.)

Add resolution logic, called from `loadFiles` after entries are set:
```ts
function titleFor(name: string): string | undefined {
  return titleCache[name] || undefined
}

async function resolveTranscripts() {
  // Only aeProvider/<id> folders carry an anime id (RawPrefix layout).
  if (props.backend === 'work' || props.prefix !== 'aeProvider/') return
  const ids = fileEntries.value
    .filter((e) => e.kind === 'dir' && /^\d+$/.test(e.name) && !(e.name in titleCache))
    .map((e) => e.name)
  await Promise.all(ids.map(async (id) => {
    try {
      const resp = await animeApi.resolveShikimori(id)
      const a = unwrap<{ name?: string; name_ru?: string }>(resp)
      titleCache[id] = a?.name || a?.name_ru || ''
    } catch {
      titleCache[id] = '' // 404/err → cache empty so we show bare id and don't refetch
    }
  }))
}
```
At the end of `loadFiles`'s `try` block (after `fileBreadcrumb.value = body.breadcrumb`), add `void resolveTranscripts()`.

In the template, in the entry-name button (copied from `RawLibrary.vue:367-377`), append the title after `{{ e.name }}`:
```html
<span class="truncate">
  {{ e.kind === 'dir' ? '📁' : '📄' }} {{ e.name }}<span v-if="titleFor(e.name)" class="text-white/50"> — {{ titleFor(e.name) }}</span>
</span>
```

- [ ] **Step 5: Run tests to verify pass**

Run: `cd /data/ae-fm-ux/frontend/web && bunx vitest run src/views/admin/rawlibrary/__tests__/FileManager.spec.ts`
Expected: PASS (5 tests).

- [ ] **Step 6: Commit**

```bash
cd /data/ae-fm-ux
git add frontend/web/src/api/client.ts frontend/web/src/views/admin/rawlibrary/FileManager.vue \
        frontend/web/src/views/admin/rawlibrary/__tests__/FileManager.spec.ts
git commit -m "feat(raw-library): resolve anime titles for aeProvider/<id> folders"
```

---

## Task 7: Full-suite verification + `/frontend-verify` + land

Run the project's FE pre-flight (DS-lint + i18n en/ru/ja parity + real `bun run build`), fix anything it flags, then land the branch.

**Files:** none new (verification + fixes only).

- [ ] **Step 1: Run the whole raw-library suite**

Run: `cd /data/ae-fm-ux/frontend/web && bunx vitest run src/views/admin/rawlibrary/`
Expected: all specs PASS (lib, TorrentClient, FileManager, RawLibrary.host).

- [ ] **Step 2: Type-check the whole app**

Run: `cd /data/ae-fm-ux/frontend/web && bunx vue-tsc --noEmit`
Expected: no errors. (Fix any dangling reference to the old inline `RawLibrary.vue` symbols — there should be none outside the files touched here.)

- [ ] **Step 3: Run `/frontend-verify`**

Invoke the `frontend-verify` skill (DS-lint gate, i18n en/ru/ja parity, real `bun run build`, lucide/TS2614/Tailwind-v4 traps). Expected: green. Fix any DS-lint or parity failures it reports (common: a hardcoded hue, or a locale key present in en but missing in ru/ja).

- [ ] **Step 4: Land the branch**

```bash
cd /data/ae-fm-ux
printf 'feat(raw-library): file-manager UX overhaul — tabs, deep-linking, transcripts\n\nSplit RawLibrary.vue into a tabbed host (Torrent Client / File Manager)\nover the DS Tabs component; deep-link the file browser at\n/admin/raw-library/file-manager/<backend>/<path>; add a ../ parent row\nand anime-title transcripts for aeProvider/<id> folders; remove the\nAdd-torrent-by-hand form. Frontend-only.' | bin/ae-land.sh \
  frontend/web/src/views/admin/RawLibrary.vue \
  frontend/web/src/views/admin/rawlibrary/ \
  frontend/web/src/router/index.ts \
  frontend/web/src/api/client.ts \
  frontend/web/src/locales/en.json frontend/web/src/locales/ru.json frontend/web/src/locales/ja.json
```
Expected: `LANDED: <sha> feat(raw-library): …`. On rebase conflict, `ae-land.sh` STOPS and lists files — resolve, then `git add <files> && GIT_EDITOR=true git rebase --continue && git push origin HEAD:main`.

- [ ] **Step 5: After-update (deploy + changelog)**

Invoke `/animeenigma-after-update` (runs `/simplify` over the diff, redeploys `web`, health-checks, appends the Russian-Trump-mode changelog entry, commits+pushes). This is a `frontend/web` change → `make redeploy-web` rebuilds nginx; the DS-lint gate also runs there.

---

## Self-Review

**Spec coverage:**
- Two tabs (Torrent Client / File Manager) → Tasks 2, 3, 4. ✓
- URL wiring `/file-manager/<backend>/<path>`, torrent default → Task 4 (route + host). ✓
- `../` parent nav → Task 5. ✓
- Anime-title transcript for numeric `aeProvider/` folders → Task 6. ✓
- Remove "Add torrent by hand" → Task 3. ✓
- `s3` URL + "S3 · firstvds" label → Task 4 Step 3. ✓
- Component split (host + TorrentClient + FileManager + lib) → Tasks 1–4. ✓
- Polling pauses on FM tab (Tabs unmounts inactive) → inherent to Task 4's slot design. ✓
- i18n en/ru/ja parity → Tasks 3 (remove add.*), 4 (tabs + s3), 5 (parent); gate in Task 7. ✓
- No backend change → only `api/client.ts` touched (Task 6); no `services/` edits. ✓
- Deferrals: Upload button (②) and orphan highlight (③) — correctly absent. ✓

**Placeholder scan:** No TBD/TODO/"handle edge cases"/"similar to Task N". Extraction tasks give exact source line ranges + exact wiring edits; new logic ships full code. ✓

**Type consistency:** `navigate` emit payload `{ backend: FileDomain; prefix: string }` is identical in FileManager (emit, Task 3), the host (`onNavigate`, Task 4), and the specs (Tasks 3/5). `FileDomain` = `'work'|'minio'|'s3'` used consistently. `unwrap`/`formatBytes` signatures match Task 1 across consumers. `resolveShikimori(shikimoriId: string)` (Task 6) matches its spec usage. Route name `admin-raw-library-files` and param names `backend`/`filepath` are identical in the route def (Task 4 Step 1), host computeds (Step 2), and host spec (Step 4). ✓
