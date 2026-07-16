<template>
  <div class="max-w-4xl mx-auto px-4 py-6">
    <button type="button" class="text-sm text-white/60 hover:text-white" @click="goBack">
      ← {{ $t('characters.back') }}
    </button>

    <div v-if="loading" class="mt-6 flex justify-center">
      <Spinner />
    </div>

    <div v-else-if="error" class="mt-6 text-center text-destructive">
      {{ error }}
    </div>

    <div v-else-if="character" class="mt-6 flex flex-col md:flex-row gap-6">
      <div class="w-48 shrink-0 mx-auto md:mx-0">
        <CharacterImage
          :src="character.image || '/placeholder.svg'"
          :alt="character.name"
          ratio="2/3"
          rounded="xl"
          :proxy-width="384"
          class="border border-white/10"
        />
      </div>

      <div class="flex-1 min-w-0">
        <h1 class="text-2xl font-semibold text-white">{{ character.name }}</h1>
        <p v-if="character.nameJp" class="text-white/50 mt-1">{{ character.nameJp }}</p>
        <p v-if="character.synonyms" class="text-sm text-white/40 mt-2">
          {{ $t('characters.synonyms') }}: {{ character.synonyms }}
        </p>

        <h2 class="text-sm font-semibold text-white/70 mt-6 mb-2">{{ $t('characters.description') }}</h2>
        <p v-if="character.description" class="text-white/80 whitespace-pre-line leading-relaxed">
          {{ character.description }}
        </p>
        <p v-else class="text-white/40">{{ $t('characters.noDescription') }}</p>

        <template v-if="character.seyu.length">
          <h2 class="text-sm font-semibold text-white/70 mt-6 mb-2">{{ $t('characters.seyu') }}</h2>
          <ul class="flex flex-col gap-2">
            <li v-for="va in character.seyu" :key="va.id" class="flex items-center gap-3">
              <img
                :src="va.image || '/placeholder.svg'"
                :alt="va.name"
                loading="lazy"
                class="size-10 shrink-0 rounded-full object-cover border border-white/10"
              />
              <span class="text-white/85 text-sm">{{ va.name }}</span>
            </li>
          </ul>
        </template>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Spinner from '@/components/ui/Spinner.vue'
import CharacterImage from '@/components/anime/CharacterImage.vue'
import { useCharacter } from '@/composables/useCharacters'

const route = useRoute()
const router = useRouter()
const { character, loading, error, fetchCharacter } = useCharacter()

function goBack() {
  if (window.history.length > 1) router.back()
  else void router.push('/')
}

function load() {
  const id = String(route.params.id ?? '')
  if (id) void fetchCharacter(id)
}

onMounted(load)
watch(() => route.params.id, load)
</script>
