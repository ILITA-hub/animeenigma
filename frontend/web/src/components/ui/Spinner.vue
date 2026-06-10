<template>
  <span :class="cn(spinnerVariants({ size, tone }), props.class)" role="status">
    <span class="sr-only">{{ label }}</span>
  </span>
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { cn } from '@/lib/utils'
import { spinnerVariants, type SpinnerVariants } from './spinner-variants'

interface Props {
  size?: NonNullable<SpinnerVariants['size']>
  tone?: NonNullable<SpinnerVariants['tone']>
  label?: string
  class?: HTMLAttributes['class']
}

const props = withDefaults(defineProps<Props>(), { size: 'md', tone: 'signature', label: 'Loading' })
</script>

<style scoped>
.ae-spinner { position: relative; }
.ae-spinner::before,
.ae-spinner::after {
  content: '';
  position: absolute;
  border-radius: 9999px;
  border-style: solid;
  border-color: transparent;
  box-sizing: border-box;
}
.ae-spinner::before { inset: 0; animation: ae-spin 0.8s linear infinite; }
.ae-spinner::after { animation: ae-spin-rev 1.1s linear infinite; }

.ae-spinner--signature::before { border-top-color: var(--brand-cyan); border-bottom-color: var(--brand-cyan); }
.ae-spinner--signature::after { border-left-color: var(--brand-pink); border-right-color: var(--brand-pink); }

.ae-spinner--mono::before { border-top-color: currentColor; border-bottom-color: currentColor; }
.ae-spinner--mono::after { border-left-color: currentColor; border-right-color: currentColor; opacity: 0.55; }

.ae-spinner--xs { width: 14px; height: 14px; }
.ae-spinner--xs::before { border-width: 2px; }
.ae-spinner--xs::after { inset: 3px; border-width: 2px; }
.ae-spinner--sm { width: 18px; height: 18px; }
.ae-spinner--sm::before { border-width: 2px; }
.ae-spinner--sm::after { inset: 4px; border-width: 2px; }
.ae-spinner--md { width: 24px; height: 24px; }
.ae-spinner--md::before { border-width: 3px; }
.ae-spinner--md::after { inset: 5px; border-width: 3px; }
.ae-spinner--lg { width: 36px; height: 36px; }
.ae-spinner--lg::before { border-width: 3px; }
.ae-spinner--lg::after { inset: 7px; border-width: 3px; }
.ae-spinner--xl { width: 52px; height: 52px; }
.ae-spinner--xl::before { border-width: 4px; }
.ae-spinner--xl::after { inset: 10px; border-width: 4px; }

@keyframes ae-spin { to { transform: rotate(360deg); } }
@keyframes ae-spin-rev { to { transform: rotate(-360deg); } }

@media (prefers-reduced-motion: reduce) {
  .ae-spinner::before, .ae-spinner::after { animation-duration: 2.4s; }
}
</style>
