<!--
  PlayerIconButton — the icon-control button used across player surfaces
  (control bar, floating menus). Extracts the former `.pl-icon` scoped style
  into a reusable primitive so every player surface shares ONE button language
  instead of re-declaring transparent/hover/active CSS.

  Visual language (was `.pl-icon`):
    - 40×40 (md) / 32×32 (sm), grid-centered, rounded (--r-md), transparent.
    - white icon; hover → bg white/14.
    - `active` (was `.is-open`) → cyan tint + cyan icon (menu-open highlight).

  Icon-only by contract: pass the lucide/SVG icon in the default slot and an
  `aria-label` (forwarded to the root <button> via attribute fallthrough, along
  with @click, data-test, etc.). For text/pill controls (e.g. the source pill)
  keep a bespoke button — this primitive is deliberately icon-only.
-->

<script setup lang="ts">
import { cva, type VariantProps } from 'class-variance-authority'
import { type HTMLAttributes } from 'vue'
import { cn } from '@/lib/utils'

const playerIconButtonVariants = cva(
  [
    'grid place-items-center shrink-0 cursor-pointer border-0 bg-transparent text-white',
    'rounded-[var(--r-md)] transition-colors duration-150',
    'hover:bg-white/[0.14]',
    'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-cyan/60',
    // active = the former `.pl-icon.is-open` menu-open highlight
    'data-[active=true]:bg-brand-cyan/20 data-[active=true]:text-brand-cyan',
  ],
  {
    variants: {
      size: {
        sm: 'size-8',
        md: 'size-10',
      },
    },
    defaultVariants: { size: 'md' },
  },
)

type Variants = VariantProps<typeof playerIconButtonVariants>

const props = withDefaults(
  defineProps<{
    active?: boolean
    size?: Variants['size']
    class?: HTMLAttributes['class']
  }>(),
  { active: false, size: 'md' },
)
</script>

<template>
  <button
    type="button"
    :data-active="props.active"
    :class="cn(playerIconButtonVariants({ size: props.size }), props.class)"
  >
    <slot />
  </button>
</template>
