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
  return (VALID_BACKENDS as readonly string[]).includes(b ?? '') ? (b as FileDomain) : 'minio'
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
