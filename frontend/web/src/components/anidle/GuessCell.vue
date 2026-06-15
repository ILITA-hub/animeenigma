<template>
  <div
    :class="[
      'rounded-lg p-2 text-sm font-medium text-center transition-colors min-w-[64px]',
      statusClass,
    ]"
    :aria-label="ariaLabel"
  >
    {{ displayValue }}
    <span v-if="hint === 'higher'" aria-hidden="true"> ↑</span>
    <span v-else-if="hint === 'lower'" aria-hidden="true"> ↓</span>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

const props = defineProps<{
  status: 'correct' | 'partial' | 'wrong'
  value: string | number
  hint?: 'higher' | 'lower' | null
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
