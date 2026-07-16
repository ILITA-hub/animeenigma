<template>
  <div class="pl-wrap" data-test="ae-player">
    <div
      ref="rootRef"
      class="pl"
      :class="{ 'pl--theater': theater, 'pl--ui-hidden': !uiVisible, 'pl--pseudo-fs': pseudoFs }"
      :style="{ '--prov': activeProviderHue }"
      tabindex="0"
      role="region"
      :aria-label="$t('player.aePlayer.rootAria')"
      @click.self="closeMenus"
      @mouseenter="onPointerEnter"
      @mouseleave="onPointerLeave"
      @mousemove="wakeUi"
      @touchstart.passive="onRootTouch"
    >
    <!-- Poster / still background — only until playback first starts; a
         mid-episode pause must NOT bring the poster back (disruptive in
         fullscreen where object-contain letterboxing exposes it). -->
    <div
      v-if="anime.still && !hasStarted"
      class="pl-scene"
      :style="{ backgroundImage: `url(${anime.still})` }"
      aria-hidden="true"
    />
    <div class="pl-grain" aria-hidden="true" />

    <!-- Video element.
         crossorigin="anonymous" lets the subtitle auto-sync VAD read audio from
         cross-origin sources. The VAD taps a captureStream() fork
         (subtitleAudioTap.ts); on a CORS-tainted element that fork's audio is
         silent, so this attr is what makes auto-sync actually LOCK on native
         cross-origin MP4 (animejoy-sibnet/allvideo, 18anime, hanime). Safe
         because every stream is served through the HLS proxy (ACAO: *); the
         MSE/blob path is same-origin and unaffected, and the public
         exp/sig-signed proxy needs no cookies. NOTE: playback audio does NOT
         depend on this — the tap is non-interruptive — so its absence would only
         degrade VAD accuracy, never silence sound. -->
    <video
      ref="videoRef"
      class="absolute inset-0 w-full h-full object-contain z-[1]"
      playsinline
      preload="auto"
      crossorigin="anonymous"
      @play="onVideoPlay"
      @pause="onVideoPause"
      @ended="onEnded"
      @click="onVideoClick"
      @volumechange="onVolumeChange"
      @waiting="onBufferStart"
      @canplay="onBufferEnd"
      @playing="onBufferEnd"
      @seeked="onSeeked"
      @timeupdate="onTimeUpdate"
      @progress="onVideoProgress"
      @error="onVideoError"
    />

    <!-- Subtitle overlay -->
    <SubtitleOverlay
      :video-element="videoRef"
      :subtitle-url="chosenSubUrl"
      :format="chosenSubFormat"
      :visible="state.subLang.value !== 'off' && !!chosenSubUrl"
      :fullscreen-container="rootRef"
      :windowed-container="rootRef"
      :offset="effectiveOffset"
      :size-scale="state.subSize.value"
      :bg-opacity="state.subBg.value"
      @error="onSubtitleError"
    />

    <!-- Source error overlay -->
    <div
      v-if="sourceError"
      role="alert"
      aria-live="assertive"
      class="absolute inset-0 z-[2] flex items-center justify-center"
      style="background: var(--black-a80);"
    >
      <div class="flex flex-col items-center gap-3 text-center px-8">
        <CircleAlert :size="48" :stroke-width="1.5" class="text-muted-foreground" aria-hidden="true" />
        <p class="text-sm font-medium text-foreground">{{ sourceError }}</p>
        <Button variant="soft" size="sm" data-test="source-error-retry" @click="retryResolution">
          {{ $t('player.aePlayer.retry') }}
        </Button>
      </div>
    </div>

    <!-- Autoplay-blocked overlay: the browser vetoed play() (NotAllowedError).
         The stream is healthy — offer an explicit click-to-play instead of
         letting the player look dead or failing over to another source. -->
    <div
      v-if="playbackBlocked && !sourceError && !isResolving"
      role="alert"
      aria-live="assertive"
      class="absolute inset-0 z-[2] flex items-center justify-center"
      style="background: var(--black-a80);"
      @click.stop
    >
      <div class="flex flex-col items-center gap-3 text-center px-8">
        <button
          type="button"
          class="pl-blocked-play"
          data-test="autoplay-blocked-play"
          :aria-label="$t('player.aePlayer.play')"
          @click="attemptPlay"
        >
          <Play :size="34" aria-hidden="true" />
        </button>
        <p class="text-sm font-medium text-foreground">{{ $t('player.aePlayer.autoplayBlocked') }}</p>
        <p v-if="playbackBlockedHint" class="text-xs text-muted-foreground max-w-md">
          {{ $t('player.aePlayer.autoplayBlockedHint') }}
        </p>
      </div>
    </div>

    <!-- Top bar -->
    <div class="pl-top" @click.stop>
      <!-- Title block -->
      <div class="pl-title-block">
        <span class="pl-eyebrow">
          <span class="pl-eyebrow-src">
            <!-- V2b: the EP block IS the episodes-sheet trigger -->
            <button
              type="button"
              class="pl-ep-trigger"
              :aria-expanded="openMenu === 'episodes'"
              :aria-label="$t('player.aePlayer.episodeList')"
              :title="$t('player.aePlayer.episodes')"
              data-test="ep-trigger"
              @click="toggleMenu('episodes')"
            >
              {{ $t('player.aePlayer.epAbbrev') }} {{ selectedEpisode?.number ?? initialEpisode ?? 1 }}
              <span v-if="selectedEpisode?.title" class="pl-ep-title">· {{ selectedEpisode.title }}</span>
              <ChevronDown
                class="pl-ep-chev"
                :class="{ 'pl-ep-chev--open': openMenu === 'episodes' }"
                :size="12"
                :stroke-width="2.5"
                aria-hidden="true"
              />
            </button>
            <span v-if="activeProviderName" class="inline-flex items-center gap-1 ml-1">
              <span class="pl-prov-dot" :style="{ background: activeProviderHue, boxShadow: `0 0 8px ${activeProviderHue}` }" aria-hidden="true" />
              {{ activeProviderName }}
            </span>
            <span v-if="audioLabel" class="ml-1 opacity-70">· {{ audioLabel }}</span>
          </span>
        </span>
        <h1 class="pl-title">{{ anime.title }}</h1>
      </div>

      <!-- Top-right actions -->
      <div class="pl-top-right">
        <WatchTogetherButton
          v-if="showWtLaunch"
          :disabled="!currentWtSeed"
          :loading="wtLaunching"
          @launch="onLaunchWt"
        />
      </div>
    </div>

    <!-- Overlays -->

    <BigPlayButton
      :visible="!state.playing.value && !sourceError && !showBuffering && !isResolving && !playbackBlocked"
      @play="togglePlay"
    />

    <!-- Mobile play/pause affordance while playing (tap = chrome toggle on touch) -->
    <button
      v-if="isCoarse && state.playing.value && uiVisible && hasStarted && !sourceError"
      class="pl-center-pause"
      :aria-label="$t('player.aePlayer.pause')"
      data-test="center-pause"
      @click.stop="togglePlay"
    >
      <Pause :size="30" aria-hidden="true" />
    </button>

    <!-- Double-tap seek indicator -->
    <div
      v-if="seekFlash"
      class="pl-seekflash"
      :class="seekFlash === 'back' ? 'pl-seekflash--back' : 'pl-seekflash--fwd'"
      aria-hidden="true"
    >
      {{ seekFlash === 'back' ? '−10s' : '+10s' }}
    </div>

    <BufferingOverlay :visible="(showBuffering || isResolving) && !sourceError" />

    <DebugHud
      v-if="hudVisible"
      :stats="playbackStats"
      :frags="engine.fragStats.value"
      :bandwidth="engine.bandwidthEstimate.value"
      :provider="activeProviderName"
      :stream-type="currentStream?.type ?? '—'"
      :level-label="engine.currentLevelLabel.value"
      :seek="lastSeek"
      :decision="sourceDecision"
      :intents="sourceFallbackDebug.intents"
      :pinned="state.hudPinned.value"
      :fading="hudFading"
      @update:pinned="v => { state.hudPinned.value = v }"
    />

    <SkipIntroChip
      :visible="!!skipTarget"
      :label="skipTarget?.kind === 'outro' ? $t('player.aePlayer.skipOutro') : $t('player.aePlayer.skipIntro')"
      @skip="onSkipSegment"
    />

    <!-- Resume-from-saved-position chip (never auto-seeks) -->
    <div v-if="resumeChipVisible" class="pl-resume" data-test="resume-chip">
      <button class="pl-resume-go" type="button" @click="onResumeFromSaved">
        <Play :size="12" :stroke-width="2.5" aria-hidden="true" />
        <span>{{ $t('player.aePlayer.resumeFrom', { time: fmtResume(resumePosSec) }) }}</span>
      </button>
      <button
        class="pl-resume-x"
        type="button"
        :aria-label="$t('player.aePlayer.dismissResume')"
        data-test="resume-chip-dismiss"
        @click="resumeChipDismissed = true"
      >
        <X :size="12" aria-hidden="true" />
      </button>
    </div>

    <NextEpisodeCard
      v-if="showNextEpisode"
      :next-ep="nextEpisodeNumber"
      :title="anime.title"
      :still-url="anime.still"
      :countdown="nextEpCountdown"
      @play="goToNextEpisode"
      @cancel="showNextEpisode = false"
    />

    <!-- Manual "Next episode" chip — end-of-episode affordance when autoplay is
         off. Styled like (and stacked above) the Skip-Ending chip. -->
    <NextEpisodeChip
      :visible="showNextEpChip"
      :label="$t('player.aePlayer.nextEpisode')"
      @next="goToNextEpisode"
    />

    <!-- Control bar -->
    <PlayerControlBar
      :playing="state.playing.value"
      :current-time="currentTime"
      :duration="duration"
      :volume="state.volume.value"
      :muted="state.muted.value"
      :provider-name="activeProviderName"
      :provider-hue="activeProviderHue"
      :audio-label="audioLabel"
      :subs-on="subsOn"
      :episode-label="selectedEpisode?.number ?? initialEpisode ?? 1"
      :progress="state.progress.value"
      :buffered="bufferedPct"
      :chapters="chapters"
      :still-url="anime.still"
      :open-menu="openMenu"
      :fragments="fragOverlay"
      :preview-url="currentStream?.url ?? null"
      :preview-type="currentStream?.type ?? null"
      :preview-storyboard-url="currentStream?.storyboardUrl ?? null"
      :fullscreen-active="fullscreenActive"
      :theater-active="theater"
      :can-theater="canTheater"
      @toggle-play="togglePlay"
      @seek-rel="onSeekRel"
      @seek="onSeek"
      @set-volume="onSetVolume"
      @toggle-mute="onToggleMute"
      @toggle-source="toggleMenu('source')"
      @toggle-episodes="toggleMenu('episodes')"
      @toggle-subs="toggleMenu('subs')"
      @toggle-settings="toggleMenu('settings')"
      @toggle-pip="onTogglePip"
      @toggle-fullscreen="onToggleFullscreen"
      @toggle-theater="emit('toggle-theater')"
    />

    <!-- Source panel (floating card on desktop, bottom sheet on mobile) -->
    <Teleport to="body" :disabled="!sheetTeleport">
      <div
        v-if="openMenu === 'source'"
        ref="sourceMenuEl"
        class="pl-floating pl-floating--source"
        :class="{ 'pl-floating--mobile-sheet': sheetTeleport }"
        :style="{ '--prov': activeProviderHue }"
        @click.stop
      >
        <SourcePanel
          :rows="rows"
          :audio="state.combo.value.audio"
          :lang="state.combo.value.lang"
          :team="state.combo.value.team"
          :provider="state.combo.value.provider"
          :server="state.combo.value.server"
          :servers="resolvedServers"
          :teams="teams"
          :cap-map="capMap"
          :hacker-mode="state.hackerMode.value"
          :playback-error="Boolean(sourceError)"
          @update:audio="onSelectAudio"
          @update:lang="state.setLang"
          @update:team="state.setTeam"
          @select-provider="onSelectProvider"
          @select-server="state.setServer"
        />
      </div>
    </Teleport>

    <!-- Episodes sheet (V2b — bottom sheet above the control bar) -->
    <Teleport to="body" :disabled="!sheetTeleport">
      <div
        v-if="openMenu === 'episodes'"
        ref="episodesMenuEl"
        class="pl-floating pl-floating--sheet"
        :class="{ 'pl-floating--mobile-sheet': sheetTeleport }"
        :style="{ '--prov': activeProviderHue }"
        @click.stop
      >
        <EpisodesPanel
          :episodes="episodes"
          :selected-number="selectedEpisode?.number ?? null"
          :upcoming="upcomingEpisode"
          :watched-up-to="watchedUpTo"
          :progress="epProgress"
          :can-mark="auth.isAuthenticated"
          :marking="tracking.marking.value"
          :marked="selectedEpisode ? isEpisodeWatched(selectedEpisode.number) : false"
          :download-mode="downloadMode"
          :download-states="downloadStates"
          @select="onSelectEpisode"
          @mark-watched="onMarkWatched"
          @download-season="onDownloadSeason"
        />
      </div>
    </Teleport>

    <!-- Playback settings menu (floating, above control bar) -->
    <Teleport to="body" :disabled="!sheetTeleport">
      <div
        v-if="openMenu === 'settings'"
        ref="settingsMenuEl"
        class="pl-floating pl-floating--btnmenu"
        :class="{ 'pl-floating--mobile-sheet': sheetTeleport }"
        :style="{ '--prov': activeProviderHue }"
        @click.stop
      >
        <PlaybackSettingsMenu
          :quality="state.quality.value"
          :qualities="qualities"
          :quality-display="qualityDisplay"
          :speed="state.speed.value"
          :speeds="[0.75, 1, 1.25, 1.5, 2]"
          :auto-next="state.autoNext.value"
          :auto-skip="state.autoSkip.value"
          :hacker-mode="state.hackerMode.value"
          :debug-stats="debugStats"
          @update:quality="onSetQuality"
          @update:speed="onSetSpeed"
          @update:auto-next="v => { state.autoNext.value = v }"
          @update:auto-skip="v => { state.autoSkip.value = v }"
          @update:hacker-mode="v => { state.hackerMode.value = v }"
          @share="onShare"
        />
      </div>
    </Teleport>

    <!-- Subtitles menu (floating, above control bar) -->
    <Teleport to="body" :disabled="!sheetTeleport">
      <div
        v-if="openMenu === 'subs'"
        ref="subsMenuEl"
        class="pl-floating pl-floating--btnmenu"
        :class="{ 'pl-floating--mobile-sheet': sheetTeleport }"
        :style="{ '--prov': activeProviderHue }"
        @click.stop
      >
        <SubtitlesMenu
          :sub-lang="state.subLang.value"
          :available-sub-langs="availableSubLangs"
          :lang-sources="langSources"
          :browse-count="subtitleTracks.length"
          :hardsub-note="hardsubNote"
          :sub-size="state.subSize.value"
          :sub-bg="state.subBg.value"
          :sub-offset="state.subOffset.value"
          @pick-lang="onPickSubLang"
          @update:sub-size="v => { state.subSize.value = v }"
          @update:sub-bg="v => { state.subBg.value = v }"
          :auto-sync="autoSyncPref.enabled.value"
          :auto-sync-info="autoSyncInfo"
          @update:sub-offset="v => { state.subOffset.value = v }"
          @update:auto-sync="v => autoSyncPref.setEnabled(v)"
          @open-browse="() => { openMenu = null; browseOpen = true; void ensureSubsLoaded() }"
        />
      </div>
    </Teleport>

    <!-- Browse subtitles modal -->
    <Teleport to="body" :disabled="!sheetTeleport">
      <div v-if="browseOpen" :class="sheetTeleport ? 'pl-bsm-host' : 'contents'">
        <BrowseSubsModal
          :tracks="subtitleTracks"
          :selected-url="chosenSubUrl"
          :loading="subsLoading"
          :error="subsError"
          :providers-down="subsProvidersDown"
          @click.stop
          @select="onSelectSubTrack"
          @retry="refetchSubs"
          @off="onSubtitlesOff"
          @close="browseOpen = false"
        />
      </div>
    </Teleport>

    <Teleport to="body" :disabled="!sheetTeleport">
      <DownloadDialog
        v-if="downloadDialogOpen"
        :season-count="seasonCount"
        :duration-min="anime.durationMin"
        :report="report"
        :initial-combo="state.combo.value"
        :sub-options="dlSubOptions"
        :load-teams="dlLoadTeams"
        :sheet="sheetTeleport"
        @confirm="onConfirmDownload"
        @close="downloadDialogOpen = false"
      />
    </Teleport>

    <!-- Mobile sheet scrim — tap closes whatever sheet is open -->
    <Teleport to="body" :disabled="!sheetTeleport">
      <div
        v-if="sheetTeleport && anySheetOpen"
        class="pl-sheet-scrim"
        data-test="sheet-scrim"
        @click="closeAllSheets"
      />
    </Teleport>
    </div>

    <!-- Mobile action row (D6) — big under-player entries. Lives inside the
         player component so every mount point (anime page, /downloads) gets
         it with zero cross-component wiring. -->
    <div v-if="isMobile" class="pl-actions" data-test="pl-actions">
      <Button variant="soft" size="sm" class="pl-action" data-test="action-episodes" @click="toggleMenu('episodes')">
        <ListVideo class="size-4" aria-hidden="true" />
        {{ $t('player.aePlayer.epAbbrev') }} {{ selectedEpisode?.number ?? initialEpisode ?? 1 }}
        <ChevronDown class="size-3.5 opacity-70" aria-hidden="true" />
      </Button>
      <Button v-if="!offline" variant="soft" size="sm" class="pl-action pl-action--src" data-test="action-source" @click="toggleMenu('source')">
        <span class="pl-prov-dot" :style="{ background: activeProviderHue }" aria-hidden="true" />
        <span class="pl-action-srcname">{{ activeProviderName || $t('player.aePlayer.source') }}</span>
        <ChevronDown class="size-3.5 opacity-70" aria-hidden="true" />
      </Button>
      <Button v-if="downloadMode === 'ready'" variant="soft" size="sm" class="pl-action" data-test="action-download" @click="onDownloadSeason">
        <Download class="size-4" aria-hidden="true" />
        {{ $t('player.aePlayer.offline.download') }}
      </Button>
    </div>
  </div>
</template>

<script setup lang="ts">
import {
  ref,
  computed,
  watch,
  onMounted,
  onUnmounted,
  toRef,
} from 'vue'
import { CircleAlert, ChevronDown, Download, ListVideo, Pause, Play, X } from 'lucide-vue-next'

import { useAuthStore } from '@/stores/auth'
import SubtitleOverlay from '@/components/player/SubtitleOverlay.vue'
import type { ResumeBanner } from '@/composables/watchState'
import { Button } from '@/components/ui'
import PlayerControlBar from './PlayerControlBar.vue'
import SourcePanel from './SourcePanel.vue'
import EpisodesPanel from './EpisodesPanel.vue'
import PlaybackSettingsMenu from './PlaybackSettingsMenu.vue'
import SubtitlesMenu from './SubtitlesMenu.vue'
import BrowseSubsModal from './BrowseSubsModal.vue'
import { buildShareUrl } from './shareLink'
import BigPlayButton from './overlays/BigPlayButton.vue'
import BufferingOverlay from './overlays/BufferingOverlay.vue'
import DebugHud, { type SourceDecision } from './overlays/DebugHud.vue'
import SkipIntroChip from './overlays/SkipIntroChip.vue'
import NextEpisodeCard from './overlays/NextEpisodeCard.vue'
import NextEpisodeChip from './overlays/NextEpisodeChip.vue'
import WatchTogetherButton from './overlays/WatchTogetherButton.vue'
import DownloadDialog from './DownloadDialog.vue'

import { usePlayerState } from '@/composables/aePlayer/usePlayerState'
import { sourceFallbackDebug } from '@/composables/aePlayer/sourceFallbackDebug'
import { useVideoEngine } from '@/composables/aePlayer/useVideoEngine'
import { useProviderResolver } from '@/composables/aePlayer/useProviderResolver'
import { useI18n } from 'vue-i18n'
import { useWatchTracking } from '@/composables/aePlayer/useWatchTracking'
import { groupOfProvider, playerKeyOfProvider } from '@/composables/aePlayer/useProviderFeed'
import { fmtResume } from '@/composables/aePlayer/episodeProgress'
import { comboToWatchCombo, clampLangForAudio } from '@/composables/aePlayer/comboMapping'
import type { WtCreateSeed } from '@/composables/aePlayer/wtCreateSeed'
import { useToast } from '@/composables/useToast'
import { useMobilePlayer } from '@/composables/aePlayer/useMobilePlayer'
import { recordPlayerEvent } from '@/utils/playerTelemetry'

import { usePlayerSyncBridge } from '@/composables/usePlayerSyncBridge'
import { makeOfflineResolver } from '@/offline/offlineAdapter'

// Cluster composables — each owns one concern of the player; this component is
// the composition root that wires them together (and keeps the template/styles).
import { useAutoplayGate } from '@/composables/aePlayer/useAutoplayGate'
import { usePlaybackClock } from '@/composables/aePlayer/usePlaybackClock'
import { useRoomSync } from '@/composables/aePlayer/useRoomSync'
import { useCapabilityFeed } from '@/composables/aePlayer/useCapabilityFeed'
import { useSourceFailover } from '@/composables/aePlayer/useSourceFailover'
import { useComboBootstrap } from '@/composables/aePlayer/useComboBootstrap'
import { useEpisodeWatchData } from '@/composables/aePlayer/useEpisodeWatchData'
import { useWtSeedUrlSync } from '@/composables/aePlayer/useWtSeedUrlSync'
import { useResumeChip } from '@/composables/aePlayer/useResumeChip'
import { useQualityControl } from '@/composables/aePlayer/useQualityControl'
import {
  useStreamResolution,
  type ResolveEpoch,
  type ResolveTelemetryTiming,
} from '@/composables/aePlayer/useStreamResolution'
import { useSkipIntro } from '@/composables/aePlayer/useSkipIntro'
import { useDebugTools } from '@/composables/aePlayer/useDebugTools'
import { useNextEpisode } from '@/composables/aePlayer/useNextEpisode'
import { usePlayerMenus, type MenuKind } from '@/composables/aePlayer/usePlayerMenus'
import { useOfflineDownloads } from '@/composables/aePlayer/useOfflineDownloads'
import { useUiIdle } from '@/composables/aePlayer/useUiIdle'
import { useSubtitleWiring } from '@/composables/aePlayer/useSubtitleWiring'
import { useGestureControls } from '@/composables/aePlayer/useGestureControls'
import { useFullscreen } from '@/composables/aePlayer/useFullscreen'
import { usePlayerKeyboard } from '@/composables/aePlayer/usePlayerKeyboard'
import { useProtocolTelemetry } from '@/composables/aePlayer/useProtocolTelemetry'

import type { EpisodeOption } from '@/components/player/EpisodeSelector.types'
import type { StreamResult, AudioKind } from '@/types/aePlayer'
import type { WatchTogetherRoomHandle } from '@/composables/useWatchTogetherRoom'

// ─── Props / Emits ───────────────────────────────────────────────────────────

const props = defineProps<{
  animeId: string
  anime: { title: string; eps: number; still?: string; durationMin?: number }
  theater: boolean
  /** Whether the host view implements theater (a real @toggle-theater handler
   *  plus the page-level CSS). Forwarded to the control bar; false ⇒ no button.
   *  Only Anime.vue passes true. */
  canTheater?: boolean
  isHentai?: boolean
  initialEpisode?: number
  /** Notification deep-link: aePlayer provider id to pin on mount (e.g. 'kodik').
   *  Ignored unless it names a real, active provider row. */
  initialProvider?: string
  /** Notification deep-link: team TITLE to preselect alongside initialProvider. */
  initialTeam?: string
  /** URL facet override (?audio=raw|sub|dub). raw/sub → RAW, dub → DUB. */
  initialAudio?: string
  /** URL facet override (?lang=en|ru|ja). */
  initialLang?: string
  /** Shared-link `?t=` — seek to this position (seconds) on the FIRST stream
   *  load, once. Suppresses the passive resume chip for that load. */
  initialTimestamp?: number
  /** Shikimori id (= MAL id) for AniSkip skip-times. Absent ⇒ no skip UI. */
  malId?: string | number
  /** Watch-Together: when set, the player mirrors playback (play/pause/seek)
   *  and source/episode state to the room. Null/undefined ⇒ zero WT behavior
   *  (the bridge is never instantiated, auto-source-select runs as normal). */
  room?: WatchTogetherRoomHandle | null
  /** Resume/airing status for the in-player banner, computed by the parent's
   *  unified watch state. Only the "caught up, waiting for the next episode"
   *  family is surfaced as a top-center overlay (next-unavailable: "ep N airs
   *  {when}" / "ep N not available yet"). none / just-finished ⇒ no banner. */
  resumeBanner?: ResumeBanner
  /** Offline playback bundle (from /downloads). When set: episodes + streams
   *  come from the local download store, the capability feed is synthetic
   *  (one 'offline' provider), and no network resolution is attempted. */
  offline?: import('@/offline/offlineAdapter').OfflinePlayback | null
}>()

const emit = defineEmits<{
  (e: 'toggle-theater'): void
  /** Watch-Together create seed. Emitted (outside a room) whenever the live
   *  combo + episode resolve to a usable source, so the anime page's Invite
   *  button can create the room AS aeplayer seeded with the current combo.
   *  `null` ⇒ no usable source yet (the InviteButton keeps the legacy default). */
  (e: 'combo-change', seed: WtCreateSeed | null): void
  /** Shareable-URL sync (outside a room). Reflects the live source/team/episode
   *  so the parent view can mirror them into `?provider/?team/?episode`. The
   *  provider/team are EMPTY for an auto/smart-default selection (so a plain
   *  reload re-runs the deterministic BEST default) and populated only for a
   *  user-pinned source (manual pick or `?provider=` deep-link). */
  (e: 'url-sync', state: { provider: string; team: string; episode: number }): void
}>()

// ─── Core state ──────────────────────────────────────────────────────────────

const videoRef = ref<HTMLVideoElement | null>(null)
const rootRef = ref<HTMLElement | null>(null)
const isPointerInside = ref(false)
const state = usePlayerState()
// Pass hackerMode as the stats-collection gate: the per-fragment fragStats/
// bandwidth churn is only consumed by the debug HUD + scrub heatmap, so skip it
// for the ~99% of sessions with hacker mode off (the watchdog uses the cheap
// always-on fragLoadedCount instead).
const engine = useVideoEngine(videoRef, state.hackerMode)
// Guard point 1 — resolver. Offline: a ProviderResolver that reads local
// downloads instead of hitting any network API (episodes + stream URLs resolve
// to /__offline/… paths). Live: the real multi-provider resolver, unchanged.
const resolver = props.offline ? makeOfflineResolver(props.offline) : useProviderResolver((id) => groupOfProvider(report.value, id))
const { t } = useI18n()
const toast = useToast()
const { isMobile, isCoarse } = useMobilePlayer()

// Shared reactive state, hoisted to the composition root: multiple cluster
// composables read/write these, so the component owns them and passes the refs
// explicitly. Pure ref declarations carry no side effects — hoisting them does
// not change watcher/lifecycle registration order.
const episodes = ref<EpisodeOption[]>([])
const selectedEpisode = ref<EpisodeOption | null>(null)
// True once the user manually switches episodes — freezes the reactive
// initialEpisode re-pick so resume resolving late never yanks a deliberate pick.
const userPickedEpisode = ref(false)
const resolvedServers = ref<{ id: string; label: string }[]>([])
const teams = ref<string[]>([])
const currentStream = ref<StreamResult | null>(null)
const isResolving = ref(false)
const sourceError = ref<string | null>(null)
// True once the playhead reaches the episode end. Drives the manual next-episode
// chip on the autoplay-OFF path; reset when playback resumes (onVideoPlay) or at
// any source swap (resetPlaybackClock — covers manual episode/server switches too).
const reachedEpisodeEnd = ref(false)
const openMenu = ref<MenuKind>(null)
const browseOpen = ref(false)
const downloadDialogOpen = ref(false)
const uiVisible = ref(true)
const pseudoFs = ref(false)
const nativeFsActive = ref(false)

// Monotonically-increasing resolve token + best-effort telemetry timing — plain
// mutable boxes shared between the resolution cluster (writes) and the failover
// watchdog / first-frame glue below (reads).
const resolveEpoch: ResolveEpoch = { token: 0 }
const telemetryTiming: ResolveTelemetryTiming = {
  resolveStartedAt: 0,
  reachedReported: false,
  stallStartedAt: 0,
}

// Hacker-mode insight: WHAT source combo is active and WHY it was chosen. Set at
// every selection site (deep-link, smart default, facet re-pick, auto-failover,
// room pin, manual). Surfaced in the DebugHud so you can see whether the player
// landed on this provider/audio/lang/team because it's the BEST pick, a pinned
// deep-link, a fallback after a failure, or your own click.
const sourceDecision = ref<SourceDecision | null>(null)
function recordDecision(reason: string) {
  const c = state.combo.value
  sourceDecision.value = {
    provider: c.provider,
    audio: c.audio,
    lang: c.lang,
    team: c.team ?? null,
    reason,
  }
}

// ─── Autoplay gate ───────────────────────────────────────────────────────────
// No watchers — safe to compose early; the room bridge below needs its
// handlePlayRejection.
const autoplay = useAutoplayGate({
  videoRef,
  state,
  getAnimeId: () => props.animeId,
  getEpisodeNumber: () => selectedEpisode.value?.number,
})
const { playbackBlocked, playbackBlockedHint, attemptPlay } = autoplay

// ─── rAF progress loop / playback clock ──────────────────────────────────────
// No watchers — composed early so every later cluster can use the reactive
// clock directly. Tracking + UI-idle hooks are late-bound thunks (those
// clusters compose further down, at their original watcher positions).
const clock = usePlaybackClock({
  videoRef,
  state,
  engine,
  reachedEpisodeEnd,
  attemptPlay,
  trackingTick: (timeSec, durationSec) => tracking.onTick(timeSec, durationSec),
  trackingSaveNow: () => tracking.saveNow(),
  armUiIdleTimer: () => ui.armUiIdleTimer(),
  clearUiIdleTimer: () => ui.clearUiIdleTimer(),
  uiVisible,
})
const {
  currentTime,
  duration,
  bufferedPct,
  hasStarted,
  writeProgress,
  resetPlaybackClock,
  stopRaf,
  onVideoPlay,
  onVideoPause,
  capturePlayhead,
  restorePlayhead,
} = clock

// ─── Watch-Together (room sync) ───────────────────────────────────────────────
// When mounted inside a WT room, wire the generic HTML5 playback bridge (mirrors
// play/pause/seek/time-tick both ways). When the room is null/undefined the
// bridge is never instantiated and the player behaves exactly as standalone.
if (props.room) {
  usePlayerSyncBridge(videoRef, props.room, { onPlayRejected: autoplay.handlePlayRejection })
}

const roomSync = useRoomSync({
  getRoom: () => props.room,
  state,
  episodes,
  selectedEpisode,
  onSelectEpisode: (ep) => onSelectEpisode(ep),
  recordDecision,
})
const { roomPinned, roomHasCombo } = roomSync

// ─── Capability feed (backend single source of truth) ────────────────────────

const animeIdRef = computed(() => props.animeId)
const feed = useCapabilityFeed({
  animeIdRef,
  getOffline: () => props.offline,
  isHentai: () => !!props.isHentai,
  state,
})
const { report, capMap, rows, activeProviderName, activeProviderHue } = feed

// ─── Source failover (dynamic BEST) ──────────────────────────────────────────

const failover = useSourceFailover({
  engine,
  state,
  videoRef,
  rows,
  report,
  resolvedServers,
  currentStream,
  selectedEpisode,
  sourceError,
  roomPinned,
  getAnimeId: () => props.animeId,
  getHasStarted: () => hasStarted.value,
  getPlaybackBlocked: () => playbackBlocked.value,
  getPlaybackStats: () => debug.playbackStats.value,
  getResolveToken: () => resolveEpoch.token,
  recordDecision,
  toast,
  t,
})
const {
  providerAutoSelected,
  advanceToNextSource,
  armPlaybackWatchdog,
  clearPlaybackWatchdog,
  resetSourceSwitching,
} = failover

// ─── Combo bootstrap: saved prefs > URL facets > smart default ───────────────

const bootstrap = useComboBootstrap({
  state,
  rows,
  report,
  capMap,
  roomHasCombo,
  providerAutoSelected,
  recordDecision,
  animeId: props.animeId,
  getInitialProvider: () => props.initialProvider,
  getInitialTeam: () => props.initialTeam,
  getInitialAudio: () => props.initialAudio,
  getInitialLang: () => props.initialLang,
  getInitialEpisode: () => props.initialEpisode,
  isHentai: () => !!props.isHentai,
})
const { preferenceSettled, pickFacetDefault } = bootstrap

// ─── Active audio display + slider handler ───────────────────────────────────

const audioLabel = computed(() =>
  state.combo.value.audio === 'dub' ? t('player.dub') : t('player.aePlayer.audioRaw'),
)

// Audio slider handler. Switching to DUB while the carried lang is 'ja' (set by a
// RAW pick on a JP source) would leave the DUB filter on a language its RU/EN
// slider can't represent and no dub provider serves — clamp it to EN.
function onSelectAudio(a: AudioKind) {
  // Clamp ja→en BEFORE switching to DUB (no Japanese dub; DUB slider is EN/RU
  // only). Setting lang first — while still RAW, where lang is inert — avoids a
  // transient doomed repick on the dub/ja facet.
  const clamped = clampLangForAudio(a, state.combo.value.lang)
  if (clamped !== state.combo.value.lang) state.setLang(clamped)
  state.setAudio(a)
}

// ─── User watch data (read-only): watched marks + per-episode progress ───────

const auth = useAuthStore()
const watchData = useEpisodeWatchData({ auth, getAnimeId: () => props.animeId })
const { watchedUpTo, refreshWatched, epProgress, loadEpisodeProgress, isEpisodeWatched } = watchData

// ─── Watch-progress tracking (server + localStorage + auto-complete) ─────────

const tracking = useWatchTracking(
  () => props.animeId,
  () => selectedEpisode.value?.number ?? null,
  {
    onMarked: () => {
      void refreshWatched()
      void loadEpisodeProgress()
    },
  },
  () => comboToWatchCombo(
    state.combo.value,
    groupOfProvider(report.value, state.combo.value.provider),
    playerKeyOfProvider(report.value, state.combo.value.provider),
  ),
)

// ─── WT create seed + shareable-URL sync + in-player launch ──────────────────

const wtSync = useWtSeedUrlSync({
  getRoom: () => props.room,
  state,
  selectedEpisode,
  providerAutoSelected,
  preferenceSettled,
  auth,
  getAnimeId: () => props.animeId,
  emitComboChange: (seed) => emit('combo-change', seed),
  emitUrlSync: (s) => emit('url-sync', s),
})
const { currentWtSeed, wtLaunching, showWtLaunch, onLaunchWt } = wtSync

/** Manual mark from the episodes drawer (Kodik-parity button). */
function onMarkWatched() {
  void tracking.markWatched()
}

// ─── Resume-from-saved-position chip + shared-link `?t=` seek ────────────────

const resume = useResumeChip({
  videoRef,
  auth,
  getAnimeId: () => props.animeId,
  initialTimestamp: props.initialTimestamp,
  selectedEpisode,
  epProgress,
  sourceError,
  isResolving,
  duration,
  currentTime,
  hasStarted,
  attemptPlay,
  writeProgress,
})
const {
  resumeChipDismissed,
  resumeChipUsed,
  resumePosSec,
  resumeChipVisible,
  onResumeFromSaved,
  applyInitialSeek,
} = resume

// Share "this exact moment" — copy a link that reproduces the live source, team,
// audio/lang, episode, and current timestamp for the recipient. Built from the
// resolved combo unconditionally (unlike the passive address-bar sync, which
// only writes a pinned source), so an auto-selected source is still shared.
async function onShare() {
  const url = buildShareUrl({
    origin: typeof window !== 'undefined' ? window.location.origin : '',
    animeId: props.animeId,
    combo: state.combo.value,
    episode: selectedEpisode.value?.number ?? 0,
    timeSec: currentTime.value,
  })
  try {
    await navigator.clipboard.writeText(url)
    toast.push(t('player.aePlayer.shareCopied'), 'success', 3000)
  } catch {
    // Clipboard blocked (insecure context / permission) — surface the link so
    // the user can copy it by hand, mirroring the WT-invite fallback.
    toast.push(t('player.aePlayer.shareCopyFailed'), 'info', 6000)
  }
}

// ─── Quality ladder ──────────────────────────────────────────────────────────

const quality = useQualityControl({
  state,
  engine,
  videoRef,
  currentStream,
  attemptPlay,
  resolveStreamForCurrentEpisode: () => resolution.resolveStreamForCurrentEpisode(),
})
const { qualities, qualityDisplay, onSetQuality } = quality

// ─── Episode list + stream resolution ────────────────────────────────────────

const resolution = useStreamResolution({
  state,
  resolver,
  engine,
  tracking,
  toast,
  t,
  recordDecision,
  getAnimeId: () => props.animeId,
  getInitialEpisode: () => props.initialEpisode,
  episodes,
  selectedEpisode,
  userPickedEpisode,
  resolvedServers,
  teams,
  currentStream,
  isResolving,
  sourceError,
  openMenu,
  rows,
  resolveEpoch,
  telemetryTiming,
  providerAutoSelected,
  roomPinned,
  advanceToNextSource,
  armPlaybackWatchdog,
  resetSourceSwitching,
  noteResolveStarted: failover.noteResolveStarted,
  hasStarted,
  resetPlaybackClock,
  capturePlayhead,
  restorePlayhead,
  resetPlaybackBlocked: autoplay.resetPlaybackBlocked,
  applyInitialSeek,
  applyOfflineAutoSub: (epNumber, stream) => subs.applyOfflineAutoSub(epNumber, stream),
  isEpisodeWatched,
  broadcastEpisodeChange: roomSync.broadcastEpisodeChange,
  pickFacetDefault,
  resumeChipDismissed,
  resumeChipUsed,
})
const {
  initSelectedEpisode,
  resolveStreamForEpisode,
  onSelectProvider,
  onSelectEpisode,
  retryResolution,
} = resolution

// Test seam (see AePlayer.*.spec.ts): expose the live combo ref + a setter so WT
// room-sync specs can assert the applied/pinned combo and simulate a genuine
// local source change without mocking usePlayerState (which hands every caller an
// independent instance), plus the selected episode for next-episode specs.
// __-prefixed keys are never read in prod.
defineExpose({
  __combo: state.combo,
  __setProvider: state.setProvider,
  __selectedEpisode: selectedEpisode,
  onSelectEpisode,
})

// ─── Intro/outro skip (AniSkip via catalog proxy) ────────────────────────────

const skip = useSkipIntro({
  getMalId: () => props.malId,
  selectedEpisode,
  state,
  videoRef,
  currentTime,
  duration,
  writeProgress,
})
const { chapters, skipTarget, onSkipSegment } = skip

// ─── Buffering indicator ──────────────────────────────────────────────────────
// waiting/seeking → on; playing/canplay → off. A 150ms grace window keeps
// instant in-buffer seeks from flashing the ring. `seeked` only clears when
// the element actually has decodable data (readyState ≥ 3) — otherwise the
// following `waiting` keeps the ring up. We deliberately do NOT bind
// `stalled`: browsers fire it spuriously when the download is throttled
// because the buffer is FULL, and nothing would clear the ring during healthy
// playback. `timeupdate` self-heals any false positive.

const isBuffering = ref(false)
const showBuffering = ref(false)
let bufferingTimer: ReturnType<typeof setTimeout> | null = null

function setBuffering(on: boolean) {
  if (on === isBuffering.value) return
  isBuffering.value = on
  if (on) {
    bufferingTimer = setTimeout(() => {
      showBuffering.value = true
    }, 150)
  } else {
    if (bufferingTimer) {
      clearTimeout(bufferingTimer)
      bufferingTimer = null
    }
    showBuffering.value = false
  }
}

function onBufferStart() {
  setBuffering(true)
  // Telemetry: mid-playback stall — capture start time (only after first frame)
  if (telemetryTiming.reachedReported && !telemetryTiming.stallStartedAt) {
    telemetryTiming.stallStartedAt = performance.now()
  }
}

function onBufferEnd() {
  setBuffering(false)
  debug.markSeekResumed()
  // Telemetry: stall resolved — emit duration (best-effort, never throws)
  if (telemetryTiming.stallStartedAt) {
    const stallMs = Math.round(performance.now() - telemetryTiming.stallStartedAt)
    telemetryTiming.stallStartedAt = 0
    recordPlayerEvent({
      kind: 'stall',
      provider: state.combo.value.provider,
      anime_id: props.animeId,
      episode: selectedEpisode.value?.number,
      stall_ms: stallMs,
    })
  }
}

function onSeeked() {
  const v = videoRef.value
  if (v && v.readyState >= 3) setBuffering(false)
  // Seek trace: decoder positioned at the target frame
  debug.noteSeeked()
}

// Self-heal: if time is advancing with decodable data, we are NOT buffering.
function onTimeUpdate() {
  const v = videoRef.value
  // Drop the poster only once playback actually progresses — the bare `play`
  // event fires even on a dead source (readyState 0), and `timeupdate` also
  // fires on a seek-while-paused before the first play; both would swap the
  // poster for a black frame. Require real (unpaused) progress.
  if (!hasStarted.value && v && v.currentTime > 0 && !v.paused) {
    hasStarted.value = true
    clearPlaybackWatchdog() // real playback — cancel the stall watchdog
    autoplay.resetPlaybackBlocked() // actually playing — any autoplay veto is history
    resetSourceSwitching() // a source that actually plays earns a fresh budget
    // A source that actually plays must clear any stale error overlay: the
    // fallback chain can set sourceError='Stream unavailable' when the switch
    // budget is exhausted WHILE the source it just landed on is still buffering
    // — then that source starts playing and the overlay would otherwise stay up
    // over a working video. First real frame ⇒ the stream works, drop the error.
    sourceError.value = null
    // Telemetry: first real frame — resolve ok + reached_playback (best-effort)
    if (!telemetryTiming.reachedReported) {
      telemetryTiming.reachedReported = true
      recordPlayerEvent({
        kind: 'resolve',
        provider: state.combo.value.provider,
        anime_id: props.animeId,
        episode: selectedEpisode.value?.number,
        outcome: 'ok',
        reached_playback: true,
        latency_ms: telemetryTiming.resolveStartedAt ? Math.round(performance.now() - telemetryTiming.resolveStartedAt) : undefined,
        audio: state.combo.value.audio,
        lang: state.combo.value.lang,
      })
    }
  }
  if (isBuffering.value && v && v.readyState >= 3 && !v.seeking) {
    setBuffering(false)
  }
  if (v && v.readyState >= 3 && !v.seeking) {
    debug.markSeekResumed()
  }
}

// A dead source must surface the error overlay, not an endless spinner.
// Covers the native/mp4 path (e.g. upstream CDN 404 → MEDIA_ERR_SRC_NOT_SUPPORTED).
function onVideoError() {
  const v = videoRef.value
  // isResolving: a resolve is in flight. switchingSource: an auto-failover just
  // committed and the destination resolve hasn't taken over yet — in both cases
  // this native error is from the source we're leaving, not a dead destination.
  if (!v?.error || isResolving.value || failover.isSwitchingSource()) return
  setBuffering(false)
  sourceError.value = t('player.aePlayer.streamUnavailable')
}

// hls.js fatal (dead playlist / unrecoverable). Dynamic BEST: try the next
// candidate source so a blocked CDN host auto-recovers to a working one.
watch(engine.fatal, async (f) => {
  if (!f) return
  setBuffering(false)
  // Telemetry: HLS fatal (best-effort, never throws)
  recordPlayerEvent({
    kind: 'resolve',
    provider: state.combo.value.provider,
    anime_id: props.animeId,
    episode: selectedEpisode.value?.number,
    outcome: 'fail',
    reached_playback: telemetryTiming.reachedReported,
    error_kind: f === 'network' ? 'stream_error' : 'media_fatal',
    latency_ms: telemetryTiming.resolveStartedAt ? Math.round(performance.now() - telemetryTiming.resolveStartedAt) : undefined,
    audio: state.combo.value.audio,
    lang: state.combo.value.lang,
  })
  // Dynamic BEST: auto-switch to the next candidate so a blocked CDN host
  // recovers to a working one. In hacker mode advanceToNextSource only records
  // the intent and returns false — "BEST" is only meaningful if it lands on a
  // source that plays, but in hacker mode we let you verify that manually.
  if (await advanceToNextSource('playback fatal')) {
    toast.push(t('player.aePlayer.switchFailed'), 'info', 4000)
    return
  }
  if (!sourceError.value) sourceError.value = t('player.aePlayer.streamUnavailable')
})

// ─── Hacker mode (debug HUD) ──────────────────────────────────────────────────

const debug = useDebugTools({ state, engine, videoRef, currentStream, duration, showBuffering })
const {
  playbackStats,
  lastSeek,
  traceSeekStart,
  onVideoProgress,
  hudVisible,
  hudFading,
  fragOverlay,
  debugStats,
} = debug

// ─── Next episode logic ───────────────────────────────────────────────────────

const nextEp = useNextEpisode({
  state,
  tracking,
  episodes,
  selectedEpisode,
  reachedEpisodeEnd,
  skipTarget,
  getInitialEpisode: () => props.initialEpisode,
  getTotalEps: () => props.anime.eps,
  isEpisodeWatched,
  resolveStreamForEpisode,
  resumeChipDismissed,
  resumeChipUsed,
})
const {
  showNextEpisode,
  nextEpCountdown,
  onEnded,
  anime_hasNextEp,
  nextEpisodeNumber,
  showNextEpChip,
  clearNextEpTimer,
  goToNextEpisode,
} = nextEp

// ─── Menu state ───────────────────────────────────────────────────────────────

const menus = usePlayerMenus({ openMenu, browseOpen, downloadDialogOpen, isMobile, nativeFsActive, videoRef })
const {
  sourceMenuEl,
  episodesMenuEl,
  settingsMenuEl,
  subsMenuEl,
  toggleMenu,
  closeMenus,
  sheetTeleport,
  anySheetOpen,
  closeAllSheets,
} = menus

// ─── Offline downloads (season-only, app-only) ──────────────────────────────

const downloads = useOfflineDownloads({
  getOffline: () => props.offline,
  getAnimeId: () => props.animeId,
  getAnimeMeta: () => ({ title: props.anime.title, still: props.anime.still, durationMin: props.anime.durationMin }),
  state,
  resolver,
  episodes,
  downloadDialogOpen,
  toast,
  t,
  getSubtitleTracks: () => subs.subtitleTracks.value,
  getBundledTracks: () => subs.providerBundledTracks.value,
  ensureSubsLoaded: () => subs.ensureSubsLoaded(),
})
const {
  downloadStates,
  seasonCount,
  downloadMode,
  dlSubOptions,
  dlLoadTeams,
  onDownloadSeason,
  onConfirmDownload,
} = downloads

// ─── Controls auto-hide (idle while playing) ─────────────────────────────────

const ui = useUiIdle({ state, uiVisible, isPointerInside, openMenu, isCoarse, videoRef })
const { wakeUi, clearUiIdleTimer, onRootTouch, onPointerEnter, onPointerLeave } = ui

// ─── Subtitles ────────────────────────────────────────────────────────────────

const subs = useSubtitleWiring({
  animeIdRef: toRef(props, 'animeId'),
  getAnimeId: () => props.animeId,
  getInitialEpisode: () => props.initialEpisode,
  getOffline: () => props.offline,
  state,
  videoRef,
  currentStream,
  selectedEpisode,
  activeProviderName,
  browseOpen,
  toast,
  t,
})
const {
  chosenSubUrl,
  chosenSubFormat,
  subsOn,
  subtitleTracks,
  subsLoading,
  subsError,
  subsProvidersDown,
  ensureSubsLoaded,
  refetchSubs,
  effectiveOffset,
  autoSyncPref,
  autoSyncInfo,
  availableSubLangs,
  langSources,
  hardsubNote,
  onSelectSubTrack,
  onSubtitlesOff,
  onPickSubLang,
  onSubtitleError,
} = subs

// ─── Upcoming episode placeholder ────────────────────────────────────────────

// The "next episode airs {when}" info is NOT overlaid on the video — the anime
// page already shows that banner above the player. Instead we surface the
// awaited episode as a disabled placeholder inside the episodes sheet. Derived
// from the resume banner's next-unavailable family.
const upcomingEpisode = computed<{ number: number; etaLabel?: string } | null>(() => {
  const b = props.resumeBanner
  return b && b.kind === 'next-unavailable' ? { number: b.episode, etaLabel: b.etaLabel } : null
})

// ─── Playback helpers (tap gestures, volume/speed/PiP, seeking) ──────────────

const gestures = useGestureControls({
  videoRef,
  rootRef,
  state,
  isCoarse,
  openMenu,
  browseOpen,
  closeMenus,
  uiVisible,
  wakeUi,
  clearUiIdleTimer,
  attemptPlay,
  traceSeekStart,
  writeProgress,
})
const {
  seekFlash,
  onVideoClick,
  togglePlay,
  onSeekRel,
  onSeek,
  onSetVolume,
  onToggleMute,
  onSetSpeed,
  onVolumeChange,
  onTogglePip,
} = gestures

// ─── Fullscreen (capability-based) ───────────────────────────────────────────

const fs = useFullscreen({ rootRef, videoRef, isCoarse, pseudoFs, nativeFsActive })
const { fullscreenActive, onToggleFullscreen } = fs

// ─── Keyboard shortcuts ───────────────────────────────────────────────────────

const keyboard = usePlayerKeyboard({
  rootRef,
  videoRef,
  isPointerInside,
  state,
  openMenu,
  browseOpen,
  wakeUi,
  closeMenus,
  toggleMenu,
  togglePlay,
  onSeekRel,
  onSetVolume,
  onToggleMute,
  onToggleFullscreen,
  onTogglePip,
  writeProgress,
  anime_hasNextEp,
  showNextEpisode,
  showNextEpChip,
  goToNextEpisode,
})
const { onKeydown } = keyboard

// ─── Lifecycle ────────────────────────────────────────────────────────────────

// Graceful save on tab close / backgrounding. `pagehide` covers navigation
// and close (incl. mobile); visibilitychange→hidden covers app switches —
// both use sendBeacon, which survives the unload where XHR doesn't.
function onPageHide() {
  tracking.beaconSave()
}
function onVisibilityChange() {
  if (document.visibilityState === 'hidden') tracking.beaconSave()
}

onMounted(() => {
  // Apply initial volume
  const v = videoRef.value
  if (v) {
    v.volume = state.volume.value / 100
    v.muted = state.muted.value
  }
  // Bootstrap episode selection so it's ready before provider resolves
  initSelectedEpisode()
  // User watch data for the episodes drawer (best-effort, anonymous = empty)
  void refreshWatched()
  void loadEpisodeProgress()
  window.addEventListener('keydown', onKeydown)
  window.addEventListener('pagehide', onPageHide)
  window.addEventListener('pagehide', protoTel.onProtocolUsagePageHide)
  document.addEventListener('visibilitychange', onVisibilityChange)
  document.addEventListener('fullscreenchange', fs.onFullscreenChange)
})

// ── Protocol ladder subscriptions + (h1/h2/h3) usage telemetry ────────────────

const protoTel = useProtocolTelemetry({
  videoRef,
  state,
  engine,
  currentStream,
  selectedEpisode,
  hasStarted,
  getAnimeId: () => props.animeId,
  getAnimeTitle: () => props.anime.title,
  recordDecision,
  resolveStreamForEpisode,
})

onUnmounted(() => {
  stopRaf()
  tracking.saveNow() // persist position when navigating away in-app
  clearNextEpTimer()
  clearUiIdleTimer()
  debug.clearHudTimers()
  clearPlaybackWatchdog()
  if (bufferingTimer) clearTimeout(bufferingTimer)
  downloads.clearDlRefreshTimer()
  gestures.clearGestureTimers()
  window.removeEventListener('keydown', onKeydown)
  window.removeEventListener('pagehide', onPageHide)
  protoTel.flushProtocolUsage()
  window.removeEventListener('pagehide', protoTel.onProtocolUsagePageHide)
  document.removeEventListener('visibilitychange', onVisibilityChange)
  document.removeEventListener('fullscreenchange', fs.onFullscreenChange)
  fs.teardownPseudoFs()
})
</script>

<style scoped>
.pl {
  position: relative;
  width: 100%;
  aspect-ratio: 16 / 9;
  border-radius: var(--r-xl, 16px);
  overflow: hidden;
  background: #000;
  border: 1px solid var(--border);
  user-select: none;
}

/* Resume-from-saved-position chip — bottom-left mirror of the skip chip. */
.pl-resume {
  position: absolute;
  left: 22px;
  bottom: 92px;
  z-index: 6;
  display: inline-flex;
  align-items: stretch;
  border-radius: var(--r-md);
  background: var(--scrim-bg-strong);
  border: 1px solid var(--white-a30);
  backdrop-filter: blur(8px);
  overflow: hidden;
}

.pl-resume-go {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 11px 14px;
  border: 0;
  background: transparent;
  color: #fff;
  font-size: 13px;
  font-weight: 600;
  cursor: pointer;
  transition: background 0.15s, color 0.15s;
}

.pl-resume-go:hover {
  background: #fff;
  color: var(--color-base, #08080f);
}

.pl-resume-x {
  display: inline-flex;
  align-items: center;
  padding: 0 10px;
  border: 0;
  border-left: 1px solid var(--white-a20);
  background: transparent;
  color: var(--muted-foreground);
  cursor: pointer;
  transition: color 0.15s, background 0.15s;
}

.pl-resume-x:hover {
  color: #fff;
  background: var(--line-strong);
}

/* Theater — full-bleed width, capped height. The base .pl aspect-ratio 16/9 is
   deliberately KEPT and merely clamped by max-height: on a wide monitor the box
   ends up wider than 16/9 and the video object-contains into it (side bars), the
   same shape YouTube's theater has. The old `height: 100vh` made this a second
   fullscreen — which is why the button was pulled in June.
   --header-offset is the navbar clearance token: subtracting it keeps the
   control bar (the player's bottom edge) on screen once the player is scrolled
   under the fixed navbar. Plain 100vh pushes it 80px below the fold.
   Gated to the SAME 1024px breakpoint as the theater button itself
   (PlayerControlBar.vue: `@media (max-width: 1023px) { .pl-theater-btn {
   display: none } }`) and as Anime.vue's body.theater-mode rules. This class
   binds to the `theater` PROP directly, not to body.theater-mode, so without
   this gate a persisted theaterMode=1 still caps the player's height below
   1024px (e.g. landscape phone) even though the exit control is hidden there
   — shrinking the player with no way to undo it. */
@media (min-width: 1024px) {
  .pl--theater {
    border-radius: 0;
    border: 0;
    max-height: calc(100vh - var(--header-offset));
  }
}

/* iPhone pseudo-fullscreen — fixed takeover (no usable element FS API on iOS).
   Black behind the notch/status bar is intended (video surface).
   svh, not % or vh: both resolve against the LARGE viewport (Safari's chrome
   collapsed), so with the toolbars actually on screen the takeover overflows the
   visible area and the control row lands under Safari's bottom bar. svh is the
   smallest-viewport height — always fully visible, never clipped by chrome. */
.pl--pseudo-fs {
  position: fixed;
  inset: 0;
  z-index: 100;
  height: 100svh;
  max-height: none; /* neutralizes .pl--theater's max-height cap (wins over height regardless of source order) */
  aspect-ratio: auto;
  border-radius: 0;
  border: 0;
}

/* The takeover runs the VIDEO under the Dynamic Island / notch / home
   indicator (enterPseudoFs opts the viewport meta into viewport-fit=cover),
   while the overlay rows pad themselves back inside the safe area so the
   episode trigger and control buttons stay visible and tappable. env() is
   all zeros whenever cover is not in effect — these rules are inert then. */
.pl--pseudo-fs .pl-top {
  padding-top: max(16px, var(--safe-top));
  padding-left: max(18px, var(--safe-left));
  padding-right: max(18px, var(--safe-right));
}

.pl--pseudo-fs :deep(.pl-controls) {
  padding-left: var(--safe-left);
  padding-right: var(--safe-right);
  padding-bottom: max(12px, env(safe-area-inset-bottom, 0px));
}

:global(html.pl-noscroll) {
  overflow: hidden;
}

.pl-scene {
  position: absolute;
  inset: 0;
  background-size: cover;
  background-position: center;
  z-index: 0;
}

.pl-grain {
  position: absolute;
  inset: 0;
  background: radial-gradient(80% 60% at 50% 38%, transparent, var(--black-a40));
  z-index: 1;
  pointer-events: none;
}

/* Episode title in the eyebrow — truncate long provider titles */
.pl-ep-title {
  display: inline-block;
  max-width: 220px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  vertical-align: bottom;
  opacity: 0.85;
}

/* V2b: the EP block doubles as the episodes-sheet trigger */
.pl-ep-trigger {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  border: 0;
  background: transparent;
  color: inherit;
  font: inherit;
  padding: 2px 6px;
  margin: -2px -2px -2px -6px;
  border-radius: var(--r-sm, 6px);
  cursor: pointer;
  transition: background 0.15s;
}

.pl-ep-trigger:hover,
.pl-ep-trigger[aria-expanded='true'] {
  background: var(--accent-soft);
}

.pl-ep-chev {
  opacity: 0.7;
  transition: transform 0.15s;
}

.pl-ep-chev--open {
  transform: rotate(180deg);
}

/* Idle while playing — fade out the chrome (top bar lives here, the control
   bar is inside <PlayerControlBar>, hence :deep). */
.pl--ui-hidden {
  cursor: none;
}

.pl--ui-hidden .pl-top,
.pl--ui-hidden :deep(.pl-controls) {
  opacity: 0;
  pointer-events: none;
}

/* Top bar */
.pl-top {
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  z-index: 6;
  display: flex;
  align-items: flex-start;
  gap: 14px;
  padding: 16px 18px 40px;
  background: linear-gradient(180deg, var(--black-a60), transparent);
  transition: opacity 0.2s;
}

.pl-icon {
  width: 40px;
  height: 40px;
  display: grid;
  place-items: center;
  border-radius: var(--r-md, 8px);
  background: transparent;
  border: 0;
  color: #fff;
  transition: background 0.15s;
  flex-shrink: 0;
  cursor: pointer;
}

.pl-icon:hover {
  background: var(--line-strong);
}

.pl-title-block {
  flex: 1;
  min-width: 0;
  padding-top: 3px;
}

.pl-eyebrow {
  font-size: 12px;
  color: var(--brand-cyan);
  display: block;
}

.pl-eyebrow-src {
  display: inline-flex;
  align-items: center;
  gap: 5px;
}

.pl-prov-dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  flex-shrink: 0;
  display: inline-block;
}

.pl-title {
  font-family: var(--font-display, inherit);
  font-weight: 600;
  font-size: 19px;
  margin: 2px 0 0;
  color: #fff;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.pl-top-right {
  display: flex;
  align-items: center;
  gap: 8px;
}

/* Player shell is tabindex=0 for hotkeys. Suppress the ring on mouse focus,
   but keyboard users need to perceive that the player region is focused (that
   focus is what routes Space/arrows to THIS player) — show a subtle inset ring
   only on :focus-visible (keyboard), not on click. */
.pl:focus {
  outline: none;
}
.pl:focus-visible {
  outline: none;
  box-shadow: inset 0 0 0 2px var(--cyan-a40);
}

/* Floating menu cards (source / settings / subtitles) — anchored over the
   video so they actually appear; without a positioned wrapper the bare menu
   components rendered in static flow and were invisible. */
.pl-floating {
  position: absolute;
  z-index: 12;
  border-radius: var(--r-lg, 12px);
  background: var(--card);
  border: 1px solid var(--border);
  box-shadow: 0 20px 50px var(--black-a60);
  animation: pl-pop 0.18s ease;
  overflow-y: auto;
  scrollbar-width: thin;
}

/* Source panel: larger, top-right under the header. */
.pl-floating--source {
  top: 64px;
  right: 14px;
  width: 320px;
  max-height: calc(100% - 130px);
}

/* Episodes sheet (V2b): full-width above the control bar. */
.pl-floating--sheet {
  left: 10px;
  right: 10px;
  bottom: 76px;
  max-height: calc(100% - 150px);
}

/* Settings / subtitles: compact card floating above the control-bar buttons. */
.pl-floating--btnmenu {
  right: 14px;
  bottom: 76px;
  padding: 6px;
  max-height: calc(100% - 130px);
}

@keyframes pl-pop {
  from {
    opacity: 0;
    transform: translateY(6px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

/* ── Mobile bottom sheets ── */
.pl-sheet-scrim {
  position: fixed;
  inset: 0;
  z-index: 105; /* above pseudo-FS (100), below the sheet (110) */
  background: var(--black-a60);
}

/* Double class beats every .pl-floating--* placement rule regardless of
   source order (scoped attribute + two classes). */
.pl-floating.pl-floating--mobile-sheet {
  position: fixed;
  top: auto;
  left: 0;
  right: 0;
  bottom: 0;
  width: auto;
  max-width: none;
  max-height: 72dvh;
  border-radius: 16px 16px 0 0;
  z-index: 110;
  padding-bottom: env(safe-area-inset-bottom);
  animation: pl-sheet-up 0.22s ease;
}

@keyframes pl-sheet-up {
  from {
    transform: translateY(24px);
    opacity: 0.6;
  }
  to {
    transform: translateY(0);
    opacity: 1;
  }
}

/* Fixed host for the browse-subs modal when teleported (its root is
   absolute inset-0 and needs a viewport-sized positioned ancestor). */
.pl-bsm-host {
  position: fixed;
  inset: 0;
  z-index: 110;
}

/* Mobile center pause — the touch play/pause affordance while playing. */
.pl-center-pause {
  position: absolute;
  top: 50%;
  left: 50%;
  transform: translate(-50%, -50%);
  z-index: 5;
  width: 64px;
  height: 64px;
  border-radius: 50%;
  border: 1px solid var(--white-a30);
  background: var(--scrim-bg-strong);
  color: #fff;
  display: grid;
  place-items: center;
  cursor: pointer;
}

/* Autoplay-blocked overlay's click-to-play button (in-flow inside the overlay
   column, unlike the absolutely-positioned center-pause). */
.pl-blocked-play {
  width: 72px;
  height: 72px;
  border-radius: 50%;
  border: 1px solid var(--white-a30);
  background: var(--scrim-bg-strong);
  color: #fff;
  display: grid;
  place-items: center;
  cursor: pointer;
  transition: transform 0.15s ease;
}
.pl-blocked-play:hover {
  transform: scale(1.06);
}

/* Double-tap ±10s flash */
.pl-seekflash {
  position: absolute;
  top: 50%;
  transform: translateY(-50%);
  z-index: 5;
  padding: 10px 16px;
  border-radius: 999px;
  background: var(--scrim-bg-strong);
  color: #fff;
  font-size: 14px;
  font-weight: 600;
  pointer-events: none;
  animation: pl-seekflash-in 0.18s ease;
}

.pl-seekflash--back {
  left: 12%;
}

.pl-seekflash--fwd {
  right: 12%;
}

@keyframes pl-seekflash-in {
  from {
    opacity: 0;
    transform: translateY(calc(-50% + 6px));
  }
  to {
    opacity: 1;
    transform: translateY(-50%);
  }
}

/* ── Under-player mobile action row ── */
.pl-actions {
  display: flex;
  align-items: stretch;
  gap: 8px;
  padding: 10px 16px 0;
}

.pl-actions .pl-action {
  flex: 1;
  min-height: 44px;
  justify-content: center;
}

.pl-action--src {
  min-width: 0;
}

.pl-action-srcname {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

@media (max-width: 680px) {
  /* Full-bleed: Anime.vue drops the card gutter (Task 9); square the box. */
  .pl {
    border-radius: 0;
    border-left: 0;
    border-right: 0;
  }

  /* Top bar trim: smaller title, no per-episode subtitle text, tighter pad. */
  .pl-top {
    padding: 10px 12px 28px;
    gap: 8px;
  }

  .pl-title {
    font-size: 15px;
  }

  .pl-ep-title {
    display: none;
  }

  .pl-resume {
    left: 12px;
    bottom: 76px;
  }
}
</style>
