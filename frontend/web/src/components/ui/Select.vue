<template>
  <div :class="wrapperClasses" ref="wrapperRef">
    <label v-if="label" :id="labelId" class="block text-sm font-medium text-white/70 mb-2">
      {{ label }}
    </label>
    <div class="relative">
      <button
        type="button"
        :disabled="disabled"
        :class="triggerClasses"
        :aria-labelledby="label ? labelId : undefined"
        :aria-expanded="isOpen"
        aria-haspopup="listbox"
        @click="toggle"
        @keydown="handleKeydown"
      >
        <span :class="selectedLabel ? 'text-white' : 'text-white/30'">
          {{ selectedLabel || placeholder }}
        </span>
        <svg
          class="w-4 h-4 text-white/50 transition-transform duration-200"
          :class="{ 'rotate-180': isOpen }"
          fill="none"
          stroke="currentColor"
          viewBox="0 0 24 24"
        >
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7" />
        </svg>
      </button>

      <Transition
        enter-active-class="transition duration-150 ease-out"
        enter-from-class="opacity-0 scale-95 -translate-y-1"
        enter-to-class="opacity-100 scale-100 translate-y-0"
        leave-active-class="transition duration-100 ease-in"
        leave-from-class="opacity-100 scale-100 translate-y-0"
        leave-to-class="opacity-0 scale-95 -translate-y-1"
      >
        <ul
          v-if="isOpen"
          :class="dropdownClasses"
          role="listbox"
          :aria-activedescendant="focusedIndex >= 0 ? `option-${focusedIndex}` : undefined"
        >
          <li
            v-for="(option, index) in options"
            :key="option.value"
            :id="`option-${index}`"
            role="option"
            :aria-selected="option.value === modelValue"
            :class="optionClasses(option, index)"
            @click="selectOption(option)"
            @mouseenter="focusedIndex = index"
          >
            <span>{{ option.label }}</span>
            <svg
              v-if="option.value === modelValue"
              class="w-4 h-4 text-cyan-400"
              fill="none"
              stroke="currentColor"
              viewBox="0 0 24 24"
            >
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
            </svg>
          </li>
        </ul>
      </Transition>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch, onMounted, onUnmounted } from 'vue'

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

const isOpen = ref(false)
const focusedIndex = ref(-1)
const wrapperRef = ref<HTMLElement | null>(null)
const labelId = `select-label-${Math.random().toString(36).slice(2, 9)}`

const selectedLabel = computed(() => {
  const option = props.options.find(o => o.value === props.modelValue)
  return option?.label
})

const wrapperClasses = computed(() => 'w-full')

const triggerClasses = computed(() => {
  const base = 'w-full flex items-center justify-between bg-white/5 border text-white transition-all duration-200 focus:outline-none cursor-pointer'

  const sizes = {
    xs: 'px-2 py-1 text-xs rounded-lg gap-1',
    sm: 'px-3 py-2 text-sm rounded-lg gap-2',
    md: 'px-4 py-3 text-base rounded-xl gap-2',
    lg: 'px-5 py-4 text-lg rounded-xl gap-3',
  }

  const states = isOpen.value
    ? 'border-cyan-400 ring-2 ring-cyan-400/20'
    : 'border-white/10 hover:border-white/20'

  return [
    base,
    sizes[props.size],
    states,
    props.disabled ? 'opacity-50 cursor-not-allowed' : '',
  ].join(' ')
})

const dropdownClasses = computed(() => {
  const sizes = {
    xs: 'text-xs',
    sm: 'text-sm',
    md: 'text-base',
    lg: 'text-lg',
  }

  return [
    'absolute z-50 w-full mt-1 py-1 bg-slate-900/95 backdrop-blur-xl border border-white/10 rounded-xl shadow-xl shadow-black/20 max-h-60 overflow-auto',
    sizes[props.size],
  ].join(' ')
})

const optionClasses = (option: SelectOption, index: number) => {
  const base = 'flex items-center justify-between px-4 py-2.5 cursor-pointer transition-colors'
  const isSelected = option.value === props.modelValue
  const isFocused = index === focusedIndex.value

  if (isSelected) {
    return `${base} bg-cyan-500/20 text-cyan-300`
  }
  if (isFocused) {
    return `${base} bg-white/10 text-white`
  }
  return `${base} text-white/70 hover:bg-white/5 hover:text-white`
}

const toggle = () => {
  if (props.disabled) return
  isOpen.value = !isOpen.value
  if (isOpen.value) {
    focusedIndex.value = props.options.findIndex(o => o.value === props.modelValue)
  }
}

const selectOption = (option: SelectOption) => {
  emit('update:modelValue', option.value)
  emit('change', option.value)
  isOpen.value = false
}

const handleKeydown = (event: KeyboardEvent) => {
  if (props.disabled) return

  switch (event.key) {
    case 'Enter':
    case ' ':
      event.preventDefault()
      if (isOpen.value && focusedIndex.value >= 0) {
        selectOption(props.options[focusedIndex.value])
      } else {
        toggle()
      }
      break
    case 'Escape':
      isOpen.value = false
      break
    case 'ArrowDown':
      event.preventDefault()
      if (!isOpen.value) {
        isOpen.value = true
      } else {
        focusedIndex.value = Math.min(focusedIndex.value + 1, props.options.length - 1)
      }
      break
    case 'ArrowUp':
      event.preventDefault()
      if (!isOpen.value) {
        isOpen.value = true
      } else {
        focusedIndex.value = Math.max(focusedIndex.value - 1, 0)
      }
      break
    case 'Home':
      event.preventDefault()
      focusedIndex.value = 0
      break
    case 'End':
      event.preventDefault()
      focusedIndex.value = props.options.length - 1
      break
  }
}

const handleClickOutside = (event: MouseEvent) => {
  if (wrapperRef.value && !wrapperRef.value.contains(event.target as Node)) {
    isOpen.value = false
  }
}

onMounted(() => {
  document.addEventListener('click', handleClickOutside)
})

onUnmounted(() => {
  document.removeEventListener('click', handleClickOutside)
})

watch(isOpen, (value) => {
  if (!value) {
    focusedIndex.value = -1
  }
})
</script>
