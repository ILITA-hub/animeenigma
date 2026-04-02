<template>
  <Teleport to="body">
    <Transition name="context-menu">
      <div
        v-if="visible"
        ref="menuRef"
        class="fixed z-[9999] min-w-[240px] max-w-[320px] bg-slate-900/95 backdrop-blur-xl border border-white/10 rounded-xl shadow-xl shadow-black/30 overflow-hidden"
        :style="{ left: `${adjustedX}px`, top: `${adjustedY}px` }"
        @click.stop
      >
        <slot />
      </div>
    </Transition>

    <!-- Backdrop (invisible click catcher) -->
    <div
      v-if="visible"
      class="fixed inset-0 z-[9998]"
      @click="$emit('update:visible', false)"
      @contextmenu.prevent="$emit('update:visible', false)"
    />
  </Teleport>
</template>

<script setup lang="ts">
import { ref, watch, nextTick, onMounted, onUnmounted } from 'vue'

const props = defineProps<{
  visible: boolean
  x: number
  y: number
}>()

const emit = defineEmits<{
  'update:visible': [value: boolean]
}>()

const menuRef = ref<HTMLElement | null>(null)
const adjustedX = ref(0)
const adjustedY = ref(0)

// Viewport clamping after render
watch(() => props.visible, async (isVisible) => {
  if (isVisible) {
    adjustedX.value = props.x
    adjustedY.value = props.y
    await nextTick()
    clampToViewport()
  }
})

watch(() => [props.x, props.y], () => {
  adjustedX.value = props.x
  adjustedY.value = props.y
  if (props.visible) {
    nextTick(clampToViewport)
  }
})

function clampToViewport() {
  if (!menuRef.value) return
  const rect = menuRef.value.getBoundingClientRect()
  const vw = window.innerWidth
  const vh = window.innerHeight
  const pad = 8

  if (rect.right > vw - pad) {
    adjustedX.value = vw - rect.width - pad
  }
  if (rect.bottom > vh - pad) {
    adjustedY.value = vh - rect.height - pad
  }
  if (adjustedX.value < pad) adjustedX.value = pad
  if (adjustedY.value < pad) adjustedY.value = pad
}

// Close on Escape
function handleKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape' && props.visible) {
    emit('update:visible', false)
  }
}

onMounted(() => {
  document.addEventListener('keydown', handleKeydown)
})

onUnmounted(() => {
  document.removeEventListener('keydown', handleKeydown)
})
</script>

<style scoped>
.context-menu-enter-active {
  transition: opacity 0.12s ease, transform 0.12s ease;
}
.context-menu-leave-active {
  transition: opacity 0.08s ease, transform 0.08s ease;
}
.context-menu-enter-from {
  opacity: 0;
  transform: scale(0.95);
}
.context-menu-leave-to {
  opacity: 0;
  transform: scale(0.95);
}
</style>
