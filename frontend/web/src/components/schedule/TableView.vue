<!-- frontend/web/src/components/schedule/TableView.vue -->
<template>
  <table class="w-full border-collapse">
    <thead>
      <tr>
        <th class="text-left text-[11px] uppercase tracking-wide text-muted-foreground p-3 border-b border-white/10 cursor-pointer w-[42%]" data-sort="name" @click="$emit('sort', 'name')">
          {{ $t('schedule.col.anime') }}<span v-if="sortKey==='name'" class="text-primary text-[10px]">{{ arrow }}</span>
        </th>
        <th class="text-left text-[11px] uppercase tracking-wide text-muted-foreground p-3 border-b border-white/10 cursor-pointer whitespace-nowrap" data-sort="date" @click="$emit('sort', 'date')">
          {{ $t('schedule.col.datetime') }}<span v-if="sortKey==='date'" class="text-primary text-[10px]">{{ arrow }}</span>
        </th>
        <th class="text-left text-[11px] uppercase tracking-wide text-muted-foreground p-3 border-b border-white/10 cursor-pointer" data-sort="episode" @click="$emit('sort', 'episode')">
          {{ $t('schedule.col.episode') }}<span v-if="sortKey==='episode'" class="text-primary text-[10px]">{{ arrow }}</span>
        </th>
        <th class="hidden md:table-cell text-left text-[11px] uppercase tracking-wide text-muted-foreground p-3 border-b border-white/10 cursor-pointer" data-sort="score" @click="$emit('sort', 'score')">
          {{ $t('schedule.col.score') }}<span v-if="sortKey==='score'" class="text-primary text-[10px]">{{ arrow }}</span>
        </th>
        <th class="hidden md:table-cell p-3 border-b border-white/10"></th>
      </tr>
    </thead>
    <tbody>
      <template v-for="(r, i) in rows" :key="r.anime.id + ':' + r.episode">
        <tr v-if="sortKey==='date' && isNewDay(i)" class="group-row">
          <td colspan="5" class="bg-white/[0.02] text-[11px] uppercase tracking-wide text-muted-foreground px-3 py-1.5">{{ dayLabel(r.date) }}</td>
        </tr>
        <tr class="dt-row border-b border-white/5 hover:bg-white/[0.04] cursor-pointer" @click="go(r.anime.id)">
          <td class="p-3">
            <div class="flex items-center gap-2.5">
              <img :src="r.anime.poster_url || '/placeholder.svg'" :alt="titleOf(r)" class="w-[30px] h-10 rounded object-cover flex-none bg-muted" />
              <div>
                <div class="font-semibold text-sm">{{ titleOf(r) }}</div>
                <div class="text-[11px] text-muted-foreground">{{ r.anime.name }}</div>
              </div>
            </div>
          </td>
          <td class="p-3 whitespace-nowrap text-sm">
            <span class="text-muted-foreground text-[11px]">{{ dowShort(r.date) }}</span>
            {{ r.date.getDate() }} {{ monthGen(r.date) }} · <span class="text-primary tabular-nums">{{ time(r.date) }}</span>
          </td>
          <td class="p-3 tabular-nums">{{ r.episode }}<span v-if="(r.anime.episodes_count ?? 0) > 0" class="text-muted-foreground"> / {{ r.anime.episodes_count }}</span></td>
          <td class="hidden md:table-cell p-3 tabular-nums"><span class="text-warning">★</span> {{ (r.anime.score ?? 0).toFixed(1) }}</td>
          <td class="hidden md:table-cell p-3">
            <RouterLink :to="`/anime/${r.anime.id}`" class="inline-block" @click.stop>
              <Button variant="default" size="sm" tabindex="-1">{{ $t('schedule.watch') }}</Button>
            </RouterLink>
          </td>
        </tr>
      </template>
    </tbody>
  </table>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import Button from '@/components/ui/Button.vue'
import type { Occurrence, TableSortKey } from '@/composables/schedule/types'
import { getLocalizedTitle } from '@/utils/title'
import { formatAirTime, formatDayTitle } from '@/composables/schedule/format'
import { isSameDay } from '@/composables/schedule/calendarGrid'

const props = defineProps<{ rows: Occurrence[]; sortKey: TableSortKey; sortDir: 1 | -1 }>()
defineEmits<{ sort: [key: TableSortKey] }>()
const router = useRouter()
const { t } = useI18n()

const arrow = computed(() => (props.sortDir === 1 ? '▲' : '▼'))
const titleOf = (r: Occurrence) => getLocalizedTitle(r.anime.name, r.anime.name_ru, r.anime.name_jp)
const time = (d: Date) => formatAirTime(d)
const monthGen = (d: Date) => t(`schedule.monthsGenitive.${['jan','feb','mar','apr','may','jun','jul','aug','sep','oct','nov','dec'][d.getMonth()]}`)
const dowShort = (d: Date) => t(`schedule.daysShort.${['monday','tuesday','wednesday','thursday','friday','saturday','sunday'][(d.getDay()+6)%7]}`)
const dayLabel = (d: Date) => formatDayTitle(d, t)
const isNewDay = (i: number) => i === 0 || !isSameDay(props.rows[i].date, props.rows[i - 1].date)
const go = (id: string) => router.push(`/anime/${id}`)
</script>
