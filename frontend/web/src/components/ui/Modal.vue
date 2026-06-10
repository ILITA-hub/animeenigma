<template>
  <DialogRoot :open="modelValue" :modal="modal" @update:open="onOpenUpdate">
    <DialogPortal>
      <DialogOverlay
        class="fixed inset-0 z-50 bg-black/60 backdrop-blur-sm transition-opacity duration-200 data-[state=open]:opacity-100 data-[state=closed]:opacity-0"
      />
      <!-- Fixed flex wrapper reproduces the legacy centered layout. The wrapper
           itself is non-interactive (pointer-events-none) so Reka's
           pointer-down-outside still fires on the overlay; DialogContent re-enables
           pointer events on itself. -->
      <div class="fixed inset-0 z-50 flex items-center justify-center p-4 pointer-events-none">
        <DialogContent
          :class="modalClasses"
          :aria-labelledby="title ? titleId : undefined"
          @escape-key-down="onEscapeKeyDown"
          @pointer-down-outside="onPointerDownOutside"
          @interact-outside="onInteractOutside"
        >
          <!-- Header -->
          <div v-if="title || $slots.header || closable" class="flex items-center justify-between mb-4">
            <slot name="header">
              <DialogTitle v-if="title" :id="titleId" class="text-xl font-semibold text-white">
                {{ title }}
              </DialogTitle>
            </slot>
            <button
              v-if="closable"
              type="button"
              class="p-2 text-white/50 hover:text-white hover:bg-white/10 rounded-lg transition-colors"
              aria-label="Close"
              @click="handleClose"
            >
              <X class="size-5" aria-hidden="true" />
            </button>
          </div>

          <!-- Body -->
          <div class="text-white/80">
            <slot />
          </div>

          <!-- Footer -->
          <div v-if="$slots.footer" class="mt-6 flex justify-end gap-3">
            <slot name="footer" />
          </div>
        </DialogContent>
      </div>
    </DialogPortal>
  </DialogRoot>
</template>

<script setup lang="ts">
import { computed, toRef } from 'vue'
import { X } from 'lucide-vue-next'
import {
  DialogRoot,
  DialogPortal,
  DialogOverlay,
  DialogContent,
  DialogTitle,
} from 'reka-ui'
import { cn } from '@/lib/utils'
import { useBodyScrollLock } from '@/composables/useBodyScrollLock'

interface Props {
  modelValue: boolean
  title?: string
  size?: 'sm' | 'md' | 'lg' | 'xl' | 'full'
  closable?: boolean
  closeOnBackdrop?: boolean
  closeOnEsc?: boolean
  // modal=false (the DEFAULT) makes Reka use the NON-modal DialogContent.
  // Reka 2.9.8's modal mode both leaks `body { pointer-events: none }` on
  // close (its restore is gated on `size === 1`, but Vue runs the layer-delete
  // cleanup before the restore cleanup) AND fights our refcounted
  // useBodyScrollLock over `body { overflow }`: reka's own scroll lock
  // captures our already-applied 'hidden' as the value to restore, so the
  // page stays unscrollable after the first close (reported on the footer
  // feedback modal, 2026-06-10). Non-modal keeps exactly one scroll-lock
  // owner — ours. Pass modal=true only if a consumer truly needs reka's
  // modal machinery and has verified the leak is gone (reka upgrade).
  modal?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  size: 'md',
  closable: true,
  closeOnBackdrop: true,
  closeOnEsc: true,
  modal: false,
})

const emit = defineEmits<{
  'update:modelValue': [value: boolean]
  'close': []
}>()

const titleId = `modal-title-${Math.random().toString(36).slice(2, 9)}`

const modalClasses = computed(() => {
  // pointer-events-auto re-enables interaction on the content (the wrapper is
  // pointer-events-none so outside-clicks still register on the overlay).
  // data-[state] utilities replicate the legacy .modal-* scale+fade transition
  // (visually-equivalent; pixel-parity verified in the in-browser gate).
  const base = 'relative glass-elevated rounded-2xl p-4 sm:p-6 w-full max-h-[90vh] overflow-y-auto pointer-events-auto transition-all duration-200 data-[state=open]:opacity-100 data-[state=open]:scale-100 data-[state=closed]:opacity-0 data-[state=closed]:scale-95'

  const sizes = {
    sm: 'max-w-sm',
    md: 'max-w-md',
    lg: 'max-w-lg',
    xl: 'max-w-xl',
    full: 'max-w-[calc(100vw-2rem)] max-h-[calc(100vh-2rem)]',
  }

  return cn(base, sizes[props.size])
})

// modelValue <-> open bridge: emit update:modelValue always, emit close on close.
const onOpenUpdate = (value: boolean) => {
  emit('update:modelValue', value)
  if (!value) emit('close')
}

// Legacy close button path — mirrors the previous handleClose() semantics.
const handleClose = () => {
  emit('update:modelValue', false)
  emit('close')
}

// closeOnEsc=false opts out of Reka's escape close by preventing the default.
const onEscapeKeyDown = (e: Event) => {
  if (!props.closeOnEsc) e.preventDefault()
}

// closeOnBackdrop=false opts out of outside-click close.
const onPointerDownOutside = (e: Event) => {
  if (!props.closeOnBackdrop) e.preventDefault()
}
const onInteractOutside = (e: Event) => {
  if (!props.closeOnBackdrop) e.preventDefault()
}

// Refcount-based body scroll lock — cooperates with other consumers
// (e.g. Navbar mobile drawer) instead of stomping their state. Reka's Dialog
// does NOT auto scroll-lock (RESEARCH Pitfall 7), so we keep this EXACTLY.
useBodyScrollLock(toRef(props, 'modelValue'))

// Exposed so the co-located spec can drive the bridge handlers without relying
// on portaled/focus-trapped DOM that jsdom cannot fully simulate.
defineExpose({ onOpenUpdate, onEscapeKeyDown, onPointerDownOutside })
</script>
