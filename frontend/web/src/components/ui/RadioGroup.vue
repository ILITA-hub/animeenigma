<template>
  <RadioGroupRoot
    v-model="model"
    :disabled="disabled"
    class="space-y-1"
  >
    <label
      v-for="opt in options"
      :key="opt.value || '__any__'"
      class="flex items-center gap-2 text-sm text-white/70 hover:text-white cursor-pointer py-0.5"
    >
      <RadioGroupItem
        :value="opt.value"
        :disabled="disabled"
        class="h-4 w-4 shrink-0 rounded-full border border-input data-[state=checked]:border-primary transition-colors disabled:opacity-50 disabled:cursor-not-allowed flex items-center justify-center"
      >
        <RadioGroupIndicator class="h-2 w-2 rounded-full bg-primary" />
      </RadioGroupItem>
      <span>{{ opt.label }}</span>
    </label>
  </RadioGroupRoot>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { RadioGroupRoot, RadioGroupItem, RadioGroupIndicator } from 'reka-ui'

interface Option {
  value: string
  label: string
}

interface Props {
  modelValue: string
  options: Option[]
  disabled?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  disabled: false,
})

const emit = defineEmits<{
  'update:modelValue': [value: string]
}>()

const model = computed<string>({
  get: () => props.modelValue,
  set: (v) => emit('update:modelValue', v),
})
</script>
