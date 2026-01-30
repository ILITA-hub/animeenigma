<template>
  <span :class="badgeClasses">
    <span v-if="$slots.icon" class="mr-1">
      <slot name="icon" />
    </span>
    <slot />
  </span>
</template>

<script setup lang="ts">
import { computed } from 'vue'

interface Props {
  variant?: 'default' | 'primary' | 'secondary' | 'success' | 'warning' | 'rating'
  size?: 'sm' | 'md' | 'lg'
}

const props = withDefaults(defineProps<Props>(), {
  variant: 'default',
  size: 'md',
})

const badgeClasses = computed(() => {
  const base = 'inline-flex items-center font-medium'

  const variants = {
    default: 'bg-white/10 text-white/80',
    primary: 'bg-cyan-500/20 text-cyan-400',
    secondary: 'bg-pink-500/20 text-pink-400',
    success: 'bg-emerald-500/20 text-emerald-400',
    warning: 'bg-amber-500/20 text-amber-400',
    rating: 'bg-black/60 text-amber-400 backdrop-blur-sm',
  }

  const sizes = {
    sm: 'px-2 py-0.5 text-xs rounded',
    md: 'px-2.5 py-1 text-sm rounded-md',
    lg: 'px-3 py-1.5 text-base rounded-lg',
  }

  return [base, variants[props.variant], sizes[props.size]].join(' ')
})
</script>
