<template>
  <section class="mx-auto w-full max-w-6xl px-4 py-10 md:px-6 md:py-16">
    <div class="mb-8 max-w-3xl">
      <Badge variant="primary" class="mb-4 gap-1.5">
        <Sparkles class="size-3.5" aria-hidden="true" />
        {{ $t('zundamon.eyebrow') }}
      </Badge>
      <h1 class="font-display text-3xl font-semibold tracking-tight text-foreground md:text-5xl">
        {{ $t('zundamon.title') }}
      </h1>
      <p class="mt-4 text-base leading-relaxed text-muted-foreground md:text-lg">
        {{ $t('zundamon.subtitle') }}
      </p>
    </div>

    <div class="grid gap-6 lg:grid-cols-[minmax(0,5fr)_minmax(0,7fr)]">
      <Card padding="none" class="relative overflow-hidden border border-border">
        <img
          src="/illustrations/zundamon-tts.webp"
          :alt="$t('zundamon.imageAlt')"
          class="aspect-square h-full w-full object-cover"
          width="960"
          height="960"
          fetchpriority="high"
        />
        <div class="absolute inset-x-0 bottom-0 bg-gradient-to-t from-background/90 to-transparent p-5 pt-16">
          <p class="font-display text-xl font-semibold text-foreground">{{ $t('zundamon.portraitTitle') }}</p>
          <p class="mt-1 text-sm text-muted-foreground">{{ $t('zundamon.portraitCaption') }}</p>
        </div>
      </Card>

      <Card variant="elevated" padding="lg" class="border border-border">
        <div class="flex items-start gap-3">
          <div class="flex size-11 shrink-0 items-center justify-center rounded-xl bg-brand-cyan/10 text-brand-cyan">
            <AudioLines class="size-5" aria-hidden="true" />
          </div>
          <div>
            <h2 class="font-display text-xl font-semibold text-foreground md:text-2xl">
              {{ $t('zundamon.studioTitle') }}
            </h2>
            <p class="mt-1 text-sm leading-relaxed text-muted-foreground">
              {{ $t('zundamon.studioHint') }}
            </p>
          </div>
        </div>

        <Alert
          v-if="!supported"
          variant="warning"
          :title="$t('zundamon.unsupportedTitle')"
          class="mt-6"
        >
          {{ $t('zundamon.unsupportedBody') }}
        </Alert>

        <div class="mt-6">
          <div class="mb-2 flex items-center justify-between gap-3">
            <label for="zundamon-copy" class="text-sm font-medium text-foreground">
              {{ $t('zundamon.textLabel') }}
            </label>
            <span class="text-xs tabular-nums text-muted-foreground">
              {{ $t('zundamon.counter', { count: text.length, max: MAX_TEXT_LENGTH }) }}
            </span>
          </div>
          <textarea
            id="zundamon-copy"
            v-model="text"
            :maxlength="MAX_TEXT_LENGTH"
            :placeholder="$t('zundamon.placeholder')"
            class="min-h-44 w-full resize-y rounded-xl border border-border bg-background/60 p-4 text-base leading-relaxed text-foreground outline-none transition focus:border-brand-cyan/60 focus:ring-2 focus:ring-brand-cyan/20 disabled:cursor-not-allowed disabled:opacity-60"
            :disabled="!supported"
          />
        </div>

        <div class="mt-5 grid gap-5 md:grid-cols-2">
          <Select
            v-model="selectedVoiceUri"
            :options="voiceOptions"
            :label="$t('zundamon.voiceLabel')"
            :placeholder="$t('zundamon.browserDefault')"
            :disabled="!supported || voices.length === 0"
          />

          <div class="grid grid-cols-2 gap-4">
            <div>
              <p class="mb-2 text-sm font-medium text-foreground">{{ $t('zundamon.rate') }}</p>
              <Stepper
                v-model="rate"
                :min="0.5"
                :max="1.5"
                :step="0.1"
                suffix="×"
                :label="$t('zundamon.rate')"
              />
            </div>
            <div>
              <p class="mb-2 text-sm font-medium text-foreground">{{ $t('zundamon.pitch') }}</p>
              <Stepper
                v-model="pitch"
                :min="0.5"
                :max="2"
                :step="0.1"
                suffix="×"
                :label="$t('zundamon.pitch')"
              />
            </div>
          </div>
        </div>

        <div class="mt-6 flex flex-wrap items-center gap-3">
          <Button
            size="lg"
            :disabled="!supported || !text.trim()"
            @click="speak(text, rate, pitch)"
          >
            <Play class="size-4" aria-hidden="true" />
            {{ isSpeaking ? $t('zundamon.restart') : $t('zundamon.speak') }}
          </Button>
          <Button size="lg" variant="ghost" :disabled="!isSpeaking" @click="stop">
            <Square class="size-4" aria-hidden="true" />
            {{ $t('zundamon.stop') }}
          </Button>
          <p class="min-h-5 text-sm text-muted-foreground" aria-live="polite">
            {{ statusMessage }}
          </p>
        </div>

        <div class="mt-6 flex gap-3 rounded-xl border border-border bg-muted/40 p-4">
          <ShieldCheck class="mt-0.5 size-5 shrink-0 text-success" aria-hidden="true" />
          <div>
            <p class="text-sm font-semibold text-foreground">{{ $t('zundamon.localTitle') }}</p>
            <p class="mt-1 text-sm leading-relaxed text-muted-foreground">
              {{ $t('zundamon.localBody') }}
            </p>
          </div>
        </div>
      </Card>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { AudioLines, Play, ShieldCheck, Sparkles, Square } from 'lucide-vue-next'
import { Alert, Badge, Button, Card, Select, Stepper } from '@/components/ui'
import { useZundamonTts } from '@/composables/useZundamonTts'

const MAX_TEXT_LENGTH = 500
const { t } = useI18n()
const text = ref(t('zundamon.sample'))
const rate = ref(1.1)
const pitch = ref(1.25)

const {
  supported,
  voices,
  selectedVoiceUri,
  isSpeaking,
  status,
  speak,
  stop,
} = useZundamonTts()

const voiceOptions = computed(() =>
  voices.value.map((voice) => ({
    value: voice.voiceURI,
    label: `${voice.name} · ${voice.lang}`,
  })),
)

const statusMessage = computed(() => {
  if (status.value === 'speaking') return t('zundamon.statusSpeaking')
  if (status.value === 'done') return t('zundamon.statusDone')
  if (status.value === 'error') return t('zundamon.statusError')
  return ''
})
</script>
