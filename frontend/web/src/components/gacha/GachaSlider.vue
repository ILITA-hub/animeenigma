<template>
  <!-- Hero banner slider: per-banner backdrop, arrows + dots, bottom scrim.
       Background: backdrop_path → gradient fallback.
       Ported from the v21 .heroC mock. -->
  <div class="hero-slider">
    <div
      v-for="(banner, i) in banners"
      :key="banner.id"
      class="slide"
      :class="{ on: i === modelValue }"
    >
      <!-- Backdrop image or gradient fallback -->
      <div
        v-if="bannerBg(banner)"
        class="art"
        :style="{ backgroundImage: `url(${bannerBg(banner)})` }"
      />
      <div v-else class="art art-fallback" />
      <div class="scrim" />
      <!-- Meta -->
      <div class="meta">
        <span class="badge" :class="banner.is_standard ? 'b-std' : 'b-event'">
          {{ banner.is_standard ? $t('gacha.banner_standard_badge') : $t('gacha.banner_list_title') }}
        </span>
        <h3>{{ banner.name }}</h3>
        <p v-if="banner.description">{{ banner.description }}</p>
      </div>
    </div>

    <!-- Arrows -->
    <button
      v-if="banners.length > 1"
      type="button"
      class="arrow l"
      :aria-label="$t('gacha.slider_prev')"
      @click="go(-1)"
    >
      <ChevronLeft class="size-5" aria-hidden="true" />
    </button>
    <button
      v-if="banners.length > 1"
      type="button"
      class="arrow r"
      :aria-label="$t('gacha.slider_next')"
      @click="go(1)"
    >
      <ChevronRight class="size-5" aria-hidden="true" />
    </button>

    <!-- Dots -->
    <div v-if="banners.length > 1" class="dots">
      <button
        v-for="(banner, i) in banners"
        :key="banner.id"
        type="button"
        class="dot"
        :class="{ on: i === modelValue }"
        :aria-label="banner.name"
        @click="select(i)"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { ChevronLeft, ChevronRight } from 'lucide-vue-next'
import { cardImageUrl, type BannerView } from '@/api/gacha'
import { cardPosterUrl } from '@/composables/useImageProxy'

const props = defineProps<{
  banners: BannerView[]
  modelValue: number
}>()

const emit = defineEmits<{
  'update:modelValue': [index: number]
}>()

function bannerBg(banner: BannerView): string {
  return banner.backdrop_path ? cardPosterUrl(cardImageUrl(banner.backdrop_path), 640) : ''
}

function select(i: number) {
  const len = props.banners.length
  if (len === 0) return
  emit('update:modelValue', ((i % len) + len) % len)
}

function go(d: number) {
  select(props.modelValue + d)
}
</script>

<style scoped>
.hero-slider {
  position: relative;
  height: clamp(380px, 62vh, 560px);
  border-radius: 1.25rem 1.25rem 0 0;
  overflow: hidden;
  border: 1px solid var(--border);
  border-bottom: none;
}
.slide {
  position: absolute;
  inset: 0;
  opacity: 0;
  transition: opacity 0.45s;
  pointer-events: none;
}
.slide.on {
  opacity: 1;
  pointer-events: auto;
}
.art {
  position: absolute;
  inset: 0;
  background-size: cover;
  background-position: center;
}
.art-fallback {
  background: linear-gradient(120deg, var(--surface-2), var(--elevated) 45%, var(--brand-cyan));
  opacity: 0.55;
}
.scrim {
  position: absolute;
  inset: 0;
  background: linear-gradient(
    180deg,
    var(--scrim-bg-soft),
    var(--scrim-bg-soft) 55%,
    var(--scrim-bg-strong)
  );
}
.meta {
  position: absolute;
  left: 2rem;
  bottom: 1.4rem;
  max-width: 60%;
  z-index: 4;
}
.meta h3 {
  font-size: 2.2rem;
  font-weight: 600;
  margin: 0.4rem 0 0.35rem;
  text-shadow: 0 2px 18px var(--black-a60);
}
.meta p {
  font-size: 0.9rem;
  color: var(--ink-2);
}
.badge {
  font-size: 0.7rem;
  font-weight: 600;
  padding: 0.18rem 0.55rem;
  border-radius: 0.4rem;
  display: inline-block;
}
.b-std {
  background: var(--cyan-a20);
  color: var(--brand-cyan);
}
.b-event {
  background: rgba(251, 113, 133, 0.18);
  color: rgb(251, 113, 133);
}
.arrow {
  position: absolute;
  top: 50%;
  transform: translateY(-50%);
  z-index: 5;
  width: 42px;
  height: 42px;
  border-radius: 999px;
  background: var(--black-a60);
  backdrop-filter: blur(8px);
  border: 1px solid var(--white-a20);
  color: var(--foreground);
  cursor: pointer;
  display: grid;
  place-items: center;
}
.arrow:hover {
  border-color: var(--brand-cyan);
  color: var(--brand-cyan);
}
.arrow.l {
  left: 0.9rem;
}
.arrow.r {
  right: 0.9rem;
}
.dots {
  position: absolute;
  bottom: 0.9rem;
  left: 50%;
  transform: translateX(-50%);
  display: flex;
  gap: 0.45rem;
  z-index: 5;
}
.dot {
  width: 8px;
  height: 8px;
  border-radius: 999px;
  background: var(--white-a30);
  cursor: pointer;
  transition: 0.25s;
  border: none;
  padding: 0;
}
.dot.on {
  width: 22px;
  background: var(--brand-cyan);
}
@media (max-width: 760px) {
  .hero-slider {
    height: 46vh;
    min-height: 300px;
  }
  .meta {
    max-width: 80%;
    left: 1.1rem;
  }
  .meta h3 {
    font-size: 1.6rem;
  }
}
</style>
