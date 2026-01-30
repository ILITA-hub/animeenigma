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
        v-model="model"
        :type="type"
        :placeholder="placeholder"
        :disabled="disabled"
        :readonly="readonly"
        :class="inputClasses"
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
        <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
        </svg>
      </button>
    </div>
    <p v-if="error" class="mt-1 text-sm text-pink-400">{{ error }}</p>
    <p v-else-if="hint" class="mt-1 text-sm text-white/50">{{ hint }}</p>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'

interface Props {
  modelValue?: string
  type?: 'text' | 'email' | 'password' | 'search' | 'number' | 'tel' | 'url'
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
  'update:modelValue': [value: string]
}>()

const model = computed({
  get: () => props.modelValue,
  set: (value: string) => emit('update:modelValue', value),
})

const focused = ref(false)
const inputId = `input-${Math.random().toString(36).slice(2, 9)}`

const wrapperClasses = computed(() => 'w-full')

const inputClasses = computed(() => {
  const base = 'w-full bg-white/5 border text-white placeholder-white/30 transition-all duration-200 focus:outline-none'

  const sizes = {
    sm: 'px-3 py-2 text-sm rounded-lg',
    md: 'px-4 py-3 text-base rounded-xl',
    lg: 'px-5 py-4 text-lg rounded-xl',
  }

  const states = props.error
    ? 'border-pink-500 focus:border-pink-400 focus:ring-2 focus:ring-pink-400/20'
    : 'border-white/10 focus:border-cyan-400 focus:ring-2 focus:ring-cyan-400/20'

  return [
    base,
    sizes[props.size],
    states,
    props.disabled ? 'opacity-50 cursor-not-allowed' : '',
  ].join(' ')
})
</script>
