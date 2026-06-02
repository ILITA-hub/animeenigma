<template>
  <!-- Relies on a TooltipProvider ancestor (mounted once at app root in App.vue,
       the standard shadcn-vue pattern). -->
  <TooltipRoot :delay-duration="delayDuration">
    <TooltipTrigger as-child>
      <slot name="trigger" />
    </TooltipTrigger>

    <TooltipPortal>
      <TooltipContent :side="side" :side-offset="sideOffset" :class="contentClasses">
        <slot />
        <TooltipArrow class="fill-popover" />
      </TooltipContent>
    </TooltipPortal>
  </TooltipRoot>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import {
  TooltipRoot,
  TooltipTrigger,
  TooltipPortal,
  TooltipContent,
  TooltipArrow,
} from 'reka-ui'
import { cn } from '@/lib/utils'

interface Props {
  /** Hover delay before the tooltip shows (ms). Forwarded to TooltipRoot. */
  delayDuration?: number
  side?: 'top' | 'right' | 'bottom' | 'left'
  sideOffset?: number
  class?: string
}

const props = withDefaults(defineProps<Props>(), {
  side: 'top',
  sideOffset: 4,
})

const contentClasses = computed(() =>
  cn(
    'z-[9999] bg-popover text-popover-foreground rounded-md px-3 py-1.5 text-xs shadow-md select-none',
    props.class,
  ),
)

// Exposed for the co-located spec (portaled content can't be asserted in jsdom).
defineExpose({ contentClasses })
</script>
