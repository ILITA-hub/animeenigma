<script setup lang="ts">
import type { ShowcaseBlock } from '@/types/showcase'
import AboutBlock from './blocks/AboutBlock.vue'
import FavoriteAnimeBlock from './blocks/FavoriteAnimeBlock.vue'
import StatsBlock from './blocks/StatsBlock.vue'
import FavoriteCharacterBlock from './blocks/FavoriteCharacterBlock.vue'
import CardCollectionBlock from './blocks/CardCollectionBlock.vue'
import ContinueWatchingBlock from './blocks/ContinueWatchingBlock.vue'
import OpEdBlock from './blocks/OpEdBlock.vue'
import AnimeDnaBlock from './blocks/AnimeDnaBlock.vue'
import CompatibilityBlock from './blocks/CompatibilityBlock.vue'

defineProps<{
  block: ShowcaseBlock
  userId: string
  isOwner: boolean
}>()
</script>

<template>
  <AboutBlock v-if="block.type === 'about'" :config="block.config as never" :variant="block.variant" />
  <FavoriteAnimeBlock v-else-if="block.type === 'favorite_anime'" :config="block.config as never" :variant="block.variant" :user-id="userId" />
  <StatsBlock v-else-if="block.type === 'stats'" :user-id="userId" :variant="block.variant" />
  <FavoriteCharacterBlock v-else-if="block.type === 'favorite_character'" :config="block.config as never" :variant="block.variant" />
  <CardCollectionBlock v-else-if="block.type === 'card_collection'" :config="block.config as never" :user-id="userId" :variant="block.variant" />
  <ContinueWatchingBlock v-else-if="block.type === 'continue_watching'" :user-id="userId" :variant="block.variant" />
  <OpEdBlock v-else-if="block.type === 'op_ed'" :config="block.config as never" :variant="block.variant" />
  <AnimeDnaBlock v-else-if="block.type === 'anime_dna'" :user-id="userId" :variant="block.variant" />
  <CompatibilityBlock v-else-if="block.type === 'compatibility'" :user-id="userId" :is-owner="isOwner" :variant="block.variant" />
</template>
