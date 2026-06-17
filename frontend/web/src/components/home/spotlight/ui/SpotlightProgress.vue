<template>
  <div class="min-w-0">
    <div class="relative h-1 rounded-full bg-white/[0.12] overflow-hidden">
      <span
        class="absolute inset-y-0 left-0 rounded-full"
        :class="barVariants({ accent })"
        :style="{ width: `${clamped}%` }"
        data-testid="progress-fill"
      />
    </div>
    <p
      v-if="label"
      class="font-mono text-[10px] uppercase tracking-[0.1em] text-muted-foreground mt-1.5"
    >
      {{ label }}
    </p>
  </div>
</template>

<script setup lang="ts">
/**
 * Spotlight UI primitive (v4 H-4 lock, 2026-06-11) — a thin brand-triad
 * progress line with an optional mono uppercase label underneath
 * (e.g. «5 из 24 эпизодов сезона»).
 */
import { computed } from 'vue'
import { cva } from 'class-variance-authority'

const props = withDefaults(
  defineProps<{
    /** 0..100 */
    percent: number
    accent?: 'cyan' | 'pink' | 'violet'
    label?: string
  }>(),
  { accent: 'pink', label: '' },
)

const clamped = computed(() => Math.max(0, Math.min(100, props.percent)))

const barVariants = cva('', {
  variants: {
    accent: {
      cyan: 'bg-gradient-to-r from-cyan-500 to-cyan-400 shadow-[0_0_8px_var(--cyan-a40)]',
      pink: 'bg-gradient-to-r from-pink-500 to-pink-400 shadow-[0_0_8px_rgba(255,45,124,0.5)]',
      violet: 'bg-gradient-to-r from-brand-violet/70 to-brand-violet shadow-[0_0_8px_rgba(167,139,250,0.5)]',
    },
  },
  defaultVariants: { accent: 'pink' },
})
</script>
