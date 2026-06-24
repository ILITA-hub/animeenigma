<template>
  <div class="min-h-screen">
    <!-- Loading State -->
    <div v-if="loading" class="flex justify-center items-center min-h-screen pt-20">
      <Spinner size="lg" />
    </div>

    <!-- Error State -->
    <div v-else-if="error" class="flex flex-col items-center justify-center min-h-screen pt-20 px-4">
      <TriangleAlert class="size-16 text-white/20 mb-4" />
      <p class="text-white/60 text-lg">{{ error }}</p>
      <router-link to="/" class="mt-4 text-cyan-400 hover:text-cyan-300">
        {{ $t('profile.goHome') }}
      </router-link>
    </div>

    <template v-else-if="profileUser">
      <!-- Profile Header -->
      <div class="relative overflow-hidden">
        <div class="absolute inset-0 bg-gradient-to-b from-cyan-500/10 to-transparent" />

        <div class="relative max-w-4xl mx-auto px-4 pt-24 pb-8">
          <div class="flex flex-col sm:flex-row items-center sm:items-end gap-6">
            <!-- Avatar -->
            <Avatar
              :src="profileUser.avatar"
              :name="profileUser.username"
              size="2xl"
              class="ring-4 ring-cyan-500/30"
            >
              <button
                v-if="isOwnProfile"
                @click="showAvatarModal = true"
                type="button"
                :aria-label="$t('profile.uploadAvatar')"
                :title="$t('profile.uploadAvatar')"
                class="absolute bottom-0 right-0 w-8 h-8 rounded-full bg-cyan-500 flex items-center justify-center text-white shadow-lg hover:bg-cyan-400 transition-colors"
              >
                <Pencil class="size-4" aria-hidden="true" />
              </button>
            </Avatar>

            <!-- User Info -->
            <div class="text-center sm:text-left flex-1">
              <h1 class="text-2xl sm:text-3xl font-semibold text-white mb-1">
                {{ profileUser.username }}
              </h1>
              <div class="flex flex-wrap items-center justify-center sm:justify-start gap-2">
                <Badge v-if="isOwnProfile" variant="primary" size="sm">{{ profileUser.role || 'Member' }}</Badge>
                <span class="text-white/60 text-sm">
                  {{ isOwnProfile ? `${$t('profile.memberSince')} ${memberSinceYear}` : $t('profile.userProfile') }}
                </span>
              </div>
            </div>

            <!-- Action Buttons -->
            <div class="flex gap-2">
              <!-- Share Button -->
              <button
                v-if="profileUser.public_id"
                @click="copyProfileLink"
                class="flex items-center gap-2 px-4 py-2 rounded-lg bg-cyan-500/10 backdrop-blur-xl border border-cyan-500/20 text-cyan-400 hover:bg-cyan-500/20 transition-colors"
              >
                <Share2 class="size-5" />
                <span>{{ copied ? $t('profile.copied') : $t('profile.share') }}</span>
              </button>
            </div>
          </div>
        </div>
      </div>

      <!-- Tabs -->
      <div class="max-w-4xl mx-auto px-4">
        <Tabs v-model="activeTab" :tabs="tabs" variant="underline" full-width class="mb-6">
          <!-- Showcase Tab (dark-ship gate: VITE_PROFILE_WALL_ADMIN_ONLY) -->
          <template v-if="profileWallVisible" #showcase>
            <ProfileShowcase
              v-if="profileUser?.id"
              :user-id="profileUser.id"
              :is-owner="!!isOwnProfile"
              @loaded="onShowcaseLoaded"
            />
          </template>

          <!-- Watchlist Tab -->
          <template #watchlist>
            <!-- Loading -->
            <div v-if="loadingWatchlist" class="flex justify-center py-12">
              <Spinner size="lg" />
            </div>

            <div v-else-if="hasAnyEntries" class="space-y-4">
              <!-- Stats Bar -->
              <div class="grid grid-cols-2 sm:grid-cols-4 gap-3">
                <div class="glass-card p-3 text-center">
                  <div class="text-2xl font-semibold text-cyan-400">{{ watchlistStats.total }}</div>
                  <div class="text-xs text-white/50">{{ $t('profile.stats.totalAnime') }}</div>
                </div>
                <div class="glass-card p-3 text-center">
                  <div class="text-2xl font-semibold text-cyan-400 flex items-center justify-center gap-1">
                    <ScoreDiamond class="size-4" />
                    {{ watchlistStats.avgScore }}
                  </div>
                  <div class="text-xs text-white/50">{{ $t('profile.stats.avgScore') }}</div>
                </div>
                <div class="glass-card p-3 text-center">
                  <div class="text-2xl font-semibold text-success">{{ watchlistStats.totalEpisodes }}</div>
                  <div class="text-xs text-white/50">{{ $t('profile.stats.episodesWatched') }}</div>
                </div>
                <div class="glass-card p-3 text-center">
                  <div class="text-2xl font-semibold text-info">{{ watchlistStats.completed }}</div>
                  <div class="text-xs text-white/50">{{ $t('profile.stats.completed') }}</div>
                </div>
              </div>

              <!-- Filter Pills — DS Chip primitive -->
              <div class="flex gap-2 overflow-x-auto pb-2 scrollbar-hide" role="group" :aria-label="$t('profile.watchlist.statusFilter')">
                <Chip
                  v-for="filter in watchlistFilters"
                  :key="filter.value"
                  :active="watchlistFilter === filter.value"
                  :count="filter.count"
                  @click="watchlistFilter = filter.value"
                >
                  {{ filter.label }}
                </Chip>
              </div>

              <!-- View Toggle + Sort -->
              <div class="flex flex-wrap items-center justify-end gap-2">
                <div class="flex-shrink-0 w-48 mr-auto">
                  <Input v-model="searchQuery" type="text" size="sm" :placeholder="$t('profile.watchlist.searchPlaceholder')" />
                </div>
                <!-- Sort -->
                <div class="w-28 sm:w-36">
                  <Select
                    v-model="sortKey"
                    :options="sortOptions"
                    size="sm"
                  />
                </div>
                <Button
                  variant="ghost"
                  size="icon-sm"
                  class="text-white/60 hover:text-white"
                  :title="sortDirection === 'asc' ? $t('profile.sort.asc') : $t('profile.sort.desc')"
                  :aria-label="sortDirection === 'asc' ? $t('profile.sort.asc') : $t('profile.sort.desc')"
                  @click="sortDirection = sortDirection === 'asc' ? 'desc' : 'asc'"
                >
                  <ArrowUpDown class="size-5! transition-transform" :class="sortDirection === 'desc' ? 'rotate-180' : ''" />
                </Button>
                <!-- Filters trigger — inline, one line with search/sort/view -->
                <Button
                  variant="ghost"
                  size="sm"
                  class="gap-1.5 text-white/70 hover:text-white"
                  :aria-expanded="filtersOpen"
                  @click="filtersOpen = !filtersOpen"
                >
                  <SlidersHorizontal class="size-4" />
                  <span class="hidden sm:inline">{{ $t('profile.filters.button') }}</span>
                  <Badge v-if="filterCount > 0" variant="primary" size="sm">{{ filterCount }}</Badge>
                  <ChevronDown class="size-4 transition-transform duration-200" :class="filtersOpen ? 'rotate-180' : ''" />
                </Button>
                <Button
                  v-if="isOwnProfile"
                  variant="ghost"
                  size="sm"
                  class="gap-1.5"
                  :class="selectionMode ? 'text-cyan-400' : 'text-white/70 hover:text-white'"
                  :aria-pressed="selectionMode"
                  @click="selectionMode ? exitSelectionMode() : (selectionMode = true)"
                >
                  <CheckSquare class="size-4" />
                  <span class="hidden sm:inline">{{ $t('profile.bulk.select') }}</span>
                </Button>
                <SegmentedControl
                  :model-value="viewMode"
                  :options="viewModeOptions"
                  icon-only
                  @update:model-value="viewMode = $event as 'table' | 'grid'"
                />
              </div>

              <!-- Separate full-width filter block, toggled by the trigger above -->
              <Transition
                enter-active-class="transition duration-150 ease-out"
                enter-from-class="opacity-0 -translate-y-1"
                enter-to-class="opacity-100 translate-y-0"
                leave-active-class="transition duration-100 ease-in"
                leave-from-class="opacity-100 translate-y-0"
                leave-to-class="opacity-0 -translate-y-1"
              >
                <WatchlistFilters
                  v-if="filtersOpen"
                  v-model:genre-ids="filterState.genreIds"
                  v-model:kinds="filterState.kinds"
                  v-model:year-min="filterState.yearMin"
                  v-model:year-max="filterState.yearMax"
                  :facets="facets"
                />
              </Transition>

              <!-- Table/Grid Content with Loading Overlay -->
              <div class="relative">
              <div v-if="watchlistPageLoading && watchlist.length > 0" class="absolute inset-0 bg-black/30 backdrop-blur-sm z-10 flex items-center justify-center rounded-lg">
                <Spinner size="lg" />
              </div>

              <!-- Table or Grid (only when the current filter has items) -->
              <template v-if="watchlist.length > 0">
              <!-- Compact list view (rebuilt 2026-06-12: responsive rows replace the
                   fixed 9-column table — controls wrap under the title on mobile
                   instead of forcing a horizontal page scroll) -->
              <label v-if="isOwnProfile && selectionMode && viewMode === 'table'" class="flex items-center gap-2 px-1 py-2 cursor-pointer">
                <Checkbox :model-value="allOnPageSelected" @update:model-value="toggleSelectAllOnPage" />
                <span class="text-xs text-muted-foreground">{{ $t('profile.bulk.selectAllPage') }}</span>
              </label>
              <div v-if="viewMode === 'table'" class="flex flex-col">
                <WatchlistRow
                  v-for="(anime, index) in filteredWatchlist"
                  :key="anime.anime_id"
                  :entry="anime"
                  :index="(watchlistPage - 1) * watchlistPerPage + index + 1"
                  :is-own="!!isOwnProfile"
                  :status-options="statusOptions"
                  :selectable="!!isOwnProfile && selectionMode"
                  :selected="selectedIds.has(anime.anime_id)"
                  @toggle-select="toggleSelect(anime.anime_id)"
                  @edit-score="(v: string) => finishEditScore(anime.anime_id, v)"
                  @update-episodes="(n: number) => updateAnimeEpisodes(anime.anime_id, n)"
                  @update-rewatch-count="(n: number) => updateAnimeRewatchCount(anime.anime_id, n)"
                  @update-date="(f: 'started_at' | 'completed_at', v: string) => updateAnimeDate(anime.anime_id, f, v)"
                  @update-status="(s: string) => updateAnimeStatus(anime.anime_id, s)"
                  @remove="removeFromWatchlist(anime.anime_id)"
                />
              </div>

              <!-- Grid View — unified PosterCard (spec 2026-06-05 migration item 11).
                   Status / remove / mark-next live in the kebab context menu;
                   the viewer's own score badge is click-to-edit on own profile. -->
              <div v-else class="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 gap-4">
                <div
                  v-for="anime in filteredWatchlist"
                  :key="anime.anime_id"
                  class="relative"
                  @touchstart="(e) => onProfileTouchstart(e, anime)"
                  @touchmove="onProfileTouchmove"
                  @touchend="onProfileTouchend"
                >
                  <PosterCard
                    :model="watchlistCardModel(anime)"
                    :menu-open="profileContextMenu.visible && String(profileContextMenu.anime?.id) === String(anime.anime_id)"
                    :score-editable="!!isOwnProfile"
                    @open-menu="(el: HTMLElement) => openProfileMenuAt(el, anime)"
                    @edit-score="editingScoreGrid = anime.anime_id"
                  />
                  <!-- Score edit popover (own profile) -->
                  <div
                    v-if="isOwnProfile && editingScoreGrid === anime.anime_id"
                    class="absolute top-2 right-2 z-40 w-14"
                    @click.prevent.stop
                  >
                    <Input
                      type="number"
                      size="sm"
                      min="0" max="10"
                      :model-value="String(anime.score || 0)"
                      @blur="(e: Event) => { finishEditScore(anime.anime_id, (e.target as HTMLInputElement).value); editingScoreGrid = null; }"
                      @keydown.enter="(e: KeyboardEvent) => (e.target as HTMLInputElement).blur()"
                      @keydown.escape="editingScoreGrid = null"
                      class="text-center text-cyan-400"
                    />
                  </div>
                  <template v-if="isOwnProfile && selectionMode">
                    <div
                      class="absolute inset-0 z-30 rounded-xl cursor-pointer"
                      :class="selectedIds.has(anime.anime_id) ? 'ring-2 ring-cyan-400 bg-cyan-400/10' : ''"
                      @click.prevent.stop="toggleSelect(anime.anime_id)"
                    />
                    <label class="absolute top-2 left-2 z-40" @click.stop>
                      <Checkbox :model-value="selectedIds.has(anime.anime_id)" @update:model-value="() => toggleSelect(anime.anime_id)" />
                    </label>
                  </template>
                </div>
              </div>

              </template><!-- /v-if="watchlist.length > 0" -->

              <!-- Current filter has no items, but the user has entries in other statuses -->
              <template v-else>
                <div v-if="watchlistPageLoading" class="flex justify-center py-12">
                  <Spinner size="lg" />
                </div>
                <EmptyState v-else class="py-12">
                  <template #icon><Archive class="size-16" /></template>
                  {{ $t('profile.empty.filter') }}
                </EmptyState>
              </template>

              </div><!-- end relative wrapper -->

              <WatchlistBulkBar
                v-if="isOwnProfile && selectionMode && selectedIds.size > 0"
                :count="selectedIds.size"
                :status-options="statusOptions"
                @set-status="bulkSetStatus"
                @remove="bulkRemove"
                @clear="exitSelectionMode"
              />

              <!-- Pagination -->
              <PaginationBar
                v-if="watchlist.length > 0"
                :current-page="watchlistPage"
                :total-pages="watchlistTotalPages"
                @update:current-page="(p: number) => { watchlistPage = p; fetchWatchlistPage() }"
              />
            </div>

            <EmptyState v-else class="py-12">
              <template #icon><Archive class="size-16" /></template>
              {{ isOwnProfile ? $t('profile.empty.watchlist') : $t('profile.listEmpty') }}
              <template v-if="isOwnProfile" #action>
                <Button variant="outline" @click="$router.push('/browse')">
                  {{ $t('profile.actions.browseCatalog') }}
                </Button>
              </template>
            </EmptyState>
          </template>

          <!-- Settings Tab (own profile only) -->
          <template v-if="isOwnProfile" #settings>
            <div class="space-y-6">
              <!-- Import -->
              <div class="glass-card p-6">
                <h2 class="text-lg font-semibold text-white mb-4">{{ $t('profile.import.title') }}</h2>
                <div class="space-y-4">
                  <div>
                    <label class="block text-white/60 text-sm mb-2">MyAnimeList</label>
                    <div class="flex gap-2">
                      <div class="flex-1">
                        <Input v-model="malUsername" type="text" :placeholder="$t('profile.import.malPlaceholder')" class="bg-white/10" :disabled="malSync.importing" />
                      </div>
                      <Button
                        variant="primary"
                        :disabled="!malUsername || malSync.importing"
                        @click="importMAL"
                      >
                        <Spinner v-if="malSync.importing" size="sm" tone="mono" class="mr-2" />
                        {{ malSync.importing ? $t('profile.import.importing') : $t('profile.import.import') }}
                      </Button>
                    </div>
                    <p class="text-white/60 text-xs mt-2">
                      {{ $t('profile.import.malDescription') }}
                    </p>
                    <!-- Progress bar -->
                    <div v-if="malSync.progress" class="mt-3">
                      <div class="h-2 bg-white/10 rounded-full overflow-hidden">
                        <div
                          class="h-full bg-cyan-500 transition-all duration-300"
                          :style="{ width: (malSync.progress.total > 0 ? ((malSync.progress.imported + malSync.progress.skipped) / malSync.progress.total * 100) : 0) + '%' }"
                        />
                      </div>
                      <p class="text-sm text-white/60 mt-1">
                        {{ malSync.progress.imported + malSync.progress.skipped }} / {{ malSync.progress.total }}
                        <span v-if="malSync.progress.status === 'completed'" class="text-success ml-2">
                          {{ $t('profile.import.imported') }}: {{ malSync.progress.imported }} | {{ $t('profile.import.skipped') }}: {{ malSync.progress.skipped }}
                        </span>
                      </p>
                    </div>
                    <div v-if="malSync.lastSync && !malSync.progress" class="mt-2">
                      <p class="text-xs text-white/60">
                        {{ $t('profile.import.lastSynced', { time: timeAgo(malSync.lastSync.completed_at), imported: malSync.lastSync.imported, skipped: malSync.lastSync.skipped }) }}
                      </p>
                    </div>
                    <div v-if="malSync.error" class="mt-3 p-3 rounded-lg bg-pink-500/20">
                      <p class="text-sm text-pink-400">{{ malSync.error }}</p>
                    </div>
                  </div>

                  <!-- Shikimori Import -->
                  <div>
                    <label class="block text-white/60 text-sm mb-2">Shikimori</label>
                    <div class="flex gap-2">
                      <div class="flex-1">
                        <Input v-model="shikimoriNickname" type="text" :placeholder="$t('profile.import.shikimoriPlaceholder')" class="bg-white/10" :disabled="shikimoriSync.importing" />
                      </div>
                      <Button
                        variant="primary"
                        :disabled="!shikimoriNickname || shikimoriSync.importing"
                        @click="importShikimori"
                      >
                        <Spinner v-if="shikimoriSync.importing" size="sm" tone="mono" class="mr-2" />
                        {{ shikimoriSync.importing ? $t('profile.import.importing') : $t('profile.import.import') }}
                      </Button>
                    </div>
                    <p class="text-white/60 text-xs mt-2">
                      {{ $t('profile.import.shikimoriDescription') }}
                    </p>
                    <!-- Progress bar -->
                    <div v-if="shikimoriSync.progress" class="mt-3">
                      <div class="h-2 bg-white/10 rounded-full overflow-hidden">
                        <div
                          class="h-full bg-cyan-500 transition-all duration-300"
                          :style="{ width: (shikimoriSync.progress.total > 0 ? ((shikimoriSync.progress.imported + shikimoriSync.progress.skipped) / shikimoriSync.progress.total * 100) : 0) + '%' }"
                        />
                      </div>
                      <p class="text-sm text-white/60 mt-1">
                        {{ shikimoriSync.progress.imported + shikimoriSync.progress.skipped }} / {{ shikimoriSync.progress.total }}
                        <span v-if="shikimoriSync.progress.status === 'completed'" class="text-success ml-2">
                          {{ $t('profile.import.imported') }}: {{ shikimoriSync.progress.imported }} | {{ $t('profile.import.skipped') }}: {{ shikimoriSync.progress.skipped }}
                        </span>
                      </p>
                    </div>
                    <div v-if="shikimoriSync.lastSync && !shikimoriSync.progress" class="mt-2">
                      <p class="text-xs text-white/60">
                        {{ $t('profile.import.lastSynced', { time: timeAgo(shikimoriSync.lastSync.completed_at), imported: shikimoriSync.lastSync.imported, skipped: shikimoriSync.lastSync.skipped }) }}
                      </p>
                    </div>
                    <div v-if="shikimoriSync.error" class="mt-3 p-3 rounded-lg bg-pink-500/20">
                      <p class="text-sm text-pink-400">{{ shikimoriSync.error }}</p>
                    </div>
                  </div>
                </div>
              </div>

              <!-- Export -->
              <div class="glass-card p-6">
                <h2 class="text-lg font-semibold text-white mb-4">{{ $t('profile.export.title') }}</h2>
                <p class="text-white/60 text-sm mb-4">{{ $t('profile.export.description') }}</p>
                <Button
                  variant="primary"
                  :disabled="exportingJSON"
                  @click="exportToJSON"
                >
                  <Spinner v-if="exportingJSON" size="sm" tone="mono" class="mr-2" />
                  <Download v-else class="size-4 mr-2" />
                  {{ exportingJSON ? $t('profile.export.exporting') : $t('profile.export.button') }}
                </Button>
                <div v-if="exportError" class="mt-3 p-3 rounded-lg bg-pink-500/20">
                  <p class="text-sm text-pink-400">{{ exportError }}</p>
                </div>
              </div>

              <!-- Public Profile -->
              <div class="glass-card p-6">
                <h2 class="text-lg font-semibold text-white mb-4">{{ $t('profile.publicProfile') }}</h2>
                <div class="space-y-4">
                  <!-- Public ID -->
                  <div>
                    <label class="block text-white/60 text-sm mb-2">{{ $t('profile.profileLink') }}</label>
                    <div class="flex gap-2">
                      <div class="flex-1 flex items-center bg-white/10 border border-white/10 rounded-lg overflow-hidden">
                        <span class="px-3 text-white/60 text-sm">/user/</span>
                        <div class="flex-1">
                          <Input v-model="publicId" type="text" size="sm" placeholder="your-username" class="bg-transparent border-0 pl-0" :disabled="savingPublicId" />
                        </div>
                      </div>
                      <Button
                        variant="primary"
                        :disabled="!publicId || savingPublicId || publicId === authStore.user?.public_id"
                        @click="savePublicId"
                      >
                        <Spinner v-if="savingPublicId" size="sm" tone="mono" />
                        <span v-else>{{ $t('profile.save') }}</span>
                      </Button>
                    </div>
                    <p v-if="publicIdError" class="text-pink-400 text-xs mt-2">{{ publicIdError }}</p>
                    <p v-else-if="publicIdSuccess" class="text-success text-xs mt-2">{{ $t('profile.linkUpdated') }}</p>
                    <p class="text-white/60 text-xs mt-2">
                      {{ $t('profile.linkValidation') }}
                    </p>
                  </div>

                  <!-- Public Link -->
                  <div v-if="authStore.user?.public_id" class="flex items-center gap-2 p-3 bg-white/5 rounded-lg">
                    <Link class="size-5 text-cyan-400 flex-shrink-0" />
                    <a
                      :href="`/user/${authStore.user.public_id}`"
                      target="_blank"
                      class="text-cyan-400 hover:text-cyan-300 text-sm truncate"
                    >
                      {{ siteOrigin }}/user/{{ authStore.user.public_id }}
                    </a>
                    <button
                      @click="copyProfileLink"
                      class="ml-auto p-1.5 rounded hover:bg-white/10 text-white/60 hover:text-white transition-colors"
                    >
                      <Copy class="size-4" />
                    </button>
                  </div>

                  <!-- Privacy Settings -->
                  <div>
                    <label class="block text-white/60 text-sm mb-2">{{ $t('profile.privacyLabel') }}</label>
                    <div class="space-y-2">
                      <label
                        v-for="status in allStatuses"
                        :key="status.value"
                        class="flex items-center gap-3 p-3 rounded-lg bg-white/5 hover:bg-white/10 cursor-pointer transition-colors"
                      >
                        <Checkbox
                          :model-value="publicStatuses.includes(status.value)"
                          @update:model-value="() => togglePublicStatus(status.value)"
                        />
                        <span class="text-white">{{ status.label }}</span>
                      </label>
                    </div>
                    <!-- Activity visibility (design 2026-06-12): what other
                         users see of this user's activity — feed + public
                         watchlist. Enforced server-side. -->
                    <label class="block text-white/60 text-sm mt-4 mb-2">{{ $t('profile.activityVisibility.label') }}</label>
                    <div class="space-y-2" role="radiogroup" :aria-label="$t('profile.activityVisibility.label')">
                      <label
                        v-for="option in activityVisibilityOptions"
                        :key="option.value"
                        class="flex items-start gap-3 p-3 rounded-lg bg-white/5 hover:bg-white/10 cursor-pointer transition-colors"
                      >
                        <!-- bespoke-keep: rich card-style radio (title + hint per option); RadioGroup primitive only models flat {value,label} options -->
                        <input
                          type="radio"
                          name="activity-visibility"
                          class="mt-1 size-4 accent-[var(--brand-violet)]"
                          :value="option.value"
                          :checked="activityVisibility === option.value"
                          @change="activityVisibility = option.value"
                        />
                        <span>
                          <span class="block text-white">{{ option.label }}</span>
                          <span class="block text-white/40 text-xs mt-0.5">{{ option.hint }}</span>
                        </span>
                      </label>
                    </div>
                    <div class="mt-3">
                      <Button
                        variant="outline"
                        size="sm"
                        :disabled="savingPrivacy"
                        @click="savePrivacy"
                      >
                        <Spinner v-if="savingPrivacy" size="sm" tone="mono" class="mr-2" />
                        {{ $t('profile.savePrivacy') }}
                      </Button>
                      <p v-if="privacySuccess" class="text-success text-xs mt-2">{{ $t('profile.privacySaved') }}</p>
                    </div>
                  </div>
                </div>
              </div>

              <!-- Phase 20 — Player polish: Skip-Intro CTA auto-dismiss
                   timeout (localStorage-backed, no backend). The OP/ED skip
                   CTA stays visible for the whole skip window otherwise,
                   which is visually noisy for users who want to watch the OP.
                   Composable: useSkipIntroSettings. Range clamped [2..60]. -->
              <div class="glass-card p-6">
                <h2 class="text-lg font-semibold text-white mb-4">{{ $t('profile.settings.player') }}</h2>
                <div>
                  <label for="skip-intro-dismiss-sec" class="block text-white/60 text-sm mb-2">
                    {{ $t('profile.settings.skipIntroDismissSec.label') }}
                  </label>
                  <div class="w-32">
                    <Input
                      id="skip-intro-dismiss-sec"
                      type="number"
                      size="sm"
                      :min="skipIntroMin" :max="skipIntroMax" step="1"
                      :model-value="String(skipIntroSec)"
                      @change="onSkipIntroSecChange"
                      class="bg-white/10"
                    />
                  </div>
                  <p class="text-white/60 text-xs mt-2">
                    {{ $t('profile.settings.skipIntroDismissSec.hint') }}
                  </p>
                </div>
              </div>

              <!-- API Key -->
              <div class="glass-card p-6">
                <h2 class="text-lg font-semibold text-white mb-4">{{ $t('profile.settings.apiKey') }}</h2>
                <p class="text-white/60 text-sm mb-4">{{ $t('profile.settings.apiKeyDescription') }}</p>

                <!-- Loading state -->
                <div v-if="apiKeyLoading" class="flex justify-center py-4">
                  <Spinner size="md" />
                </div>

                <template v-else>
                  <!-- Show generated key (once) -->
                  <div v-if="generatedApiKey" class="space-y-3">
                    <p class="text-sm text-warning font-medium">{{ $t('profile.settings.apiKeyGenerated') }}</p>
                    <div class="flex items-center gap-2 p-3 bg-white/5 rounded-lg font-mono text-sm text-white break-all">
                      <span class="flex-1">{{ generatedApiKey }}</span>
                      <button
                        @click="copyApiKey"
                        :aria-label="$t('profile.settings.apiKeyCopy')"
                        class="flex-shrink-0 p-1.5 rounded hover:bg-white/10 text-white/60 hover:text-white transition-colors"
                      >
                        <Check v-if="apiKeyCopied" class="size-4 text-success" />
                        <Copy v-else class="size-4" />
                      </button>
                    </div>
                    <p v-if="apiKeyCopied" class="text-success text-xs">{{ $t('profile.settings.apiKeyCopied') }}</p>
                    <div class="p-3 bg-white/5 rounded-lg">
                      <p class="text-white/60 text-xs mb-1">{{ $t('profile.settings.apiKeyUsageHint') }}</p>
                      <code class="text-xs text-cyan-400 break-all">curl -H "Authorization: Bearer {{ generatedApiKey }}" {{ siteOrigin }}/api/users/import/mal -d '{"username":"..."}'</code>
                    </div>
                  </div>

                  <!-- Has key state -->
                  <div v-else-if="hasApiKey" class="space-y-3">
                    <p class="text-sm text-success">{{ $t('profile.settings.apiKeyHasKey') }}</p>
                    <div class="flex gap-2">
                      <Button variant="primary" size="sm" :disabled="apiKeyActioning" @click="regenerateApiKey">
                        {{ $t('profile.settings.regenerateApiKey') }}
                      </Button>
                      <Button variant="secondary" size="sm" :disabled="apiKeyActioning" @click="revokeApiKey">
                        {{ $t('profile.settings.revokeApiKey') }}
                      </Button>
                    </div>
                  </div>

                  <!-- No key state -->
                  <div v-else class="space-y-3">
                    <p class="text-sm text-white/60">{{ $t('profile.settings.apiKeyNone') }}</p>
                    <Button variant="primary" size="sm" :disabled="apiKeyActioning" @click="generateApiKey">
                      {{ $t('profile.settings.generateApiKey') }}
                    </Button>
                  </div>

                  <div v-if="apiKeyError" class="mt-3 p-3 rounded-lg bg-pink-500/20">
                    <p class="text-sm text-pink-400">{{ apiKeyError }}</p>
                  </div>
                  <p v-if="apiKeyRevoked" class="text-success text-xs mt-2">{{ $t('profile.settings.apiKeyRevoked') }}</p>
                </template>
              </div>

              <!-- Timezone -->
              <TimezoneCard />

              <!-- Active Sessions -->
              <ActiveSessionsCard />

              <!-- Account -->
              <div class="glass-card p-6">
                <h2 class="text-lg font-semibold text-white mb-4">{{ $t('profile.settings.account') }}</h2>
                <div class="space-y-4">
                  <Button variant="secondary" full-width @click="logout">
                    {{ $t('profile.settings.signOut') }}
                  </Button>
                </div>
              </div>
            </div>
          </template>

          <!-- Advanced Tab (own profile only) — Phase 7 B-05 -->
          <template v-if="isOwnProfile" #advanced>
            <div class="space-y-6">
              <div class="glass-card p-6">
                <h2 class="text-lg font-semibold text-white mb-2">{{ $t('profile.advanced.title') }}</h2>
                <p class="text-white/60 text-sm mb-4">{{ $t('profile.advanced.description') }}</p>

                <div v-if="loadingTier2View" class="text-white/60 text-sm py-4">
                  {{ $t('profile.advanced.loading') }}
                </div>
                <div v-else-if="!tier2View" class="text-white/60 text-sm py-4">
                  {{ $t('profile.advanced.unavailable') }}
                </div>
                <div v-else class="space-y-5">
                  <!-- Lock summary -->
                  <div class="rounded-lg p-4" :class="tier2View.lock ? 'bg-success/10 border border-success/30' : 'bg-warning/10 border border-warning/30'">
                    <div class="flex items-start gap-3">
                      <span class="text-2xl">{{ tier2View.lock ? '🎯' : '🛟' }}</span>
                      <div class="flex-1">
                        <div class="text-white font-semibold">
                          {{ tier2View.lock
                            ? $t('profile.advanced.lockActive', { lang: tier2View.lock.language.toUpperCase(), wt: $t(`profile.advanced.wt.${tier2View.lock.watch_type}`) })
                            : $t('profile.advanced.lockInactive') }}
                        </div>
                        <div class="text-white/60 text-sm mt-1">
                          {{ tier2View.lock
                            ? $t('profile.advanced.lockTeam', { team: tier2View.lock.top_translation_title || $t('profile.advanced.noTeam') })
                            : $t('profile.advanced.lockInactiveDescription', { current: tier2View.total_weight.toFixed(0), floor: tier2View.min_confidence.toFixed(0) }) }}
                        </div>
                      </div>
                    </div>
                  </div>

                  <!-- Tunables -->
                  <div class="grid grid-cols-1 sm:grid-cols-3 gap-3 text-sm">
                    <div class="glass-card p-3">
                      <div class="text-white/60 text-xs">{{ $t('profile.advanced.totalWeight') }}</div>
                      <div class="text-white font-mono mt-1">{{ tier2View.total_weight.toFixed(1) }}</div>
                    </div>
                    <div class="glass-card p-3">
                      <div class="text-white/60 text-xs">{{ $t('profile.advanced.minConfidence') }}</div>
                      <div class="text-white font-mono mt-1">{{ tier2View.min_confidence.toFixed(0) }}</div>
                    </div>
                    <div class="glass-card p-3">
                      <div class="text-white/60 text-xs">{{ $t('profile.advanced.halfLifeDays') }}</div>
                      <div class="text-white font-mono mt-1">{{ tier2View.half_life_days }}d</div>
                    </div>
                  </div>

                  <!-- Coarse signal table -->
                  <div>
                    <h3 class="text-white font-medium mb-2">{{ $t('profile.advanced.coarseTitle') }}</h3>
                    <p class="text-white/60 text-xs mb-3">{{ $t('profile.advanced.coarseDescription') }}</p>
                    <div v-if="tier2View.coarse.length === 0" class="text-white/60 text-sm py-2">
                      {{ $t('profile.advanced.empty') }}
                    </div>
                    <div v-else class="overflow-x-auto">
                      <table class="w-full text-sm">
                        <thead class="text-white/60 text-xs">
                          <tr>
                            <th class="pb-2 text-left">{{ $t('profile.advanced.col.lang') }}</th>
                            <th class="pb-2 text-left">{{ $t('profile.advanced.col.wt') }}</th>
                            <th class="pb-2 text-right">{{ $t('profile.advanced.col.weight') }}</th>
                          </tr>
                        </thead>
                        <tbody class="text-white/80">
                          <tr v-for="(c, i) in tier2View.coarse" :key="`coarse-${i}`" class="border-t border-white/5">
                            <td class="py-2">{{ c.language.toUpperCase() }}</td>
                            <td class="py-2">{{ $t(`profile.advanced.wt.${c.watch_type}`) }}</td>
                            <td class="py-2 text-right font-mono">{{ c.weight.toFixed(1) }}</td>
                          </tr>
                        </tbody>
                      </table>
                    </div>
                  </div>

                  <!-- Fine signal table (top 8) -->
                  <div>
                    <h3 class="text-white font-medium mb-2">{{ $t('profile.advanced.fineTitle') }}</h3>
                    <p class="text-white/60 text-xs mb-3">{{ $t('profile.advanced.fineDescription') }}</p>
                    <div v-if="tier2View.fine.length === 0" class="text-white/60 text-sm py-2">
                      {{ $t('profile.advanced.empty') }}
                    </div>
                    <div v-else class="overflow-x-auto">
                      <table class="w-full text-sm">
                        <thead class="text-white/60 text-xs">
                          <tr>
                            <th class="pb-2 text-left">{{ $t('profile.advanced.col.team') }}</th>
                            <th class="pb-2 text-left">{{ $t('profile.advanced.col.lang') }}</th>
                            <th class="pb-2 text-left">{{ $t('profile.advanced.col.wt') }}</th>
                            <th class="pb-2 text-left">{{ $t('profile.advanced.col.player') }}</th>
                            <th class="pb-2 text-right">{{ $t('profile.advanced.col.weight') }}</th>
                          </tr>
                        </thead>
                        <tbody class="text-white/80">
                          <tr v-for="(f, i) in tier2View.fine.slice(0, 8)" :key="`fine-${i}`" class="border-t border-white/5">
                            <td class="py-2">{{ f.translation_title || '—' }}</td>
                            <td class="py-2">{{ f.language.toUpperCase() }}</td>
                            <td class="py-2">{{ $t(`profile.advanced.wt.${f.watch_type}`) }}</td>
                            <td class="py-2">{{ f.player }}</td>
                            <td class="py-2 text-right font-mono">{{ f.weight.toFixed(1) }}</td>
                          </tr>
                        </tbody>
                      </table>
                    </div>
                  </div>
                </div>
              </div>

              <!-- Reset learned preferences -->
              <div class="glass-card p-6">
                <h2 class="text-lg font-semibold text-white mb-2">{{ $t('profile.advanced.resetTitle') }}</h2>
                <p class="text-white/60 text-sm mb-4">{{ $t('profile.advanced.resetDescription') }}</p>
                <div class="flex flex-col sm:flex-row gap-3">
                  <Button variant="secondary" :disabled="resettingPrefs" @click="onResetLearnedPreferences">
                    <Spinner v-if="resettingPrefs" size="sm" tone="mono" class="mr-2" />
                    {{ resettingPrefs ? $t('profile.advanced.resetting') : $t('profile.advanced.resetButton') }}
                  </Button>
                  <Button variant="ghost" @click="loadTier2View" :disabled="loadingTier2View">
                    {{ $t('profile.advanced.refresh') }}
                  </Button>
                </div>
                <p v-if="resetMessage" class="text-success text-sm mt-3">{{ resetMessage }}</p>
              </div>
            </div>
          </template>

          <!-- Gacha Collection Tab — own profile only (I2: don't leak viewer's collection to other users' profiles) -->
          <template v-if="isOwnProfile && gachaVisible" #collection>
            <GachaCollection />
          </template>
        </Tabs>
      </div>
    </template>

    <!-- Avatar Upload Modal -->
    <Modal v-model="showAvatarModal" :title="$t('profile.avatar.title')" size="sm">
      <div class="space-y-4">
        <!-- Preview -->
        <div class="flex justify-center">
          <Avatar
            :src="avatarPreview || undefined"
            :name="profileUser?.username"
            size="3xl"
            class="ring-4 ring-cyan-500/30"
          />
        </div>
        <!-- File Input -->
        <div class="text-center">
          <label class="inline-flex items-center gap-2 px-4 py-2 rounded-lg bg-white/10 border border-white/10 text-white hover:bg-white/20 cursor-pointer transition-colors">
            <ImageIcon class="size-5" />
            {{ $t('profile.avatar.selectFile') }}
            <!-- bespoke-keep: file upload; out of Input-primitive scope -->
            <input
              type="file"
              accept="image/jpeg,image/png,image/webp"
              class="hidden"
              @change="handleAvatarFile"
            />
          </label>
          <p class="text-white/60 text-xs mt-2">{{ $t('profile.avatar.formats') }}</p>
        </div>
      </div>
      <template #footer>
        <Button variant="ghost" @click="showAvatarModal = false">{{ $t('common.cancel') }}</Button>
        <Button
          variant="primary"
          :disabled="!avatarPreview || uploadingAvatar"
          @click="uploadAvatar"
        >
          <Spinner v-if="uploadingAvatar" size="sm" tone="mono" class="mr-2" />
          {{ uploadingAvatar ? $t('profile.avatar.uploading') : $t('profile.avatar.upload') }}
        </Button>
      </template>
    </Modal>
  </div>

  <!-- Context Menu for grid cards -->
  <AnimeContextMenu
    :visible="profileContextMenu.visible"
    :x="profileContextMenu.x"
    :y="profileContextMenu.y"
    :anchor-el="profileContextMenu.anchorEl"
    :anime="profileContextMenu.anime"
    :list-status="profileContextMenu.listStatus"
    :site-rating="profileContextMenu.siteRating"
    :episodes-watched="profileContextMenu.episodesWatched"
    :episodes-total="profileContextMenu.episodesTotal"
    @update:visible="profileContextMenu.visible = $event"
    @status-change="onContextMenuStatusChange"
    @remove-from-list="onContextMenuRemove"
    @episodes-change="onContextMenuEpisodesChange"
  />
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch, nextTick } from 'vue'
import { TriangleAlert, Pencil, Share2, ArrowUpDown, List, LayoutGrid, Archive, Download, Link, Copy, Check, Image as ImageIcon, SlidersHorizontal, ChevronDown, CheckSquare } from 'lucide-vue-next'
import { useDebounceFn } from '@vueuse/core'
import { useRouter, useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '@/stores/auth'
import { useWatchlistStore } from '@/stores/watchlist'
import { Avatar, Badge, Button, Checkbox, Chip, EmptyState, Input, Modal, Tabs, Select, PaginationBar, ScoreDiamond, Spinner, SegmentedControl, type SelectOption } from '@/components/ui'
import ActiveSessionsCard from '@/components/profile/ActiveSessionsCard.vue'
import TimezoneCard from '@/components/profile/TimezoneCard.vue'
import GachaCollection from '@/components/profile/GachaCollection.vue'
import { useGachaVisible } from '@/utils/gachaGate'
import ProfileShowcase from '@/components/profile/showcase/ProfileShowcase.vue'
import { useProfileWallVisible } from '@/utils/profileWallGate'
import { AnimeContextMenu, PosterCard } from '@/components/anime'
import WatchlistRow from '@/components/profile/WatchlistRow.vue'
import WatchlistFilters from '@/components/profile/WatchlistFilters.vue'
import WatchlistBulkBar from '@/components/profile/WatchlistBulkBar.vue'
import type { WatchlistFacets, WatchlistFilterState } from '@/types/watchlist-facets'
import { EMPTY_FILTER_STATE, filterParams, filterKey, activeFilterCount } from '@/types/watchlist-facets'
import { fromWatchlistEntry } from '@/utils/toCardModel'
import { userApi, publicApi } from '@/api/client'
import { useToast } from '@/composables/useToast'
import { useConfirm } from '@/composables/useConfirm'
import { getLocalizedTitle } from '@/utils/title'
import { getImageUrl } from '@/composables/useImageProxy'
import { useContextMenu } from '@/composables/useContextMenu'
import { useSkipIntroSettings } from '@/composables/useSkipIntroSettings'
import { timeAgo as formatTimeAgo, importErrorMessage as buildImportErrorMessage, resizeAvatarToDataUrl, type ApiError } from '@/composables/profile/profileHelpers'

interface ApiKeyError {
  response?: {
    data?: {
      error?: {
        message?: string
      }
    }
  }
}

interface WatchlistEntry {
  anime_id: string
  anime?: {
    name: string
    name_ru?: string
    name_jp?: string
    poster_url?: string
    episodes_count: number
    episodes_aired?: number
    genres?: Array<{ id: string; name: string; name_ru?: string }>
  }
  status: string
  score?: number
  episodes?: number
  rewatch_count?: number
  started_at?: string | null
  completed_at?: string | null
}

interface ProfileUser {
  id: string
  username: string
  public_id?: string
  public_statuses?: string[]
  role?: string
  avatar?: string
  created_at?: string
}

const router = useRouter()
const route = useRoute()
const { t, te } = useI18n()
const authStore = useAuthStore()
const watchlistStore = useWatchlistStore()
const toast = useToast()
const { confirm } = useConfirm()
const gachaVisible = useGachaVisible()
const profileWallVisible = useProfileWallVisible()

const siteOrigin = window.location.origin

// Context menu for grid view (kebab affordance + mobile long-press)
const {
  contextMenu: profileContextMenu,
  openAtElement: openProfileCtxAt,
  onTouchstart: onProfileCtxTouchstart,
  onTouchmove: onProfileTouchmove,
  onTouchend: onProfileTouchend,
} = useContextMenu()

function ctxAnimeFromEntry(entry: WatchlistEntry) {
  return {
    id: entry.anime_id,
    title: animeTitle(entry),
    name: entry.anime?.name,
    nameRu: entry.anime?.name_ru,
    nameJp: entry.anime?.name_jp,
    coverImage: animeCover(entry),
    episodes: entry.anime?.episodes_count,
  }
}

function ctxOptsFromEntry(entry: WatchlistEntry) {
  return {
    listStatus: entry.status,
    episodesWatched: entry.episodes ?? 0,
    episodesTotal: animeTotalEpisodes(entry),
  }
}

function openProfileMenuAt(el: HTMLElement, entry: WatchlistEntry) {
  openProfileCtxAt(el, ctxAnimeFromEntry(entry), ctxOptsFromEntry(entry))
}

// Unified PosterCard view-model for the grid (spec 2026-06-05 item 11).
const watchlistCardModel = (entry: WatchlistEntry) => fromWatchlistEntry(entry)

function onProfileTouchstart(event: TouchEvent, entry: WatchlistEntry) {
  onProfileCtxTouchstart(event, ctxAnimeFromEntry(entry), ctxOptsFromEntry(entry))
}

// Helpers for nested anime data from Preload
const animeTitle = (entry: WatchlistEntry): string =>
  getLocalizedTitle(entry.anime?.name, entry.anime?.name_ru, entry.anime?.name_jp) || 'Anime'
const animeCover = (entry: WatchlistEntry): string =>
  getImageUrl(entry.anime?.poster_url) || ''
const animeTotalEpisodes = (entry: WatchlistEntry): number =>
  entry.anime?.episodes_count || 0

// Profile state
const profileUser = ref<ProfileUser | null>(null)
const loading = ref(true)
const error = ref<string | null>(null)
const copied = ref(false)

// Check if viewing own profile
const isOwnProfile = computed(() => {
  if (!authStore.user) return false
  const publicId = route.params.publicId as string
  return publicId && authStore.user.public_id === publicId
})

// Member since year
const memberSinceYear = computed(() => {
  const raw = profileUser.value?.created_at || authStore.user?.created_at
  if (raw) {
    try {
      return new Date(raw).getFullYear()
    } catch { /* fallback below */ }
  }
  return new Date().getFullYear()
})

// Tabs
const activeTab = ref(profileWallVisible.value ? 'showcase' : 'watchlist')
const tabs = computed(() => {
  const baseTabs: Array<{ value: string; label: string }> = []
  if (profileWallVisible.value) baseTabs.push({ value: 'showcase', label: t('profile.tabs.showcase') })
  baseTabs.push({ value: 'watchlist', label: t('profile.tabs.watchlist') })
  if (isOwnProfile.value && gachaVisible.value) {
    baseTabs.push({ value: 'collection', label: t('gacha.collection_tab') })
  }
  if (isOwnProfile.value) {
    baseTabs.push(
      { value: 'settings', label: t('profile.tabs.settings') },
    )
  }
  return baseTabs
})

function onShowcaseLoaded(count: number) {
  if (count === 0 && !isOwnProfile.value && activeTab.value === 'showcase') {
    activeTab.value = 'watchlist'
  }
}

// Phase 7 — Advanced Settings tab state. Lazy-loaded the first time the user
// opens the tab so we don't spend a request on every Profile mount.
interface Tier2DebugView {
  coarse: Array<{ language: string; watch_type: string; weight: number }>
  fine: Array<{ language: string; watch_type: string; player: string; translation_id: string; translation_title: string; weight: number }>
  total_weight: number
  min_confidence: number
  half_life_days: number
  lock: { language: string; watch_type: string; top_translation_title: string; confidence: number } | null
}
const tier2View = ref<Tier2DebugView | null>(null)
const loadingTier2View = ref(false)
const resettingPrefs = ref(false)
const resetMessage = ref('')

async function loadTier2View() {
  if (!isOwnProfile.value) return
  loadingTier2View.value = true
  try {
    const { data } = await userApi.getTier2DebugView()
    const envelope = (data as { data?: Tier2DebugView }).data ?? (data as Tier2DebugView)
    tier2View.value = envelope
  } catch (err) {
    console.error('Failed to load Tier 2 debug view:', err)
    tier2View.value = null
  } finally {
    loadingTier2View.value = false
  }
}

async function onResetLearnedPreferences() {
  if (!(await confirm({
    title: t('common.confirmTitle'),
    description: t('profile.advanced.resetConfirm'),
    confirmText: t('common.confirm'),
    cancelText: t('common.cancel'),
    variant: 'destructive',
  }))) return
  resettingPrefs.value = true
  resetMessage.value = ''
  try {
    await userApi.resetLearnedPreferences()
    resetMessage.value = t('profile.advanced.resetSuccess')
    // Reload the debug view — coarse/fine signals come from watch_history
    // (preserved) so they'll still be there, but the per-anime locks are wiped.
    await loadTier2View()
  } catch (err) {
    console.error('Failed to reset learned preferences:', err)
    resetMessage.value = t('profile.advanced.resetError')
  } finally {
    resettingPrefs.value = false
  }
}

watch(activeTab, (newTab) => {
  if (newTab === 'advanced' && !tier2View.value && !loadingTier2View.value) {
    void loadTier2View()
  }
})

// Watchlist
const watchlist = ref<WatchlistEntry[]>([])
const loadingWatchlist = ref(false)
const watchlistFilter = ref('all')
const searchQuery = ref('')
const facets = ref<WatchlistFacets>({ genres: [], kinds: [], years: { min: null, max: null } })
const filterState = ref<WatchlistFilterState>({ ...EMPTY_FILTER_STATE })
const filtersOpen = ref(false)

// --- Bulk selection mode (own profile only) ---
const selectionMode = ref(false)
const selectedIds = ref<Set<string>>(new Set())

const allOnPageSelected = computed(() =>
  watchlist.value.length > 0 && watchlist.value.every((a) => selectedIds.value.has(a.anime_id)),
)

function toggleSelect(animeId: string) {
  const next = new Set(selectedIds.value)
  if (next.has(animeId)) next.delete(animeId)
  else next.add(animeId)
  selectedIds.value = next
}

function toggleSelectAllOnPage() {
  if (allOnPageSelected.value) selectedIds.value = new Set()
  else selectedIds.value = new Set(watchlist.value.map((a) => a.anime_id))
}

function exitSelectionMode() {
  selectionMode.value = false
  selectedIds.value = new Set()
}
const filterCount = computed(() => activeFilterCount(filterState.value))
const viewMode = ref<'table' | 'grid'>('grid')
const viewModeOptions = computed(() => [
  { value: 'table', label: t('profile.view.table'), icon: List },
  { value: 'grid', label: t('profile.view.grid'), icon: LayoutGrid },
])

// Pagination
const watchlistPage = ref(1)
const watchlistTotalPages = ref(0)
const watchlistTotalCount = ref(0)
const watchlistPerPage = 24
const watchlistPageLoading = ref(false)
const _watchlistInitialized = ref(false)

// Page cache
interface CachedPage {
  data: WatchlistEntry[]
  totalPages: number
  totalCount: number
  fetchedAt: number
}
const pageCache = new Map<string, CachedPage>()
const PAGE_CACHE_TTL = 5 * 60 * 1000 // 5 minutes

function pageCacheKey(status: string, page: number): string {
  // Include the profile's user id so navigating between /user/alice and /user/bob
  // — which reuses the same Profile.vue component instance and therefore the same
  // in-memory pageCache Map — cannot serve one user's cached data to another user.
  const uid = profileUser.value?.id || 'anon'
  const q = searchQuery.value.trim().toLowerCase()
  return `${uid}:${status}:${page}:${sortKey.value}:${sortDirection.value}:${q}:${filterKey(filterState.value)}`
}
function clearPageCache() {
  pageCache.clear()
}

// Inline editing (grid score popover; compact-list rows manage their own edit state)
const editingScoreGrid = ref<string | null>(null)

// Sorting
const sortKey = ref<string>(localStorage.getItem('profile_sortKey') || 'title')
const sortDirection = ref<'asc' | 'desc'>((localStorage.getItem('profile_sortDir') as 'asc' | 'desc') || 'asc')

// Persist only for the own profile — sort changes made while browsing someone
// else's list must not overwrite the owner's saved preference.
watch(sortKey, (v) => { if (isOwnProfile.value) localStorage.setItem('profile_sortKey', v) })
watch(sortDirection, (v) => { if (isOwnProfile.value) localStorage.setItem('profile_sortDir', v) })

const sortOptions = computed<SelectOption[]>(() => [
  { value: 'title', label: t('profile.sort.title') },
  { value: 'score', label: t('profile.sort.score') },
  { value: 'progress', label: t('profile.sort.progress') },
  { value: 'status', label: t('profile.sort.status') },
  { value: 'genre', label: t('profile.sort.genre') },
])

// Map frontend sort keys to backend column names
const backendSortKey = computed(() => {
  const map: Record<string, string> = {
    title: 'title',
    score: 'score',
    progress: 'episodes',
    status: 'status',
    genre: 'genre',
  }
  return map[sortKey.value] || 'updated_at'
})

const statusLabels = computed<Record<string, string>>(() => ({
  all: t('profile.watchlist.all'),
  watching: t('profile.watchlist.watching'),
  completed: t('profile.watchlist.completed'),
  plan_to_watch: t('profile.watchlist.planToWatch'),
  on_hold: t('profile.watchlist.onHold'),
  dropped: t('profile.watchlist.dropped'),
}))

const statusOptions = computed<SelectOption[]>(() => [
  { value: 'watching', label: t('profile.watchlist.watching') },
  { value: 'plan_to_watch', label: t('profile.watchlist.planToWatch') },
  { value: 'completed', label: t('profile.watchlist.completed') },
  { value: 'on_hold', label: t('profile.watchlist.onHold') },
  { value: 'dropped', label: t('profile.watchlist.dropped') },
])

const watchlistFilters = computed(() => {
  const statuses = isOwnProfile.value
    ? ['all', 'watching', 'plan_to_watch', 'completed', 'on_hold', 'dropped']
    : ['all', ...(profileUser.value?.public_statuses || [])]

  // Use the status store for accurate per-status counts (all entries, not just current page)
  const statusMap = isOwnProfile.value ? watchlistStore.watchlistMap : null

  return statuses.map(status => {
    let count = 0
    if (status === 'all') {
      if (isOwnProfile.value) {
        count = watchlistStore.entries.length
      } else {
        // Sum of all public status counts, fallback to server total
        const sumCounts = Object.values(publicStatusCounts.value).reduce((s, c) => s + c, 0)
        count = sumCounts || watchlistTotalCount.value
      }
    } else if (statusMap) {
      // Count from the lightweight statuses endpoint (own profile)
      count = [...statusMap.entries()].filter(([, s]) => s === status).length
    } else {
      // Public profile: use per-status counts from parallel requests
      count = publicStatusCounts.value[status] ?? 0
    }
    return {
      value: status,
      label: statusLabels.value[status] || status,
      count,
    }
  })
})

const filteredWatchlist = computed(() => {
  // Sorting AND searching are handled server-side via fetchWatchlistPage.
  // The server returns the page that matches the current status/sort/q, so this
  // is just a passthrough — we keep the computed for stable template references.
  return watchlist.value
})

// Public profile aggregate stats (fetched from backend)
const publicWatchlistStats = ref<{ avg_score: number; total_episodes: number; total_entries: number; completed: number } | null>(null)

// True when the user has ANY watchlist entries across all statuses.
// Used to distinguish "current filter is empty" (keep filter UI visible) from
// "user has no entries at all" (show onboarding CTA).
const hasAnyEntries = computed(() => {
  if (_isOwnProfile.value) {
    // Own profile — lightweight statuses endpoint is fetched up-front, so this is authoritative
    return watchlistStore.entries.length > 0
  }
  // Public profile — prefer server aggregate, then per-status counts, then fall back to current-page data
  const stats = publicWatchlistStats.value
  if (stats && typeof stats.total_entries === 'number') return stats.total_entries > 0
  const sumCounts = Object.values(publicStatusCounts.value).reduce((s, c) => s + c, 0)
  if (sumCounts > 0) return true
  return watchlist.value.length > 0 || watchlistTotalCount.value > 0
})

// Stats
const watchlistStats = computed(() => {
  if (_isOwnProfile.value) {
    // Own profile: use full statuses data from the store
    const allEntries = watchlistStore.entries
    const scored = allEntries.filter(a => a.score && a.score > 0)
    const avgScore = scored.length > 0
      ? (scored.reduce((sum, a) => sum + (a.score || 0), 0) / scored.length).toFixed(1)
      : '-'
    const completedCount = allEntries.filter(a => a.status === 'completed').length
    return {
      total: allEntries.length || watchlistTotalCount.value || watchlist.value.length,
      avgScore,
      totalEpisodes: allEntries.reduce((sum, a) => sum + (a.episodes || 0), 0),
      completed: completedCount,
    }
  } else {
    // Public profile: use server-side aggregate stats
    const stats = publicWatchlistStats.value
    const sumCounts = Object.values(publicStatusCounts.value).reduce((s, c) => s + c, 0)
    return {
      total: stats?.total_entries || sumCounts || watchlistTotalCount.value,
      avgScore: stats ? (stats.avg_score > 0 ? stats.avg_score.toFixed(1) : '-') : '-',
      totalEpisodes: stats?.total_episodes ?? '-',
      completed: stats?.completed ?? publicStatusCounts.value['completed'] ?? 0,
    }
  }
})

// Unified import state
interface ImportJobState {
  jobId: string | null
  importing: boolean
  progress: { total: number; imported: number; skipped: number; status: string } | null
  error: string | null
  lastSync: { completed_at: string; imported: number; skipped: number } | null
}

const malUsername = ref('')
const malSync = ref<ImportJobState>({
  jobId: null, importing: false, progress: null, error: null, lastSync: null,
})

const shikimoriNickname = ref('')
const shikimoriSync = ref<ImportJobState>({
  jobId: null, importing: false, progress: null, error: null, lastSync: null,
})

const pollIntervals: Record<string, ReturnType<typeof setInterval> | null> = {
  mal: null,
  shikimori: null,
}

// Export
const exportingJSON = ref(false)
const exportError = ref<string | null>(null)

// Public Profile settings
const publicId = ref('')
const publicStatuses = ref<string[]>([])
type ActivityVisibility = 'all' | 'non_hentai' | 'none'
const activityVisibility = ref<ActivityVisibility>('all')
const savingPublicId = ref(false)
const savingPrivacy = ref(false)
const publicIdError = ref<string | null>(null)
const publicIdSuccess = ref(false)
const privacySuccess = ref(false)

const allStatuses = computed(() => [
  { value: 'watching', label: t('profile.watchlist.watching') },
  { value: 'completed', label: t('profile.watchlist.completed') },
  { value: 'plan_to_watch', label: t('profile.watchlist.planToWatch') },
  { value: 'on_hold', label: t('profile.watchlist.onHold') },
  { value: 'dropped', label: t('profile.watchlist.dropped') },
])

const activityVisibilityOptions = computed(() => [
  { value: 'all' as const, label: t('profile.activityVisibility.all'), hint: t('profile.activityVisibility.allHint') },
  { value: 'non_hentai' as const, label: t('profile.activityVisibility.nonHentai'), hint: t('profile.activityVisibility.nonHentaiHint') },
  { value: 'none' as const, label: t('profile.activityVisibility.none'), hint: t('profile.activityVisibility.noneHint') },
])

// Avatar
const showAvatarModal = ref(false)
const avatarPreview = ref<string | null>(null)
const avatarDataUrl = ref<string | null>(null)
const uploadingAvatar = ref(false)

// Phase 20 — Skip-Intro CTA auto-dismiss setting. Persists to localStorage
// via the composable; players observe the same reactive ref. Input handler
// clamps + writes through `set` (out-of-range values are normalized).
const skipIntroSettings = useSkipIntroSettings()
const skipIntroSec = skipIntroSettings.seconds
const skipIntroMin = skipIntroSettings.MIN_SEC
const skipIntroMax = skipIntroSettings.MAX_SEC
function onSkipIntroSecChange(e: Event) {
  const target = e.target as HTMLInputElement
  const raw = Number(target.value)
  skipIntroSettings.set(raw)
  // Reflect the clamped value back into the input so the user sees the
  // normalization happen if they typed something out of range.
  target.value = String(skipIntroSettings.seconds.value)
}

// API Key
const apiKeyLoading = ref(false)
const hasApiKey = ref(false)
const generatedApiKey = ref<string | null>(null)
const apiKeyCopied = ref(false)
const apiKeyActioning = ref(false)
const apiKeyError = ref<string | null>(null)
const apiKeyRevoked = ref(false)

// Methods
const fetchProfile = async () => {
  loading.value = true
  error.value = null

  try {
    const publicIdParam = route.params.publicId as string
    if (!publicIdParam) {
      error.value = t('profile.userNotFound')
      return
    }

    // Check if this is own profile
    if (!authStore.user) {
      await authStore.fetchUser()
    }

    const isOwn = authStore.user?.public_id === publicIdParam

    if (isOwn && authStore.user) {
      // Use authStore data for own profile (has more fields)
      profileUser.value = {
        id: authStore.user.id,
        username: authStore.user.username,
        public_id: authStore.user.public_id,
        public_statuses: authStore.user.public_statuses,
        role: authStore.user.role,
        avatar: authStore.user.avatar,
        created_at: authStore.user.created_at,
      }
      publicId.value = authStore.user.public_id || ''
      publicStatuses.value = authStore.user.public_statuses || ['watching', 'completed', 'plan_to_watch']
      activityVisibility.value = authStore.user.activity_visibility || 'all'
    } else {
      // Fetch public profile data
      const response = await publicApi.getUserProfile(publicIdParam)
      const data = response.data?.data || response.data
      profileUser.value = data

      // If navigated via UUID and user has a publicId, silently replace URL
      if (data?.public_id && data.public_id !== publicIdParam) {
        router.replace({ name: 'public-profile', params: { publicId: data.public_id } })
      }
    }

    await fetchWatchlist(isOwn)
  } catch (err: unknown) {
    console.error('Failed to load profile:', err)
    if ((err as ApiError).response?.status === 404) {
      error.value = t('profile.userNotFound')
    } else {
      error.value = t('profile.profileLoadError')
    }
  } finally {
    loading.value = false
  }
}

// Track current profile ownership for fetchWatchlistPage
const _isOwnProfile = ref(false)

// Public profile status counts
const publicStatusCounts = ref<Record<string, number>>({})

const fetchPublicStatusCounts = async (userId: string, statuses: string[]) => {
  const promises = statuses.map(async (status) => {
    try {
      const response = await publicApi.getPublicWatchlist(userId, {
        status,
        page: 1,
        per_page: 1,
      })
      return { status, count: response.data?.meta?.total_count || 0 }
    } catch {
      return { status, count: 0 }
    }
  })
  const results = await Promise.all(promises)
  const counts: Record<string, number> = {}
  for (const { status, count } of results) {
    counts[status] = count
  }
  publicStatusCounts.value = counts
}

const fetchWatchlistPage = async (backgroundRefresh = false) => {
  if (!profileUser.value) return

  const currentFilter = watchlistFilter.value
  const currentPage = watchlistPage.value
  const key = pageCacheKey(currentFilter, currentPage)

  // Show cached data immediately if available
  const cached = pageCache.get(key)
  if (cached && !backgroundRefresh) {
    watchlist.value = cached.data
    watchlistTotalPages.value = cached.totalPages
    watchlistTotalCount.value = cached.totalCount
    // If cache is fresh enough, just do a background refresh
    if (Date.now() - cached.fetchedAt < PAGE_CACHE_TTL) {
      fetchWatchlistPage(true) // background refresh, no await
      return
    }
  }

  if (!backgroundRefresh) {
    watchlistPageLoading.value = true
    // Only show the full-tab spinner when there is genuinely nothing to display.
    // For refetches triggered by search / sort / status change there is already a
    // list on screen — replacing it with a spinner unmounts the search input,
    // costs focus, and feels like a page reload on every keystroke. Use the
    // subtle overlay (`watchlistPageLoading`) for those cases instead.
    if (currentPage === 1 && !cached && watchlist.value.length === 0) {
      loadingWatchlist.value = true
    }
  }

  try {
    const statusParam = currentFilter === 'all' ? undefined : currentFilter

    let data: any[] = []
    let meta: any

    const trimmedQuery = searchQuery.value.trim()

    if (_isOwnProfile.value) {
      const response = await userApi.getWatchlist({
        page: currentPage,
        per_page: watchlistPerPage,
        ...(statusParam && { status: statusParam }),
        sort: backendSortKey.value,
        order: sortDirection.value,
        ...(trimmedQuery && { q: trimmedQuery }),
        ...filterParams(filterState.value),
      })
      data = response.data?.data || response.data || []
      meta = response.data?.meta
    } else {
      const userId = profileUser.value.id
      if (!userId) return
      const visibleStatuses = profileUser.value.public_statuses || []
      const response = await publicApi.getPublicWatchlist(userId, {
        page: currentPage,
        per_page: watchlistPerPage,
        ...(statusParam && { status: statusParam }),
        ...(visibleStatuses.length && !statusParam && { statuses: visibleStatuses.join(',') }),
        sort: backendSortKey.value,
        order: sortDirection.value,
        ...(trimmedQuery && { q: trimmedQuery }),
        ...filterParams(filterState.value),
      })
      data = response.data?.data || response.data || []
      meta = response.data?.meta
    }

    const newData = Array.isArray(data) ? data : []
    const newTotalPages = meta?.total_pages || 0
    const newTotalCount = meta?.total_count || 0

    // Store in cache
    pageCache.set(key, {
      data: newData,
      totalPages: newTotalPages,
      totalCount: newTotalCount,
      fetchedAt: Date.now(),
    })

    // Race condition guard: only update state if filter/page/query hasn't changed while we were fetching
    if (
      watchlistFilter.value === currentFilter &&
      watchlistPage.value === currentPage &&
      searchQuery.value.trim() === trimmedQuery
    ) {
      watchlist.value = newData
      watchlistTotalPages.value = newTotalPages
      watchlistTotalCount.value = newTotalCount
    }
  } catch (err) {
    console.error('Failed to fetch watchlist:', err)
  } finally {
    if (!backgroundRefresh) {
      watchlistPageLoading.value = false
      loadingWatchlist.value = false
    }
  }
}

async function loadFacets() {
  try {
    const resp = _isOwnProfile.value
      ? await userApi.getWatchlistFacets()
      : await publicApi.getPublicWatchlistFacets(profileUser.value!.id)
    facets.value = resp.data?.data || resp.data || { genres: [], kinds: [], years: { min: null, max: null } }
  } catch {
    facets.value = { genres: [], kinds: [], years: { min: null, max: null } }
  }
}

const fetchWatchlist = async (isOwn: boolean) => {
  _isOwnProfile.value = isOwn
  _watchlistInitialized.value = false
  // Own profile defaults to "watching" tab (most actionable for the owner).
  // Public profile defaults to "all" so visitors see the full picture.
  // This means sorting (e.g. by score) produces different results because
  // the underlying datasets differ — own shows one status, public shows all.
  watchlistFilter.value = isOwn ? 'watching' : 'all'
  // Someone else's profile defaults to score-first (best-rated on top); the
  // own profile keeps the persisted sort preference. Applied here (while
  // _watchlistInitialized is false) so the sort watchers don't double-fetch.
  if (isOwn) {
    sortKey.value = localStorage.getItem('profile_sortKey') || 'title'
    sortDirection.value = (localStorage.getItem('profile_sortDir') as 'asc' | 'desc') || 'asc'
  } else {
    sortKey.value = 'score'
    sortDirection.value = 'desc'
  }
  if (isOwn) {
    // Also fetch lightweight statuses for badge map and per-status counts
    await watchlistStore.fetchStatuses(true)
  }
  // Fetch public stats and status counts in background (fire and forget)
  if (!isOwn && profileUser.value) {
    const visibleStatuses = profileUser.value.public_statuses || []
    // Fetch aggregate stats (avg score, total episodes)
    publicApi.getPublicWatchlistStats(profileUser.value.id, visibleStatuses.length > 0 ? visibleStatuses : undefined)
      .then(r => { publicWatchlistStats.value = r.data?.data || r.data || null })
      .catch(() => {})
    if (visibleStatuses.length > 0) {
      fetchPublicStatusCounts(profileUser.value.id, visibleStatuses)
    }
  }
  // Reset filters so switching profiles starts clean, and load this profile's
  // facet options (fire-and-forget — loadFacets handles its own errors).
  filterState.value = { ...EMPTY_FILTER_STATE }
  loadFacets()
  watchlistPage.value = 1
  await fetchWatchlistPage()
  _watchlistInitialized.value = true

  // Prefetch page 1 of all other status tabs in background for instant switching.
  // Skip when a search query is active — searching is short-lived and we don't
  // want to pollute the cache with status-specific pages that are already
  // search-filtered (they'd be wrong as soon as the user clears the query).
  if (!searchQuery.value.trim()) {
    const allStatuses = isOwn
      ? ['all', 'watching', 'plan_to_watch', 'completed', 'on_hold', 'dropped']
      : ['all', ...(profileUser.value?.public_statuses || [])]
    const currentStatus = watchlistFilter.value
    for (const status of allStatuses) {
      if (status === currentStatus) continue
      // Fire and forget — populate the cache silently
      const key = pageCacheKey(status, 1)
      if (!pageCache.has(key)) {
        const statusParam = status === 'all' ? undefined : status
        if (isOwn) {
          userApi.getWatchlist({ page: 1, per_page: watchlistPerPage, ...(statusParam && { status: statusParam }), sort: backendSortKey.value, order: sortDirection.value })
            .then(r => { pageCache.set(key, { data: r.data?.data || [], totalPages: r.data?.meta?.total_pages || 0, totalCount: r.data?.meta?.total_count || 0, fetchedAt: Date.now() }) })
            .catch(() => {})
        } else if (profileUser.value?.id) {
          const ps = profileUser.value.public_statuses || []
          publicApi.getPublicWatchlist(profileUser.value.id, { page: 1, per_page: watchlistPerPage, ...(statusParam && { status: statusParam }), ...(!statusParam && ps.length && { statuses: ps.join(',') }), sort: backendSortKey.value, order: sortDirection.value })
            .then(r => { pageCache.set(key, { data: r.data?.data || [], totalPages: r.data?.meta?.total_pages || 0, totalCount: r.data?.meta?.total_count || 0, fetchedAt: Date.now() }) })
            .catch(() => {})
        }
      }
    }
  }
}

// Clear bulk selection whenever the visible page / filter / search changes.
watch([watchlistPage, watchlistFilter, searchQuery, filterState], () => {
  selectedIds.value = new Set()
}, { deep: true })

// Server-side pagination watchers (defined after fetchWatchlistPage)
watch(watchlistFilter, () => {
  if (!_watchlistInitialized.value) return
  watchlistPage.value = 1
  fetchWatchlistPage()
})

// Re-fetch when sort key or direction changes (server-side sorting)
watch([sortKey, sortDirection], () => {
  if (!_watchlistInitialized.value) return
  watchlistPage.value = 1
  fetchWatchlistPage()
})

// Re-fetch when watchlist filters (genres / kinds / year range) change
watch(filterState, () => {
  watchlistPage.value = 1
  fetchWatchlistPage()
}, { deep: true })

// Re-fetch when search query changes (server-side search across full watchlist).
// Debounced so we don't hammer the API on every keystroke.
const debouncedSearchRefetch = useDebounceFn(() => {
  if (!_watchlistInitialized.value) return
  watchlistPage.value = 1
  fetchWatchlistPage()
}, 300)
watch(searchQuery, () => {
  debouncedSearchRefetch()
})

const updateAnimeStatus = async (animeId: string, newStatus: string) => {
  const animeRow = watchlist.value.find(a => a.anime_id === animeId)
  const priorStatus = animeRow?.status ?? null
  // Optimistic: flip the visible row status immediately.
  if (animeRow) animeRow.status = newStatus
  clearPageCache()
  try {
    await watchlistStore.setStatusOptimistic(animeId, newStatus)
  } catch (err) {
    console.error('Failed to update status:', err)
    if (animeRow && priorStatus !== null) animeRow.status = priorStatus
    toast.push(t('watchlist.errors.updateFailed'))
  }
}

const updateAnimeDate = async (animeId: string, field: 'started_at' | 'completed_at', value: string) => {
  try {
    const anime = watchlist.value.find(a => a.anime_id === animeId)
    if (!anime) return

    const dateValue = value ? new Date(value).toISOString() : null
    await userApi.updateWatchlistEntry({
      anime_id: animeId,
      status: anime.status,
      [field]: dateValue,
    })

    // Update local state
    anime[field] = dateValue
    clearPageCache()
    watchlistStore.invalidate()
  } catch (err) {
    console.error('Failed to update date:', err)
  }
}

// Phase 13 / UX-27 — debounced score commit: rapid blur/re-edit cycles
// collapse to a single PUT 500 ms after the last edit settles.
const debouncedCommitScore = useDebounceFn(async (animeId: string, score: number, priorScore: number) => {
  try {
    await watchlistStore.setScoreOptimistic(animeId, score)
  } catch (err) {
    console.error('Failed to update score:', err)
    const row = watchlist.value.find(a => a.anime_id === animeId)
    if (row) row.score = priorScore
    toast.push(t('watchlist.errors.updateFailed'))
  }
}, 500)

const finishEditScore = async (animeId: string, rawValue: string) => {
  const score = Math.max(0, Math.min(10, parseInt(rawValue) || 0))
  const anime = watchlist.value.find(a => a.anime_id === animeId)
  if (!anime) return

  const priorScore = anime.score ?? 0
  // Optimistic: flip the visible score immediately.
  anime.score = score
  clearPageCache()
  // API fires on debounced settle.
  void debouncedCommitScore(animeId, score, priorScore)
}

const updateAnimeEpisodes = async (animeId: string, episodes: number) => {
  const anime = watchlist.value.find(a => a.anime_id === animeId)
  if (!anime) return

  const maxEp = animeTotalEpisodes(anime) || 9999
  const clamped = Math.max(0, Math.min(maxEp, episodes))

  try {
    await userApi.updateWatchlistEntry({
      anime_id: animeId,
      status: anime.status,
      episodes: clamped,
    })
    anime.episodes = clamped
    clearPageCache()
    watchlistStore.invalidate()
  } catch (err) {
    console.error('Failed to update episodes:', err)
  }
}

const updateAnimeRewatchCount = async (animeId: string, count: number) => {
  const anime = watchlist.value.find(a => a.anime_id === animeId)
  if (!anime) return

  const clamped = Math.max(0, count)

  try {
    await userApi.updateWatchlistEntry({
      anime_id: animeId,
      status: anime.status,
      rewatch_count: clamped,
    })
    anime.rewatch_count = clamped
    clearPageCache()
    watchlistStore.invalidate()
  } catch (err) {
    console.error('Failed to update rewatch count:', err)
  }
}

const removeFromWatchlist = async (animeId: string) => {
  const priorRow = watchlist.value.find(a => a.anime_id === animeId)
  const priorIdx = priorRow ? watchlist.value.indexOf(priorRow) : -1
  // Optimistic: drop the row from the visible list immediately.
  watchlist.value = watchlist.value.filter(a => a.anime_id !== animeId)
  clearPageCache()
  try {
    await watchlistStore.removeEntryOptimistic(animeId)
  } catch (err) {
    console.error('Failed to remove from watchlist:', err)
    // Re-insert at original position on failure.
    if (priorRow) {
      const restoreAt = Math.min(priorIdx, watchlist.value.length)
      watchlist.value.splice(restoreAt >= 0 ? restoreAt : 0, 0, priorRow)
    }
    toast.push(t('watchlist.errors.removeFailed'))
  }
}

// --- Bulk actions ---
async function bulkSetStatus(status: string) {
  const ids = [...selectedIds.value]
  if (!ids.length) return
  for (const a of watchlist.value) if (selectedIds.value.has(a.anime_id)) a.status = status
  clearPageCache()
  try {
    await userApi.bulkWatchlist({ anime_ids: ids, action: 'set_status', status })
    exitSelectionMode()
    await fetchWatchlistPage()
  } catch (err) {
    console.error('bulk set status failed', err)
    toast.push(t('profile.bulk.error'))
    await fetchWatchlistPage()
  }
}

async function bulkRemove() {
  const ids = [...selectedIds.value]
  if (!ids.length) return
  if (!(await confirm({
    title: t('common.confirmTitle'),
    description: t('profile.bulk.removeConfirm', { n: ids.length }),
    confirmText: t('common.confirm'),
    cancelText: t('common.cancel'),
    variant: 'destructive',
  }))) return
  watchlist.value = watchlist.value.filter((a) => !selectedIds.value.has(a.anime_id))
  clearPageCache()
  try {
    await userApi.bulkWatchlist({ anime_ids: ids, action: 'remove' })
    exitSelectionMode()
    await fetchWatchlistPage()
  } catch (err) {
    console.error('bulk remove failed', err)
    toast.push(t('profile.bulk.error'))
    await fetchWatchlistPage()
  }
}

// Grid context-menu handlers. AnimeContextMenu already performs the store
// mutation + API call itself, so these only sync Profile's local `watchlist`
// mirror for an immediate re-render. We must NOT refetch here: the menu emits
// these events *before* awaiting its own network call, so a refetch would race
// ahead of the pending mutation and repopulate the just-changed row (the cause
// of "deleted anime only disappears after reload"). We also must NOT call the
// store/API again or the DELETE/PATCH would double-fire.
const onContextMenuRemove = (animeId: string) => {
  watchlist.value = watchlist.value.filter(a => a.anime_id !== animeId)
  clearPageCache()
}

const onContextMenuStatusChange = (animeId: string, status: string) => {
  const row = watchlist.value.find(a => a.anime_id === animeId)
  if (row) row.status = status
  // When a specific status tab is active, an item whose status no longer
  // matches the filter should drop out of the visible list.
  if (watchlistFilter.value !== 'all' && status !== watchlistFilter.value) {
    watchlist.value = watchlist.value.filter(a => a.anime_id !== animeId)
  }
  clearPageCache()
}

const onContextMenuEpisodesChange = (animeId: string, episodes: number) => {
  const row = watchlist.value.find(a => a.anime_id === animeId)
  if (row) row.episodes = episodes
  clearPageCache()
}

const copyProfileLink = async () => {
  const link = profileUser.value?.public_id
    ? `${siteOrigin}/user/${profileUser.value.public_id}`
    : window.location.href
  try {
    await navigator.clipboard.writeText(link)
    copied.value = true
    setTimeout(() => { copied.value = false }, 2000)
  } catch (err) {
    console.error('Failed to copy:', err)
  }
}

const getSyncState = (source: 'mal' | 'shikimori') =>
  source === 'mal' ? malSync : shikimoriSync

const startPolling = (source: 'mal' | 'shikimori', jobId: string) => {
  stopPolling(source)
  const state = getSyncState(source)

  pollIntervals[source] = setInterval(async () => {
    try {
      const resp = await userApi.getImportJobStatus(jobId)
      const data = resp.data?.data || resp.data
      state.value.progress = {
        total: data.total,
        imported: data.imported,
        skipped: data.skipped,
        status: data.status,
      }

      if (data.status === 'completed' || data.status === 'failed') {
        stopPolling(source)
        state.value.importing = false
        if (data.status === 'completed') {
          state.value.lastSync = {
            completed_at: data.completed_at,
            imported: data.imported,
            skipped: data.skipped,
          }
          await fetchWatchlist(true)
        } else {
          state.value.error = data.error_message || t('profile.import.errors.generic', { source })
        }
      }
    } catch {
      stopPolling(source)
      state.value.importing = false
    }
  }, 2000)
}

const stopPolling = (source: 'mal' | 'shikimori') => {
  if (pollIntervals[source]) {
    clearInterval(pollIntervals[source]!)
    pollIntervals[source] = null
  }
}

// Localized import-card error message (pure helper in composables/profile).
const importErrorMessage = (err: ApiError, source: 'mal' | 'shikimori'): string =>
  buildImportErrorMessage(err, source, t, te)

const startImport = async (source: 'mal' | 'shikimori') => {
  const state = getSyncState(source)
  const username = source === 'mal' ? malUsername.value : shikimoriNickname.value
  if (!username) return

  state.value.importing = true
  state.value.progress = null
  state.value.error = null

  try {
    const response = source === 'mal'
      ? await userApi.importMAL(username)
      : await userApi.importShikimori(username)
    const data = response.data?.data || response.data

    state.value.jobId = data.job_id
    state.value.progress = {
      total: data.total,
      imported: 0,
      skipped: 0,
      status: 'processing',
    }

    startPolling(source, data.job_id)
  } catch (err: unknown) {
    state.value.error = importErrorMessage(err as ApiError, source)
    state.value.importing = false
  }
}

const importMAL = () => startImport('mal')
const importShikimori = () => startImport('shikimori')

const exportToJSON = async () => {
  exportingJSON.value = true
  exportError.value = null
  try {
    const resp = await userApi.exportJSON()
    const blob = new Blob([resp.data], { type: 'application/json' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `animeenigma-export-${new Date().toISOString().slice(0, 10)}.json`
    document.body.appendChild(a)
    a.click()
    document.body.removeChild(a)
    URL.revokeObjectURL(url)
  } catch (err: unknown) {
    const axiosErr = err as { response?: { status?: number } }
    if (axiosErr?.response?.status === 429) {
      exportError.value = t('profile.export.rateLimited')
    } else {
      exportError.value = t('profile.export.error')
    }
  } finally {
    exportingJSON.value = false
  }
}

const checkActiveSyncJobs = async () => {
  try {
    const resp = await userApi.getSyncStatus()
    const data = resp.data?.data || resp.data
    if (!data) return

    for (const source of ['mal', 'shikimori'] as const) {
      const sourceData = data[source]
      if (!sourceData) continue
      const state = getSyncState(source)

      if (sourceData.last_sync) {
        state.value.lastSync = {
          completed_at: sourceData.last_sync.completed_at,
          imported: sourceData.last_sync.imported,
          skipped: sourceData.last_sync.skipped,
        }
      }

      if (sourceData.active) {
        state.value.jobId = sourceData.active.id
        state.value.importing = true
        state.value.progress = {
          total: sourceData.active.total,
          imported: sourceData.active.imported,
          skipped: sourceData.active.skipped,
          status: sourceData.active.status,
        }
        startPolling(source, sourceData.active.id)
      }
    }
  } catch {
    // Ignore — non-critical
  }
}

// Relative-time string for the import "last synced" line (pure helper).
const timeAgo = (dateStr: string): string => formatTimeAgo(dateStr, t)

const savePublicId = async () => {
  if (!publicId.value) return

  const validPattern = /^[a-zA-Z0-9-]{3,32}$/
  if (!validPattern.test(publicId.value)) {
    publicIdError.value = t('profile.linkValidation')
    return
  }

  savingPublicId.value = true
  publicIdError.value = null
  publicIdSuccess.value = false

  try {
    await userApi.updatePublicId(publicId.value)
    publicIdSuccess.value = true
    await authStore.fetchUser()
    // Navigate to the new profile URL so route.params.publicId matches the
    // updated public_id. Without this the page stays on /user/<oldId>, fetchProfile
    // sees isOwn=false (store has the new id, URL still has the old), and renders an
    // empty public profile for the now-unassigned old id (the "empty profile until
    // re-login" bug). The route.params.publicId watcher re-runs fetchProfile.
    if (route.params.publicId && route.params.publicId !== publicId.value) {
      router.replace(`/user/${publicId.value}`)
    }
    setTimeout(() => { publicIdSuccess.value = false }, 3000)
  } catch (err: unknown) {
    const apiErr = err as ApiError
    const errBody = apiErr.response?.data?.error
    const errBodyMsg = typeof errBody === 'string'
      ? errBody
      : errBody?.message
    const message = apiErr.response?.data?.message || errBodyMsg
    if (message?.includes('already taken') || message?.includes('уже занят')) {
      publicIdError.value = t('profile.linkTaken')
    } else {
      publicIdError.value = message || t('profile.saveFailed')
    }
  } finally {
    savingPublicId.value = false
  }
}

const togglePublicStatus = (status: string) => {
  const index = publicStatuses.value.indexOf(status)
  if (index === -1) {
    publicStatuses.value.push(status)
  } else {
    publicStatuses.value.splice(index, 1)
  }
}

const savePrivacy = async () => {
  savingPrivacy.value = true
  privacySuccess.value = false

  try {
    await userApi.updatePrivacy(publicStatuses.value)
    await userApi.updateActivityVisibility(activityVisibility.value)
    privacySuccess.value = true
    await authStore.fetchUser()
    setTimeout(() => { privacySuccess.value = false }, 3000)
  } catch (err: unknown) {
    console.error('Failed to save privacy:', err)
  } finally {
    savingPrivacy.value = false
  }
}

// Avatar upload — pure 256x256 center-crop resize lives in composables/profile;
// here we only wire the resulting data URL into the preview refs.
const handleAvatarFile = (e: Event) => {
  const input = e.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file) return

  void resizeAvatarToDataUrl(file).then((dataUrl) => {
    if (!dataUrl) return // > 2 MB — silently ignored, matching prior behaviour
    avatarPreview.value = dataUrl
    avatarDataUrl.value = dataUrl
  })
  // Reset input so same file can be re-selected
  input.value = ''
}

const uploadAvatar = async () => {
  if (!avatarDataUrl.value) return

  uploadingAvatar.value = true
  try {
    await userApi.updateAvatar(avatarDataUrl.value)
    await authStore.fetchUser()
    if (profileUser.value) {
      profileUser.value.avatar = avatarDataUrl.value
    }
    showAvatarModal.value = false
    avatarPreview.value = null
    avatarDataUrl.value = null
  } catch (err) {
    console.error('Failed to upload avatar:', err)
  } finally {
    uploadingAvatar.value = false
  }
}

// API Key methods
const fetchApiKeyStatus = async () => {
  apiKeyLoading.value = true
  try {
    const res = await userApi.hasApiKey()
    hasApiKey.value = res.data?.data?.has_api_key ?? false
  } catch {
    hasApiKey.value = false
  } finally {
    apiKeyLoading.value = false
  }
}

const generateApiKey = async () => {
  apiKeyActioning.value = true
  apiKeyError.value = null
  apiKeyRevoked.value = false
  generatedApiKey.value = null
  try {
    const res = await userApi.generateApiKey()
    generatedApiKey.value = res.data?.data?.api_key ?? null
    hasApiKey.value = true
  } catch (e: unknown) {
    apiKeyError.value = (e as ApiKeyError).response?.data?.error?.message || 'Failed to generate API key'
  } finally {
    apiKeyActioning.value = false
  }
}

const regenerateApiKey = async () => {
  await generateApiKey()
}

const revokeApiKey = async () => {
  if (!(await confirm({
    title: t('common.confirmTitle'),
    description: t('profile.settings.apiKeyRevokeConfirm'),
    confirmText: t('common.confirm'),
    cancelText: t('common.cancel'),
    variant: 'destructive',
  }))) return
  apiKeyActioning.value = true
  apiKeyError.value = null
  generatedApiKey.value = null
  try {
    await userApi.revokeApiKey()
    hasApiKey.value = false
    apiKeyRevoked.value = true
  } catch (e: unknown) {
    apiKeyError.value = (e as ApiKeyError).response?.data?.error?.message || 'Failed to revoke API key'
  } finally {
    apiKeyActioning.value = false
  }
}

const copyApiKey = async () => {
  if (!generatedApiKey.value) return
  try {
    await navigator.clipboard.writeText(generatedApiKey.value)
    apiKeyCopied.value = true
    setTimeout(() => { apiKeyCopied.value = false }, 2000)
  } catch {
    // Fallback
  }
}

const logout = () => {
  authStore.logout()
  router.push('/')
}

// Watch route changes to reload profile
watch(() => route.params.publicId, (newId) => {
  if (newId) {
    fetchProfile()
  }
})

// UA-068 (UX-04 Phase 2): inject the username into <title> once the profile
// is loaded, so /user/:public_id renders e.g. "ui_audit_bot — AnimeEnigma"
// instead of the static "Профиль - AnimeEnigma" fallback set by the router
// guard.
watch(() => profileUser.value?.username, (newUsername) => {
  if (newUsername) {
    document.title = `${newUsername} — AnimeEnigma`
  }
})

// Focus score input when editing starts (grid popover)
watch(editingScoreGrid, (id) => {
  if (id) {
    nextTick(() => {
      const input = document.querySelector('input[type="number"][min="0"][max="10"]') as HTMLInputElement
      input?.focus()
      input?.select()
    })
  }
})

onMounted(async () => {
  await fetchProfile()
  if (isOwnProfile.value) {
    checkActiveSyncJobs()
    fetchApiKeyStatus()
  }
})
onUnmounted(() => {
  stopPolling('mal')
  stopPolling('shikimori')
})
</script>

<style scoped>
.scrollbar-hide::-webkit-scrollbar {
  display: none;
}
.scrollbar-hide {
  -ms-overflow-style: none;
  scrollbar-width: none;
}
.line-clamp-2 {
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
/* Hide number input spin buttons */
input[type="number"]::-webkit-inner-spin-button,
input[type="number"]::-webkit-outer-spin-button {
  -webkit-appearance: none;
  margin: 0;
}
input[type="number"] {
  -moz-appearance: textfield;
}
</style>
