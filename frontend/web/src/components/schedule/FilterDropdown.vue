<!-- frontend/web/src/components/schedule/FilterDropdown.vue -->
<template>
  <div ref="root" class="relative">
    <button
      type="button"
      class="flex items-center gap-1.5 text-xs rounded-lg border px-2.5 py-1.5 whitespace-nowrap transition-colors"
      :class="selected.size ? 'bg-primary/15 border-primary/50 text-primary' : 'bg-white/[0.06] border-white/10 text-foreground/80 hover:bg-white/10'"
      @click.stop="toggleOpen"
    >
      {{ label }}<span class="opacity-50 text-[10px]">▾</span>
    </button>
    <div v-if="open" class="absolute top-[calc(100%+6px)] left-0 z-50 w-52 rounded-xl border border-white/10 bg-popover text-popover-foreground p-1.5 shadow-xl shadow-black/30">
      <input
        v-if="searchable"
        ref="searchEl"
        v-model="query"
        :placeholder="searchPlaceholder"
        class="w-full mb-1.5 rounded-lg bg-white/[0.06] px-2.5 py-1.5 text-sm text-foreground outline-none placeholder:text-muted-foreground"
        @click.stop
      />
      <div class="max-h-60 overflow-y-auto">
        <button
          v-for="opt in visibleOptions"
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
        <div v-if="!visibleOptions.length" class="px-2 py-2 text-xs text-muted-foreground text-center">{{ emptyText }}</div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, nextTick, onMounted, onBeforeUnmount } from 'vue'

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
const root = ref<HTMLElement | null>(null)
const searchEl = ref<HTMLInputElement | null>(null)

const visibleOptions = computed(() => {
  const q = query.value.trim().toLowerCase()
  if (!props.searchable || !q) return props.options
  return props.options.filter((o) => o.label.toLowerCase().includes(q))
})

function toggleOpen() {
  open.value = !open.value
  if (!open.value) query.value = ''
}
function toggle(v: string) { emit('toggle', v) }
function onDocClick(e: MouseEvent) {
  if (root.value && !root.value.contains(e.target as Node)) { open.value = false; query.value = '' }
}

// Focus the search field when a searchable dropdown opens.
watch(open, (isOpen) => {
  if (isOpen && props.searchable) nextTick(() => searchEl.value?.focus())
})

onMounted(() => document.addEventListener('click', onDocClick))
onBeforeUnmount(() => document.removeEventListener('click', onDocClick))
</script>
