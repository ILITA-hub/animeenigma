<template>
  <div class="space-y-2">
    <Input
      v-if="searchable"
      v-model="query"
      type="text"
      size="sm"
      :placeholder="searchPlaceholder"
      :aria-label="searchPlaceholder"
    />
    <div :class="['overflow-y-auto pr-1 space-y-1', maxHeightClass]">
      <label
        v-for="item in visibleItems"
        :key="item.id"
        class="flex items-center gap-2 text-sm text-white/70 hover:text-white cursor-pointer py-0.5"
      >
        <Checkbox
          :model-value="selected.includes(item.id)"
          @update:model-value="(v) => onToggle(item.id, v === true)"
        />
        <span class="flex-1">{{ item.label }}</span>
        <span v-if="item.count != null" class="text-xs text-white/40">{{ item.count }}</span>
      </label>
      <p v-if="!visibleItems.length" class="text-xs text-white/40 py-1">{{ emptyText }}</p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { Input, Checkbox } from '@/components/ui'

export interface FilterItem {
  id: string
  label: string
  count?: number
}

const props = withDefaults(
  defineProps<{
    items: FilterItem[]
    selected: string[]
    searchable?: boolean
    searchPlaceholder?: string
    maxHeightClass?: string
    emptyText?: string
  }>(),
  { searchable: true, searchPlaceholder: '', maxHeightClass: 'max-h-48', emptyText: '—' },
)

const emit = defineEmits<{ (e: 'update:selected', value: string[]): void }>()

const query = ref('')

const visibleItems = computed(() => {
  const q = query.value.trim().toLowerCase()
  if (!q) return props.items
  return props.items.filter(i => i.label.toLowerCase().includes(q))
})

function onToggle(id: string, checked: boolean) {
  const set = new Set(props.selected)
  if (checked) set.add(id)
  else set.delete(id)
  emit('update:selected', [...set])
}
</script>
