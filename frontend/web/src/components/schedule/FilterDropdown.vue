<!-- frontend/web/src/components/schedule/FilterDropdown.vue -->
<template>
  <Popover v-model:open="open" align="start" :side-offset="6" class="w-52 p-1.5">
    <template #trigger>
      <button
        type="button"
        class="flex items-center gap-1.5 text-xs rounded-lg border px-2.5 py-1.5 whitespace-nowrap transition-colors"
        :class="selected.size ? 'bg-primary/15 border-primary/50 text-primary' : 'bg-white/[0.06] border-white/10 text-foreground/80 hover:bg-white/10'"
      >
        {{ label }}<span class="opacity-50 text-[10px]">▾</span>
      </button>
    </template>

    <input
      v-if="searchable"
      ref="searchEl"
      v-model="query"
      :placeholder="searchPlaceholder"
      class="w-full mb-1.5 rounded-lg bg-white/[0.06] px-2.5 py-1.5 text-sm text-foreground outline-none placeholder:text-muted-foreground"
    />
    <div class="max-h-60 overflow-y-auto">
      <button
        v-for="opt in visibleOptions"
        :key="opt.value"
        type="button"
        class="flex w-full items-center gap-2 rounded-lg px-2 py-1.5 text-sm text-left hover:bg-white/5"
        @click="toggle(opt.value)"
      >
        <span class="w-[15px] h-[15px] rounded border flex items-center justify-center text-[10px] flex-none"
          :class="selected.has(opt.value) ? 'bg-primary border-primary text-primary-foreground' : 'border-white/30'">
          {{ selected.has(opt.value) ? '✓' : '' }}
        </span>
        {{ opt.label }}
      </button>
      <div v-if="!visibleOptions.length" class="px-2 py-2 text-xs text-muted-foreground text-center">{{ emptyText }}</div>
    </div>
  </Popover>
</template>

<script setup lang="ts">
// Searchable multi-select filter. The floating panel is the reka Popover
// primitive (open/close, click-outside, Escape, collision-aware positioning,
// portal — so the panel is never clipped by a scroll container). This file
// keeps only the search filter + checkbox list; selection state lives in the
// parent (the `selected` Set), toggled via the `toggle` emit — the panel stays
// open across toggles, which is the point of a multi-select.
import { ref, computed, watch, nextTick } from 'vue'
import { Popover } from '@/components/ui'

const props = withDefaults(defineProps<{
  label: string
  options: { value: string; label: string }[]
  selected: Set<string>
  searchable?: boolean
  searchPlaceholder?: string
  emptyText?: string
}>(), { searchable: false, searchPlaceholder: '', emptyText: '' })

const emit = defineEmits<{ toggle: [value: string] }>()
const open = ref(false)
const query = ref('')
const searchEl = ref<HTMLInputElement | null>(null)

const visibleOptions = computed(() => {
  const q = query.value.trim().toLowerCase()
  if (!props.searchable || !q) return props.options
  return props.options.filter((o) => o.label.toLowerCase().includes(q))
})

function toggle(v: string) { emit('toggle', v) }

// Focus the search field on open; clear the query on close so a reopened
// dropdown starts unfiltered (matches the pre-Popover behavior).
watch(open, (isOpen) => {
  if (isOpen) {
    if (props.searchable) nextTick(() => searchEl.value?.focus())
  } else {
    query.value = ''
  }
})
</script>
