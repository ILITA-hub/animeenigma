<template>
  <component
    :is="href ? 'a' : as"
    :href="href"
    :class="cardClasses"
  >
    <slot />
  </component>
</template>

<script setup lang="ts">
import { computed } from 'vue'

interface Props {
  as?: string
  href?: string
  variant?: 'default' | 'elevated' | 'interactive'
  padding?: 'none' | 'sm' | 'md' | 'lg'
  rounded?: 'md' | 'lg' | 'xl' | '2xl'
}

const props = withDefaults(defineProps<Props>(), {
  as: 'div',
  variant: 'default',
  padding: 'md',
  rounded: '2xl',
})

const cardClasses = computed(() => {
  const base = 'block'

  const variants = {
    default: 'glass-card',
    elevated: 'glass-elevated rounded-2xl',
    interactive: 'glass-card card-hover cursor-pointer',
  }

  const paddings = {
    none: '',
    sm: 'p-3',
    md: 'p-4',
    lg: 'p-6',
  }

  const roundings = {
    md: 'rounded-md',
    lg: 'rounded-lg',
    xl: 'rounded-xl',
    '2xl': 'rounded-2xl',
  }

  return [
    base,
    variants[props.variant],
    paddings[props.padding],
    props.variant === 'default' ? roundings[props.rounded] : '',
  ].join(' ')
})
</script>
