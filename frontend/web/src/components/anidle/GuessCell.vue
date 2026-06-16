<template>
  <div
    :class="[
      'flex items-center justify-center text-center rounded-lg px-1.5 text-xs font-medium leading-tight transition-colors w-[72px] h-[44px] overflow-hidden',
      statusClass,
    ]"
    :aria-label="ariaLabel"
  >
    <Tooltip v-if="full" :delay-duration="120" class="max-w-[260px] text-center">
      <template #trigger>
        <span class="line-clamp-2 break-words cursor-help">{{ displayValue }}</span>
      </template>
      {{ full }}
    </Tooltip>
    <span v-else class="line-clamp-2 break-words">{{ displayValue }}<span v-if="hint === 'higher'" aria-hidden="true"> ↑</span><span v-else-if="hint === 'lower'" aria-hidden="true"> ↓</span></span>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Tooltip from '@/components/ui/Tooltip.vue'

const props = defineProps<{
  status: 'correct' | 'partial' | 'wrong'
  value: string | number
  hint?: 'higher' | 'lower' | null
  /** Full, untruncated text shown as a native tooltip on hover (e.g. all genres). */
  full?: string
}>()

const { t } = useI18n()

const statusClass = computed(() => {
  if (props.status === 'correct') return 'bg-success text-success-foreground'
  if (props.status === 'partial') return 'bg-warning text-warning-foreground'
  return 'bg-muted text-muted-foreground'
})

const displayValue = computed(() => String(props.value))

const ariaLabel = computed(() => {
  if (props.hint === 'higher') return `${displayValue.value} ${t('anidle.hint_higher')}`
  if (props.hint === 'lower') return `${displayValue.value} ${t('anidle.hint_lower')}`
  return displayValue.value
})
</script>
