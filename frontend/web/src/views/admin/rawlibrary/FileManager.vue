<template>
  <div>
    <!-- Domain switch -->
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

    <div v-if="errorBanner" class="glass-card p-4 mb-4 border border-destructive/40">
      <p class="text-destructive">{{ errorBanner }}</p>
    </div>

    <!-- Breadcrumb -->
    <nav class="flex items-center flex-wrap gap-1 text-sm text-white/60 mb-2">
      <button type="button" class="hover:text-white transition" @click="goTo('')">
        {{ $t('player.adminLibrary.files.root') }}
      </button>
      <template v-for="(c, i) in fileBreadcrumb" :key="i">
        <span>/</span>
        <button type="button" class="hover:text-white transition" @click="crumbTo(i)">{{ c }}</button>
      </template>
    </nav>

    <div v-if="filesLoading" class="flex justify-center py-6">
      <Spinner size="md" />
    </div>
    <ul v-else class="glass-card divide-y divide-white/10 overflow-hidden">
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
      <li v-if="fileEntries.length === 0" class="p-4 text-sm text-white/60">
        {{ $t('player.adminLibrary.files.empty') }}
      </li>
      <li
        v-for="e in fileEntries"
        :key="e.name"
        class="flex items-center justify-between gap-2 p-3"
      >
        <button
          type="button"
          class="flex items-center gap-2 min-w-0 text-left"
          :class="e.kind === 'dir' ? 'text-white' : 'text-white/70'"
          @click="openEntry(e)"
        >
          <span class="truncate">
            {{ e.kind === 'dir' ? '📁' : '📄' }} {{ e.name }}<span v-if="titleFor(e.name)" class="text-white/50"> — {{ titleFor(e.name) }}</span>
          </span>
          <Badge v-if="e.episode" size="sm" :variant="freshnessVariant(e.episode.freshness)">
            {{ freshnessLabel(e.episode.freshness) }}
          </Badge>
        </button>
        <div class="flex items-center gap-2 flex-shrink-0" :aria-label="$t('player.adminLibrary.files.col.actions')">
          <span class="text-xs text-white/50 font-mono">{{ formatBytes(e.size) }}</span>
          <Button v-if="e.kind === 'file'" variant="ghost" size="xs" @click="downloadEntry(e)">
            {{ $t('player.adminLibrary.files.download') }}
          </Button>
          <Button variant="ghost" size="xs" @click="deleteEntry(e)">
            {{ $t('player.adminLibrary.files.delete') }}
          </Button>
        </div>
      </li>
    </ul>
  </div>
</template>

<script lang="ts">
import { reactive } from 'vue'

// Module-level Shikimori-id → display title cache, shared across FileManager
// remounts within the session. Value '' = resolved-but-untitled/404 (don't refetch).
// Declared in a plain (non-setup) <script> block so it lives at true ES-module
// scope — a `<script setup>` top-level const would be re-created every mount.
export const titleCache = reactive<Record<string, string>>({})

// Test-only helper: clear the module-scoped cache so specs don't bleed
// resolved titles across cases (the cache otherwise survives remounts).
export function resetTitleCache() {
  for (const k of Object.keys(titleCache)) delete titleCache[k]
}
</script>

<script setup lang="ts">
import { ref, computed, watch, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminLibraryApi, animeApi } from '@/api/client'
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

// Only aeProvider/<id> folders carry an anime id (RawPrefix layout); gate
// both the cache read and the resolver on it so an entry name that happens
// to collide with a cached numeric id elsewhere (e.g. the work domain, or a
// different prefix) doesn't pick up an unrelated title.
const isAeProviderRoot = computed(() => props.backend !== 'work' && props.prefix === 'aeProvider/')

function titleFor(name: string): string | undefined {
  if (!isAeProviderRoot.value) return undefined
  return titleCache[name] || undefined
}

async function resolveTranscripts() {
  if (!isAeProviderRoot.value) return
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
    void resolveTranscripts()
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
function parentPrefix(): string {
  const segs = props.prefix.split('/').filter(Boolean)
  segs.pop()
  return segs.length ? segs.join('/') + '/' : ''
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
</script>
