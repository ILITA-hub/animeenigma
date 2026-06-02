<template>
  <span :class="cn(badgeVariants({ variant, size }), props.class)">
    <span v-if="$slots.icon" class="mr-1">
      <slot name="icon" />
    </span>
    <slot />
  </span>
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { cn } from '@/lib/utils'
import { badgeVariants, type BadgeVariants } from './badge-variants'

interface Props {
  variant?: NonNullable<BadgeVariants['variant']>
  size?: NonNullable<BadgeVariants['size']>
  class?: HTMLAttributes['class']
}

const props = withDefaults(defineProps<Props>(), {
  variant: 'default',
  size: 'md',
})
</script>
