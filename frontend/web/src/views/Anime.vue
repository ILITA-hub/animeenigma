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
    <div class="anime-main relative z-10 max-w-7xl mx-auto px-4 lg:px-8 -mt-64 md:-mt-72">
      <div class="flex flex-col md:flex-row gap-6 md:gap-8">
        <!-- Poster -->
        <div class="flex-shrink-0">
          <button
            type="button"
            class="block w-40 md:w-56 cursor-zoom-in rounded-xl"
            :aria-label="$t('anime.posterZoom.open')"
            @click="openPosterZoom"
          >
            <PosterImage
              :src="anime.coverImage"
              :alt="anime.title"
              ratio="2/3"
              rounded="xl"
              :proxy-width="448"
              class="shadow-2xl ring-1 ring-white/10"
            />
          </button>
          <PosterLightbox
            v-if="posterZoomEverOpened"
            v-model="posterZoomOpen"
            :src="anime.coverImage"
            :alt="anime.title"
          />
        </div>

        <!-- Info -->
        <div class="flex-1 pt-2">
          <!-- Title -->
          <h1 class="text-2xl md:text-4xl font-semibold text-white mb-2">
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

          <!-- Ratings -->
          <div class="flex flex-wrap items-center gap-4 mb-4">
            <!-- Shikimori Rating -->
            <div v-if="anime.rating" class="flex items-center gap-2">
              <div class="flex items-center gap-1 text-warning">
                <Star class="size-5" fill="currentColor" />
                <span class="font-semibold text-lg">{{ anime.rating.toFixed(1) }}</span>
              </div>
              <span class="text-white/60 text-sm">Shikimori</span>
            </div>

            <!-- Site Rating -->
            <div v-if="siteRating && siteRating.total_reviews > 0" class="flex items-center gap-2">
              <div class="flex items-center gap-1 text-cyan-400">
                <ScoreDiamond class="size-5" />
                <span class="font-semibold text-lg">{{ siteRating.average_score.toFixed(1) }}</span>
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
              class="flex items-center gap-2 px-6 py-3 rounded-lg font-semibold bg-white/5 text-white/70 border border-white/10 cursor-default"
            >
              <Clock class="size-5" aria-hidden="true" />
              <span>{{ premiereDate ? $t('anime.notReleased.cta', { date: premiereDate }) : $t('anime.notReleased.ctaNoDate') }}</span>
            </div>
            <template v-else>
              <Button
                v-if="watchCta.action !== 'rewatch'"
                @click="onWatchCtaClick"
                type="button"
                variant="default"
                size="md"
                radius="lg"
                class="shadow-lg shadow-cyan-500/20"
              >
                <Check v-if="watchCta.action === 'mark-watched'" class="size-5" aria-hidden="true" />
                <Play v-else class="size-5" fill="currentColor" aria-hidden="true" />
                <span>{{ watchCtaLabel }}</span>
              </Button>
              <!-- Workstream watch-together — discovery-stage Invite mount.
                   Anonymous users don't see it (creating a room requires JWT).
                   The button fetches a translation_id from the catalog on
                   click (~100ms) when none is resolved yet, so clicking
                   never waits on the player chain. -->
              <InviteButton
                v-if="authStore.isAuthenticated && anime"
                :anime-id="anime.id"
                :episode-id="wtInvitePayload.episodeId"
                :player="wtInvitePayload.player"
                :translation-id="wtInvitePayload.translationId"
              />
            </template>
          </div>

          <!-- Actions / Status row — status (user) · next-episode (everyone) · admin kebab (admin) -->
          <div
            v-if="authStore.isAuthenticated || (anime.nextEpisodeAt && anime.status === 'ongoing')"
            class="flex flex-wrap items-center gap-3 mb-6"
          >
            <!-- Watchlist Status Dropdown -->
            <div v-if="authStore.isAuthenticated" class="relative" ref="dropdownRef">
              <button
                @click="showStatusDropdown = !showStatusDropdown"
                type="button"
                aria-haspopup="menu"
                :aria-expanded="showStatusDropdown"
                aria-controls="watchlist-status-menu"
                class="flex items-center gap-2 h-10 px-4 rounded-lg font-medium transition-all"
                :class="currentListStatus
                  ? 'bg-cyan-500/20 text-cyan-400 border border-cyan-500/30 hover:bg-cyan-500/30'
                  : 'bg-white/5 text-white border border-white/10 hover:bg-white/10'"
              >
                <Check v-if="currentListStatus" class="size-5" aria-hidden="true" />
                <Plus v-else class="size-5" aria-hidden="true" />
                <span>{{ currentListStatus ? statusLabels[currentListStatus] : $t('anime.addToList') }}</span>
                <ChevronDown class="size-4 transition-transform" :class="{ 'rotate-180': showStatusDropdown }" aria-hidden="true" />
              </button>

              <!-- bespoke-keep: single-select status picker with radio semantics
                   (role=menuitemradio + aria-checked) and inline (non-portaled)
                   positioning. The ui DropdownMenu wrapper is action-menu-only
                   (Item/Separator/Label, no radio variant); migrating would lose
                   the radio a11y or require extending a shared primitive. The
                   admin kebab below DOES use the DropdownMenu primitive. -->
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
                    <Check v-if="currentListStatus === status" class="size-4" aria-hidden="true" />
                  </button>

                  <!-- Rewatch tally — moved here from the hero (an unlabeled
                       ↻ − 0 + confused new users). Shown once it can matter:
                       completed entries, or any entry with a count. +/− keep
                       the menu open (click-outside only closes on outside). -->
                  <div
                    v-if="currentListStatus === 'completed' || currentRewatchCount > 0"
                    data-testid="rewatch-menu-row"
                    class="border-t border-white/10 px-4 py-3 flex items-center justify-between gap-2 text-sm text-white/80"
                  >
                    <span class="flex items-center gap-1.5">
                      <span class="opacity-70" aria-hidden="true">↻</span>
                      {{ $t('anime.rewatchLabel') }}
                    </span>
                    <span class="inline-flex items-center gap-1">
                      <button
                        type="button"
                        data-testid="rewatch-menu-dec"
                        class="px-1.5 leading-none transition-colors hover:text-white disabled:opacity-40"
                        :disabled="currentRewatchCount <= 0"
                        :aria-label="$t('anime.rewatchDec')"
                        @click="setRewatchCount(currentRewatchCount - 1)"
                      >
                        −
                      </button>
                      <span class="tabular-nums text-white">{{ currentRewatchCount }}</span>
                      <button
                        type="button"
                        data-testid="rewatch-menu-inc"
                        class="px-1.5 leading-none transition-colors hover:text-white"
                        :aria-label="$t('anime.rewatchInc')"
                        @click="setRewatchCount(currentRewatchCount + 1)"
                      >
                        +
                      </button>
                    </span>
                  </div>

                  <!-- Remove from list -->
                  <div v-if="currentListStatus" class="border-t border-white/10">
                    <button
                      @click="removeFromList"
                      role="menuitem"
                      class="w-full px-4 py-3 text-left text-sm text-pink-400 hover:bg-pink-500/10 transition-colors flex items-center gap-2"
                    >
                      <Trash2 class="size-4" aria-hidden="true" />
                      {{ $t('anime.removeFromList') }}
                    </button>
                  </div>
                </div>
              </Transition>
            </div>

            <!-- Next Episode Info — sits between the status dropdown and the
                 admin kebab; shown to everyone (incl. anonymous), not auth-gated. -->
            <div
              v-if="anime.nextEpisodeAt && anime.status === 'ongoing'"
              class="inline-flex items-center gap-2 h-10 px-3 rounded-lg bg-cyan-500/10 backdrop-blur-xl border border-cyan-500/20"
            >
              <Clock class="size-5 text-cyan-400" aria-hidden="true" />
              <span class="text-cyan-400 font-medium">
                {{ $t('anime.nextEpisode', { episode: (anime.episodesAired || 0) + 1 }) }}
              </span>
              <span class="text-white">
                {{ formatNextEpisode(anime.nextEpisodeAt) }}
              </span>
            </div>

            <!-- Admin Tools (Admin only) — trigger is absolutely positioned so
                 the kebab floats to the hero top-right (anchored to the relative
                 hero container at line ~16), out of the user action row. Muted-
                 pink tint marks the admin zone; the portaled menu stays anchored
                 to the trigger. -->
            <DropdownMenu
              v-if="authStore.isAdmin"
              v-model:open="showAdminMenu"
              align="end"
              side="bottom"
            >
              <template #trigger>
                <button
                  type="button"
                  :aria-label="$t('anime.adminMenu')"
                  aria-haspopup="menu"
                  :aria-expanded="showAdminMenu"
                  class="absolute top-0 right-4 lg:right-8 z-20 flex items-center justify-center w-10 h-10 rounded-lg bg-pink-500/10 text-pink-400/80 border border-pink-500/25 hover:bg-pink-500/20 hover:text-pink-300 transition-colors"
                >
                  <EllipsisVertical class="size-5" aria-hidden="true" />
                </button>
              </template>

              <!-- Stylized /admin label — non-interactive menu header -->
              <div class="px-2 pt-1 pb-1.5 mb-1 border-b border-white/10 select-none">
                <span class="font-mono text-[11px] uppercase tracking-[0.18em] text-pink-400/70">/admin</span>
              </div>

              <!-- Refresh Data — moved out of the user row; admin/maintenance only -->
              <DropdownMenuItem
                :disabled="refreshing"
                class="w-full flex items-center gap-2 px-2 py-1.5 rounded-lg text-sm transition-colors text-left cursor-pointer outline-none text-white/70 hover:bg-white/5 hover:text-white data-[highlighted]:bg-white/5 data-[highlighted]:text-white data-[disabled]:opacity-50 data-[disabled]:cursor-not-allowed"
                @select="refreshAnimeData"
              >
                <RefreshCw class="size-4 flex-shrink-0" :class="{ 'animate-spin': refreshing }" aria-hidden="true" />
                {{ refreshing ? $t('anime.refreshing') : $t('anime.refresh') }}
              </DropdownMenuItem>

              <!-- Hide / Unhide -->
              <DropdownMenuItem
                class="w-full flex items-center gap-2 px-2 py-1.5 rounded-lg text-sm transition-colors text-left cursor-pointer outline-none data-[highlighted]:bg-white/5"
                :class="isHidden
                  ? 'text-warning hover:bg-warning/10 data-[highlighted]:bg-warning/10'
                  : 'text-white/70 hover:bg-white/5 hover:text-white data-[highlighted]:text-white'"
                @select="toggleHidden"
              >
                <Eye v-if="isHidden" class="size-4 flex-shrink-0" aria-hidden="true" />
                <EyeOff v-else class="size-4 flex-shrink-0" aria-hidden="true" />
                {{ isHidden ? $t('anime.unhide') : $t('anime.hide') }}
              </DropdownMenuItem>

              <!-- Edit Shikimori ID — toggles the inline edit panel below -->
              <DropdownMenuItem
                class="w-full flex items-center gap-2 px-2 py-1.5 rounded-lg text-sm transition-colors text-left cursor-pointer outline-none text-white/70 hover:bg-white/5 hover:text-white data-[highlighted]:bg-white/5 data-[highlighted]:text-white"
                @select="showShikimoriEdit = !showShikimoriEdit"
              >
                <Pencil class="size-4 flex-shrink-0" aria-hidden="true" />
                Shikimori ID
              </DropdownMenuItem>
            </DropdownMenu>
          </div>

          <!-- Shikimori ID Edit Panel (Admin only) -->
          <div v-if="authStore.isAdmin && showShikimoriEdit" class="mb-4 p-3 rounded-lg bg-white/5 border border-white/10">
            <div class="flex items-center gap-3">
              <label class="text-white/60 text-sm whitespace-nowrap">Shikimori ID:</label>
              <div class="flex-1">
                <Input v-model="editShikimoriId" type="text" size="sm" :placeholder="$t('anime.examplePlaceholder')" />
              </div>
              <Button
                @click="saveShikimoriId"
                :disabled="savingShikimoriId"
                variant="default"
                size="sm"
                radius="lg"
              >
                {{ savingShikimoriId ? '...' : $t('anime.save') }}
              </Button>
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
      <section v-if="anime.description" id="section-overview" class="mt-8">
        <h2 class="text-xl font-semibold text-white mb-3">{{ $t('anime.synopsis') }}</h2>
        <div class="glass-card p-4">
          <p
            class="text-white/70 leading-relaxed"
            :class="{ 'line-clamp-4': !synopsisExpanded }"
            v-html="parsedDescription"
          />
          <Button
            v-if="anime.description && anime.description.length > 300"
            variant="link"
            @click="synopsisExpanded = !synopsisExpanded"
            class="mt-2 text-sm"
          >
            {{ synopsisExpanded ? $t('anime.showLess') : $t('anime.showMore') }}
          </Button>
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
        <div class="player-head flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3 mb-4">
          <div class="flex items-center justify-between gap-3 sm:gap-4">
            <h2 class="text-xl font-semibold text-white">
              <span class="flex items-center gap-2">
                <Play class="size-6 text-cyan-400" fill="currentColor" aria-hidden="true" />
                {{ $t('anime.watch') || 'Смотреть онлайн' }}
              </span>
            </h2>
          </div>
          <!-- InviteButton is mounted ABOVE THE FOLD next to the Primary
               Watch CTA (see line ~128). The previous mount here (gated on
               playerActivated, adjacent to the language tabs) was removed
               to keep a single discovery point next to Continue Watching. -->
          <!-- Plan B — the per-language tabs + provider sub-tabs are retired.
               AePlayer is the DEFAULT player and selects every surviving source
               (ae / kodik-HLS / EN scraper chain / raw / 18anime) inside its own
               SourcePanel. The only page-level surface toggle is the binary
               "Classic Kodik" iframe fallback. Hidden for not-yet-released
               titles (no sources to switch). -->
          <div
            v-if="!notReleasedYet && aePlayerEnabled"
            class="flex flex-wrap items-center gap-2 player-tabs"
          >
            <Button
              type="button"
              variant="ghost"
              size="sm"
              radius="md"
              :aria-pressed="classicKodik"
              @click="classicKodik = !classicKodik"
            >
              {{ classicKodik ? $t('player.classicKodik.back') : $t('player.classicKodik.switch') }}
            </Button>
          </div>
        </div>

        <!-- Default surface: AePlayer. Premiere notice replaces it for
             not-yet-released titles so users see "premieres on {date}" rather
             than an empty-source error. -->
        <div class="glass-card player-card p-4 md:p-6" v-if="!classicKodik && aePlayerEnabled">
          <div
            v-if="notReleasedYet"
            class="w-full aspect-video rounded-lg bg-white/5 border border-white/10 flex flex-col items-center justify-center text-center gap-3 px-6"
          >
            <Calendar class="size-12 text-cyan-400/80" aria-hidden="true" />
            <p class="text-lg font-semibold text-white">{{ $t('anime.notReleased.title') }}</p>
            <p class="text-sm text-white/60 max-w-md">
              {{ premiereDate ? $t('anime.notReleased.withDate', { date: premiereDate }) : $t('anime.notReleased.noDate') }}
            </p>
          </div>
          <AePlayer
            v-else
            :anime-id="anime.id"
            :anime="{ title: anime.title, eps: (anime.totalEpisodes || anime.episodesAired || 1), still: anime.coverImage, durationMin: anime.episodeDuration }"
            :theater="theaterMode"
            :can-theater="true"
            :is-hentai="isHentai"
            :initial-episode="resumeStartEpisode"
            :resume-banner="resumeBanner"
            :initial-provider="queryProvider"
            :initial-team="queryTeam"
            :initial-audio="queryAudio"
            :initial-lang="queryLang"
            :initial-timestamp="queryTimestamp"
            :mal-id="anime.shikimoriId"
            @toggle-theater="onToggleTheater"
            @combo-change="aeWtSeed = $event"
            @url-sync="onUrlSync"
          />
        </div>

        <!-- Classic Kodik fallback: the iframe KodikPlayer. Also the surface
             when AePlayer is disabled via VITE_AE_PLAYER_ENABLED=false. -->
        <div class="glass-card player-card p-4 md:p-6" v-else>
          <div
            v-if="notReleasedYet"
            class="w-full aspect-video rounded-lg bg-white/5 border border-white/10 flex flex-col items-center justify-center text-center gap-3 px-6"
          >
            <Calendar class="size-12 text-cyan-400/80" aria-hidden="true" />
            <p class="text-lg font-semibold text-white">{{ $t('anime.notReleased.title') }}</p>
            <p class="text-sm text-white/60 max-w-md">
              {{ premiereDate ? $t('anime.notReleased.withDate', { date: premiereDate }) : $t('anime.notReleased.noDate') }}
            </p>
          </div>
          <KodikPlayer
            v-else
            :anime-id="anime.id"
            :anime-name="anime.title"
            :total-episodes="anime.totalEpisodes"
            :episode-duration-min="anime.episodeDuration"
            :preferred-combo="resolvedCombo"
            :initial-episode="resumeStartEpisode"
            @available-translations="handleAvailableTranslations"
          >
            <template #header-middle>
              <ResumePill :banner="resumeBanner" />
            </template>
          </KodikPlayer>
          <!-- When AePlayer is disabled there's no toggle above; offer a way
               back is moot (Classic Kodik is the only surface). When AePlayer
               is enabled the toggle above flips back. -->
        </div>

        <PlayerDiscoveryTip v-if="!notReleasedYet" :key="anime.id" />
      </section>

      <!-- Reviews + Comments Section (SOCIAL-06: two-tab UGC strip) -->
      <!-- Phase 11 / UX-22 — section-comments anchor for AnimeQuickNav. -->
      <section id="section-comments" ref="ugcSectionEl" class="mt-8 cv-below-fold">
        <div class="flex items-center justify-between mb-4">
          <h2 class="text-xl font-semibold text-white">
            <span class="flex items-center gap-2">
              <MessageSquare class="size-6 text-cyan-400" />
              {{ ugcTab === 'comments' ? $t('anime.ugc.commentsTab') : $t('anime.reviews') }}
            </span>
          </h2>
          <span v-if="ugcTab === 'reviews' && reviews.length > 0" class="text-white/60 text-sm">{{ $t('anime.reviewsCount', { count: reviews.length }, reviews.length) }}</span>
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

          <!-- Score rating (◆ scale) -->
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
                <ScoreDiamond
                  class="size-6 sm:size-8 transition-colors"
                  :class="star <= reviewForm.score ? 'text-cyan-400' : 'text-white/30'"
                />
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
              class="w-full bg-white/5 border border-white/10 rounded-lg px-4 py-3 text-white placeholder-white/30 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-cyan-500/50 transition-colors resize-none"
              :placeholder="$t('anime.reviewPlaceholder')"
            ></textarea>
          </div>

          <!-- Submit Buttons -->
          <div class="flex gap-3">
            <Button
              @click="submitReview"
              :disabled="reviewForm.score === 0 || reviewSubmitting"
              variant="default"
              size="md"
              radius="lg"
            >
              {{ reviewSubmitting ? $t('anime.publishing') : (myReview ? $t('anime.update') : $t('anime.publish')) }}
            </Button>
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
                <Avatar :src="review.user_avatar" :name="review.username" size="md" />
                <div>
                  <router-link
                    :to="`/user/${review.user_id}`"
                    class="font-medium text-white hover:text-brand-violet transition-colors"
                  >
                    {{ review.username || $t('anime.user') }}
                  </router-link>
                  <p class="text-white/60 text-sm">
                    {{ formatDate(review.created_at) }}
                    <template v-if="review.status">
                      <span class="text-white/30 mx-1">·</span>
                      <span :class="isReviewFlagged(review) ? 'text-warning' : 'text-white/60'">{{ formatReviewStats(review) }}</span>
                    </template>
                  </p>
                </div>
              </div>
              <div class="flex items-center gap-1 text-cyan-400">
                <ScoreDiamond class="size-5" />
                <span class="font-semibold">{{ review.score }}</span>
              </div>
            </div>
            <p v-if="review.review_text" class="text-white/70 whitespace-pre-wrap">{{ review.review_text }}</p>
            <ReviewReactions
              class="mt-3"
              :review-id="review.id"
              :anime-id="anime.id"
              :initial-reactions="review.reactions"
              :viewer-reacted="review.my_reactions"
              :is-own-review="review.user_id === authStore.user?.id"
            />
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
                class="w-full bg-white/5 border border-white/10 rounded-lg px-3 py-2 text-white placeholder-white/30 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-cyan-500/50 transition-colors resize-none"
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
                <Button
                  type="button"
                  @click="postComment"
                  :disabled="posting || newCommentBody.trim().length === 0 || runeLen(newCommentBody.trim()) > 2000"
                  variant="default"
                  size="sm"
                  radius="lg"
                >
                  {{ posting ? $t('anime.ugc.posting') : $t('anime.ugc.postComment') }}
                </Button>
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
              <Button
                type="button"
                @click="fetchComments"
                variant="default"
                size="sm"
                radius="lg"
              >
                {{ $t('common.retry') }}
              </Button>
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
                    <Avatar :src="c.user_avatar" :name="c.username" size="md" />
                    <div>
                      <router-link
                        :to="`/user/${c.user_id}`"
                        class="font-semibold text-white hover:text-brand-violet transition-colors"
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
                      <Pencil class="size-5" aria-hidden="true" />
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
                      <Trash2 class="size-5" aria-hidden="true" />
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
                    class="w-full bg-white/5 border border-white/10 rounded-lg px-3 py-2 text-white placeholder-white/30 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-cyan-500/50 transition-colors resize-none"
                  ></textarea>
                  <div class="flex items-center gap-2 mt-2">
                    <Button
                      type="button"
                      @click="saveEditComment"
                      :disabled="editSaving || editingBody.trim().length === 0 || runeLen(editingBody.trim()) > 2000"
                      variant="default"
                      size="sm"
                      radius="lg"
                    >
                      {{ editSaving ? $t('anime.ugc.posting') : $t('anime.ugc.saveEdit') }}
                    </Button>
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
              <Button variant="soft" size="sm" :disabled="commentsLoadingMore" @click="loadMoreComments">
                {{ commentsLoadingMore ? $t('anime.ugc.loading') : $t('anime.ugc.loadMore') }}
              </Button>
            </div>
          </template>
        </Tabs>
      </section>

      <!-- Related Anime -->
      <!-- Lazy-load sentinel (page-fetch optimization 2026-06-11): the related
           rail itself is v-if'd on data, so this always-rendered marker is what
           the IntersectionObserver watches to trigger the Shikimori fetch. -->
      <div ref="relatedSentinelEl" aria-hidden="true" />
      <!-- Phase 11 / UX-22 — section-similar anchor for AnimeQuickNav. -->
      <section
        v-if="relatedAnime.length > 0"
        id="section-similar"
        class="mt-8 cv-below-fold"
      >
        <Carousel
          :items="relatedAnime"
          :title="$t('anime.related')"
          item-key="id"
          :item-width="{ mobile: 128, tablet: 160, desktop: 176, large: 190 }"
        >
          <template #default="{ item }">
            <div>
              <PosterCard
                :model="relatedCardModel(item as RelatedAnime)"
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
      <div ref="characterSentinelEl" aria-hidden="true" />
      <section
        v-if="characters.length > 0"
        id="section-characters"
        class="mt-8 cv-below-fold"
      >
        <Carousel
          :items="characters"
          :title="$t('characters.heading')"
          item-key="id"
          :item-width="{ mobile: 110, tablet: 128, desktop: 140, large: 150 }"
        >
          <template #default="{ item }">
            <CharacterCard :model="item as CharacterCardModel" />
          </template>
        </Carousel>
      </section>
    </div>
  </div>

  <!-- Loading State -->
  <div v-else-if="loading" class="min-h-screen flex items-center justify-center">
    <div class="text-center">
      <Spinner size="lg" class="mx-auto mb-4" />
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
    :anchor-el="contextMenu.anchorEl"
    :anime="contextMenu.anime"
    :list-status="contextMenu.listStatus"
    :site-rating="contextMenu.siteRating"
    @update:visible="contextMenu.visible = $event"
  />
</template>

<script setup lang="ts">
import { ref, computed, watch, defineAsyncComponent } from 'vue'
import { useMediaQuery } from '@vueuse/core'
import { Star, Clock, Play, Check, Plus, ChevronDown, Trash2, RefreshCw, Eye, EyeOff, Pencil, Calendar, MessageSquare, EllipsisVertical } from 'lucide-vue-next'
import { useAnime } from '@/composables/useAnime'
import { useAuthStore } from '@/stores/auth'
import { Avatar, Badge, Button, DropdownMenu, DropdownMenuItem, Input, ScoreDiamond, Spinner } from '@/components/ui'
import { GenreChip, PosterCard, PosterImage, AnimeContextMenu } from '@/components/anime'
import ReviewReactions from '@/components/anime/ReviewReactions.vue'
import CharacterCard from '@/components/anime/CharacterCard.vue'
import Carousel from '@/components/carousel/Carousel.vue'
import { useContextMenu } from '@/composables/useContextMenu'
import { useCharacters } from '@/composables/useCharacters'
import type { CharacterCardModel } from '@/types/character'
import ResumePill from '@/components/player/ResumePill.vue'
import PlayerDiscoveryTip from '@/components/player/PlayerDiscoveryTip.vue'
import AePlayerSkeleton from '@/components/player/aePlayer/AePlayerSkeleton.vue'
import Tabs from '@/components/ui/Tabs.vue'
import { getImageUrl } from '@/composables/useImageProxy'
import { runeLen } from '@/composables/anime/animeFormatters'
// Page-scoped composables — the <script setup> concerns of this view live in
// src/composables/animePage/* (one file per concern); this component keeps the
// template, the styles, and the composition wiring.
import type { AnimeWithExtras, RelatedAnime } from '@/composables/animePage/types'
import { useTheaterMode } from '@/composables/animePage/useTheaterMode'
import { useAnimeDisplay } from '@/composables/animePage/useAnimeDisplay'
import { useAnimeListStatus } from '@/composables/animePage/useAnimeListStatus'
import { useAnimeAdmin } from '@/composables/animePage/useAnimeAdmin'
import { useAnimeDeepLinks } from '@/composables/animePage/useAnimeDeepLinks'
import { useAnimeSocial } from '@/composables/animePage/useAnimeSocial'
import { useAnimeWatchFlow } from '@/composables/animePage/useAnimeWatchFlow'
import { usePlayerSurface } from '@/composables/animePage/usePlayerSurface'
import { useRelatedAnime } from '@/composables/animePage/useRelatedAnime'
import { useLazyAnimeSections } from '@/composables/animePage/useLazyAnimeSections'
import { useAnimeComments } from '@/composables/animePage/useAnimeComments'
import { useAnimeDataLoader } from '@/composables/animePage/useAnimeDataLoader'

// The unified aePlayer is the default playback surface. KodikPlayer remains
// as the deliberately separate "Classic Kodik" iframe fallback.
const KodikPlayer = defineAsyncComponent(() => import('@/components/player/KodikPlayer.vue'))
// Poster tap-to-zoom lightbox — lazy + mounted only after the first tap (the
// posterZoomEverOpened latch), so ~250 lines of gesture code stay out of the
// eager route chunk and the chunk isn't even fetched until someone zooms.
const PosterLightbox = defineAsyncComponent(() => import('@/components/anime/PosterLightbox.vue'))
// Unified player (Task 15) — single-surface player; default ON, set
// VITE_AE_PLAYER_ENABLED=false to disable for dark-ship.
// loadingComponent: selecting the tab unmounts the existing-player card
// immediately, so render a player-shaped skeleton (not a blank gap) while the
// chunk loads. delay:0 shows it instantly; the chunk is usually warm anyway.
const AePlayer = defineAsyncComponent({
  loader: () => import('@/components/player/aePlayer/AePlayer.vue'),
  loadingComponent: AePlayerSkeleton,
  delay: 0,
})
const aePlayerEnabled = import.meta.env.VITE_AE_PLAYER_ENABLED !== 'false'
// Workstream watch-together / Phase 02 Plan 02.9 (WT-SHELL-05) — lazy-loaded
// invite button. Keeps Anime.vue's eager bundle clean — InviteButton pulls in
// the watch-together api client + types + toast composable transitively, paid
// only on first render (i.e. when a logged-in user has activated the player).
const InviteButton = defineAsyncComponent(() => import('@/components/watch-together/InviteButton.vue'))

const authStore = useAuthStore()
const { anime, loading, error, fetchAnime } = useAnime()
const { contextMenu, openAtElement: openContextMenuAt } = useContextMenu()

// --- Page-local UI state ----------------------------------------------------
const synopsisExpanded = ref(false)
const posterZoomOpen = ref(false)
// Latch (never unset): keeps the lightbox mounted after first use so the
// close fade-out plays; gating v-if on posterZoomOpen would unmount mid-fade.
const posterZoomEverOpened = ref(false)
function openPosterZoom() {
  posterZoomEverOpened.value = true
  posterZoomOpen.value = true
}

// Phase 11 / UX-23 — Theater Mode (body class + ESC + localStorage persistence).
const { theaterMode, setTheater } = useTheaterMode()

// Codebase-standard reduced-motion check (see HeroSpotlightBlock.vue,
// Carousel.vue, RandomTailCard.vue, Gacha.vue) — replaces a hand-rolled
// matchMedia check.
const prefersReducedMotion = useMediaQuery('(prefers-reduced-motion: reduce)')

// The player sits below the hero + description, so entering theater must bring
// it up to the navbar line — that is the position the capped height is framed
// for. scroll-margin-top on the section (see the global style block) supplies
// the offset, so this stays free of hardcoded pixels.
async function onToggleTheater() {
  const on = !theaterMode.value
  setTheater(on)
  if (!on) return
  // Respect prefers-reduced-motion: jump instead of animating the scroll.
  // 'instant' is the standardized value that actually skips the animation —
  // 'auto' defers to the element's computed scroll-behavior, which this
  // codebase sets to 'smooth' globally, so 'auto' would silently animate.
  await scrollToPlayerSection(prefersReducedMotion.value ? 'instant' : 'smooth')
}

// Locale-bound formatters + derived metadata computeds.
const {
  statusVariant, parsedDescription, isHentai, notReleasedYet, premiereDate,
  formatDate, formatNextEpisode, formatEpisodeCount, formatCount,
  formatReviewStats, isReviewFlagged,
} = useAnimeDisplay(anime)

// Watchlist status dropdown + rewatch-count stepper (click-outside inside).
const listStatus = useAnimeListStatus(anime)
const {
  currentListStatus, currentRewatchCount, showStatusDropdown, dropdownRef,
  statusLabels, setRewatchCount, setListStatus, removeFromList,
} = listStatus

// Admin kebab (Refresh / Hide / Shikimori ID).
const {
  refreshing, isHidden, showShikimoriEdit, showAdminMenu, editShikimoriId,
  savingShikimoriId, fetchHiddenStatus, toggleHidden, saveShikimoriId, refreshAnimeData,
} = useAnimeAdmin(anime, fetchAnime)

// Deep-link query-param hints (?episode/?provider/?team/?audio/?lang/?t).
const { queryEpisode, queryProvider, queryTeam, queryAudio, queryLang, queryTimestamp } =
  useAnimeDeepLinks(anime)

// Reviews feed + own review + site rating + watchers count + viewer-context apply.
const social = useAnimeSocial(anime, currentListStatus, currentRewatchCount)
const {
  reviews, myReview, siteRating, watchersCount, reviewSubmitting, reviewForm,
  reviewsFetched, fetchReviewsList, submitReview, deleteMyReview,
} = social

// Unified watch state + CTA + player activation + rewatch flow.
const watchFlow = useAnimeWatchFlow({
  anime,
  currentListStatus,
  formatNextEpisode,
  queryEpisode,
  setListStatus,
  applyViewerContext: social.applyViewerContext,
})
const {
  playerSectionRef, scrollToPlayerSection, resumeStartEpisode, resumeBanner,
  watchCta, watchCtaLabel, onWatchCtaClick,
} = watchFlow

// Player surface: Classic Kodik toggle, preference resolution, WT seed, URL sync.
const {
  classicKodik, resolvedCombo, aeWtSeed, wtInvitePayload, onUrlSync,
  initPreferences, handleAvailableTranslations,
} = usePlayerSurface({
  queryProvider,
  resumeStartEpisode,
  resumeLoadedEpisodes: watchFlow.resumeLoadedEpisodes,
})

// Theater is an aePlayer-only feature — AePlayer's :can-theater is hardcoded
// true only while AePlayer is actually the mounted surface. That mount
// condition is `!classicKodik && aePlayerEnabled && !notReleasedYet`
// (template above: the outer `v-if` plus the premiere-notice `v-else`
// inside it) — three independent ways for AePlayer (and therefore the only
// visible theater-exit control) to be absent, not just `classicKodik`:
//   1. classicKodik === true — user flipped to (or persisted into) the
//      Classic Kodik iframe fallback.
//   2. aePlayerEnabled === false (VITE_AE_PLAYER_ENABLED=false) — Classic
//      Kodik is the ONLY surface regardless of classicKodik's value, and the
//      toggle button itself is also gated out (`v-if="aePlayerEnabled"`).
//   3. notReleasedYet === true — an announced/upcoming title with no
//      resolved sources; AePlayer is replaced by the premiere notice.
// Any one of these leaves theaterMode CSS applied to a surface with no
// visible way out (no heading, no toggle, no Esc on touch). aePlayerMounted
// mirrors the template's real mount condition so the guard actually covers
// every hole, not just the classicKodik one.
const aePlayerMounted = computed(() =>
  !classicKodik.value && aePlayerEnabled && !notReleasedYet.value
)
// `immediate: true` (replacing the old two-part watch-plus-setup-time-`if`)
// is required for case 3: notReleasedYet only resolves after the async
// anime fetch completes, so a setup-time `if` runs too early to see it — it
// would only ever observe the pre-fetch `anime.value === null` state. A
// reactive watch instead re-fires once `anime` loads and notReleasedYet
// flips, while `immediate: true` still covers cases 1 and 2, which ARE
// known synchronously at setup (classicKodik/aePlayerEnabled from
// localStorage/env, no fetch needed).
watch(aePlayerMounted, (mounted) => {
  if (!mounted && theaterMode.value) setTheater(false)
}, { immediate: true })

// Related rail + characters rail.
const related = useRelatedAnime(anime)
const { relatedAnime, relatedCardModel } = related
const { characters, fetchCharacters } = useCharacters()
const charactersFetched = ref(false)

// Lazy below-the-fold sentinels (template refs) + one IntersectionObserver.
const ugcSectionEl = ref<HTMLElement | null>(null)
const relatedSentinelEl = ref<HTMLElement | null>(null)
const characterSentinelEl = ref<HTMLElement | null>(null)
const { armLazySections, disarmLazySections } = useLazyAnimeSections([
  {
    el: ugcSectionEl,
    trigger: () => {
      if (!reviewsFetched.value) void fetchReviewsList()
    },
  },
  {
    el: relatedSentinelEl,
    trigger: () => {
      if (!related.relatedFetched.value) {
        related.relatedFetched.value = true
        void related.fetchRelatedAnime()
      }
    },
  },
  {
    el: characterSentinelEl,
    trigger: () => {
      if (!charactersFetched.value) {
        charactersFetched.value = true
        void fetchCharacters(String(anime.value?.id))
      }
    },
  },
])

// Comments + UGC tab (URL-persisted via ?ugc=).
const commentsUgc = useAnimeComments(anime)
const {
  ugcTab, comments, commentsHasMore, commentsLoading, commentsLoadingMore,
  commentsError, commentsFetched, newCommentBody, posting, postError,
  editingCommentId, editingBody, editError, editSaving, deleteError,
  fetchComments, loadMoreComments, postComment, startEditComment,
  cancelEditComment, saveEditComment, deleteCommentItem,
} = commentsUgc

// Synchronous per-concern reset so stale data doesn't flash on a route-param
// change — same writes, in the same order, as the historical inline block.
function resetForAnime() {
  watchFlow.reset()
  listStatus.reset()
  social.reset()
  related.reset()
  characters.value = []
  charactersFetched.value = false
  disarmLazySections()
  commentsUgc.reset()
  synopsisExpanded.value = false
}

// Data-loading orchestration: mount + route-param change + retry, with
// generation-based cancellation of in-flight loads.
const { retry } = useAnimeDataLoader({
  anime,
  fetchAnime,
  resetForAnime,
  watchState: watchFlow.watchState,
  applyViewerContext: social.applyViewerContext,
  initPreferences,
  fetchWatchlistStatus: listStatus.fetchWatchlistStatus,
  fetchReviews: social.fetchReviews,
  fetchWatchersCount: social.fetchWatchersCount,
  fetchHiddenStatus,
  ugcTab,
  commentsFetched,
  fetchComments,
  armLazySections,
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
</script>

<style scoped>
/* Player tab buttons have their own active visual state (bg + border).
   The global :focus-visible box-shadow ring is distracting during fullscreen. */
.player-tabs button:focus-visible {
  box-shadow: none;
}

:deep(.shiki-link) {
  color: var(--brand-cyan);
  text-decoration: none;
  border-bottom: 1px dotted var(--cyan-a40);
}
:deep(.shiki-link:hover) {
  text-decoration: underline;
}
:deep(.shiki-footnote) {
  font-size: 0.75rem;
  color: var(--ink-4);
}

/* Full-bleed player on phones: kill the card gutter and the page padding
   (px-4 on the page container) so the video spans the full viewport width.
   Scoped selector (.player-card[data-v]) outranks the unlayered .glass-card. */
@media (max-width: 680px) {
  .player-card {
    margin-inline: -1rem;
    padding: 0;
    border-radius: 0;
    border-left: 0;
    border-right: 0;
    background: transparent;
    backdrop-filter: none;
  }

  .player-card > .aspect-video {
    border-radius: 0;
  }
}

/* iPhone pseudo-fullscreen (AePlayer.vue enterPseudoFs) takes over via
   position:fixed on the player root. .glass-card's backdrop-filter makes
   THIS element the containing block for that fixed descendant instead of
   the viewport (backdrop-filter/filter/transform/perspective all do this
   per spec), so the "fullscreen" video only fills this card's box while
   the site header, page gutters, and the Reviews section below it show
   through. The >680px width (i.e. most iPhones in LANDSCAPE, the
   orientation people actually rotate to for fullscreen) falls outside the
   media-query override above, so it needs its own rule keyed off
   html.pl-noscroll — the class enterPseudoFs()/exitPseudoFs() toggle in
   lockstep with pseudo-fs state, independent of viewport width. */
html.pl-noscroll .player-card {
  backdrop-filter: none;
}

/* Second half of the same takeover bug (report 2026-07-15T12-52-24): the
   content wrapper's `relative z-10` (needed to sit above the hero banner)
   makes it a stacking context, so the takeover's z-index:100 only competes
   INSIDE a z-10 box — every root-context fixed element above z-10 (navbar
   z-50, toaster z-50, mobile tab bar z-40) still paints over the
   "fullscreen" video and eats its taps. While the takeover is active, lift
   the whole wrapper to the takeover's own level; the player covers the
   viewport, so nothing inside or below it shows anyway. */
html.pl-noscroll .anime-main {
  z-index: 100;
}
</style>

<!-- Phase 11 / UX-23 — Theater Mode global rules.
     Unscoped because the selectors target body.theater-mode and the
     global .navbar-root class from <Navbar />, which a scoped block
     could not reach. Anime.vue is the only mount site for theater mode
     so co-locating the CSS here keeps it discoverable. -->
<style>
/* Theater = full-bleed player, page INTACT. The navbar and .non-player-content
   used to be display:none here, which made this a fullscreen clone with no
   reason to exist; both rules are deliberately gone. */
/* Gated to the SAME 1024px breakpoint as the theater button itself
   (PlayerControlBar.vue: `@media (max-width: 1023px) { .pl-theater-btn {
   display: none } }`). Below that width there is no trigger to turn theater
   off — not canTheater (the prop stays true), but the button's own media
   query — yet `theaterMode` persists in localStorage across viewports and
   sessions (including from before the button existed, June 2026). Without
   this gate, a stale/cross-device `theaterMode=1` flips `body.theater-mode`
   on mount regardless of screen size: the heading and Classic Kodik toggle
   vanish and the section goes full-bleed on a phone or a rotated tablet,
   with no visible control and no Esc key on touch. Scoping the whole rule
   block to the button's breakpoint makes persisted state inert everywhere
   the trigger does not exist. */
@media (min-width: 1024px) {
  body.theater-mode [data-anime-player-wrapper="true"] {
    /* The width constraint lives on the ANCESTOR container
       (`.max-w-7xl mx-auto px-4 lg:px-8`, wrapping the whole page body —
       see the template) — this section itself carries only `mt-8` and has
       no width/margin/padding of its own to reset. Negative side margins
       escape the ancestor's max-width instead (the standard full-bleed
       technique): 100vw is the true viewport width regardless of the
       ancestor's max-w-7xl cap, and `calc(50% - 50vw)` walks the box back
       out to the viewport edges symmetrically. On desktop with a visible
       scrollbar this overhangs the content area by half a scrollbar width
       on each side; `#app { overflow-x: clip }` (styles/main.css) exists
       precisely to absorb exactly this kind of edge overhang without
       introducing a horizontal scrollbar. */
    width: 100vw;
    margin-left: calc(50% - 50vw);
    margin-right: calc(50% - 50vw);
    max-width: none;
    /* mt-8 would push the player below the navbar line the cap is framed for. */
    margin-top: 0 !important;
    /* Offset for scrollIntoView in onToggleTheater — same token as the cap. */
    scroll-margin-top: var(--header-offset);
  }

  /* The section's own title row + Classic-Kodik toggle step aside so the section
     top IS the player top; otherwise they eat into the capped height and push the
     control bar back under the fold. Leaving theater brings them straight back. */
  body.theater-mode .player-head {
    display: none;
  }

  /* The glass card's padding and side border would frame a full-bleed player. */
  body.theater-mode [data-anime-player-wrapper="true"] .player-card {
    padding: 0;
    border-left: 0;
    border-right: 0;
    border-radius: 0;
  }
}
</style>
