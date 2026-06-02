<template>
  <PopoverRoot :open="open" @update:open="onOpenUpdate">
    <PopoverTrigger as-child>
      <slot name="trigger" />
    </PopoverTrigger>

    <PopoverPortal>
      <PopoverContent :side="side" :align="align" :side-offset="sideOffset" :class="contentClasses">
        <slot />
        <PopoverArrow class="fill-popover" />
      </PopoverContent>
    </PopoverPortal>
  </PopoverRoot>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import {
  PopoverRoot,
  PopoverTrigger,
  PopoverPortal,
  PopoverContent,
  PopoverArrow,
} from 'reka-ui'
import { cn } from '@/lib/utils'

interface Props {
  /** Controlled open state. Pair with @update:open for v-model:open. */
  open?: boolean
  side?: 'top' | 'right' | 'bottom' | 'left'
  align?: 'start' | 'center' | 'end'
  sideOffset?: number
  class?: string
}

const props = withDefaults(defineProps<Props>(), {
  side: 'bottom',
  align: 'center',
  sideOffset: 4,
})

const emit = defineEmits<{
  'update:open': [value: boolean]
}>()

const contentClasses = computed(() =>
  cn(
    'z-[9999] bg-popover text-popover-foreground border border-white/10 rounded-xl p-4 shadow-xl',
    props.class,
  ),
)

// Bridge Reka's open-change back onto the controlled v-model:open contract.
const onOpenUpdate = (value: boolean) => {
  emit('update:open', value)
}

// Exposed for the co-located spec (portaled content can't be asserted in jsdom).
defineExpose({ onOpenUpdate, contentClasses })
</script>
