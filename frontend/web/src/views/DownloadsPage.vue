<template>
  <div class="container mx-auto p-4 md:p-6 lg:p-8">
    <div class="flex items-center justify-between mb-6">
      <h1 class="text-xl font-semibold">{{ t('downloads.title') }}</h1>
      <p v-if="store.storage" class="text-sm text-muted-foreground">
        {{ t('downloads.storage', { used: fmtBytes(store.storage.usage), total: fmtBytes(store.storage.quota) }) }}
      </p>
    </div>

    <section v-if="playing" class="mb-8">
      <AePlayer
        :key="playing.animeId"
        :anime-id="playing.animeId"
        :anime="{ title: playing.title, eps: playing.downloads.length }"
        :theater="false"
        :offline="playing"
        :initial-episode="playingEpisode"
      />
      <Button variant="ghost" size="sm" class="mt-2" @click="playing = null">
        {{ t('common.close') }}
      </Button>
    </section>

    <p v-if="!store.loading && store.entries.length === 0" class="text-muted-foreground">
      {{ t('downloads.empty') }}
    </p>

    <div class="grid gap-4">
      <Card v-for="[animeId, group] in store.byAnime" :key="animeId" class="p-4 md:p-6">
        <div class="flex items-start gap-4">
          <img
            v-if="group[0].posterPath"
            :src="group[0].posterPath"
            :alt="group[0].animeTitle"
            class="w-16 rounded-md object-cover"
            loading="lazy"
          >
          <div class="flex-1 min-w-0">
            <h2 class="font-semibold truncate">{{ group[0].animeTitle }}</h2>
            <ul class="mt-2 grid gap-2">
              <li v-for="d in group" :key="d.id" class="flex items-center gap-3 text-sm">
                <span class="text-muted-foreground">{{ t('downloads.episode', { n: d.episode.number }) }}</span>
                <Badge v-if="d.state === 'done'" variant="success">{{ t('downloads.offlineReady') }}</Badge>
                <!-- queued ≠ downloading: the engine is strictly serial, so most
                     of a season batch waits in line — say so. An active download
                     whose playlist is still being planned has no resource count
                     yet: show "preparing", not a meaningless 0/0. -->
                <Badge v-else-if="d.state === 'queued'" variant="secondary">
                  {{ t('downloads.state.queued') }}
                </Badge>
                <Badge v-else-if="d.state === 'downloading'" variant="secondary">
                  {{ progressOf(d).total > 0 ? t('downloads.state.downloading', progressOf(d)) : t('downloads.state.preparing') }}
                </Badge>
                <Badge v-else-if="d.state === 'error'" variant="destructive">
                  {{ t(`downloads.error.${d.error ?? 'network'}`) }}
                </Badge>
                <Badge v-else variant="secondary">{{ t(`downloads.state.${d.state}`) }}</Badge>
                <span class="text-muted-foreground">{{ sizeLabel(d) }}</span>
                <span class="ml-auto flex items-center gap-2">
                  <Button
                    v-if="d.state === 'downloading' || d.state === 'queued'"
                    variant="ghost"
                    size="sm"
                    @click="store.pause(d.id)"
                  >
                    {{ t('downloads.pause') }}
                  </Button>
                  <Button
                    v-else-if="d.state === 'paused' || (d.state === 'error' && (d.error === 'network' || d.error === 'resolve'))"
                    variant="ghost"
                    size="sm"
                    @click="store.resume(d)"
                  >
                    {{ t('downloads.resume') }}
                  </Button>
                  <Button
                    :data-testid="`del-${d.id}`"
                    variant="ghost"
                    size="sm"
                    :class="armed === d.id ? 'text-destructive' : 'text-muted-foreground'"
                    @click="onDelete(d.id)"
                  >
                    {{ armed === d.id ? t('downloads.confirmDelete') : t('downloads.delete') }}
                  </Button>
                </span>
              </li>
            </ul>
            <Button
              v-if="group.some((d) => d.state === 'done')"
              :data-testid="`watch-${animeId}`"
              class="mt-3"
              size="sm"
              :disabled="!swReady"
              :title="!swReady ? t('downloads.noSw') : undefined"
              @click="play(animeId, group)"
            >
              {{ t('downloads.watch') }}
            </Button>
          </div>
        </div>
      </Card>
    </div>
  </div>
</template>

<script setup lang="ts">
import { defineAsyncComponent, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
// The ui dir is FLAT with one barrel (src/components/ui/index.ts) — there are
// no per-component subdirs, so '@/components/ui/button' does not resolve.
import { Button, Card, Badge } from '@/components/ui'
import { useDownloadsStore } from '@/stores/downloads'
import type { OfflineDownload } from '@/offline/types'
import type { OfflinePlayback } from '@/offline/offlineAdapter'
import { offlineRuntimeReady } from '@/offline/flag'
import { projectedBytesFor } from '@/offline/downloadEngine'

const AePlayer = defineAsyncComponent(() => import('@/components/player/aePlayer/AePlayer.vue'))

const { t } = useI18n()
const store = useDownloadsStore()
const playing = ref<OfflinePlayback | null>(null)
const playingEpisode = ref<number | undefined>(undefined)
const armed = ref<string | null>(null)
const swReady = ref(true)

onMounted(() => {
  swReady.value = offlineRuntimeReady()
  void store.refresh()
})

function progressOf(d: OfflineDownload): { done: number; total: number } {
  return store.progress[d.id] ?? { done: d.resourcesDone, total: d.resourcesTotal }
}

function fmtBytes(n: number): string {
  if (n >= 1 << 30) return `${(n / (1 << 30)).toFixed(1)} GB`
  if (n >= 1 << 20) return `${(n / (1 << 20)).toFixed(0)} MB`
  return `${Math.max(1, Math.round(n / 1024))} KB`
}

/** "X из ~Y" while in flight (Y = duration-scaled projection stamped at
 *  enqueue; legacy records fall back to the 24-min baseline), plain actual
 *  size once done. Nothing yet downloaded → just the estimate. */
function sizeLabel(d: OfflineDownload): string {
  if (d.state === 'done' || d.bytes >= (d.projectedBytes ?? Infinity)) return fmtBytes(d.bytes)
  const projected = d.projectedBytes ?? projectedBytesFor(d.quality)
  if (d.bytes <= 0) return `~${fmtBytes(projected)}`
  return `${fmtBytes(d.bytes)} / ~${fmtBytes(projected)}`
}

function play(animeId: string, group: OfflineDownload[]) {
  playing.value = { animeId, title: group[0].animeTitle, downloads: group }
  playingEpisode.value = group.find((d) => d.state === 'done')?.episode.number
}

function onDelete(id: string) {
  if (armed.value !== id) {
    armed.value = id
    setTimeout(() => { if (armed.value === id) armed.value = null }, 4000)
    return
  }
  armed.value = null
  void store.remove(id)
}
</script>
