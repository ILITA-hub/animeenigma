<template>
  <SwitchRoot
    v-model="model"
    :disabled="disabled"
    :class="rootClasses"
  >
    <SwitchThumb
      class="pointer-events-none block h-5 w-5 rounded-full bg-white shadow-lg transition-transform data-[state=checked]:translate-x-5 data-[state=unchecked]:translate-x-0.5"
    />
  </SwitchRoot>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { SwitchRoot, SwitchThumb } from 'reka-ui'
import { cn } from '@/lib/utils'

interface Props {
  modelValue: boolean
  disabled?: boolean
  class?: string
}

const props = withDefaults(defineProps<Props>(), {
  disabled: false,
})

const emit = defineEmits<{
  'update:modelValue': [value: boolean]
}>()

// Plain boolean v-model bridge (Reka 2.x — v-model on SwitchRoot, NOT
// v-model:checked). Mirrors the Input/Select get/set computed pattern.
const model = computed<boolean>({
  get: () => props.modelValue,
  set: (v) => emit('update:modelValue', v),
})

const rootClasses = computed(() =>
  cn(
    'relative inline-flex h-6 w-11 shrink-0 items-center rounded-full transition-colors data-[state=checked]:bg-primary data-[state=unchecked]:bg-white/10 disabled:opacity-50 disabled:cursor-not-allowed',
    props.class,
  ),
)
</script>
