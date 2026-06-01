<template>
  <div v-if="anime" class="min-h-screen">
    <!-- Hero Banner with Blurred Background -->
    <div class="relative h-[50vh] md:h-[60vh] overflow-hidden">
      <!-- Background Image -->
      <div
        class="absolute inset-0 bg-cover bg-center scale-110 blur-sm"
        :style="{ backgroundImage: `url(${getImageUrl((anime?.bannerImage || anime?.coverImage) ?? '')})` }"
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
              @error="(e: Event) => { const img = e.target as HTMLImageElement; if (!img.dataset.fallback) { img.dataset.fallback = '1'; img.src = getImageFallbackUrl(anime?.coverImage ?? '') } }"
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
            <span class="text-white/60">{{ formatEpisodeCount(anime) }}</span>
            <template v-if="anime.shikimoriId">
              <span class="text-white/30">•</span>
              <a
                :href="`https://shikimori.one/animes/${anime.shikimoriId}`"
                target="_blank"
                rel="noopener noreferrer"
                class="text-cyan-400 hover:text-cyan-300 transition-colors"
              >
                Shikimori #{{ anime.shikimoriId }}
              </a>
            </template>
          </div>

          <!-- Next Episode Info -->
          <div v-if="anime.nextEpisodeAt && anime.status === 'ongoing'" class="mb-4">
            <div class="inline-flex items-center gap-2 px-3 py-2 rounded-lg bg-cyan-500/10 backdrop-blur-xl border border-cyan-500/20">
              <svg class="w-5 h-5 text-cyan-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z" />
              </svg>
              <span class="text-cyan-400 font-medium">
                {{ $t('anime.nextEpisode', { episode: (anime.episodesAired || 0) + 1 }) }}
              </span>
              <span class="text-white">
                {{ formatNextEpisode(anime.nextEpisodeAt) }}
              </span>
            </div>
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
              <span class="text-white/60 text-sm">Shikimori</span>
            </div>

            <!-- Site Rating -->
            <div v-if="siteRating && siteRating.total_reviews > 0" class="flex items-center gap-2">
              <div class="flex items-center gap-1 text-cyan-400">
                <svg class="w-5 h-5" fill="currentColor" viewBox="0 0 20 20">
                  <path d="M9.049 2.927c.3-.921 1.603-.921 1.902 0l1.07 3.292a1 1 0 00.95.69h3.462c.969 0 1.371 1.24.588 1.81l-2.8 2.034a1 1 0 00-.364 1.118l1.07 3.292c.3.921-.755 1.688-1.54 1.118l-2.8-2.034a1 1 0 00-1.175 0l-2.8 2.034c-.784.57-1.838-.197-1.539-1.118l1.07-3.292a1 1 0 00-.364-1.118L2.98 8.72c-.783-.57-.38-1.81.588-1.81h3.461a1 1 0 00.951-.69l1.07-3.292z" />
                </svg>
                <span class="font-bold text-lg">{{ siteRating.average_score.toFixed(1) }}</span>
              </div>
              <span class="text-white/60 text-sm">AnimeEnigma ({{ siteRating.total_reviews }})</span>
            </div>

            <!-- Phase 14 / UX-28 — soft social-proof: hide below 5 to avoid
                 empty signals on niche/new titles. Public endpoint, no auth. -->
            <Badge v-if="watchersCount >= 5" variant="default" class="flex items-center gap-1">
              <span aria-hidden="true">👥</span>
              <span>{{ $t('anime.watchersCount', { count: formatCount(watchersCount) }) }}</span>
            </Badge>
          </div>

          <!-- Primary Watch CTA (visible above the fold, for everyone) -->
          <div class="flex flex-wrap items-center gap-3 mb-4">
            <!-- Announced/upcoming title with no sources yet: a "Watch now"
                 button would dead-end on a misleading "no videos" message,
                 so show a non-actionable premiere indicator instead. -->
            <div
              v-if="notReleasedYet"
              class="flex items-center gap-2 px-6 py-3 rounded-lg font-bold bg-white/5 text-white/70 border border-white/10 cursor-default"
            >
              <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z" />
              </svg>
              <span>{{ premiereDate ? $t('anime.notReleased.cta', { date: premiereDate }) : $t('anime.notReleased.ctaNoDate') }}</span>
            </div>
            <template v-else>
              <button
                @click="activatePlayer"
                type="button"
                class="flex items-center gap-2 px-6 py-3 rounded-lg font-bold bg-cyan-500 hover:bg-cyan-400 text-black shadow-lg shadow-cyan-500/20 transition-all"
              >
                <svg class="w-5 h-5" fill="currentColor" viewBox="0 0 24 24" aria-hidden="true">
                  <path d="M8 5v14l11-7z" />
                </svg>
                <span>{{ lastEpisode ? $t('anime.continueEp', { n: lastEpisode }) : $t('anime.watchNow') }}</span>
              </button>
              <!-- Workstream watch-together — discovery-stage Invite mount.
                   Anonymous users don't see it (creating a room requires JWT).
                   The button fetches a translation_id from the catalog on
                   click (~100ms) when none is resolved yet, so clicking
                   never waits on the player chain. -->
              <InviteButton
                v-if="authStore.isAuthenticated && anime"
                :anime-id="anime.id"
                :episode-id="String(resumeStartEpisode ?? lastEpisode ?? 1)"
                :player="(resolvedCombo?.player === 'english' ? 'ourenglish' : (resolvedCombo?.player ?? 'kodik')) as PlayerKind"
                :translation-id="resolvedCombo?.translation_id ?? ''"
              />
            </template>
          </div>

          <!-- Actions -->
          <div v-if="authStore.isAuthenticated" class="flex flex-wrap items-center gap-3 mb-6">
            <!-- Refresh Data Button -->
            <button
              @click="refreshAnimeData"
              :disabled="refreshing"
              class="flex items-center gap-2 px-4 py-2.5 rounded-lg font-medium transition-all bg-white/5 text-white border border-white/10 hover:bg-white/10 disabled:opacity-50"
              :title="$t('anime.refreshTooltip')"
            >
              <svg
                class="w-5 h-5 transition-transform"
                :class="{ 'animate-spin': refreshing }"
                fill="none"
                stroke="currentColor"
                viewBox="0 0 24 24"
              >
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
              </svg>
              <span class="hidden sm:inline">{{ refreshing ? $t('anime.refreshing') : $t('anime.refresh') }}</span>
            </button>

            <!-- Watchlist Status Dropdown -->
            <div class="relative" ref="dropdownRef">
              <button
                @click="showStatusDropdown = !showStatusDropdown"
                type="button"
                aria-haspopup="menu"
                :aria-expanded="showStatusDropdown"
                aria-controls="watchlist-status-menu"
                class="flex items-center gap-2 px-4 py-2.5 rounded-lg font-medium transition-all"
                :class="currentListStatus
                  ? 'bg-cyan-500/20 text-cyan-400 border border-cyan-500/30 hover:bg-cyan-500/30'
                  : 'bg-white/5 text-white border border-white/10 hover:bg-white/10'"
              >
                <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
                  <path v-if="currentListStatus" stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
                  <path v-else stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4" />
                </svg>
                <span>{{ currentListStatus ? statusLabels[currentListStatus] : $t('anime.addToList') }}</span>
                <svg class="w-4 h-4 transition-transform" :class="{ 'rotate-180': showStatusDropdown }" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
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
                  id="watchlist-status-menu"
                  role="menu"
                  class="absolute top-full left-0 mt-2 w-48 rounded-xl bg-surface border border-white/10 shadow-xl overflow-hidden z-50"
                >
                  <button
                    v-for="(label, status) in statusLabels"
                    :key="status"
                    role="menuitemradio"
                    :aria-checked="currentListStatus === status"
                    @click="setListStatus(status)"
                    class="w-full px-4 py-3 text-left text-sm transition-colors flex items-center justify-between"
                    :class="currentListStatus === status
                      ? 'bg-cyan-500/20 text-cyan-400'
                      : 'text-white/80 hover:bg-white/5 hover:text-white'"
                  >
                    {{ label }}
                    <svg v-if="currentListStatus === status" class="w-4 h-4" fill="currentColor" viewBox="0 0 20 20" aria-hidden="true">
                      <path fill-rule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clip-rule="evenodd" />
                    </svg>
                  </button>

                  <!-- Remove from list -->
                  <div v-if="currentListStatus" class="border-t border-white/10">
                    <button
                      @click="removeFromList"
                      role="menuitem"
                      class="w-full px-4 py-3 text-left text-sm text-pink-400 hover:bg-pink-500/10 transition-colors flex items-center gap-2"
                    >
                      <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
                      </svg>
                      {{ $t('anime.removeFromList') }}
                    </button>
                  </div>
                </div>
              </Transition>
            </div>

            <!-- Hide Button (Admin only) -->
            <button
              v-if="authStore.isAdmin"
              @click="toggleHidden"
              class="flex items-center gap-2 px-4 py-2.5 rounded-lg font-medium transition-all"
              :class="isHidden
                ? 'bg-amber-500/20 text-amber-400 border border-amber-500/30 hover:bg-amber-500/30'
                : 'bg-white/5 text-white border border-white/10 hover:bg-white/10'"
            >
              <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path v-if="isHidden" stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
                <path v-if="isHidden" stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M2.458 12C3.732 7.943 7.523 5 12 5c4.478 0 8.268 2.943 9.542 7-1.274 4.057-5.064 7-9.542 7-4.477 0-8.268-2.943-9.542-7z" />
                <path v-else stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13.875 18.825A10.05 10.05 0 0112 19c-4.478 0-8.268-2.943-9.543-7a9.97 9.97 0 011.563-3.029m5.858.908a3 3 0 114.243 4.243M9.878 9.878l4.242 4.242M9.88 9.88l-3.29-3.29m7.532 7.532l3.29 3.29M3 3l3.59 3.59m0 0A9.953 9.953 0 0112 5c4.478 0 8.268 2.943 9.543 7a10.025 10.025 0 01-4.132 5.411m0 0L21 21" />
              </svg>
              <span>{{ isHidden ? $t('anime.unhide') : $t('anime.hide') }}</span>
            </button>

            <!-- Edit Shikimori ID (Admin only) -->
            <button
              v-if="authStore.isAdmin"
              @click="showShikimoriEdit = !showShikimoriEdit"
              class="flex items-center gap-2 px-4 py-2.5 rounded-lg font-medium transition-all bg-white/5 text-white border border-white/10 hover:bg-white/10"
            >
              <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" />
              </svg>
              <span>Shikimori ID</span>
            </button>
          </div>

          <!-- Shikimori ID Edit Panel (Admin only) -->
          <div v-if="authStore.isAdmin && showShikimoriEdit" class="mb-4 p-3 rounded-lg bg-white/5 border border-white/10">
            <div class="flex items-center gap-3">
              <label class="text-white/60 text-sm whitespace-nowrap">Shikimori ID:</label>
              <input
                v-model="editShikimoriId"
                type="text"
                class="flex-1 bg-white/5 border border-white/10 rounded-lg px-3 py-2 text-white text-sm focus:outline-none focus:border-cyan-500"
                :placeholder="$t('anime.examplePlaceholder')"
              />
              <button
                @click="saveShikimoriId"
                :disabled="savingShikimoriId"
                class="px-4 py-2 bg-cyan-500 hover:bg-cyan-400 text-black font-medium rounded-lg transition-colors disabled:opacity-50 text-sm"
              >
                {{ savingShikimoriId ? '...' : $t('anime.save') }}
              </button>
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
      <!-- Phase 11 / UX-22 — section-overview anchor for AnimeQuickNav. -->
      <section id="section-overview" class="mt-8 non-player-content">
        <h2 class="text-xl font-semibold text-white mb-3">{{ $t('anime.synopsis') }}</h2>
        <div class="glass-card p-4">
          <p
            class="text-white/70 leading-relaxed"
            :class="{ 'line-clamp-4': !synopsisExpanded }"
            v-html="parsedDescription"
          />
          <button
            v-if="anime.description && anime.description.length > 300"
            class="mt-2 text-cyan-400 hover:text-cyan-300 transition-colors text-sm"
            @click="synopsisExpanded = !synopsisExpanded"
          >
            {{ synopsisExpanded ? $t('anime.showLess') : $t('anime.showMore') }}
          </button>
        </div>
      </section>

      <!-- Video Player Section -->
      <!-- Phase 11 / UX-22 — section-episodes anchor. Phase 11 / UX-23 —
           data-anime-player-wrapper hook so theater-mode CSS can widen it. -->
      <section
        id="section-episodes"
        data-anime-player-wrapper="true"
        class="mt-8"
        ref="playerSectionRef"
      >
        <div class="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3 mb-4">
          <div class="flex items-center justify-between gap-3 sm:gap-4">
            <h2 class="text-xl font-semibold text-white">
              <span class="flex items-center gap-2">
                <svg class="w-6 h-6 text-cyan-400" fill="currentColor" viewBox="0 0 24 24" aria-hidden="true">
                  <path d="M8 5v14l11-7z" />
                </svg>
                {{ $t('anime.watch') || 'Смотреть онлайн' }}
              </span>
            </h2>
          </div>
          <!-- InviteButton is mounted ABOVE THE FOLD next to the Primary
               Watch CTA (see line ~128). The previous mount here (gated on
               playerActivated, adjacent to the language tabs) was removed
               to keep a single discovery point next to Continue Watching. -->
          <!-- Language tabs + Provider sub-tabs -->
          <!-- UA-062 (UX-12 Phase 5): ButtonGroup wraps the RU/EN/18+ toggle
               with role="group" + aria-label; each child button binds
               aria-pressed to its selected state. -->
          <!-- Hidden for not-yet-released titles: no sources exist to switch
               between, so the language/provider toggles are noise. -->
          <div v-if="!notReleasedYet" class="flex flex-wrap gap-2">
            <ButtonGroup
              :label="$t('anime.languageSwitchLabel')"
              container-class="flex gap-1 bg-white/5 rounded-lg p-1"
            >
              <button
                @click="switchLanguage('ru')"
                :aria-pressed="videoLanguage === 'ru'"
                class="px-3 py-1.5 rounded-md text-sm font-medium transition-all"
                :class="videoLanguage === 'ru'
                  ? 'bg-white/15 text-white'
                  : 'text-white/50 hover:text-white/70'"
              >
                RU
              </button>
              <!-- Phase 24-28 — OurEnglish (scraper-microservice failover) -->
              <button
                v-if="ourEnglishEnabled"
                @click="switchLanguage('en')"
                :aria-pressed="videoLanguage === 'en'"
                class="px-3 py-1.5 rounded-md text-sm font-medium transition-all"
                :class="videoLanguage === 'en'
                  ? 'bg-white/15 text-white'
                  : 'text-white/50 hover:text-white/70'"
              >
                EN
              </button>
              <button
                v-if="isHentai"
                @click="switchLanguage('18+')"
                :aria-pressed="videoLanguage === '18+'"
                class="px-3 py-1.5 rounded-md text-sm font-medium transition-all"
                :class="videoLanguage === '18+'
                  ? 'bg-white/15 text-white'
                  : 'text-white/50 hover:text-white/70'"
              >
                18+
              </button>
              <!-- Workstream raw-jp / Phase 04 — RAW JP language pill behind
                   VITE_RAW_PROVIDER_ENABLED flag. -->
              <button
                v-if="rawProviderEnabled"
                @click="switchLanguage('raw')"
                :aria-pressed="videoLanguage === 'raw'"
                class="px-3 py-1.5 rounded-md text-sm font-medium transition-all"
                :class="videoLanguage === 'raw'
                  ? 'bg-white/15 text-white'
                  : 'text-white/50 hover:text-white/70'"
              >
                {{ $t('player.raw.tab') }}
              </button>
            </ButtonGroup>

            <!-- Provider sub-tabs -->
            <!-- UA-063 (UX-12 Phase 5): ButtonGroup wraps provider chips. -->
            <ButtonGroup
              v-if="videoLanguage === 'ru'"
              :label="$t('anime.providerSwitchLabel')"
              container-class="contents"
            >
              <button
                @click="onUserPickedProvider('kodik')"
                :aria-pressed="videoProvider === 'kodik'"
                class="px-4 py-2 rounded-lg text-sm font-medium transition-all"
                :class="videoProvider === 'kodik'
                  ? 'bg-cyan-500/20 text-cyan-400 border border-cyan-500/50'
                  : 'bg-white/5 text-white/60 border border-transparent hover:bg-white/10'"
              >
                Kodik
              </button>
              <button
                v-if="animeLibEnabled"
                @click="onUserPickedProvider('animelib')"
                :aria-pressed="videoProvider === 'animelib'"
                class="px-4 py-2 rounded-lg text-sm font-medium transition-all"
                :class="videoProvider === 'animelib'
                  ? 'bg-orange-500/20 text-orange-400 border border-orange-500/50'
                  : 'bg-white/5 text-white/60 border border-transparent hover:bg-white/10'"
              >
                AniLib
              </button>
            </ButtonGroup>
            <template v-else-if="videoLanguage === 'en' && ourEnglishEnabled">
              <button
                @click="videoProvider = 'ourenglish'"
                :aria-pressed="videoProvider === 'ourenglish'"
                class="px-4 py-2 rounded-lg text-sm font-medium transition-all"
                :class="videoProvider === 'ourenglish'
                  ? 'bg-emerald-500/20 text-emerald-400 border border-emerald-500/50'
                  : 'bg-white/5 text-white/60 border border-transparent hover:bg-white/10'"
              >
                {{ $t('player.ourenglish.label') }}
              </button>
            </template>
            <template v-else-if="videoLanguage === '18+'">
              <button
                @click="videoProvider = 'hanime'"
                class="px-4 py-2 rounded-lg text-sm font-medium transition-all"
                :class="videoProvider === 'hanime'
                  ? 'bg-pink-500/20 text-pink-400 border border-pink-500/50'
                  : 'bg-white/5 text-white/60 border border-transparent hover:bg-white/10'"
              >
                Hanime
              </button>
            </template>
            <!-- Workstream raw-jp / Phase 04 — single-chip group for v0.1.
                 v0.2's hybrid resolver adds 'minio' here. -->
            <template v-else-if="videoLanguage === 'raw' && rawProviderEnabled">
              <button
                @click="onUserPickedProvider('raw')"
                :aria-pressed="videoProvider === 'raw'"
                class="px-4 py-2 rounded-lg text-sm font-medium transition-all"
                :class="videoProvider === 'raw'
                  ? 'bg-rose-500/20 text-rose-300 border border-rose-500/50'
                  : 'bg-white/5 text-white/60 border border-transparent hover:bg-white/10'"
              >
                AllAnime
              </button>
            </template>
          </div>
        </div>
        <div class="glass-card p-4 md:p-6">
          <!-- Not-released notice: an announced/upcoming title with no sources
               yet. Replaces the player so users see "premieres on {date}"
               rather than a misleading "no available videos" error. -->
          <div
            v-if="notReleasedYet"
            class="w-full aspect-video rounded-lg bg-white/5 border border-white/10 flex flex-col items-center justify-center text-center gap-3 px-6"
          >
            <svg class="w-12 h-12 text-cyan-400/80" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M8 7V3m8 4V3m-9 8h10M5 21h14a2 2 0 002-2V7a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z" />
            </svg>
            <p class="text-lg font-semibold text-white">{{ $t('anime.notReleased.title') }}</p>
            <p class="text-sm text-white/60 max-w-md">
              {{ premiereDate ? $t('anime.notReleased.withDate', { date: premiereDate }) : $t('anime.notReleased.noDate') }}
            </p>
          </div>
          <!-- Click-to-load placeholder (saves bandwidth / no auto-buffer) -->
          <button
            v-else-if="!playerActivated"
            type="button"
            @click="activatePlayer"
            class="relative w-full aspect-video rounded-lg overflow-hidden group focus:outline-none focus:ring-2 focus:ring-cyan-400"
            :aria-label="lastEpisode ? $t('anime.continueEp', { n: lastEpisode }) : $t('anime.watchNow')"
          >
            <img
              :src="anime.coverImage"
              :alt="anime.title"
              class="absolute inset-0 w-full h-full object-cover blur-sm scale-110"
              @error="(e: Event) => { const img = e.target as HTMLImageElement; if (!img.dataset.fallback) { img.dataset.fallback = '1'; img.src = getImageFallbackUrl(anime?.coverImage ?? '') } }"
            />
            <div class="absolute inset-0 bg-gradient-to-t from-black/80 via-black/40 to-black/40" aria-hidden="true" />
            <div class="absolute inset-0 flex flex-col items-center justify-center gap-3 text-white">
              <span class="w-16 h-16 sm:w-20 sm:h-20 rounded-full bg-cyan-500/90 group-hover:bg-cyan-400 text-black flex items-center justify-center shadow-lg transition-colors">
                <svg class="w-8 h-8 sm:w-10 sm:h-10 ml-1" fill="currentColor" viewBox="0 0 24 24" aria-hidden="true">
                  <path d="M8 5v14l11-7z" />
                </svg>
              </span>
              <span class="text-base sm:text-lg font-semibold">
                {{ lastEpisode ? $t('anime.continueEp', { n: lastEpisode }) : $t('anime.watchNow') }}
              </span>
            </div>
          </button>
          <template v-else>
            <!-- Kodik Player -->
            <KodikPlayer
              v-if="videoProvider === 'kodik'"
              :anime-id="anime.id"
              :anime-name="anime.title"
              :total-episodes="anime.totalEpisodes"
              :preferred-combo="resolvedCombo"
              :initial-episode="resumeStartEpisode"
              @available-translations="handleAvailableTranslations"
            >
              <template #header-middle>
                <ResumePill v-bind="resumePillProps" @rewatch="resumeRewatch" @mark-complete-in-list="setListStatus('completed')" />
              </template>
            </KodikPlayer>
            <!-- AnimeLib Player -->
            <AnimeLibPlayer
              v-else-if="videoProvider === 'animelib' && animeLibEnabled"
              :anime-id="anime.id"
              :anime-name="anime.title"
              :total-episodes="anime.totalEpisodes"
              :preferred-combo="resolvedCombo"
              :initial-episode="resumeStartEpisode"
              @available-translations="handleAvailableTranslations"
            >
              <template #header-middle>
                <ResumePill v-bind="resumePillProps" @rewatch="resumeRewatch" @mark-complete-in-list="setListStatus('completed')" />
              </template>
            </AnimeLibPlayer>
            <!-- OurEnglish Player (Phase 24-28 scraper microservice) -->
            <OurEnglishPlayer
              v-else-if="videoProvider === 'ourenglish' && ourEnglishEnabled"
              :anime-id="anime.id"
              :initial-episode="resumeStartEpisode"
            >
              <template #header-middle>
                <ResumePill v-bind="resumePillProps" @rewatch="resumeRewatch" @mark-complete-in-list="setListStatus('completed')" />
              </template>
            </OurEnglishPlayer>
            <!-- Hanime Player -->
            <HanimePlayer
              v-else-if="videoProvider === 'hanime'"
              :anime-id="anime.id"
              :anime-name="anime.title"
              :total-episodes="anime.totalEpisodes"
              :initial-episode="resumeStartEpisode"
            >
              <template #header-middle>
                <ResumePill v-bind="resumePillProps" @rewatch="resumeRewatch" @mark-complete-in-list="setListStatus('completed')" />
              </template>
            </HanimePlayer>
            <!-- Workstream raw-jp / Phase 04 — RawPlayer mounts behind the
                 same VITE_RAW_PROVIDER_ENABLED flag that gates the chip. -->
            <RawPlayer
              v-else-if="videoProvider === 'raw' && rawProviderEnabled"
              :anime-id="anime.id"
            >
              <template #header-middle>
                <ResumePill v-bind="resumePillProps" @rewatch="resumeRewatch" @mark-complete-in-list="setListStatus('completed')" />
              </template>
            </RawPlayer>
          </template>
        </div>
      </section>

      <!-- Reviews + Comments Section (SOCIAL-06: two-tab UGC strip) -->
      <!-- Phase 11 / UX-22 — section-comments anchor for AnimeQuickNav. -->
      <section id="section-comments" class="mt-8 non-player-content">
        <div class="flex items-center justify-between mb-4">
          <h2 class="text-xl font-semibold text-white">
            <span class="flex items-center gap-2">
              <svg class="w-6 h-6 text-cyan-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 8h10M7 12h4m1 8l-4-4H5a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v8a2 2 0 01-2 2h-3l-4 4z" />
              </svg>
              {{ ugcTab === 'comments' ? $t('anime.ugc.commentsTab') : $t('anime.reviews') }}
            </span>
          </h2>
          <span v-if="ugcTab === 'reviews' && reviews.length > 0" class="text-white/60 text-sm">{{ $t('anime.reviewsCount', { count: reviews.length }) }}</span>
        </div>

        <Tabs
          v-model="ugcTab"
          :tabs="[
            { value: 'reviews', label: $t('anime.ugc.reviewsTab'), count: reviews.length },
            { value: 'comments', label: $t('anime.ugc.commentsTab'), count: comments.length },
          ]"
          variant="underline"
        >
          <template #reviews>
        <!-- Write Review Form -->
        <div v-if="authStore.isAuthenticated" class="glass-card p-4 md:p-6 mb-6">
          <h3 class="text-lg font-medium text-white mb-4">
            {{ myReview ? $t('anime.editReview') : $t('anime.writeReview') }}
          </h3>

          <!-- Star Rating -->
          <div class="mb-4">
            <label class="block text-white/60 text-sm mb-2">{{ $t('anime.yourRating') }}</label>
            <div class="flex flex-wrap gap-1" role="radiogroup" :aria-label="$t('anime.yourRating')">
              <button
                v-for="star in 10"
                :key="star"
                type="button"
                role="radio"
                :aria-checked="reviewForm.score === star"
                :aria-label="$t('anime.rateStar', { n: star })"
                @click="reviewForm.score = star"
                class="p-0.5 sm:p-1 transition-transform hover:scale-110"
              >
                <svg
                  class="w-6 h-6 sm:w-8 sm:h-8 transition-colors"
                  :class="star <= reviewForm.score ? 'text-amber-400' : 'text-white/30'"
                  fill="currentColor"
                  viewBox="0 0 20 20"
                  aria-hidden="true"
                >
                  <path d="M9.049 2.927c.3-.921 1.603-.921 1.902 0l1.07 3.292a1 1 0 00.95.69h3.462c.969 0 1.371 1.24.588 1.81l-2.8 2.034a1 1 0 00-.364 1.118l1.07 3.292c.3.921-.755 1.688-1.54 1.118l-2.8-2.034a1 1 0 00-1.175 0l-2.8 2.034c-.784.57-1.838-.197-1.539-1.118l1.07-3.292a1 1 0 00-.364-1.118L2.98 8.72c-.783-.57-.38-1.81.588-1.81h3.461a1 1 0 00.951-.69l1.07-3.292z" />
                </svg>
              </button>
            </div>
            <p v-if="reviewForm.score > 0" class="text-cyan-400 text-sm mt-1">{{ reviewForm.score }}/10</p>
          </div>

          <!-- Review Text -->
          <div class="mb-4">
            <label class="block text-white/60 text-sm mb-2">{{ $t('anime.reviewOptional') }}</label>
            <textarea
              v-model="reviewForm.text"
              rows="4"
              class="w-full bg-white/5 border border-white/10 rounded-lg px-4 py-3 text-white placeholder-white/30 focus:outline-none focus:border-cyan-500 transition-colors resize-none"
              :placeholder="$t('anime.reviewPlaceholder')"
            ></textarea>
          </div>

          <!-- Submit Buttons -->
          <div class="flex gap-3">
            <button
              @click="submitReview"
              :disabled="reviewForm.score === 0 || reviewSubmitting"
              class="px-6 py-2.5 bg-cyan-500 hover:bg-cyan-400 text-black font-medium rounded-lg transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {{ reviewSubmitting ? $t('anime.publishing') : (myReview ? $t('anime.update') : $t('anime.publish')) }}
            </button>
            <button
              v-if="myReview"
              @click="deleteMyReview"
              class="px-6 py-2.5 bg-pink-500/20 hover:bg-pink-500/30 text-pink-400 font-medium rounded-lg transition-colors"
            >
              {{ $t('common.delete') }}
            </button>
          </div>
        </div>

        <!-- Login prompt -->
        <div v-else class="glass-card p-6 mb-6 text-center">
          <p class="text-white/60 mb-3">{{ $t('anime.loginToReview') }}</p>
          <router-link
            to="/auth"
            class="inline-block px-6 py-2.5 bg-cyan-500 hover:bg-cyan-400 text-black font-medium rounded-lg transition-colors"
          >
            {{ $t('nav.login') }}
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
                  <router-link
                    :to="`/user/${review.user_id}`"
                    class="font-medium text-white hover:text-purple-400 transition-colors"
                  >
                    {{ review.username || $t('anime.user') }}
                  </router-link>
                  <p class="text-white/60 text-sm">
                    {{ formatDate(review.created_at) }}
                    <template v-if="review.status">
                      <span class="text-white/30 mx-1">·</span>
                      <span :class="isReviewFlagged(review) ? 'text-amber-400' : 'text-white/60'">{{ formatReviewStats(review) }}</span>
                    </template>
                  </p>
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
          <p class="text-white/50">{{ $t('anime.noReviews') }}</p>
        </div>
          </template>

          <template #comments>
            <!-- Write Comment Form (logged in) -->
            <div v-if="authStore.isAuthenticated" class="glass-card p-4 md:p-6 mb-6">
              <textarea
                v-model="newCommentBody"
                rows="3"
                :placeholder="$t('anime.ugc.commentPlaceholder')"
                :disabled="posting"
                class="w-full bg-white/5 border border-white/10 rounded-lg px-3 py-2 text-white placeholder-white/30 focus:outline-none focus:border-cyan-500 transition-colors resize-none"
                :class="posting ? 'opacity-50 pointer-events-none' : ''"
              ></textarea>
              <div
                class="text-sm text-right mt-1"
                :class="runeLen(newCommentBody) > 2000 ? 'text-pink-400' : 'text-white/40'"
                :aria-live="runeLen(newCommentBody) > 2000 ? 'assertive' : 'polite'"
              >
                {{ $t('anime.ugc.charCount', { count: runeLen(newCommentBody) }) }}
              </div>
              <div class="flex items-center gap-3 mt-2">
                <button
                  type="button"
                  @click="postComment"
                  :disabled="posting || newCommentBody.trim().length === 0 || runeLen(newCommentBody.trim()) > 2000"
                  class="bg-cyan-500 hover:bg-cyan-400 text-black font-semibold rounded-lg px-6 py-2 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
                >
                  {{ posting ? $t('anime.ugc.posting') : $t('anime.ugc.postComment') }}
                </button>
              </div>
              <p v-if="postError" class="text-pink-400 text-sm mt-2">{{ postError }}</p>
            </div>

            <!-- Login prompt (anonymous) -->
            <div v-else class="glass-card p-6 mb-6 text-center">
              <p class="text-white/60 mb-3">{{ $t('anime.ugc.loginToComment') }}</p>
              <router-link
                to="/auth"
                class="inline-block px-6 py-2 bg-cyan-500 hover:bg-cyan-400 text-black font-semibold rounded-lg transition-colors"
              >
                {{ $t('nav.login') }}
              </router-link>
            </div>

            <!-- Delete error toast (auto-clear via setTimeout in deleteCommentItem) -->
            <p v-if="deleteError" class="text-pink-400 text-sm mb-4">{{ deleteError }}</p>

            <!-- Load error -->
            <div v-if="commentsError && comments.length === 0" class="glass-card p-8 text-center">
              <p class="text-pink-400 text-sm mb-3">{{ commentsError }}</p>
              <button
                type="button"
                @click="fetchComments"
                class="px-4 py-2 bg-cyan-500 hover:bg-cyan-400 text-black font-semibold rounded-lg transition-colors"
              >
                {{ $t('common.retry') }}
              </button>
            </div>

            <!-- Empty state -->
            <div
              v-else-if="commentsFetched && comments.length === 0 && !commentsLoading"
              class="glass-card p-8 text-center"
            >
              <p class="text-white/50">{{ $t('anime.ugc.emptyComments') }}</p>
            </div>

            <!-- Comment list -->
            <div v-else-if="comments.length > 0" class="space-y-4">
              <article
                v-for="c in comments"
                :key="c.id"
                class="glass-card p-4 space-y-2"
              >
                <div class="flex items-start justify-between">
                  <div class="flex items-center gap-3">
                    <div class="w-10 h-10 rounded-full bg-cyan-500/20 flex items-center justify-center text-cyan-400 font-semibold">
                      {{ c.username?.slice(0, 2).toUpperCase() || '??' }}
                    </div>
                    <div>
                      <router-link
                        :to="`/user/${c.user_id}`"
                        class="font-semibold text-white hover:text-purple-400 transition-colors"
                      >
                        {{ c.username || $t('anime.user') }}
                      </router-link>
                      <p class="text-white/40 text-sm">{{ formatDate(c.created_at) }}</p>
                    </div>
                  </div>
                  <div class="flex items-center gap-2">
                    <!-- Edit: own comment only (admins do NOT edit others) -->
                    <button
                      v-if="c.user_id === authStore.user?.id && editingCommentId !== c.id"
                      type="button"
                      :aria-label="$t('anime.ugc.editComment')"
                      :title="$t('anime.ugc.editComment')"
                      @click="startEditComment(c)"
                      class="text-white/40 hover:text-cyan-400 p-2 rounded-lg transition-colors"
                    >
                      <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15.232 5.232l3.536 3.536m-2.036-5.036a2.5 2.5 0 113.536 3.536L6.5 21.036H3v-3.572L16.732 3.732z" />
                      </svg>
                    </button>
                    <!-- Delete: own comment OR admin -->
                    <button
                      v-if="c.user_id === authStore.user?.id || authStore.isAdmin"
                      type="button"
                      :aria-label="$t('anime.ugc.deleteComment')"
                      :title="$t('anime.ugc.deleteComment')"
                      @click="deleteCommentItem(c)"
                      class="text-white/40 hover:text-pink-400 hover:bg-pink-500/10 p-2 rounded-lg transition-colors"
                    >
                      <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6M1 7h22M9 7V4a2 2 0 012-2h2a2 2 0 012 2v3" />
                      </svg>
                    </button>
                  </div>
                </div>

                <!-- Body / edit mode -->
                <template v-if="editingCommentId === c.id">
                  <textarea
                    v-model="editingBody"
                    rows="3"
                    :placeholder="$t('anime.ugc.editPlaceholder')"
                    :disabled="editSaving"
                    class="w-full bg-white/5 border border-white/10 rounded-lg px-3 py-2 text-white placeholder-white/30 focus:outline-none focus:border-cyan-500 transition-colors resize-none"
                  ></textarea>
                  <div class="flex items-center gap-2 mt-2">
                    <button
                      type="button"
                      @click="saveEditComment"
                      :disabled="editSaving || editingBody.trim().length === 0 || runeLen(editingBody.trim()) > 2000"
                      class="bg-cyan-500 hover:bg-cyan-400 text-black font-semibold rounded-lg px-4 py-2 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
                    >
                      {{ editSaving ? $t('anime.ugc.posting') : $t('anime.ugc.saveEdit') }}
                    </button>
                    <button
                      type="button"
                      @click="cancelEditComment"
                      :disabled="editSaving"
                      class="bg-pink-500/20 hover:bg-pink-500/30 text-pink-400 font-semibold rounded-lg px-4 py-2 transition-colors disabled:opacity-50"
                    >
                      {{ $t('anime.ugc.cancelEdit') }}
                    </button>
                  </div>
                  <p v-if="editError" class="text-pink-400 text-sm mt-2">{{ editError }}</p>
                </template>
                <p v-else class="whitespace-pre-wrap text-white/70">{{ c.body }}</p>
              </article>
            </div>

            <!-- Skeleton (first load) -->
            <div v-else-if="commentsLoading" class="space-y-4">
              <div v-for="n in 3" :key="n" class="glass-card p-4 space-y-2 animate-pulse">
                <div class="flex items-center gap-3">
                  <div class="w-10 h-10 rounded-full bg-white/5"></div>
                  <div class="space-y-2 flex-1">
                    <div class="h-3 w-32 bg-white/10 rounded"></div>
                    <div class="h-2 w-20 bg-white/5 rounded"></div>
                  </div>
                </div>
                <div class="h-3 w-3/4 bg-white/5 rounded"></div>
              </div>
            </div>

            <!-- Load more -->
            <div v-if="commentsHasMore" class="mt-4 flex justify-center">
              <button
                type="button"
                @click="loadMoreComments"
                :disabled="commentsLoadingMore"
                class="text-white/70 hover:text-white px-4 py-2 rounded-lg glass-card transition-colors disabled:opacity-50"
              >
                {{ commentsLoadingMore ? $t('anime.ugc.loading') : $t('anime.ugc.loadMore') }}
              </button>
            </div>
          </template>
        </Tabs>
      </section>

      <!-- Related Anime -->
      <!-- Phase 11 / UX-22 — section-similar anchor for AnimeQuickNav. -->
      <section
        v-if="relatedAnime.length > 0"
        id="section-similar"
        class="mt-8 non-player-content"
      >
        <Carousel
          :items="relatedAnime"
          :title="$t('anime.related')"
          item-key="id"
        >
          <template #default="{ item }">
            <div>
              <AnimeCardNew
                :anime="(item as RelatedAnime)"
                :menu-open="contextMenu.visible && String(contextMenu.anime?.id) === String((item as RelatedAnime).id)"
                @open-menu="(el: HTMLElement) => openContextMenuAt(el, (item as RelatedAnime))"
              />
              <p v-if="(item as RelatedAnime).relationLabel" class="text-xs text-white/60 mt-1 text-center">
                {{ (item as RelatedAnime).relationLabel }}
              </p>
            </div>
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

  <!-- Context Menu for related anime -->
  <AnimeContextMenu
    :visible="contextMenu.visible"
    :x="contextMenu.x"
    :y="contextMenu.y"
    :anime="contextMenu.anime"
    :list-status="contextMenu.listStatus"
    :site-rating="contextMenu.siteRating"
    @update:visible="contextMenu.visible = $event"
  />
</template>

<script setup lang="ts">
import { ref, reactive, computed, watch, onMounted, onBeforeUnmount, onUnmounted, defineAsyncComponent, nextTick } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useAnime } from '@/composables/useAnime'
import { useAuthStore } from '@/stores/auth'
import { Badge, Button, ButtonGroup } from '@/components/ui'
import { GenreChip, AnimeCardNew, AnimeContextMenu } from '@/components/anime'
import { Carousel } from '@/components/carousel'
import { useWatchPreferences } from '@/composables/useWatchPreferences'
import { useOverrideTracker } from '@/composables/useOverrideTracker'
import { useResumeStateMachine } from '@/composables/useResumeStateMachine'
import { useContextMenu } from '@/composables/useContextMenu'
import type { WatchCombo } from '@/types/preference'

const KodikPlayer = defineAsyncComponent(() => import('@/components/player/KodikPlayer.vue'))
const AnimeLibPlayer = defineAsyncComponent(() => import('@/components/player/AnimeLibPlayer.vue'))
const HanimePlayer = defineAsyncComponent(() => import('@/components/player/HanimePlayer.vue'))
// Workstream raw-jp, Phase 04 — lazy-load RawPlayer behind a Vite flag.
const RawPlayer = defineAsyncComponent(() => import('@/components/player/RawPlayer.vue'))
const rawProviderEnabled = import.meta.env.VITE_RAW_PROVIDER_ENABLED === 'true'
// Phase 24-28 — OurEnglish (scraper-microservice-backed EN player).
// Behind VITE_OURENGLISH_ENABLED so it can ship dark until upstream
// gates are green in production.
const OurEnglishPlayer = defineAsyncComponent(() => import('@/components/player/OurEnglishPlayer.vue'))
const ourEnglishEnabled = import.meta.env.VITE_OURENGLISH_ENABLED !== 'false'
// AniLib direct-MP4 hidden 2026-06-01: AnimeLib's upstream API
// (hapi.hentaicdn.org) now returns Kodik-only players for every title — zero
// "Animelib" direct-MP4 sources — so the AniLib path resolves to empty
// translations for ALL anime (verified across 11 titles incl. Frieren/One
// Piece, even with a valid ANIMELIB_TOKEN). Since the no-Kodik-fallback rule
// drops Kodik-only translations, the chip would always dead-end on "no
// sources". Default OFF; flip VITE_ANIMELIB_ENABLED=true to restore the chip
// if upstream brings direct video back.
const animeLibEnabled = import.meta.env.VITE_ANIMELIB_ENABLED === 'true'
// Workstream watch-together / Phase 02 Plan 02.9 (WT-SHELL-05) — lazy-loaded
// invite button. Keeps Anime.vue's eager bundle clean — InviteButton pulls in
// the watch-together api client + types + toast composable transitively, paid
// only on first render (i.e. when a logged-in user has activated the player).
const InviteButton = defineAsyncComponent(() => import('@/components/watch-together/InviteButton.vue'))
import type { PlayerKind } from '@/types/watch-together'
import ResumePill from '@/components/player/ResumePill.vue'
import { animeApi, userApi, reviewApi, adminApi, commentApi } from '@/api/client'
import Tabs from '@/components/ui/Tabs.vue'
import { useWatchlistStore } from '@/stores/watchlist'
import { useToast } from '@/composables/useToast'
import { parseDescription } from '@/utils/description-parser'
import { getImageUrl, getImageFallbackUrl } from '@/composables/useImageProxy'

interface AnimeWithExtras {
  japaneseTitle?: string
  type?: string
  hidden?: boolean
  shikimoriId?: string
}

interface RelatedAnime {
  id: string
  title: string
  name?: string
  nameRu?: string
  coverImage: string
  rating?: number
  releaseYear?: number
  episodes?: number
  genres?: string[]
  relationLabel?: string
}

interface Review {
  id: string
  user_id: string
  anime_id: string
  username: string
  score: number
  review_text: string
  created_at: string
  // Steam-style review context (2026-05-21). Live values from anime_list
  // row — NOT snapshotted at review time. `anime` carries episodes_count
  // for the "watched / total" rendering; backend preloads it.
  status?: string
  episodes?: number
  anime?: {
    episodes_count?: number
  }
}

interface Comment {
  id: string
  user_id: string
  anime_id: string
  username: string
  body: string
  created_at: string
  updated_at: string
}

const UGC_ALLOWED = ['reviews', 'comments'] as const
type UgcTab = typeof UGC_ALLOWED[number]

interface AnimeRating {
  anime_id: string
  average_score: number
  total_reviews: number
}

const route = useRoute()
const router = useRouter()
const { t, locale } = useI18n()
const authStore = useAuthStore()
const watchlistStore = useWatchlistStore()
const toast = useToast()
const { anime, loading, error, fetchAnime } = useAnime()
const { contextMenu, openAtElement: openContextMenuAt } = useContextMenu()

let loadGeneration = 0
const synopsisExpanded = ref(false)
const currentListStatus = ref<string | null>(null)
const showStatusDropdown = ref(false)
const dropdownRef = ref<HTMLElement | null>(null)
const playerSectionRef = ref<HTMLElement | null>(null)
const playerActivated = ref(false)

async function activatePlayer() {
  playerActivated.value = true
  await nextTick()
  playerSectionRef.value?.scrollIntoView({ behavior: 'smooth', block: 'start' })
}


// Phase 11 / UX-23 — Theater Mode.
// State persists via localStorage so a reload keeps the user's choice.
// `body.theater-mode` drives the CSS rules at the bottom of this file that
// hide .navbar-root + .non-player-content and widen the player wrapper.
// MANDATORY cleanup: onBeforeUnmount removes the body class so leaving
// /anime/:id never strands the rest of the app with a hidden navbar.
const theaterMode = ref<boolean>(
  typeof localStorage !== 'undefined' && localStorage.getItem('theaterMode') === '1',
)

function setTheater(on: boolean) {
  theaterMode.value = on
  if (typeof localStorage !== 'undefined') {
    localStorage.setItem('theaterMode', on ? '1' : '0')
  }
}

function applyBodyTheaterClass(on: boolean) {
  if (typeof document === 'undefined') return
  document.body.classList.toggle('theater-mode', on)
}

function onTheaterEscape(e: KeyboardEvent) {
  if (e.key === 'Escape' && theaterMode.value) {
    setTheater(false)
  }
}
const relatedAnime = ref<RelatedAnime[]>([])
const refreshing = ref(false)
const isHidden = ref(false)
const showShikimoriEdit = ref(false)
const editShikimoriId = ref('')
const savingShikimoriId = ref(false)
// Runtime-validate localStorage values — users who previously selected an
// EN player ('english' / 'hianime' / 'consumet') would otherwise hit a value
// outside the new union, no v-if branch matches, and nothing renders.
const VALID_LANGUAGES = ['ru', 'en', '18+', 'raw'] as const
type VideoLanguage = (typeof VALID_LANGUAGES)[number]
const _savedLang = localStorage.getItem('preferred_video_language')
const videoLanguage = ref<VideoLanguage>(
  (VALID_LANGUAGES as readonly string[]).includes(_savedLang ?? '') ? (_savedLang as VideoLanguage) : 'ru'
)
// Workstream raw-jp, Phase 04 — 'raw' is the AllAnime-backed raw-JP provider.
// Phase 24-28 — 'ourenglish' is the scraper-microservice-backed EN provider
// (failover across gogoanime/animepahe/allanime/animefever/miruro/nineanime).
const VALID_PROVIDERS = ['kodik', 'animelib', 'ourenglish', 'hanime', 'raw'] as const
type VideoProvider = (typeof VALID_PROVIDERS)[number]
const _savedProv = localStorage.getItem('preferred_video_provider')
// Coerce a pinned-but-disabled 'animelib' back to 'kodik' so users who last
// watched on AniLib don't boot into a hidden tab with no player mounted.
const _initialProv =
  (VALID_PROVIDERS as readonly string[]).includes(_savedProv ?? '') ? (_savedProv as VideoProvider) : 'kodik'
const videoProvider = ref<VideoProvider>(
  _initialProv === 'animelib' && !animeLibEnabled ? 'kodik' : _initialProv
)

// Last-watched episode. For authenticated users this comes from server-side
// watch_progress (Phase 4 — A-03 / D-02 single source of truth). For
// anonymous users we still read localStorage; D-01 in Phase 7 will close the
// gap with a localStorage-driven anonymous state machine.
const lastEpisode = ref<number | undefined>(undefined)

// Phase 4 resume state machine. Inputs are reactive refs that the composable
// re-evaluates as anime data + watch_progress arrives. The composable owns
// the kind / startEpisode / banner-relevant state; Anime.vue only renders.
const resumeAnimeId = computed(() => anime.value?.id ?? '')
const resumeTotal = computed(() => anime.value?.totalEpisodes ?? 0)
const resumeAired = computed(() => anime.value?.episodesAired ?? 0)
const resumeStatus = computed(() => anime.value?.status ?? '')
const resumeNextAt = computed(() => anime.value?.nextEpisodeAt ?? undefined)
const resumeAuth = computed(() => authStore.isAuthenticated)
const resume = useResumeStateMachine({
  animeId: resumeAnimeId,
  totalEpisodes: resumeTotal,
  episodesAired: resumeAired,
  nextEpisodeAt: resumeNextAt,
  status: resumeStatus,
  isAuthenticated: resumeAuth,
})

function loadLastEpisode(animeId: string) {
  const raw = localStorage.getItem(`watch_progress:${animeId}`)
  if (!raw) return
  try {
    const data = JSON.parse(raw) as Record<string, { updatedAt?: number }>
    let latest = 0
    let latestEp: number | undefined
    for (const [ep, info] of Object.entries(data)) {
      if (info.updatedAt && info.updatedAt > latest) {
        latest = info.updatedAt
        // WR-10: explicit radix 10 to defend against the historic octal-on-
        // leading-zero foot-gun ("08" -> 0 in pre-ES5 engines) and to satisfy
        // ESLint's `radix` rule.
        latestEp = parseInt(ep, 10)
      }
    }
    if (latestEp && !isNaN(latestEp)) lastEpisode.value = latestEp
  } catch { /* ignore corrupted data */ }
}

// Once the resume state machine loads from server, it overrides the
// localStorage last-episode for authenticated users.
watch(() => resume.lastWatched.value, (n) => {
  if (resumeAuth.value && n > 0) lastEpisode.value = n
})

// User-driven override of the state machine's start episode. Set when the
// user clicks "Rewatch from ep. 1" on the finished banner; otherwise null.
const resumeOverrideEpisode = ref<number | null>(null)

// Phase 8 (CR-01) — explicit deep-link hint from the Continue-Watching row.
// `/anime/{id}?episode=N` is the contract used by ContinueWatchingRow.vue;
// without this read, the link silently falls back to the resume-state-machine
// startEpisode (which usually matches, but diverges whenever the server-side
// resume state has advanced past the row's episode_number — e.g. when the
// user just finished E5 but is being shown an older "in-progress" E4 row).
//
// The query is a HINT, not a hard override: the manual "Rewatch from ep. 1"
// button (resumeOverrideEpisode) still wins above this. The episode is
// clamped to [1, totalEpisodes] when totalEpisodes is known; otherwise the
// raw value is accepted (the player components defend their own bounds).
const queryEpisode = computed<number | undefined>(() => {
  const v = route.query.episode
  const s = Array.isArray(v) ? v[0] : v
  if (typeof s !== 'string' || s === '') return undefined
  const n = parseInt(s, 10)
  if (!Number.isFinite(n) || n <= 0) return undefined
  const total = anime.value?.totalEpisodes ?? 0
  if (total > 0 && n > total) return total
  return n
})

// What episode the player should mount on. Authenticated + state machine
// loaded → use the state machine's startEpisode. Otherwise fall back to the
// existing lastEpisode (localStorage path) so anonymous users keep the
// pre-Phase-4 behavior. Manual override (rewatch click) wins over both.
// Deep-link query param wins over the state-machine resume (explicit user
// selection), but still loses to the in-page "Rewatch" override.
const resumeStartEpisode = computed<number | undefined>(() => {
  if (resumeOverrideEpisode.value && resumeOverrideEpisode.value > 0) {
    return resumeOverrideEpisode.value
  }
  if (queryEpisode.value !== undefined) {
    return queryEpisode.value
  }
  if (resumeAuth.value && resume.loaded.value) {
    const s = resume.startEpisode.value
    return s > 0 ? s : (lastEpisode.value ?? 1)
  }
  return lastEpisode.value
})

// Phase 8 (CR-01) — auto-activate the player when arriving with ?episode=N.
// The deep link's intent is "land me on episode N ready to watch", so we
// expand the click-to-load placeholder automatically. Also re-mount when the
// query flips between values without a full route change (e.g. user clicks
// two different Continue-Watching cards for the same anime in succession).
watch(
  () => queryEpisode.value,
  (n) => {
    if (n !== undefined && !playerActivated.value) {
      // Defer to onMounted's anime-load path so the player has its data.
      // activatePlayer() is idempotent; calling it here is safe even before
      // the anime resolves because the template gates on `v-if="anime"`.
      activatePlayer()
    }
  },
  { immediate: true },
)

// Episode number the "not yet available" / "currently airing" banners refer
// to. Always lastWatched + 1 in those states (the state machine has already
// guaranteed last < total).
const resumeNextEpisodeNumber = computed<number | undefined>(() => {
  if (resume.kind.value === 'not-yet-aired' || resume.kind.value === 'currently-airing') {
    return Math.max(1, resume.lastWatched.value + 1)
  }
  return undefined
})

// Resume pill props bundled into one object so each player slot can
// `v-bind` them in one line. Visibility is auth-gated here — the pill
// component itself decides per-kind visibility.
const resumePillProps = computed(() => {
  if (!authStore.isAuthenticated || !resume.loaded.value || resume.kind.value === 'first-time') {
    return { kind: 'first-time' as const }
  }
  const etaLabel = (resume.kind.value === 'not-yet-aired' && resumeNextAt.value)
    ? formatNextEpisode(resumeNextAt.value)
    : undefined
  return {
    kind: resume.kind.value,
    finishedEpisode: resume.finishedEpisode.value,
    nextEpisodeNumber: resumeNextEpisodeNumber.value,
    nextEpisodeEtaLabel: etaLabel,
    canMarkCompleteInList: currentListStatus.value !== 'completed',
    findSimilarRoute: anime.value?.genres?.length
      ? { path: '/browse', query: { genres: anime.value.genres[0] } }
      : undefined,
  }
})

// Restart from episode 1 — used by the "Rewatch" action on the finished
// banner. Sets the override (which wins over the state machine's startEpisode
// for 'finished') and activates the player.
function resumeRewatch() {
  resumeOverrideEpisode.value = 1
  activatePlayer()
}

// Watch preference resolution
const preferenceState = ref<{
  resolvedCombo: WatchCombo | null
  resolve: (available: WatchCombo[]) => Promise<void>
} | null>(null)

const resolvedCombo = computed(() => preferenceState.value?.resolvedCombo ?? null)

// Numeric episode ref for the tracker. lastEpisode is Ref<number | undefined>
// (resume-progress is unknown until localStorage is read); the tracker only
// reads this for the optional payload, so default to 0 when unset.
const currentEpisodeForTracker = computed(() => lastEpisode.value ?? 0)

// Player-dimension override tracker. Instantiated ONCE at the Anime.vue level
// because the player choice happens here, not inside any single player
// component — a per-player composable instance can't observe a switch from
// inside the unmounting player. This is the ONLY dimension this composable
// handles outside the four player components (the per-player tracker handles
// episode/team/language). Only fires from user-driven button clicks routed
// through onUserPickedProvider — programmatic mutations to videoProvider.value
// (initial preference resolution in initPreferences, switchLanguage tab
// auto-pick, fallback when a parser fails) are intentionally direct
// assignments and do NOT route through the tracker, per CONTEXT D-08.
// See .planning/phases/01-instrumentation-baseline/01-RESEARCH.md §Pattern 2.
const playerSwitchTracker = useOverrideTracker({
  animeId: route.params.id as string,
  // The composable's `player` is a static label — it identifies which player
  // bucket this tracker belongs to in Loki. For Anime.vue the bucket is the
  // CURRENT player at construction time; subsequent switches are recorded as
  // dimension='player', new_combo.player=<destination>. The label is only used
  // to filter Grafana panels, not to gate emissions. The 18+ 'hanime' provider
  // is not part of the tracked PlayerName set; map it to 'kodik' for the
  // bucket label so the type compiles. (No override fires for hanime anyway —
  // onUserPickedProvider is typed to exclude it.)
  // Workstream raw-jp / Phase 04 — 'raw' is also outside the tracked
  // PlayerName set; map to 'kodik' for the bucket label like hanime.
  // Phase 24-28 — 'ourenglish' maps to 'english' (the existing analytics
  // bucket for EN providers, preserved across the HiAnime→Consumet→OurEnglish
  // generation rollover).
  player: videoProvider.value === 'hanime' || videoProvider.value === 'raw'
    ? 'kodik'
    : videoProvider.value === 'ourenglish'
      ? 'english'
      : videoProvider.value,
  resolvedCombo,
  currentEpisode: currentEpisodeForTracker,
})

function onUserPickedProvider(newProvider: 'kodik' | 'animelib' | 'raw') {
  // Only fire override if the user is genuinely SWITCHING. The composable's
  // first-per-(load_session_id, dimension) lock would also catch repeats, but
  // an explicit guard keeps E2E timing predictable.
  if (newProvider !== videoProvider.value) {
    // Workstream raw-jp / Phase 04 — 'raw' is outside the tracked
    // PlayerName set; map to 'kodik' for the picker event like hanime.
    const trackedProvider = newProvider === 'raw' ? 'kodik' : newProvider
    playerSwitchTracker.recordPickerEvent('player', { player: trackedProvider })
  }
  videoProvider.value = newProvider
}

const initPreferences = (animeId: string) => {
  const pref = useWatchPreferences(animeId)
  preferenceState.value = {
    resolvedCombo: pref.resolvedCombo.value,
    resolve: async (available: WatchCombo[]) => {
      await pref.resolve(available)
      if (preferenceState.value) {
        preferenceState.value.resolvedCombo = pref.resolvedCombo.value
      }
      // Auto-switch player/language based on resolved combo. Skip resolved
      // EN combos — the EN tab is offline pending new providers.
      if (pref.resolvedCombo.value) {
        applyResolvedCombo(pref.resolvedCombo.value)
      }
    }
  }
  // If we already have a cached result, apply it
  if (pref.resolvedCombo.value) {
    applyResolvedCombo(pref.resolvedCombo.value)
  }
}

function applyResolvedCombo(combo: WatchCombo) {
  if (combo.language === 'ru' || combo.language === '18+') {
    videoLanguage.value = combo.language
  }
  if (combo.player === 'kodik' || combo.player === 'animelib' || combo.player === 'hanime') {
    // AniLib hidden (see animeLibEnabled): never auto-resolve onto the disabled
    // provider — fall back to Kodik, which carries the same RU translations.
    videoProvider.value = combo.player === 'animelib' && !animeLibEnabled ? 'kodik' : combo.player
  }
}

const handleAvailableTranslations = (combos: WatchCombo[]) => {
  if (preferenceState.value) {
    preferenceState.value.resolve(combos)
  }
}

// Reviews
const reviews = ref<Review[]>([])
const myReview = ref<Review | null>(null)
const siteRating = ref<AnimeRating | null>(null)
// Phase 14 / UX-28 — soft social-proof watchers count. Public endpoint,
// no auth. Render badge only when count >= 5 (avoids embarrassingly empty
// signals on niche or fresh titles).
const watchersCount = ref(0)
const reviewSubmitting = ref(false)
const reviewForm = reactive({
  score: 0,
  text: '',
})

// Comments (Plan 1-6, SOCIAL-06)
// UGC tab state, URL-persisted via ?ugc=reviews|comments via two watchers.
// Default = 'reviews'. Unknown values fall back to 'reviews' synchronously
// (initial state set BEFORE first render, no flicker on deep links).
const _initialUgc = (route.query.ugc as string | undefined)
const ugcTab = ref<UgcTab>(
  _initialUgc && (UGC_ALLOWED as readonly string[]).includes(_initialUgc)
    ? (_initialUgc as UgcTab)
    : 'reviews'
)
const comments = ref<Comment[]>([])
const commentsHasMore = ref(false)
const commentsNextCursor = ref<string>('')
const commentsLoading = ref(false)
const commentsLoadingMore = ref(false)
const commentsError = ref('')
const commentsFetched = ref(false)

const newCommentBody = ref('')
const posting = ref(false)
const postError = ref('')

const editingCommentId = ref<string | null>(null)
const editingBody = ref('')
const editError = ref('')
const editSaving = ref(false)
const deleteError = ref('')

// Rune-count helper (UTF-8 code points; matches the backend's 1–2000 rune
// validation rather than UTF-16 length).
const runeLen = (s: string) => [...s].length

const statusLabels = computed((): Record<string, string> => ({
  watching: t('profile.watchlist.watching'),
  plan_to_watch: t('profile.watchlist.planToWatch'),
  completed: t('profile.watchlist.completed'),
  on_hold: t('profile.watchlist.onHold'),
  dropped: t('profile.watchlist.dropped'),
}))

const statusVariant = computed(() => {
  const status = anime.value?.status?.toLowerCase()
  if (status === 'completed' || status === 'released') return 'success'
  if (status === 'upcoming' || status === 'announced') return 'warning'
  return 'primary' // ongoing
})

const parsedDescription = computed(() =>
  anime.value?.description ? parseDescription(anime.value.description) : ''
)

const isHentai = computed(() =>
  anime.value?.rawGenres?.some(g => g.name === 'Hentai') ?? false
)

const formatDate = (dateStr: string) => {
  const date = new Date(dateStr)
  // REVIEW.md WR-05: respect the i18n locale instead of hardcoding ru-RU.
  // English / Japanese users previously saw review and comment dates
  // formatted as Russian (e.g. "13 мая 2026 г."). Map the active vue-i18n
  // locale to a BCP-47 tag accepted by Intl. The pattern mirrors the
  // existing formatNextEpisode helper a few lines below.
  const loc = locale.value === 'ru' ? 'ru-RU' : locale.value === 'ja' ? 'ja-JP' : 'en-US'
  return date.toLocaleDateString(loc, {
    day: 'numeric',
    month: 'long',
    year: 'numeric',
  })
}

// An announced/upcoming title with no resolved sources hasn't aired yet —
// so "no available videos" is misleading. Show a premiere notice instead.
// hasVideo guards the rare case of an announced title with an early leak.
const notReleasedYet = computed(() => {
  const s = anime.value?.status?.toLowerCase()
  return (s === 'announced' || s === 'upcoming') && !anime.value?.hasVideo
})
const premiereDate = computed(() =>
  anime.value?.airedOn ? formatDate(anime.value.airedOn) : ''
)

const formatReviewStats = (review: Review): string => {
  const status = review.status || 'watching'
  const episodes = review.episodes ?? 0
  const total = review.anime?.episodes_count ?? 0

  // Map raw status enum -> existing watchlist.* i18n keys.
  const statusKeyMap: Record<string, string> = {
    watching: 'profile.watchlist.watching',
    completed: 'profile.watchlist.completed',
    on_hold: 'profile.watchlist.onHold',
    dropped: 'profile.watchlist.dropped',
    plan_to_watch: 'profile.watchlist.planToWatch',
  }
  const statusLabel = t(statusKeyMap[status] || statusKeyMap.watching)

  // Pick template variant: closed (total known) vs open (total unknown);
  // flagged (plan_to_watch OR episodes==0) vs normal.
  const flagged = status === 'plan_to_watch' || episodes === 0
  const open = total === 0

  let key: string
  if (flagged && status === 'plan_to_watch' && open) {
    key = 'anime.reviewStats.planToWatchOpenFlag'
  } else if (flagged && status === 'plan_to_watch') {
    key = 'anime.reviewStats.planToWatchFlag'
  } else if (flagged && open) {
    key = 'anime.reviewStats.noProgressOpen'
  } else if (flagged) {
    key = 'anime.reviewStats.noProgress'
  } else if (open) {
    key = 'anime.reviewStats.watchedOpen'
  } else {
    key = 'anime.reviewStats.watched'
  }

  return t(key, { watched: episodes, total, status: statusLabel })
}

const isReviewFlagged = (review: Review): boolean => {
  const status = review.status || 'watching'
  const episodes = review.episodes ?? 0
  return status === 'plan_to_watch' || episodes === 0
}

const formatNextEpisode = (dateStr: string) => {
  const date = new Date(dateStr)
  const now = new Date()
  const diffMs = date.getTime() - now.getTime()
  const diffDays = Math.floor(diffMs / (1000 * 60 * 60 * 24))

  const timeStr = date.toLocaleTimeString('ru-RU', {
    hour: '2-digit',
    minute: '2-digit',
    timeZone: 'Europe/Moscow'
  })

  if (diffDays === 0) return t('anime.todayAt', { time: timeStr })
  if (diffDays === 1) return t('anime.tomorrowAt', { time: timeStr })
  if (diffDays > 1 && diffDays < 7) {
    const dayKeys = ['sunday', 'monday', 'tuesday', 'wednesday', 'thursday', 'friday', 'saturday']
    return t('anime.dayAtTime', { day: t(`schedule.days.${dayKeys[date.getDay()]}`), time: timeStr })
  }
  return t('anime.timeMsk', { time: date.toLocaleDateString(locale.value === 'ru' ? 'ru-RU' : locale.value === 'ja' ? 'ja-JP' : 'en-US', { day: 'numeric', month: 'long', hour: '2-digit', minute: '2-digit', timeZone: 'Europe/Moscow' }) })
}

const formatEpisodeCount = (anime: { episodesAired?: number; totalEpisodes?: number; status?: string }) => {
  const aired = anime.episodesAired || 0
  const total = anime.totalEpisodes || 0

  if (total > 0) {
    // Total known - show "aired / total" for ongoing, or just "total" for completed
    if (anime.status === 'ongoing' && aired > 0 && aired < total) {
      return t('anime.episodeProgress', { aired, total })
    }
    return t('anime.episodeTotal', { total })
  } else if (aired > 0) {
    // Total unknown but some aired
    return t('anime.episodeAiredUnknown', { aired })
  }
  // Nothing known
  return t('anime.episodeUnknown')
}

// Phase 14 / UX-28 — render compact watchers count ("1.2K", "12K") via the
// platform's Intl.NumberFormat. Locale-aware: passes the active i18n locale
// so the grouping/notation matches the user's language settings.
const formatCount = (n: number): string => {
  try {
    return new Intl.NumberFormat(locale.value, { notation: 'compact', maximumFractionDigits: 1 }).format(n)
  } catch {
    // Old browser / unknown locale — graceful degradation.
    return n.toString()
  }
}

// Phase 14 / UX-28 — soft social-proof fetch. Public endpoint, no auth.
// Errors are swallowed: missing badge is preferable to a noisy console for
// a non-critical UI signal. Endpoint returns { count: number } (or wrapped
// in { data: ... } depending on httputil response shape).
const fetchWatchersCount = async () => {
  if (!anime.value) return
  try {
    const response = await animeApi.getWatchersCount(anime.value.id)
    const payload = (response.data as { data?: { count?: number }; count?: number } | undefined) ?? {}
    const raw = payload.data ? payload.data.count : payload.count
    watchersCount.value = typeof raw === 'number' && raw >= 0 ? raw : 0
  } catch {
    watchersCount.value = 0
  }
}

const fetchWatchlistStatus = async () => {
  if (!authStore.isAuthenticated || !anime.value) return

  try {
    await watchlistStore.fetchWatchlist()
    const entries = watchlistStore.entries

    // Direct UUID match
    let entry = entries.find((e) => e.anime_id === anime.value?.id)

    // If not found, check for mal_XXXXX entries that match this anime's MAL ID
    if (!entry && anime.value.malId) {
      const malAnimeId = `mal_${anime.value.malId}`
      entry = entries.find((e) => e.anime_id === malAnimeId)

      if (entry) {
        // Auto-migrate from mal_XXXXX to real UUID
        try {
          await userApi.migrateListEntry(
            malAnimeId,
            anime.value.id
          )
        } catch (e) {
          console.warn('Auto-migration of MAL entry failed:', e)
        }
      }
    }

    if (entry) {
      currentListStatus.value = entry.status
    } else {
      currentListStatus.value = null
    }
  } catch (err) {
    console.error('Failed to fetch watchlist status:', err)
  }
}

const fetchHiddenStatus = () => {
  // Hidden status comes from the anime object itself
  if (anime.value) {
    isHidden.value = (anime.value as AnimeWithExtras).hidden || false
    editShikimoriId.value = (anime.value as AnimeWithExtras).shikimoriId || ''
  }
}

const toggleHidden = async () => {
  if (!anime.value || !authStore.isAdmin) return

  try {
    if (isHidden.value) {
      await adminApi.unhideAnime(anime.value.id)
      isHidden.value = false
    } else {
      await adminApi.hideAnime(anime.value.id)
      isHidden.value = true
    }
  } catch (err) {
    console.error('Failed to toggle hidden status:', err)
  }
}

const saveShikimoriId = async () => {
  if (!anime.value || !authStore.isAdmin || savingShikimoriId.value) return

  savingShikimoriId.value = true
  try {
    await adminApi.updateShikimoriId(anime.value.id, editShikimoriId.value)
    showShikimoriEdit.value = false
    // Refresh anime data to get updated translations
    await fetchAnime(anime.value.id)
  } catch (err) {
    console.error('Failed to update Shikimori ID:', err)
  } finally {
    savingShikimoriId.value = false
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
      reviewForm.text
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

// --- Comments (SOCIAL-06) ---

const fetchComments = async () => {
  if (!anime.value) return
  commentsLoading.value = true
  commentsError.value = ''
  try {
    const resp = await commentApi.getAnimeComments(anime.value.id, { limit: 50 })
    const data = resp.data?.data || resp.data || {}
    comments.value = data.comments || []
    commentsHasMore.value = !!data.has_more
    commentsNextCursor.value = data.next_cursor || ''
    commentsFetched.value = true
  } catch (err) {
    console.error('Failed to fetch comments:', err)
    commentsError.value = t('anime.ugc.loadFailed')
  } finally {
    commentsLoading.value = false
  }
}

const loadMoreComments = async () => {
  if (!anime.value || !commentsHasMore.value || commentsLoadingMore.value) return
  commentsLoadingMore.value = true
  commentsError.value = ''
  try {
    const resp = await commentApi.getAnimeComments(anime.value.id, {
      limit: 50,
      cursor: commentsNextCursor.value,
    })
    const data = resp.data?.data || resp.data || {}
    const newPage: Comment[] = data.comments || []
    // Dedup by id in case the cursor reaches an already-loaded boundary.
    const known = new Set(comments.value.map((c) => c.id))
    for (const c of newPage) {
      if (!known.has(c.id)) comments.value.push(c)
    }
    commentsHasMore.value = !!data.has_more
    commentsNextCursor.value = data.next_cursor || ''
  } catch (err) {
    console.error('Failed to load more comments:', err)
    commentsError.value = t('anime.ugc.loadMoreFailed')
  } finally {
    commentsLoadingMore.value = false
  }
}

interface ApiError {
  response?: { status?: number; data?: { error?: string; message?: string } }
}

const postComment = async () => {
  if (!anime.value) return
  const trimmed = newCommentBody.value.trim()
  if (!trimmed) {
    postError.value = t('anime.ugc.bodyEmpty')
    return
  }
  if (runeLen(trimmed) > 2000) {
    postError.value = t('anime.ugc.bodyTooLong')
    return
  }
  posting.value = true
  postError.value = ''
  try {
    const resp = await commentApi.createComment(anime.value.id, trimmed)
    // REVIEW.md WR-03: prepend the newly-created comment to the list
    // instead of refetching page 1. Refetching destroys commentsNextCursor /
    // commentsHasMore and snaps users who had paginated through several
    // pages of comments back to the top. Prepending preserves cursor
    // state and lets infinite scroll continue working below the newly
    // posted card.
    const created: Comment | undefined = resp.data?.data || resp.data
    if (created?.id) {
      comments.value.unshift(created)
      commentsFetched.value = true
    }
    newCommentBody.value = ''
  } catch (err) {
    const e = err as ApiError
    const status = e.response?.status
    if (status === 429) {
      postError.value = t('anime.ugc.rateLimitError')
    } else if (status === 400) {
      const body = (e.response?.data?.error || e.response?.data?.message || '').toString().toLowerCase()
      if (body.includes('long') || body.includes('2000')) {
        postError.value = t('anime.ugc.bodyTooLong')
      } else {
        postError.value = t('anime.ugc.bodyEmpty')
      }
    } else {
      postError.value = t('anime.ugc.loadFailed')
    }
    console.error('Failed to post comment:', err)
  } finally {
    posting.value = false
  }
}

const startEditComment = (c: Comment) => {
  editingCommentId.value = c.id
  editingBody.value = c.body
  editError.value = ''
}

const cancelEditComment = () => {
  editingCommentId.value = null
  editingBody.value = ''
  editError.value = ''
}

const saveEditComment = async () => {
  if (!anime.value || !editingCommentId.value) return
  const trimmed = editingBody.value.trim()
  if (!trimmed) {
    editError.value = t('anime.ugc.bodyEmpty')
    return
  }
  if (runeLen(trimmed) > 2000) {
    editError.value = t('anime.ugc.bodyTooLong')
    return
  }
  const id = editingCommentId.value
  editSaving.value = true
  editError.value = ''
  try {
    const resp = await commentApi.updateComment(anime.value.id, id, trimmed)
    const updated: Comment = resp.data?.data || resp.data
    const idx = comments.value.findIndex((c) => c.id === id)
    if (idx !== -1 && updated && updated.id) {
      comments.value.splice(idx, 1, updated)
    } else if (idx !== -1) {
      // Server returned no body — patch the local copy with the new text.
      comments.value[idx] = { ...comments.value[idx], body: trimmed, updated_at: new Date().toISOString() }
    }
    cancelEditComment()
  } catch (err) {
    console.error('Failed to update comment:', err)
    editError.value = t('anime.ugc.editFailed')
  } finally {
    editSaving.value = false
  }
}

const deleteCommentItem = async (c: Comment) => {
  if (!anime.value) return
  if (!window.confirm(t('anime.ugc.deleteCommentConfirm'))) return
  const originalIdx = comments.value.findIndex((x) => x.id === c.id)
  if (originalIdx === -1) return
  const snapshot = comments.value[originalIdx]
  // Optimistic remove.
  comments.value.splice(originalIdx, 1)
  deleteError.value = ''
  try {
    await commentApi.deleteComment(anime.value.id, c.id)
  } catch (err) {
    console.error('Failed to delete comment:', err)
    // REVIEW.md WR-04: restore via id-keyed re-insertion rather than
    // splicing at the captured originalIdx. Between the optimistic
    // removal and the error path, the user may have triggered
    // loadMoreComments() (appends new entries) or saveEditComment()
    // (mutates in place), which would make originalIdx stale and put
    // the restored card at the wrong position. Find a stable anchor by
    // walking the current array and inserting before the first comment
    // strictly older than the snapshot, falling back to the front if
    // it's the newest or the array is empty. Newest-first ordering is
    // preserved without depending on a captured index.
    const insertAt = comments.value.findIndex((x) => {
      const a = new Date(snapshot.created_at).getTime()
      const b = new Date(x.created_at).getTime()
      return a > b || (a === b && snapshot.id > x.id)
    })
    if (insertAt === -1) {
      comments.value.push(snapshot)
    } else {
      comments.value.splice(insertAt, 0, snapshot)
    }
    deleteError.value = t('anime.ugc.deleteFailed')
    // Auto-clear after 5 seconds (mirrors the SPEC's "auto-dismiss 5s").
    setTimeout(() => {
      if (deleteError.value === t('anime.ugc.deleteFailed')) deleteError.value = ''
    }, 5000)
  }
}

// Watchers — Vue+Router URL-persistence pattern (RESEARCH.md Pattern 6).
// Two-way: route.query.ugc → ugcTab handles deep links + back/forward;
// ugcTab → router.replace + lazy fetch on first activation.
watch(
  () => route.query.ugc,
  (v) => {
    const val = (typeof v === 'string' ? v : 'reviews') as UgcTab
    const normalized: UgcTab = (UGC_ALLOWED as readonly string[]).includes(val) ? val : 'reviews'
    if (normalized !== ugcTab.value) ugcTab.value = normalized
  }
)

watch(ugcTab, (v) => {
  if (route.query.ugc !== v) {
    router.replace({ query: { ...route.query, ugc: v } })
  }
  if (v === 'comments' && !commentsFetched.value && !commentsLoading.value) {
    void fetchComments()
  }
})

const setListStatus = async (status: string) => {
  if (!anime.value) return
  const animeId = anime.value.id
  const prior = currentListStatus.value
  // Optimistic: flip the visible status + close the dropdown immediately.
  currentListStatus.value = status
  showStatusDropdown.value = false
  try {
    await watchlistStore.setStatusOptimistic(animeId, status)
  } catch (err) {
    console.error('Failed to update list status:', err)
    // Rollback the view-local mirror (store action already rolled back its map).
    currentListStatus.value = prior
    toast.push(t('watchlist.errors.updateFailed'))
  }
}

const removeFromList = async () => {
  if (!anime.value) return
  const animeId = anime.value.id
  const prior = currentListStatus.value
  // Optimistic: clear the visible status + close the dropdown immediately.
  currentListStatus.value = null
  showStatusDropdown.value = false
  try {
    await watchlistStore.removeEntryOptimistic(animeId)
  } catch (err) {
    console.error('Failed to remove from list:', err)
    currentListStatus.value = prior
    toast.push(t('watchlist.errors.removeFailed'))
  }
}

// Close dropdown on click outside
const handleClickOutside = (event: MouseEvent) => {
  if (dropdownRef.value && !dropdownRef.value.contains(event.target as Node)) {
    showStatusDropdown.value = false
  }
}

const refreshAnimeData = async () => {
  if (!anime.value || refreshing.value) return

  refreshing.value = true
  try {
    await animeApi.refresh(anime.value.id)
    // Refetch anime data to show updated info
    await fetchAnime(anime.value.id)
  } catch (err) {
    console.error('Failed to refresh anime data:', err)
  } finally {
    refreshing.value = false
  }
}

const retry = () => {
  const animeId = route.params.id as string
  fetchAnime(animeId)
}

// Language / provider switching
const switchLanguage = (lang: 'ru' | 'en' | '18+' | 'raw') => {
  videoLanguage.value = lang
  // Auto-select first provider in the group
  if (lang === 'ru') {
    const savedRu = localStorage.getItem('preferred_ru_provider') as 'kodik' | 'animelib' | null
    videoProvider.value = savedRu && (savedRu !== 'animelib' || animeLibEnabled) ? savedRu : 'kodik'
  } else if (lang === 'en') {
    // Phase 24-28 — single-provider group; the in-player source dropdown
    // pins the failover preference inside OurEnglishPlayer itself.
    videoProvider.value = 'ourenglish'
  } else if (lang === '18+') {
    videoProvider.value = 'hanime'
  } else if (lang === 'raw') {
    // Workstream raw-jp, Phase 04 — single-option group for v0.1; the
    // preferred_raw_provider key exists for v0.2's hybrid resolver
    // (where 'minio' joins 'raw' as a viable choice).
    const savedRaw = localStorage.getItem('preferred_raw_provider') as 'raw' | null
    videoProvider.value = savedRaw || 'raw'
  }
}

// Save preferred video provider to localStorage
watch(videoProvider, (newProvider) => {
  localStorage.setItem('preferred_video_provider', newProvider)
  if (videoLanguage.value === 'ru') {
    localStorage.setItem('preferred_ru_provider', newProvider)
  } else if (videoLanguage.value === 'raw') {
    localStorage.setItem('preferred_raw_provider', newProvider)
  }
})

watch(videoLanguage, (newLang) => {
  localStorage.setItem('preferred_video_language', newLang)
})

// Shared data-loading function — called on mount and on route param change
const loadAnimeData = async (animeId: string) => {
  // Increment generation so previous in-flight calls become stale
  const gen = ++loadGeneration

  // Reset state so stale data doesn't flash
  anime.value = null
  lastEpisode.value = undefined
  resume.reset()
  resumeOverrideEpisode.value = null
  currentListStatus.value = null
  reviews.value = []
  myReview.value = null
  siteRating.value = null
  watchersCount.value = 0
  // Comments — reset cache for new anime so a stale list doesn't leak across
  // navigations. Per-anime fetch is gated on tab activation (or deep-link
  // ugc=comments path below).
  comments.value = []
  commentsHasMore.value = false
  commentsNextCursor.value = ''
  commentsError.value = ''
  commentsFetched.value = false
  newCommentBody.value = ''
  postError.value = ''
  editingCommentId.value = null
  editingBody.value = ''
  editError.value = ''
  deleteError.value = ''
  synopsisExpanded.value = false
  showStatusDropdown.value = false
  reviewForm.score = 0
  reviewForm.text = ''

  // Handle MAL-prefixed IDs: resolve via backend
  if (animeId.startsWith('mal_')) {
    const malId = animeId.replace('mal_', '')
    try {
      const response = await animeApi.resolveMAL(malId)
      if (gen !== loadGeneration) return
      const result = response.data?.data || response.data
      if (result?.status === 'resolved' && result.anime) {
        // Migrate list entry if user is authenticated
        if (authStore.isAuthenticated) {
          try {
            await userApi.migrateListEntry(
              animeId,
              result.anime.id
            )
          } catch (e) {
            console.warn('List migration failed:', e)
          }
        }
        router.replace(`/anime/${result.anime.id}`)
        return
      } else if (result?.status === 'ambiguous') {
        const searchQuery = result.mal_title || ''
        router.replace({ path: '/browse', query: { q: searchQuery, bind_mal: animeId } })
        return
      }
    } catch (e) {
      console.error('MAL resolution failed:', e)
    }
  }

  const fetched = await fetchAnime(animeId)
  if (gen !== loadGeneration) return

  // Reset 18+ tab if this anime doesn't have the Hentai genre
  if (videoLanguage.value === '18+') {
    const fetchedIsHentai = fetched?.rawGenres?.some(g => g.name === 'Hentai') ?? false
    if (!fetchedIsHentai) {
      videoLanguage.value = 'ru'
    }
  }

  // Load last-watched episode from localStorage (anon path) and from server
  // watch_progress in parallel (auth path).
  if (fetched) {
    loadLastEpisode(fetched.id)
    void resume.init()
  }

  // Initialize watch preferences for this anime — anon users included so the
  // combo_resolve_total denominator increments alongside combo_override_total
  // (CONTEXT D-12: per-anon-user override rate). The composable + axios
  // interceptor handle the X-Anon-ID header for unauthenticated callers.
  if (fetched) {
    initPreferences(fetched.id)
  }

  // Check for pending MAL bind from Browse page
  const pendingBind = sessionStorage.getItem('pending_mal_bind')
  if (pendingBind && fetched && authStore.isAuthenticated) {
    sessionStorage.removeItem('pending_mal_bind')
    try {
      await userApi.migrateListEntry(
        pendingBind,
        fetched.id
      )
    } catch (e) {
      console.warn('Pending MAL bind migration failed:', e)
    }
  }

  await fetchWatchlistStatus()
  if (gen !== loadGeneration) return
  await fetchHiddenStatus()
  if (gen !== loadGeneration) return
  await fetchReviews()
  // Phase 14 / UX-28 — non-blocking; failure leaves the badge hidden.
  void fetchWatchersCount()

  // Deep-link path: if the URL already has ?ugc=comments on first paint,
  // kick off the initial comments fetch (the watch(ugcTab) lazy-fetch only
  // fires on subsequent changes, not on initial value).
  if (ugcTab.value === 'comments' && !commentsFetched.value) {
    void fetchComments()
  }

  // Fetch related anime (non-blocking)
  fetchRelatedAnime()
}

async function fetchRelatedAnime() {
  if (!anime.value?.id) return
  try {
    const resp = await animeApi.getRelated(anime.value.id as string)
    const data = (resp.data?.data || resp.data) as Array<{
      shikimori_id: string
      local_id?: string
      name: string
      name_ru: string
      relation_ru: string
      relation_en: string
      score: number
      status: string
      poster_url: string
    }>
    relatedAnime.value = data.map(r => ({
      id: r.local_id || `shiki_${r.shikimori_id}`,
      title: r.name,
      nameRu: r.name_ru,
      name: r.name,
      coverImage: getImageUrl(r.poster_url),
      rating: r.score || undefined,
      relationLabel: locale.value === 'ru' ? r.relation_ru : r.relation_en,
    }))
  } catch (e) {
    console.warn('Failed to fetch related anime:', e)
  }
}

// Re-load when route param changes (Vue Router reuses the component for /anime/:id)
watch(() => route.params.id, (newId) => {
  if (newId) {
    loadAnimeData(newId as string)
  }
})

// UA-051 (UX-04 Phase 2): inject the anime title into <title> once data has
// loaded, so /anime/:id renders e.g. "Chainsaw Man — AnimeEnigma" instead of
// the static "Детали аниме - AnimeEnigma" fallback set by the router guard.
// `anime.title` is already locale-resolved by the transform in useAnime.ts.
watch(() => anime.value?.title, (newTitle) => {
  if (newTitle) {
    document.title = `${newTitle} — AnimeEnigma`
  }
})

onMounted(() => {
  document.addEventListener('click', handleClickOutside)
  loadAnimeData(route.params.id as string)
  // Phase 11 / UX-23 — apply persisted theater-mode state + bind ESC.
  applyBodyTheaterClass(theaterMode.value)
  document.addEventListener('keydown', onTheaterEscape)
})

// Phase 11 / UX-23 — MANDATORY theater-mode cleanup. onBeforeUnmount
// strips the body class so navigating away from /anime/:id never leaves
// the global navbar / non-player sections hidden everywhere else.
onBeforeUnmount(() => {
  applyBodyTheaterClass(false)
  document.removeEventListener('keydown', onTheaterEscape)
})

// React to programmatic state changes (toggle button click, ESC).
watch(theaterMode, (on) => applyBodyTheaterClass(on))

onUnmounted(() => {
  loadGeneration++ // cancel any in-flight loadAnimeData
  document.removeEventListener('click', handleClickOutside)
})
</script>

<style scoped>
:deep(.shiki-link) {
  color: rgb(34 211 238);
  text-decoration: none;
  border-bottom: 1px dotted rgb(34 211 238 / 0.4);
}
:deep(.shiki-link:hover) {
  text-decoration: underline;
}
:deep(.shiki-footnote) {
  font-size: 0.75rem;
  color: rgb(255 255 255 / 0.4);
}
</style>

<!-- Phase 11 / UX-23 — Theater Mode global rules.
     Unscoped because the selectors target body.theater-mode and the
     global .navbar-root class from <Navbar />, which a scoped block
     could not reach. Anime.vue is the only mount site for theater mode
     so co-locating the CSS here keeps it discoverable. -->
<style>
body.theater-mode .navbar-root {
  display: none !important;
}
body.theater-mode .non-player-content {
  display: none;
}
body.theater-mode [data-anime-player-wrapper="true"] {
  max-width: none !important;
  margin-left: 0 !important;
  margin-right: 0 !important;
  padding-left: 0 !important;
  padding-right: 0 !important;
}
</style>
