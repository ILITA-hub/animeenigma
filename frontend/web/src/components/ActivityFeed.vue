<template>
  <!-- .activity shell from Neon Tokyo handoff -->
  <div class="activity-shell">
    <!-- Section header -->
    <div class="section-head">
      <h2 class="section-title">{{ $t('activity.title') }}</h2>
    </div>

    <!-- Loading skeleton -->
    <div v-if="loading && events.length === 0" class="feed-list">
      <div v-for="i in 4" :key="i" class="feed-item-skeleton">
        <div class="skeleton-av" />
        <div class="skeleton-body">
          <div class="skeleton-line skel-w-1q" />
          <div class="skeleton-line skel-w-3q" />
          <div class="skeleton-line skel-w-1-3" />
        </div>
        <div class="skeleton-poster" />
      </div>
    </div>

    <!-- Events list -->
    <div v-else class="feed-list">
      <div
        v-for="event in events"
        :key="event.id"
        class="feed-item"
      >
        <!-- 28px user avatar (falls back to the username initials) -->
        <router-link
          :to="`/user/${event.public_id || event.user_id}`"
          class="flex-shrink-0"
          tabindex="-1"
          aria-hidden="true"
        >
          <Avatar
            :src="event.user_avatar"
            :name="event.username"
            size="xs"
            class="size-7 ring-1 ring-white/10"
          />
        </router-link>

        <!-- Text block -->
        <div class="feed-text-block">
          <div class="feed-text">
            <router-link
              :to="`/user/${event.public_id || event.user_id}`"
              class="feed-who"
            >@{{ event.username }}</router-link>{{ ' ' }}<span class="feed-action">{{ actionText(event) }}</span>{{ ' ' }}<router-link
              :to="`/anime/${event.anime_id}`"
              class="feed-ttl"
            >{{ animeName(event) }}</router-link>
          </div>
          <p v-if="event.content" class="feed-excerpt">{{ event.content }}</p>
          <div class="feed-time">{{ formatRelativeTime(event.created_at) }}</div>
        </div>

        <!-- Anime poster thumbnail — right-anchored, the subject of the
             activity. Decorative: the title link above already routes to the
             same anime, so it's hidden from the a11y tree (like the avatar). -->
        <router-link
          v-if="event.anime?.poster_url"
          :to="`/anime/${event.anime_id}`"
          class="feed-poster"
          tabindex="-1"
          aria-hidden="true"
        >
          <img
            :src="cardPosterUrl(event.anime.poster_url, 128)"
            :alt="animeName(event)"
            class="feed-poster-img"
            loading="lazy"
            @error="onPosterError"
          />
        </router-link>
      </div>

      <!-- Empty state -->
      <div v-if="events.length === 0 && !loading" class="feed-empty">
        {{ $t('activity.empty') }}
      </div>

      <!-- Load more -->
      <button
        v-if="hasMore"
        type="button"
        @click="loadMore"
        :disabled="loading"
        class="feed-load-more"
      >
        {{ loading ? $t('common.loading') : $t('activity.loadMore') }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import Avatar from '@/components/ui/Avatar.vue'
import { activityApi } from '@/api/client'
import { cardPosterUrl } from '@/composables/useImageProxy'
import { getLocalizedTitle } from '@/utils/title'

interface ActivityEvent {
  id: string
  user_id: string
  // REVIEW.md WR-06: optional `public_id` (the user-chosen slug used by
  // the /user/:publicId route). When present, the username link routes
  // directly to the public profile URL; when absent, we fall back to
  // user_id which the auth service's GetUserByPublicID handler resolves
  // (UUID lookup → silent redirect). The backend may start populating
  // this field by joining activity_events with users.
  public_id?: string
  username: string
  // Populated by the backend feed from the users table (current avatar).
  user_avatar?: string
  anime_id: string
  anime?: {
    id: string
    name: string
    name_ru?: string
    poster_url?: string
  }
  type: string
  old_value: string
  new_value: string
  content?: string
  created_at: string
}

const { t, locale } = useI18n()
const events = ref<ActivityEvent[]>([])
const hasMore = ref(false)
const loading = ref(true)

const loadFeed = async (before?: string) => {
  loading.value = true
  try {
    const response = await activityApi.getFeed(10, before)
    const data = response.data?.data || response.data
    const newEvents: ActivityEvent[] = data?.events || []
    if (before) {
      events.value.push(...newEvents)
    } else {
      events.value = newEvents
    }
    hasMore.value = data?.has_more || false
  } catch (err) {
    console.error('Failed to load activity feed:', err)
  } finally {
    loading.value = false
  }
}

const loadMore = () => {
  if (events.value.length > 0) {
    const lastEvent = events.value[events.value.length - 1]
    loadFeed(lastEvent.id)
  }
}

const actionText = (event: ActivityEvent): string => {
  if (event.type === 'comment') {
    return t('activity.comment.posted')
  }
  if (event.type === 'score') {
    return t('activity.score', { score: event.new_value })
  }
  if (event.type === 'review') {
    if (event.old_value === 'score') {
      return t('activity.score', { score: event.new_value })
    }
    const key = event.old_value === 'new' ? 'activity.review.wrote' : 'activity.review.updated'
    return t(key, { score: event.new_value })
  }
  return t(`activity.status.${event.new_value}`)
}

const animeName = (event: ActivityEvent): string => {
  if (!event.anime) return t('home.noData')
  return getLocalizedTitle(event.anime.name, event.anime.name_ru) || t('home.noData')
}

// A broken poster <img> renders its (long) alt text inside the 40×60 slot,
// ballooning the row height. Swap to the placeholder on error (guard against
// a loop if the placeholder itself ever fails). Mirrors ColumnItem.vue.
const onPosterError = (e: Event): void => {
  const img = e.target as HTMLImageElement
  if (img.src.endsWith('/placeholder.svg')) return
  img.src = '/placeholder.svg'
}

const formatRelativeTime = (dateStr: string): string => {
  const date = new Date(dateStr)
  const now = new Date()
  const diffMs = now.getTime() - date.getTime()
  const diffMinutes = Math.floor(diffMs / (1000 * 60))
  const diffHours = Math.floor(diffMs / (1000 * 60 * 60))
  const diffDays = Math.floor(diffMs / (1000 * 60 * 60 * 24))

  if (diffMinutes < 1) return t('time.justNow')
  if (diffMinutes < 60) return t('time.minutesAgo', { n: diffMinutes })
  if (diffHours < 24) return t('time.hoursAgo', { n: diffHours })
  if (diffDays === 1) return t('common.yesterday')
  if (diffDays < 7) return t('time.daysAgo', { n: diffDays })
  return date.toLocaleDateString(locale.value === 'ru' ? 'ru-RU' : locale.value === 'ja' ? 'ja-JP' : 'en-US', { day: 'numeric', month: 'short' })
}

onMounted(() => {
  loadFeed()
})
</script>

<style scoped>
/* Neon Tokyo .activity shell */
.activity-shell {
  background: rgba(255, 255, 255, 0.025);
  border: 1px solid var(--line);
  border-radius: var(--r-xl);
  padding: 18px;
  /* Cap + internal scroll so this panel stays the same height as the
     side-by-side LastUpdates panel. MUST match LastUpdates'
     .activity-shell max-height. The feed list below becomes the
     flex-1 scroll region. */
  display: flex;
  flex-direction: column;
  max-height: 600px;
}

/* Section header */
.section-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 14px;
}

.section-title {
  font-family: var(--f-display);
  font-size: 17px;
  font-weight: 700;
  letter-spacing: -0.01em;
  color: var(--foreground);
}

/* Feed list — flex-1 scroll region within the capped .activity-shell */
.feed-list {
  display: flex;
  flex-direction: column;
  gap: 14px;
  flex: 1;
  min-height: 0;
  overflow-y: auto;
}

/* Scrollbar — matches HomeColumn/.col-list + LastUpdates styling */
.feed-list::-webkit-scrollbar { width: 4px; }
.feed-list::-webkit-scrollbar-track { background: transparent; }
.feed-list::-webkit-scrollbar-thumb { background: rgba(255, 255, 255, 0.1); border-radius: 2px; }
.feed-list::-webkit-scrollbar-thumb:hover { background: rgba(255, 255, 255, 0.2); }

/* .feed-item from handoff */
.feed-item {
  display: flex;
  gap: 12px;
  align-items: flex-start;
  /* Right gutter so the right-anchored .feed-poster clears the 4px scroll
     thumb instead of colliding with it (matches the row-level padding
     HomeColumn/.item and LastUpdates/.update-row use to clear the same
     scrollbar). Content-box spacing, so it holds for overlay scrollbars too. */
  padding-right: 10px;
}

/* Text block */
.feed-text-block {
  flex: 1;
  min-width: 0;
}

/* Anime poster thumbnail — 2:3 portrait, right-anchored. Matches the app's
   poster convention (--r-sm radius, --line border, object-fit cover). */
.feed-poster {
  flex-shrink: 0;
  display: block;
  width: 40px;
  height: 60px;
  border-radius: var(--r-sm);
  overflow: hidden;
  border: 1px solid var(--line);
  text-decoration: none;
}

.feed-poster-img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  display: block;
  transition: opacity 0.15s ease;
}
.feed-poster:hover .feed-poster-img {
  opacity: 0.82;
}

.feed-text {
  font-size: 13px;
  line-height: 1.45;
  color: var(--ink-2);
}

/* @username bold */
.feed-who {
  font-weight: 600;
  color: var(--foreground);
  text-decoration: none;
  transition: color 0.15s ease;
}
.feed-who:hover {
  color: var(--brand-cyan);
}

.feed-action {
  color: var(--ink-2);
}

/* Anime title in accent color */
.feed-ttl {
  color: var(--brand-cyan);
  text-decoration: none;
  transition: color 0.15s ease;
}
.feed-ttl:hover {
  color: var(--foreground);
}

/* Optional italic excerpt */
.feed-excerpt {
  font-size: 12px;
  color: var(--muted-foreground);
  font-style: italic;
  margin-top: 4px;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

/* Mono timestamp */
.feed-time {
  font-family: var(--f-mono);
  font-size: 11px;
  color: var(--ink-4);
  margin-top: 2px;
}

/* Empty state */
.feed-empty {
  text-align: center;
  padding: 32px 0;
  color: var(--ink-4);
  font-size: 13px;
}

/* Load more */
.feed-load-more {
  width: 100%;
  margin-top: 4px;
  padding: 8px 0;
  font-size: 13px;
  color: var(--muted-foreground);
  background: rgba(255, 255, 255, 0.04);
  border: 1px solid var(--line);
  border-radius: var(--r-md);
  cursor: pointer;
  transition: background 0.15s ease, color 0.15s ease;
}
.feed-load-more:hover {
  background: rgba(255, 255, 255, 0.08);
  color: var(--foreground);
}
.feed-load-more:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

/* Loading skeleton */
.feed-item-skeleton {
  display: flex;
  gap: 12px;
  align-items: flex-start;
  /* Match .feed-item so the skeleton poster sits in the same slot. */
  padding-right: 10px;
}

.skeleton-av {
  width: 28px;
  height: 28px;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.08);
  flex-shrink: 0;
  animation: pulse 1.5s ease-in-out infinite;
}

.skeleton-body {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.skeleton-line {
  height: 12px;
  border-radius: 4px;
  background: rgba(255, 255, 255, 0.08);
  animation: pulse 1.5s ease-in-out infinite;
}

/* Skeleton poster — mirrors the loaded .feed-poster slot */
.skeleton-poster {
  width: 40px;
  height: 60px;
  flex-shrink: 0;
  border-radius: var(--r-sm);
  background: rgba(255, 255, 255, 0.08);
  animation: pulse 1.5s ease-in-out infinite;
}

/* Skeleton-only width helpers — prefixed skel- to avoid shadowing Tailwind utilities */
.skel-w-1q   { width: 25%; }
.skel-w-3q   { width: 75%; }
.skel-w-1-3  { width: 33%; }

@keyframes pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.5; }
}
</style>
