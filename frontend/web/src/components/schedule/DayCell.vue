<!-- frontend/web/src/components/schedule/DayCell.vue -->
<template>
  <div
    class="relative min-h-[150px] rounded-xl border border-white/[0.06] bg-white/[0.025] p-2.5 overflow-hidden transition-colors"
    :class="[
      cell.inCurrentMonth ? (hasEpisodes ? 'cursor-pointer hover:bg-white/[0.045] hover:border-white/12' : '') : 'bg-transparent border-white/[0.03]',
      cell.isToday ? 'today-bar' : '',
    ]"
    @click="onClick"
  >
    <div
      class="text-xs mb-1.5 font-display"
      :class="cell.isToday ? 'text-primary font-bold' : cell.inCurrentMonth ? 'text-muted-foreground' : 'text-white/20'"
    >
      {{ cell.date.getDate() }}
    </div>
    <EpisodeRow v-for="o in visible" :key="o.anime.id + ':' + o.episode" :occurrence="o" />
    <div v-if="overflow > 0" class="text-[10px] text-muted-foreground mt-1.5">
      {{ $t('schedule.more', { n: overflow }) }}
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import EpisodeRow from './EpisodeRow.vue'
import type { DayCellModel } from '@/composables/useScheduleCalendar'

const props = defineProps<{ cell: DayCellModel }>()
const emit = defineEmits<{ open: [date: Date] }>()

const hasEpisodes = computed(() => props.cell.inCurrentMonth && props.cell.occurrences.length > 0)
const visible = computed(() => props.cell.occurrences.slice(0, 3))
const overflow = computed(() => Math.max(0, props.cell.occurrences.length - 3))

function onClick() {
  if (hasEpisodes.value) emit('open', props.cell.date)
}
</script>

<style scoped>
.today-bar::before {
  content: '';
  position: absolute;
  top: 0; left: 0; right: 0;
  height: 3px;
  background: linear-gradient(90deg, var(--brand-cyan), var(--brand-violet));
}
</style>
