<template>
  <div class="min-h-screen">
    <!-- Hero Section -->
    <Hero
      background-image="/images/hero-bg.jpg"
      @signin="showLoginModal"
    />

    <!-- Main Content -->
    <div class="relative z-10 -mt-20 space-y-12 pb-12">
      <!-- Continue Watching (logged in users with history) -->
      <section v-if="authStore.isAuthenticated && continueWatching.length > 0">
        <Carousel
          :items="continueWatching"
          :title="$t('home.continueWatching')"
          item-key="id"
        >
          <template #default="{ item }">
            <ContinueCard
              :anime="(item as ContinueWatchingItem).anime"
              :current-episode="(item as ContinueWatchingItem).currentEpisode"
              :total-episodes="(item as ContinueWatchingItem).totalEpisodes"
              :progress="(item as ContinueWatchingItem).progress"
            />
          </template>
        </Carousel>
      </section>

      <!-- Recommended for You (logged in users) -->
      <section v-if="authStore.isAuthenticated && recommendedAnime.length > 0">
        <Carousel
          :items="recommendedAnime"
          :title="$t('home.recommended')"
          see-all-link="/browse?filter=recommended"
          item-key="id"
        >
          <template #default="{ item }">
            <AnimeCardNew :anime="(item as Anime)" />
          </template>
        </Carousel>
      </section>

      <!-- Trending Now -->
      <section>
        <Carousel
          :items="trendingAnime"
          :title="$t('home.trending')"
          see-all-link="/browse?sort=trending"
          item-key="id"
        >
          <template #default="{ item }">
            <AnimeCardNew :anime="(item as Anime)" />
          </template>
        </Carousel>
      </section>

      <!-- New Episodes -->
      <section v-if="newEpisodes.length > 0">
        <Carousel
          :items="newEpisodes"
          :title="$t('home.newEpisodes')"
          see-all-link="/browse?sort=new"
          item-key="id"
        >
          <template #default="{ item }">
            <AnimeCardNew :anime="(item as Anime)" />
          </template>
        </Carousel>
      </section>

      <!-- Genres -->
      <section class="px-4 lg:px-8 max-w-7xl mx-auto">
        <h2 class="text-xl md:text-2xl font-bold text-white mb-4">
          {{ $t('home.genres') }}
        </h2>
        <div class="flex flex-wrap gap-3">
          <GenreChip
            v-for="genre in genres"
            :key="genre.name"
            :genre="genre.name"
            :label="genre.localizedName"
          />
        </div>
      </section>

      <!-- Popular -->
      <section>
        <Carousel
          :items="popularAnime"
          :title="$t('home.popular')"
          see-all-link="/browse?sort=popular"
          item-key="id"
        >
          <template #default="{ item }">
            <AnimeCardNew :anime="(item as Anime)" />
          </template>
        </Carousel>
      </section>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '@/stores/auth'
import { useAnime } from '@/composables/useAnime'
import { Hero } from '@/components/hero'
import { Carousel } from '@/components/carousel'
import { AnimeCardNew, ContinueCard, GenreChip } from '@/components/anime'

interface Anime {
  id: string
  title: string
  coverImage: string
  rating?: number
  releaseYear?: number
  episodes?: number
  status?: string
  genres?: string[]
  quality?: string
}

interface ContinueWatchingItem {
  id: string
  anime: Anime
  currentEpisode: number
  totalEpisodes: number
  progress: number
}

interface Genre {
  name: string
  localizedName: string
}

const { locale } = useI18n()
const authStore = useAuthStore()
const { fetchTrending, fetchPopular } = useAnime()

const trendingAnime = ref<Anime[]>([])
const popularAnime = ref<Anime[]>([])
const newEpisodes = ref<Anime[]>([])
const recommendedAnime = ref<Anime[]>([])
const continueWatching = ref<ContinueWatchingItem[]>([])

const genres = ref<Genre[]>([
  { name: 'Action', localizedName: locale.value === 'ru' ? 'Экшен' : 'Action' },
  { name: 'Adventure', localizedName: locale.value === 'ru' ? 'Приключения' : 'Adventure' },
  { name: 'Comedy', localizedName: locale.value === 'ru' ? 'Комедия' : 'Comedy' },
  { name: 'Drama', localizedName: locale.value === 'ru' ? 'Драма' : 'Drama' },
  { name: 'Fantasy', localizedName: locale.value === 'ru' ? 'Фэнтези' : 'Fantasy' },
  { name: 'Romance', localizedName: locale.value === 'ru' ? 'Романтика' : 'Romance' },
  { name: 'Sci-Fi', localizedName: locale.value === 'ru' ? 'Научная фантастика' : 'Sci-Fi' },
  { name: 'Slice of Life', localizedName: locale.value === 'ru' ? 'Повседневность' : 'Slice of Life' },
  { name: 'Sports', localizedName: locale.value === 'ru' ? 'Спорт' : 'Sports' },
  { name: 'Supernatural', localizedName: locale.value === 'ru' ? 'Сверхъестественное' : 'Supernatural' },
  { name: 'Mecha', localizedName: locale.value === 'ru' ? 'Меха' : 'Mecha' },
  { name: 'Isekai', localizedName: locale.value === 'ru' ? 'Исекай' : 'Isekai' },
])

const showLoginModal = () => {
  // TODO: Implement login modal
  console.log('Show login modal')
}

onMounted(async () => {
  try {
    // Fetch anime data
    const [trending, popular] = await Promise.all([
      fetchTrending(),
      fetchPopular(),
    ])

    trendingAnime.value = trending || []
    popularAnime.value = popular || []

    // Mock new episodes (would come from API)
    newEpisodes.value = trending?.slice(0, 10) || []

    // Mock recommended for authenticated users
    if (authStore.isAuthenticated) {
      recommendedAnime.value = popular?.slice(0, 10) || []

      // Mock continue watching
      continueWatching.value = trending?.slice(0, 5).map((anime: Anime, index: number) => ({
        id: `cw-${anime.id}`,
        anime,
        currentEpisode: index + 1,
        totalEpisodes: 12,
        progress: Math.floor(Math.random() * 80) + 10,
      })) || []
    }
  } catch (err) {
    console.error('Failed to load anime data:', err)
  }
})
</script>
