<template>
  <CheckboxRoot
    v-model="model"
    :disabled="disabled"
    :class="rootClasses"
  >
    <CheckboxIndicator class="flex items-center justify-center text-primary-foreground">
      <!-- Inline check icon (no lucide dependency in this project — same
           inline-SVG approach as Select/Modal). Reka renders the indicator only
           when checked / indeterminate. -->
      <svg
        v-if="model === 'indeterminate'"
        class="h-3.5 w-3.5"
        fill="none"
        stroke="currentColor"
        viewBox="0 0 24 24"
        aria-hidden="true"
      >
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="3" d="M5 12h14" />
      </svg>
      <svg
        v-else
        class="h-3.5 w-3.5"
        fill="none"
        stroke="currentColor"
        viewBox="0 0 24 24"
        aria-hidden="true"
      >
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="3" d="M5 13l4 4L19 7" />
      </svg>
    </CheckboxIndicator>
  </CheckboxRoot>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { CheckboxRoot, CheckboxIndicator } from 'reka-ui'
import { cn } from '@/lib/utils'

type CheckedValue = boolean | 'indeterminate'

interface Props {
  modelValue: CheckedValue
  disabled?: boolean
  class?: string
}

const props = withDefaults(defineProps<Props>(), {
  disabled: false,
})

const emit = defineEmits<{
  'update:modelValue': [value: CheckedValue]
}>()

// Boolean | 'indeterminate' v-model bridge (Reka 2.x — v-model on CheckboxRoot,
// NOT v-model:checked).
const model = computed<CheckedValue>({
  get: () => props.modelValue,
  set: (v) => emit('update:modelValue', v),
})

const rootClasses = computed(() =>
  cn(
    'h-5 w-5 shrink-0 rounded-md border border-input data-[state=checked]:bg-primary data-[state=checked]:border-primary data-[state=indeterminate]:bg-primary transition-colors disabled:opacity-50 disabled:cursor-not-allowed flex items-center justify-center',
    props.class,
  ),
)
</script>
