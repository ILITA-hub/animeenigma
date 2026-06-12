<template>
  <PopoverRoot v-model:open="open">
    <PopoverTrigger as-child>
      <button
        type="button"
        data-testid="datepicker-trigger"
        :title="title"
        :class="cn(
          'inline-flex items-center gap-1.5 h-8 px-2.5 rounded-lg bg-white/5 border border-white/10 text-xs transition-colors',
          'hover:bg-white/10 hover:border-white/20 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-cyan-500/50',
          modelValue ? 'text-white' : 'text-white/40',
          props.class,
        )"
      >
        <CalendarDays class="size-3.5 shrink-0 text-white/40" aria-hidden="true" />
        <span class="truncate tabular-nums">{{ displayValue || placeholder }}</span>
      </button>
    </PopoverTrigger>

    <PopoverPortal>
      <PopoverContent
        side="bottom"
        align="start"
        :side-offset="6"
        class="z-[9999] bg-popover text-popover-foreground border border-white/10 rounded-xl p-3 shadow-xl"
      >
        <CalendarRoot
          v-slot="{ weekDays, grid }"
          :model-value="calendarValue"
          :locale="calendarLocale"
          :week-starts-on="1"
          weekday-format="short"
          fixed-weeks
          class="select-none"
          @update:model-value="onSelect"
        >
          <CalendarHeader class="flex items-center justify-between gap-2 mb-2">
            <CalendarPrev
              class="inline-flex items-center justify-center w-7 h-7 rounded-lg text-white/60 hover:text-white hover:bg-white/10 transition-colors cursor-pointer"
            >
              <ChevronLeft class="size-4" />
            </CalendarPrev>
            <CalendarHeading class="text-sm font-semibold text-white capitalize" />
            <CalendarNext
              class="inline-flex items-center justify-center w-7 h-7 rounded-lg text-white/60 hover:text-white hover:bg-white/10 transition-colors cursor-pointer"
            >
              <ChevronRight class="size-4" />
            </CalendarNext>
          </CalendarHeader>

          <CalendarGrid v-for="month in grid" :key="month.value.toString()" class="border-collapse">
            <CalendarGridHead>
              <CalendarGridRow class="grid grid-cols-7">
                <CalendarHeadCell
                  v-for="day in weekDays"
                  :key="day"
                  class="w-8 h-7 text-[10px] font-medium uppercase text-white/40 flex items-center justify-center"
                >
                  {{ day }}
                </CalendarHeadCell>
              </CalendarGridRow>
            </CalendarGridHead>
            <CalendarGridBody>
              <CalendarGridRow
                v-for="(weekDates, i) in month.rows"
                :key="`week-${i}`"
                class="grid grid-cols-7"
              >
                <CalendarCell
                  v-for="weekDate in weekDates"
                  :key="weekDate.toString()"
                  :date="weekDate"
                  class="p-0"
                >
                  <CalendarCellTrigger
                    :day="weekDate"
                    :month="month.value"
                    class="w-8 h-8 rounded-lg text-xs tabular-nums flex items-center justify-center cursor-pointer transition-colors
                           text-white/80 hover:bg-white/10
                           data-[outside-view]:text-white/25
                           data-[today]:font-semibold data-[today]:text-cyan-400
                           data-[selected]:bg-cyan-500/25 data-[selected]:text-cyan-300 data-[selected]:font-semibold
                           data-[disabled]:opacity-30 data-[disabled]:cursor-not-allowed
                           focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-cyan-500/50"
                  />
                </CalendarCell>
              </CalendarGridRow>
            </CalendarGridBody>
          </CalendarGrid>
        </CalendarRoot>

        <div class="flex items-center justify-between gap-2 mt-2 pt-2 border-t border-white/10">
          <button
            type="button"
            data-testid="datepicker-today"
            class="px-2 py-1 rounded-lg text-xs text-cyan-400 hover:bg-cyan-500/10 transition-colors"
            @click="selectToday"
          >
            {{ t('datePicker.today') }}
          </button>
          <button
            type="button"
            data-testid="datepicker-clear"
            class="px-2 py-1 rounded-lg text-xs text-white/50 hover:text-white hover:bg-white/10 transition-colors"
            :disabled="!modelValue"
            :class="!modelValue ? 'opacity-40 cursor-not-allowed' : ''"
            @click="clear"
          >
            {{ t('datePicker.clear') }}
          </button>
        </div>
      </PopoverContent>
    </PopoverPortal>
  </PopoverRoot>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { CalendarDays, ChevronLeft, ChevronRight } from 'lucide-vue-next'
import {
  PopoverRoot,
  PopoverTrigger,
  PopoverPortal,
  PopoverContent,
  CalendarRoot,
  CalendarHeader,
  CalendarHeading,
  CalendarPrev,
  CalendarNext,
  CalendarGrid,
  CalendarGridHead,
  CalendarGridBody,
  CalendarGridRow,
  CalendarHeadCell,
  CalendarCell,
  CalendarCellTrigger,
} from 'reka-ui'
import { parseDate, today, getLocalTimeZone, type DateValue } from '@internationalized/date'
import type { HTMLAttributes } from 'vue'
import { cn } from '@/lib/utils'

const props = defineProps<{
  /** ISO date string 'YYYY-MM-DD' or '' / undefined when unset. */
  modelValue?: string
  placeholder?: string
  title?: string
  class?: HTMLAttributes['class']
}>()

const emit = defineEmits<{
  /** '' means cleared. */
  'update:modelValue': [value: string]
}>()

const { t, locale } = useI18n()

const open = ref(false)

const localeMap: Record<string, string> = { ru: 'ru-RU', en: 'en-US', ja: 'ja-JP' }
const calendarLocale = computed(() => localeMap[locale.value] || 'en-US')

const calendarValue = computed<DateValue | undefined>(() => {
  if (!props.modelValue) return undefined
  try {
    return parseDate(props.modelValue)
  } catch {
    return undefined
  }
})

const displayValue = computed(() => {
  if (!props.modelValue) return ''
  try {
    const [y, m, d] = props.modelValue.split('-').map(Number)
    return new Date(y, m - 1, d).toLocaleDateString(calendarLocale.value, {
      day: '2-digit',
      month: '2-digit',
      year: 'numeric',
    })
  } catch {
    return props.modelValue
  }
})

function onSelect(value: DateValue | undefined) {
  if (!value) return
  emit('update:modelValue', value.toString())
  open.value = false
}

function selectToday() {
  onSelect(today(getLocalTimeZone()))
}

function clear() {
  if (!props.modelValue) return
  emit('update:modelValue', '')
  open.value = false
}
</script>
