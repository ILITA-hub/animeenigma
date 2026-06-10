<template>
  <div :class="cn('flex flex-col items-center justify-center gap-3 p-8', props.class)" role="status">
    <Spinner :size="size" :tone="tone" :label="label ? '' : 'Loading'" />
    <span v-if="label" data-testid="loadingstate-label" class="text-sm text-muted-foreground">{{ label }}</span>
  </div>
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { cn } from '@/lib/utils'
import Spinner from './Spinner.vue'
import type { SpinnerVariants } from './spinner-variants'

interface Props {
  label?: string
  size?: NonNullable<SpinnerVariants['size']>
  tone?: NonNullable<SpinnerVariants['tone']>
  class?: HTMLAttributes['class']
}

const props = withDefaults(defineProps<Props>(), { size: 'lg', tone: 'signature' })
</script>
