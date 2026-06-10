<template>
  <Primitive
    :as="href ? 'a' : 'button'"
    :href="href"
    :type="href ? undefined : type"
    :disabled="disabled || loading"
    :class="cn(buttonVariants({ variant, size }), radius && radiusClass[radius], fullWidth && 'w-full', 'touch-target', props.class)"
  >
    <Spinner v-if="loading" size="sm" tone="mono" class="mr-2" />
    <span v-if="$slots.icon && !loading" class="mr-2">
      <slot name="icon" />
    </span>
    <slot />
  </Primitive>
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { Primitive } from 'reka-ui'
import { cn } from '@/lib/utils'
import { buttonVariants, type ButtonVariants } from './button-variants'
import Spinner from './Spinner.vue'

interface Props {
  variant?: NonNullable<ButtonVariants['variant']>
  size?: NonNullable<ButtonVariants['size']>
  type?: 'button' | 'submit' | 'reset'
  href?: string
  disabled?: boolean
  loading?: boolean
  fullWidth?: boolean
  radius?: 'sm' | 'md' | 'lg' | 'xl' | 'full'
  class?: HTMLAttributes['class']
}

const props = withDefaults(defineProps<Props>(), {
  variant: 'default',
  size: 'md',
  type: 'button',
  disabled: false,
  loading: false,
  fullWidth: false,
})

const radiusClass = {
  sm: 'rounded-sm', md: 'rounded-md', lg: 'rounded-lg', xl: 'rounded-xl', full: 'rounded-full',
} as const
</script>
