<script setup lang="ts">
import { ref, onMounted } from 'vue'
import type { FavoriteCharacterConfig } from '@/types/showcase'
import { charactersApi } from '@/api/client'
import { getLocalizedTitle } from '@/utils/title'
import { getImageUrl } from '@/composables/useImageProxy'
import CharacterCard from '@/components/anime/CharacterCard.vue'
import type { CharacterCardModel } from '@/types/character'

const props = defineProps<{ config: FavoriteCharacterConfig }>()

interface ApiCharacter {
  shikimori_id: string
  name?: string
  name_ru?: string
  name_jp?: string
  poster_url?: string
}

const items = ref<CharacterCardModel[]>([])

onMounted(async () => {
  const ids = props.config.character_ids ?? []
  if (!ids.length) return
  const results = await Promise.all(
    ids.map((id) =>
      charactersApi
        .getCharacter(String(id))
        .then((r): CharacterCardModel => {
          const raw = r.data as { data?: ApiCharacter } & ApiCharacter
          const c: ApiCharacter = 'data' in raw && raw.data ? raw.data : raw
          return {
            id: c.shikimori_id,
            name: getLocalizedTitle(c.name, c.name_ru, c.name_jp),
            image: getImageUrl(c.poster_url),
            role: 'supporting',
          }
        })
        .catch(() => null),
    ),
  )
  items.value = results.filter((c): c is CharacterCardModel => c !== null)
})
</script>

<template>
  <div class="rounded-xl border border-border bg-card p-4 md:p-6">
    <h3 class="mb-3 text-lg font-semibold text-foreground">{{ $t('showcase.block.favorite_character') }}</h3>
    <div v-if="items.length" class="grid grid-cols-3 gap-3 sm:grid-cols-4 md:grid-cols-6">
      <CharacterCard v-for="c in items" :key="c.id" :model="c" />
    </div>
    <p v-else class="text-sm text-muted-foreground">{{ $t('showcase.empty') }}</p>
  </div>
</template>
