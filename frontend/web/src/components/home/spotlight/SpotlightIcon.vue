<template>
  <!--
    Workstream hero-spotlight — v1.1-polish Phase 01 (HSB-V11-CC-03).

    Inline-SVG sprite for the 9 spotlight icons. Why inline rather than an
    icon library:
      - Adds <3 KB gzipped vs. ~30 KB for heroicons/lucide.
      - One pass through the template — Tailwind class forwarding works
        without a wrapper component.
      - Each path was hand-trimmed to a uniform 24×24 viewBox with stroke-
        based geometry so accent color flows from `currentColor`.

    `class` is forwarded onto the SVG root element so callers can size +
    color the icon via Tailwind utilities (e.g. `class="w-4 h-4 text-cyan-300"`).
  -->
  <svg
    v-if="name === 'telegram'"
    :class="forwardedClass"
    viewBox="0 0 24 24"
    fill="currentColor"
    :aria-hidden="ariaHidden"
    :aria-label="forwardedAriaLabel"
    :role="role"
  >
    <path d="M21.2 4.3 2.9 11.4c-1.2.5-1.2 1.8 0 2.2l4.7 1.5 1.8 5.8c.2.6.9.8 1.4.4l2.6-2.2 5.1 3.8c.9.7 2.3.2 2.5-1l3.1-15.6c.3-1.4-1-2.5-2.4-2zm-3.7 4.5L9.8 16l-.3 4.2 5-4.5z" />
  </svg>

  <svg
    v-else-if="name === 'sparkles'"
    :class="forwardedClass"
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    stroke-width="1.8"
    stroke-linecap="round"
    stroke-linejoin="round"
    :aria-hidden="ariaHidden"
    :aria-label="forwardedAriaLabel"
    :role="role"
  >
    <path d="M12 3v3M12 18v3M5.4 5.4l2.1 2.1M16.5 16.5l2.1 2.1M3 12h3M18 12h3M5.4 18.6l2.1-2.1M16.5 7.5l2.1-2.1" />
    <circle cx="12" cy="12" r="2.5" fill="currentColor" stroke="none" />
  </svg>

  <svg
    v-else-if="name === 'chart'"
    :class="forwardedClass"
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    stroke-width="1.8"
    stroke-linecap="round"
    stroke-linejoin="round"
    :aria-hidden="ariaHidden"
    :aria-label="forwardedAriaLabel"
    :role="role"
  >
    <path d="M3 20h18M6 20V10M11 20V4M16 20v-7M21 20v-4" />
  </svg>

  <svg
    v-else-if="name === 'pulse'"
    :class="forwardedClass"
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    stroke-width="1.8"
    stroke-linecap="round"
    stroke-linejoin="round"
    :aria-hidden="ariaHidden"
    :aria-label="forwardedAriaLabel"
    :role="role"
  >
    <path d="M3 12h4l2-7 4 14 2-7h6" />
  </svg>

  <svg
    v-else-if="name === 'clock'"
    :class="forwardedClass"
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    stroke-width="1.8"
    stroke-linecap="round"
    stroke-linejoin="round"
    :aria-hidden="ariaHidden"
    :aria-label="forwardedAriaLabel"
    :role="role"
  >
    <circle cx="12" cy="12" r="9" />
    <path d="M12 7v5l3 2" />
  </svg>

  <svg
    v-else-if="name === 'play'"
    :class="forwardedClass"
    viewBox="0 0 24 24"
    fill="currentColor"
    :aria-hidden="ariaHidden"
    :aria-label="forwardedAriaLabel"
    :role="role"
  >
    <path d="M8 5v14l11-7z" />
  </svg>

  <svg
    v-else-if="name === 'shuffle'"
    :class="forwardedClass"
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    stroke-width="1.8"
    stroke-linecap="round"
    stroke-linejoin="round"
    :aria-hidden="ariaHidden"
    :aria-label="forwardedAriaLabel"
    :role="role"
  >
    <path d="M3 6h3l4 5-4 5H3M21 6h-4l-9 12H4M16 3l5 3-5 3M16 15l5 3-5 3" />
  </svg>

  <svg
    v-else-if="name === 'wrench'"
    :class="forwardedClass"
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    stroke-width="1.8"
    stroke-linecap="round"
    stroke-linejoin="round"
    :aria-hidden="ariaHidden"
    :aria-label="forwardedAriaLabel"
    :role="role"
  >
    <path d="M14 7a4 4 0 1 1 3.6 3.96L9 19.5a2.12 2.12 0 0 1-3-3l8.54-8.55A4 4 0 0 1 14 7z" />
  </svg>

  <svg
    v-else-if="name === 'lightning'"
    :class="forwardedClass"
    viewBox="0 0 24 24"
    fill="currentColor"
    :aria-hidden="ariaHidden"
    :aria-label="forwardedAriaLabel"
    :role="role"
  >
    <path d="M13 2 3 14h7l-1 8 10-12h-7z" />
  </svg>

  <svg
    v-else-if="name === 'star'"
    :class="forwardedClass"
    viewBox="0 0 24 24"
    fill="currentColor"
    :aria-hidden="ariaHidden"
    :aria-label="forwardedAriaLabel"
    :role="role"
  >
    <path d="M12 2l2.9 6.1 6.6.9-4.8 4.6 1.2 6.6L12 17.8 6 21l1.2-6.6L2.4 9.8l6.6-.9z" />
  </svg>

  <svg
    v-else-if="name === 'quote'"
    :class="forwardedClass"
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    stroke-width="1.8"
    stroke-linecap="round"
    stroke-linejoin="round"
    :aria-hidden="ariaHidden"
    :aria-label="forwardedAriaLabel"
    :role="role"
  >
    <path d="M7 17H4a2 2 0 0 1-2-2v-3a5 5 0 0 1 5-5h1v4H7a1 1 0 0 0-1 1v1h3v2a2 2 0 0 1-2 2ZM19 17h-3a2 2 0 0 1-2-2v-3a5 5 0 0 1 5-5h1v4h-1a1 1 0 0 0-1 1v1h3v2a2 2 0 0 1-2 2Z" />
  </svg>
</template>

<script setup lang="ts">
import { computed, useAttrs } from 'vue'
import type { SpotlightIconName } from './tokens'

defineProps<{
  name: SpotlightIconName
}>()

// We forward $attrs.class + $attrs['aria-label'] manually onto the SVG root.
// Disable default fallthrough so an extra wrapper isn't generated and so
// `class` doesn't leak twice in a single-root branch.
//
// aria-label handling (v1.1-polish Phase 06): when callers pass aria-label,
// the icon becomes a semantically labeled `<svg role="img">` — screen
// readers announce the label and the default aria-hidden is dropped.
// Default (no aria-label) keeps aria-hidden="true" so the icon stays
// decorative — this matches CarouselControls.vue (label lives on the
// wrapping <button>).
defineOptions({ inheritAttrs: false })

const attrs = useAttrs()
const forwardedClass = computed<string | undefined>(() =>
  typeof attrs.class === 'string' ? attrs.class : undefined,
)
const forwardedAriaLabel = computed<string | undefined>(() => {
  const v = attrs['aria-label']
  return typeof v === 'string' && v.length > 0 ? v : undefined
})
const isLabeled = computed<boolean>(() => forwardedAriaLabel.value !== undefined)
// When labeled, drop the per-template aria-hidden="true" attribute (we cannot
// remove an inline attribute conditionally in the template without a v-bind
// override, so each SVG binds :aria-hidden="ariaHidden" and :role="role"
// computed values).
const ariaHidden = computed<'true' | undefined>(() =>
  isLabeled.value ? undefined : 'true',
)
const role = computed<'img' | undefined>(() =>
  isLabeled.value ? 'img' : undefined,
)
</script>
