<template>
  <div class="mx-auto min-h-screen max-w-7xl px-4 pb-16 pt-24">
    <header class="mb-8 max-w-3xl">
      <p class="mb-2 text-sm font-medium uppercase tracking-wider text-brand-cyan">
        {{ t('recs.pageEyebrow') }}
      </p>
      <h1 class="text-3xl font-semibold text-foreground md:text-4xl">
        {{ t('recs.pageTitle') }}
      </h1>
      <p class="mt-3 text-muted-foreground">
        {{ t('recs.pageSubtitle') }}
      </p>
    </header>

    <div v-if="isLoading" class="flex justify-center py-20">
      <Spinner size="lg" />
    </div>

    <EmptyState
      v-else-if="error"
      :title="t('recs.loadErrorTitle')"
      :description="t('recs.loadErrorDescription')"
      class="glass-card"
    >
      <template #icon><Sparkles class="size-12" /></template>
      <template #action>
        <Button variant="outline" @click="refresh">{{ t('common.retry') }}</Button>
      </template>
    </EmptyState>

    <EmptyState
      v-else-if="recs.length === 0"
      :title="t('recs.empty')"
      :description="t('recs.emptyDescription')"
      class="glass-card"
    >
      <template #icon><Sparkles class="size-12" /></template>
      <template #action>
        <router-link
          to="/browse"
          :class="buttonVariants({ variant: 'outline', size: 'md' })"
        >
          {{ t('recs.browseCta') }}
        </router-link>
      </template>
    </EmptyState>

    <section v-else :aria-label="t('recs.pageTitle')">
      <div class="grid grid-cols-2 gap-4 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6">
        <article
          v-for="item in recs"
          :key="item.anime.id"
          class="min-w-0"
          @click="onCardClick($event, item)"
        >
          <PosterCard :model="fromHomeAnime(item.anime)" />
          <div class="mt-2 flex min-h-6 items-center gap-2 px-1">
            <Badge variant="default" size="sm">#{{ item.rank }}</Badge>
            <p v-if="recommendationReason(item)" class="truncate text-xs text-muted-foreground">
              {{ recommendationReason(item) }}
            </p>
          </div>
        </article>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { Sparkles } from 'lucide-vue-next'
import { useI18n } from 'vue-i18n'
import { PosterCard } from '@/components/anime'
import { Badge, Button, EmptyState, Spinner } from '@/components/ui'
import { buttonVariants } from '@/components/ui/button-variants'
import { useRecs, type RecItem } from '@/composables/useRecs'
import { fromHomeAnime } from '@/utils/toCardModel'
import { emitRecClick } from '@/utils/recsAnalytics'

const { t } = useI18n()
const { recs, isLoading, error, refresh } = useRecs()

function recommendationReason(item: RecItem): string {
  if (item.pin_reason_key) {
    return t(item.pin_reason_key, (item.pin_reason_data ?? {}) as Record<string, unknown>)
  }
  if (item.pin_reason) return item.pin_reason
  if (/^s[1-5]$/.test(item.top_contributor ?? '')) {
    return t(`recs.reason.${item.top_contributor}`)
  }
  return ''
}

function onCardClick(event: MouseEvent, item: RecItem): void {
  const target = event.target instanceof Element ? event.target.closest('a') : null
  if (!target?.getAttribute('href')?.startsWith('/anime/')) return
  void emitRecClick({
    event_type: 'rec_click',
    anime_id: String(item.anime.id),
    signal_id: item.pinned ? 's6_pin' : item.top_contributor || '',
    pinned: item.pinned,
    pin_source: item.pin_source,
    pin_seed_anime_id: item.pin_seed_anime_id,
    source_route: '/recs',
    rank: item.rank,
  })
}
</script>
