<!-- frontend/web/src/components/schedule/WeekView.vue -->
<template>
  <div class="grid grid-cols-7 gap-1.5">
    <div
      v-for="(c, i) in columns"
      :key="i"
      class="relative min-h-[220px] rounded-xl border border-white/[0.06] bg-white/[0.025] p-2.5 overflow-hidden transition-colors"
      :class="[c.occurrences.length ? 'cursor-pointer hover:bg-white/[0.045] hover:border-white/12' : '', c.isToday ? 'today-bar' : '']"
      @click="c.occurrences.length && $emit('open', c.date)"
    >
      <div class="text-center pb-2 mb-2 border-b border-white/5">
        <div class="text-[10px] uppercase tracking-wide" :class="c.isToday ? 'text-primary' : 'text-muted-foreground'">{{ dows[i] }}</div>
        <div class="text-base font-semibold font-display" :class="c.isToday ? 'text-primary' : 'text-foreground'">{{ c.date.getDate() }}</div>
      </div>
      <div v-if="!c.occurrences.length" class="text-[10px] text-white/20 text-center pt-3">—</div>
      <EpisodeRow v-for="o in c.occurrences.slice(0, 4)" :key="o.anime.id + ':' + o.episode" :occurrence="o" size="lg" />
      <div v-if="c.occurrences.length > 4" class="text-[11px] text-muted-foreground mt-1.5 text-center">
        {{ $t('schedule.more', { n: c.occurrences.length - 4 }) }}
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import EpisodeRow from './EpisodeRow.vue'
import type { DayCellModel } from '@/composables/useScheduleCalendar'

defineProps<{ columns: DayCellModel[] }>()
defineEmits<{ open: [date: Date] }>()
const { t } = useI18n()
const dows = computed(() => ['monday','tuesday','wednesday','thursday','friday','saturday','sunday'].map(k => t(`schedule.daysShort.${k}`)))
</script>

<style scoped>
.today-bar::before {
  content: '';
  position: absolute; top: 0; left: 0; right: 0; height: 3px;
  background: linear-gradient(90deg, var(--brand-cyan), var(--brand-violet));
}
</style>
