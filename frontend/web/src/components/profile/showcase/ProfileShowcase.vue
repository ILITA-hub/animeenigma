<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import type { ShowcaseBlock } from '@/types/showcase'
import { showcaseApi } from '@/api/client'
import { useToast } from '@/composables/useToast'
import AboutBlock from './blocks/AboutBlock.vue'
import FavoriteAnimeBlock from './blocks/FavoriteAnimeBlock.vue'
import StatsBlock from './blocks/StatsBlock.vue'
import FavoriteCharacterBlock from './blocks/FavoriteCharacterBlock.vue'
import CardCollectionBlock from './blocks/CardCollectionBlock.vue'
import ContinueWatchingBlock from './blocks/ContinueWatchingBlock.vue'
import OpEdBlock from './blocks/OpEdBlock.vue'
import AnimeDnaBlock from './blocks/AnimeDnaBlock.vue'
import CompatibilityBlock from './blocks/CompatibilityBlock.vue'
import ShowcaseEditor from './ShowcaseEditor.vue'

const props = defineProps<{ userId: string; isOwner: boolean }>()

const { t } = useI18n()
const toast = useToast()

const blocks = ref<ShowcaseBlock[]>([])
const editing = ref(false)
const loading = ref(true)

async function load() {
  loading.value = true
  try {
    const res = await showcaseApi.getShowcase(props.userId)
    const data = 'data' in res.data
      ? (res.data as { data: { blocks: ShowcaseBlock[] } }).data
      : res.data
    blocks.value = data.blocks ?? []
  } catch {
    blocks.value = []
  } finally {
    loading.value = false
  }
}

async function onSave(next: ShowcaseBlock[]) {
  try {
    await showcaseApi.saveShowcase(next)
    blocks.value = next
    editing.value = false
    toast.push(t('showcase.saved'), 'success')
  } catch {
    toast.push(t('showcase.save_error'), 'error')
  }
}

onMounted(load)
</script>

<template>
  <section class="space-y-4">
    <div class="flex items-center justify-between">
      <h2 class="text-xl font-semibold text-foreground">{{ $t('showcase.title') }}</h2>
      <button
        v-if="isOwner && !editing"
        type="button"
        class="rounded-lg border border-border px-3 py-1 text-sm font-medium text-foreground hover:bg-accent"
        @click="editing = true"
      >
        {{ $t('showcase.edit') }}
      </button>
    </div>

    <ShowcaseEditor
      v-if="editing"
      :user-id="userId"
      :model-value="blocks"
      @save="onSave"
      @cancel="editing = false"
    />

    <template v-else>
      <p v-if="!loading && !blocks.length" class="text-sm text-muted-foreground">
        {{ $t('showcase.empty') }}
      </p>
      <template v-for="(b, i) in blocks" :key="i">
        <AboutBlock v-if="b.type === 'about'" :config="b.config as never" :variant="b.variant" />
        <FavoriteAnimeBlock v-else-if="b.type === 'favorite_anime'" :config="b.config as never" :variant="b.variant" :user-id="userId" />
        <StatsBlock v-else-if="b.type === 'stats'" :user-id="userId" :variant="b.variant" />
        <FavoriteCharacterBlock v-else-if="b.type === 'favorite_character'" :config="b.config as never" :variant="b.variant" />
        <CardCollectionBlock v-else-if="b.type === 'card_collection'" :config="b.config as never" :user-id="userId" :variant="b.variant" />
        <ContinueWatchingBlock v-else-if="b.type === 'continue_watching'" :user-id="userId" :variant="b.variant" />
        <OpEdBlock v-else-if="b.type === 'op_ed'" :config="b.config as never" :variant="b.variant" />
        <AnimeDnaBlock v-else-if="b.type === 'anime_dna'" :user-id="userId" :variant="b.variant" />
        <CompatibilityBlock v-else-if="b.type === 'compatibility'" :user-id="userId" :is-owner="isOwner" :variant="b.variant" />
      </template>
    </template>
  </section>
</template>
