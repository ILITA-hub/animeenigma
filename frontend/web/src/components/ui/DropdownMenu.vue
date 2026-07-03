<template>
  <DropdownMenuRoot :open="open" @update:open="onOpenUpdate">
    <!-- Trigger modes:
         1. #trigger slot  → normal trigger-anchored menu (keyboard/click open).
         2. anchorPoint    → an invisible zero-size trigger positioned at the
            given viewport point. Reka anchors the popper to a REAL trigger
            element (the only reliable anchor — a bare `reference` prop on
            Content is NOT honored by reka-ui@2.9.8's DropdownMenuContent, which
            left the menu unanchored at 0,0). `open` stays controlled; the
            invisible trigger is never interacted with — it exists only as the
            popper anchor at the kebab/touch coordinates. -->
    <DropdownMenuTrigger v-if="$slots.trigger" as-child>
      <slot name="trigger" />
    </DropdownMenuTrigger>
    <!-- `&& open` is REQUIRED: while closed, the anchor span would sit as a
         permanently-mounted position:fixed element at the last anchor point —
         (0,0) before any open — touching the viewport top edge. iOS 26 Safari
         samples such fixed elements to pick its status-bar treatment and
         paints an opaque band over the Dynamic Island zone (same constraint
         as Modal.vue's wrapper). Mounting in the same flush as open is fine:
         the trigger registers on the root context before the portaled
         Content's popper resolves its anchor. -->
    <DropdownMenuTrigger v-else-if="anchorPoint && open" as-child>
      <span aria-hidden="true" :style="anchorStyle" />
    </DropdownMenuTrigger>

    <DropdownMenuPortal>
      <DropdownMenuContent
        :align="align"
        :side="side"
        :side-offset="sideOffset"
        :class="contentClasses"
        @open-auto-focus="onOpenAutoFocus"
        @pointer-down-outside="onAutoDismiss"
        @focus-outside="onAutoDismiss"
        @interact-outside="onAutoDismiss"
      >
        <slot />
      </DropdownMenuContent>
    </DropdownMenuPortal>
  </DropdownMenuRoot>
</template>

<script setup lang="ts">
import { computed, watch } from 'vue'
import {
  DropdownMenuRoot,
  DropdownMenuTrigger,
  DropdownMenuPortal,
  DropdownMenuContent,
  type ReferenceElement,
} from 'reka-ui'
import { cn } from '@/lib/utils'

interface Props {
  /** Controlled open state. Pair with @update:open for v-model:open. */
  open?: boolean
  /**
   * @deprecated reka-ui@2.9.8's DropdownMenuContent does NOT honor a bare
   * `reference` prop (menu rendered unanchored at 0,0). Use `anchorPoint`
   * instead — it renders an invisible zero-size DropdownMenuTrigger at the
   * point, which IS a real popper anchor. Kept only for type compat.
   */
  reference?: ReferenceElement
  /**
   * Anchored mode: viewport coordinates to anchor the menu at (e.g. the
   * anime-card kebab's rect, or a touch point). Renders an invisible trigger
   * there so Reka's popper anchors correctly without a visible trigger child.
   */
  anchorPoint?: { x: number; y: number } | null
  align?: 'start' | 'center' | 'end'
  side?: 'top' | 'right' | 'bottom' | 'left'
  sideOffset?: number
  class?: string
}

const props = withDefaults(defineProps<Props>(), {
  align: 'start',
  side: 'bottom',
  sideOffset: 4,
})

// Invisible-anchor style for anchorPoint mode — a fixed zero-size box at the
// viewport coordinates; the menu (side/align/offset) positions relative to it.
const anchorStyle = computed(() =>
  props.anchorPoint
    ? {
        position: 'fixed' as const,
        left: `${props.anchorPoint.x}px`,
        top: `${props.anchorPoint.y}px`,
        width: '0px',
        height: '0px',
        pointerEvents: 'none' as const,
      }
    : {},
)

const emit = defineEmits<{
  'update:open': [value: boolean]
}>()

// Token-driven content surface. data-[state] utilities mirror the bespoke
// ContextMenu fade+scale transition so Plan 04's kebab rebuild gets parity.
const contentClasses = computed(() =>
  cn(
    'z-[9999] min-w-[220px] max-w-[320px] bg-popover text-popover-foreground border border-white/10 rounded-xl shadow-xl shadow-black/30 p-1 overflow-hidden',
    // Fade+scale transition for parity with the bespoke ContextMenu. Uses real
    // utilities (no tailwindcss-animate plugin in this project) — same approach
    // as Modal.vue's data-[state] transition.
    'origin-[--reka-dropdown-menu-content-transform-origin] transition-all duration-150 data-[state=open]:opacity-100 data-[state=open]:scale-100 data-[state=closed]:opacity-0 data-[state=closed]:scale-95',
    props.class,
  ),
)

// Bridge Reka's open-change back onto the controlled v-model:open contract.
const onOpenUpdate = (value: boolean) => {
  emit('update:open', value)
}

// --- Anchored-mode dismiss guard ---------------------------------------
// In anchored mode (external kebab trigger, no Reka <DropdownMenuTrigger>), the
// menu is opened by setting `open` from an outside button. Immediately after
// open, Reka's content sees focus still on the kebab (focus-outside) and any
// hover-reveal layout-shift scroll, which would dismiss the just-opened menu.
// Ignore auto-dismiss within a short grace window after open; genuine
// outside interactions after that still close the menu normally.
let openedAt = 0
watch(
  () => props.open,
  (o) => {
    if (o) openedAt = typeof performance !== 'undefined' ? performance.now() : 0
  },
)

function onOpenAutoFocus(_e: Event) {
  // Keep Reka's default (focus first item) so focus lands inside the content —
  // do NOT preventDefault, or focus stays on the kebab and focus-outside fires.
}

function onAutoDismiss(e: Event) {
  const since = (typeof performance !== 'undefined' ? performance.now() : 0) - openedAt
  if (since < 300) {
    e.preventDefault()
    return
  }
  // Also ignore dismisses originating on the reference element itself (the kebab).
  const refEl =
    props.reference && typeof (props.reference as HTMLElement).contains === 'function'
      ? (props.reference as HTMLElement)
      : null
  const orig = (e as CustomEvent).detail?.originalEvent as Event | undefined
  const target = (orig && (orig.target as Node | null)) || null
  if (refEl && target && refEl.contains(target)) e.preventDefault()
}

// Exposed so the co-located spec can drive the open bridge + assert the
// token-driven content classes without depending on portaled DOM.
defineExpose({ onOpenUpdate, contentClasses })
</script>
