<template>
  <div class="flex items-center gap-1">
    <button
      v-for="n in 10"
      :key="n"
      class="w-7 h-7 rounded text-sm font-medium transition-all duration-150"
      :class="[
        n <= displayScore
          ? 'bg-cyan-500/30 text-cyan-400 border border-cyan-500/50'
          : 'bg-white/5 text-white/60 border border-white/10 hover:bg-white/10 hover:text-white/80',
        disabled ? 'cursor-default' : 'cursor-pointer'
      ]"
      :disabled="disabled"
      @click="handleClick(n)"
      @mouseenter="hoverScore = n"
      @mouseleave="hoverScore = 0"
    >
      {{ n }}
    </button>
    <button
      v-if="modelValue && !disabled"
      class="ml-1 p-1 text-white/60 hover:text-destructive transition-colors"
      title="Remove rating"
      @click="$emit('remove')"
    >
      <X class="size-4" aria-hidden="true" />
    </button>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { X } from 'lucide-vue-next'

const props = defineProps<{
  modelValue?: number | null
  disabled?: boolean
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', value: number): void
  (e: 'remove'): void
}>()

const hoverScore = ref(0)

const displayScore = computed(() => {
  if (hoverScore.value > 0 && !props.disabled) return hoverScore.value
  return props.modelValue || 0
})

const handleClick = (n: number) => {
  if (props.disabled) return
  emit('update:modelValue', n)
}
</script>
