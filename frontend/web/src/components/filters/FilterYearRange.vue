<template>
  <div class="flex items-center gap-2">
    <Select
      :model-value="minStr"
      :options="options"
      size="sm"
      class="flex-1"
      @update:model-value="(v) => emitVal('min', v as string)"
    />
    <span class="text-white/40">—</span>
    <Select
      :model-value="maxStr"
      :options="options"
      size="sm"
      class="flex-1"
      @update:model-value="(v) => emitVal('max', v as string)"
    />
  </div>
</template>

<script setup lang="ts">
// Shared year-range control (Browse + Profile watchlist). Two DS Select
// dropdowns over a descending year list bounded by floorYear..ceilYear.
// reka-ui's SelectItem forbids an empty-string value, so the "any year"
// option uses a non-empty sentinel. min <= max is enforced here so both
// consumers inherit it.
import { computed } from 'vue'
import { Select } from '@/components/ui'

const props = defineProps<{
  min: number | null
  max: number | null
  floorYear: number
  ceilYear: number
}>()

const emit = defineEmits<{
  (e: 'update:min', v: number | null): void
  (e: 'update:max', v: number | null): void
}>()

const ANY = 'any'

const options = computed(() => {
  const out = [{ value: ANY, label: '—' }]
  for (let y = props.ceilYear; y >= props.floorYear; y--) {
    out.push({ value: String(y), label: String(y) })
  }
  return out
})

const minStr = computed(() => (props.min == null ? ANY : String(props.min)))
const maxStr = computed(() => (props.max == null ? ANY : String(props.max)))

function emitVal(which: 'min' | 'max', v: string) {
  const n = v === ANY || v === '' ? null : Number(v)
  if (which === 'min') {
    emit('update:min', n)
    if (n != null && props.max != null && n > props.max) emit('update:max', n)
  } else {
    emit('update:max', n)
    if (n != null && props.min != null && n < props.min) emit('update:min', n)
  }
}
</script>
