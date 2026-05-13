<template>
  <!-- Phase 14 / UX-30 — public marketing surface. Native <details> for
       zero-JS accordion behavior (keyboard-accessible by default, SEO-
       friendly because content is always in the DOM). -->
  <div class="max-w-3xl mx-auto px-4 py-12">
    <header class="mb-8">
      <h1 class="text-3xl md:text-4xl font-bold text-white mb-2">
        {{ t('about.title') }}
      </h1>
      <p class="text-white/60 text-lg">
        {{ t('about.subtitle') }}
      </p>
    </header>

    <section :aria-label="t('about.title')">
      <details
        v-for="item in faqs"
        :key="item.qKey"
        class="border-b border-white/10 py-3 group"
      >
        <summary
          class="cursor-pointer text-lg font-medium text-white flex items-center justify-between list-none"
        >
          <span>{{ t(item.qKey) }}</span>
          <svg
            class="w-5 h-5 transition-transform group-open:rotate-180 text-white/40 flex-shrink-0 ml-3"
            fill="none"
            stroke="currentColor"
            viewBox="0 0 24 24"
            aria-hidden="true"
          >
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              stroke-width="2"
              d="M19 9l-7 7-7-7"
            />
          </svg>
        </summary>
        <p class="mt-3 text-white/70 leading-relaxed">{{ t(item.aKey) }}</p>
      </details>
    </section>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'

const { t } = useI18n()

// Phase 14 / UX-30 — eight curated FAQs covering platform overview,
// monetization, sources, recommendations, MAL import, mobile, bug
// reports, and ownership. Keys live under `about.faqs.qN.{q,a}` in
// en/ru/ja locale files.
const faqs = [
  { qKey: 'about.faqs.q1.q', aKey: 'about.faqs.q1.a' },
  { qKey: 'about.faqs.q2.q', aKey: 'about.faqs.q2.a' },
  { qKey: 'about.faqs.q3.q', aKey: 'about.faqs.q3.a' },
  { qKey: 'about.faqs.q4.q', aKey: 'about.faqs.q4.a' },
  { qKey: 'about.faqs.q5.q', aKey: 'about.faqs.q5.a' },
  { qKey: 'about.faqs.q6.q', aKey: 'about.faqs.q6.a' },
  { qKey: 'about.faqs.q7.q', aKey: 'about.faqs.q7.a' },
  { qKey: 'about.faqs.q8.q', aKey: 'about.faqs.q8.a' },
]
</script>

<style scoped>
/* Hide the default disclosure triangle so our chevron is the only indicator. */
summary::-webkit-details-marker {
  display: none;
}
</style>
