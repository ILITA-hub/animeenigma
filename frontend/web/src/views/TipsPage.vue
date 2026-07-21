<template>
  <div class="max-w-4xl mx-auto px-4 py-12">
    <!-- Secret tips & hotkeys page (feedback 2026-07-08T15-21-31) — nav-less;
         reached via the footer roulette or the global F1 hotkey. -->
    <header class="mb-10">
      <div class="flex items-center gap-3 mb-2">
        <Sparkles class="size-8 text-primary" aria-hidden="true" />
        <h1 class="text-3xl md:text-4xl font-semibold text-white">
          {{ t('tips.title') }}
        </h1>
      </div>
      <p class="text-white/60 text-lg">{{ t('tips.subtitle') }}</p>
      <div class="mt-5 flex flex-wrap gap-3">
        <Button data-testid="site-guide-launch" @click="startGuide">
          <template #icon><Compass class="size-4" aria-hidden="true" /></template>
          {{ t('siteGuide.launch') }}
        </Button>
        <Button variant="outline" data-testid="player-guide-launch" @click="startPlayerGuide">
          <template #icon><Clapperboard class="size-4" aria-hidden="true" /></template>
          {{ t('siteGuide.launchPlayer') }}
        </Button>
      </div>
    </header>

    <section :aria-label="t('tips.player.title')" class="mb-12">
      <div class="flex items-center gap-2 mb-4">
        <Keyboard class="size-5 text-primary" aria-hidden="true" />
        <h2 class="text-xl font-semibold text-white">{{ t('tips.player.title') }}</h2>
      </div>
      <div class="grid gap-4 md:grid-cols-2">
        <Card v-for="group in playerGroups" :key="group.titleKey">
          <CardHeader class="pb-2">
            <CardTitle class="text-base">{{ t(group.titleKey) }}</CardTitle>
          </CardHeader>
          <CardContent>
            <HotkeyRows :rows="group.rows" />
          </CardContent>
        </Card>
      </div>
    </section>

    <section :aria-label="t('tips.global.title')" class="mb-12">
      <div class="flex items-center gap-2 mb-4">
        <Globe class="size-5 text-primary" aria-hidden="true" />
        <h2 class="text-xl font-semibold text-white">{{ t('tips.global.title') }}</h2>
      </div>
      <Card>
        <CardContent class="pt-4">
          <HotkeyRows :rows="globalRows" />
        </CardContent>
      </Card>
    </section>

    <section :aria-label="t('tips.tricks.title')" class="mb-10">
      <div class="flex items-center gap-2 mb-4">
        <Lightbulb class="size-5 text-primary" aria-hidden="true" />
        <h2 class="text-xl font-semibold text-white">{{ t('tips.tricks.title') }}</h2>
      </div>
      <div class="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
        <Card v-for="trick in tricks" :key="trick.key">
          <CardContent class="pt-5">
            <component
              :is="trick.icon"
              class="size-6 text-primary mb-3"
              aria-hidden="true"
            />
            <h3 class="text-sm font-semibold text-white mb-1">
              {{ t(`tips.tricks.${trick.key}.title`) }}
            </h3>
            <p class="text-sm text-white/60 leading-relaxed">
              {{ t(`tips.tricks.${trick.key}.body`) }}
            </p>
          </CardContent>
        </Card>
      </div>
    </section>

    <p class="text-center text-sm text-muted-foreground italic">
      {{ t('tips.secretNote') }}
    </p>
  </div>
</template>

<script setup lang="ts">
import type { Component } from 'vue'
import {
  AudioLines,
  Clapperboard,
  Compass,
  Film,
  Flag,
  Globe,
  Keyboard,
  Lightbulb,
  MonitorPlay,
  MousePointer2,
  Smartphone,
  Sparkles,
  Users,
  Wand2,
} from 'lucide-vue-next'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { Button, Card, CardContent, CardHeader, CardTitle } from '@/components/ui'
import HotkeyRows from '@/components/tips/HotkeyRows.vue'

const { t } = useI18n()
const router = useRouter()

function startGuide(): void {
  void router.push({ path: '/', query: { guide: 'start' } })
}

function startPlayerGuide(): void {
  void router.push({ path: '/browse', query: { guide: 'player', status: 'ongoing' } })
}

// Mirrors composables/aePlayer/playerHotkeys.ts — keep in sync when the
// player's key→action contract changes.
// Row shapes are validated structurally by HotkeyRows' `rows` prop.
const playerGroups = [
  {
    titleKey: 'tips.groups.playback',
    rows: [
      { keys: ['Space', 'K'], descKey: 'tips.keys.playPause' },
      { keys: ['←', 'J'], descKey: 'tips.keys.seekBack' },
      { keys: ['→', 'L'], descKey: 'tips.keys.seekFwd' },
      { keys: ['0–9'], descKey: 'tips.keys.seekPct' },
      { keys: ['Home', 'End'], descKey: 'tips.keys.homeEnd' },
      { keys: ['Shift+N'], descKey: 'tips.keys.nextEpisode' },
      { keys: ['N'], descKey: 'tips.keys.nextEpisodeCard' },
    ],
  },
  {
    titleKey: 'tips.groups.volume',
    rows: [
      { keys: ['↑'], descKey: 'tips.keys.volUp' },
      { keys: ['↓'], descKey: 'tips.keys.volDown' },
      { keys: ['M'], descKey: 'tips.keys.mute' },
    ],
  },
  {
    titleKey: 'tips.groups.subtitles',
    rows: [
      { keys: ['C'], descKey: 'tips.keys.subs' },
      { keys: ['Z'], descKey: 'tips.keys.subEarlier' },
      { keys: ['X'], descKey: 'tips.keys.subLater' },
    ],
  },
  {
    titleKey: 'tips.groups.view',
    rows: [
      { keys: ['F'], descKey: 'tips.keys.fullscreen' },
      { keys: ['P'], descKey: 'tips.keys.pip' },
      { keys: ['Esc'], descKey: 'tips.keys.closePanels' },
    ],
  },
]

const globalRows = [
  { keys: ['F1'], descKey: 'tips.keys.help' },
  { keys: ['Esc'], descKey: 'tips.keys.escape' },
  { keys: ['Enter', 'Shift+Enter'], descKey: 'tips.keys.chat' },
  { keys: ['←', '→'], descKey: 'tips.keys.spotlight' },
]

// Every entry documents a real shipped feature — verify against the code
// before adding one (this page must never oversell).
const tricks: { key: string; icon: Component }[] = [
  { key: 'roulette', icon: Sparkles },
  { key: 'rawdub', icon: AudioLines },
  { key: 'autosync', icon: Wand2 },
  { key: 'storyboard', icon: Film },
  { key: 'contextmenu', icon: MousePointer2 },
  { key: 'theater', icon: MonitorPlay },
  { key: 'pwa', icon: Smartphone },
  { key: 'watchtogether', icon: Users },
  { key: 'report', icon: Flag },
]
</script>
