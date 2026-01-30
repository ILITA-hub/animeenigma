<template>
  <component
    :is="href ? 'a' : 'button'"
    :href="href"
    :type="href ? undefined : type"
    :disabled="disabled || loading"
    :class="buttonClasses"
    class="touch-target"
  >
    <span v-if="loading" class="animate-spin mr-2">
      <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24">
        <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" />
        <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
      </svg>
    </span>
    <span v-if="$slots.icon && !loading" class="mr-2">
      <slot name="icon" />
    </span>
    <slot />
  </component>
</template>

<script setup lang="ts">
import { computed } from 'vue'

interface Props {
  variant?: 'primary' | 'secondary' | 'ghost' | 'outline'
  size?: 'sm' | 'md' | 'lg'
  type?: 'button' | 'submit' | 'reset'
  href?: string
  disabled?: boolean
  loading?: boolean
  fullWidth?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  variant: 'primary',
  size: 'md',
  type: 'button',
  disabled: false,
  loading: false,
  fullWidth: false,
})

const buttonClasses = computed(() => {
  const base = 'inline-flex items-center justify-center font-medium transition-all duration-200 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-cyan-400 disabled:opacity-50 disabled:cursor-not-allowed'

  const variants = {
    primary: 'bg-cyan-500 hover:bg-cyan-400 text-base rounded-xl hover:shadow-[0_0_30px_rgba(0,212,255,0.3)] active:scale-95',
    secondary: 'bg-pink-500 hover:bg-pink-400 text-white rounded-xl hover:shadow-[0_0_30px_rgba(255,45,124,0.3)] active:scale-95',
    ghost: 'bg-white/5 hover:bg-white/10 text-white rounded-lg border border-white/10 hover:border-white/20',
    outline: 'bg-transparent hover:bg-white/5 text-cyan-400 rounded-xl border border-cyan-400/50 hover:border-cyan-400',
  }

  const sizes = {
    sm: 'px-3 py-1.5 text-sm',
    md: 'px-6 py-3 text-base',
    lg: 'px-8 py-4 text-lg',
  }

  return [
    base,
    variants[props.variant],
    sizes[props.size],
    props.fullWidth ? 'w-full' : '',
  ].join(' ')
})
</script>
