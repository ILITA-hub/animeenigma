<template>
  <div class="min-h-screen">
    <!-- Loading State -->
    <div v-if="loading" class="flex justify-center items-center min-h-screen">
      <svg class="w-12 h-12 animate-spin text-cyan-400" fill="none" viewBox="0 0 24 24">
        <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
        <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
      </svg>
    </div>

    <!-- Error State -->
    <div v-else-if="error" class="flex flex-col items-center justify-center min-h-screen px-4">
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
                      <td class="py-3 pr-2 text-white/40">{{ index + 1 }}</td>
                      <td class="py-3 px-2">
                        <router-link :to="`/anime/${anime.anime_id}`" class="block w-12 h-16 rounded overflow-hidden bg-surface">
                          <img
                            v-if="anime.anime_cover"
                            :src="anime.anime_cover"
                            :alt="anime.anime_title"
                            class="w-full h-full object-cover"
                          />
                        </router-link>
                      </td>
                      <td class="py-3 px-2">
                        <router-link :to="`/anime/${anime.anime_id}`" class="text-white hover:text-cyan-400 transition-colors font-medium">
                          {{ anime.anime_title }}
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
                            :max="anime.anime_total_episodes || 9999"
                            @blur="(e) => updateAnimeEpisodes(anime.anime_id, parseInt((e.target as HTMLInputElement).value) || 0)"
                            @keydown.enter="(e) => (e.target as HTMLInputElement).blur()"
                            class="w-10 h-6 text-center text-xs bg-white/10 border border-white/10 rounded text-white focus:outline-none focus:border-cyan-500"
                          />
                          <span class="text-white/40">/</span>
                          <span class="text-white/60">{{ anime.anime_total_episodes || '?' }}</span>
                          <button
                            @click="updateAnimeEpisodes(anime.anime_id, (anime.episodes || 0) + 1)"
                            class="w-6 h-6 rounded flex items-center justify-center bg-white/10 text-white/60 hover:bg-white/20 hover:text-white transition-colors"
                            :disabled="anime.anime_total_episodes ? (anime.episodes || 0) >= anime.anime_total_episodes : false"
                          >+</button>
                        </div>
                        <div v-else class="flex items-center gap-1">
                          <span class="text-white">{{ anime.episodes || 0 }}</span>
                          <span class="text-white/40">/</span>
                          <span class="text-white/60">{{ anime.anime_total_episodes || '?' }}</span>
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
                        v-if="anime.anime_cover"
                        :src="anime.anime_cover"
                        :alt="anime.anime_title"
                        class="w-full h-full object-cover group-hover:scale-105 transition-transform duration-300"
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
                      {{ anime.anime_title }}
                    </h3>
                  </router-link>
                  <div class="flex items-center gap-1 mt-1">
                    <p class="text-xs text-white/50">
                      {{ anime.episodes || 0 }} / {{ anime.anime_total_episodes || '?' }} {{ $t('profile.ep') }}
                    </p>
                    <button
                      v-if="isOwnProfile"
                      @click="updateAnimeEpisodes(anime.anime_id, (anime.episodes || 0) + 1)"
                      class="w-5 h-5 rounded flex items-center justify-center bg-white/10 text-white/40 hover:bg-cyan-500/20 hover:text-cyan-400 transition-colors opacity-0 group-hover:opacity-100 text-xs"
                      :disabled="anime.anime_total_episodes ? (anime.episodes || 0) >= anime.anime_total_episodes : false"
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

          <!-- History Tab (own profile only) -->
          <template v-if="isOwnProfile" #history>
            <div v-if="history.length > 0" class="space-y-3">
              <div
                v-for="item in history"
                :key="item.id"
                class="flex gap-4 p-4 rounded-xl bg-white/5 border border-white/10 hover:bg-white/10 transition-colors"
              >
                <router-link :to="`/anime/${item.animeId}`" class="flex-shrink-0 w-16 aspect-[2/3] rounded-lg overflow-hidden">
                  <img :src="item.coverImage" :alt="item.title" class="w-full h-full object-cover" />
                </router-link>
                <div class="flex-1 min-w-0">
                  <router-link :to="`/anime/${item.animeId}`" class="font-medium text-white hover:text-cyan-400 transition-colors">
                    {{ item.title }}
                  </router-link>
                  <p class="text-white/50 text-sm">{{ $t('profile.history.episode') }} {{ item.episode }}</p>
                  <div class="mt-2 h-1 bg-white/10 rounded-full overflow-hidden">
                    <div class="h-full bg-cyan-400" :style="{ width: `${item.progress}%` }" />
                  </div>
                </div>
                <div class="text-right text-sm text-white/40">
                  {{ item.watchedAt }}
                </div>
              </div>
            </div>
            <div v-else class="text-center py-12">
              <p class="text-white/50">{{ $t('profile.history.empty') }}</p>
            </div>
          </template>

          <!-- Settings Tab (own profile only) -->
          <template v-if="isOwnProfile" #settings>
            <div class="space-y-6">
              <!-- Appearance -->
              <div class="glass-card p-6">
                <h3 class="text-lg font-semibold text-white mb-4">{{ $t('profile.settings.appearance') }}</h3>
                <div class="space-y-4">
                  <div class="flex items-center justify-between">
                    <div>
                      <p class="text-white">{{ $t('profile.settings.language') }}</p>
                      <p class="text-white/50 text-sm">{{ $t('profile.settings.languageDesc') }}</p>
                    </div>
                    <div class="w-32">
                      <Select
                        v-model="settings.language"
                        :options="languageOptions"
                        size="sm"
                      />
                    </div>
                  </div>
                  <div class="flex items-center justify-between">
                    <div>
                      <p class="text-white">{{ $t('profile.settings.reduceMotion') }}</p>
                      <p class="text-white/50 text-sm">{{ $t('profile.settings.reduceMotionDesc') }}</p>
                    </div>
                    <button
                      class="w-12 h-7 rounded-full transition-colors relative"
                      :class="settings.reduceMotion ? 'bg-cyan-500' : 'bg-white/20'"
                      @click="settings.reduceMotion = !settings.reduceMotion"
                    >
                      <span
                        class="absolute top-1 w-5 h-5 rounded-full bg-white transition-transform"
                        :class="settings.reduceMotion ? 'left-6' : 'left-1'"
                      />
                    </button>
                  </div>
                </div>
              </div>

              <!-- Playback -->
              <div class="glass-card p-6">
                <h3 class="text-lg font-semibold text-white mb-4">{{ $t('profile.settings.playback') }}</h3>
                <div class="space-y-4">
                  <div class="flex items-center justify-between">
                    <div>
                      <p class="text-white">{{ $t('profile.settings.autoplay') }}</p>
                    </div>
                    <button
                      class="w-12 h-7 rounded-full transition-colors relative"
                      :class="settings.autoplay ? 'bg-cyan-500' : 'bg-white/20'"
                      @click="settings.autoplay = !settings.autoplay"
                    >
                      <span
                        class="absolute top-1 w-5 h-5 rounded-full bg-white transition-transform"
                        :class="settings.autoplay ? 'left-6' : 'left-1'"
                      />
                    </button>
                  </div>
                  <div class="flex items-center justify-between">
                    <div>
                      <p class="text-white">{{ $t('profile.settings.defaultQuality') }}</p>
                    </div>
                    <div class="w-28">
                      <Select
                        v-model="settings.defaultQuality"
                        :options="qualityOptions"
                        size="sm"
                      />
                    </div>
                  </div>
                </div>
              </div>

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
                        :disabled="malImporting"
                      />
                      <Button
                        variant="primary"
                        :disabled="!malUsername || malImporting"
                        @click="importMAL"
                      >
                        <svg v-if="malImporting" class="w-4 h-4 animate-spin mr-2" fill="none" viewBox="0 0 24 24">
                          <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                          <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                        </svg>
                        {{ malImporting ? $t('profile.import.importing') : $t('profile.import.import') }}
                      </Button>
                    </div>
                    <p class="text-white/40 text-xs mt-2">
                      {{ $t('profile.import.malDescription') }}
                    </p>
                    <div v-if="malImportResult" class="mt-3 p-3 rounded-lg" :class="malImportResult.errors?.length ? 'bg-amber-500/20' : 'bg-green-500/20'">
                      <p class="text-sm" :class="malImportResult.errors?.length ? 'text-amber-400' : 'text-green-400'">
                        {{ $t('profile.import.imported') }}: {{ malImportResult.imported }} | {{ $t('profile.import.skipped') }}: {{ malImportResult.skipped }}
                      </p>
                    </div>
                    <div v-if="malImportError" class="mt-3 p-3 rounded-lg bg-pink-500/20">
                      <p class="text-sm text-pink-400">{{ malImportError }}</p>
                    </div>
                  </div>
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
import { ref, computed, reactive, onMounted, watch, nextTick } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '@/stores/auth'
import { Badge, Button, Modal, Tabs, Select, type SelectOption } from '@/components/ui'
import { userApi, publicApi } from '@/api/client'

interface WatchlistEntry {
  anime_id: string
  anime_title: string
  anime_cover: string
  status: string
  score: number
  episodes: number
  anime_total_episodes: number
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

interface HistoryItem {
  id: string
  animeId: string
  title: string
  coverImage: string
  episode: number
  progress: number
  watchedAt: string
}

const router = useRouter()
const route = useRoute()
const { t, locale } = useI18n()
const authStore = useAuthStore()

const siteOrigin = window.location.origin

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
      { value: 'history', label: t('profile.tabs.history') },
      { value: 'settings', label: t('profile.tabs.settings') }
    )
  }
  return baseTabs
})

// Watchlist
const watchlist = ref<WatchlistEntry[]>([])
const loadingWatchlist = ref(false)
const watchlistFilter = ref('all')
const viewMode = ref<'table' | 'grid'>('grid')
const history = ref<HistoryItem[]>([])

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

  return statuses.map(status => ({
    value: status,
    label: statusLabels.value[status] || status,
    count: status === 'all'
      ? watchlist.value.length
      : watchlist.value.filter(a => a.status === status).length
  }))
})

const filteredWatchlist = computed(() => {
  let list = watchlistFilter.value === 'all'
    ? [...watchlist.value]
    : watchlist.value.filter(a => a.status === watchlistFilter.value)

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
        cmp = (a.anime_title || '').localeCompare(b.anime_title || '')
        break
    }
    return sortDirection.value === 'desc' ? -cmp : cmp
  })

  return list
})

// Stats
const watchlistStats = computed(() => {
  const list = watchlist.value
  const scored = list.filter(a => a.score && a.score > 0)
  const avgScore = scored.length > 0
    ? (scored.reduce((sum, a) => sum + a.score, 0) / scored.length).toFixed(1)
    : '-'
  return {
    total: list.length,
    avgScore,
    totalEpisodes: list.reduce((sum, a) => sum + (a.episodes || 0), 0),
    completed: list.filter(a => a.status === 'completed').length,
  }
})

const userInitials = computed(() => {
  if (!profileUser.value?.username) return '?'
  return profileUser.value.username.slice(0, 2).toUpperCase()
})

// Settings (own profile only)
const settings = reactive({
  language: locale.value,
  reduceMotion: false,
  autoplay: false,
  defaultQuality: 'auto',
})

const languageOptions: SelectOption[] = [
  { value: 'ru', label: 'Русский' },
  { value: 'ja', label: '日本語' },
  { value: 'en', label: 'English' },
]

const qualityOptions: SelectOption[] = [
  { value: 'auto', label: 'Auto' },
  { value: '1080p', label: '1080p' },
  { value: '720p', label: '720p' },
  { value: '480p', label: '480p' },
]

// MAL Import
const malUsername = ref('')
const malImporting = ref(false)
const malImportResult = ref<{ imported: number; skipped: number; errors?: string[] } | null>(null)
const malImportError = ref<string | null>(null)

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
    }

    await fetchWatchlist(isOwn)
  } catch (err: any) {
    console.error('Failed to load profile:', err)
    if (err.response?.status === 404) {
      error.value = t('profile.userNotFound')
    } else {
      error.value = t('profile.profileLoadError')
    }
  } finally {
    loading.value = false
  }
}

const fetchWatchlist = async (isOwn: boolean) => {
  if (!profileUser.value) return

  loadingWatchlist.value = true
  try {
    if (isOwn) {
      // Fetch own watchlist
      const response = await userApi.getWatchlist()
      const entries = response.data?.data || response.data || []
      watchlist.value = entries.map((entry: any) => ({
        anime_id: entry.anime_id,
        anime_title: entry.anime_title || `Anime ${entry.anime_id}`,
        anime_cover: entry.anime_cover || '',
        status: entry.status,
        score: entry.score,
        episodes: entry.episodes,
        anime_total_episodes: entry.anime_total_episodes,
        started_at: entry.started_at,
        completed_at: entry.completed_at,
      }))
    } else {
      // Fetch public watchlist
      const userId = profileUser.value.id
      if (!userId) {
        console.error('No user ID for public watchlist')
        return
      }
      const response = await publicApi.getPublicWatchlist(
        userId,
        profileUser.value.public_statuses || []
      )
      watchlist.value = response.data?.data || response.data || []
    }
  } catch (err) {
    console.error('Failed to fetch watchlist:', err)
  } finally {
    loadingWatchlist.value = false
  }
}

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
  } catch (err) {
    console.error('Failed to update score:', err)
  }
}

const updateAnimeEpisodes = async (animeId: string, episodes: number) => {
  const anime = watchlist.value.find(a => a.anime_id === animeId)
  if (!anime) return

  const maxEp = anime.anime_total_episodes || 9999
  const clamped = Math.max(0, Math.min(maxEp, episodes))

  try {
    await userApi.updateWatchlistEntry({
      anime_id: animeId,
      status: anime.status,
      episodes: clamped,
    })
    anime.episodes = clamped
  } catch (err) {
    console.error('Failed to update episodes:', err)
  }
}

const removeFromWatchlist = async (animeId: string) => {
  try {
    await userApi.removeFromWatchlist(animeId)
    watchlist.value = watchlist.value.filter(a => a.anime_id !== animeId)
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

const importMAL = async () => {
  if (!malUsername.value) return

  malImporting.value = true
  malImportResult.value = null
  malImportError.value = null

  try {
    const response = await userApi.importMAL(malUsername.value)
    malImportResult.value = response.data?.data || response.data
    await fetchWatchlist(true)
  } catch (err: any) {
    malImportError.value = err.response?.data?.message || 'Failed to import list'
  } finally {
    malImporting.value = false
  }
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
  } catch (err: any) {
    const message = err.response?.data?.message || err.response?.data?.error
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
  } catch (err: any) {
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

onMounted(fetchProfile)
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
