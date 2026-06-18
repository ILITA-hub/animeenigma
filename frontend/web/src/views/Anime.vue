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
          <PosterImage
            :src="anime.coverImage"
            :alt="anime.title"
            ratio="2/3"
            rounded="xl"
            :proxy-width="448"
            class="w-40 md:w-56 shadow-2xl ring-1 ring-white/10"
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

            <!-- Rewatch tally — muted ↻ N beside the status badge; editable
                 stepper, hidden entirely until the entry exists (design 2026-06-05). -->
            <RewatchCounter
              v-if="authStore.isAuthenticated && currentListStatus"
              :count="currentRewatchCount"
              editable
              @update:count="setRewatchCount"
            />

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
      <section v-if="anime.description" id="section-overview" class="mt-8 non-player-content">
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
        <div class="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3 mb-4">
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
          <!-- Language tabs + Provider sub-tabs -->
          <!-- UA-062 (UX-12 Phase 5): ButtonGroup wraps the RU/EN/18+ toggle
               with role="group" + aria-label; each child button binds
               aria-pressed to its selected state. -->
          <!-- Hidden for not-yet-released titles: no sources exist to switch
               between, so the language/provider toggles are noise. -->
          <div v-if="!notReleasedYet" class="flex flex-wrap gap-2 player-tabs">
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
              <!-- Task 15 — Unified Player pill (default ON, VITE_AE_PLAYER_ENABLED=false to hide) -->
              <button
                v-if="aePlayerEnabled"
                @click="aeSelected = !aeSelected"
                :aria-pressed="aeSelected"
                class="px-3 py-1.5 rounded-md text-sm font-medium transition-all inline-flex items-center gap-1.5"
                :class="aeSelected ? 'bg-white/15 text-white' : 'text-white/50 hover:text-white/70'"
              >
                {{ $t('player.aePlayer.tab') }}
                <span class="text-[10px] px-1 rounded bg-cyan-500/20 text-cyan-400">{{ $t('player.aePlayer.beta') }}</span>
              </button>
            </ButtonGroup>

            <!-- Provider sub-tabs — hidden when unified player is active (it has its own source picker) -->
            <!-- UA-063 (UX-12 Phase 5): ButtonGroup wraps provider chips. -->
            <ButtonGroup
              v-if="videoLanguage === 'ru' && !aeSelected"
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
              <button
                v-if="kodikAdfreeEnabled"
                @click="onUserPickedProvider('kodik-adfree')"
                :aria-pressed="videoProvider === 'kodik-adfree'"
                class="px-4 py-2 rounded-lg text-sm font-medium transition-all"
                :class="videoProvider === 'kodik-adfree'
                  ? 'bg-cyan-500/20 text-cyan-400 border border-cyan-500/50'
                  : 'bg-white/5 text-white/60 border border-transparent hover:bg-white/10'"
              >
                {{ $t('player.kodikAdfree.tab') }}
              </button>
            </ButtonGroup>
            <template v-else-if="videoLanguage === 'en' && ourEnglishEnabled && !aeSelected">
              <button
                @click="videoProvider = 'ourenglish'"
                :aria-pressed="videoProvider === 'ourenglish'"
                class="px-4 py-2 rounded-lg text-sm font-medium transition-all"
                :class="videoProvider === 'ourenglish'
                  ? 'bg-success-soft text-success border border-success/50'
                  : 'bg-white/5 text-white/60 border border-transparent hover:bg-white/10'"
              >
                {{ $t('player.ourenglish.label') }}
              </button>
            </template>
            <template v-else-if="videoLanguage === '18+' && !aeSelected">
              <button
                @click="videoProvider = 'hanime'"
                class="px-4 py-2 rounded-lg text-sm font-medium transition-all"
                :class="videoProvider === 'hanime'
                  ? 'bg-pink-500/20 text-pink-400 border border-pink-500/50'
                  : 'bg-white/5 text-white/60 border border-transparent hover:bg-white/10'"
              >
                Hanime
              </button>
              <button
                v-if="anime18Enabled"
                @click="videoProvider = 'anime18'"
                class="px-4 py-2 rounded-lg text-sm font-medium transition-all"
                :class="videoProvider === 'anime18'
                  ? 'bg-rose-500/20 text-rose-300 border border-rose-500/50'
                  : 'bg-white/5 text-white/60 border border-transparent hover:bg-white/10'"
              >
                {{ $t('player.anime18.label') }}
              </button>
            </template>
            <!-- Workstream raw-jp / Phase 04 — single-chip group for v0.1.
                 v0.2's hybrid resolver adds 'minio' here. -->
            <template v-else-if="videoLanguage === 'raw' && rawProviderEnabled && !aeSelected">
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
        <!-- Task 15: existing player chain unmounted when unified player is selected (prevents hidden audio) -->
        <div class="glass-card p-4 md:p-6" v-if="!aeSelected">
          <!-- Not-released notice: an announced/upcoming title with no sources
               yet. Replaces the player so users see "premieres on {date}"
               rather than a misleading "no available videos" error. -->
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
          <!-- Click-to-load placeholder (saves bandwidth / no auto-buffer) -->
          <button
            v-else-if="!playerActivated"
            type="button"
            @click="onPlaceholderCtaClick"
            class="relative w-full aspect-video rounded-lg overflow-hidden group focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-cyan-500/50"
            :aria-label="placeholderCtaLabel"
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
                <Play class="size-8 sm:size-10 ml-1" fill="currentColor" aria-hidden="true" />
              </span>
              <span class="text-base sm:text-lg font-semibold">
                {{ placeholderCtaLabel }}
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
              :episode-duration-min="anime.episodeDuration"
              :preferred-combo="resolvedCombo"
              :initial-episode="resumeStartEpisode"
              @available-translations="handleAvailableTranslations"
            >
              <template #header-middle>
                <ResumePill v-bind="resumePillProps" />
              </template>
            </KodikPlayer>
            <!-- Ad-free Kodik Player (HLS via kodikextract) -->
            <KodikAdFreePlayer
              v-else-if="videoProvider === 'kodik-adfree' && kodikAdfreeEnabled"
              :anime-id="anime.id"
              :anime-name="anime.title"
              :total-episodes="anime.totalEpisodes"
              :episode-duration-min="anime.episodeDuration"
              :preferred-combo="resolvedCombo"
              :initial-episode="resumeStartEpisode"
              @available-translations="handleAvailableTranslations"
            >
              <template #header-middle>
                <ResumePill v-bind="resumePillProps" />
              </template>
            </KodikAdFreePlayer>
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
                <ResumePill v-bind="resumePillProps" />
              </template>
            </AnimeLibPlayer>
            <!-- OurEnglish Player (Phase 24-28 scraper microservice) -->
            <OurEnglishPlayer
              v-else-if="videoProvider === 'ourenglish' && ourEnglishEnabled"
              :anime-id="anime.id"
              :initial-episode="resumeStartEpisode"
            >
              <template #header-middle>
                <ResumePill v-bind="resumePillProps" />
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
                <ResumePill v-bind="resumePillProps" />
              </template>
            </HanimePlayer>
            <!-- 18anime (18+) Player — second 18+ provider, behind VITE_ANIME18_ENABLED. -->
            <Anime18Player
              v-else-if="videoProvider === 'anime18' && anime18Enabled"
              :anime-id="anime.id"
              :anime-name="anime.title"
              :total-episodes="anime.totalEpisodes"
              :initial-episode="resumeStartEpisode"
            >
              <template #header-middle>
                <ResumePill v-bind="resumePillProps" />
              </template>
            </Anime18Player>
            <!-- Workstream raw-jp / Phase 04 — RawPlayer mounts behind the
                 same VITE_RAW_PROVIDER_ENABLED flag that gates the chip. -->
            <RawPlayer
              v-else-if="videoProvider === 'raw' && rawProviderEnabled"
              :anime-id="anime.id"
            >
              <template #header-middle>
                <ResumePill v-bind="resumePillProps" />
              </template>
            </RawPlayer>
          </template>
        </div>
        <!-- Task 15: Unified player mounts as a sibling AFTER the existing chain;
             glass-card above is v-show="!aeSelected" so only one renders. -->
        <AePlayer
          v-if="aeSelected && aePlayerEnabled"
          :anime-id="anime.id"
          :anime="{ title: anime.title, ep: (anime.episodesAired || 1), eps: (anime.totalEpisodes || anime.episodesAired || 1), still: anime.coverImage }"
          :theater="theaterMode"
          :is-hentai="isHentai"
          :initial-episode="resumeStartEpisode"
          :initial-provider="queryProvider"
          :initial-team="queryTeam"
          :mal-id="anime.shikimoriId"
          @toggle-theater="setTheater(!theaterMode)"
          @combo-change="aeWtSeed = $event"
        />
      </section>

      <!-- Reviews + Comments Section (SOCIAL-06: two-tab UGC strip) -->
      <!-- Phase 11 / UX-22 — section-comments anchor for AnimeQuickNav. -->
      <section id="section-comments" ref="ugcSectionEl" class="mt-8 non-player-content">
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
        class="mt-8 non-player-content"
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
        class="mt-8 non-player-content"
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
import { ref, reactive, computed, watch, onMounted, onBeforeUnmount, onUnmounted, defineAsyncComponent, nextTick } from 'vue'
import { Star, Clock, Play, Check, Plus, ChevronDown, Trash2, RefreshCw, Eye, EyeOff, Pencil, Calendar, MessageSquare, EllipsisVertical } from 'lucide-vue-next'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useAnime } from '@/composables/useAnime'
import { useAuthStore } from '@/stores/auth'
import { Avatar, Badge, Button, ButtonGroup, DropdownMenu, DropdownMenuItem, Input, ScoreDiamond, Spinner } from '@/components/ui'
import { GenreChip, PosterCard, PosterImage, AnimeContextMenu } from '@/components/anime'
import ReviewReactions from '@/components/anime/ReviewReactions.vue'
import CharacterCard from '@/components/anime/CharacterCard.vue'
import Carousel from '@/components/carousel/Carousel.vue'
import { useWatchPreferences } from '@/composables/useWatchPreferences'
import { useOverrideTracker } from '@/composables/useOverrideTracker'
import { useResumeStateMachine } from '@/composables/useResumeStateMachine'
import { computeWatchCta, type WatchCta } from '@/composables/watchCta'
import RewatchCounter from '@/components/anime/RewatchCounter.vue'
import { useContextMenu } from '@/composables/useContextMenu'
import { useSiteRatings } from '@/composables/useSiteRatings'
import { useUserTimezone } from '@/composables/useUserTimezone'
import { wallClockDate, formatUtcOffset } from '@/composables/schedule/timezone'
import { fromCatalogAnime } from '@/utils/toCardModel'
import { useCharacters } from '@/composables/useCharacters'
import type { AnimeCardModel } from '@/types/card'
import type { CharacterCardModel } from '@/types/character'
import type { WatchCombo } from '@/types/preference'

const KodikPlayer = defineAsyncComponent(() => import('@/components/player/KodikPlayer.vue'))
// Ad-free Kodik player (libs/kodikextract HLS extraction). Behind a flag so it
// can dark-ship; defaults ON.
const KodikAdFreePlayer = defineAsyncComponent(() => import('@/components/player/KodikAdFreePlayer.vue'))
const kodikAdfreeEnabled = import.meta.env.VITE_KODIK_ADFREE_ENABLED !== 'false'
const AnimeLibPlayer = defineAsyncComponent(() => import('@/components/player/AnimeLibPlayer.vue'))
const HanimePlayer = defineAsyncComponent(() => import('@/components/player/HanimePlayer.vue'))
// 18anime (18+) — second 18+ provider alongside Hanime. Behind
// VITE_ANIME18_ENABLED (default OFF) so it can dark-ship until verified.
const Anime18Player = defineAsyncComponent(() => import('@/components/player/Anime18Player.vue'))
const anime18Enabled = import.meta.env.VITE_ANIME18_ENABLED === 'true'
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
import type { PlayerKind } from '@/types/watch-together'
import type { WtCreateSeed } from '@/composables/aePlayer/wtCreateSeed'
import ResumePill from '@/components/player/ResumePill.vue'
import AePlayerSkeleton from '@/components/player/aePlayer/AePlayerSkeleton.vue'
import { animeApi, userApi, reviewApi, adminApi, commentApi } from '@/api/client'
import Tabs from '@/components/ui/Tabs.vue'
import { useWatchlistStore } from '@/stores/watchlist'
import { useViewerContextStore, type ViewerContextData } from '@/stores/viewerContext'
import { useToast } from '@/composables/useToast'
import { useConfirm } from '@/composables/useConfirm'
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
  // Author's CURRENT avatar (read-time join in the player service, not
  // snapshotted). Absent → Avatar primitive falls back to initials.
  user_avatar?: string
  score: number
  review_text: string
  created_at: string
  // Steam-style review context (2026-05-21). Live values from anime_list
  // row — NOT snapshotted at review time. `anime` carries episodes_count
  // for the "watched / total" rendering; backend preloads it.
  status?: string
  episodes?: number
  // True when the reviewer is on a rewatch — renders a "🔁 On rewatch"
  // segment after the watch stats (repo-todo 19:00:01).
  is_rewatching?: boolean
  anime?: {
    episodes_count?: number
  }
  // Emoji reactions (AUTO-408). `reactions` carries per-emoji counts +
  // reacted_by_me; `my_reactions` is the viewer's reacted-emoji subset.
  reactions?: { emoji: string; count: number; reacted_by_me: boolean; users?: string[] }[]
  my_reactions?: string[]
}

interface Comment {
  id: string
  user_id: string
  anime_id: string
  username: string
  // Author's CURRENT avatar — same read-time join as reviews/activity feed.
  user_avatar?: string
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
const viewerCtxStore = useViewerContextStore()
const toast = useToast()
const { confirm } = useConfirm()
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

// --- Watched-aware play button + tracked rewatch (design 2026-06-05) -------
// The verb comes from actual episode progress; list status only disambiguates
// the fully-watched terminal: not-completed → mark-watched, completed → rewatch.
const currentRewatchCount = ref(0)
const rewatchPending = ref(false)

const watchCta = computed<WatchCta>(() => computeWatchCta({
  isAuthenticated: authStore.isAuthenticated,
  lastWatched: resumeAuth.value && resume.loaded.value
    ? resume.lastWatched.value
    : (lastEpisode.value ?? 0),
  totalEpisodes: anime.value?.totalEpisodes ?? 0,
  listStatus: currentListStatus.value,
}))

const watchCtaLabel = computed(() => t(watchCta.value.labelKey, watchCta.value.labelParams ?? {}))

// The click-to-load player placeholder is a playback surface — clicking it must
// never mutate the list, so the mark-watched terminal degrades to plain watch.
const placeholderCta = computed<WatchCta>(() => {
  const cta = watchCta.value
  return cta.action === 'mark-watched'
    ? { action: 'watch', startEpisode: 1, labelKey: 'anime.watchNow' }
    : cta
})
const placeholderCtaLabel = computed(() => t(placeholderCta.value.labelKey, placeholderCta.value.labelParams ?? {}))

// startRewatchFlow — server-side cycle reset (status→watching, episodes=0,
// watch_progress reset, is_rewatching=true; the count bumps on the new finale),
// then re-init the resume machine and mount the player at ep. 1.
async function startRewatchFlow() {
  if (!anime.value || rewatchPending.value) return
  rewatchPending.value = true
  const animeId = anime.value.id
  try {
    await userApi.startRewatch(animeId)
    currentListStatus.value = 'watching'
    resumeOverrideEpisode.value = 1
    await resume.init()
    void viewerCtxStore.load(animeId, true).then((ctx) => { if (ctx) applyViewerContext(ctx) })
    void activatePlayer()
  } catch (err) {
    console.error('Failed to start rewatch:', err)
    toast.push(t('watchlist.errors.updateFailed'))
  } finally {
    rewatchPending.value = false
  }
}

async function dispatchWatchCta(cta: WatchCta) {
  if (cta.action === 'mark-watched') {
    await setListStatus('completed')
    return
  }
  if (cta.action === 'rewatch') {
    await startRewatchFlow()
    return
  }
  if (cta.action === 'start-from-1') {
    resumeOverrideEpisode.value = cta.startEpisode
  }
  void activatePlayer()
}

const onWatchCtaClick = () => dispatchWatchCta(watchCta.value)
const onPlaceholderCtaClick = () => dispatchWatchCta(placeholderCta.value)

// Manual rewatch-count stepper (RewatchCounter beside the status badge).
// Optimistic; the PUT carries the current status so the entry isn't moved.
async function setRewatchCount(n: number) {
  if (!anime.value || !currentListStatus.value) return
  const prior = currentRewatchCount.value
  currentRewatchCount.value = n
  try {
    await userApi.updateWatchlistEntry({
      anime_id: anime.value.id,
      status: currentListStatus.value,
      rewatch_count: n,
    })
  } catch (err) {
    console.error('Failed to update rewatch count:', err)
    currentRewatchCount.value = prior
    toast.push(t('watchlist.errors.updateFailed'))
  }
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
// Admin kebab (Refresh / Hide / Shikimori ID) — admin-only, grouped out of the
// user action row. Controlled open state for the DropdownMenu #trigger.
const showAdminMenu = ref(false)
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
// Task 15 — dedicated flag so selecting the unified player never overwrites
// videoLanguage (which controls all existing-player routing).
// Persisted so a stale-chunk recovery reload (after a deploy) — or any reload —
// restores the user's AnimeEnigma selection instead of silently dropping back
// to the default player.
const aeSelected = ref(localStorage.getItem('unified_player_selected') === '1')
watch(aeSelected, (on) => {
  if (on) localStorage.setItem('unified_player_selected', '1')
  else localStorage.removeItem('unified_player_selected')
})
// Workstream raw-jp, Phase 04 — 'raw' is the AllAnime-backed raw-JP provider.
// Phase 24-28 — 'ourenglish' is the scraper-microservice-backed EN provider
// (failover across gogoanime/animepahe/allanime/animefever/miruro/nineanime).
const VALID_PROVIDERS = ['kodik', 'kodik-adfree', 'animelib', 'ourenglish', 'hanime', 'anime18', 'raw'] as const
type VideoProvider = (typeof VALID_PROVIDERS)[number]
const _savedProv = localStorage.getItem('preferred_video_provider')
// Coerce a pinned-but-disabled 'animelib' back to 'kodik' so users who last
// watched on AniLib don't boot into a hidden tab with no player mounted.
const _initialProv =
  (VALID_PROVIDERS as readonly string[]).includes(_savedProv ?? '') ? (_savedProv as VideoProvider) : 'kodik'
const videoProvider = ref<VideoProvider>(
  (_initialProv === 'animelib' && !animeLibEnabled) || (_initialProv === 'kodik-adfree' && !kodikAdfreeEnabled) ? 'kodik' : _initialProv
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
// Highest episode actually loaded into our sources, surfaced by the active
// player via `available-translations` (max episodes_count across teams). This
// is the ground truth for availability and overrides Shikimori's lagging
// `episodesAired` in the resume state machine — so a freshly-uploaded episode
// is never mislabeled "not loaded yet". 0 until the player emits.
const resumeLoadedEpisodes = ref(0)
const resume = useResumeStateMachine({
  animeId: resumeAnimeId,
  totalEpisodes: resumeTotal,
  episodesAired: resumeAired,
  nextEpisodeAt: resumeNextAt,
  status: resumeStatus,
  isAuthenticated: resumeAuth,
  loadedEpisodes: resumeLoadedEpisodes,
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

// Notification deep-link — `?provider=` is an aePlayer source id, `?team=` is a
// team TITLE. Both are HINTS preselected on aePlayer (see AePlayer initialProvider).
const queryProvider = computed<string | undefined>(() => {
  const v = route.query.provider
  const s = Array.isArray(v) ? v[0] : v
  return typeof s === 'string' && s !== '' ? s : undefined
})
const queryTeam = computed<string | undefined>(() => {
  const v = route.query.team
  const s = Array.isArray(v) ? v[0] : v
  return typeof s === 'string' && s !== '' ? s : undefined
})

// A `?provider=` deep-link always opens aePlayer (the param speaks aePlayer's
// source vocabulary). Set the ref directly; its localStorage watcher persists
// the switch, which matches the retire-all-but-aePlayer direction.
if (queryProvider.value) aeSelected.value = true

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
  if (resume.kind.value === 'not-yet-aired' || resume.kind.value === 'episode-not-loaded-yet') {
    // Use episodesAired + 1 — the episode the announced air time refers to, and
    // the same formula the top-of-page airing banner uses — so both surfaces
    // name the same episode instead of diverging on `lastWatched + 1`.
    return Math.max(1, resumeAired.value + 1)
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
  // Only surface an ETA when the air time is genuinely in the future. A past
  // nextEpisodeAt would otherwise format into a date in the PAST (a past air
  // time means the episode aired and the state is episode-not-loaded-yet, not
  // not-yet-aired); suppress it so we never render a nonsensical past ETA.
  const hasFutureEta =
    !!resumeNextAt.value && new Date(resumeNextAt.value).getTime() > Date.now()
  const etaLabel =
    resume.kind.value === 'not-yet-aired' && hasFutureEta
      ? formatNextEpisode(resumeNextAt.value as string)
      : undefined
  // For episode-not-loaded-yet: localized "aired N ago" (reads episodeAiredAgoMs
  // so it stays reactive to the 60s clock tick).
  const airedAgoLabel =
    resume.kind.value === 'episode-not-loaded-yet'
      ? formatAiredAgo(resume.episodeAiredAgoMs.value)
      : undefined
  return {
    kind: resume.kind.value,
    finishedEpisode: resume.finishedEpisode.value,
    nextEpisodeNumber: resumeNextEpisodeNumber.value,
    nextEpisodeEtaLabel: etaLabel,
    airedAgoLabel,
  }
})

// Watch preference resolution
const preferenceState = ref<{
  resolvedCombo: WatchCombo | null
  resolve: (available: WatchCombo[]) => Promise<void>
} | null>(null)

const resolvedCombo = computed(() => preferenceState.value?.resolvedCombo ?? null)

// Watch-Together create seed surfaced by AePlayer (live combo + episode). When
// the aePlayer surface is the active one at room-create time, this drives the
// Invite button to create the room AS aeplayer seeded with that exact combo.
// Null until AePlayer has a resolved source (or when aePlayer isn't selected).
const aeWtSeed = ref<WtCreateSeed | null>(null)

// The Invite button's create-room payload. Prefers the aePlayer seed when the
// aePlayer surface is active AND a usable source is resolved; otherwise falls
// back to the legacy coarse-combo default (kodik / resolved player).
const wtInvitePayload = computed<{ player: PlayerKind; translationId: string; episodeId: string }>(() => {
  if (aeSelected.value && aeWtSeed.value) {
    return {
      player: aeWtSeed.value.player,
      translationId: aeWtSeed.value.translation_id,
      episodeId: aeWtSeed.value.episode_id,
    }
  }
  return {
    player: (resolvedCombo.value?.player === 'english'
      ? 'ourenglish'
      : (resolvedCombo.value?.player ?? 'kodik')) as PlayerKind,
    translationId: resolvedCombo.value?.translation_id ?? '',
    episodeId: String(resumeStartEpisode.value ?? lastEpisode.value ?? 1),
  }
})

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
  player: videoProvider.value === 'hanime' || videoProvider.value === 'raw' || videoProvider.value === 'anime18' || videoProvider.value === 'kodik-adfree'
    ? 'kodik'
    : videoProvider.value === 'ourenglish'
      ? 'english'
      : videoProvider.value,
  resolvedCombo,
  currentEpisode: currentEpisodeForTracker,
})

function onUserPickedProvider(newProvider: 'kodik' | 'kodik-adfree' | 'animelib' | 'raw') {
  // Only fire override if the user is genuinely SWITCHING. The composable's
  // first-per-(load_session_id, dimension) lock would also catch repeats, but
  // an explicit guard keeps E2E timing predictable.
  if (newProvider !== videoProvider.value) {
    // Workstream raw-jp / Phase 04 — 'raw' is outside the tracked
    // PlayerName set; map to 'kodik' for the picker event like hanime.
    // Ad-free Kodik is also outside the tracked PlayerName set; map to 'kodik'
    // for the bucket label so the analytics type compiles.
    const trackedProvider = (newProvider === 'raw' || newProvider === 'kodik-adfree') ? 'kodik' : newProvider
    playerSwitchTracker.recordPickerEvent('player', { player: trackedProvider })
  }
  videoProvider.value = newProvider
}

const initPreferences = (animeId: string, tier1Combo?: ViewerContextData['combo']) => {
  const pref = useWatchPreferences(animeId, { tier1Combo })
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
    // 'kodik-adfree' is our ad-free player surface for the SAME RU Kodik source
    // as 'kodik'. The watch-preference resolver only knows 'kodik', so when a
    // user has explicitly picked the ad-free variant, don't let auto-resolve
    // clobber them back to the iframe Kodik (the bug: pick ad-free → its
    // available-translations emit triggers resolve → snaps back to 'kodik').
    if (combo.player === 'kodik' && videoProvider.value === 'kodik-adfree') {
      return
    }
    // AniLib hidden (see animeLibEnabled): never auto-resolve onto the disabled
    // provider — fall back to Kodik, which carries the same RU translations.
    videoProvider.value = combo.player === 'animelib' && !animeLibEnabled ? 'kodik' : combo.player
  }
}

const handleAvailableTranslations = (combos: WatchCombo[]) => {
  if (preferenceState.value) {
    preferenceState.value.resolve(combos)
  }
  // Feed the real provider episode count into the resume state machine so a
  // freshly-aired-but-already-uploaded episode is classified 'watching', not a
  // false "not loaded yet" (Shikimori episodesAired lags reality by hours).
  // Take the max across teams — if ANY team has ep N, it's watchable.
  const maxLoaded = combos.reduce((m, c) => Math.max(m, c.episodes_count ?? 0), 0)
  if (maxLoaded > resumeLoadedEpisodes.value) resumeLoadedEpisodes.value = maxLoaded
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

  const base = t(key, { watched: episodes, total, status: statusLabel })
  // Append the rewatch segment when the reviewer is rewatching.
  return review.is_rewatching ? `${base} · ${t('anime.reviewStats.rewatch')}` : base
}

const isReviewFlagged = (review: Review): boolean => {
  const status = review.status || 'watching'
  const episodes = review.episodes ?? 0
  return status === 'plan_to_watch' || episodes === 0
}

const { timezone: userTimezone } = useUserTimezone()

const formatNextEpisode = (dateStr: string) => {
  const date = new Date(dateStr)
  const now = new Date()
  const diffMs = date.getTime() - now.getTime()
  const diffDays = Math.floor(diffMs / (1000 * 60 * 60 * 24))
  const tz = userTimezone.value

  const timeStr = date.toLocaleTimeString('ru-RU', {
    hour: '2-digit',
    minute: '2-digit',
    timeZone: tz
  })

  if (diffDays === 0) return t('anime.todayAt', { time: timeStr })
  if (diffDays === 1) return t('anime.tomorrowAt', { time: timeStr })
  if (diffDays > 1 && diffDays < 7) {
    const dayKeys = ['sunday', 'monday', 'tuesday', 'wednesday', 'thursday', 'friday', 'saturday']
    return t('anime.dayAtTime', { day: t(`schedule.daysWhen.${dayKeys[wallClockDate(date, tz).getDay()]}`), time: timeStr })
  }
  return t('anime.timeAt', { time: date.toLocaleDateString(locale.value === 'ru' ? 'ru-RU' : locale.value === 'ja' ? 'ja-JP' : 'en-US', { day: 'numeric', month: 'long', hour: '2-digit', minute: '2-digit', timeZone: tz }), tz: formatUtcOffset(tz) })
}

// Localized "N ago" for an episode that already aired (episode-not-loaded-yet).
// Uses Intl.RelativeTimeFormat so pluralization + the "ago"/"назад"/"前" suffix
// are correct per-locale. Picks the coarsest sensible unit.
const formatAiredAgo = (agoMs: number) => {
  const loc = locale.value === 'ru' ? 'ru-RU' : locale.value === 'ja' ? 'ja-JP' : 'en-US'
  const rtf = new Intl.RelativeTimeFormat(loc, { numeric: 'always' })
  const sec = Math.max(0, Math.floor(agoMs / 1000))
  if (sec < 3600) return rtf.format(-Math.max(1, Math.round(sec / 60)), 'minute')
  if (sec < 86400) return rtf.format(-Math.round(sec / 3600), 'hour')
  return rtf.format(-Math.round(sec / 86400), 'day')
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
      currentRewatchCount.value = (entry as { rewatch_count?: number }).rewatch_count ?? 0
    } else {
      currentListStatus.value = null
      currentRewatchCount.value = 0
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

// applyViewerContext — populate the page's social/user state from the
// aggregate viewer-context payload (page-fetch optimization 2026-06-11).
// Replaces the separate rating / watchers-count / my-review / watchlist-status
// fetches on page load and after review/list mutations.
const applyViewerContext = (ctx: ViewerContextData) => {
  watchersCount.value = typeof ctx.watchers_count === 'number' && ctx.watchers_count >= 0
    ? ctx.watchers_count
    : 0
  siteRating.value = (ctx.rating as AnimeRating | null) ?? null
  if (authStore.isAuthenticated) {
    const review = ctx.my_review as Review | null
    if (review && review.id) {
      myReview.value = review
      reviewForm.score = review.score
      reviewForm.text = review.review_text || ''
    } else {
      myReview.value = null
    }
    currentListStatus.value = ctx.watchlist_entry?.status ?? null
    currentRewatchCount.value = ctx.watchlist_entry?.rewatch_count ?? 0
  }
}

// fetchReviewsList — the reviews FEED only. Rating / my-review come from the
// viewer-context aggregate; this stays a separate (heavier) request, fetched
// lazily when the UGC section scrolls near the viewport.
const reviewsFetched = ref(false)
const fetchReviewsList = async () => {
  if (!anime.value) return
  reviewsFetched.value = true
  try {
    const reviewsResponse = await reviewApi.getAnimeReviews(anime.value.id)
    reviews.value = reviewsResponse.data?.data || reviewsResponse.data || []
  } catch (err) {
    console.error('Failed to fetch reviews:', err)
  }
}

// ── Lazy below-the-fold loaders (page-fetch optimization 2026-06-11) ─────────
// The reviews feed and the related rail render far below the player; fetching
// them on mount put two requests (plus a ~500ms Shikimori upstream call) on
// the critical path. One IntersectionObserver with a generous rootMargin
// fetches each once as the user approaches.
const ugcSectionEl = ref<HTMLElement | null>(null)
const relatedSentinelEl = ref<HTMLElement | null>(null)
const relatedFetched = ref(false)
const { characters, fetchCharacters } = useCharacters()
const characterSentinelEl = ref<HTMLElement | null>(null)
const charactersFetched = ref(false)
let lazySectionObserver: IntersectionObserver | null = null

function disarmLazySections() {
  lazySectionObserver?.disconnect()
  lazySectionObserver = null
}

function armLazySections() {
  disarmLazySections()
  if (typeof IntersectionObserver === 'undefined') {
    // jsdom / ancient browser — keep the eager behavior.
    if (!reviewsFetched.value) void fetchReviewsList()
    if (!relatedFetched.value) {
      relatedFetched.value = true
      void fetchRelatedAnime()
    }
    if (!charactersFetched.value) {
      charactersFetched.value = true
      void fetchCharacters(String(anime.value?.id))
    }
    return
  }
  lazySectionObserver = new IntersectionObserver(
    (entries) => {
      for (const entry of entries) {
        if (!entry.isIntersecting) continue
        if (entry.target === ugcSectionEl.value) {
          if (!reviewsFetched.value) void fetchReviewsList()
          lazySectionObserver?.unobserve(entry.target)
        } else if (entry.target === relatedSentinelEl.value) {
          if (!relatedFetched.value) {
            relatedFetched.value = true
            void fetchRelatedAnime()
          }
          lazySectionObserver?.unobserve(entry.target)
        } else if (entry.target === characterSentinelEl.value) {
          if (!charactersFetched.value) {
            charactersFetched.value = true
            void fetchCharacters(String(anime.value?.id))
          }
          lazySectionObserver?.unobserve(entry.target)
        }
      }
    },
    // Prefetch well before the section enters the viewport so the content is
    // there by the time the user arrives (and the QuickNav anchor exists).
    { rootMargin: '1200px 0px' },
  )
  if (ugcSectionEl.value) lazySectionObserver.observe(ugcSectionEl.value)
  if (relatedSentinelEl.value) lazySectionObserver.observe(relatedSentinelEl.value)
  if (characterSentinelEl.value) lazySectionObserver.observe(characterSentinelEl.value)
}

onBeforeUnmount(disarmLazySections)

// Legacy full path — used only when the viewer-context aggregate failed
// (older backend / transient error): falls back to the historical
// per-endpoint fetches.
const fetchReviews = async () => {
  if (!anime.value) return

  reviewsFetched.value = true
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

// refreshSocial — post-mutation refresh: one forced viewer-context reload
// (rating + my-review + watchlist status + watchers) plus the reviews feed.
const refreshSocial = async () => {
  if (!anime.value) return
  const [ctx] = await Promise.all([
    viewerCtxStore.load(anime.value.id, true),
    fetchReviewsList(),
  ])
  if (ctx) applyViewerContext(ctx)
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
    await refreshSocial()
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
    await refreshSocial()
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
  if (!(await confirm({
    title: t('common.confirmTitle'),
    description: t('anime.ugc.deleteCommentConfirm'),
    confirmText: t('common.delete'),
    cancelText: t('common.cancel'),
    variant: 'destructive',
  }))) return
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
  const priorCount = currentRewatchCount.value
  // Optimistic: clear the visible status + close the dropdown immediately.
  currentListStatus.value = null
  currentRewatchCount.value = 0
  showStatusDropdown.value = false
  try {
    await watchlistStore.removeEntryOptimistic(animeId)
  } catch (err) {
    console.error('Failed to remove from list:', err)
    currentListStatus.value = prior
    currentRewatchCount.value = priorCount
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
  // Deselect unified player whenever user picks an existing language tab.
  aeSelected.value = false
  videoLanguage.value = lang
  // Auto-select first provider in the group
  if (lang === 'ru') {
    const savedRu = localStorage.getItem('preferred_ru_provider') as 'kodik' | 'kodik-adfree' | 'animelib' | null
    videoProvider.value = savedRu && (savedRu !== 'animelib' || animeLibEnabled) && (savedRu !== 'kodik-adfree' || kodikAdfreeEnabled) ? savedRu : 'kodik'
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
  resumeLoadedEpisodes.value = 0
  resumeOverrideEpisode.value = null
  currentListStatus.value = null
  currentRewatchCount.value = 0
  reviews.value = []
  myReview.value = null
  siteRating.value = null
  watchersCount.value = 0
  relatedAnime.value = []
  characters.value = []
  reviewsFetched.value = false
  relatedFetched.value = false
  charactersFetched.value = false
  disarmLazySections()
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

  // Load last-watched episode from localStorage (anon path), then pull the
  // viewer-context aggregate — ONE request carrying rating, watchers-count,
  // watch progress, watchlist entry, my review and the saved combo (page-fetch
  // optimization 2026-06-11). The legacy per-endpoint fetches remain as the
  // fallback when the aggregate is unavailable.
  let viewerCtx: ViewerContextData | null = null
  if (fetched) {
    loadLastEpisode(fetched.id)
    viewerCtx = await viewerCtxStore.load(fetched.id, false, fetched.malId || undefined)
    if (gen !== loadGeneration) return
    if (viewerCtx) {
      applyViewerContext(viewerCtx)
      void resume.init(viewerCtx.progress ?? [])
      // Legacy MAL-import entry surfaced under anime_id="mal_{malId}" —
      // auto-migrate it to the real UUID (same behavior the statuses-scan
      // path had), fire-and-forget.
      const entryAnimeId = viewerCtx.watchlist_entry?.anime_id
      if (authStore.isAuthenticated && entryAnimeId && entryAnimeId.startsWith('mal_')) {
        userApi.migrateListEntry(entryAnimeId, fetched.id).catch((e) => {
          console.warn('Auto-migration of MAL entry failed:', e)
        })
      }
    } else {
      void resume.init()
    }
  }

  // Initialize watch preferences for this anime — anon users included so the
  // combo_resolve_total denominator increments alongside combo_override_total
  // (CONTEXT D-12: per-anon-user override rate). The composable + axios
  // interceptor handle the X-Anon-ID header for unauthenticated callers.
  // The viewer-context Tier-1 combo lets resolve() short-circuit client-side.
  if (fetched) {
    initPreferences(fetched.id, viewerCtx?.combo)
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

  // Viewer-context already delivered watchlist status / rating / my-review /
  // watchers-count; the legacy fetches below only run as fallback. In the
  // fallback the rating rides on fetchReviews, so it stays eager there; in
  // the normal path the reviews feed is lazy (IntersectionObserver below).
  if (!viewerCtx) {
    await fetchWatchlistStatus()
    if (gen !== loadGeneration) return
  }
  await fetchHiddenStatus()
  if (gen !== loadGeneration) return
  if (!viewerCtx) {
    await fetchReviews()
    // Phase 14 / UX-28 — non-blocking; failure leaves the badge hidden.
    void fetchWatchersCount()
  }

  // Deep-link path: if the URL already has ?ugc=comments on first paint,
  // kick off the initial comments fetch (the watch(ugcTab) lazy-fetch only
  // fires on subsequent changes, not on initial value).
  if (ugcTab.value === 'comments' && !commentsFetched.value) {
    void fetchComments()
  }

  // Reviews feed + related rail load lazily as the user scrolls toward them
  // (page-fetch optimization 2026-06-11). Arm after the DOM has rendered the
  // observed elements (they're v-if'd on `anime`).
  await nextTick()
  if (gen !== loadGeneration) return
  armLazySections()
}

const { ratings: relatedSiteRatings, fetchRatings: fetchRelatedSiteRatings } = useSiteRatings()

function relatedCardModel(item: RelatedAnime): AnimeCardModel {
  const sr = relatedSiteRatings.value[String(item.id)]
  return fromCatalogAnime(
    { ...item, totalEpisodes: item.episodes },
    { siteScore: sr && sr.total_reviews > 0 ? sr.average_score : undefined },
  )
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
      year?: number
      episodes?: number
    }>
    relatedAnime.value = data.map(r => ({
      id: r.local_id || `shiki_${r.shikimori_id}`,
      title: r.name,
      nameRu: r.name_ru,
      name: r.name,
      coverImage: getImageUrl(r.poster_url),
      rating: r.score || undefined,
      releaseYear: r.year || undefined,
      episodes: r.episodes || undefined,
      relationLabel: locale.value === 'ru' ? r.relation_ru : r.relation_en,
    }))
    // Site ratings exist only for anime already in the local catalog (UUID ids)
    void fetchRelatedSiteRatings(data.filter(r => r.local_id).map(r => String(r.local_id)))
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
