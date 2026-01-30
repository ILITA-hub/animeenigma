<template>
  <div class="w-full">
    <div :class="tabListClasses" role="tablist">
      <button
        v-for="tab in tabs"
        :key="tab.value"
        role="tab"
        :aria-selected="modelValue === tab.value"
        :class="getTabClasses(tab.value)"
        @click="$emit('update:modelValue', tab.value)"
      >
        <span v-if="tab.icon" class="mr-2">
          <component :is="tab.icon" class="w-4 h-4" />
        </span>
        {{ tab.label }}
        <span v-if="tab.count !== undefined" class="ml-2 px-1.5 py-0.5 text-xs rounded-full bg-white/10">
          {{ tab.count }}
        </span>
      </button>
    </div>
    <div class="mt-4" role="tabpanel">
      <slot :name="modelValue" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, type Component } from 'vue'

interface Tab {
  value: string
  label: string
  icon?: Component
  count?: number
  disabled?: boolean
}

interface Props {
  modelValue: string
  tabs: Tab[]
  variant?: 'default' | 'pills' | 'underline'
  fullWidth?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  variant: 'default',
  fullWidth: false,
})

defineEmits<{
  'update:modelValue': [value: string]
}>()

const tabListClasses = computed(() => {
  const base = 'flex gap-1'

  const variants = {
    default: 'p-1 bg-white/5 rounded-xl',
    pills: 'gap-2',
    underline: 'border-b border-white/10 gap-4',
  }

  return [
    base,
    variants[props.variant],
    props.fullWidth ? 'w-full' : 'w-fit',
  ].join(' ')
})

const getTabClasses = (value: string) => {
  const isActive = props.modelValue === value
  const tab = props.tabs.find(t => t.value === value)
  const isDisabled = tab?.disabled

  const base = 'px-4 py-2 text-sm font-medium transition-all duration-200 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-cyan-400'

  const variants = {
    default: {
      active: 'bg-white/10 text-white rounded-lg',
      inactive: 'text-white/60 hover:text-white hover:bg-white/5 rounded-lg',
    },
    pills: {
      active: 'bg-cyan-500/20 text-cyan-400 rounded-full border border-cyan-500/30',
      inactive: 'text-white/60 hover:text-white hover:bg-white/5 rounded-full border border-transparent',
    },
    underline: {
      active: 'text-cyan-400 border-b-2 border-cyan-400 -mb-px',
      inactive: 'text-white/60 hover:text-white border-b-2 border-transparent -mb-px',
    },
  }

  const disabled = 'opacity-50 cursor-not-allowed pointer-events-none'

  return [
    base,
    isActive ? variants[props.variant].active : variants[props.variant].inactive,
    props.fullWidth ? 'flex-1 justify-center' : '',
    isDisabled ? disabled : '',
  ].join(' ')
}
</script>
