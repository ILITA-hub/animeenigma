<!--
  ReviewMarkdown — renders review mini-markdown via parseReviewMarkdown()
  typed tokens and TEXT interpolation only (never v-html — the XSS defense).
  Spoilers render as redacted chips until clicked. `collapsible` clamps long
  content (measured overflow, not char count) behind a Show-more toggle.
-->
<template>
  <div>
    <div
      ref="bodyEl"
      class="relative"
      :class="collapsed ? 'max-h-56 overflow-hidden' : ''"
    >
      <template v-for="(block, bi) in blocks" :key="bi">
        <ul v-if="block.type === 'ul'" class="list-disc pl-5 space-y-0.5 my-1.5">
          <li v-for="(item, ii) in block.items" :key="ii">
            <ReviewInline :tokens="item" :revealed="revealed" :block="`${bi}:${ii}`" @reveal="reveal" />
          </li>
        </ul>
        <p v-else class="my-1.5 first:mt-0 last:mb-0">
          <template v-for="(line, li) in block.lines" :key="li">
            <br v-if="li > 0" />
            <ReviewInline :tokens="line" :revealed="revealed" :block="`${bi}:${li}`" @reveal="reveal" />
          </template>
        </p>
      </template>
      <div
        v-if="collapsed && overflowing"
        class="pointer-events-none absolute inset-x-0 bottom-0 h-14 bg-gradient-to-t from-black/60 to-transparent"
      />
    </div>
    <button
      v-if="collapsible && overflowing"
      type="button"
      class="mt-1.5 text-sm text-cyan-400 hover:text-cyan-300 transition-colors"
      @click="expanded = !expanded"
    >
      {{ expanded ? $t('anime.reviewFmt.showLess') : $t('anime.reviewFmt.showMore') }}
    </button>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { parseReviewMarkdown } from '@/utils/reviewMarkdown'
import ReviewInline from './ReviewInline.vue'

const props = withDefaults(defineProps<{ source: string; collapsible?: boolean }>(), {
  collapsible: false,
})

const blocks = computed(() => parseReviewMarkdown(props.source))

// Per-spoiler reveal state, keyed "block:line:token".
const revealed = ref<Set<string>>(new Set())
function reveal(key: string) {
  revealed.value.add(key)
}

// Collapse: clamp only when the rendered body actually overflows the clamp
// height (max-h-56 = 224px), re-measured when the source changes.
const bodyEl = ref<HTMLElement | null>(null)
const expanded = ref(false)
const overflowing = ref(false)
const collapsed = computed(() => props.collapsible && !expanded.value)

async function measure() {
  if (!props.collapsible) return
  await nextTick()
  const el = bodyEl.value
  if (el) overflowing.value = el.scrollHeight > 240
}
onMounted(measure)
watch(() => props.source, () => {
  expanded.value = false
  revealed.value = new Set()
  void measure()
})
</script>
