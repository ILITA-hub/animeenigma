<template>
  <Modal
    :model-value="open"
    :title="$t(solved ? 'anidle.result_win_title' : 'anidle.result_loss_title')"
    :closable="true"
    @update:model-value="$emit('close')"
  >
    <div class="space-y-4">
      <!-- Answer poster -->
      <div class="flex justify-center">
        <img
          :src="posterSrc"
          :alt="answer.name_ru"
          class="w-32 h-44 rounded-xl object-cover shadow-lg"
        />
      </div>

      <!-- Answer name -->
      <div class="text-center">
        <h2 class="text-xl font-semibold text-white">{{ answer.name_ru }}</h2>
        <p class="text-sm text-muted-foreground">{{ answer.name_en }}</p>
      </div>

      <!-- Attempt count -->
      <p class="text-center text-muted-foreground text-sm">
        {{ $t('anidle.result_attempts', { n: guesses.length }) }}
      </p>

      <!-- Emoji share grid preview -->
      <div class="rounded-lg border border-white/10 bg-white/5 p-4">
        <ShareCard :guesses="guesses" :date="date" :solved="solved" />
      </div>

      <!-- Share button -->
      <div class="flex gap-3 justify-center">
        <button
          type="button"
          class="px-4 py-2 rounded-lg bg-white/10 hover:bg-white/20 text-white text-sm font-medium transition-colors"
          @click="onShare"
        >
          {{ copied ? $t('anidle.result_share_copied') : $t('anidle.result_share_button') }}
        </button>
        <button
          type="button"
          class="px-4 py-2 rounded-lg bg-white/5 hover:bg-white/10 text-muted-foreground text-sm font-medium transition-colors"
          @click="$emit('close')"
        >
          {{ $t('anidle.result_close') }}
        </button>
      </div>
    </div>
  </Modal>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { cardPosterUrl } from '@/composables/useImageProxy'
import { buildShareText } from '@/api/anidle'
import type { VisibleAnime, GuessOutcome } from '@/api/anidle'
import Modal from '@/components/ui/Modal.vue'
import ShareCard from './ShareCard.vue'

const props = defineProps<{
  open: boolean
  answer: VisibleAnime
  guesses: GuessOutcome[]
  date: string
  solved: boolean
}>()

defineEmits<{
  close: []
}>()

const copied = ref(false)

const posterSrc = computed(() => cardPosterUrl(props.answer.poster_url, 256))

async function onShare() {
  const text = buildShareText(props.guesses, props.date, props.solved)
  try {
    await navigator.clipboard.writeText(text)
    copied.value = true
    setTimeout(() => {
      copied.value = false
    }, 2000)
  } catch {
    // clipboard may not be available — silently ignore
  }
}
</script>
