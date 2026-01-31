<template>
  <div v-if="anime" class="min-h-screen pb-20 md:pb-0">
    <!-- Hero Banner with Blurred Background -->
    <div class="relative h-[50vh] md:h-[60vh] overflow-hidden">
      <!-- Background Image -->
      <div
        class="absolute inset-0 bg-cover bg-center scale-110 blur-sm"
        :style="{ backgroundImage: `url(${anime.bannerImage || anime.coverImage})` }"
      />
      <!-- Gradient Overlays -->
      <div class="absolute inset-0 bg-gradient-to-t from-base via-base/70 to-transparent" />
      <div class="absolute inset-0 bg-gradient-to-r from-base/80 to-transparent" />
    </div>

    <!-- Main Content -->
    <div class="relative z-10 max-w-7xl mx-auto px-4 lg:px-8 -mt-64 md:-mt-72">
      <div class="flex flex-col md:flex-row gap-6 md:gap-8">
        <!-- Poster -->
        <div class="flex-shrink-0">
          <div class="w-40 md:w-56 aspect-[2/3] rounded-xl overflow-hidden shadow-2xl ring-1 ring-white/10">
            <img
              :src="anime.coverImage"
              :alt="anime.title"
              class="w-full h-full object-cover"
            />
          </div>
        </div>

        <!-- Info -->
        <div class="flex-1 pt-2">
          <!-- Title -->
          <h1 class="text-2xl md:text-4xl font-bold text-white mb-2">
            {{ anime.title }}
          </h1>
          <p v-if="(anime as AnimeWithExtras).japaneseTitle" class="text-lg text-white/50 mb-4">
            {{ (anime as AnimeWithExtras).japaneseTitle }}
          </p>

          <!-- Meta Info -->
          <div class="flex flex-wrap items-center gap-3 mb-4">
            <Badge :variant="statusVariant" size="md">
              {{ $t(`anime.status.${anime.status?.toLowerCase() || 'ongoing'}`) }}
            </Badge>
            <span class="text-white/60">{{ anime.releaseYear }}</span>
            <span class="text-white/30">•</span>
            <span class="text-white/60">{{ (anime as AnimeWithExtras).type || 'TV' }}</span>
            <span class="text-white/30">•</span>
            <span class="text-white/60">{{ anime.totalEpisodes }} {{ $t('anime.episodes') }}</span>
          </div>

          <!-- Ratings -->
          <div class="flex flex-wrap items-center gap-4 mb-4">
            <!-- Shikimori Rating -->
            <div v-if="anime.rating" class="flex items-center gap-2">
              <div class="flex items-center gap-1 text-amber-400">
                <svg class="w-5 h-5" fill="currentColor" viewBox="0 0 20 20">
                  <path d="M9.049 2.927c.3-.921 1.603-.921 1.902 0l1.07 3.292a1 1 0 00.95.69h3.462c.969 0 1.371 1.24.588 1.81l-2.8 2.034a1 1 0 00-.364 1.118l1.07 3.292c.3.921-.755 1.688-1.54 1.118l-2.8-2.034a1 1 0 00-1.175 0l-2.8 2.034c-.784.57-1.838-.197-1.539-1.118l1.07-3.292a1 1 0 00-.364-1.118L2.98 8.72c-.783-.57-.38-1.81.588-1.81h3.461a1 1 0 00.951-.69l1.07-3.292z" />
                </svg>
                <span class="font-bold text-lg">{{ anime.rating.toFixed(1) }}</span>
              </div>
              <span class="text-white/40 text-sm">Shikimori</span>
            </div>

            <!-- Site Rating -->
            <div v-if="siteRating && siteRating.total_reviews > 0" class="flex items-center gap-2">
              <div class="flex items-center gap-1 text-cyan-400">
                <svg class="w-5 h-5" fill="currentColor" viewBox="0 0 20 20">
                  <path d="M9.049 2.927c.3-.921 1.603-.921 1.902 0l1.07 3.292a1 1 0 00.95.69h3.462c.969 0 1.371 1.24.588 1.81l-2.8 2.034a1 1 0 00-.364 1.118l1.07 3.292c.3.921-.755 1.688-1.54 1.118l-2.8-2.034a1 1 0 00-1.175 0l-2.8 2.034c-.784.57-1.838-.197-1.539-1.118l1.07-3.292a1 1 0 00-.364-1.118L2.98 8.72c-.783-.57-.38-1.81.588-1.81h3.461a1 1 0 00.951-.69l1.07-3.292z" />
                </svg>
                <span class="font-bold text-lg">{{ siteRating.average_score.toFixed(1) }}</span>
              </div>
              <span class="text-white/40 text-sm">AnimeEnigma ({{ siteRating.total_reviews }})</span>
            </div>
          </div>

          <!-- Actions -->
          <div class="flex flex-wrap items-center gap-3 mb-6">
            <!-- Watchlist Status Dropdown -->
            <div class="relative" ref="dropdownRef">
              <button
                @click="showStatusDropdown = !showStatusDropdown"
                class="flex items-center gap-2 px-4 py-2.5 rounded-lg font-medium transition-all"
                :class="currentListStatus
                  ? 'bg-cyan-500/20 text-cyan-400 border border-cyan-500/30 hover:bg-cyan-500/30'
                  : 'bg-white/5 text-white border border-white/10 hover:bg-white/10'"
              >
                <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path v-if="currentListStatus" stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
                  <path v-else stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4" />
                </svg>
                <span>{{ currentListStatus ? statusLabels[currentListStatus] : 'В список' }}</span>
                <svg class="w-4 h-4 transition-transform" :class="{ 'rotate-180': showStatusDropdown }" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7" />
                </svg>
              </button>

              <!-- Dropdown Menu -->
              <Transition
                enter-active-class="transition ease-out duration-100"
                enter-from-class="transform opacity-0 scale-95"
                enter-to-class="transform opacity-100 scale-100"
                leave-active-class="transition ease-in duration-75"
                leave-from-class="transform opacity-100 scale-100"
                leave-to-class="transform opacity-0 scale-95"
              >
                <div
                  v-if="showStatusDropdown"
                  class="absolute top-full left-0 mt-2 w-48 rounded-xl bg-surface border border-white/10 shadow-xl overflow-hidden z-50"
                >
                  <button
                    v-for="(label, status) in statusLabels"
                    :key="status"
                    @click="setListStatus(status)"
                    class="w-full px-4 py-3 text-left text-sm transition-colors flex items-center justify-between"
                    :class="currentListStatus === status
                      ? 'bg-cyan-500/20 text-cyan-400'
                      : 'text-white/80 hover:bg-white/5 hover:text-white'"
                  >
                    {{ label }}
                    <svg v-if="currentListStatus === status" class="w-4 h-4" fill="currentColor" viewBox="0 0 20 20">
                      <path fill-rule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clip-rule="evenodd" />
                    </svg>
                  </button>

                  <!-- Remove from list -->
                  <div v-if="currentListStatus" class="border-t border-white/10">
                    <button
                      @click="removeFromList"
                      class="w-full px-4 py-3 text-left text-sm text-pink-400 hover:bg-pink-500/10 transition-colors flex items-center gap-2"
                    >
                      <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
                      </svg>
                      Удалить из списка
                    </button>
                  </div>
                </div>
              </Transition>
            </div>
          </div>

          <!-- Genres -->
          <div class="flex flex-wrap gap-2">
            <GenreChip
              v-for="genre in anime.genres"
              :key="genre"
              :genre="genre"
            />
          </div>
        </div>
      </div>

      <!-- Synopsis -->
      <section class="mt-8">
        <h2 class="text-xl font-semibold text-white mb-3">{{ $t('anime.synopsis') }}</h2>
        <div class="glass-card p-4">
          <p
            class="text-white/70 leading-relaxed"
            :class="{ 'line-clamp-4': !synopsisExpanded }"
          >
            {{ anime.description }}
          </p>
          <button
            v-if="anime.description && anime.description.length > 300"
            class="mt-2 text-cyan-400 hover:text-cyan-300 transition-colors text-sm"
            @click="synopsisExpanded = !synopsisExpanded"
          >
            {{ synopsisExpanded ? 'Скрыть' : 'Показать полностью' }}
          </button>
        </div>
      </section>

      <!-- Kodik Player -->
      <section class="mt-8">
        <div class="flex items-center justify-between mb-4">
          <h2 class="text-xl font-semibold text-white">
            <span class="flex items-center gap-2">
              <svg class="w-6 h-6 text-cyan-400" fill="currentColor" viewBox="0 0 24 24">
                <path d="M8 5v14l11-7z" />
              </svg>
              {{ $t('anime.watch') || 'Смотреть онлайн' }}
            </span>
          </h2>
        </div>
        <div class="glass-card p-4 md:p-6">
          <KodikPlayer
            :anime-id="anime.id"
            :total-episodes="anime.totalEpisodes"
          />
        </div>
      </section>

      <!-- Reviews Section -->
      <section class="mt-8">
        <div class="flex items-center justify-between mb-4">
          <h2 class="text-xl font-semibold text-white">
            <span class="flex items-center gap-2">
              <svg class="w-6 h-6 text-cyan-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 8h10M7 12h4m1 8l-4-4H5a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v8a2 2 0 01-2 2h-3l-4 4z" />
              </svg>
              Отзывы
            </span>
          </h2>
          <span v-if="reviews.length > 0" class="text-white/40 text-sm">{{ reviews.length }} отзывов</span>
        </div>

        <!-- Write Review Form -->
        <div v-if="authStore.isAuthenticated" class="glass-card p-4 md:p-6 mb-6">
          <h3 class="text-lg font-medium text-white mb-4">
            {{ myReview ? 'Изменить отзыв' : 'Написать отзыв' }}
          </h3>

          <!-- Star Rating -->
          <div class="mb-4">
            <label class="block text-white/60 text-sm mb-2">Ваша оценка</label>
            <div class="flex gap-1">
              <button
                v-for="star in 10"
                :key="star"
                @click="reviewForm.score = star"
                class="p-1 transition-transform hover:scale-110"
              >
                <svg
                  class="w-8 h-8 transition-colors"
                  :class="star <= reviewForm.score ? 'text-amber-400' : 'text-white/20'"
                  fill="currentColor"
                  viewBox="0 0 20 20"
                >
                  <path d="M9.049 2.927c.3-.921 1.603-.921 1.902 0l1.07 3.292a1 1 0 00.95.69h3.462c.969 0 1.371 1.24.588 1.81l-2.8 2.034a1 1 0 00-.364 1.118l1.07 3.292c.3.921-.755 1.688-1.54 1.118l-2.8-2.034a1 1 0 00-1.175 0l-2.8 2.034c-.784.57-1.838-.197-1.539-1.118l1.07-3.292a1 1 0 00-.364-1.118L2.98 8.72c-.783-.57-.38-1.81.588-1.81h3.461a1 1 0 00.951-.69l1.07-3.292z" />
                </svg>
              </button>
            </div>
            <p v-if="reviewForm.score > 0" class="text-cyan-400 text-sm mt-1">{{ reviewForm.score }}/10</p>
          </div>

          <!-- Review Text -->
          <div class="mb-4">
            <label class="block text-white/60 text-sm mb-2">Ваш отзыв (необязательно)</label>
            <textarea
              v-model="reviewForm.text"
              rows="4"
              class="w-full bg-white/5 border border-white/10 rounded-lg px-4 py-3 text-white placeholder-white/30 focus:outline-none focus:border-cyan-500 transition-colors resize-none"
              placeholder="Поделитесь своими впечатлениями об аниме..."
            ></textarea>
          </div>

          <!-- Submit Buttons -->
          <div class="flex gap-3">
            <button
              @click="submitReview"
              :disabled="reviewForm.score === 0 || reviewSubmitting"
              class="px-6 py-2.5 bg-cyan-500 hover:bg-cyan-400 text-black font-medium rounded-lg transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {{ reviewSubmitting ? 'Сохранение...' : (myReview ? 'Обновить' : 'Опубликовать') }}
            </button>
            <button
              v-if="myReview"
              @click="deleteMyReview"
              class="px-6 py-2.5 bg-pink-500/20 hover:bg-pink-500/30 text-pink-400 font-medium rounded-lg transition-colors"
            >
              Удалить
            </button>
          </div>
        </div>

        <!-- Login prompt -->
        <div v-else class="glass-card p-6 mb-6 text-center">
          <p class="text-white/60 mb-3">Войдите, чтобы оставить отзыв</p>
          <router-link
            to="/auth"
            class="inline-block px-6 py-2.5 bg-cyan-500 hover:bg-cyan-400 text-black font-medium rounded-lg transition-colors"
          >
            Войти
          </router-link>
        </div>

        <!-- Reviews List -->
        <div v-if="reviews.length > 0" class="space-y-4">
          <div
            v-for="review in reviews"
            :key="review.id"
            class="glass-card p-4"
          >
            <div class="flex items-start justify-between mb-2">
              <div class="flex items-center gap-3">
                <div class="w-10 h-10 rounded-full bg-cyan-500/20 flex items-center justify-center text-cyan-400 font-bold">
                  {{ review.username?.slice(0, 2).toUpperCase() || '??' }}
                </div>
                <div>
                  <p class="font-medium text-white">{{ review.username || 'Пользователь' }}</p>
                  <p class="text-white/40 text-sm">{{ formatDate(review.created_at) }}</p>
                </div>
              </div>
              <div class="flex items-center gap-1 text-amber-400">
                <svg class="w-5 h-5" fill="currentColor" viewBox="0 0 20 20">
                  <path d="M9.049 2.927c.3-.921 1.603-.921 1.902 0l1.07 3.292a1 1 0 00.95.69h3.462c.969 0 1.371 1.24.588 1.81l-2.8 2.034a1 1 0 00-.364 1.118l1.07 3.292c.3.921-.755 1.688-1.54 1.118l-2.8-2.034a1 1 0 00-1.175 0l-2.8 2.034c-.784.57-1.838-.197-1.539-1.118l1.07-3.292a1 1 0 00-.364-1.118L2.98 8.72c-.783-.57-.38-1.81.588-1.81h3.461a1 1 0 00.951-.69l1.07-3.292z" />
                </svg>
                <span class="font-bold">{{ review.score }}</span>
              </div>
            </div>
            <p v-if="review.review_text" class="text-white/70 whitespace-pre-wrap">{{ review.review_text }}</p>
          </div>
        </div>

        <div v-else class="glass-card p-8 text-center">
          <p class="text-white/50">Пока нет отзывов. Будьте первым!</p>
        </div>
      </section>

      <!-- Related Anime -->
      <section v-if="relatedAnime.length > 0" class="mt-8">
        <Carousel
          :items="relatedAnime"
          :title="$t('anime.related')"
          item-key="id"
        >
          <template #default="{ item }">
            <AnimeCardNew :anime="(item as RelatedAnime)" />
          </template>
        </Carousel>
      </section>
    </div>
  </div>

  <!-- Loading State -->
  <div v-else-if="loading" class="min-h-screen flex items-center justify-center">
    <div class="text-center">
      <div class="w-12 h-12 border-2 border-cyan-400 border-t-transparent rounded-full animate-spin mx-auto mb-4" />
      <p class="text-white/60">{{ $t('common.loading') }}</p>
    </div>
  </div>

  <!-- Error State -->
  <div v-else-if="error" class="min-h-screen flex items-center justify-center">
    <div class="text-center">
      <p class="text-pink-400 mb-4">{{ error }}</p>
      <Button variant="outline" @click="retry">{{ $t('common.retry') }}</Button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted, onUnmounted } from 'vue'
import { useRoute } from 'vue-router'
import { useAnime } from '@/composables/useAnime'
import { useAuthStore } from '@/stores/auth'
import { Badge, Button } from '@/components/ui'
import { GenreChip, AnimeCardNew } from '@/components/anime'
import { Carousel } from '@/components/carousel'
import KodikPlayer from '@/components/player/KodikPlayer.vue'
import { userApi, reviewApi } from '@/api/client'

interface AnimeWithExtras {
  japaneseTitle?: string
  type?: string
}

interface RelatedAnime {
  id: string
  title: string
  coverImage: string
  rating?: number
  releaseYear?: number
  episodes?: number
  genres?: string[]
}

interface Review {
  id: string
  user_id: string
  anime_id: string
  username: string
  score: number
  review_text: string
  created_at: string
}

interface AnimeRating {
  anime_id: string
  average_score: number
  total_reviews: number
}

const route = useRoute()
const authStore = useAuthStore()
const { anime, loading, error, fetchAnime } = useAnime()

const synopsisExpanded = ref(false)
const currentListStatus = ref<string | null>(null)
const showStatusDropdown = ref(false)
const dropdownRef = ref<HTMLElement | null>(null)
const relatedAnime = ref<RelatedAnime[]>([])

// Reviews
const reviews = ref<Review[]>([])
const myReview = ref<Review | null>(null)
const siteRating = ref<AnimeRating | null>(null)
const reviewSubmitting = ref(false)
const reviewForm = reactive({
  score: 0,
  text: '',
})

const statusLabels: Record<string, string> = {
  watching: 'Смотрю',
  plan_to_watch: 'Запланировано',
  completed: 'Просмотрено',
  on_hold: 'Отложено',
  dropped: 'Брошено',
}

const statusVariant = computed(() => {
  const status = anime.value?.status?.toLowerCase()
  if (status === 'completed') return 'success'
  if (status === 'upcoming') return 'warning'
  return 'primary' // ongoing
})

const formatDate = (dateStr: string) => {
  const date = new Date(dateStr)
  return date.toLocaleDateString('ru-RU', {
    day: 'numeric',
    month: 'long',
    year: 'numeric',
  })
}

const fetchWatchlistStatus = async () => {
  if (!authStore.isAuthenticated || !anime.value) return

  try {
    const response = await userApi.getWatchlist()
    const entries = response.data?.data || response.data || []
    const entry = entries.find((e: any) => e.anime_id === anime.value?.id)
    if (entry) {
      currentListStatus.value = entry.status
    } else {
      currentListStatus.value = null
    }
  } catch (err) {
    console.error('Failed to fetch watchlist status:', err)
  }
}

const fetchReviews = async () => {
  if (!anime.value) return

  try {
    // Fetch reviews
    const reviewsResponse = await reviewApi.getAnimeReviews(anime.value.id)
    reviews.value = reviewsResponse.data?.data || reviewsResponse.data || []

    // Fetch rating
    const ratingResponse = await reviewApi.getAnimeRating(anime.value.id)
    siteRating.value = ratingResponse.data?.data || ratingResponse.data

    // Fetch user's review if authenticated
    if (authStore.isAuthenticated) {
      try {
        const myReviewResponse = await reviewApi.getMyReview(anime.value.id)
        const review = myReviewResponse.data?.data || myReviewResponse.data
        if (review && review.id) {
          myReview.value = review
          reviewForm.score = review.score
          reviewForm.text = review.review_text || ''
        }
      } catch {
        // No review from this user
      }
    }
  } catch (err) {
    console.error('Failed to fetch reviews:', err)
  }
}

const submitReview = async () => {
  if (!anime.value || reviewForm.score === 0) return

  reviewSubmitting.value = true
  try {
    await reviewApi.createReview(
      anime.value.id,
      reviewForm.score,
      reviewForm.text,
      anime.value.title,
      anime.value.coverImage
    )
    await fetchReviews()
  } catch (err) {
    console.error('Failed to submit review:', err)
  } finally {
    reviewSubmitting.value = false
  }
}

const deleteMyReview = async () => {
  if (!anime.value) return

  try {
    await reviewApi.deleteReview(anime.value.id)
    myReview.value = null
    reviewForm.score = 0
    reviewForm.text = ''
    await fetchReviews()
  } catch (err) {
    console.error('Failed to delete review:', err)
  }
}

const setListStatus = async (status: string) => {
  if (!anime.value) return

  try {
    await userApi.updateWatchlistStatus(
      anime.value.id,
      status,
      anime.value.title,
      anime.value.coverImage
    )
    currentListStatus.value = status
    showStatusDropdown.value = false
  } catch (err) {
    console.error('Failed to update list status:', err)
  }
}

const removeFromList = async () => {
  if (!anime.value) return

  try {
    await userApi.removeFromWatchlist(anime.value.id)
    currentListStatus.value = null
    showStatusDropdown.value = false
  } catch (err) {
    console.error('Failed to remove from list:', err)
  }
}

// Close dropdown on click outside
const handleClickOutside = (event: MouseEvent) => {
  if (dropdownRef.value && !dropdownRef.value.contains(event.target as Node)) {
    showStatusDropdown.value = false
  }
}

const retry = () => {
  const animeId = route.params.id as string
  fetchAnime(animeId)
}

onMounted(async () => {
  const animeId = route.params.id as string
  await fetchAnime(animeId)
  await fetchWatchlistStatus()
  await fetchReviews()
  document.addEventListener('click', handleClickOutside)
})

onUnmounted(() => {
  document.removeEventListener('click', handleClickOutside)
})
</script>
