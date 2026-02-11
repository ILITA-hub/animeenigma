<template>
  <Teleport to="body">
    <Transition name="modal">
      <div
        v-if="modelValue"
        class="fixed inset-0 z-50 flex items-center justify-center p-4"
        @keydown.esc="handleClose"
      >
        <!-- Backdrop -->
        <div
          class="absolute inset-0 bg-black/60 backdrop-blur-sm"
          @click="closeOnBackdrop && handleClose()"
        />

        <!-- Modal Content -->
        <div
          ref="modalRef"
          :class="modalClasses"
          role="dialog"
          aria-modal="true"
          :aria-labelledby="titleId"
          tabindex="-1"
        >
          <!-- Header -->
          <div v-if="title || $slots.header || closable" class="flex items-center justify-between mb-4">
            <slot name="header">
              <h2 v-if="title" :id="titleId" class="text-xl font-semibold text-white">
                {{ title }}
              </h2>
            </slot>
            <button
              v-if="closable"
              type="button"
              class="p-2 text-white/50 hover:text-white hover:bg-white/10 rounded-lg transition-colors"
              aria-label="Close"
              @click="handleClose"
            >
              <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
              </svg>
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
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
import { computed, ref, watch, onMounted, onUnmounted } from 'vue'

interface Props {
  modelValue: boolean
  title?: string
  size?: 'sm' | 'md' | 'lg' | 'xl' | 'full'
  closable?: boolean
  closeOnBackdrop?: boolean
  closeOnEsc?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  size: 'md',
  closable: true,
  closeOnBackdrop: true,
  closeOnEsc: true,
})

const emit = defineEmits<{
  'update:modelValue': [value: boolean]
  'close': []
}>()

const modalRef = ref<HTMLElement | null>(null)
const titleId = `modal-title-${Math.random().toString(36).slice(2, 9)}`

const modalClasses = computed(() => {
  const base = 'relative glass-elevated rounded-2xl p-4 sm:p-6 w-full max-h-[90vh] overflow-y-auto'

  const sizes = {
    sm: 'max-w-sm',
    md: 'max-w-md',
    lg: 'max-w-lg',
    xl: 'max-w-xl',
    full: 'max-w-[calc(100vw-2rem)] max-h-[calc(100vh-2rem)]',
  }

  return [base, sizes[props.size]].join(' ')
})

const handleClose = () => {
  if (props.closeOnEsc || props.closable) {
    emit('update:modelValue', false)
    emit('close')
  }
}

// Lock body scroll when modal is open
watch(() => props.modelValue, (isOpen) => {
  if (isOpen) {
    document.body.style.overflow = 'hidden'
    // Focus the modal for keyboard navigation
    setTimeout(() => modalRef.value?.focus(), 0)
  } else {
    document.body.style.overflow = ''
  }
})

// Handle escape key globally
const handleKeydown = (e: KeyboardEvent) => {
  if (e.key === 'Escape' && props.modelValue && props.closeOnEsc) {
    handleClose()
  }
}

onMounted(() => {
  document.addEventListener('keydown', handleKeydown)
})

onUnmounted(() => {
  document.removeEventListener('keydown', handleKeydown)
  document.body.style.overflow = ''
})
</script>

<style scoped>
.modal-enter-active,
.modal-leave-active {
  transition: opacity 0.2s ease;
}

.modal-enter-active > div:last-child,
.modal-leave-active > div:last-child {
  transition: transform 0.2s ease, opacity 0.2s ease;
}

.modal-enter-from,
.modal-leave-to {
  opacity: 0;
}

.modal-enter-from > div:last-child,
.modal-leave-to > div:last-child {
  transform: scale(0.95);
  opacity: 0;
}
</style>
