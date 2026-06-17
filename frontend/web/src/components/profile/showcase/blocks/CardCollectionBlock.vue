<script setup lang="ts">
import { ref, onMounted } from 'vue'
import type { CardCollectionConfig } from '@/types/showcase'
import { gachaApi, cardImageUrl, type GachaCard } from '@/api/gacha'

const props = defineProps<{ config: CardCollectionConfig; userId?: string }>()

const cards = ref<GachaCard[]>([])

onMounted(async () => {
  const ids = new Set(props.config.card_ids ?? [])
  if (!ids.size) return
  try {
    const res = await gachaApi.getCollection()
    // CollectionView has cards: CollectionCardView[], each with .card: GachaCard + .owned bool
    const view = res.data?.data ?? res.data
    const all = (view as { cards: Array<{ card: GachaCard; owned: boolean }> }).cards ?? []
    cards.value = all
      .filter((entry) => entry.owned && ids.has(entry.card.id))
      .map((entry) => entry.card)
  } catch {
    cards.value = []
  }
})
</script>

<template>
  <div class="rounded-xl border border-border bg-card p-4 md:p-6">
    <h3 class="mb-3 text-lg font-semibold text-foreground">{{ $t('showcase.block.card_collection') }}</h3>
    <div v-if="cards.length" class="grid grid-cols-3 gap-3 sm:grid-cols-4 md:grid-cols-6">
      <div
        v-for="c in cards"
        :key="c.id"
        class="relative overflow-hidden rounded-lg aspect-[2/3] bg-card border border-border"
      >
        <img
          v-if="c.image_path"
          :src="cardImageUrl(c.image_path)"
          :alt="c.name"
          class="w-full h-full object-cover"
        />
        <div class="absolute bottom-0 left-0 right-0 bg-black/60 px-1 py-0.5">
          <p class="text-foreground text-[10px] font-medium truncate">{{ c.name }}</p>
        </div>
      </div>
    </div>
    <p v-else class="text-sm text-muted-foreground">{{ $t('showcase.empty') }}</p>
  </div>
</template>
