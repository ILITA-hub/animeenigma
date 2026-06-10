<!--
  SegmentedControl — a panel-less single-select switcher (the inset-pill row).

  Unlike `Tabs` (which couples a tab row to a `<slot :name>` panel), this is just
  the control: mutually-exclusive options that emit `update:modelValue`. Use it
  when the switched content lives elsewhere (e.g. a calendar view-mode, a
  table/grid toggle) rather than directly under the buttons.

  ARIA: `role="radiogroup"` on the container, `role="radio"` + `aria-checked`
  on each segment — the correct semantics for a single-choice control.

  Items may be text (`label`) and/or icon (`icon`). Set `icon-only` to render
  just the glyph (the `label` is still used as the accessible name).
-->
<script setup lang="ts">
import { type Component, type HTMLAttributes } from 'vue'
import { cn } from '@/lib/utils'
import {
  segmentedControlVariants,
  segmentVariants,
  type SegmentedControlVariants,
} from './segmented-control-variants'

interface SegmentOption {
  value: string
  label: string
  icon?: Component
}

const props = withDefaults(
  defineProps<{
    modelValue: string
    options: SegmentOption[]
    size?: SegmentedControlVariants['size']
    iconOnly?: boolean
    fullWidth?: boolean
    /** Accessible name for the whole group (recommended). */
    ariaLabel?: string
    class?: HTMLAttributes['class']
  }>(),
  { size: 'sm', iconOnly: false, fullWidth: false },
)

defineEmits<{ 'update:modelValue': [value: string] }>()
</script>

<template>
  <div
    role="radiogroup"
    :aria-label="props.ariaLabel"
    :class="cn(segmentedControlVariants({ fullWidth: props.fullWidth }), props.class)"
  >
    <button
      v-for="opt in props.options"
      :key="opt.value"
      type="button"
      role="radio"
      :aria-checked="props.modelValue === opt.value"
      :aria-label="props.iconOnly ? opt.label : undefined"
      :title="props.iconOnly ? opt.label : undefined"
      :data-active="props.modelValue === opt.value"
      :data-value="opt.value"
      :class="segmentVariants({ size: props.size, iconOnly: props.iconOnly, fullWidth: props.fullWidth })"
      @click="$emit('update:modelValue', opt.value)"
    >
      <component :is="opt.icon" v-if="opt.icon" class="size-4 shrink-0" aria-hidden="true" />
      <span v-if="!props.iconOnly">{{ opt.label }}</span>
    </button>
  </div>
</template>
