<script setup lang="ts">
import { ref } from 'vue'
import draggable from 'vuedraggable'
import type { ShowcaseBlock, ShowcaseBlockType } from '@/types/showcase'
import { MAX_SHOWCASE_BLOCKS } from '@/types/showcase'

const props = defineProps<{ userId: string; modelValue: ShowcaseBlock[] }>()
const emit = defineEmits<{ save: [ShowcaseBlock[]]; cancel: [] }>()

const local = ref<ShowcaseBlock[]>(props.modelValue.map((b) => ({ ...b })))

const ADDABLE: ShowcaseBlockType[] = ['about', 'favorite_anime', 'stats', 'favorite_character', 'card_collection']

function addBlock(type: ShowcaseBlockType) {
  if (local.value.length >= MAX_SHOWCASE_BLOCKS) return
  const config = type === 'about' ? { title: '', text: '' } : {}
  local.value.push({ type, order: local.value.length, config })
}

function removeBlock(i: number) {
  local.value.splice(i, 1)
}

function save() {
  const renumbered = local.value.map((b, i) => ({ ...b, order: i }))
  emit('save', renumbered)
}
</script>

<template>
  <div class="space-y-4">
    <div class="flex flex-wrap items-center gap-2">
      <button
        v-for="t in ADDABLE"
        :key="t"
        type="button"
        class="rounded-lg border border-border px-3 py-1 text-sm font-medium text-foreground hover:bg-accent"
        @click="addBlock(t)"
      >
        + {{ $t(`showcase.block.${t}`) }}
      </button>
    </div>

    <draggable v-model="local" item-key="order" handle=".showcase-drag-handle">
      <template #item="{ element, index }">
        <div class="mb-3 rounded-xl border border-border bg-card p-3">
          <div class="mb-2 flex items-center justify-between">
            <span class="showcase-drag-handle cursor-grab text-sm font-semibold text-foreground">
              ⠿ {{ $t(`showcase.block.${element.type}`) }}
            </span>
            <button
              type="button"
              :data-test="`showcase-remove-${index}`"
              class="text-sm font-medium text-destructive"
              @click="removeBlock(index)"
            >
              {{ $t('showcase.remove_block') }}
            </button>
          </div>

          <!-- About block inline editor -->
          <div v-if="element.type === 'about'" class="space-y-2">
            <input
              v-model="(element.config as { title?: string }).title"
              :placeholder="$t('showcase.about_title_placeholder')"
              maxlength="64"
              class="w-full rounded-lg border border-border bg-background px-3 py-2 text-sm"
            />
            <textarea
              v-model="(element.config as { text?: string }).text"
              :placeholder="$t('showcase.about_placeholder')"
              rows="4"
              maxlength="2000"
              class="w-full rounded-lg border border-border bg-background px-3 py-2 text-sm"
            />
          </div>
          <!-- Other block types: picker stubs wired to existing pickers -->
          <p v-else class="text-xs text-muted-foreground">
            {{
              $t(
                `showcase.pick_${
                  element.type === 'favorite_anime'
                    ? 'anime'
                    : element.type === 'favorite_character'
                      ? 'character'
                      : element.type === 'card_collection'
                        ? 'cards'
                        : 'anime'
                }`,
              )
            }}
          </p>
        </div>
      </template>
    </draggable>

    <div class="flex gap-2">
      <button
        type="button"
        data-test="showcase-save"
        class="rounded-lg bg-primary px-4 py-2 text-sm font-semibold text-primary-foreground"
        @click="save"
      >
        {{ $t('showcase.save') }}
      </button>
      <button
        type="button"
        class="rounded-lg border border-border px-4 py-2 text-sm font-medium text-foreground"
        @click="emit('cancel')"
      >
        {{ $t('showcase.cancel') }}
      </button>
    </div>
  </div>
</template>
