<template>
  <details
    :open="open"
    class="border-b border-white/10 py-3 group"
    @toggle="onToggle"
  >
    <summary
      class="flex items-center justify-between text-sm font-medium text-white/80 cursor-pointer select-none list-none px-1 hover:text-white"
    >
      <span class="flex items-center gap-2">
        <slot name="label">{{ label }}</slot>
        <span
          v-if="count"
          class="inline-flex items-center justify-center min-w-[1.25rem] h-5 px-1.5 rounded-full bg-cyan-500/20 text-cyan-300 text-[10px] font-semibold"
        >{{ count }}</span>
      </span>
      <ChevronDown
        class="size-4 text-white/40 transition-transform duration-150 group-open:rotate-180"
        aria-hidden="true"
      />
    </summary>
    <div class="pt-3 px-1 space-y-2">
      <slot />
    </div>
  </details>
</template>

<script setup lang="ts">
// Phase 15 (UX-31) — collapsible <details>-based section wrapper. The
// browser owns the open/close state via the native <details> element, so
// keyboard interaction (Enter/Space) and screen reader semantics are
// inherited for free — no manual ARIA expanded wiring needed.
import { ChevronDown } from 'lucide-vue-next'

interface Props {
  label?: string
  open?: boolean
  count?: number
}
withDefaults(defineProps<Props>(), { open: true, count: 0 })

const emit = defineEmits<{ (e: 'toggle', open: boolean): void }>()

function onToggle(ev: Event) {
  emit('toggle', (ev.target as HTMLDetailsElement).open)
}
</script>
