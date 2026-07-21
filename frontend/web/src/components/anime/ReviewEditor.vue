<!--
  ReviewEditor — review writing surface (feedback 2026-07-21): auto-growing
  textarea (~8 rows → 60vh), formatting toolbar wrapping the current selection
  with review mini-markdown markers, Write/Preview toggle. Pure v-model.
-->
<template>
  <div class="rounded-lg border border-white/10 bg-white/5 focus-within:ring-2 focus-within:ring-cyan-500/50 transition-shadow">
    <!-- Toolbar -->
    <div class="flex items-center gap-1 border-b border-white/10 px-2 py-1.5">
      <button
        v-for="a in actions"
        :key="a.key"
        type="button"
        class="rounded p-1.5 text-white/60 hover:bg-white/10 hover:text-white transition-colors disabled:opacity-40"
        :title="$t(`anime.reviewFmt.${a.key}`)"
        :aria-label="$t(`anime.reviewFmt.${a.key}`)"
        :disabled="mode === 'preview'"
        @click="a.run"
      >
        <component :is="a.icon" class="size-4" />
      </button>
      <div class="ml-auto">
        <SegmentedControl
          :model-value="mode"
          :options="[
            { value: 'write', label: $t('anime.reviewFmt.write') },
            { value: 'preview', label: $t('anime.reviewFmt.preview') },
          ]"
          size="sm"
          @update:model-value="mode = $event as 'write' | 'preview'"
        />
      </div>
    </div>

    <!-- Write -->
    <textarea
      v-if="mode === 'write'"
      ref="ta"
      :value="modelValue"
      :placeholder="placeholder"
      class="block w-full resize-none bg-transparent px-4 py-3 text-white placeholder-white/30 focus-visible:outline-none"
      :style="{ minHeight: '11rem', maxHeight: '60vh', height: taHeight }"
      @input="onInput"
      @keydown="onKeydown"
    ></textarea>

    <!-- Preview -->
    <div v-else class="min-h-44 px-4 py-3 text-white/80">
      <ReviewMarkdown v-if="modelValue.trim()" :source="modelValue" />
      <p v-else class="text-white/30">{{ placeholder }}</p>
    </div>

    <p class="border-t border-white/10 px-3 py-1.5 text-xs text-white/40">
      {{ $t('anime.reviewFmt.hint') }}
    </p>
  </div>
</template>

<script setup lang="ts">
import { nextTick, ref, watch } from 'vue'
import { Bold, Italic, Strikethrough, EyeOff, List } from 'lucide-vue-next'
import { SegmentedControl } from '@/components/ui'
import ReviewMarkdown from './ReviewMarkdown.vue'

const props = defineProps<{ modelValue: string; placeholder?: string }>()
const emit = defineEmits<{ 'update:modelValue': [value: string] }>()

const ta = ref<HTMLTextAreaElement | null>(null)
const mode = ref<'write' | 'preview'>('write')
const taHeight = ref('11rem')

function autoGrow() {
  const el = ta.value
  if (!el) return
  el.style.height = 'auto'
  taHeight.value = `${el.scrollHeight}px`
  el.style.height = ''
}

function onInput(e: Event) {
  emit('update:modelValue', (e.target as HTMLTextAreaElement).value)
  autoGrow()
}

// External value changes (e.g. loading an existing review) also resize.
watch(
  () => props.modelValue,
  () => void nextTick(autoGrow),
  { immediate: true },
)

function wrapSelection(before: string, after = before) {
  const el = ta.value
  if (!el) return
  const { selectionStart: s, selectionEnd: e, value } = el
  const next = value.slice(0, s) + before + value.slice(s, e) + after + value.slice(e)
  emit('update:modelValue', next)
  void nextTick(() => {
    el.focus()
    el.setSelectionRange(s + before.length, e + before.length)
    autoGrow()
  })
}

function prefixLines() {
  const el = ta.value
  if (!el) return
  const { selectionStart: s, selectionEnd: e, value } = el
  const start = value.lastIndexOf('\n', s - 1) + 1
  const segment = value.slice(start, e)
  const prefixed = segment
    .split('\n')
    .map((l) => (l.trim() ? `- ${l.replace(/^[-*]\s+/, '')}` : l))
    .join('\n')
  emit('update:modelValue', value.slice(0, start) + prefixed + value.slice(e))
  void nextTick(() => {
    el.focus()
    autoGrow()
  })
}

const actions = [
  { key: 'bold', icon: Bold, run: () => wrapSelection('**') },
  { key: 'italic', icon: Italic, run: () => wrapSelection('*') },
  { key: 'strike', icon: Strikethrough, run: () => wrapSelection('~~') },
  { key: 'spoiler', icon: EyeOff, run: () => wrapSelection('||') },
  { key: 'list', icon: List, run: prefixLines },
] as const

function onKeydown(e: KeyboardEvent) {
  if (!(e.ctrlKey || e.metaKey)) return
  if (e.key === 'b') {
    e.preventDefault()
    wrapSelection('**')
  } else if (e.key === 'i') {
    e.preventDefault()
    wrapSelection('*')
  }
}
</script>
