<template>
  <article
    class="w-full h-full flex flex-col gap-3 p-4 md:p-4 lg:p-6"
  >
    <header>
      <h3
        class="text-lg md:text-xl font-semibold text-white"
      >
        {{ t('spotlight.telegramNews.title') }}
      </h3>
    </header>

    <div
      class="flex-1 grid grid-cols-1 md:grid-cols-3 gap-3 md:gap-4 min-h-0"
    >
      <article
        v-for="(post, i) in data.posts.slice(0, 3)"
        :key="post.link ?? `tg-${i}`"
        class="flex flex-col gap-1 p-3 rounded-xl bg-white/5 hover:bg-white/10 transition-colors min-w-0"
      >
        <h4
          v-if="post.title"
          class="text-sm font-semibold text-white truncate"
        >
          {{ post.title }}
        </h4>
        <p
          class="text-sm font-medium text-gray-300 line-clamp-2"
        >
          {{ post.excerpt }}
        </p>
        <p
          v-if="post.date"
          class="text-xs font-medium text-gray-500"
        >
          {{ post.date }}
        </p>
        <a
          v-if="post.link"
          :href="post.link"
          target="_blank"
          rel="noopener noreferrer"
          class="text-xs font-medium text-cyan-400 hover:text-cyan-300 transition-colors mt-auto"
        >
          {{ t('spotlight.telegramNews.openCta') }}
        </a>
      </article>
    </div>
  </article>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { TelegramNewsData } from '@/types/spotlight'

defineProps<{ data: TelegramNewsData }>()
const { t } = useI18n()
</script>
