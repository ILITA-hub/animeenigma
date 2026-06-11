<template>
  <component :is="as" :class="tileVariants({ tone, interactive })">
    <slot />
  </component>
</template>

<script setup lang="ts">
/**
 * Spotlight UI primitive (v4 lock, 2026-06-11) — the glass tile every
 * multi-item spotlight card builds rows/panels from. Part of the
 * spotlight-scoped primitive set: same DS token base as components/ui,
 * but tuned for the hero-card context (see docs/spotlight-card-guidelines.md).
 */
import { cva } from 'class-variance-authority'

withDefaults(
  defineProps<{
    /** Surface tone: glass (on mesh) or dark (on imagery). */
    tone?: 'glass' | 'dark'
    /** Adds the hover lift used by clickable rows. */
    interactive?: boolean
    /** Rendered element/component (e.g. 'li', 'article'). */
    as?: string
  }>(),
  { tone: 'glass', interactive: false, as: 'div' },
)

const tileVariants = cva(
  'border border-white/10 rounded-xl backdrop-blur-sm min-w-0',
  {
    variants: {
      tone: {
        glass: 'bg-white/5',
        dark: 'bg-black/30',
      },
      interactive: {
        true: 'hover:bg-white/10 transition-colors',
        false: '',
      },
    },
    defaultVariants: { tone: 'glass', interactive: false },
  },
)
</script>
