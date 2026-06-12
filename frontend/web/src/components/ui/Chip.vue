<!--
  Chip — DS primitive for filter pills.

  Two modes:
  1. Toggle (default) — root is a <button> with aria-pressed, parent listens
     to the native click:
       <Chip :active="filter === 'all'" :count="42" @click="filter = 'all'">Все</Chip>
  2. Removable — root is an inert <span>, the ✕ button emits `remove`
     (a button can't nest inside a button):
       <Chip active removable size="sm" @remove="drop()">★ Мой список</Chip>
-->
<template>
  <component
    :is="removable ? 'span' : 'button'"
    :type="removable ? undefined : 'button'"
    :aria-pressed="removable ? undefined : active === true"
    :class="cn(chipVariants({ size, active }), props.class)"
  >
    <slot />
    <span v-if="count !== undefined" class="opacity-80 tabular-nums" data-testid="chip-count">({{ count }})</span>
    <button
      v-if="removable"
      type="button"
      data-testid="chip-remove"
      class="cursor-pointer opacity-70 hover:opacity-100 transition-opacity -mr-0.5"
      :aria-label="removeLabel || t('common.cancel')"
      @click="emit('remove')"
    >
      <X class="size-3" aria-hidden="true" />
    </button>
  </component>
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { useI18n } from 'vue-i18n'
import { X } from 'lucide-vue-next'
import { cn } from '@/lib/utils'
import { chipVariants, type ChipVariants } from './chip-variants'

const props = withDefaults(
  defineProps<{
    active?: boolean
    size?: NonNullable<ChipVariants['size']>
    /** Renders a ✕ button emitting `remove`; the chip itself becomes inert. */
    removable?: boolean
    /** Muted "(n)" suffix after the label. */
    count?: number | string
    removeLabel?: string
    class?: HTMLAttributes['class']
  }>(),
  { active: false, size: 'md', removable: false, count: undefined, removeLabel: undefined },
)

const emit = defineEmits<{ remove: [] }>()

const { t } = useI18n()
</script>
