<template>
  <div class="w-full">
    <label v-if="label" :id="labelId" class="block text-sm font-medium text-white/70 mb-2">
      {{ label }}
    </label>
    <SelectRoot
      :model-value="modelValue !== undefined ? String(modelValue) : undefined"
      :disabled="disabled"
      @update:model-value="onModelUpdate"
    >
      <SelectTrigger
        :class="triggerClasses"
        :aria-labelledby="label ? labelId : undefined"
        aria-haspopup="listbox"
      >
        <SelectValue :placeholder="placeholder">
          <span :class="selectedLabel ? 'text-white' : 'text-white/30'">
            {{ selectedLabel || placeholder }}
          </span>
        </SelectValue>
        <SelectIcon as-child>
          <ChevronDown
            class="size-4 text-white/50 transition-transform duration-200 data-[state=open]:rotate-180"
            aria-hidden="true"
          />
        </SelectIcon>
      </SelectTrigger>

      <SelectPortal>
        <SelectContent
          position="popper"
          :side-offset="4"
          :class="dropdownClasses"
          :style="{ width: 'var(--reka-select-trigger-width)' }"
        >
          <SelectViewport class="py-1">
            <SelectItem
              v-for="option in options"
              :key="option.value"
              :value="String(option.value)"
              :class="optionClasses"
            >
              <SelectItemText>{{ option.label }}</SelectItemText>
              <SelectItemIndicator>
                <Check class="size-4 text-cyan-400" aria-hidden="true" />
              </SelectItemIndicator>
            </SelectItem>
          </SelectViewport>
        </SelectContent>
      </SelectPortal>
    </SelectRoot>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { Check, ChevronDown } from 'lucide-vue-next'
import {
  SelectRoot,
  SelectTrigger,
  SelectValue,
  SelectIcon,
  SelectPortal,
  SelectContent,
  SelectViewport,
  SelectItem,
  SelectItemText,
  SelectItemIndicator,
} from 'reka-ui'
import { cn } from '@/lib/utils'

export interface SelectOption {
  value: string | number
  label: string
}

interface Props {
  modelValue?: string | number
  options: SelectOption[]
  placeholder?: string
  label?: string
  disabled?: boolean
  size?: 'xs' | 'sm' | 'md' | 'lg'
  /** Extra classes merged onto the trigger (e.g. per-status color). Wins over base via tailwind-merge. */
  triggerClass?: string
}

const props = withDefaults(defineProps<Props>(), {
  placeholder: 'Select...',
  disabled: false,
  size: 'md',
})

const emit = defineEmits<{
  'update:modelValue': [value: string | number]
  'change': [value: string | number]
}>()

const labelId = `select-label-${Math.random().toString(36).slice(2, 9)}`

const selectedLabel = computed(() => {
  const option = props.options.find(o => o.value === props.modelValue)
  return option?.label
})

const triggerClasses = computed(() => {
  // Standardized focus/selected outline: thin cyan-500/50 ring matching DatePicker.
  // focus-visible:* neutralizes the global :focus-visible fat double-ring; the same
  // ring shows on open so the selected state matches the date picker exactly.
  const base = 'w-full flex items-center justify-between bg-white/5 border border-white/10 text-white transition-all duration-200 cursor-pointer hover:border-white/20 focus:outline-none focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-cyan-500/50 data-[state=open]:ring-2 data-[state=open]:ring-cyan-500/50'

  const sizes = {
    xs: 'px-2 py-1 text-xs rounded-lg gap-1',
    sm: 'px-3 py-2 text-sm rounded-lg gap-2',
    md: 'px-4 py-3 text-base rounded-xl gap-2',
    lg: 'px-5 py-4 text-lg rounded-xl gap-3',
  }

  return cn(
    base,
    sizes[props.size],
    props.disabled ? 'opacity-50 cursor-not-allowed' : '',
    props.triggerClass,
  )
})

const dropdownClasses = computed(() => {
  const sizes = {
    xs: 'text-xs',
    sm: 'text-sm',
    md: 'text-base',
    lg: 'text-lg',
  }

  return cn(
    'z-50 mt-1 bg-popover/95 backdrop-blur-xl border border-white/10 rounded-xl shadow-xl shadow-black/20 max-h-60 overflow-auto',
    sizes[props.size],
  )
})

// Static per-option base; Reka toggles selected/highlighted state via data-attrs:
// - data-[state=checked]    → selected  (bg-cyan-500/20 text-cyan-300)
// - data-[highlighted]      → keyboard/hover focus (bg-white/10 text-white)
// - default                 → text-white/70 hover:bg-white/5 hover:text-white
const optionClasses = cn(
  'flex items-center justify-between px-4 py-2.5 cursor-pointer transition-colors outline-none',
  'text-white/70 hover:bg-white/5 hover:text-white',
  'data-[highlighted]:bg-white/10 data-[highlighted]:text-white',
  'data-[state=checked]:bg-cyan-500/20 data-[state=checked]:text-cyan-300',
)

// Bridge Reka's string model back onto the original number|string API and
// re-fire BOTH emits the legacy component fired in selectOption().
const onSelect = (value: string | number) => {
  // Recover the original (possibly numeric) option value by identity match.
  const match = props.options.find(o => String(o.value) === String(value))
  const out = match ? match.value : value
  emit('update:modelValue', out)
  emit('change', out)
}

const onModelUpdate = (value: unknown) => {
  if (value === undefined || value === null) return
  onSelect(value as string | number)
}

// Exposed so the co-located spec can drive selection without depending on the
// portaled SelectItem DOM (jsdom can't render the Reka portal — RESEARCH Pitfall 6).
defineExpose({ onSelect })
</script>
