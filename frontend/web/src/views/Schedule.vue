<!-- frontend/web/src/views/Schedule.vue -->
<template>
  <div class="min-h-screen bg-background pt-20">
    <div class="container mx-auto px-4 py-8">
      <div class="flex items-center justify-between gap-3 flex-wrap mb-5">
        <div class="flex items-center gap-3 flex-wrap">
          <h1 class="text-2xl font-bold text-foreground font-display min-w-[130px]">{{ headerTitle }}</h1>
          <div class="flex gap-1.5">
            <button class="h-8 w-8 rounded-lg bg-white/[0.06] hover:bg-white/12 flex items-center justify-center" @click="cal.shift(-1)">‹</button>
            <button class="h-8 w-8 rounded-lg bg-white/[0.06] hover:bg-white/12 flex items-center justify-center" @click="cal.shift(1)">›</button>
          </div>
          <button class="h-8 px-3 rounded-lg text-primary border border-primary/40 text-xs" @click="cal.goToday()">{{ $t('schedule.todayBtn') }}</button>
        </div>
        <div class="flex bg-white/[0.06] rounded-lg p-0.5">
          <button v-for="v in views" :key="v" class="text-xs px-3.5 py-1.5 rounded-md transition-colors"
            :class="cal.view.value === v ? 'bg-primary text-primary-foreground font-semibold' : 'text-muted-foreground'"
            @click="cal.setView(v)">{{ $t('schedule.view' + cap(v)) }}</button>
        </div>
      </div>

      <ScheduleFilters
        :filters="cal.filters"
        :genres="cal.genres.value"
        :logged-in="loggedIn"
        :match-count="cal.filteredAnimes.value.length"
        :total="schedule.length"
        @reset="cal.resetFilters()"
      />

      <div v-if="loading" class="flex justify-center py-12">
        <Spinner size="lg" />
      </div>

      <template v-else>
        <MonthView v-if="cal.view.value === 'month'" :cells="cal.monthCells.value" @open="openDay" />
        <WeekView v-else-if="cal.view.value === 'week'" :columns="cal.weekColumns.value" @open="openDay" />
        <TableView v-else :rows="cal.tableRows.value" :sort-key="cal.sortKey.value" :sort-dir="cal.sortDir.value" @sort="cal.setSort($event)" />

        <div v-if="isEmpty" class="text-center py-12 text-muted-foreground">{{ $t('schedule.empty') }}</div>
      </template>
    </div>

    <DayModal v-model="modalOpen" :date="modalDate" :occurrences="modalOccurrences" />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useAnime } from '@/composables/useAnime'
import { useWatchlistStore } from '@/stores/watchlist'
import { useAuthStore } from '@/stores/auth'
import { useScheduleCalendar, type ScheduleView } from '@/composables/useScheduleCalendar'
import type { ScheduleAnime, Occurrence } from '@/composables/schedule/types'
import { isSameDay } from '@/composables/schedule/calendarGrid'
import ScheduleFilters from '@/components/schedule/ScheduleFilters.vue'
import MonthView from '@/components/schedule/MonthView.vue'
import WeekView from '@/components/schedule/WeekView.vue'
import TableView from '@/components/schedule/TableView.vue'
import DayModal from '@/components/schedule/DayModal.vue'
import { Spinner } from '@/components/ui'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const { fetchSchedule, loading } = useAnime()
const watchlist = useWatchlistStore()
const auth = useAuthStore()

const schedule = ref<ScheduleAnime[]>([])
const now = ref(new Date())
const loggedIn = computed(() => auth.isAuthenticated)

const cal = useScheduleCalendar({
  animes: computed(() => schedule.value),
  now,
  statusOf: (id: string) => watchlist.getStatus(id),
  loggedIn,
})

const views: ScheduleView[] = ['month', 'week', 'table']
const cap = (s: string) => s.charAt(0).toUpperCase() + s.slice(1)
const headerTitle = computed(() => formatHeader())

const isEmpty = computed(() => {
  if (cal.view.value === 'table') return cal.tableRows.value.length === 0
  if (cal.view.value === 'week') return cal.weekColumns.value.every(c => c.occurrences.length === 0)
  return cal.monthCells.value.every(c => c.occurrences.length === 0)
})

function formatHeader(): string {
  const months = ['jan','feb','mar','apr','may','jun','jul','aug','sep','oct','nov','dec']
  const d = cal.viewDate.value
  if (cal.view.value === 'week' || cal.view.value === 'table') {
    const off = (d.getDay() + 6) % 7
    const s = new Date(d.getFullYear(), d.getMonth(), d.getDate() - off)
    const e = new Date(s.getFullYear(), s.getMonth(), s.getDate() + 6)
    return `${s.getDate()}–${e.getDate()} ${t('schedule.monthsGenitive.' + months[e.getMonth()])}`
  }
  return `${t('schedule.monthsNominative.' + months[d.getMonth()])} ${d.getFullYear()}`
}

const modalOpen = ref(false)
const modalDate = ref<Date | null>(null)
function openDay(date: Date) {
  modalDate.value = date
  modalOpen.value = true
}
const modalOccurrences = computed<Occurrence[]>(() => {
  if (!modalDate.value) return []
  const src = cal.view.value === 'week' ? cal.weekColumns.value : cal.monthCells.value
  const cell = src.find(c => isSameDay(c.date, modalDate.value as Date))
  return cell ? cell.occurrences : []
})

function readQuery() {
  const v = route.query.view as string | undefined
  if (v === 'month' || v === 'week' || v === 'table') cal.setView(v)
  const dstr = route.query.date as string | undefined
  if (dstr) {
    const d = new Date(dstr)
    if (!Number.isNaN(d.getTime())) cal.viewDate.value = d
  }
}
watch([cal.view, cal.viewDate], () => {
  const d = cal.viewDate.value
  const dstr = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
  router.replace({ query: { ...route.query, view: cal.view.value, date: dstr } })
})

onMounted(async () => {
  readQuery()
  if (loggedIn.value) watchlist.fetchStatuses().catch(() => {})
  try {
    schedule.value = await fetchSchedule()
  } catch (err) {
    console.error('Failed to fetch schedule:', err)
  }
})
</script>
