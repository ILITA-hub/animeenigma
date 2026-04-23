<template>
  <button
    ref="btnEl"
    type="button"
    :class="[
      'absolute z-20 w-9 h-9 rounded-full',
      'bg-black/65 backdrop-blur flex items-center justify-center',
      'text-white opacity-0 scale-90',
      'group-hover:opacity-100 group-hover:scale-100',
      'group-hover:animate-kebab-glow',
      'focus-visible:opacity-100 focus-visible:scale-100 focus-visible:animate-kebab-glow',
      'transition-all duration-200',
      'hover:bg-cyan-500/90 hover:rotate-[12deg] hover:scale-110',
      'pointer-events-auto',
      positionClass,
      extraClass,
    ]"
    :aria-label="$t('contextMenu.openMenu')"
    aria-haspopup="menu"
    :aria-expanded="menuOpen"
    @click.prevent.stop="onActivate"
    @keydown.enter.prevent="onActivate"
    @keydown.space.prevent="onActivate"
  >
    <svg class="w-4 h-4" viewBox="0 0 20 20" fill="currentColor">
      <circle cx="10" cy="4" r="1.5" />
      <circle cx="10" cy="10" r="1.5" />
      <circle cx="10" cy="16" r="1.5" />
    </svg>
  </button>
</template>

<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'

const props = withDefaults(
  defineProps<{
    menuOpen?: boolean
    position?: 'top-right' | 'top-left' | 'bottom-right'
    extraClass?: string
  }>(),
  { menuOpen: false, position: 'top-right', extraClass: '' }
)

const emit = defineEmits<{ open: [el: HTMLElement] }>()

const btnEl = ref<HTMLButtonElement | null>(null)

const positionClass = computed(() => {
  switch (props.position) {
    case 'top-left': return 'top-2 left-2'
    case 'bottom-right': return 'bottom-2 right-2'
    case 'top-right':
    default: return 'top-2 right-2'
  }
})

function onActivate() {
  if (btnEl.value) emit('open', btnEl.value)
}

if (import.meta.env.DEV) {
  onMounted(() => {
    // Walk ancestors until we find a `.group` element. group-hover: matches
    // ANY ancestor with the class, not just the direct parent.
    let el: HTMLElement | null = btnEl.value?.parentElement ?? null
    let found = false
    while (el) {
      if (el.classList.contains('group')) { found = true; break }
      el = el.parentElement
    }
    if (!found) {
      console.warn(
        '[AnimeKebab] no ancestor element has the `group` Tailwind class — ' +
        'hover/focus reveal will not work. Add `class="group relative"` to the wrapper.'
      )
    }
  })
}
</script>
