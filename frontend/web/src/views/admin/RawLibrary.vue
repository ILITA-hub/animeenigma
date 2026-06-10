<template>
  <div class="min-h-screen bg-base pt-20">
    <div class="container mx-auto px-4 py-8 max-w-7xl">
      <!-- Header -->
      <div class="flex flex-wrap items-center justify-between gap-3 mb-6">
        <h1 class="text-3xl font-bold text-white">{{ $t('player.adminLibrary.title') }}</h1>
      </div>

      <!-- Error banner -->
      <div v-if="errorBanner" class="glass-card p-4 mb-6 border border-destructive/40">
        <p class="text-destructive">{{ errorBanner }}</p>
      </div>

      <!-- 1. Stats strip -->
      <section class="grid grid-cols-1 sm:grid-cols-3 gap-4 mb-8" aria-label="library stats">
        <div class="glass-card p-4">
          <div class="text-white/60 text-xs uppercase tracking-wide mb-1">
            {{ $t('player.adminLibrary.stats.diskFree') }}
          </div>
          <div class="text-2xl font-bold text-cyan-300">
            <template v-if="health">
              {{ formatPct(health.disk_free_bytes, health.disk_total_bytes) }}%
            </template>
            <template v-else>—</template>
          </div>
          <div v-if="health" class="text-xs text-white/50 mt-1">
            {{ formatGB(health.disk_free_bytes) }} / {{ formatGB(health.disk_total_bytes) }} GB
          </div>
        </div>
        <div class="glass-card p-4">
          <div class="text-white/60 text-xs uppercase tracking-wide mb-1">
            {{ $t('player.adminLibrary.stats.activeTorrents') }}
          </div>
          <div class="text-2xl font-bold text-cyan-300">{{ health?.active_torrents ?? '—' }}</div>
        </div>
        <div class="glass-card p-4">
          <div class="text-white/60 text-xs uppercase tracking-wide mb-1">
            {{ $t('player.adminLibrary.stats.activeJobs') }}
          </div>
          <div class="text-2xl font-bold text-cyan-300">{{ totalActiveJobs }}</div>
        </div>
      </section>

      <!-- 2. Search panel -->
      <section class="mb-8">
        <h2 class="text-xl font-semibold text-white mb-3">
          {{ $t('player.adminLibrary.search.title') }}
        </h2>
        <form class="glass-card p-4 mb-4 flex flex-wrap gap-3 items-end" @submit.prevent="handleSearch">
          <div class="flex-1 min-w-[260px]">
            <Input v-model="searchQuery" type="text" size="sm" :placeholder="$t('player.adminLibrary.search.placeholder')" :aria-label="$t('player.adminLibrary.search.placeholder')" />
          </div>
          <div class="w-32">
            <Input v-model.number="searchMalId" type="number" size="sm" placeholder="MAL ID" aria-label="MAL ID" />
          </div>
          <Button
            type="submit"
            variant="default"
            size="sm"
            :disabled="searching"
            :aria-label="$t('player.adminLibrary.search.submit')"
          >
            {{ $t('player.adminLibrary.search.submit') }}
          </Button>
        </form>

        <div v-if="searching" class="flex justify-center py-6">
          <Spinner size="md" />
        </div>
        <div v-else-if="searchResults.length === 0" class="glass-card p-4 text-white/60 text-sm">
          {{ $t('player.adminLibrary.search.empty') }}
        </div>
        <div v-else class="glass-card overflow-x-auto">
          <table class="w-full text-sm text-white">
            <thead class="bg-black/40">
              <tr class="text-white/70 text-xs uppercase">
                <th class="px-3 py-2 text-left">Provider</th>
                <th class="px-3 py-2 text-left">Uploader</th>
                <th class="px-3 py-2 text-left">Title</th>
                <th class="px-3 py-2 text-left">Quality</th>
                <th class="px-3 py-2 text-right">Size</th>
                <th class="px-3 py-2 text-left">Magnet</th>
                <th class="px-3 py-2"></th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="(release, idx) in searchResults"
                :key="`${release.source}-${idx}-${release.magnet.slice(0, 16)}`"
                class="border-t border-white/10 hover:bg-white/5"
              >
                <td class="px-3 py-2">
                  <Badge :variant="release.source === 'animetosho' ? 'primary' : 'info'" size="sm">
                    {{ $t('player.adminLibrary.search.providers.' + release.source) }}
                  </Badge>
                </td>
                <td class="px-3 py-2 text-white/70">{{ release.uploader || '—' }}</td>
                <td class="px-3 py-2 truncate max-w-md" :title="release.title">{{ release.title }}</td>
                <td class="px-3 py-2 text-white/70">{{ release.quality || '—' }}</td>
                <td class="px-3 py-2 text-right font-mono text-white/70">{{ formatBytes(release.size_bytes) }}</td>
                <td class="px-3 py-2 text-white/40 font-mono text-xs">{{ truncateMagnet(release.magnet) }}</td>
                <td class="px-3 py-2 text-right">
                  <Button
                    variant="default"
                    size="xs"
                    :aria-label="$t('player.adminLibrary.search.queue')"
                    @click="queueJob(release)"
                  >
                    {{ $t('player.adminLibrary.search.queue') }}
                  </Button>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>

      <!-- 3. Jobs panel -->
      <section class="mb-8">
        <h2 class="text-xl font-semibold text-white mb-3">
          {{ $t('player.adminLibrary.jobs.title') }}
        </h2>

        <div v-if="activeJobs.length === 0" class="glass-card p-4 text-white/60 text-sm mb-4">
          {{ $t('player.adminLibrary.jobs.empty') }}
        </div>
        <ul v-else class="space-y-3 mb-6">
          <li
            v-for="job in activeJobs"
            :key="job.id"
            class="glass-card p-4"
          >
            <div class="flex items-start justify-between gap-3 mb-2">
              <div class="min-w-0 flex-1">
                <div class="text-white font-medium truncate" :title="job.title">{{ job.title }}</div>
                <div class="text-xs text-white/40 font-mono mt-0.5">{{ job.id.slice(0, 8) }} · {{ job.source }}</div>
              </div>
              <Badge :variant="statusVariant(job.status)" size="sm">
                {{ $t('player.adminLibrary.jobs.status.' + job.status) }}
              </Badge>
            </div>
            <div class="h-2 bg-white/10 rounded overflow-hidden mb-1">
              <div
                class="h-full bg-cyan-500 transition-all duration-500"
                :style="{ width: job.progress_pct + '%' }"
              />
            </div>
            <div class="flex items-center justify-between text-xs text-white/50">
              <span>
                <template v-if="job.size_bytes > 0">
                  {{ formatBytes(Math.floor(job.size_bytes * job.progress_pct / 100)) }} /
                  {{ formatBytes(job.size_bytes) }}
                </template>
                <template v-else>{{ job.progress_pct }}%</template>
              </span>
              <button
                type="button"
                class="px-2 py-1 rounded bg-white/10 hover:bg-destructive/40 text-white/80 hover:text-white text-xs transition"
                :aria-label="$t('player.adminLibrary.jobs.cancel')"
                @click="cancelJob(job)"
              >
                {{ $t('player.adminLibrary.jobs.cancel') }}
              </button>
            </div>
          </li>
        </ul>

        <!-- Failed sub-section -->
        <div v-if="failedJobs.length > 0" class="mb-6">
          <h3 class="text-sm font-semibold text-destructive mb-2 uppercase tracking-wide">
            {{ $t('player.adminLibrary.jobs.failed.title') }}
          </h3>
          <ul class="space-y-2">
            <li
              v-for="job in failedJobs"
              :key="job.id"
              class="glass-card p-3 border border-destructive/20"
            >
              <div class="flex items-start justify-between gap-3">
                <div class="min-w-0 flex-1">
                  <div class="text-white text-sm font-medium truncate" :title="job.title">{{ job.title }}</div>
                  <div
                    v-if="job.error_text"
                    class="text-xs text-destructive mt-1 truncate"
                    :title="job.error_text"
                  >
                    {{ $t('player.adminLibrary.jobs.failed.errorText') }}: {{ job.error_text }}
                  </div>
                </div>
                <button
                  type="button"
                  class="px-3 py-1 rounded bg-warning/30 hover:bg-warning/60 text-warning text-xs font-medium transition"
                  :aria-label="$t('player.adminLibrary.jobs.retry')"
                  @click="retryJob(job)"
                >
                  {{ $t('player.adminLibrary.jobs.retry') }}
                </button>
              </div>
            </li>
          </ul>
        </div>

        <!-- Pending-link sub-section -->
        <div v-if="pendingLinkJobs.length > 0">
          <h3 class="text-sm font-semibold text-warning mb-2 uppercase tracking-wide">
            {{ $t('player.adminLibrary.jobs.pendingLink.title') }}
          </h3>
          <ul class="space-y-2">
            <li
              v-for="job in pendingLinkJobs"
              :key="job.id"
              class="glass-card p-3 border border-warning/30"
            >
              <div class="text-white text-sm font-medium truncate mb-2" :title="job.title">{{ job.title }}</div>
              <div class="relative">
                <Input
                  v-model="pendingLinkSearchQueries[job.id]"
                  type="text"
                  size="sm"
                  :placeholder="$t('player.adminLibrary.jobs.pendingLink.searchPlaceholder')"
                  :aria-label="$t('player.adminLibrary.jobs.pendingLink.searchPlaceholder')"
                  class="focus:ring-warning"
                  @input="onPendingLinkInput(job.id)"
                />
                <ul
                  v-if="pendingLinkResults[job.id]?.length"
                  class="absolute z-10 mt-1 w-full max-h-64 overflow-y-auto bg-base border border-white/20 rounded shadow-lg"
                >
                  <li
                    v-for="anime in pendingLinkResults[job.id]"
                    :key="anime.id"
                    class="px-3 py-2 hover:bg-white/10 cursor-pointer flex items-center gap-2 text-sm"
                    @click="linkJob(job, anime)"
                  >
                    <img
                      v-if="anime.poster_url"
                      :src="anime.poster_url"
                      :alt="anime.name"
                      class="w-8 h-12 object-cover rounded flex-shrink-0"
                    />
                    <span class="text-white truncate">{{ anime.name || anime.name_ru || anime.shikimori_id }}</span>
                  </li>
                </ul>
              </div>
            </li>
          </ul>
        </div>
      </section>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { adminLibraryApi, animeApi } from '@/api/client'
import type { Job, JobStatus, Release, LibraryHealth, CreateJobPayload } from '@/types/library'
import Badge from '@/components/ui/Badge.vue'
import Button from '@/components/ui/Button.vue'
import { Spinner } from '@/components/ui'
import Input from '@/components/ui/Input.vue'
import { useConfirm } from '@/composables/useConfirm'

const { confirm } = useConfirm()

// Phase 5 (LIB-09): RawLibrary admin view.
//
// Three sections: stats strip (30s poll), search panel (debounced 300ms),
// jobs panel (5s poll for active + 30s poll for failed/pending-link).

interface AnimeSearchResult {
  id: string
  shikimori_id: string
  name?: string
  name_ru?: string
  name_jp?: string
  poster_url?: string
}

// ---- Refs ----
const health = ref<LibraryHealth | null>(null)
const errorBanner = ref<string | null>(null)

const searchQuery = ref('')
const searchMalId = ref<number | undefined>(undefined)
const searchResults = ref<Release[]>([])
const searching = ref(false)

const activeJobs = ref<Job[]>([])
const failedJobs = ref<Job[]>([])
const pendingLinkJobs = ref<Job[]>([])

const pendingLinkSearchQueries = ref<Record<string, string>>({})
const pendingLinkResults = ref<Record<string, AnimeSearchResult[]>>({})

// Interval handles for cleanup.
let healthInterval: ReturnType<typeof setInterval> | null = null
let activeJobsInterval: ReturnType<typeof setInterval> | null = null
let failedPendingInterval: ReturnType<typeof setInterval> | null = null

// Per-job debounce timers for the pending-link anime search.
const pendingLinkDebounces = new Map<string, ReturnType<typeof setTimeout>>()

// ---- Computed ----
const totalActiveJobs = computed(() => {
  if (!health.value) return '—'
  return Object.values(health.value.active_jobs_by_status).reduce((a, b) => a + b, 0)
})

// ---- Helpers ----
function formatBytes(n: number): string {
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

function formatGB(n: number): string {
  return (n / 1024 / 1024 / 1024).toFixed(1)
}

function formatPct(num: number, denom: number): string {
  if (!denom || denom <= 0) return '0.0'
  return ((num * 100) / denom).toFixed(1)
}

function truncateMagnet(s: string): string {
  if (s.length <= 60) return s
  return s.slice(0, 60) + '…'
}

function statusVariant(status: JobStatus): 'default' | 'primary' | 'secondary' | 'success' | 'warning' | 'destructive' {
  switch (status) {
    case 'queued':
      return 'default'
    case 'downloading':
      return 'primary'
    case 'encoding':
      return 'warning'
    case 'uploading':
      return 'secondary'
    case 'done':
      return 'success'
    case 'failed':
      return 'destructive'
    case 'cancelled':
      return 'default'
    default:
      return 'default'
  }
}

// Unwrap the httputil envelope: { success, data }.
function unwrap<T>(resp: { data?: { data?: T } | T }): T | undefined {
  const body = resp.data as { data?: T } | T | undefined
  if (body && typeof body === 'object' && 'data' in (body as Record<string, unknown>)) {
    return (body as { data: T }).data
  }
  return body as T | undefined
}

// ---- Fetchers ----
async function fetchHealth() {
  try {
    const resp = await adminLibraryApi.healthExtended()
    const data = unwrap<LibraryHealth>(resp)
    if (data) health.value = data
  } catch (err) {
    console.warn('fetchHealth failed', err)
  }
}

async function fetchActiveJobs() {
  try {
    const resp = await adminLibraryApi.listJobs('queued,downloading,encoding,uploading', 50)
    const data = unwrap<{ jobs?: Job[] } | Job[]>(resp)
    activeJobs.value = Array.isArray(data) ? data : data?.jobs ?? []
  } catch (err) {
    console.warn('fetchActiveJobs failed', err)
  }
}

async function fetchFailedJobs() {
  try {
    const resp = await adminLibraryApi.listJobs('failed', 20)
    const data = unwrap<{ jobs?: Job[] } | Job[]>(resp)
    failedJobs.value = Array.isArray(data) ? data : data?.jobs ?? []
  } catch (err) {
    console.warn('fetchFailedJobs failed', err)
  }
}

async function fetchPendingLinkJobs() {
  try {
    const resp = await adminLibraryApi.listJobs('done', 20)
    const data = unwrap<{ jobs?: Job[] } | Job[]>(resp)
    const list: Job[] = Array.isArray(data) ? data : data?.jobs ?? []
    pendingLinkJobs.value = list.filter((j) => !j.shikimori_id)
  } catch (err) {
    console.warn('fetchPendingLinkJobs failed', err)
  }
}

// ---- Search ----
let searchDebounceTimer: ReturnType<typeof setTimeout> | null = null
function handleSearch() {
  if (searchDebounceTimer) clearTimeout(searchDebounceTimer)
  searchDebounceTimer = setTimeout(async () => {
    if (!searchQuery.value.trim() && !searchMalId.value) {
      searchResults.value = []
      return
    }
    searching.value = true
    try {
      const resp = await adminLibraryApi.search(searchQuery.value.trim(), searchMalId.value, 50)
      const data = unwrap<{ releases?: Release[] } | Release[]>(resp)
      searchResults.value = Array.isArray(data) ? data : data?.releases ?? []
    } catch (err) {
      console.warn('search failed', err)
      errorBanner.value = 'Search failed: ' + ((err as { message?: string })?.message ?? 'unknown')
      searchResults.value = []
    } finally {
      searching.value = false
    }
  }, 300)
}

// ---- Job actions ----
async function queueJob(release: Release) {
  const payload: CreateJobPayload = {
    magnet: release.magnet,
    title: release.title,
    source: release.source,
    uploader: release.uploader,
    quality: release.quality,
    size_bytes: release.size_bytes,
  }
  try {
    const resp = await adminLibraryApi.createJob(payload)
    const created = unwrap<Job>(resp)
    if (created) {
      // Optimistic prepend so the operator sees the new job in <100ms.
      activeJobs.value = [created, ...activeJobs.value]
    }
  } catch (err) {
    const status = (err as { response?: { status?: number } })?.response?.status
    if (status === 507) {
      errorBanner.value = 'Cannot queue: disk full'
    } else {
      errorBanner.value = 'Queue failed: ' + ((err as { message?: string })?.message ?? 'unknown')
    }
  }
}

async function cancelJob(job: Job) {
  const needsConfirm = ['downloading', 'encoding', 'uploading'].includes(job.status)
  if (needsConfirm && !(await confirm({
    title: 'Cancel job?',
    description: `Cancel: ${job.title}?`,
    confirmText: 'Cancel job',
    cancelText: 'Keep',
    variant: 'destructive',
  }))) {
    return
  }
  try {
    await adminLibraryApi.cancelJob(job.id)
    activeJobs.value = activeJobs.value.filter((j) => j.id !== job.id)
  } catch (err) {
    console.warn('cancel failed', err)
  }
}

async function retryJob(job: Job) {
  try {
    const resp = await adminLibraryApi.retryJob(job.id)
    const fresh = unwrap<Job>(resp)
    if (fresh) {
      // Optimistic add to active list; remove from failed.
      activeJobs.value = [fresh, ...activeJobs.value]
      failedJobs.value = failedJobs.value.filter((j) => j.id !== job.id)
    }
  } catch (err) {
    console.warn('retry failed', err)
  }
}

function onPendingLinkInput(jobID: string) {
  if (pendingLinkDebounces.has(jobID)) {
    clearTimeout(pendingLinkDebounces.get(jobID)!)
  }
  const t = setTimeout(async () => {
    const q = pendingLinkSearchQueries.value[jobID]?.trim() ?? ''
    if (!q) {
      pendingLinkResults.value[jobID] = []
      return
    }
    try {
      const resp = await animeApi.search(q, undefined, 5)
      const data = (resp.data?.data ?? resp.data) as AnimeSearchResult[] | undefined
      pendingLinkResults.value[jobID] = Array.isArray(data) ? data : []
    } catch (err) {
      console.warn('pendingLink search failed', err)
      pendingLinkResults.value[jobID] = []
    }
  }, 300)
  pendingLinkDebounces.set(jobID, t)
}

async function linkJob(job: Job, anime: AnimeSearchResult) {
  if (!anime.shikimori_id) {
    errorBanner.value = 'Selected anime has no shikimori_id'
    return
  }
  try {
    await adminLibraryApi.linkJob(job.id, anime.shikimori_id)
    pendingLinkJobs.value = pendingLinkJobs.value.filter((j) => j.id !== job.id)
    delete pendingLinkSearchQueries.value[job.id]
    delete pendingLinkResults.value[job.id]
  } catch (err) {
    console.warn('link failed', err)
    errorBanner.value = 'Link failed: ' + ((err as { message?: string })?.message ?? 'unknown')
  }
}

// ---- Lifecycle ----
onMounted(() => {
  void fetchHealth()
  void fetchActiveJobs()
  void fetchFailedJobs()
  void fetchPendingLinkJobs()

  healthInterval = setInterval(fetchHealth, 30000)
  activeJobsInterval = setInterval(fetchActiveJobs, 5000)
  failedPendingInterval = setInterval(() => {
    void fetchFailedJobs()
    void fetchPendingLinkJobs()
  }, 30000)
})

onUnmounted(() => {
  if (healthInterval) clearInterval(healthInterval)
  if (activeJobsInterval) clearInterval(activeJobsInterval)
  if (failedPendingInterval) clearInterval(failedPendingInterval)
  if (searchDebounceTimer) clearTimeout(searchDebounceTimer)
  pendingLinkDebounces.forEach((t) => clearTimeout(t))
  pendingLinkDebounces.clear()
})
</script>
