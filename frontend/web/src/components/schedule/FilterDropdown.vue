<!-- frontend/web/src/components/schedule/FilterDropdown.vue -->
<template>
  <div ref="root" class="relative">
    <button
      type="button"
      class="flex items-center gap-1.5 text-xs rounded-lg border px-2.5 py-1.5 whitespace-nowrap transition-colors"
      :class="selected.size ? 'bg-primary/15 border-primary/50 text-primary' : 'bg-white/[0.06] border-white/10 text-foreground/80 hover:bg-white/10'"
      @click.stop="open = !open"
    >
      {{ label }}<span class="opacity-50 text-[10px]">▾</span>
    </button>
    <div v-if="open" class="absolute top-[calc(100%+6px)] left-0 z-50 w-52 rounded-xl border border-white/10 bg-popover text-popover-foreground p-1.5 shadow-xl shadow-black/30">
      <button
        v-for="opt in options"
        :key="opt.value"
        type="button"
        class="flex w-full items-center gap-2 rounded-lg px-2 py-1.5 text-sm text-left hover:bg-white/5"
        @click.stop="toggle(opt.value)"
      >
        <span class="w-[15px] h-[15px] rounded border flex items-center justify-center text-[10px] flex-none"
          :class="selected.has(opt.value) ? 'bg-primary border-primary text-primary-foreground' : 'border-white/30'">
          {{ selected.has(opt.value) ? '✓' : '' }}
        </span>
        {{ opt.label }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onBeforeUnmount } from 'vue'

defineProps<{ label: string; options: { value: string; label: string }[]; selected: Set<string> }>()
const emit = defineEmits<{ toggle: [value: string] }>()
const open = ref(false)
const root = ref<HTMLElement | null>(null)

function toggle(v: string) { emit('toggle', v) }
function onDocClick(e: MouseEvent) { if (root.value && !root.value.contains(e.target as Node)) open.value = false }
onMounted(() => document.addEventListener('click', onDocClick))
onBeforeUnmount(() => document.removeEventListener('click', onDocClick))
</script>
