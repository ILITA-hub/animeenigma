<template>
  <div class="min-h-screen">
    <!-- Loading State -->
    <div v-if="loading" class="flex justify-center items-center min-h-screen pt-20">
      <svg class="w-12 h-12 animate-spin text-cyan-400" fill="none" viewBox="0 0 24 24">
        <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
        <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
      </svg>
    </div>

    <!-- Error State -->
    <div v-else-if="error" class="flex flex-col items-center justify-center min-h-screen pt-20 px-4">
      <svg class="w-16 h-16 text-white/20 mb-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
      </svg>
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
            <div class="relative">
              <div class="w-28 h-28 sm:w-32 sm:h-32 rounded-full overflow-hidden ring-4 ring-cyan-500/30 bg-surface">
                <img
                  v-if="profileUser.avatar"
                  :src="profileUser.avatar"
                  :alt="profileUser.username"
                  class="w-full h-full object-cover"
                />
                <div
                  v-else
                  class="w-full h-full flex items-center justify-center text-4xl font-bold text-cyan-400 bg-cyan-500/10"
                >
                  {{ userInitials }}
                </div>
              </div>
              <button
                v-if="isOwnProfile"
                @click="showAvatarModal = true"
                class="absolute bottom-0 right-0 w-8 h-8 rounded-full bg-cyan-500 flex items-center justify-center text-white shadow-lg hover:bg-cyan-400 transition-colors"
              >
                <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15.232 5.232l3.536 3.536m-2.036-5.036a2.5 2.5 0 113.536 3.536L6.5 21.036H3v-3.572L16.732 3.732z" />
                </svg>
              </button>
            </div>

            <!-- User Info -->
            <div class="text-center sm:text-left flex-1">
              <h1 class="text-2xl sm:text-3xl font-bold text-white mb-1">
                {{ profileUser.username }}
              </h1>
              <div class="flex flex-wrap items-center justify-center sm:justify-start gap-2">
                <Badge v-if="isOwnProfile" variant="primary" size="sm">{{ profileUser.role || 'Member' }}</Badge>
                <span class="text-white/40 text-sm">
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
                class="flex items-center gap-2 px-4 py-2 rounded-lg bg-cyan-500/10 border border-cyan-500/20 text-cyan-400 hover:bg-cyan-500/20 transition-colors"
              >
                <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8.684 13.342C8.886 12.938 9 12.482 9 12c0-.482-.114-.938-.316-1.342m0 2.684a3 3 0 110-2.684m0 2.684l6.632 3.316m-6.632-6l6.632-3.316m0 0a3 3 0 105.367-2.684 3 3 0 00-5.367 2.684zm0 9.316a3 3 0 105.368 2.684 3 3 0 00-5.368-2.684z" />
                </svg>
                <span>{{ copied ? $t('profile.copied') : $t('profile.share') }}</span>
              </button>
            </div>
          </div>
        </div>
      </div>

      <!-- Tabs -->
      <div class="max-w-4xl mx-auto px-4">
        <Tabs v-model="activeTab" :tabs="tabs" variant="underline" full-width class="mb-6">
          <!-- Watchlist Tab -->
          <template #watchlist>
            <!-- Loading -->
            <div v-if="loadingWatchlist" class="flex justify-center py-12">
              <svg class="w-8 h-8 animate-spin text-cyan-400" fill="none" viewBox="0 0 24 24">
                <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
              </svg>
            </div>

            <div v-else-if="watchlist.length > 0" class="space-y-4">
              <!-- Stats Bar -->
              <div class="grid grid-cols-2 sm:grid-cols-4 gap-3">
                <div class="glass-card p-3 text-center">
                  <div class="text-2xl font-bold text-cyan-400">{{ watchlistStats.total }}</div>
                  <div class="text-xs text-white/50">{{ $t('profile.stats.totalAnime') }}</div>
                </div>
                <div class="glass-card p-3 text-center">
                  <div class="text-2xl font-bold text-yellow-400">{{ watchlistStats.avgScore }}</div>
                  <div class="text-xs text-white/50">{{ $t('profile.stats.avgScore') }}</div>
                </div>
                <div class="glass-card p-3 text-center">
                  <div class="text-2xl font-bold text-green-400">{{ watchlistStats.totalEpisodes }}</div>
                  <div class="text-xs text-white/50">{{ $t('profile.stats.episodesWatched') }}</div>
                </div>
                <div class="glass-card p-3 text-center">
                  <div class="text-2xl font-bold text-blue-400">{{ watchlistStats.completed }}</div>
                  <div class="text-xs text-white/50">{{ $t('profile.stats.completed') }}</div>
                </div>
              </div>

              <!-- Filter Pills -->
              <div class="flex gap-2 overflow-x-auto pb-2 scrollbar-hide">
                <button
                  v-for="filter in watchlistFilters"
                  :key="filter.value"
                  class="flex-shrink-0 px-4 py-2 rounded-full text-sm font-medium transition-colors"
                  :class="watchlistFilter === filter.value
                    ? 'bg-cyan-500/20 text-cyan-400 border border-cyan-500/30'
                    : 'bg-white/5 text-white/60 border border-transparent hover:text-white'"
                  @click="watchlistFilter = filter.value"
                >
                  {{ filter.label }}
                  <span class="ml-1 opacity-60">({{ filter.count }})</span>
                </button>
              </div>

              <!-- View Toggle + Sort -->
              <div class="flex items-center justify-end gap-2">
                <input
                  v-model="searchQuery"
                  type="text"
                  :placeholder="$t('profile.watchlist.searchPlaceholder')"
                  class="flex-shrink-0 w-48 px-3 py-2 rounded-full text-sm bg-white/5 text-white/80 border border-transparent focus:border-cyan-500/30 focus:outline-none placeholder-white/40 mr-auto"
                >
                <!-- Sort -->
                <div class="w-36">
                  <Select
                    v-model="sortKey"
                    :options="sortOptions"
                    size="sm"
                  />
                </div>
                <button
                  class="p-2 rounded-lg bg-white/5 text-white/60 hover:text-white transition-colors"
                  @click="sortDirection = sortDirection === 'asc' ? 'desc' : 'asc'"
                  :title="sortDirection === 'asc' ? 'Ascending' : 'Descending'"
                >
                  <svg class="w-5 h-5 transition-transform" :class="sortDirection === 'desc' ? 'rotate-180' : ''" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 4h13M3 8h9m-9 4h6m4 0l4-4m0 0l4 4m-4-4v12" />
                  </svg>
                </button>
                <button
                  class="p-2 rounded-lg transition-colors"
                  :class="viewMode === 'table' ? 'bg-cyan-500/20 text-cyan-400' : 'bg-white/5 text-white/60 hover:text-white'"
                  @click="viewMode = 'table'"
                  title="Table view"
                >
                  <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6h16M4 10h16M4 14h16M4 18h16" />
                  </svg>
                </button>
                <button
                  class="p-2 rounded-lg transition-colors"
                  :class="viewMode === 'grid' ? 'bg-cyan-500/20 text-cyan-400' : 'bg-white/5 text-white/60 hover:text-white'"
                  @click="viewMode = 'grid'"
                  title="Grid view"
                >
                  <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2H6a2 2 0 01-2-2V6zM14 6a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2h-2a2 2 0 01-2-2V6zM4 16a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2H6a2 2 0 01-2-2v-2zM14 16a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2h-2a2 2 0 01-2-2v-2z" />
                  </svg>
                </button>
              </div>

              <!-- Table/Grid Content with Loading Overlay -->
              <div class="relative">
              <div v-if="watchlistPageLoading && watchlist.length > 0" class="absolute inset-0 bg-black/30 backdrop-blur-sm z-10 flex items-center justify-center rounded-lg">
                <svg class="w-8 h-8 animate-spin text-cyan-400" fill="none" viewBox="0 0 24 24">
                  <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                  <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                </svg>
              </div>

              <!-- Table View -->
              <div v-if="viewMode === 'table'" class="overflow-x-auto">
                <table class="w-full text-sm">
                  <thead>
                    <tr class="text-left text-white/60 border-b border-white/10">
                      <th class="pb-3 pr-2 w-8">#</th>
                      <th class="pb-3 px-2 w-16">{{ $t('profile.table.poster') }}</th>
                      <th class="pb-3 px-2">{{ $t('profile.table.title') }}</th>
                      <th class="pb-3 px-2 w-16 text-center">{{ $t('profile.table.score') }}</th>
                      <th class="pb-3 px-2 w-32">{{ $t('profile.table.progress') }}</th>
                      <th class="pb-3 px-2 w-28 text-center hidden sm:table-cell">{{ $t('profile.table.startDate') }}</th>
                      <th class="pb-3 px-2 w-28 text-center hidden sm:table-cell">{{ $t('profile.table.endDate') }}</th>
                      <th class="pb-3 pl-2 w-32 text-center">{{ $t('profile.table.status') }}</th>
                      <th v-if="isOwnProfile" class="pb-3 pl-2 w-10"></th>
                    </tr>
                  </thead>
                  <tbody>
                    <tr
                      v-for="(anime, index) in filteredWatchlist"
                      :key="anime.anime_id"
                      class="border-b border-white/5 hover:bg-white/5 transition-colors"
                    >
                      <td class="py-3 pr-2 text-white/40">{{ (watchlistPage - 1) * watchlistPerPage + index + 1 }}</td>
                      <td class="py-3 px-2">
                        <router-link :to="`/anime/${anime.anime_id}`" class="block w-12 h-16 rounded overflow-hidden bg-surface">
                          <img
                            v-if="animeCover(anime)"
                            :src="animeCover(anime)"
                            :alt="animeTitle(anime)"
                            class="w-full h-full object-cover"
                            @error="(e: Event) => { const img = e.target as HTMLImageElement; if (!img.dataset.fallback) { img.dataset.fallback = '1'; img.src = getImageFallbackUrl(anime.anime?.poster_url || '') } }"
                          />
                        </router-link>
                      </td>
                      <td class="py-3 px-2">
                        <router-link :to="`/anime/${anime.anime_id}`" class="text-white hover:text-cyan-400 transition-colors font-medium">
                          {{ animeTitle(anime) }}
                        </router-link>
                      </td>
                      <!-- Score (inline edit) -->
                      <td class="py-3 px-2 text-center">
                        <template v-if="isOwnProfile">
                          <input
                            v-if="editingScore === anime.anime_id"
                            type="number"
                            min="0"
                            max="10"
                            :value="anime.score || 0"
                            @blur="(e) => { finishEditScore(anime.anime_id, (e.target as HTMLInputElement).value); }"
                            @keydown.enter="(e) => { (e.target as HTMLInputElement).blur(); }"
                            @keydown.escape="editingScore = null"
                            class="w-12 h-8 text-center bg-white/10 border border-cyan-500/50 rounded text-cyan-400 font-bold focus:outline-none focus:ring-1 focus:ring-cyan-500"
                            ref="scoreInputRef"
                          />
                          <button
                            v-else
                            @click="editingScore = anime.anime_id"
                            class="inline-flex items-center justify-center w-8 h-8 rounded-full transition-colors cursor-pointer"
                            :class="anime.score && anime.score > 0 ? 'bg-cyan-500/20 text-cyan-400 font-bold hover:bg-cyan-500/30' : 'text-white/30 hover:bg-white/10 hover:text-white/60'"
                          >
                            {{ anime.score && anime.score > 0 ? anime.score : '-' }}
                          </button>
                        </template>
                        <template v-else>
                          <span v-if="anime.score && anime.score > 0" class="inline-flex items-center justify-center w-8 h-8 rounded-full bg-cyan-500/20 text-cyan-400 font-bold">
                            {{ anime.score }}
                          </span>
                          <span v-else class="text-white/30">-</span>
                        </template>
                      </td>
                      <!-- Progress (inline edit) -->
                      <td class="py-3 px-2">
                        <div v-if="isOwnProfile" class="flex items-center gap-1">
                          <button
                            @click="updateAnimeEpisodes(anime.anime_id, (anime.episodes || 0) - 1)"
                            class="w-6 h-6 rounded flex items-center justify-center bg-white/10 text-white/60 hover:bg-white/20 hover:text-white transition-colors"
                            :disabled="(anime.episodes || 0) <= 0"
                          >-</button>
                          <input
                            type="number"
                            :value="anime.episodes || 0"
                            min="0"
                            :max="animeTotalEpisodes(anime) || 9999"
                            @blur="(e) => updateAnimeEpisodes(anime.anime_id, parseInt((e.target as HTMLInputElement).value) || 0)"
                            @keydown.enter="(e) => (e.target as HTMLInputElement).blur()"
                            class="w-10 h-6 text-center text-xs bg-white/10 border border-white/10 rounded text-white focus:outline-none focus:border-cyan-500"
                          />
                          <span class="text-white/40">/</span>
                          <span class="text-white/60">{{ animeTotalEpisodes(anime) || '?' }}</span>
                          <button
                            @click="updateAnimeEpisodes(anime.anime_id, (anime.episodes || 0) + 1)"
                            class="w-6 h-6 rounded flex items-center justify-center bg-white/10 text-white/60 hover:bg-white/20 hover:text-white transition-colors"
                            :disabled="animeTotalEpisodes(anime) ? (anime.episodes || 0) >= animeTotalEpisodes(anime) : false"
                          >+</button>
                        </div>
                        <div v-else class="flex items-center gap-1">
                          <span class="text-white">{{ anime.episodes || 0 }}</span>
                          <span class="text-white/40">/</span>
                          <span class="text-white/60">{{ animeTotalEpisodes(anime) || '?' }}</span>
                        </div>
                      </td>
                      <td class="py-3 px-2 text-center hidden sm:table-cell">
                        <input
                          v-if="isOwnProfile"
                          type="date"
                          :value="formatDateForInput(anime.started_at)"
                          @change="(e) => updateAnimeDate(anime.anime_id, 'started_at', (e.target as HTMLInputElement).value)"
                          class="bg-white/10 border border-white/10 rounded px-2 py-1 text-white text-xs w-full focus:outline-none focus:border-cyan-500"
                        />
                        <span v-else class="text-white/60 text-xs">
                          {{ formatDateDisplay(anime.started_at) }}
                        </span>
                      </td>
                      <td class="py-3 px-2 text-center hidden sm:table-cell">
                        <input
                          v-if="isOwnProfile"
                          type="date"
                          :value="formatDateForInput(anime.completed_at)"
                          @change="(e) => updateAnimeDate(anime.anime_id, 'completed_at', (e.target as HTMLInputElement).value)"
                          class="bg-white/10 border border-white/10 rounded px-2 py-1 text-white text-xs w-full focus:outline-none focus:border-cyan-500"
                        />
                        <span v-else class="text-white/60 text-xs">
                          {{ formatDateDisplay(anime.completed_at) }}
                        </span>
                      </td>
                      <td class="py-3 pl-2">
                        <div v-if="isOwnProfile" class="w-28">
                          <Select
                            :model-value="anime.status"
                            :options="statusOptions"
                            size="xs"
                            @change="(val: string | number) => updateAnimeStatus(anime.anime_id, String(val))"
                          />
                        </div>
                        <span v-else class="text-xs px-2 py-1 rounded-full" :class="statusColors[anime.status]">
                          {{ statusLabels[anime.status] }}
                        </span>
                      </td>
                      <!-- Remove button -->
                      <td v-if="isOwnProfile" class="py-3 pl-2">
                        <button
                          @click="removeFromWatchlist(anime.anime_id)"
                          class="p-1.5 rounded hover:bg-red-500/20 text-white/30 hover:text-red-400 transition-colors"
                          :title="$t('profile.actions.removeFromList')"
                        >
                          <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
                          </svg>
                        </button>
                      </td>
                    </tr>
                  </tbody>
                </table>
              </div>

              <!-- Grid View -->
              <div v-else class="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 gap-4">
                <div
                  v-for="anime in filteredWatchlist"
                  :key="anime.anime_id"
                  class="relative group"
                >
                  <router-link :to="`/anime/${anime.anime_id}`" class="block">
                    <div class="aspect-[2/3] rounded-xl overflow-hidden bg-surface relative">
                      <img
                        v-if="animeCover(anime)"
                        :src="animeCover(anime)"
                        :alt="animeTitle(anime)"
                        class="w-full h-full object-cover group-hover:scale-105 transition-transform duration-300"
                        @error="(e: Event) => { const img = e.target as HTMLImageElement; if (!img.dataset.fallback) { img.dataset.fallback = '1'; img.src = getImageFallbackUrl(anime.anime?.poster_url || '') } }"
                      />
                      <div v-else class="w-full h-full flex items-center justify-center text-white/20">
                        <svg class="w-12 h-12" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M4 16l4.586-4.586a2 2 0 012.828 0L16 16m-2-2l1.586-1.586a2 2 0 012.828 0L20 14m-6-6h.01M6 20h12a2 2 0 002-2V6a2 2 0 00-2-2H6a2 2 0 00-2 2v12a2 2 0 002 2z" />
                        </svg>
                      </div>

                      <!-- Score Badge -->
                      <div
                        v-if="anime.score && anime.score > 0"
                        class="absolute top-2 right-2 px-2 py-1 rounded bg-black/60 text-yellow-400 text-sm font-bold"
                        :class="{ 'cursor-pointer hover:bg-black/80': isOwnProfile }"
                        @click.prevent="isOwnProfile && (editingScoreGrid = anime.anime_id)"
                      >
                        {{ anime.score }}
                      </div>
                      <!-- Score edit popover for grid -->
                      <div
                        v-if="isOwnProfile && editingScoreGrid === anime.anime_id"
                        class="absolute top-2 right-2 z-20"
                        @click.prevent.stop
                      >
                        <input
                          type="number"
                          min="0"
                          max="10"
                          :value="anime.score || 0"
                          @blur="(e) => { finishEditScore(anime.anime_id, (e.target as HTMLInputElement).value); editingScoreGrid = null; }"
                          @keydown.enter="(e) => (e.target as HTMLInputElement).blur()"
                          @keydown.escape="editingScoreGrid = null"
                          class="w-14 h-8 text-center bg-black/80 border border-cyan-500/50 rounded text-yellow-400 font-bold text-sm focus:outline-none focus:ring-1 focus:ring-cyan-500"
                        />
                      </div>

                      <!-- Status Badge -->
                      <div class="absolute bottom-0 left-0 right-0 p-2 bg-gradient-to-t from-black/80 to-transparent">
                        <span class="text-xs px-2 py-0.5 rounded-full" :class="statusColors[anime.status]">
                          {{ statusLabels[anime.status] }}
                        </span>
                      </div>
                    </div>
                    <h3 class="mt-2 text-sm font-medium text-white line-clamp-2 group-hover:text-cyan-400 transition-colors">
                      {{ animeTitle(anime) }}
                    </h3>
                  </router-link>
                  <div class="flex items-center gap-1 mt-1">
                    <p class="text-xs text-white/50">
                      {{ anime.episodes || 0 }} / {{ animeTotalEpisodes(anime) || '?' }} {{ $t('profile.ep') }}
                    </p>
                    <button
                      v-if="isOwnProfile"
                      @click="updateAnimeEpisodes(anime.anime_id, (anime.episodes || 0) + 1)"
                      class="w-5 h-5 rounded flex items-center justify-center bg-white/10 text-white/40 hover:bg-cyan-500/20 hover:text-cyan-400 transition-colors opacity-0 group-hover:opacity-100 text-xs"
                      :disabled="animeTotalEpisodes(anime) ? (anime.episodes || 0) >= animeTotalEpisodes(anime) : false"
                    >+</button>
                  </div>
                  <p v-if="anime.started_at || anime.completed_at" class="text-xs text-white/40 mt-0.5">
                    <span v-if="anime.started_at">{{ formatDateDisplay(anime.started_at) }}</span>
                    <span v-if="anime.started_at && anime.completed_at"> - </span>
                    <span v-if="anime.completed_at">{{ formatDateDisplay(anime.completed_at) }}</span>
                  </p>

                  <!-- Quick actions for own profile -->
                  <template v-if="isOwnProfile">
                    <div class="absolute top-2 left-2 opacity-0 group-hover:opacity-100 transition-opacity z-10" @click.stop>
                      <div class="w-24">
                        <Select
                          :model-value="anime.status"
                          :options="statusOptions"
                          size="xs"
                          @change="(val: string | number) => updateAnimeStatus(anime.anime_id, String(val))"
                        />
                      </div>
                    </div>
                  </template>
                </div>
              </div>

              </div><!-- end relative wrapper -->

              <!-- Pagination -->
              <PaginationBar
                :current-page="watchlistPage"
                :total-pages="watchlistTotalPages"
                @update:current-page="(p: number) => { watchlistPage = p; fetchWatchlistPage() }"
              />
            </div>

            <div v-else class="text-center py-12">
              <svg class="w-16 h-16 mx-auto text-white/20 mb-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10" />
              </svg>
              <p class="text-white/50 mb-4">{{ isOwnProfile ? $t('profile.empty.watchlist') : $t('profile.listEmpty') }}</p>
              <Button v-if="isOwnProfile" variant="outline" @click="$router.push('/browse')">
                {{ $t('profile.actions.browseCatalog') }}
              </Button>
            </div>
          </template>

          <!-- Settings Tab (own profile only) -->
          <template v-if="isOwnProfile" #settings>
            <div class="space-y-6">
              <!-- Import -->
              <div class="glass-card p-6">
                <h3 class="text-lg font-semibold text-white mb-4">{{ $t('profile.import.title') }}</h3>
                <div class="space-y-4">
                  <div>
                    <label class="block text-white/60 text-sm mb-2">MyAnimeList</label>
                    <div class="flex gap-2">
                      <input
                        v-model="malUsername"
                        type="text"
                        :placeholder="$t('profile.import.malPlaceholder')"
                        class="flex-1 bg-white/10 border border-white/10 rounded-lg px-4 py-2 text-white placeholder-white/40 focus:outline-none focus:border-cyan-500"
                        :disabled="malSync.importing"
                      />
                      <Button
                        variant="primary"
                        :disabled="!malUsername || malSync.importing"
                        @click="importMAL"
                      >
                        <svg v-if="malSync.importing" class="w-4 h-4 animate-spin mr-2" fill="none" viewBox="0 0 24 24">
                          <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                          <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                        </svg>
                        {{ malSync.importing ? $t('profile.import.importing') : $t('profile.import.import') }}
                      </Button>
                    </div>
                    <p class="text-white/40 text-xs mt-2">
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
                        <span v-if="malSync.progress.status === 'completed'" class="text-green-400 ml-2">
                          {{ $t('profile.import.imported') }}: {{ malSync.progress.imported }} | {{ $t('profile.import.skipped') }}: {{ malSync.progress.skipped }}
                        </span>
                      </p>
                    </div>
                    <div v-if="malSync.lastSync && !malSync.progress" class="mt-2">
                      <p class="text-xs text-white/40">
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
                      <input
                        v-model="shikimoriNickname"
                        type="text"
                        :placeholder="$t('profile.import.shikimoriPlaceholder')"
                        class="flex-1 bg-white/10 border border-white/10 rounded-lg px-4 py-2 text-white placeholder-white/40 focus:outline-none focus:border-cyan-500"
                        :disabled="shikimoriSync.importing"
                      />
                      <Button
                        variant="primary"
                        :disabled="!shikimoriNickname || shikimoriSync.importing"
                        @click="importShikimori"
                      >
                        <svg v-if="shikimoriSync.importing" class="w-4 h-4 animate-spin mr-2" fill="none" viewBox="0 0 24 24">
                          <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                          <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                        </svg>
                        {{ shikimoriSync.importing ? $t('profile.import.importing') : $t('profile.import.import') }}
                      </Button>
                    </div>
                    <p class="text-white/40 text-xs mt-2">
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
                        <span v-if="shikimoriSync.progress.status === 'completed'" class="text-green-400 ml-2">
                          {{ $t('profile.import.imported') }}: {{ shikimoriSync.progress.imported }} | {{ $t('profile.import.skipped') }}: {{ shikimoriSync.progress.skipped }}
                        </span>
                      </p>
                    </div>
                    <div v-if="shikimoriSync.lastSync && !shikimoriSync.progress" class="mt-2">
                      <p class="text-xs text-white/40">
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
                <h3 class="text-lg font-semibold text-white mb-4">{{ $t('profile.export.title') }}</h3>
                <p class="text-white/60 text-sm mb-4">{{ $t('profile.export.description') }}</p>
                <Button
                  variant="primary"
                  :disabled="exportingJSON"
                  @click="exportToJSON"
                >
                  <svg v-if="exportingJSON" class="w-4 h-4 animate-spin mr-2" fill="none" viewBox="0 0 24 24">
                    <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                    <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                  </svg>
                  <svg v-else class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4" />
                  </svg>
                  {{ exportingJSON ? $t('profile.export.exporting') : $t('profile.export.button') }}
                </Button>
                <div v-if="exportError" class="mt-3 p-3 rounded-lg bg-pink-500/20">
                  <p class="text-sm text-pink-400">{{ exportError }}</p>
                </div>
              </div>

              <!-- Public Profile -->
              <div class="glass-card p-6">
                <h3 class="text-lg font-semibold text-white mb-4">{{ $t('profile.publicProfile') }}</h3>
                <div class="space-y-4">
                  <!-- Public ID -->
                  <div>
                    <label class="block text-white/60 text-sm mb-2">{{ $t('profile.profileLink') }}</label>
                    <div class="flex gap-2">
                      <div class="flex-1 flex items-center bg-white/10 border border-white/10 rounded-lg overflow-hidden">
                        <span class="px-3 text-white/40 text-sm">/user/</span>
                        <input
                          v-model="publicId"
                          type="text"
                          placeholder="your-username"
                          class="flex-1 bg-transparent py-2 pr-3 text-white placeholder-white/40 focus:outline-none"
                          :disabled="savingPublicId"
                        />
                      </div>
                      <Button
                        variant="primary"
                        :disabled="!publicId || savingPublicId || publicId === authStore.user?.public_id"
                        @click="savePublicId"
                      >
                        <svg v-if="savingPublicId" class="w-4 h-4 animate-spin" fill="none" viewBox="0 0 24 24">
                          <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                          <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                        </svg>
                        <span v-else>{{ $t('profile.save') }}</span>
                      </Button>
                    </div>
                    <p v-if="publicIdError" class="text-pink-400 text-xs mt-2">{{ publicIdError }}</p>
                    <p v-else-if="publicIdSuccess" class="text-green-400 text-xs mt-2">{{ $t('profile.linkUpdated') }}</p>
                    <p class="text-white/40 text-xs mt-2">
                      {{ $t('profile.linkValidation') }}
                    </p>
                  </div>

                  <!-- Public Link -->
                  <div v-if="authStore.user?.public_id" class="flex items-center gap-2 p-3 bg-white/5 rounded-lg">
                    <svg class="w-5 h-5 text-cyan-400 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13.828 10.172a4 4 0 00-5.656 0l-4 4a4 4 0 105.656 5.656l1.102-1.101m-.758-4.899a4 4 0 005.656 0l4-4a4 4 0 00-5.656-5.656l-1.1 1.1" />
                    </svg>
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
                      <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z" />
                      </svg>
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
                        <input
                          type="checkbox"
                          :checked="publicStatuses.includes(status.value)"
                          @change="togglePublicStatus(status.value)"
                          class="w-4 h-4 rounded border-white/20 bg-white/10 text-cyan-500 focus:ring-cyan-500 focus:ring-offset-0"
                        />
                        <span class="text-white">{{ status.label }}</span>
                      </label>
                    </div>
                    <div class="mt-3">
                      <Button
                        variant="outline"
                        size="sm"
                        :disabled="savingPrivacy"
                        @click="savePrivacy"
                      >
                        <svg v-if="savingPrivacy" class="w-4 h-4 animate-spin mr-2" fill="none" viewBox="0 0 24 24">
                          <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                          <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                        </svg>
                        {{ $t('profile.savePrivacy') }}
                      </Button>
                      <p v-if="privacySuccess" class="text-green-400 text-xs mt-2">{{ $t('profile.privacySaved') }}</p>
                    </div>
                  </div>
                </div>
              </div>

              <!-- API Key -->
              <div class="glass-card p-6">
                <h3 class="text-lg font-semibold text-white mb-4">{{ $t('profile.settings.apiKey') }}</h3>
                <p class="text-white/60 text-sm mb-4">{{ $t('profile.settings.apiKeyDescription') }}</p>

                <!-- Loading state -->
                <div v-if="apiKeyLoading" class="flex justify-center py-4">
                  <svg class="w-6 h-6 animate-spin text-cyan-400" fill="none" viewBox="0 0 24 24">
                    <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                    <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                  </svg>
                </div>

                <template v-else>
                  <!-- Show generated key (once) -->
                  <div v-if="generatedApiKey" class="space-y-3">
                    <p class="text-sm text-yellow-400 font-medium">{{ $t('profile.settings.apiKeyGenerated') }}</p>
                    <div class="flex items-center gap-2 p-3 bg-white/5 rounded-lg font-mono text-sm text-white break-all">
                      <span class="flex-1">{{ generatedApiKey }}</span>
                      <button
                        @click="copyApiKey"
                        class="flex-shrink-0 p-1.5 rounded hover:bg-white/10 text-white/60 hover:text-white transition-colors"
                      >
                        <svg v-if="apiKeyCopied" class="w-4 h-4 text-green-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
                        </svg>
                        <svg v-else class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z" />
                        </svg>
                      </button>
                    </div>
                    <p v-if="apiKeyCopied" class="text-green-400 text-xs">{{ $t('profile.settings.apiKeyCopied') }}</p>
                    <div class="p-3 bg-white/5 rounded-lg">
                      <p class="text-white/40 text-xs mb-1">{{ $t('profile.settings.apiKeyUsageHint') }}</p>
                      <code class="text-xs text-cyan-400 break-all">curl -H "Authorization: Bearer {{ generatedApiKey }}" {{ siteOrigin }}/api/users/import/mal -d '{"username":"..."}'</code>
                    </div>
                  </div>

                  <!-- Has key state -->
                  <div v-else-if="hasApiKey" class="space-y-3">
                    <p class="text-sm text-green-400">{{ $t('profile.settings.apiKeyHasKey') }}</p>
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
                    <p class="text-sm text-white/40">{{ $t('profile.settings.apiKeyNone') }}</p>
                    <Button variant="primary" size="sm" :disabled="apiKeyActioning" @click="generateApiKey">
                      {{ $t('profile.settings.generateApiKey') }}
                    </Button>
                  </div>

                  <div v-if="apiKeyError" class="mt-3 p-3 rounded-lg bg-pink-500/20">
                    <p class="text-sm text-pink-400">{{ apiKeyError }}</p>
                  </div>
                  <p v-if="apiKeyRevoked" class="text-green-400 text-xs mt-2">{{ $t('profile.settings.apiKeyRevoked') }}</p>
                </template>
              </div>

              <!-- Account -->
              <div class="glass-card p-6">
                <h3 class="text-lg font-semibold text-white mb-4">{{ $t('profile.settings.account') }}</h3>
                <div class="space-y-4">
                  <Button variant="ghost" full-width class="justify-start">
                    {{ $t('profile.settings.changePassword') }}
                  </Button>
                  <Button variant="secondary" full-width @click="logout">
                    {{ $t('profile.settings.signOut') }}
                  </Button>
                </div>
              </div>
            </div>
          </template>
        </Tabs>
      </div>
    </template>

    <!-- Avatar Upload Modal -->
    <Modal v-model="showAvatarModal" :title="$t('profile.avatar.title')" size="sm">
      <div class="space-y-4">
        <!-- Preview -->
        <div class="flex justify-center">
          <div class="w-40 h-40 rounded-full overflow-hidden ring-4 ring-cyan-500/30 bg-surface">
            <img
              v-if="avatarPreview"
              :src="avatarPreview"
              class="w-full h-full object-cover"
            />
            <div v-else class="w-full h-full flex items-center justify-center text-5xl font-bold text-cyan-400 bg-cyan-500/10">
              {{ userInitials }}
            </div>
          </div>
        </div>
        <!-- File Input -->
        <div class="text-center">
          <label class="inline-flex items-center gap-2 px-4 py-2 rounded-lg bg-white/10 border border-white/10 text-white hover:bg-white/20 cursor-pointer transition-colors">
            <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16l4.586-4.586a2 2 0 012.828 0L16 16m-2-2l1.586-1.586a2 2 0 012.828 0L20 14m-6-6h.01M6 20h12a2 2 0 002-2V6a2 2 0 00-2-2H6a2 2 0 00-2 2v12a2 2 0 002 2z" />
            </svg>
            {{ $t('profile.avatar.selectFile') }}
            <input
              type="file"
              accept="image/jpeg,image/png,image/webp"
              class="hidden"
              @change="handleAvatarFile"
            />
          </label>
          <p class="text-white/40 text-xs mt-2">{{ $t('profile.avatar.formats') }}</p>
        </div>
      </div>
      <template #footer>
        <Button variant="ghost" @click="showAvatarModal = false">{{ $t('common.cancel') }}</Button>
        <Button
          variant="primary"
          :disabled="!avatarPreview || uploadingAvatar"
          @click="uploadAvatar"
        >
          <svg v-if="uploadingAvatar" class="w-4 h-4 animate-spin mr-2" fill="none" viewBox="0 0 24 24">
            <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
            <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
          </svg>
          {{ uploadingAvatar ? $t('profile.avatar.uploading') : $t('profile.avatar.upload') }}
        </Button>
      </template>
    </Modal>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch, nextTick } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '@/stores/auth'
import { useWatchlistStore } from '@/stores/watchlist'
import { Badge, Button, Modal, Tabs, Select, PaginationBar, type SelectOption } from '@/components/ui'
import { userApi, publicApi } from '@/api/client'
import { getLocalizedTitle } from '@/utils/title'
import { getImageUrl, getImageFallbackUrl } from '@/composables/useImageProxy'

interface ApiError {
  response?: {
    status?: number
    data?: {
      message?: string
      error?: string
    }
  }
}

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
const { t, locale } = useI18n()
const authStore = useAuthStore()
const watchlistStore = useWatchlistStore()

const siteOrigin = window.location.origin

// Helpers for nested anime data from Preload
const animeTitle = (entry: WatchlistEntry): string =>
  getLocalizedTitle(entry.anime?.name, entry.anime?.name_ru, entry.anime?.name_jp) || 'Anime'
const animeCover = (entry: WatchlistEntry): string =>
  getImageUrl(entry.anime?.poster_url) || ''
const animeTotalEpisodes = (entry: WatchlistEntry): number =>
  entry.anime?.episodes_count || 0

const localeMap: Record<string, string> = {
  ru: 'ru-RU',
  en: 'en-US',
  ja: 'ja-JP',
}

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
const activeTab = ref('watchlist')
const tabs = computed(() => {
  const baseTabs = [
    { value: 'watchlist', label: t('profile.tabs.watchlist') },
  ]
  if (isOwnProfile.value) {
    baseTabs.push(
      { value: 'settings', label: t('profile.tabs.settings') }
    )
  }
  return baseTabs
})

// Watchlist
const watchlist = ref<WatchlistEntry[]>([])
const loadingWatchlist = ref(false)
const watchlistFilter = ref('all')
const searchQuery = ref('')
const viewMode = ref<'table' | 'grid'>('grid')

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
  return `${status}:${page}`
}
function clearPageCache() {
  pageCache.clear()
}

// Inline editing
const editingScore = ref<string | null>(null)
const editingScoreGrid = ref<string | null>(null)

// Sorting
const sortKey = ref<string>(localStorage.getItem('profile_sortKey') || 'title')
const sortDirection = ref<'asc' | 'desc'>((localStorage.getItem('profile_sortDir') as 'asc' | 'desc') || 'asc')

watch(sortKey, (v) => localStorage.setItem('profile_sortKey', v))
watch(sortDirection, (v) => localStorage.setItem('profile_sortDir', v))

const sortOptions = computed<SelectOption[]>(() => [
  { value: 'title', label: t('profile.sort.title') },
  { value: 'score', label: t('profile.sort.score') },
  { value: 'progress', label: t('profile.sort.progress') },
  { value: 'status', label: t('profile.sort.status') },
])

const statusLabels = computed<Record<string, string>>(() => ({
  all: t('profile.watchlist.all'),
  watching: t('profile.watchlist.watching'),
  completed: t('profile.watchlist.completed'),
  plan_to_watch: t('profile.watchlist.planToWatch'),
  on_hold: t('profile.watchlist.onHold'),
  dropped: t('profile.watchlist.dropped'),
}))

const statusColors: Record<string, string> = {
  watching: 'bg-green-500/80 text-white',
  completed: 'bg-blue-500/80 text-white',
  plan_to_watch: 'bg-purple-500/80 text-white',
  on_hold: 'bg-yellow-500/80 text-black',
  dropped: 'bg-red-500/80 text-white'
}

const statusOrder: Record<string, number> = {
  watching: 0,
  plan_to_watch: 1,
  completed: 2,
  on_hold: 3,
  dropped: 4,
}

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
  // Status filtering is handled server-side via fetchWatchlistPage.
  // Here we apply search, genre, and sort on the current page's data.
  let list = [...watchlist.value]

  if (searchQuery.value) {
    const q = searchQuery.value.toLowerCase()
    list = list.filter(a => {
      const name = a.anime?.name?.toLowerCase() || ''
      const nameRu = a.anime?.name_ru?.toLowerCase() || ''
      const nameJp = a.anime?.name_jp?.toLowerCase() || ''
      return name.includes(q) || nameRu.includes(q) || nameJp.includes(q)
    })
  }

  // Sort
  list.sort((a, b) => {
    let cmp = 0
    switch (sortKey.value) {
      case 'score':
        cmp = (a.score || 0) - (b.score || 0)
        break
      case 'progress':
        cmp = (a.episodes || 0) - (b.episodes || 0)
        break
      case 'status':
        cmp = (statusOrder[a.status] ?? 99) - (statusOrder[b.status] ?? 99)
        break
      case 'title':
      default:
        cmp = animeTitle(a).localeCompare(animeTitle(b))
        break
    }
    return sortDirection.value === 'desc' ? -cmp : cmp
  })

  return list
})

// Public profile aggregate stats (fetched from backend)
const publicWatchlistStats = ref<{ avg_score: number; total_episodes: number; total_entries: number; completed: number } | null>(null)

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

const userInitials = computed(() => {
  if (!profileUser.value?.username) return '?'
  return profileUser.value.username.slice(0, 2).toUpperCase()
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

// Avatar
const showAvatarModal = ref(false)
const avatarPreview = ref<string | null>(null)
const avatarDataUrl = ref<string | null>(null)
const uploadingAvatar = ref(false)

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
    if (currentPage === 1 && !cached) {
      loadingWatchlist.value = true
    }
  }

  try {
    const statusParam = currentFilter === 'all' ? undefined : currentFilter

    let data: any[] = []
    let meta: any

    if (_isOwnProfile.value) {
      const response = await userApi.getWatchlist({
        page: currentPage,
        per_page: watchlistPerPage,
        ...(statusParam && { status: statusParam }),
        sort: 'updated_at',
        order: 'desc',
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

    // Race condition guard: only update state if filter/page hasn't changed while we were fetching
    if (watchlistFilter.value === currentFilter && watchlistPage.value === currentPage) {
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

const fetchWatchlist = async (isOwn: boolean) => {
  _isOwnProfile.value = isOwn
  _watchlistInitialized.value = false
  watchlistFilter.value = isOwn ? 'watching' : 'all'
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
  watchlistPage.value = 1
  await fetchWatchlistPage()
  _watchlistInitialized.value = true

  // Prefetch page 1 of all other status tabs in background for instant switching
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
        userApi.getWatchlist({ page: 1, per_page: watchlistPerPage, ...(statusParam && { status: statusParam }), sort: 'updated_at', order: 'desc' })
          .then(r => { pageCache.set(key, { data: r.data?.data || [], totalPages: r.data?.meta?.total_pages || 0, totalCount: r.data?.meta?.total_count || 0, fetchedAt: Date.now() }) })
          .catch(() => {})
      } else if (profileUser.value?.id) {
        const ps = profileUser.value.public_statuses || []
        publicApi.getPublicWatchlist(profileUser.value.id, { page: 1, per_page: watchlistPerPage, ...(statusParam && { status: statusParam }), ...(!statusParam && ps.length && { statuses: ps.join(',') }) })
          .then(r => { pageCache.set(key, { data: r.data?.data || [], totalPages: r.data?.meta?.total_pages || 0, totalCount: r.data?.meta?.total_count || 0, fetchedAt: Date.now() }) })
          .catch(() => {})
      }
    }
  }
}

// Server-side pagination watchers (defined after fetchWatchlistPage)
watch(watchlistFilter, () => {
  if (!_watchlistInitialized.value) return
  watchlistPage.value = 1
  fetchWatchlistPage()
})

const formatDateForInput = (dateStr: string | null | undefined): string => {
  if (!dateStr) return ''
  try {
    const date = new Date(dateStr)
    return date.toISOString().split('T')[0]
  } catch {
    return ''
  }
}

const formatDateDisplay = (dateStr: string | null | undefined): string => {
  if (!dateStr) return '-'
  try {
    const date = new Date(dateStr)
    return date.toLocaleDateString(localeMap[locale.value] || 'en-US', { day: '2-digit', month: '2-digit', year: 'numeric' })
  } catch {
    return '-'
  }
}

const updateAnimeStatus = async (animeId: string, newStatus: string) => {
  try {
    await userApi.updateWatchlistStatus(animeId, newStatus)
    const anime = watchlist.value.find(a => a.anime_id === animeId)
    if (anime) {
      anime.status = newStatus
    }
    clearPageCache()
    watchlistStore.invalidate()
    await watchlistStore.fetchStatuses(true)
  } catch (err) {
    console.error('Failed to update status:', err)
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

const finishEditScore = async (animeId: string, rawValue: string) => {
  editingScore.value = null
  const score = Math.max(0, Math.min(10, parseInt(rawValue) || 0))
  const anime = watchlist.value.find(a => a.anime_id === animeId)
  if (!anime) return

  try {
    await userApi.updateWatchlistEntry({
      anime_id: animeId,
      status: anime.status,
      score,
    })
    anime.score = score
    clearPageCache()
    watchlistStore.invalidate()
  } catch (err) {
    console.error('Failed to update score:', err)
  }
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

const removeFromWatchlist = async (animeId: string) => {
  try {
    await userApi.removeFromWatchlist(animeId)
    watchlist.value = watchlist.value.filter(a => a.anime_id !== animeId)
    clearPageCache()
    watchlistStore.invalidate()
    await watchlistStore.fetchStatuses(true)
  } catch (err) {
    console.error('Failed to remove from watchlist:', err)
  }
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
          state.value.error = data.error_message || 'Import failed'
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
    state.value.error = (err as ApiError).response?.data?.message || 'Failed to import list'
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

const timeAgo = (dateStr: string): string => {
  const date = new Date(dateStr)
  if (isNaN(date.getTime())) return t('profile.import.justNow')
  const now = new Date()
  const diffMs = now.getTime() - date.getTime()
  const diffMin = Math.floor(diffMs / 60000)
  const diffHours = Math.floor(diffMs / 3600000)
  const diffDays = Math.floor(diffMs / 86400000)

  if (diffMin < 1) return t('profile.import.justNow')
  if (diffMin < 60) return t('profile.import.minutesAgo', { n: diffMin })
  if (diffHours < 24) return t('profile.import.hoursAgo', { n: diffHours })
  return t('profile.import.daysAgo', { n: diffDays })
}

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
    setTimeout(() => { publicIdSuccess.value = false }, 3000)
  } catch (err: unknown) {
    const apiErr = err as ApiError
    const message = apiErr.response?.data?.message || apiErr.response?.data?.error
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
    privacySuccess.value = true
    await authStore.fetchUser()
    setTimeout(() => { privacySuccess.value = false }, 3000)
  } catch (err: unknown) {
    console.error('Failed to save privacy:', err)
  } finally {
    savingPrivacy.value = false
  }
}

// Avatar upload
const handleAvatarFile = (e: Event) => {
  const input = e.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file) return

  if (file.size > 2 * 1024 * 1024) {
    return
  }

  const reader = new FileReader()
  reader.onload = () => {
    const img = new Image()
    img.onload = () => {
      // Resize to 256x256 center-crop
      const canvas = document.createElement('canvas')
      canvas.width = 256
      canvas.height = 256
      const ctx = canvas.getContext('2d')!

      // Center crop to square
      const size = Math.min(img.width, img.height)
      const sx = (img.width - size) / 2
      const sy = (img.height - size) / 2

      ctx.drawImage(img, sx, sy, size, size, 0, 0, 256, 256)

      const dataUrl = canvas.toDataURL('image/jpeg', 0.85)
      avatarPreview.value = dataUrl
      avatarDataUrl.value = dataUrl
    }
    img.src = reader.result as string
  }
  reader.readAsDataURL(file)
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
  if (!confirm(t('profile.settings.apiKeyRevokeConfirm'))) return
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

// Focus score input when editing starts
watch(editingScore, (id) => {
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
