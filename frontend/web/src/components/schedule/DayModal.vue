<!-- frontend/web/src/components/schedule/DayModal.vue -->
<template>
  <Modal :model-value="modelValue" :title="headerTitle" size="md" :modal="false" @update:model-value="$emit('update:modelValue', $event)">
    <p class="text-xs text-primary -mt-2 mb-3">{{ subtitle }}</p>
    <div class="space-y-2.5">
      <router-link
        v-for="o in sorted"
        :key="o.anime.id + ':' + o.episode"
        :to="`/anime/${o.anime.id}`"
        class="flex items-center gap-3 rounded-xl border border-white/[0.06] bg-white/[0.045] p-2.5 hover:bg-white/[0.08] transition-colors"
      >
        <img :src="o.anime.poster_url || '/placeholder.svg'" :alt="titleOf(o)" class="w-[54px] h-[76px] rounded-lg object-cover flex-none bg-muted" />
        <div class="min-w-0 flex-1">
          <div class="text-sm font-semibold text-foreground line-clamp-2">{{ titleOf(o) }}</div>
          <div class="text-xs text-primary mt-1">{{ $t('schedule.episode', { n: o.episode }) }}</div>
          <div class="text-[11px] text-muted-foreground mt-0.5 flex items-center gap-1">
            <span>{{ formatAirTime(o.date) }} {{ $t('schedule.mskSuffix') }} ·</span>
            <Star class="size-2.5 text-warning" fill="currentColor" aria-hidden="true" />
            <span>{{ (o.anime.score ?? 0).toFixed(1) }}</span>
          </div>
        </div>
        <Button variant="default" size="sm" class="ml-auto flex-none" tabindex="-1">{{ $t('schedule.watch') }}</Button>
      </router-link>
    </div>
  </Modal>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Star } from 'lucide-vue-next'
import Modal from '@/components/ui/Modal.vue'
import Button from '@/components/ui/Button.vue'
import type { Occurrence } from '@/composables/schedule/types'
import { getLocalizedTitle } from '@/utils/title'
import { formatAirTime, formatDayTitle } from '@/composables/schedule/format'
import { sortByTime } from '@/composables/schedule/filterSort'

const props = defineProps<{ modelValue: boolean; date: Date | null; occurrences: Occurrence[] }>()
defineEmits<{ 'update:modelValue': [value: boolean] }>()
const { t } = useI18n()

const sorted = computed(() => sortByTime(props.occurrences))
const headerTitle = computed(() => (props.date ? formatDayTitle(props.date, t) : ''))
// vue-i18n pluralization: the count selects the branch and is auto-exposed as {n}
const subtitle = computed(() => t('schedule.episodeCountPlural', props.occurrences.length))
const titleOf = (o: Occurrence) => getLocalizedTitle(o.anime.name, o.anime.name_ru, o.anime.name_jp)
</script>
