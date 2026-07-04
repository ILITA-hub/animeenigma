<template>
  <Teleport to="body">
    <!-- v-if is REQUIRED (iOS 26 Safari): a closed overlay must contribute
         NOTHING to the DOM, or Safari paints an opaque status-bar band —
         same constraint as ui/Modal.vue. `appear` because the host mounts us
         lazily with modelValue already true on the first poster tap. -->
    <Transition name="pl-fade" appear>
      <div
        v-if="modelValue"
        ref="rootRef"
        class="fixed inset-0 z-50"
        role="dialog"
        aria-modal="true"
        :aria-label="alt"
      >
        <div class="absolute inset-0 bg-black/90 backdrop-blur-sm" aria-hidden="true" />

        <!-- Gesture surface: pinch to zoom, drag to pan when zoomed,
             double-tap toggles zoom, single tap (unzoomed) closes.
             touch-none hands ALL touch gestures to the pointer handlers —
             without it the browser pans/zooms the page instead. -->
        <div
          ref="surfaceRef"
          class="absolute inset-0 flex items-center justify-center overflow-hidden touch-none select-none"
          @pointerdown="onPointerDown"
          @pointermove="onPointerMove"
          @pointerup="onPointerUp"
          @pointercancel="onPointerUp"
          @wheel.prevent="onWheel"
        >
          <img
            ref="imgRef"
            :src="imageSrc"
            :alt="alt"
            draggable="false"
            class="max-w-[calc(100vw-1.5rem)] max-h-[calc(100dvh-1.5rem)] object-contain rounded-lg shadow-2xl will-change-transform"
            :class="{ 'transition-transform duration-200 ease-out': !gestureActive }"
            :style="{ transform: `translate3d(${tx}px, ${ty}px, 0) scale(${scale})` }"
            @error="onError"
          />
        </div>

        <button
          type="button"
          class="absolute right-4 z-10 p-2.5 rounded-full bg-black/50 text-white/80 hover:text-white hover:bg-black/70 transition-colors"
          :style="{ top: 'calc(var(--safe-top) + 1rem)' }"
          :aria-label="$t('common.close')"
          @click="close"
        >
          <X class="size-6" aria-hidden="true" />
        </button>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
import { ref, watch, toRef, onScopeDispose } from 'vue'
import { clamp, onKeyStroke } from '@vueuse/core'
import { X } from 'lucide-vue-next'
import { useImageProxy } from '@/composables/useImageProxy'
import { useBodyScrollLock } from '@/composables/useBodyScrollLock'
import { useFocusTrap } from '@/composables/useFocusTrap'

const props = defineProps<{
  modelValue: boolean
  /** Original (unproxied) poster URL — served full-size, proxy on fallback. */
  src: string
  alt: string
}>()

const emit = defineEmits<{ 'update:modelValue': [value: boolean] }>()

const open = toRef(props, 'modelValue')
const rootRef = ref<HTMLElement | null>(null)
const surfaceRef = ref<HTMLElement | null>(null)
const imgRef = ref<HTMLImageElement | null>(null)

useBodyScrollLock(open)

// Restore focus to whatever opened us (the poster trigger button). Reset the
// view on open only — resetting on close would visibly snap a zoomed image
// back to scale 1 during the fade-out.
const returnFocusTo = ref<HTMLElement | null>(null)
watch(open, (isOpen) => {
  if (!isOpen) return
  returnFocusTo.value = document.activeElement as HTMLElement | null
  resetView()
})
useFocusTrap({ active: open, container: rootRef, returnFocusTo })

// Full-resolution source: original URL first (the page thumbnail already went
// through the resizing proxy), backend proxy as the error fallback.
const { imageSrc, onError } = useImageProxy(() => props.src)

// --- Zoom / pan state ---------------------------------------------------
const MIN_SCALE = 1
const MAX_SCALE = 4
const DOUBLE_TAP_SCALE = 2.5
const TAP_SLOP_PX = 8
const DOUBLE_TAP_SLOP_PX = 40
const DOUBLE_TAP_MS = 300

const scale = ref(1)
const tx = ref(0)
const ty = ref(0)

const pointers = new Map<number, { x: number; y: number }>()
const gestureActive = ref(false)

let pinchLastDist = 0
let pinchLastCenter = { x: 0, y: 0 }
let downPos = { x: 0, y: 0 }
let moved = false
let lastTapAt = 0
let lastTapPos = { x: 0, y: 0 }
let singleTapTimer: ReturnType<typeof setTimeout> | undefined

function resetView() {
  scale.value = 1
  tx.value = 0
  ty.value = 0
  pointers.clear()
  gestureActive.value = false
  moved = false
  lastTapAt = 0
  clearTimeout(singleTapTimer)
}

onScopeDispose(() => clearTimeout(singleTapTimer))

function close() {
  clearTimeout(singleTapTimer)
  emit('update:modelValue', false)
}

// Document-level Escape, NOT a template @keydown.escape: after a pointerdown
// on the (non-focusable) gesture surface focus moves to <body> and a subtree
// key binding would never fire again — same trap Navbar.vue documents.
onKeyStroke('Escape', (e) => {
  if (!open.value) return
  e.preventDefault()
  close()
})

/** Rescale keeping the image point under (px, py) fixed on screen. */
function zoomAt(px: number, py: number, nextScale: number) {
  const s = clamp(nextScale, MIN_SCALE, MAX_SCALE)
  // The surface is fixed inset-0, so its center IS the viewport center — no
  // getBoundingClientRect on the per-frame pinch/wheel path.
  const cx = window.innerWidth / 2
  const cy = window.innerHeight / 2
  const ux = (px - cx - tx.value) / scale.value
  const uy = (py - cy - ty.value) / scale.value
  tx.value = px - cx - ux * s
  ty.value = py - cy - uy * s
  scale.value = s
  if (s === MIN_SCALE) {
    tx.value = 0
    ty.value = 0
  }
}

/** Keep the (scaled) image from being dragged fully off-screen; recenter axes
 *  where it still fits inside the viewport. */
function clampPan() {
  const img = imgRef.value
  const surf = surfaceRef.value
  if (!img || !surf) return
  // getBoundingClientRect is post-transform (scale AND mid-gesture translate
  // baked in) — derive extents from the untransformed layout size instead.
  const maxX = Math.max(0, (img.offsetWidth * scale.value - surf.clientWidth) / 2)
  const maxY = Math.max(0, (img.offsetHeight * scale.value - surf.clientHeight) / 2)
  tx.value = clamp(tx.value, -maxX, maxX)
  ty.value = clamp(ty.value, -maxY, maxY)
}

function pinchGeometry(): { dist: number; center: { x: number; y: number } } {
  const [a, b] = [...pointers.values()]
  return {
    dist: Math.hypot(a.x - b.x, a.y - b.y),
    center: { x: (a.x + b.x) / 2, y: (a.y + b.y) / 2 },
  }
}

function onPointerDown(e: PointerEvent) {
  surfaceRef.value?.setPointerCapture(e.pointerId)
  pointers.set(e.pointerId, { x: e.clientX, y: e.clientY })
  gestureActive.value = true
  if (pointers.size === 1) {
    downPos = { x: e.clientX, y: e.clientY }
    moved = false
  } else if (pointers.size === 2) {
    const g = pinchGeometry()
    pinchLastDist = g.dist
    pinchLastCenter = g.center
    moved = true
  }
}

function onPointerMove(e: PointerEvent) {
  const prev = pointers.get(e.pointerId)
  if (!prev) return
  pointers.set(e.pointerId, { x: e.clientX, y: e.clientY })

  if (pointers.size === 2) {
    const { dist, center } = pinchGeometry()
    if (pinchLastDist > 0) {
      zoomAt(center.x, center.y, scale.value * (dist / pinchLastDist))
    }
    tx.value += center.x - pinchLastCenter.x
    ty.value += center.y - pinchLastCenter.y
    pinchLastDist = dist
    pinchLastCenter = center
    return
  }

  if (!moved && Math.hypot(e.clientX - downPos.x, e.clientY - downPos.y) > TAP_SLOP_PX) {
    moved = true
  }
  if (scale.value > 1) {
    tx.value += e.clientX - prev.x
    ty.value += e.clientY - prev.y
  }
}

function onPointerUp(e: PointerEvent) {
  if (!pointers.delete(e.pointerId)) return
  // Pinch ending with a finger still down needs no re-anchor: pans use the
  // per-pointer prev position and tap detection is already off (moved=true).
  if (pointers.size > 0) return

  gestureActive.value = false
  clampPan()

  if (moved || e.type === 'pointercancel') return

  const now = Date.now()
  const isDoubleTap =
    now - lastTapAt < DOUBLE_TAP_MS &&
    Math.hypot(e.clientX - lastTapPos.x, e.clientY - lastTapPos.y) < DOUBLE_TAP_SLOP_PX
  if (isDoubleTap) {
    clearTimeout(singleTapTimer)
    lastTapAt = 0
    zoomAt(e.clientX, e.clientY, scale.value > 1 ? MIN_SCALE : DOUBLE_TAP_SCALE)
    return
  }

  lastTapAt = now
  lastTapPos = { x: e.clientX, y: e.clientY }
  if (scale.value === 1) {
    // Delay so a second tap can upgrade this to double-tap-zoom.
    clearTimeout(singleTapTimer)
    singleTapTimer = setTimeout(close, DOUBLE_TAP_MS)
  }
}

function onWheel(e: WheelEvent) {
  zoomAt(e.clientX, e.clientY, scale.value * (e.deltaY < 0 ? 1.15 : 1 / 1.15))
  clampPan()
}
</script>

<style scoped>
.pl-fade-enter-active,
.pl-fade-leave-active {
  transition: opacity 0.2s ease;
}
.pl-fade-enter-from,
.pl-fade-leave-to {
  opacity: 0;
}
</style>
