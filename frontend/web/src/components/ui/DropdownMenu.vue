<template>
  <DropdownMenuRoot :open="open" @update:open="onOpenUpdate">
    <!-- #trigger is OPTIONAL: in anchored mode (Plan 04 kebab) the menu is
         positioned via the `reference` virtual-element prop instead of a literal
         trigger child. When a trigger slot IS provided it wraps in the Reka
         DropdownMenuTrigger so keyboard/click open semantics work normally. -->
    <DropdownMenuTrigger v-if="$slots.trigger" as-child>
      <slot name="trigger" />
    </DropdownMenuTrigger>

    <DropdownMenuPortal>
      <!--
        Plan 04 anchored-mode usage:
          <DropdownMenu :open="open" :reference="virtualEl" @update:open="...">
            <DropdownMenuItem .../>
          </DropdownMenu>
        `reference` is forwarded to DropdownMenuContent's :reference (Reka
        PopperContent virtual-element anchor). With a reference set, no #trigger
        child is required — the menu anchors to the supplied bounding-rect source
        (e.g. the anime-card kebab button).
      -->
      <DropdownMenuContent
        :reference="reference"
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
   * Anchored mode: a virtual element / HTMLElement bounding-rect source
   * forwarded to DropdownMenuContent's :reference. Lets Plan 04 anchor the menu
   * to the anime-card kebab WITHOUT a literal trigger child.
   */
  reference?: ReferenceElement
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
