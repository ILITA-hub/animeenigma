<template>
  <Primitive
    :as="href ? 'a' : as"
    :href="href"
    :class="cardClasses"
  >
    <slot />
  </Primitive>
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { computed } from 'vue'
import { Primitive } from 'reka-ui'
import { cn } from '@/lib/utils'

interface Props {
  as?: string
  href?: string
  variant?: 'default' | 'elevated' | 'interactive'
  padding?: 'none' | 'sm' | 'md' | 'lg'
  rounded?: 'md' | 'lg' | 'xl' | '2xl'
  class?: HTMLAttributes['class']
}

const props = withDefaults(defineProps<Props>(), {
  as: 'div',
  variant: 'default',
  padding: 'md',
  rounded: '2xl',
})

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

const cardClasses = computed(() =>
  cn(
    'block',
    variants[props.variant],
    paddings[props.padding],
    props.variant === 'default' ? roundings[props.rounded] : '',
    props.class,
  ),
)
</script>
