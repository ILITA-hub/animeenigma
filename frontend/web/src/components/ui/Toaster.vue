<template>
  <div
    aria-live="polite"
    aria-atomic="true"
    class="fixed top-20 right-4 z-50 flex flex-col gap-2 pointer-events-none max-w-sm w-[calc(100%-2rem)] sm:w-auto toaster-safe"
  >
    <TransitionGroup name="toast">
      <div
        v-for="toast in toasts"
        :key="toast.id"
        role="alert"
        :class="toastClasses(toast.type)"
      >
        <span class="flex-1 text-sm">{{ toast.message }}</span>
        <button
          type="button"
          class="text-white/80 hover:text-white text-lg leading-none flex-shrink-0"
          :aria-label="$t('system.statusBanner.dismiss')"
          @click="dismiss(toast.id)"
        >
          ×
        </button>
      </div>
    </TransitionGroup>
  </div>
</template>

<script setup lang="ts">
import { useToast, type ToastType } from '@/composables/useToast'

const { toasts, dismiss } = useToast()

function toastClasses(type: ToastType): string {
  const base =
    'pointer-events-auto flex items-start gap-3 px-4 py-3 rounded-lg shadow-lg text-white'
  switch (type) {
    case 'error':
      return `${base} bg-destructive/90`
    case 'success':
      return `${base} bg-success/90`
    case 'info':
    default:
      return `${base} bg-cyan-500/90`
  }
}
</script>

<style scoped>
/* Clears the header on notch/Dynamic-Island phones (viewport-fit=cover):
   the header sits --safe-top lower there, so the toast stack must too.
   0px on cutout-less devices — identical to plain top-20. */
.toaster-safe {
  top: calc(var(--safe-top) + 5rem);
}

.toast-enter-active,
.toast-leave-active {
  transition: opacity 200ms ease, transform 200ms ease;
}
.toast-enter-from {
  opacity: 0;
  transform: translateX(20px);
}
.toast-leave-to {
  opacity: 0;
  transform: translateX(20px);
}
</style>
