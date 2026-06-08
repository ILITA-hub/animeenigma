<!-- frontend/web/src/components/schedule/MonthView.vue -->
<template>
  <div>
    <div class="grid grid-cols-7 gap-1.5 mb-1.5">
      <div v-for="d in dows" :key="d" class="text-center text-[10px] uppercase tracking-wide text-muted-foreground">{{ d }}</div>
    </div>
    <div class="grid grid-cols-7 gap-1.5">
      <DayCell v-for="(c, i) in cells" :key="i" :cell="c" @open="$emit('open', $event)" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import DayCell from './DayCell.vue'
import type { DayCellModel } from '@/composables/useScheduleCalendar'

defineProps<{ cells: DayCellModel[] }>()
defineEmits<{ open: [date: Date] }>()
const { t } = useI18n()
const dows = computed(() => ['monday','tuesday','wednesday','thursday','friday','saturday','sunday'].map(k => t(`schedule.daysShort.${k}`)))
</script>
