<!--
  LibraryGrid — grid of the current user's generated fanfics (fanficApi.list()).

  Presentational-ish: owns its own fetch + delete-confirm flow (useConfirm),
  but stays dumb about navigation — `open(id)` just tells the parent
  (FanficsView.vue) to fetch the full Fanfic and show it in a reader dialog.
  `refresh()` is exposed so the parent can re-pull the list after a
  generation completes.
-->
<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { Trash2 } from 'lucide-vue-next'
import { fanficApi } from '@/api/fanfic'
import { useConfirm } from '@/composables/useConfirm'
import PosterImage from '@/components/anime/PosterImage.vue'
import { Card, Badge, Button, EmptyState, LoadingState, Alert } from '@/components/ui'
import type { Fanfic, FanficRating } from '@/types/fanfic'

const emit = defineEmits<{ open: [id: string]; remove: [id: string] }>()

const { t, locale } = useI18n()
const { confirm } = useConfirm()

const items = ref<Fanfic[]>([])
const loading = ref(false)
const error = ref('')

async function load(): Promise<void> {
  loading.value = true
  error.value = ''
  try {
    const res = await fanficApi.list(1, 50)
    items.value = res.items
  } catch {
    error.value = t('fanfic.library.loadError')
  } finally {
    loading.value = false
  }
}

onMounted(load)

async function onDelete(f: Fanfic): Promise<void> {
  const ok = await confirm({
    title: t('fanfic.library.delete'),
    description: t('fanfic.library.deleteConfirm'),
    confirmText: t('common.delete'),
    cancelText: t('common.cancel'),
    variant: 'destructive',
  })
  if (!ok) return
  await fanficApi.remove(f.id)
  items.value = items.value.filter((x) => x.id !== f.id)
  emit('remove', f.id)
}

function ratingVariant(rating: FanficRating): 'default' | 'warning' | 'destructive' {
  if (rating === 'explicit') return 'destructive'
  if (rating === 'mature') return 'warning'
  return 'default'
}

function formattedDate(iso: string): string {
  try {
    return new Date(iso).toLocaleDateString(locale.value === 'en' ? 'en-US' : 'ru-RU')
  } catch {
    return iso
  }
}

defineExpose({ refresh: load, items })
</script>

<template>
  <div>
    <LoadingState v-if="loading && items.length === 0" :label="t('common.loading')" />
    <Alert v-else-if="error" variant="destructive">{{ error }}</Alert>
    <EmptyState v-else-if="items.length === 0" :description="t('fanfic.library.empty')" />
    <div v-else class="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-4 gap-4">
      <Card v-for="f in items" :key="f.id" variant="interactive" padding="none" class="overflow-hidden">
        <div class="cursor-pointer" @click="emit('open', f.id)">
          <PosterImage :src="f.anime_poster || '/placeholder.svg'" :alt="f.anime_title" ratio="2/3" :proxy-width="256" />
          <div class="p-3 pb-0">
            <h3 class="font-medium text-foreground line-clamp-2 mb-1 text-sm">{{ f.title || f.anime_title }}</h3>
            <p class="text-xs text-muted-foreground truncate mb-2">{{ f.anime_title }}</p>
            <div class="flex flex-wrap items-center gap-1 mb-2">
              <Badge :variant="ratingVariant(f.rating)" size="sm">{{ t(`fanfic.rating.${f.rating}`) }}</Badge>
              <Badge v-for="tag in f.tags.slice(0, 2)" :key="tag" size="sm">{{ tag }}</Badge>
            </div>
            <p v-if="f.status === 'failed'" class="text-xs text-destructive">{{ t('fanfic.status.failed') }}</p>
            <p v-else-if="f.status === 'generating'" class="text-xs text-muted-foreground">{{ t('fanfic.status.generating') }}</p>
          </div>
        </div>
        <div class="flex items-center justify-between px-3 pb-3 pt-1">
          <span class="text-xs text-muted-foreground">{{ formattedDate(f.created_at) }}</span>
          <Button variant="ghost" size="sm" :aria-label="t('fanfic.library.delete')" @click="onDelete(f)">
            <Trash2 class="size-4" aria-hidden="true" />
          </Button>
        </div>
      </Card>
    </div>
  </div>
</template>
