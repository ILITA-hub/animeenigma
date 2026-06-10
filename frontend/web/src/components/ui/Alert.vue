<template>
  <div :class="cn(alertVariants({ variant }), props.class)" role="alert">
    <span :class="cn('mt-0.5 shrink-0', alertIconColor[variant])" aria-hidden="true">
      <slot name="icon">
        <component :is="defaultIcon" class="size-[18px]" />
      </slot>
    </span>

    <div class="min-w-0 flex-1">
      <div v-if="title" class="font-semibold text-foreground">{{ title }}</div>
      <div class="text-muted-foreground [overflow-wrap:anywhere]"><slot /></div>
    </div>

    <button
      v-if="dismissible"
      type="button"
      class="-my-1 -mr-1 ml-auto shrink-0 rounded-md p-1 text-muted-foreground transition-colors hover:bg-white/10 hover:text-foreground"
      :aria-label="dismissLabel"
      @click="emit('dismiss')"
    >
      <X class="size-4" />
    </button>
  </div>
</template>

<script setup lang="ts">
import { computed, type HTMLAttributes } from 'vue'
import { Info, CircleCheck, TriangleAlert, CircleX, X } from 'lucide-vue-next'
import { cn } from '@/lib/utils'
import { alertVariants, alertIconColor, type AlertVariant } from './alert-variants'

interface Props {
  variant?: AlertVariant
  title?: string
  dismissible?: boolean
  dismissLabel?: string
  class?: HTMLAttributes['class']
}

const props = withDefaults(defineProps<Props>(), {
  variant: 'info',
  dismissible: false,
  dismissLabel: 'Dismiss',
})

const emit = defineEmits<{ dismiss: [] }>()

const icons = { info: Info, success: CircleCheck, warning: TriangleAlert, destructive: CircleX }
const defaultIcon = computed(() => icons[props.variant])
</script>
