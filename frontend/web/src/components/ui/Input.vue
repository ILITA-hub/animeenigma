<template>
  <div :class="wrapperClasses">
    <label v-if="label" :for="inputId" class="block text-sm font-medium text-white/70 mb-2">
      {{ label }}
    </label>
    <div class="relative">
      <span v-if="$slots.prefix" class="absolute left-3 top-1/2 -translate-y-1/2 text-white/50">
        <slot name="prefix" />
      </span>
      <input
        :id="inputId"
        ref="inputRef"
        v-bind="restAttrs"
        v-model="model"
        :type="type"
        :placeholder="placeholder"
        :disabled="disabled"
        :readonly="readonly"
        :class="cn(inputClasses, $slots.prefix ? 'pl-10' : '', (clearable || $slots.suffix) ? 'pr-10' : '', passedClass)"
        @focus="focused = true"
        @blur="focused = false"
      />
      <span v-if="$slots.suffix" class="absolute right-3 top-1/2 -translate-y-1/2 text-white/50">
        <slot name="suffix" />
      </span>
      <button
        v-if="clearable && model"
        type="button"
        class="absolute right-3 top-1/2 -translate-y-1/2 text-white/50 hover:text-white transition-colors"
        @click="model = ''"
      >
        <X class="size-4" aria-hidden="true" />
      </button>
    </div>
    <p v-if="error" class="mt-1 text-sm text-destructive">{{ error }}</p>
    <p v-else-if="hint" class="mt-1 text-sm text-white/50">{{ hint }}</p>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, useAttrs } from 'vue'
import { X } from 'lucide-vue-next'
import { cn } from '@/lib/utils'

defineOptions({ inheritAttrs: false })

interface Props {
  modelValue?: string | number
  type?: 'text' | 'email' | 'password' | 'search' | 'number' | 'tel' | 'url' | 'date'
  placeholder?: string
  label?: string
  hint?: string
  error?: string
  disabled?: boolean
  readonly?: boolean
  clearable?: boolean
  size?: 'sm' | 'md' | 'lg'
  variant?: 'default' | 'filled'
}

const props = withDefaults(defineProps<Props>(), {
  modelValue: '',
  type: 'text',
  disabled: false,
  readonly: false,
  clearable: false,
  size: 'md',
  variant: 'default',
})

const emit = defineEmits<{
  'update:modelValue': [value: string | number]
}>()

const model = computed({
  get: () => props.modelValue,
  set: (value: string | number) => emit('update:modelValue', value),
})

const focused = ref(false)
const inputId = `input-${Math.random().toString(36).slice(2, 9)}`

const attrs = useAttrs()
const passedClass = computed(() => attrs.class as string | undefined)
const restAttrs = computed(() => {
  const { class: _omitClass, ...rest } = attrs
  return rest
})
const inputRef = ref<HTMLInputElement | null>(null)
defineExpose({ focus: () => inputRef.value?.focus() })

const wrapperClasses = computed(() => 'w-full')

const inputClasses = computed(() => {
  const base = 'w-full bg-white/5 border text-white placeholder-white/30 transition-all duration-200 focus:outline-none focus-visible:outline-none'

  const sizes = {
    sm: 'px-3 py-2 text-sm rounded-lg touch-target',
    md: 'px-4 py-3 text-base rounded-xl',
    lg: 'px-5 py-4 text-lg rounded-xl',
  }

  // Standardized focus outline: thin cyan-500/50 ring on focus-visible (keyboard +
  // text-field focus), neutral border — same as Select.vue / DatePicker.vue.
  const states = props.error
    ? 'border-destructive focus-visible:ring-2 focus-visible:ring-destructive/50'
    : 'border-white/10 focus-visible:ring-2 focus-visible:ring-cyan-500/50'

  return cn(
    base,
    sizes[props.size],
    states,
    props.disabled ? 'opacity-50 cursor-not-allowed' : '',
  )
})
</script>

<style scoped>
/* Hide native search input clear button (browsers add their own "x") */
input[type="search"]::-webkit-search-cancel-button,
input[type="search"]::-webkit-search-decoration {
  -webkit-appearance: none;
  appearance: none;
}
</style>
