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
  nextTick,
  onMounted,
  onUnmounted,
  toRef,
} from 'vue'
import { onClickOutside } from '@vueuse/core'
import { CircleAlert, ChevronDown, Download, ListVideo, Pause, Play, X } from 'lucide-vue-next'

import { userApi } from '@/api/client'
import { useAuthStore } from '@/stores/auth'
import { useViewerContextStore } from '@/stores/viewerContext'
import { useWatchedEpisodes } from '@/composables/useWatchedEpisodes'
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
import DebugHud, { type SeekTrace, type SourceDecision } from './overlays/DebugHud.vue'
import SkipIntroChip from './overlays/SkipIntroChip.vue'
import NextEpisodeCard from './overlays/NextEpisodeCard.vue'
import NextEpisodeChip from './overlays/NextEpisodeChip.vue'
import WatchTogetherButton from './overlays/WatchTogetherButton.vue'
import DownloadDialog from './DownloadDialog.vue'

import { useSkipTimes } from '@/composables/useSkipTimes'
import { usePlayerState } from '@/composables/aePlayer/usePlayerState'
import { usePlaybackStats } from '@/composables/aePlayer/usePlaybackStats'
import { scrubDebug } from '@/composables/aePlayer/scrubPreviewDebug'
import {
  sourceFallbackDebug,
  recordFallbackIntent,
  resetFallbackIntents,
} from '@/composables/aePlayer/sourceFallbackDebug'
import { segmentsToChapters, activeSkipSegment } from '@/composables/aePlayer/skipSegments'
import { useVideoEngine } from '@/composables/aePlayer/useVideoEngine'
import { useProviderResolver, KODIK_QUALITY_PREF_KEY } from '@/composables/aePlayer/useProviderResolver'
import { useI18n } from 'vue-i18n'
import { useWatchTracking } from '@/composables/aePlayer/useWatchTracking'
import { mapKeyToAction } from '@/composables/aePlayer/playerHotkeys'
import { pickSmartDefault, pickRawBiased, pickSelectableFallback, defaultPool } from '@/composables/aePlayer/smartDefault'
import { resolveDeepLinkProvider } from '@/composables/aePlayer/deepLinkProvider'
import { useCapabilities, flattenCapabilities } from '@/composables/aePlayer/useCapabilities'
import { rowsFromReport, groupOfProvider, playerKeyOfProvider } from '@/composables/aePlayer/useProviderFeed'
import { langsForCap, langForProviderUnderRaw } from '@/composables/aePlayer/providerGroups'
import { pickEpisodeForProvider, providerMissesTargetEpisode, shouldReselectEpisode } from '@/composables/aePlayer/episodeSelection'
import { progressRowsToMap, fmtResume, type ProgressRow } from '@/composables/aePlayer/episodeProgress'
import { useWatchPreferences } from '@/composables/useWatchPreferences'
import { useSubtitleTracks } from '@/composables/aePlayer/useSubtitleTracks'
import { pickBestForLang } from '@/composables/aePlayer/pickDefaultSubtitle'
import { useSubtitleCues } from '@/composables/aePlayer/useSubtitleCues'
import { useSubtitleAutoSyncPref } from '@/composables/aePlayer/useSubtitleAutoSyncPref'
import { useSubtitleAutoSync } from '@/composables/aePlayer/useSubtitleAutoSync'
import { comboToWatchCombo, watchComboToPartialCombo, providerToLegacyPlayer, tokenToCombo, comboToToken, clampLangForAudio } from '@/composables/aePlayer/comboMapping'
import { wtCreateSeed, type WtCreateSeed } from '@/composables/aePlayer/wtCreateSeed'
import { useWatchTogetherLaunch } from '@/composables/watch-together/useWatchTogetherLaunch'
import { useToast } from '@/composables/useToast'
import { useMobilePlayer } from '@/composables/aePlayer/useMobilePlayer'
import { recordPlayerEvent, flushPlayerTelemetry } from '@/utils/playerTelemetry'
import {
  classifyPlaybackFailure,
  mapErrorKind,
  type FailureInputs,
} from './playbackFailure'

import { usePlayerSyncBridge } from '@/composables/usePlayerSyncBridge'
import { offlineDownloadsEnabled, offlineRuntimeReady } from '@/offline/flag'
import { engineState } from '@/offline/downloadEngine'
import { useDownloadGate } from '@/offline/downloadGate'
import { seasonTargets, enqueueSeason } from '@/offline/seasonDownload'
import { listDownloads } from '@/offline/registry'
import { makeOfflineResolver, offlineCapabilityReport, pickOfflineAutoSub } from '@/offline/offlineAdapter'
import { makeExternalSubResolver, externalSubOptions } from '@/offline/externalSubs'
import { ladder, shouldDeferStallToLadder, formatLadderRows, type TierResidency } from '@/utils/protocolLadder'
import { buildProtocolUsageDetail, readVideoQuality, droppedFramesPct } from '@/composables/aePlayer/protocolUsage'
import { probeH3 } from '@/utils/probeH3'

import type { EpisodeOption } from '@/components/player/EpisodeSelector.types'
import type { StreamResult, ProviderRow, AudioKind, TrackLang, Combo } from '@/types/aePlayer'
import type { CapabilityReport, ProviderCap } from '@/types/capabilities'
import type { WatchCombo } from '@/types/preference'
import type { WatchTogetherRoomHandle } from '@/composables/useWatchTogetherRoom'
import type { DownloadState, SubPref, SubOption } from '@/offline/types'

// ─── Types ───────────────────────────────────────────────────────────────────

interface SubTrack {
  url: string
  provider: string
  lang: string
  label: string
  format: string
}

// ─── Props / Emits ───────────────────────────────────────────────────────────

const props = defineProps<{
  animeId: string
  anime: { title: string; eps: number; still?: string; durationMin?: number }
  theater: boolean
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

// ─── Watch-Together (room sync) ───────────────────────────────────────────────
// When mounted inside a WT room, wire the generic HTML5 playback bridge (mirrors
// play/pause/seek/time-tick both ways). When the room is null/undefined the
// bridge is never instantiated and the player behaves exactly as standalone.
if (props.room) {
  usePlayerSyncBridge(videoRef, props.room, { onPlayRejected: handlePlayRejection })
}

// True iff mounted inside a WT room — the room's combo is authoritative, so
// AePlayer's own auto-source-selection (Stage 1a smart default + Stage 1b
// saved-combo restore) is suppressed.
const roomPinned = computed(() => !!props.room)

// Whether the room currently pins a usable aePlayer combo. True only when the
// room's translation_id parses to a real combo token. When false (a token-less
// room, or an in-flight LEGACY room whose translation_id is a kodik id, not an
// aePlayer combo) there is nothing to adopt, so we let the normal smart-default
// resolution run — the player picks the BEST source and the broadcast watcher
// publishes it to the room. Once a combo lands this flips true and pins.
const roomHasCombo = computed(
  () => roomPinned.value && !!tokenToCombo(props.room?.room.value?.translation_id ?? ''),
)

// Guard so the combo-broadcast watcher does not echo a combo we applied FROM
// the room back TO the room. Set true around every applyRoomCombo, cleared on
// the next tick once the deep combo watcher has observed the change.
let applyingRoomCombo = false

// Guard so a room-driven episode switch does not re-emit the episode change to
// the room (which would loop forever). Mirrors OurEnglishPlayer's fromRoomSync.
let applyingRoomEpisode = false

// Parse a WT room token (carried in `translation_id`) and apply it wholesale to
// the player's combo. The room is the single source of truth for the source, so
// every field (audio/lang/team/provider/server) is set from the token.
// Hacker-mode insight: WHAT source combo is active and WHY it was chosen. Set at
// every selection site (deep-link, smart default, facet re-pick, auto-failover,
// room pin, manual). Surfaced in the DebugHud so you can see whether the player
// landed on this provider/audio/lang/team because it's the BEST pick, a pinned
// deep-link, a fallback after a failure, or your own click. Declared above
// applyRoomCombo because the immediate room watcher calls that during setup.
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

function applyRoomCombo(token: string | undefined | null) {
  if (!token) return
  const fields = tokenToCombo(token)
  if (!fields) return
  applyingRoomCombo = true
  state.combo.value = {
    audio: fields.audio,
    lang: fields.lang,
    team: fields.team,
    provider: fields.provider,
    server: fields.server,
  }
  recordDecision('pinned by the Watch-Together room')
  // Release after Vue flushes the deep combo watcher for this change so the
  // broadcast watcher sees applyingRoomCombo===true and skips the echo.
  void nextTick(() => {
    applyingRoomCombo = false
  })
}

// Apply the room's combo on mount and on every remote source switch. immediate
// so a joiner adopts the room's source before its own (non-immediate) broadcast
// watcher can fire — guaranteeing no echo of the room's own combo on join.
if (props.room) {
  watch(
    () => props.room?.room.value?.translation_id,
    (tid) => applyRoomCombo(tid),
    { immediate: true },
  )

  // Broadcast a genuine LOCAL source change to the room. NOT immediate, so the
  // initial (room-applied) combo is the watcher's baseline and never echoes.
  // Skipped while applyingRoomCombo (a remote apply) or when no real provider
  // is picked yet (provider==='' is the un-resolved initial state).
  watch(
    () => state.combo.value,
    (combo) => {
      if (!props.room || applyingRoomCombo || !combo.provider) return
      props.room.emitChangeTranslation(
        comboToToken({
          provider: combo.provider,
          audio: combo.audio,
          lang: combo.lang,
          team: combo.team,
          server: combo.server,
        }),
      )
    },
    { deep: true },
  )

  // Adopt a remote episode change. The room's episode_id carries the episode
  // NUMBER as a string for aeplayer rooms. Switch the local episode under the
  // applyingRoomEpisode guard so onSelectEpisode does not re-emit it.
  watch(
    () => props.room?.room.value?.episode_id,
    (epId) => {
      if (!epId) return
      const num = Number(epId)
      if (!Number.isFinite(num)) return
      if (selectedEpisode.value?.number === num) return
      const ep =
        episodes.value.find((e) => e.number === num) ??
        ({ key: num, label: num, number: num } as EpisodeOption)
      applyingRoomEpisode = true
      try {
        onSelectEpisode(ep)
      } finally {
        applyingRoomEpisode = false
      }
    },
  )
}

// ─── Provider health filter ───────────────────────────────────────────────────

const filter = computed(() => ({
  audio: state.combo.value.audio,
  lang: state.combo.value.lang,
  content: (props.isHentai ? 'hentai' : 'common') as 'hentai' | 'common',
}))

// The backend capability feed (/api/anime/{id}/capabilities) is the single
// source of truth for which providers the player may use/show: state, order,
// audios, group, selectable/hacker-only all arrive from it. `rows` is a pure
// derivation of the report + the active audio/lang/content filter — no FE-side
// registry, health poll, or availability probe. Disabled providers are omitted
// backend-side; `ae` with no local copy arrives as state:'no_content'.
const animeIdRef = computed(() => props.animeId)
// Guard point 2 — capability feed. Offline: a synthetic one-provider report
// ('offline'), and NO network fetch/poll fires — useCapabilities (whose
// immediate watch triggers the /capabilities GET) is never constructed. Live
// (every existing usage): identical to before — useCapabilities runs with the
// same immediate fetch, and `report`/`capMap` transparently forward its refs
// (same object identity, same reactivity timing).
const cap = props.offline ? null : useCapabilities(animeIdRef)
const report = computed<CapabilityReport | null>(() =>
  props.offline ? offlineCapabilityReport(props.offline) : (cap?.report.value ?? null),
)
const capMap = computed<Map<string, ProviderCap>>(() =>
  props.offline ? flattenCapabilities(report.value) : (cap?.capMap.value ?? new Map()),
)
const rows = computed<ProviderRow[]>(() => rowsFromReport(report.value, filter.value))

// ─── Provider defaults ────────────────────────────────────────────────────────

// props.animeId can change without a remount (no :key on the player), so the
// per-title selection state must be reset when the title changes (the ae
// library-presence check now lives backend-side, surfaced as state:'no_content').
// Reset the saved-combo fallback so the new title gets a fresh attempt.
// Reactive so the shareable-URL sync (urlSyncState) can distinguish a
// user-pinned source from an auto/smart-default one.
const providerAutoSelected = ref(false)

// Dynamic-BEST source switching: when an AUTO-selected source fails to actually
// play (a dead playlist at playback — e.g. a megaplay CDN host that 403/502s our
// IP), advance through candidate sources — untried servers of the current
// provider (megaplay hands a different CDN host per server) then the next-ranked
// providers — until one plays. "BEST" = the best source that actually works.
const triedSources = new Set<string>()
let sourceSwitchAttempts = 0
const MAX_SOURCE_SWITCHES = 5
// True from the moment an auto-failover commits the switch (setProvider/setServer)
// until the next resolve takes over (isResolving flips true). During that handoff
// the OLD <video> element can fire a native error for the source we're leaving —
// onVideoError must NOT mistake that for a dead destination and strand the
// recovering source behind "Stream unavailable".
let switchingSource = false

// Terminal playback-failure telemetry state. `attemptTrail` is the cross-source
// failover history (the engine's edgeTrail only covers per-CDN edges within one
// source); `emittedFailureKeys` de-dups so a retry streak on one broken episode
// records at most one row per (tag, episode).
const attemptTrail: Array<{ provider: string; server: string; reason: string }> = []
const emittedFailureKeys = new Set<string>()

function resetSourceSwitching() {
  triedSources.clear()
  sourceSwitchAttempts = 0
  switchingSource = false
  attemptTrail.length = 0
  emittedFailureKeys.clear()
}

// Full diagnostic bundle ("all logs"), assembled regardless of hacker mode
// (debugStats stays hacker-gated for the HUD; this is the always-on copy).
function buildDiagnosticBundle() {
  const frs = engine.fragStats.value
  const last = frs[frs.length - 1]
  // Optional chaining: servedEdge/edgeTrail are only populated for
  // Kodik/solodcdn sources (the real useVideoEngine() always defines the
  // refs — empty string for non-Kodik sources); the `?.` here is purely a
  // safety net against a degenerate/partial engine object.
  const edge = engine.servedEdge?.value ?? ''
  const trail = engine.edgeTrail?.value ?? ''
  return {
    combo: { ...state.combo.value },
    engine: {
      bw_bps: engine.bandwidthEstimate.value,
      level:
        engine.currentLevelLabel.value ||
        (currentStream.value?.type === 'mp4' ? 'mp4' : ''),
      frag_size_kb: last ? Math.round(last.size / 1024) : 0,
      frag_load_ms: last ? Math.round(last.loadMs) : 0,
      served_edge: edge,
      edge_trail: edge ? formatEdgeTrail(trail) : '',
      edge_rotations: edge && trail ? trail.split(',').length - 1 : 0,
      buffer_ahead_s: playbackStats.value?.bufferAheadSec ?? 0,
      buffer_behind_s: playbackStats.value?.bufferBehindSec ?? 0,
      video_ready_state: videoRef.value?.readyState ?? 0,
    },
    attempt_trail: attemptTrail.slice(-30),
    capability_snapshot: rows.value
      .slice(0, 40)
      .map((r) => ({ provider: r.id, group: r.group, state: r.state })),
    client: {
      ua: typeof navigator !== 'undefined' ? navigator.userAgent : '',
      viewport:
        typeof window !== 'undefined' ? `${window.innerWidth}x${window.innerHeight}` : '',
      connection:
        (typeof navigator !== 'undefined' &&
          (navigator as { connection?: { effectiveType?: string } }).connection
            ?.effectiveType) ||
        '',
    },
  }
}

// Classify the current failure; on a positive, de-duped decision, emit one
// playback_failed telemetry event with the diagnostic bundle.
function reportIfTerminal(inputs: FailureInputs) {
  const d = classifyPlaybackFailure(inputs)
  if (!d.emit || !d.tag) return
  const key = `${d.tag}:${props.animeId}:${selectedEpisode.value?.number ?? ''}`
  if (emittedFailureKeys.has(key)) return
  emittedFailureKeys.add(key)
  recordPlayerEvent({
    kind: 'playback_failed',
    provider: inputs.failingProvider,
    anime_id: props.animeId,
    episode: selectedEpisode.value?.number,
    audio: state.combo.value.audio,
    lang: state.combo.value.lang,
    error_kind: mapErrorKind(inputs.reason),
    detail: {
      schema_version: 1,
      ...buildDiagnosticBundle(),
      reason: d.tag,
      all_exhausted: d.exhausted ?? false,
      is_first_party: inputs.firstParty,
      fail_reason: inputs.reason,
    },
  })
}

watch(() => props.animeId, () => {
  providerAutoSelected.value = false
  resetSourceSwitching()
  resetFallbackIntents()
})

// Switch to the next candidate source after a playback failure. Returns true if
// it actually initiated a switch (the combo/provider watcher re-resolves), false
// otherwise. Only an AUTO-selected source is switched — a manual pick is the
// user's deliberate choice and is left alone.
//
// HACKER MODE — do NOT auto-switch. The whole point of hacker mode is to verify
// the smart-source behavior manually before trusting it to act, AND server-side
// probing can't see what actually plays in the real user's browser (a CDN host
// that 403s our datacenter may stream fine for a residential viewer). So in
// hacker mode we record the fallback the resolver WOULD make (`acted: false`)
// and stay put — letting you confirm whether the current source really plays —
// instead of thrashing away from a source that may be working. With hacker mode
// off the switch is performed and the same ledger records it with `acted: true`.
async function advanceToNextSource(reason: string): Promise<boolean> {
  const failingProvider = state.combo.value.provider
  const server = state.combo.value.server || resolvedServers.value[0]?.id || ''
  const curKey = `${failingProvider}:${server}`

  // Compute the next candidate WITHOUT committing — hacker mode only records the
  // intent, and the failure classifier needs to know if any candidate remains.
  const triedWithCurrent = new Set(triedSources)
  triedWithCurrent.add(curKey)
  const nextServer = resolvedServers.value.find(
    (s) => !triedWithCurrent.has(`${failingProvider}:${s.id}`),
  )
  let toProvider: string | null = null
  let switchServerId: string | null = null
  if (nextServer) {
    toProvider = failingProvider
    switchServerId = nextServer.id
  } else {
    const triedProviders = new Set([...triedWithCurrent].map((k) => k.split(':')[0]))
    toProvider = pickSmartDefault(rows.value.filter((r) => !triedProviders.has(r.id)))?.id ?? null
  }
  const candidateExists = Boolean(switchServerId) || Boolean(toProvider)

  // Record the cross-source failover history, then decide (from pre-mutation
  // state) whether this is a terminal, alert-worthy playback failure.
  attemptTrail.push({ provider: failingProvider, server, reason })
  reportIfTerminal({
    reason,
    failingProvider,
    hackerMode: state.hackerMode.value,
    roomPinned: roomPinned.value,
    providerAutoSelected: providerAutoSelected.value,
    candidateExists,
    attemptsExceeded: sourceSwitchAttempts >= MAX_SOURCE_SWITCHES,
    // AUTO-608: group-derived, not a literal 'ae' id check, so a second
    // first-party provider trips the same alert. The `|| failingProvider ===
    // 'ae'` safety net is NOT redundant: report.value (useCapabilities) can
    // be null/stale mid-failure — props.animeId is reactive (no :key remount
    // on route param change, see Anime.vue), so navigating to a new anime
    // while the old stream is still erroring can race the capability refetch
    // through a null report (fetch-error branch) or a stale one that doesn't
    // list the old provider. resolveStreamForEpisode's `if (!provider) return`
    // guard rules this out for the FIRST resolve of a given combo — but not
    // for a later failure (silent stall / playback fatal) on an
    // already-resolved stream once the anime id has since changed underneath
    // it. Keeping the literal check preserves today's guarantee for 'ae'
    // itself in that window; only a genuinely-new first-party id would miss
    // the net there, and it'd still resolve correctly once the new report
    // loads.
    firstParty: groupOfProvider(report.value, failingProvider) === 'firstparty' || failingProvider === 'ae',
  })

  // ── Original control flow (unchanged) ──────────────────────────────────────
  // In a Watch-Together room the source is pinned to the shared room combo:
  // per-member auto-failover would diverge members onto different streams
  // (different encodes/intros → drift sync meaningless). Today this is also
  // covered because providerAutoSelected stays false in room mode, but guard
  // explicitly so a future change to that flag can't reintroduce divergence.
  if (roomPinned.value) return false
  if (!providerAutoSelected.value) return false
  if (sourceSwitchAttempts >= MAX_SOURCE_SWITCHES) return false

  const provider = failingProvider
  if (state.hackerMode.value) {
    recordFallbackIntent({ from: provider, to: toProvider, reason, acted: false })
    const target = switchServerId ? `${provider} · server ${switchServerId}` : toProvider
    sourceError.value = target
      ? `Hacker mode: auto-switch suppressed — would try ${target} (${reason}). Pick a source manually.`
      : `Hacker mode: auto-switch suppressed — ${provider} failed (${reason}), no fallback candidate.`
    return false
  }

  if (!toProvider) return false

  // Commit the switch.
  triedSources.add(curKey)
  sourceSwitchAttempts++
  switchingSource = true // suppress the outgoing element's error during handoff
  if (switchServerId) {
    state.setServer(switchServerId) // combo watcher re-resolves the stream
  } else {
    providerAutoSelected.value = true
    state.setProvider(toProvider, '') // provider watcher re-lists + re-resolves
  }
  recordFallbackIntent({ from: provider, to: toProvider, reason, acted: true })
  recordDecision(
    switchServerId
      ? `auto-failover — switched server (${reason})`
      : `auto-failover — previous source failed (${reason})`,
  )
  return true
}

// Silent-stall watchdog. A CODECS-less HLS manifest (megaplay/owocdn) loads fine
// but hls.js then requests ZERO fragments and emits NO error — the player just
// hangs at 0:00 (the documented platform-wide codec stall). The fatal-driven
// switch can't see that. So after a stream attaches, if NO fragment has loaded
// AND playback never started within the window, treat it as a dead source and
// advance. Guarded on fragLoadedCount === 0 so a merely-slow stream (fragments
// trickling in) is NOT switched away from.
let playbackWatchdog: ReturnType<typeof setTimeout> | null = null
const PLAYBACK_WATCHDOG_MS = 12000
function clearPlaybackWatchdog() {
  if (playbackWatchdog) {
    clearTimeout(playbackWatchdog)
    playbackWatchdog = null
  }
}
function armPlaybackWatchdog() {
  clearPlaybackWatchdog()
  const tok = resolveToken
  playbackWatchdog = setTimeout(() => {
    if (tok !== resolveToken) return            // superseded by a newer resolve
    if (hasStarted.value) return                // already playing
    if (sourceError.value) return               // already errored/handled
    // Browser vetoed play() (NotAllowedError): the stream is fine, only the
    // START was blocked. Without this guard the watchdog misreads "blocked" as
    // "stalled" — especially on native MP4 sources where fragLoadedCount stays
    // 0 — and churns through every provider pulling gigabytes for nothing.
    if (playbackBlocked.value) return
    // A first fragment that is downloading (bytes flowing) is SLOW, not dead —
    // aborting it re-resolves the same source forever (the 2026-07-11 tNeymik
    // "stale" loop: seg0 restarted 3×, video never possible). Let the ladder's
    // projected-too-slow rule downshift the tier instead; just re-arm.
    if (shouldDeferStallToLadder(ladder.inflight())) {
      armPlaybackWatchdog()
      return
    }
    if (engine.fragLoadedCount.value > 0) return // fragments flowing — just slow
    void (async () => {
      if (await advanceToNextSource('silent stall')) {
        toast.push(t('player.aePlayer.switchNotPlaying'), 'info', 4000)
      } else if (!sourceError.value) {
        sourceError.value = t('player.aePlayer.streamUnavailable')
      }
    })()
  }, PLAYBACK_WATCHDOG_MS)
}

// ─── Saved-combo restore (Stage 1b) ──────────────────────────────────────────
// Resolve the user's saved watch combo first; the Stage 1a smart default is
// gated on `preferenceSettled` so the saved pick always wins when present.

const preferenceSettled = ref(false)
const { resolve: resolvePreference, resolvedCombo } = useWatchPreferences(props.animeId)

function applyResolvedCombo() {
  const rc = resolvedCombo.value
  if (!rc || state.combo.value.provider) return
  // Restore the user's saved audio/lang/team PREFERENCES only — NOT the source.
  // Per product rule, the selected source (BEST) is a deterministic function of
  // first-party availability + third-party stats, so the smart default (below)
  // always picks it; a previously-watched source must not override it.
  const { audio, lang, team } = watchComboToPartialCombo(rc)
  // setAudio/setLang each reset team → null, so setTeam must come AFTER them.
  state.setAudio(audio)
  state.setLang(clampLangForAudio(audio, lang)) // no Japanese dub → dub/ja becomes dub/en
  if (team) state.setTeam(team)
}

// Notification / shared-link `?provider=` override: pin that source BEFORE the
// smart default runs. Honored for any real, content-compatible, non-static-
// disabled provider def (coarse/legacy/unavailable values fall through to the
// smart default). Runs after applyResolvedCombo so initialTeam wins over the
// saved-pref team, and after setAudio/setLang (which reset team → null) so the
// team sticks.
//
// CRUCIAL: clamp audio/lang to what the provider serves. A row is only `active`
// (and thus pinnable) when it matches the live audio/lang/content filter, and
// the default combo is sub/en — so a ?provider=kodik (RU) pin would otherwise
// land an `irrelevant` row and silently fall through to BEST. Switching the
// facet first makes the deep-linked row relevant so the explicit choice holds.
function applyInitialProvider() {
  if (state.combo.value.provider) return
  const pin = resolveDeepLinkProvider(
    props.initialProvider,
    state.combo.value,
    props.isHentai ? 'hentai' : 'common',
    capMap.value,
  )
  if (!pin) return
  providerAutoSelected.value = false // user-intent pin, not an auto-selection
  // setAudio/setLang each reset team → null, so they must come BEFORE setProvider
  // + setTeam (which is why initialTeam is applied last).
  state.setAudio(pin.audio)
  state.setLang(pin.lang)
  state.setProvider(pin.provider, '')
  if (props.initialTeam) state.setTeam(props.initialTeam)
  recordDecision('deep-link — pinned from the ?provider/?team URL')
}

// URL facet override (?audio=raw|sub|dub, ?lang=en|ru|ja) — applied after the
// saved combo, before the ?provider clamp. Precedence: URL > saved > smart default.
function applyUrlFacet() {
  if (state.combo.value.provider) return
  const a = props.initialAudio
  if (a === 'dub') state.setAudio('dub')
  else if (a === 'raw' || a === 'sub') state.setAudio('sub')
  const l = props.initialLang
  if (l === 'en' || l === 'ru' || l === 'ja') state.setLang(clampLangForAudio(state.combo.value.audio, l))
}

// Enumerate EVERY real source's facet (across all families, NOT just the rows
// matching the current audio/lang filter) so a saved/URL combo in any
// language/audio can be matched. The old version iterated the facet-filtered
// `rows` — which at mount only carry the default RAW/EN options — so every saved
// preference (ru/dub/ja) failed to match and the player collapsed to SUB EN.
const buildAvailable = (): WatchCombo[] => {
  const combos: WatchCombo[] = []
  const seen = new Set<string>()
  const rep = report.value
  if (!rep || !Array.isArray(rep.families)) return combos
  for (const fam of rep.families) {
    for (const cap of fam.providers ?? []) {
      if (cap.state === 'no_content') continue
      const player = providerToLegacyPlayer(cap.provider, cap.group, cap.player_key)
      if (!player) continue
      // A cap's real per-title `lang` (Phase C source-panel truth — set only
      // for ae's probed dub) overrides the group's default language set, so
      // e.g. an ae English dub routes under EN only, not every language
      // `firstparty` nominally serves. Caps without `lang` are unchanged.
      const langs = langsForCap(cap)
      const audios = [...new Set((cap.audios ?? []).map((a) => (a === 'dub' ? 'dub' : 'sub')))]
      for (const audio of audios) {
        for (const language of langs) {
          const key = `${player}:${audio}:${language}`
          if (seen.has(key)) continue
          seen.add(key)
          combos.push({
            player,
            language: language as WatchCombo['language'],
            watch_type: audio as WatchCombo['watch_type'],
            translation_id: '',
            translation_title: '',
          })
        }
      }
    }
  }
  return combos
}

// one-shot latch (non-reactive on purpose — read/written only inside the watcher)
let resolveAttempted = false
watch(rows, () => {
  // WT: when the room pins a usable combo it is authoritative — never run the
  // saved-combo restore. A token-less / legacy room has nothing to pin, so we
  // fall through and resolve normally (BEST source + saved audio/lang).
  if (roomHasCombo.value) return
  if (resolveAttempted) return
  const available = buildAvailable()
  if (available.length === 0) return
  resolveAttempted = true
  resolvePreference(available).finally(() => {
    applyResolvedCombo()
    applyUrlFacet()
    applyInitialProvider()
    preferenceSettled.value = true
  })
}, { immediate: true })

watch(
  [rows, preferenceSettled],
  () => {
    // WT: a room that pins a usable combo suppresses the smart default. A
    // token-less / legacy room resolves BEST and broadcasts it (see roomHasCombo).
    if (roomHasCombo.value) return
    if (state.combo.value.provider) return
    if (!preferenceSettled.value) return // let saved prefs (audio/lang) settle first
    const pick = pickFacetDefault()
    if (pick && !state.combo.value.provider) {
      providerAutoSelected.value = true
      state.setProvider(pick.id, '')
      recordDecision('smart default — best available source')
    }
  },
  { immediate: true },
)

// Best provider for the current facet: language-biased under RAW (don't cross
// language when a same-language source exists), plain best under DUB, with a
// dead-player fallback to the top-ranked SELECTABLE (degraded) row so a
// fully-degraded fleet still attempts playback instead of dead-ending.
//
// When no specific episode was requested (a fresh / first-time open, initial
// episode ≤ 1), ae's partial first-party library must NOT win the default — it
// may hold only a late auto-cached episode and would open that instead of
// episode 1. `defaultPool` drops firstparty in that case (unless ae is the only
// playable source). A resume / deep-link to a real episode (> 1) keeps ae
// eligible; ae is always still MANUALLY selectable in the Source panel.
function pickFacetDefault(): ProviderRow | null {
  const episodeSpecified = (props.initialEpisode ?? 1) > 1
  const pool = defaultPool(rows.value, episodeSpecified)
  const primary =
    state.combo.value.audio === 'sub'
      ? pickRawBiased(pool, state.combo.value.lang)
      : pickSmartDefault(pool)
  return primary ?? pickSelectableFallback(rows.value)
}

// Under RAW (audio:'sub') the language slider is hidden — derive combo.lang from
// the chosen provider's group so persistence + the subtitle menu stay correct.
// setServedLang preserves team; the facet watcher ignores RAW lang changes, so
// this never churns the source.
watch(
  () => state.combo.value.provider,
  (id) => {
    if (!id || state.combo.value.audio !== 'sub') return
    const row = rows.value.find((r) => r.id === id)
    if (!row) return
    const want: TrackLang = langForProviderUnderRaw(row.group, state.combo.value.lang)
    if (want !== state.combo.value.lang) state.setServedLang(want)
  },
)

// ─── Active provider display info ────────────────────────────────────────────
// Name from the capability feed (display_name) → row label → raw id. Cosmetics
// are state-driven, not per-provider: the active dot is always brand cyan.

const activeProviderName = computed(() => {
  const id = state.combo.value.provider
  return capMap.value.get(id)?.display_name ?? rows.value.find((r) => r.id === id)?.label ?? id ?? ''
})

const activeProviderHue = computed(() => 'var(--brand-cyan)')

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

// ─── Episode list + stream resolution ────────────────────────────────────────

const episodes = ref<EpisodeOption[]>([])
const selectedEpisode = ref<EpisodeOption | null>(null)

// Test seam (see AePlayer.*.spec.ts): expose the live combo ref + a setter so WT
// room-sync specs can assert the applied/pinned combo and simulate a genuine
// local source change without mocking usePlayerState (which hands every caller an
// independent instance), plus the selected episode for next-episode specs. Placed
// here so selectedEpisode is in scope; defineExpose runs once. __-prefixed keys
// are never read in prod.
defineExpose({
  __combo: state.combo,
  __setProvider: state.setProvider,
  __selectedEpisode: selectedEpisode,
  onSelectEpisode,
})
// True once the user manually switches episodes — freezes the reactive
// initialEpisode re-pick so resume resolving late never yanks a deliberate pick.
const userPickedEpisode = ref(false)

// ─── User watch data (read-only): watched marks + per-episode progress ───────

const auth = useAuthStore()
const { watchedUpTo, refresh: refreshWatched } = useWatchedEpisodes(() => props.animeId)

const epProgress = ref<Record<number, { pct: number; sec: number; completed: boolean }>>({})

// Page-fetch optimization (2026-06-11): the FIRST load per anime consumes the
// viewer-context aggregate Anime.vue already fetched, killing the duplicate
// GET /users/progress/{id} on mount. Post-mutation reloads go to the network.
let progressPrefetchConsumedFor: string | null = null

async function loadEpisodeProgress() {
  if (!auth.isAuthenticated) {
    epProgress.value = {}
    return
  }
  if (progressPrefetchConsumedFor !== props.animeId) {
    progressPrefetchConsumedFor = props.animeId
    // whenLoaded (not forAnime): on deep-link mounts the aggregate request is
    // often still in flight — await it instead of duplicating the fetch.
    const ctx = await useViewerContextStore().whenLoaded(props.animeId)
    if (ctx) {
      epProgress.value = progressRowsToMap(ctx.progress ?? [])
      return
    }
  }
  try {
    const res = await userApi.getProgress(props.animeId)
    const rows = (res.data?.data ?? res.data ?? []) as ProgressRow[]
    epProgress.value = progressRowsToMap(rows)
  } catch {
    // 404 / anonymous / network — no user data, panel renders plain numbers
    epProgress.value = {}
  }
}

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

// ─── Watch-Together create seed ──────────────────────────────────────────────
// The live combo + current episode as a create-room seed (null until a usable
// source resolves). Drives BOTH the in-player launch button (below) and the
// anime-page Invite button (via the combo-change emit).
const currentWtSeed = computed<WtCreateSeed | null>(() =>
  wtCreateSeed(state.combo.value, selectedEpisode.value?.number ?? 0),
)

// Outside a room, surface the seed to the parent (Anime.vue) so the Invite
// button can CREATE the room AS aeplayer seeded with exactly this source.
// Suppressed in a room — there the room is authoritative and the broadcast
// watcher above already syncs the combo. immediate so the parent has the
// latest seed (or null) before the user can click Invite.
watch(
  currentWtSeed,
  (seed) => {
    if (props.room) return
    emit('combo-change', seed)
  },
  { immediate: true },
)

// ─── Shareable-URL sync ──────────────────────────────────────────────────────
// Reflect the live source/team/episode so the parent view can mirror them into
// `?provider/?team/?episode` (shareable + bookmarkable links). DONE RIGHT after
// the 2026-06-21 revert: provider/team are emitted ONLY for a USER-PINNED source
// (manual pick or `?provider=` deep-link → providerAutoSelected=false). For an
// auto/smart-default selection they are emitted EMPTY, so the parent strips them
// and a plain reload re-runs the deterministic BEST default (the product rule a
// previously-watched source must not override). Suppressed inside a WT room.
const urlSyncState = computed(() => {
  const ep = selectedEpisode.value?.number ?? 0
  const pinned = !providerAutoSelected.value && !!state.combo.value.provider
  return {
    provider: pinned ? state.combo.value.provider : '',
    team: pinned ? (state.combo.value.team ?? '') : '',
    episode: ep > 0 ? ep : 0,
  }
})
// Gate on preferenceSettled: applyInitialProvider() (which CONSUMES the
// `?provider=` deep-link from props) runs in resolvePreference().finally() right
// before preferenceSettled flips true. Emitting earlier would let the initial
// empty/auto state strip the deep-link param from the URL before the read path
// reads it. Once settled, every later source/team/episode change syncs through.
watch(
  [preferenceSettled, urlSyncState],
  ([settled, s]) => {
    if (props.room) return
    if (!settled) return
    emit('url-sync', s as { provider: string; team: string; episode: number })
  },
  { immediate: true },
)

// ─── In-player Watch-Together launch button ──────────────────────────────────
// Shown only standalone (not inside a room) and only to authenticated users —
// creating a room requires a JWT, and inside a room the RoomSidebar owns
// invites. Disabled until a usable source resolves; clicking creates the room
// from the live combo and routes to it (same flow as the anime-page Invite).
const { launching: wtLaunching, launch: launchWt } = useWatchTogetherLaunch()
const showWtLaunch = computed(() => !props.room && auth.isAuthenticated)

async function onLaunchWt(): Promise<void> {
  const seed = currentWtSeed.value
  if (!seed) return
  await launchWt({
    animeId: props.animeId,
    episodeId: seed.episode_id,
    player: seed.player,
    translationId: seed.translation_id,
  })
}

/** Whether the user already has this episode marked watched (drawer data). */
function isEpisodeWatched(n: number): boolean {
  return n <= watchedUpTo.value || !!epProgress.value[n]?.completed
}

/** Manual mark from the episodes drawer (Kodik-parity button). */
function onMarkWatched() {
  void tracking.markWatched()
}

// ─── Resume-from-saved-position chip ─────────────────────────────────────────
// Saved position for the current episode: server watch_progress first (logged
// in), localStorage fallback (anonymous parity with KodikPlayer). The chip
// offers the jump — it never auto-seeks.

const resumeChipDismissed = ref(false)
const resumeChipUsed = ref(false)

// Shared-link `?t=` → seek the video to this position on the FIRST stream load,
// once. While it is pending it also suppresses the passive resume chip below:
// the sharer's explicit position wins over the viewer's own saved progress for
// this load. Cleared (→ 0) the moment the seek is applied, restoring normal
// resume-chip behavior for every later episode.
const initialSeekSec = ref(Math.max(0, props.initialTimestamp ?? 0))
function applyInitialSeek() {
  if (initialSeekSec.value <= 0) return
  const target = initialSeekSec.value
  initialSeekSec.value = 0 // consume once
  const v = videoRef.value
  if (!v) return
  const seek = () => {
    try {
      v.currentTime = target
    } catch {
      /* element not seekable yet — best-effort */
    }
  }
  if (v.readyState >= 1) seek()
  else v.addEventListener('loadedmetadata', seek, { once: true })
}

function localResumeSec(ep: number): number {
  try {
    const data = JSON.parse(localStorage.getItem(`watch_progress:${props.animeId}`) || '{}')
    return Number(data[ep]?.time) || 0
  } catch {
    return 0
  }
}

const resumePosSec = computed(() => {
  const ep = selectedEpisode.value?.number
  if (!ep) return 0
  const server = epProgress.value[ep]
  if (server && !server.completed && server.sec > 0) return server.sec
  if (!auth.isAuthenticated) return localResumeSec(ep)
  return 0
})

const resumeChipVisible = computed(() => {
  if (initialSeekSec.value > 0) return false // shared-link position takes over
  if (resumeChipDismissed.value || resumeChipUsed.value) return false
  if (sourceError.value || isResolving.value) return false
  const pos = resumePosSec.value
  if (pos < 30) return false // too little progress to bother
  // Once near the end the next-episode flow takes over instead.
  if (duration.value > 0 && pos >= 0.95 * duration.value) return false
  // The offer expires once the user has clearly chosen to watch from here.
  if (hasStarted.value && currentTime.value > 5) return false
  return true
})

function onResumeFromSaved() {
  const v = videoRef.value
  if (!v) return
  resumeChipUsed.value = true
  v.currentTime = resumePosSec.value
  if (v.paused) attemptPlay()
  writeProgress()
}

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
const sourceError = ref<string | null>(null)

// ── Autoplay-blocked state ────────────────────────────────────────────────────
// Any browser can reject video.play() with NotAllowedError — strict autoplay
// policies (Firefox "Block Audio and Video", Chrome's engagement heuristics,
// Safari power-saving), blocker extensions, or a play() that lands outside the
// user-gesture window (our resolves are async, so the click's activation can
// expire before play() runs). The media itself is FINE — segments/bytes load —
// only STARTING is vetoed. These rejections used to be swallowed
// (`void v.play()`), so affected users saw a dead player while the stall
// watchdog churned through every source. All play() calls now funnel through
// attemptPlay(); a NotAllowedError raises this dedicated overlay and must
// NEVER count as a dead source.
const playbackBlocked = ref(false)
// Second consecutive rejection (the overlay's own button also got vetoed) →
// show the browser-permission hint.
const playbackBlockedHint = ref(false)
let blockReported = false // one playback_start_rejected event per resolve

function handlePlayRejection(err: unknown) {
  const name = err instanceof Error ? err.name : ''
  // AbortError (play() interrupted by a load/pause during source swaps) and
  // friends are benign lifecycle noise — only a browser start-veto matters.
  if (name !== 'NotAllowedError') return
  if (playbackBlocked.value) playbackBlockedHint.value = true
  playbackBlocked.value = true
  if (!blockReported) {
    blockReported = true
    recordPlayerEvent({
      kind: 'playback_start_rejected',
      provider: state.combo.value.provider,
      anime_id: props.animeId,
      episode: selectedEpisode.value?.number,
      error_kind: name,
      audio: state.combo.value.audio,
      lang: state.combo.value.lang,
    })
  }
  const msg = err instanceof Error && err.message ? `: ${err.message}` : ''
  console.warn(`[AePlayer] play() rejected — ${name}${msg}`)
}

function resetPlaybackBlocked() {
  playbackBlocked.value = false
  playbackBlockedHint.value = false
  blockReported = false
}

// The single sanctioned way to start playback. Success clears the blocked
// overlay (the user allowed autoplay / the veto lifted); a veto raises it.
function attemptPlay() {
  const v = videoRef.value
  if (!v) return
  v.play().then(
    () => {
      playbackBlocked.value = false
      playbackBlockedHint.value = false
    },
    handlePlayRejection,
  )
}

const resolvedServers = ref<{ id: string; label: string }[]>([])
const teams = ref<string[]>([])
// Latest-wins guard for the team chips, independent of resolveToken: the team
// list is (re)loaded both on provider switch and on a same-provider audio
// toggle, so it needs its own epoch so a slow earlier fetch can't overwrite a
// newer one.
let teamsToken = 0
const currentStream = ref<StreamResult | null>(null)
const isResolving = ref(false)

// Monotonically-increasing request token — only the latest resolve applies.
// Prevents a stale audio/lang/server re-resolve from clobbering a concurrent
// provider-change full re-list+resolve that started after it.
let resolveToken = 0

// ─── Telemetry timing state ───────────────────────────────────────────────────
// Best-effort; never influences playback logic.
let resolveStartedAt = 0
let reachedReported = false
let stallStartedAt = 0

// ─── Quality ladder ──────────────────────────────────────────────────────────
// HLS: data-driven from hls.js levels. MP4: only when the provider returned
// multiple URL-valued qualities. Per-URL HLS (Kodik: one manifest per quality,
// numeric values): switching re-resolves the stream instead of changing an
// hls.js level. Single-variant streams stay Auto-only (D-05).
// NOTE: declared after currentStream — watch() below evaluates its computed
// source eagerly during setup.

const mp4Qualities = computed(() => {
  const s = currentStream.value
  if (!s || s.type !== 'mp4' || !s.qualities) return []
  return s.qualities.filter(
    (q) => typeof q.value === 'string' && /^(https?:|\/)/.test(q.value as string),
  )
})

const perUrlHlsQualities = computed(() => {
  const s = currentStream.value
  if (!s || s.type !== 'hls' || !s.qualities) return []
  return s.qualities.filter((q) => typeof q.value === 'number')
})

const qualities = computed(() => {
  if (mp4Qualities.value.length > 1) return ['Auto', ...mp4Qualities.value.map((q) => q.label)]
  if (perUrlHlsQualities.value.length > 1) {
    return ['Auto', ...perUrlHlsQualities.value.map((q) => q.label)]
  }
  return ['Auto', ...engine.levels.value.map((l) => l.label)]
})

// While auto-switching, show what is actually playing: hls.js's current level,
// or for per-URL ladders the quality the provider reported serving.
const qualityDisplay = computed(() => {
  const served = engine.currentLevelLabel.value || currentStream.value?.qualityLabel
  return state.quality.value === 'Auto' && served
    ? `Auto · ${served}`
    : state.quality.value
})

// New stream may not offer the previously-chosen quality — snap back to Auto.
// If it DOES offer it, re-apply: each load() creates a fresh hls instance
// that starts at auto, so a pinned level must be re-pinned. (Per-URL ladders
// need no re-apply — the resolved URL already carries the pinned quality.)
watch(qualities, (qs) => {
  if (!qs.includes(state.quality.value)) {
    state.quality.value = 'Auto'
  } else if (
    state.quality.value !== 'Auto' &&
    mp4Qualities.value.length === 0 &&
    perUrlHlsQualities.value.length === 0
  ) {
    engine.setLevel(state.quality.value)
  }
})

function swapMp4Source(url: string) {
  const v = videoRef.value
  if (!v) return
  const t = v.currentTime
  const wasPlaying = !v.paused
  v.addEventListener(
    'loadedmetadata',
    () => {
      v.currentTime = t
      if (wasPlaying) attemptPlay()
    },
    { once: true },
  )
  v.src = url
}

function onSetQuality(q: string) {
  state.quality.value = q
  const mq = mp4Qualities.value.find((x) => x.label === q)
  if (mq) {
    swapMp4Source(mq.value as string)
    return
  }
  if (q === 'Auto' && currentStream.value?.type === 'mp4') {
    // mp4 has no auto ladder — Auto = the originally-resolved URL
    swapMp4Source(currentStream.value.url)
    return
  }
  if (perUrlHlsQualities.value.length > 0) {
    // Per-URL ladder (Kodik): persist the choice (the adapter reads it on the
    // next resolve), then re-resolve the stream at the new quality in place.
    const pq = perUrlHlsQualities.value.find((x) => x.label === q)
    if (pq) localStorage.setItem(KODIK_QUALITY_PREF_KEY, String(pq.value))
    else if (q === 'Auto') localStorage.removeItem(KODIK_QUALITY_PREF_KEY)
    // Same episode, new manifest URL → resolveStreamForCurrentEpisode keeps the
    // playhead (keepPosition) so the quality swap is seamless.
    void resolveStreamForCurrentEpisode()
    return
  }
  engine.setLevel(q)
}

// Restore the playhead (and play state) after a stream swap that stayed on the
// same episode — a provider/facet/quality switch must not dump the viewer back
// to 0:00. Restoring the real position ALSO stops the regressed near-zero
// playhead from clobbering saved watch-progress on the next heartbeat/beacon
// (both persist the CURRENT playhead, not the max), which is why "resume from
// the same moment" vanished after a mid-watch source switch. No-op when there's
// nothing to restore (a fresh load sits at 0).
function restorePlayhead(t: number, wasPlaying: boolean) {
  const v = videoRef.value
  if (!v || t <= 0.5) return
  const restore = () => {
    try {
      v.currentTime = t
      // Keep the reactive clock in step with the seeked element: resetPlaybackClock
      // zeroed it for the swap, and a PAUSED preserve fires no rAF/writeProgress to
      // re-sync, so the scrub bar / time pill would otherwise read 0:00 at position t.
      currentTime.value = t
    } catch {
      /* not seekable yet — best-effort */
    }
    if (wasPlaying) attemptPlay()
  }
  if (v.readyState >= 1) restore()
  else v.addEventListener('loadedmetadata', restore, { once: true })
}

// Snapshot the playhead + play state to hand to restorePlayhead after a
// same-episode stream swap. `keep` is false for a genuine episode change (start
// fresh at 0); wasPlaying is gated on a real position so a fresh 0:00 load never
// auto-resumes. Prefer engine.lastKnownPlayback when a fatal error just fired:
// hls.js's destroy() already reset videoRef's own currentTime to 0 by the time
// a retry/failover reaches here (see useVideoEngine's snapshotPlayback), so the
// live element is not a safe read in that case.
function capturePlayhead(keep: boolean): { restoreT: number; wasPlaying: boolean } {
  if (!keep) return { restoreT: 0, wasPlaying: false }
  const salvaged = engine.lastKnownPlayback.value
  const restoreT = salvaged?.time ?? videoRef.value?.currentTime ?? 0
  const wasPlaying = restoreT > 0 && (salvaged?.wasPlaying ?? state.playing.value)
  return { restoreT, wasPlaying }
}

// Initialize selectedEpisode from initialEpisode (NEVER episodesAired).
function initSelectedEpisode() {
  const targetEp = props.initialEpisode ?? 1
  const found = episodes.value.find((e) => e.number === targetEp)
  if (found) {
    selectedEpisode.value = found
  } else if (episodes.value.length > 0) {
    selectedEpisode.value = episodes.value[0]
  } else {
    // Synthetic placeholder for when the episode list isn't loaded yet.
    selectedEpisode.value = { key: targetEp, label: targetEp, number: targetEp }
  }
}

// Resume / watch-progress resolves asynchronously AFTER mount, so initialEpisode
// flips from its default (1) to e.g. lastWatched+1 a tick later. Re-pick then —
// unless the user already chose an episode. Closes the mount race that used to
// fall back to episodesAired and open the latest aired episode instead of ep 1.
watch(
  () => props.initialEpisode,
  () => {
    if (shouldReselectEpisode(selectedEpisode.value?.number ?? null, props.initialEpisode, userPickedEpisode.value)) {
      initSelectedEpisode()
    }
  },
)

// Load the provider-native team chips (e.g. Kodik translation titles) for the
// CURRENT audio facet. Kodik exposes different teams for sub vs dub, so the list
// is scoped to state.combo.audio — otherwise SUB shows a wall of DUB teams.
// Best-effort and self-epoched (teamsToken) so it never blocks or races a
// stream resolve.
function loadTeams(provider: string) {
  const tok = ++teamsToken
  teams.value = [] // clear stale chips immediately
  resolver
    .listTeams(provider, props.animeId, state.combo.value.audio)
    .then((t) => { if (tok === teamsToken) teams.value = t })
    .catch(() => { if (tok === teamsToken) teams.value = [] })
}

// Reload the team chips when the audio facet flips on the SAME provider (a
// sub↔dub toggle on Kodik). Provider switches reload teams via
// loadEpisodesAndStream; this covers the in-place toggle so the chips track the
// selected audio. Guarded on a real provider being selected.
watch(() => state.combo.value.audio, () => {
  const provider = state.combo.value.provider
  if (provider) loadTeams(provider)
})

async function loadEpisodesAndStream() {
  const provider = state.combo.value.provider
  if (!provider) return

  // Playhead-preservation: capture the episode the viewer is on BEFORE the
  // re-list (which may reassign selectedEpisode). If the switched-to source
  // still serves this episode, the playhead is restored after load (below) so a
  // mid-watch source switch doesn't restart at 0:00 or regress saved progress.
  const keepEpNum = selectedEpisode.value?.number ?? null

  sourceError.value = null
  isResolving.value = true
  switchingSource = false // resolve owns error handling now — handoff window over
  hasStarted.value = false
  resetPlaybackBlocked() // new stream = a fresh autoplay verdict
  // Re-listing a provider: its servers are unknown until the resolve below, so
  // drop the previous provider's stale set. This also keeps advanceToNextSource's
  // server-first dodge from targeting a nonexistent server of the new provider.
  resolvedServers.value = []
  const token = ++resolveToken
  resolveStartedAt = performance.now()
  reachedReported = false
  stallStartedAt = 0

  try {
    // Load episode list
    const eps = await resolver.listEpisodes(provider, props.animeId)
    if (token !== resolveToken) return // superseded by a later request

    episodes.value = eps

    // Which episode does the viewer want? Resume-resolved selection, else the
    // initial/first episode.
    const targetNum =
      selectedEpisode.value?.number ?? props.initialEpisode ?? 1

    // Episode-aware auto-default: a source that was AUTO-selected (smart default
    // or failover) but doesn't actually carry the episode the viewer wants is the
    // wrong source for them — ae's partial library can hold only ep 27 while a
    // first-time viewer wants ep 1, and pickEpisodeForProvider would otherwise
    // snap them UP to ep 27. Advance to the next-best source instead of silently
    // playing a different episode. A manual pick (providerAutoSelected=false) is
    // respected as-is; an exhausted / hacker-suppressed / room-pinned advance
    // falls through and plays what this source does have rather than dead-ending.
    // Runs BEFORE loadTeams so a source we're about to abandon never fires a
    // wasted teams fetch.
    if (providerAutoSelected.value && providerMissesTargetEpisode(eps, targetNum)) {
      if (await advanceToNextSource('source missing the requested episode')) return
    }

    // Provider-native teams (e.g. Kodik translation titles) for the Source
    // panel, scoped to the current audio facet. Best-effort — never blocks the
    // stream resolve.
    loadTeams(provider)

    // Preserve the selected episode across provider changes: keep the same
    // episode NUMBER when the new source has it, and never snap back to EP 1
    // when it doesn't (pickEpisodeForProvider handles the nearest-fallback).
    const ep = pickEpisodeForProvider(eps, targetNum, selectedEpisode.value)

    if (!ep) {
      sourceError.value = t('player.aePlayer.noEpisodes')
      return
    }

    selectedEpisode.value = ep

    // Resolve stream
    const stream = await resolver.resolveStream(
      provider,
      props.animeId,
      ep,
      state.combo.value,
    )

    if (token !== resolveToken) return // superseded

    resolvedServers.value = stream.servers ?? []
    // Set BEFORE the await: a superseded resolve must never clobber the
    // winner's stream descriptor after resuming from engine.load.
    currentStream.value = stream
    applyOfflineAutoSub(ep.number, stream)
    // Restore the playhead only when the switched-to source stayed on the same
    // episode — a provider change that lands on a different episode starts fresh.
    const { restoreT, wasPlaying } = capturePlayhead(keepEpNum !== null && ep.number === keepEpNum)
    resetPlaybackClock() // drop the outgoing source's playhead before the swap
    await engine.load(stream)
    applyInitialSeek() // shared-link `?t=` one-shot seek (no-op after first load)
    restorePlayhead(restoreT, wasPlaying)
    armPlaybackWatchdog() // catch a silent CODECS-less stall (manifest OK, no frags)
  } catch (err: unknown) {
    if (token !== resolveToken) return // superseded
    const isNotAvailable =
      err instanceof Error && err.name === 'NotAvailableError'
    // Telemetry: resolve failure (best-effort, never throws)
    recordPlayerEvent({
      kind: 'resolve',
      provider: state.combo.value.provider,
      anime_id: props.animeId,
      episode: selectedEpisode.value?.number,
      outcome: 'fail',
      reached_playback: false,
      error_kind: isNotAvailable ? 'not_available' : 'stream_error',
      latency_ms: resolveStartedAt ? Math.round(performance.now() - resolveStartedAt) : undefined,
      audio: state.combo.value.audio,
      lang: state.combo.value.lang,
    })
    // Any resolve failure (not-available OR HTTP/stream error like allanime's
    // 500) advances the dynamic-BEST chain to the next candidate, so it keeps
    // going until a source actually resolves AND plays — never strands on a
    // dead provider.
    if (await advanceToNextSource('resolve failed')) {
      toast.push(t('player.aePlayer.switchNotAvailable'), 'info', 4000)
      return
    }
    // advanceToNextSource may have set a hacker-mode "suppressed" message — keep it.
    if (!sourceError.value) {
      sourceError.value = isNotAvailable ? t('player.aePlayer.sourceUnavailable') : t('player.aePlayer.streamUnavailable')
    }
  } finally {
    if (token === resolveToken) {
      isResolving.value = false
    }
  }
}

// Provider change → full re-list + resolve (episodes are provider-specific)
watch(
  () => state.combo.value.provider,
  (newProvider) => {
    if (newProvider) {
      void loadEpisodesAndStream()
    }
  },
)

// Audio / language / server / team change, or episode selection → re-resolve stream
// only (no need to re-list episodes). The token inside resolveStreamForEpisode
// ensures a concurrent provider-change full-resolve wins if they race.
// Skip when loadEpisodesAndStream is in-flight: it sets selectedEpisode itself
// and will call resolveStream at the end — we must not fire a duplicate.
watch(
  () => [
    state.combo.value.audio,
    state.combo.value.lang,
    state.combo.value.server,
    state.combo.value.team,
    selectedEpisode.value,
  ] as const,
  (newVal, oldVal) => {
    // Skip the very first run (oldVal is undefined on initial watch fire)
    if (oldVal === undefined) return
    // Skip if a full re-list is already in progress (provider changed)
    if (isResolving.value) return
    // Skip an EPISODE change while the episode list hasn't loaded: that's the
    // mount-time null→synthetic-placeholder transition from
    // initSelectedEpisode, whose key is a bare episode number — scraper
    // episode ids are opaque, so resolving it fires a doomed
    // scraper/servers?episode=<number> request (seen in prod HARs on
    // ?episode=N deep links). loadEpisodesAndStream reconciles the selection
    // and resolves the stream itself once the real list arrives. Combo
    // (audio/lang/server) changes are NOT gated on the list.
    if (newVal[4] !== oldVal[4] && episodes.value.length === 0) return
    // An AUDIO or LANGUAGE change is a facet change: the current provider may no
    // longer serve it (e.g. switching to English while on Kodik, which is
    // RU-only). When it can't, drop the current stream and re-pick the best
    // provider for the new facet — refreshing episodes/servers/teams — instead
    // of silently re-resolving a provider that has no such stream.
    const audioChanged = newVal[0] !== oldVal[0]
    const langChanged = newVal[1] !== oldVal[1]
    // Under RAW the language slider is hidden — combo.lang is a DERIVED value that
    // follows the chosen provider (see the lang-follows-provider watcher below).
    // A RAW-only lang change must NOT repick or re-resolve (it would churn the
    // source); language is a real facet only under DUB.
    if (!audioChanged && langChanged && newVal[0] === 'sub') return
    const facetChanged = audioChanged || (newVal[0] === 'dub' && langChanged)
    if (facetChanged && !roomPinned.value) {
      void repickProviderForFacet()
      return
    }
    void resolveStreamForCurrentEpisode()
  },
)

// Re-pick the provider after an audio/language change. If the current provider
// still serves the new facet, keep it and just re-resolve (so a sub↔dub toggle
// on a provider that has both stays put). Otherwise drop the now-wrong stream
// and switch to the top-ranked provider that serves the new facet, which
// triggers a full re-list + team/server refresh via the provider watcher.
function repickProviderForFacet() {
  // rows is a computed over the live audio/lang filter — already reflects the
  // just-changed facet, no manual recompute needed.
  const cur = state.combo.value.provider
  const curStillActive = rows.value.some((r) => r.id === cur && r.state === 'active')
  if (curStillActive) {
    recordDecision('kept current source — it still serves your audio / language')
    void resolveStreamForCurrentEpisode()
    return
  }
  // Current provider can't serve the new facet — drop its stream immediately so
  // the user doesn't keep hearing the wrong-language audio while we resolve.
  engine.destroy()
  sourceError.value = null
  resetSourceSwitching()
  const pick = pickFacetDefault()
  if (!pick) {
    sourceError.value = t('player.aePlayer.noSourceForFacet')
    return
  }
  providerAutoSelected.value = true
  state.setProvider(pick.id, '') // provider watcher re-lists episodes + refreshes teams
  recordDecision('re-picked best source for the new audio / language')
}

// ─── Provider selection helper ────────────────────────────────────────────────

function onSelectProvider(id: string) {
  providerAutoSelected.value = false
  resetSourceSwitching() // manual pick — fresh state, and don't auto-switch it
  state.setProvider(id, '')
  recordDecision('manual — you picked this source')
  // loadEpisodesAndStream fires via the provider watcher above
}

// ─── Episode selection (episodes drawer) ─────────────────────────────────────
// Resolve DIRECTLY (mirrors goToNextEpisode) — the combo/episode watcher
// early-returns while isResolving and would silently swallow a click made
// during an in-flight resolve. resolveStreamForEpisode sets isResolving
// synchronously, so the watcher's deferred fire is deduped, and resolveToken
// arbitrates any race with the in-flight request.

function onSelectEpisode(ep: EpisodeOption) {
  openMenu.value = null
  if (selectedEpisode.value?.number === ep.number) return
  // WT: broadcast a genuine local episode pick to the room. Skipped when this
  // switch was itself driven by a remote room change (applyingRoomEpisode) so
  // the echo doesn't loop. The room's episode_id carries the episode number.
  if (props.room && !applyingRoomEpisode) {
    props.room.emitChangeEpisode(String(ep.number))
  }
  resetSourceSwitching() // new episode — fresh switch budget
  tracking.saveNow() // persist the outgoing episode's position
  selectedEpisode.value = ep
  userPickedEpisode.value = true
  tracking.resetEpisode(isEpisodeWatched(ep.number))
  resumeChipDismissed.value = false
  resumeChipUsed.value = false
  void resolveStreamForEpisode(ep)
}

// ─── Retry ───────────────────────────────────────────────────────────────────

function retryResolution() {
  sourceError.value = null
  resetSourceSwitching() // manual retry — give every candidate a fresh shot
  void loadEpisodesAndStream()
}

// ─── rAF progress loop ───────────────────────────────────────────────────────

const currentTime = ref(0)
const duration = ref(0)
const bufferedPct = ref(0)
/** true once playback has started for the current stream — gates the poster */
const hasStarted = ref(false)
let rafId: number | null = null

// Clear all playback-derived reactive state at a source swap. currentTime.value
// is synced from the <video> element ONLY by the rAF loop (while playing) and
// the final writeProgress() on pause — neither runs between attaching a new
// source and the first play. Without this, the OUTGOING source's playhead
// lingers in currentTime.value while the incoming source sits paused at 0, so
// currentTime-derived UI reads a stale position: e.g. a viewer parked in the
// ending, then switching server/episode, saw the "Skip Ending" chip render
// before pressing play (activeSkipSegment saw the stale ~ending-window time).
// restorePlayhead() re-seats currentTime.value when a same-episode swap keeps
// the position; a fresh (episode-change) load leaves it at 0.
function resetPlaybackClock() {
  currentTime.value = 0
  duration.value = 0
  bufferedPct.value = 0
  state.progress.value = 0
  // The incoming source hasn't reached its own end — drop any lingering
  // end-of-episode flag so the next-episode chip doesn't leak across the swap
  // (same stale-state class as the Skip-Ending chip above).
  reachedEpisodeEnd.value = false
}

/** rAF-path snap rates (see writeProgress) — mirrors SubtitleOverlay's
 *  TIME_SYNC_HZ pattern. 4 Hz time / half-percent buffered ≈ sub-pixel on
 *  the scrub bar while cutting reactive re-renders ~15×. */
const PROGRESS_SYNC_HZ = 4
const BUFFERED_SYNC_STEPS_PER_PCT = 2

function writeProgress(quantize = false) {
  const v = videoRef.value
  if (!v) return
  // Change-gate every reactive write, and QUANTIZE the fast movers on the rAF
  // path: currentTime moves every frame, so the change-gate never held during
  // playback and the control bar re-rendered 60×/sec (style recalc + layout
  // per frame — the 2026-07-04 render trace). Snapping time to 250ms and
  // buffered to 0.5% drops reactive updates to ~4 Hz, visually
  // indistinguishable on the scrub bar. Event-driven callers (seek, pause)
  // stay exact so a paused UI is frame-accurate. SubtitleOverlay syncs off
  // the <video> element directly (own rAF + snap grid) and is unaffected.
  const t = quantize ? Math.floor(v.currentTime * PROGRESS_SYNC_HZ) / PROGRESS_SYNC_HZ : v.currentTime
  if (t !== currentTime.value) currentTime.value = t
  const dur = v.duration || 0
  if (dur !== duration.value) duration.value = dur
  if (dur > 0) {
    const pct = (t / dur) * 100
    if (pct !== state.progress.value) state.progress.value = pct
  }
  // Buffered
  if (v.buffered.length > 0 && dur > 0) {
    const bpct = (v.buffered.end(v.buffered.length - 1) / dur) * 100
    const qb = quantize ? Math.floor(bpct * BUFFERED_SYNC_STEPS_PER_PCT) / BUFFERED_SYNC_STEPS_PER_PCT : bpct
    if (qb !== bufferedPct.value) bufferedPct.value = qb
  }
  // Watch tracking: heartbeat saves + duration-aware auto-complete — always
  // fed the RAW position (it does its own gating). Only feed real playback
  // positions — a paused pre-start frame (currentTime 0) or a dead source
  // must not write progress.
  if (v.currentTime > 0) {
    tracking.onTick(v.currentTime, dur)
  }
}

function tick() {
  writeProgress(true)
  rafId = requestAnimationFrame(tick)
}

function startRaf() {
  if (rafId === null) {
    rafId = requestAnimationFrame(tick)
  }
}

function stopRaf() {
  if (rafId !== null) {
    cancelAnimationFrame(rafId)
    rafId = null
  }
  // One final write so progress/time are up-to-date while paused
  writeProgress()
}

function onVideoPlay() {
  state.playing.value = true
  reachedEpisodeEnd.value = false // playback resumed — the end chip is stale
  startRaf()
  armUiIdleTimer()
}

function onVideoPause() {
  state.playing.value = false
  stopRaf() // final writeProgress() inside keeps tracking's lastKnown fresh
  tracking.saveNow()
  clearUiIdleTimer()
  uiVisible.value = true
}

// ─── Intro/outro skip (AniSkip via catalog proxy) ────────────────────────────

const epNumber = computed(() => selectedEpisode.value?.number ?? null)
const malIdRef = computed(() => props.malId ?? null)
const { opening, ending } = useSkipTimes(malIdRef, epNumber)

const chapters = computed(() =>
  segmentsToChapters(opening.value, ending.value, duration.value),
)

const skipTarget = computed(() =>
  activeSkipSegment(currentTime.value, opening.value, ending.value),
)

function onSkipSegment() {
  const v = videoRef.value
  const target = skipTarget.value
  if (!v || !target) return
  v.currentTime = target.end
  writeProgress()
}

// Auto-skip intro (settings toggle) — once per episode view so a manual
// seek back into the OP isn't fought.
let autoSkippedEp: number | null = null
watch(epNumber, () => {
  autoSkippedEp = null
})
watch(currentTime, (t) => {
  if (!state.autoSkip.value) return
  const op = opening.value
  if (!op) return
  const ep = epNumber.value
  if (ep === null || autoSkippedEp === ep) return
  if (t >= op.start && t < op.end - 1) {
    autoSkippedEp = ep
    const v = videoRef.value
    if (v) {
      v.currentTime = op.end
      writeProgress()
    }
  }
})

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
  if (reachedReported && !stallStartedAt) {
    stallStartedAt = performance.now()
  }
}

function onBufferEnd() {
  setBuffering(false)
  markSeekResumed()
  // Telemetry: stall resolved — emit duration (best-effort, never throws)
  if (stallStartedAt) {
    const stallMs = Math.round(performance.now() - stallStartedAt)
    stallStartedAt = 0
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
  const s = lastSeek.value
  if (s && !s.done && s.seekedMs === null) {
    s.seekedMs = Math.round(performance.now() - s.t0)
  }
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
    resetPlaybackBlocked() // actually playing — any autoplay veto is history
    resetSourceSwitching() // a source that actually plays earns a fresh budget
    // A source that actually plays must clear any stale error overlay: the
    // fallback chain can set sourceError='Stream unavailable' when the switch
    // budget is exhausted WHILE the source it just landed on is still buffering
    // — then that source starts playing and the overlay would otherwise stay up
    // over a working video. First real frame ⇒ the stream works, drop the error.
    sourceError.value = null
    // Telemetry: first real frame — resolve ok + reached_playback (best-effort)
    if (!reachedReported) {
      reachedReported = true
      recordPlayerEvent({
        kind: 'resolve',
        provider: state.combo.value.provider,
        anime_id: props.animeId,
        episode: selectedEpisode.value?.number,
        outcome: 'ok',
        reached_playback: true,
        latency_ms: resolveStartedAt ? Math.round(performance.now() - resolveStartedAt) : undefined,
        audio: state.combo.value.audio,
        lang: state.combo.value.lang,
      })
    }
  }
  if (isBuffering.value && v && v.readyState >= 3 && !v.seeking) {
    setBuffering(false)
  }
  if (v && v.readyState >= 3 && !v.seeking) {
    markSeekResumed()
  }
}

// A dead source must surface the error overlay, not an endless spinner.
// Covers the native/mp4 path (e.g. upstream CDN 404 → MEDIA_ERR_SRC_NOT_SUPPORTED).
function onVideoError() {
  const v = videoRef.value
  // isResolving: a resolve is in flight. switchingSource: an auto-failover just
  // committed and the destination resolve hasn't taken over yet — in both cases
  // this native error is from the source we're leaving, not a dead destination.
  if (!v?.error || isResolving.value || switchingSource) return
  setBuffering(false)
  sourceError.value = t('player.aePlayer.streamUnavailable')
}

// SubtitleOverlay failed to fetch/parse the chosen track (e.g. a dead upstream
// link). Turn the selection off rather than leaving it silently stuck — the
// video keeps playing, but the subtitle button should reflect reality so the
// user knows to pick a different track from the Subtitles menu.
function onSubtitleError() {
  toast.push(t('player.aePlayer.subtitleLoadFailed'), 'error', 5000)
  state.subLang.value = 'off'
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
    reached_playback: reachedReported,
    error_kind: f === 'network' ? 'stream_error' : 'media_fatal',
    latency_ms: resolveStartedAt ? Math.round(performance.now() - resolveStartedAt) : undefined,
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

const statsEnabled = computed(() => state.hackerMode.value)
const { stats: playbackStats } = usePlaybackStats(videoRef, statsEnabled)

// Hacker mode also mirrors the scrub-preview engine log to the console
// (prefix [ScrubPreview]) — frontend pump health vs provider fetch latency.
watch(
  () => state.hackerMode.value,
  (v) => {
    scrubDebug.console = v
  },
  { immediate: true },
)

// Seek pipeline trace — what actually happens between releasing the scrubber
// and playback resuming: buffer check (hit = instant, no network), pipeline
// flush + segment fetch, decode from the nearest keyframe to the target
// (the `seeked` event), then buffer refill to readyState ≥ 3 (resume).
const lastSeek = ref<SeekTrace | null>(null)

function traceSeekStart(target: number) {
  if (!state.hackerMode.value) return
  const v = videoRef.value
  if (!v) return
  let hit = false
  for (let i = 0; i < v.buffered.length; i++) {
    if (target >= v.buffered.start(i) && target <= v.buffered.end(i)) {
      hit = true
      break
    }
  }
  lastSeek.value = {
    target,
    bufferHit: hit,
    t0: performance.now(),
    fetchMs: null,
    fetchedRange: null,
    seekedMs: null,
    resumeMs: null,
    frags: 0,
    bytes: 0,
    done: false,
  }
}

// Fetch-phase depth: the `progress` event fires as bytes arrive — the moment
// a buffered range covers the seek target, the network part of the seek is
// done (mp4: the ranged bytes landed; hls: the segment(s) arrived).
function onVideoProgress() {
  const s = lastSeek.value
  const v = videoRef.value
  if (!s || s.done || s.fetchMs !== null || !v) return
  for (let i = 0; i < v.buffered.length; i++) {
    const start = v.buffered.start(i)
    const end = v.buffered.end(i)
    if (s.target >= start && s.target <= end) {
      s.fetchMs = Math.round(performance.now() - s.t0)
      s.fetchedRange = [start, end]
      return
    }
  }
}

function markSeekResumed() {
  const s = lastSeek.value
  if (!s || s.done) return
  if (s.seekedMs === null) return // resume can't precede the seeked event
  s.resumeMs = Math.round(performance.now() - s.t0)
  s.done = true
}

// Count fragments fetched while a seek is in flight (hls path; FRAG_LOADED
// appends exactly one entry per fragment to the rolling fragStats window).
watch(engine.fragStats, (arr) => {
  const s = lastSeek.value
  if (!s || s.done) return
  const last = arr[arr.length - 1]
  if (!last) return
  s.frags++
  s.bytes += last.size
})

// HUD shows while paused or while actively buffering/seeking (or always when
// pinned). When the show-condition drops (playback resumed), the panel
// LINGERS ~1s, fades 0.4s, then unmounts — so the final seek numbers are
// readable instead of vanishing the instant video continues.
const hudCondition = computed(
  () =>
    state.hackerMode.value &&
    (state.hudPinned.value || !state.playing.value || showBuffering.value),
)
const hudVisible = ref(false)
const hudFading = ref(false)
let hudLingerTimer: ReturnType<typeof setTimeout> | null = null
let hudFadeTimer: ReturnType<typeof setTimeout> | null = null

function clearHudTimers() {
  if (hudLingerTimer) { clearTimeout(hudLingerTimer); hudLingerTimer = null }
  if (hudFadeTimer) { clearTimeout(hudFadeTimer); hudFadeTimer = null }
}

watch(hudCondition, (on) => {
  clearHudTimers()
  if (on) {
    hudVisible.value = true
    hudFading.value = false
    return
  }
  // linger 1s with full opacity, then 0.4s CSS fade, then unmount
  hudLingerTimer = setTimeout(() => {
    hudFading.value = true
    hudFadeTimer = setTimeout(() => {
      hudVisible.value = false
      hudFading.value = false
    }, 450)
  }, 1000)
}, { immediate: true })

// Scrub-bar heatmap segments — size-tinted (green <300KB, amber <1MB, red ≥1MB).
const fragOverlay = computed(() => {
  if (!state.hackerMode.value) return []
  const dur = duration.value
  if (!dur) return []
  return engine.fragStats.value.map((f) => ({
    startPct: (f.start / dur) * 100,
    widthPct: (f.duration / dur) * 100,
    tone: (f.size < 300_000 ? 'ok' : f.size < 1_000_000 ? 'warn' : 'bad') as 'ok' | 'warn' | 'bad',
    label: `${Math.round(f.size / 1024)} KB · ${Math.round(f.loadMs)} ms`,
  }))
})

// Compact line set for the settings-menu mini-stats section.
// Render the raw X-AE-Edge-Trail header ("p13:timeout:45003,p12:ok:210") as the
// hacker-mode logic+metrics line: "p13 45.0s✗ → p12 0.21s✓". A trailing ✓ marks
// the edge that served (outcome "ok"); every other outcome is a ✗ with its
// latency, so a cold-start wait or a burned timeout is legible at a glance.
function formatEdgeTrail(raw: string): string {
  if (!raw) return ''
  return raw
    .split(',')
    .map((part) => {
      const [edge, outcome, ms] = part.split(':')
      const secs = Number(ms) / 1000
      const t = Number.isFinite(secs) ? (secs >= 10 ? secs.toFixed(1) : secs.toFixed(2)) : '?'
      return `${edge} ${t}s${outcome === 'ok' ? '✓' : '✗'}`
    })
    .join(' → ')
}

const debugStats = computed(() => {
  if (!state.hackerMode.value) return null
  const bwv = engine.bandwidthEstimate.value
  const frs = engine.fragStats.value
  const last = frs[frs.length - 1]
  const edge = engine.servedEdge.value
  const trail = engine.edgeTrail.value
  const ladderSnap = ladder.debugSnapshot()
  return {
    bw: bwv > 0 ? `${(bwv / 1_000_000).toFixed(1)} Mbit/s` : '—',
    buffer: `+${playbackStats.value.bufferAheadSec.toFixed(1)}s / −${playbackStats.value.bufferBehindSec.toFixed(1)}s`,
    level:
      engine.currentLevelLabel.value ||
      (currentStream.value?.type === 'mp4' ? 'mp4' : '—'),
    frag: last ? `${Math.round(last.size / 1024)} KB · ${Math.round(last.loadMs)} ms` : '—',
    // Kodik/solodcdn edge telemetry — empty for every other source (no header).
    // The decision (served edge) + the logic/metrics (attempt trail, rotations).
    edge,
    edgeTrail: edge ? formatEdgeTrail(trail) : '',
    edgeRot: edge && trail ? trail.split(',').length - 1 : 0,
    // Protocol-ladder telemetry (multi-tier prod only; null snapshot in dev).
    // I4: tier display is 1-based ("tier 2/3") via formatLadderRows — the
    // underlying debugSnapshot().tierIndex contract stays 0-based.
    ...(ladderSnap ? formatLadderRows(ladderSnap) : {}),
  }
})

// ─── Next episode logic ───────────────────────────────────────────────────────

const showNextEpisode = ref(false)
const nextEpCountdown = ref(5)
let nextEpTimer: ReturnType<typeof setInterval> | null = null
// True once the playhead reaches the episode end. Drives the manual next-episode
// chip on the autoplay-OFF path; reset when playback resumes (onVideoPlay) or at
// any source swap (resetPlaybackClock — covers manual episode/server switches too).
const reachedEpisodeEnd = ref(false)

function onEnded() {
  state.playing.value = false
  // Reaching the end IS a completed watch — mark even if the 90% tick raced.
  tracking.saveNow()
  void tracking.markWatched()
  // Playhead hit the end. Autoplay-ON opens the countdown card; autoplay-OFF
  // surfaces the manual "Next episode" chip (via reachedEpisodeEnd → showNextEpChip).
  reachedEpisodeEnd.value = true
  if (anime_hasNextEp.value && state.autoNext.value) {
    startNextEpCountdown()
  }
}

// The episode the user is actually on — initialEpisode is the resume-resolved
// starting point; selectedEpisode.value tracks in-session switches.
const currentEpNumber = computed(
  () => selectedEpisode.value?.number ?? props.initialEpisode ?? 1,
)

// "Has a next episode" derived from the loaded episode list (authoritative for
// the current source), falling back to the catalog ep/eps counts before the
// list resolves. Using props.anime.ep here was the bug: after switching a few
// episodes it still pointed at the mount episode, so the countdown could start
// at the series end (and then goToNextEpisode found nothing → silent stall).
const anime_hasNextEp = computed(() => {
  const sel = selectedEpisode.value
  if (episodes.value.length && sel) {
    const idx = episodes.value.findIndex((e) => e.number === sel.number)
    if (idx >= 0) return idx + 1 < episodes.value.length
  }
  return currentEpNumber.value < props.anime.eps
})

// The actual next episode number for the "Up next" card label.
const nextEpisodeNumber = computed(() => {
  const sel = selectedEpisode.value
  if (episodes.value.length && sel) {
    const idx = episodes.value.findIndex((e) => e.number === sel.number)
    if (idx >= 0 && idx + 1 < episodes.value.length) return episodes.value[idx + 1].number
  }
  return currentEpNumber.value + 1
})

// Manual "Next episode" chip — the autoplay-OFF affordance. The countdown card
// owns the autoplay-ON path, so the two never show together. Visible from the
// ending/outro segment through the episode end, whenever a next episode exists.
const showNextEpChip = computed(
  () =>
    anime_hasNextEp.value &&
    !state.autoNext.value &&
    !showNextEpisode.value &&
    (reachedEpisodeEnd.value || skipTarget.value?.kind === 'outro'),
)

function startNextEpCountdown() {
  showNextEpisode.value = true
  nextEpCountdown.value = 5
  nextEpTimer = setInterval(() => {
    nextEpCountdown.value--
    if (nextEpCountdown.value <= 0) {
      clearNextEpTimer()
      goToNextEpisode()
    }
  }, 1000)
}

function clearNextEpTimer() {
  if (nextEpTimer) {
    clearInterval(nextEpTimer)
    nextEpTimer = null
  }
}

function goToNextEpisode() {
  showNextEpisode.value = false
  reachedEpisodeEnd.value = false // episode is changing — drop the end-of-ep chip
  clearNextEpTimer()
  // Find next episode in list
  const current = selectedEpisode.value
  if (!current) return
  const idx = episodes.value.findIndex((e) => e.number === current.number)
  const next = episodes.value[idx + 1]
  if (next) {
    tracking.saveNow()
    selectedEpisode.value = next
    tracking.resetEpisode(isEpisodeWatched(next.number))
    resumeChipDismissed.value = false
    resumeChipUsed.value = false
    void resolveStreamForEpisode(next)
  }
}

async function resolveStreamForEpisode(ep: EpisodeOption, keepPosition = false) {
  const provider = state.combo.value.provider
  if (!provider) return
  sourceError.value = null
  isResolving.value = true
  switchingSource = false // resolve owns error handling now — handoff window over
  hasStarted.value = false
  resetPlaybackBlocked() // new stream = a fresh autoplay verdict
  const token = ++resolveToken
  resolveStartedAt = performance.now()
  reachedReported = false
  stallStartedAt = 0
  try {
    const stream = await resolver.resolveStream(
      provider,
      props.animeId,
      ep,
      state.combo.value,
    )
    if (token !== resolveToken) return // superseded
    resolvedServers.value = stream.servers ?? []
    // Set BEFORE the await — see loadEpisodesAndStream.
    currentStream.value = stream
    applyOfflineAutoSub(ep.number, stream)
    // Same-episode re-resolve (facet/server/team/quality switch) keeps the
    // viewer's spot; a genuine episode change (keepPosition=false) starts fresh.
    const { restoreT, wasPlaying } = capturePlayhead(keepPosition)
    resetPlaybackClock() // drop the outgoing source's playhead before the swap
    await engine.load(stream)
    applyInitialSeek() // shared-link `?t=` one-shot seek (no-op after first load)
    restorePlayhead(restoreT, wasPlaying)
    armPlaybackWatchdog() // catch a silent CODECS-less stall (manifest OK, no frags)
  } catch (err: unknown) {
    if (token !== resolveToken) return // superseded
    const isNotAvailable =
      err instanceof Error && err.name === 'NotAvailableError'
    // Telemetry: resolve failure (best-effort, never throws)
    recordPlayerEvent({
      kind: 'resolve',
      provider: state.combo.value.provider,
      anime_id: props.animeId,
      episode: ep.number,
      outcome: 'fail',
      reached_playback: false,
      error_kind: isNotAvailable ? 'not_available' : 'stream_error',
      latency_ms: resolveStartedAt ? Math.round(performance.now() - resolveStartedAt) : undefined,
      audio: state.combo.value.audio,
      lang: state.combo.value.lang,
    })
    if (await advanceToNextSource('resolve failed')) {
      toast.push(t('player.aePlayer.switchNotAvailable'), 'info', 4000)
      return
    }
    if (!sourceError.value) {
      sourceError.value = isNotAvailable
        ? t('player.aePlayer.sourceUnavailable')
        : t('player.aePlayer.streamUnavailable')
    }
  } finally {
    if (token === resolveToken) {
      isResolving.value = false
    }
  }
}

// Re-resolve stream (without re-listing episodes) for the currently-selected
// episode when audio, lang, server, or selectedEpisode changes.
// Provider changes are handled separately by the provider watcher above
// (which does a full re-list + resolve) — we skip here when it's active.
async function resolveStreamForCurrentEpisode() {
  const ep = selectedEpisode.value
  if (!ep) return
  // Same episode, different stream (audio/lang/server/team/quality) → keep the
  // playhead so the swap is seamless and can't regress saved progress.
  await resolveStreamForEpisode(ep, true)
}

// ─── Menu state ───────────────────────────────────────────────────────────────

type MenuKind = 'source' | 'settings' | 'subs' | 'episodes' | null
const openMenu = ref<MenuKind>(null)
const browseOpen = ref(false)

// One floating dropdown is open at a time (mutually-exclusive v-if), so the
// active element resolves from whichever menu `openMenu` selects.
const sourceMenuEl = ref<HTMLElement | null>(null)
const episodesMenuEl = ref<HTMLElement | null>(null)
const settingsMenuEl = ref<HTMLElement | null>(null)
const subsMenuEl = ref<HTMLElement | null>(null)
const activeMenuEl = computed<HTMLElement | null>(() => {
  switch (openMenu.value) {
    case 'source': return sourceMenuEl.value
    case 'episodes': return episodesMenuEl.value
    case 'settings': return settingsMenuEl.value
    case 'subs': return subsMenuEl.value
    default: return null
  }
})

// ─── Offline downloads (season-only, app-only) ──────────────────────────────

const downloadDialogOpen = ref(false)
const downloadStates = ref<Record<number, DownloadState>>({})
const seasonCount = computed(() => seasonTargets(episodes.value, downloadStates.value).length)
// offlineRuntimeReady() is non-reactive (navigator.serviceWorker.controller) —
// a plain check in the template would never appear after the SW's first claim.
// Track it in a ref, refreshed when the SW becomes ready.
const canDownload = ref(false)
// Downloads are app-only: in a plain browser tab every download surface is
// hidden entirely (owner call 2026-07-14) — useDownloadGate owns that policy
// for all surfaces.
const { appOnly } = useDownloadGate()
const downloadMode = computed<'off' | 'ready'>(() => {
  if (props.offline || !offlineDownloadsEnabled) return 'off'
  if (appOnly.value) return 'off'
  return canDownload.value ? 'ready' : 'off'
})

async function refreshDownloadStates() {
  if (!offlineRuntimeReady()) return
  const all = await listDownloads()
  const mine: Record<number, DownloadState> = {}
  for (const d of all) if (d.animeId === props.animeId) mine[d.episode.number] = d.state
  downloadStates.value = mine
}
onMounted(() => {
  canDownload.value = offlineRuntimeReady()
  navigator.serviceWorker?.ready.then(() => { canDownload.value = offlineRuntimeReady() }).catch(() => {})
  void refreshDownloadStates()
})
// Throttle: engineState.progress ticks per segment (~300×/episode); a raw
// watcher would hammer IndexedDB with listDownloads() on every tick.
let dlRefreshQueued = false
let dlRefreshTimer: ReturnType<typeof setTimeout> | null = null
watch(engineState.progress, () => {
  if (dlRefreshQueued) return
  dlRefreshQueued = true
  dlRefreshTimer = setTimeout(() => { dlRefreshQueued = false; void refreshDownloadStates() }, 1000)
})
watch(() => engineState.cellularPauses.value, () => {
  // DownloadsPage mounts an inline offline AePlayer — skip there or the
  // page's own watcher (Task 10) double-toasts the same event.
  if (props.offline) return
  toast.push(t('player.aePlayer.offline.cellularAutoPaused'), 'info', 5000)
})

// Bundled entries come from the CURRENT stream (per-episode availability is
// re-matched by the engine); external entries from the aggregated list.
const dlSubOptions = computed<SubOption[]>(() => {
  const opts: SubOption[] = []
  const seenLangs = new Set<string>()
  const bundledUrls = new Set<string>()
  for (const tr of providerBundledTracks.value) {
    bundledUrls.add(tr.url)
    if (seenLangs.has(tr.lang)) continue
    seenLangs.add(tr.lang)
    opts.push({ key: `b:${tr.lang}`, label: `${t('player.aePlayer.offline.subsBundled')} · ${tr.lang.toUpperCase()}`, pref: { kind: 'bundled', lang: tr.lang } })
  }
  opts.push(...externalSubOptions(subtitleTracks.value.filter((tr) => !bundledUrls.has(tr.url))))
  return opts
})

function dlLoadTeams(provider: string, audio: AudioKind): Promise<string[]> {
  return resolver.listTeams(provider, props.animeId, audio)
}

function onDownloadSeason() {
  downloadDialogOpen.value = true
  void ensureSubsLoaded() // aggregated tracks feed the dialog's subtitle picker
}

async function onConfirmDownload(quality: string, combo: Combo | null, subPref: SubPref | null) {
  downloadDialogOpen.value = false
  const comboSnapshot = combo ? { ...combo } : { ...state.combo.value } // freeze — user may switch sources mid-download
  const resolveSubsFor = makeExternalSubResolver(props.animeId, subPref)
  // A dialog-picked provider lists episodes with its own keys — re-list via
  // that provider before computing the season targets against it.
  let eps = episodes.value
  if (comboSnapshot.provider !== state.combo.value.provider) {
    try {
      eps = await resolver.listEpisodes(comboSnapshot.provider, props.animeId)
    } catch {
      toast.push(t('player.aePlayer.offline.sourceListFailed'), 'error')
      return
    }
  }
  const targets = seasonTargets(eps, downloadStates.value)
  await enqueueSeason(targets, {
    animeId: props.animeId,
    animeTitle: props.anime.title,
    poster: props.anime.still,
    combo: comboSnapshot,
    quality,
    durationMin: props.anime.durationMin,
    subPref: subPref ?? undefined,
    resolveSubsFor,
    resolveFor: (target) => () => resolver.resolveStream(comboSnapshot.provider, props.animeId, target, comboSnapshot),
  })
  void refreshDownloadStates()
}

// Click-outside dismiss: a click anywhere outside the open dropdown closes it.
// Ignore the trigger regions — the control bar (source/subs/settings pills) and
// top bar (EP trigger) own their own toggle, so letting click-outside fire there
// too would race the trigger and reopen-then-close. The <video> is also ignored
// because onVideoClick handles it (and suppresses the play/pause side effect);
// without this the pointerdown-phase click-outside would close the menu first,
// leaving onVideoClick's click to fall through to togglePlay.
onClickOutside(
  activeMenuEl,
  () => { if (openMenu.value !== null) closeMenus() },
  { ignore: ['.pl-controls', '.pl-top', videoRef] },
)

function toggleMenu(menu: MenuKind) {
  openMenu.value = openMenu.value === menu ? null : menu
  if (openMenu.value !== null) browseOpen.value = false
}

function closeMenus() {
  openMenu.value = null
  browseOpen.value = false
}

// Mobile sheets: teleport the floating menus to <body> and present them as
// bottom sheets. Disabled inside NATIVE fullscreen — body children render
// under the fullscreen element — where fixed positioning already fills the
// fullscreen viewport correctly in place.
const sheetTeleport = computed(() => isMobile.value && !nativeFsActive.value)
const anySheetOpen = computed(() => openMenu.value !== null || browseOpen.value || downloadDialogOpen.value)

function closeAllSheets() {
  closeMenus()
  downloadDialogOpen.value = false
}

// ─── Controls auto-hide (idle while playing) ─────────────────────────────────
// Top bar + control bar fade out after UI_IDLE_MS of pointer inactivity while
// playing (matters most in fullscreen). Any pointer/keyboard activity, a pause,
// or an open menu brings them back and keeps them visible.

const UI_IDLE_MS = 2500
const uiVisible = ref(true)
let uiIdleTimer: ReturnType<typeof setTimeout> | null = null

function clearUiIdleTimer() {
  if (uiIdleTimer !== null) {
    clearTimeout(uiIdleTimer)
    uiIdleTimer = null
  }
}

function armUiIdleTimer() {
  clearUiIdleTimer()
  if (!state.playing.value || openMenu.value !== null) return
  uiIdleTimer = setTimeout(() => {
    uiVisible.value = false
  }, UI_IDLE_MS)
}

function wakeUi() {
  uiVisible.value = true
  armUiIdleTimer()
}

// On touch the video tap handler owns chrome toggling — pre-waking from the
// root touchstart would make every tap see "chrome already visible" and
// immediately hide it again (toggle would never show the chrome).
function onRootTouch(e: TouchEvent) {
  if (isCoarse.value && e.target === videoRef.value) return
  wakeUi()
}

function onPointerEnter() {
  isPointerInside.value = true
  wakeUi()
}

function onPointerLeave() {
  isPointerInside.value = false
  // Pointer left the player while playing — hide right away (menus pin it)
  if (state.playing.value && openMenu.value === null) {
    clearUiIdleTimer()
    uiVisible.value = false
  }
}

watch(openMenu, (menu) => {
  if (menu !== null) {
    clearUiIdleTimer()
    uiVisible.value = true
  } else {
    armUiIdleTimer()
  }
})

// ─── Subtitles ────────────────────────────────────────────────────────────────

const chosenSub = ref<SubTrack | null>(null)

const chosenSubUrl = computed<string | null>(() => chosenSub.value?.url ?? null)
// Whether a subtitle overlay is actually rendering — drives the CC button's
// enabled affordance (distinct from the menu-open highlight).
const subsOn = computed(() => state.subLang.value !== 'off' && !!chosenSubUrl.value)
const chosenSubFormat = computed<'ass' | 'srt' | 'vtt' | null>(() => {
  const fmt = chosenSub.value?.format ?? null
  if (fmt === 'ass' || fmt === 'srt' || fmt === 'vtt') return fmt
  return null
})

// Episode number the subtitle aggregation keys on.
const subEpisode = computed(() => selectedEpisode.value?.number ?? props.initialEpisode ?? 1)
// Provider's own signed soft-subs from the resolved stream.
const providerSubtitles = computed(() => currentStream.value?.subtitles)
const {
  tracks: subtitleTracks,
  loading: subsLoading,
  error: subsError,
  providersDown: subsProvidersDown,
  ensureLoaded: ensureSubsLoaded,
  refetch: refetchSubs,
} = useSubtitleTracks(toRef(props, 'animeId'), subEpisode, providerSubtitles)

// ─── Subtitle auto-sync (frontend VAD; spec 2026-06-29) ──────────────────────
const { cues: subtitleCues } = useSubtitleCues(chosenSubUrl, chosenSubFormat)
const autoSyncEpisodeKey = computed(() => `${props.animeId}:${subEpisode.value}`)
const autoSyncPref = useSubtitleAutoSyncPref(autoSyncEpisodeKey)
const autoSync = useSubtitleAutoSync({
  videoElement: videoRef,
  cues: subtitleCues,
  enabled: computed(() => autoSyncPref.enabled.value && subsOn.value),
  episodeKey: autoSyncEpisodeKey,
})
// Manual offset layers on top of the auto result.
const effectiveOffset = computed(() => autoSync.autoOffset.value + state.subOffset.value)
const autoSyncInfo = computed(() =>
  state.hackerMode.value
    ? { status: autoSync.status.value, offset: autoSync.autoOffset.value, confidence: autoSync.confidence.value, events: autoSync.syncEvents.value }
    : null,
)

// Subtitles default OFF (state.subLang starts 'off') and the player NEVER
// auto-enables one — the user opts in via the Subtitles menu. That choice
// (language + on/off) then PERSISTS across episodes: the track URL is
// episode-specific so it's dropped on episode change, but the re-bind watcher
// below re-resolves a track in the chosen language for the new episode.

// A NEW episode drops the stale (episode-specific) track URL but KEEPS the
// user's subtitle language choice (state.subLang). Keyed on episode, NOT
// currentStream: a same-episode re-resolve (server fallback, quality swap) must
// not drop a track the user is watching.
watch(subEpisode, () => {
  chosenSub.value = null
})

// Fetch the aggregation eagerly once a RAW (sub) stream resolves so the menu's
// language list is ready — but DO NOT auto-enable (subs default off).
watch(
  [currentStream, () => state.combo.value.audio],
  async () => {
    if (!currentStream.value || state.combo.value.audio !== 'sub') return
    await ensureSubsLoaded()
  },
)

// Re-bind the chosen track to the persisted subtitle language whenever the track
// list changes (new episode, or late provider/aggregation arrival). 'off' stays
// off — there is no auto-enable.
watch(subtitleTracks, () => {
  const lang = state.subLang.value
  if (lang === 'off') return
  const track = pickBestForLang(subtitleTracks.value, lang)
  if (track) chosenSub.value = track
})

// Real distinct languages that have a loaded soft track (provider-bundled +
// aggregated Jimaku/OpenSubtitles). Drives which RU/EN/JP fast buttons are enabled.
const availableSubLangs = computed(() =>
  [...new Set(subtitleTracks.value.map((t) => t.lang))],
)

// Provider-bundled soft subs that shipped with the resolved stream.
const providerBundledTracks = computed<SubTrack[]>(
  () => (providerSubtitles.value ?? []) as SubTrack[],
)

// Per-language source label for the quick menu rows ("Русский · Crunchyroll").
// The best track for a language (bundled-first) supplies the meta line.
const langSources = computed<Record<string, string>>(() => {
  const m: Record<string, string> = {}
  for (const l of ['ru', 'en', 'ja']) {
    const best = pickBestForLang(subtitleTracks.value, l)
    if (best) m[l] = best.label
  }
  return m
})

// Informational note for the subs menu: when the provider shipped no soft track
// for an EN/RU SUB cut, the subs the user sees are hardsubbed into the video.
// A raw original-JP cut (lang 'ja') is NOT hardsubbed — its subs come from the
// optional Jimaku/OpenSubtitles overlay — so the note never applies there.
const hardsubNote = computed(() => {
  if (chosenSub.value) return null
  if (state.combo.value.audio !== 'sub') return null
  if (state.combo.value.lang === 'ja') return null         // raw JP → overlay, not burned in
  if (providerBundledTracks.value.length > 0) return null  // provider soft subs → not hardsubbed
  const prov = activeProviderName.value
  if (!prov) return null
  return t('player.aePlayer.subs.hardsub', { provider: prov })
})

// Session opt-out: once the viewer explicitly turns subs off, offline
// auto-enable must not re-arm on the next episode.
let userDisabledSubs = false

// The ONLY sanctioned subtitle auto-enable: explicit download-time choice,
// offline playback only. Called after EVERY currentStream assignment.
// Note: the "re-bind chosen track to subLang" watcher may later swap to
// pickBestForLang's pick for the same lang — same track in practice.
function applyOfflineAutoSub(epNumber: number, stream: StreamResult): void {
  if (!props.offline || userDisabledSubs) return
  const auto = pickOfflineAutoSub(props.offline, epNumber, stream.subtitles)
  if (auto) {
    chosenSub.value = auto as SubTrack
    state.subLang.value = auto.lang // session ref — the global "subs off by default" pref is untouched
  }
}

function onSelectSubTrack(track: SubTrack) {
  chosenSub.value = track
  // Selecting a track turns the overlay on for that language (persists across episodes).
  state.subLang.value = track.lang
  browseOpen.value = false
}

function onSubtitlesOff() {
  userDisabledSubs = true
  chosenSub.value = null
  state.subLang.value = 'off'
  browseOpen.value = false
}

// Quick-chooser RU/EN/JP language row → pick the best track for that language.
function onPickSubLang(v: string) {
  if (v === 'off') { onSubtitlesOff(); return }
  const track = pickBestForLang(subtitleTracks.value, v)
  if (track) onSelectSubTrack(track)
}

// ─── Upcoming episode placeholder ────────────────────────────────────────────

// The "next episode airs {when}" info is NOT overlaid on the video — the anime
// page already shows that banner above the player. Instead we surface the
// awaited episode as a disabled placeholder inside the episodes sheet. Derived
// from the resume banner's next-unavailable family.
const upcomingEpisode = computed<{ number: number; etaLabel?: string } | null>(() => {
  const b = props.resumeBanner
  return b && b.kind === 'next-unavailable' ? { number: b.episode, etaLabel: b.etaLabel } : null
})

// ─── Playback helpers ─────────────────────────────────────────────────────────

// A tap on the video: backdrop-dismiss if a menu is open (no side effect).
// Desktop click = play/pause. Touch (coarse): single tap toggles the chrome,
// double-tap on the side thirds seeks ±10s (center double-tap = play/pause) —
// so the play/pause affordance on phones is the center overlay button, never
// a stray tap.
const DOUBLE_TAP_MS = 280
let lastTapAt = 0
let lastTapX = 0
let singleTapTimer: ReturnType<typeof setTimeout> | null = null

const seekFlash = ref<'back' | 'fwd' | null>(null)
let seekFlashTimer: ReturnType<typeof setTimeout> | null = null

function flashSeek(dir: 'back' | 'fwd') {
  seekFlash.value = dir
  if (seekFlashTimer) clearTimeout(seekFlashTimer)
  seekFlashTimer = setTimeout(() => {
    seekFlash.value = null
  }, 500)
}

function onVideoClick(e: MouseEvent) {
  if (openMenu.value !== null || browseOpen.value) {
    closeMenus()
    return
  }
  if (!isCoarse.value) {
    togglePlay()
    return
  }

  const now = performance.now()
  const isDouble = now - lastTapAt < DOUBLE_TAP_MS && Math.abs(e.clientX - lastTapX) < 64
  lastTapAt = now
  lastTapX = e.clientX

  if (isDouble) {
    if (singleTapTimer) {
      clearTimeout(singleTapTimer)
      singleTapTimer = null
    }
    lastTapAt = 0
    const rect = rootRef.value?.getBoundingClientRect()
    const x = rect && rect.width > 0 ? (e.clientX - rect.left) / rect.width : 0.5
    if (x < 0.4) {
      onSeekRel(-10)
      flashSeek('back')
    } else if (x > 0.6) {
      onSeekRel(10)
      flashSeek('fwd')
    } else {
      togglePlay()
    }
    return
  }

  singleTapTimer = setTimeout(() => {
    singleTapTimer = null
    if (uiVisible.value && state.playing.value) {
      clearUiIdleTimer()
      uiVisible.value = false
    } else {
      wakeUi()
    }
  }, DOUBLE_TAP_MS)
}

function togglePlay() {
  const v = videoRef.value
  if (!v) return
  if (v.paused) {
    attemptPlay()
  } else {
    v.pause()
  }
}

function onSeekRel(delta: number) {
  const v = videoRef.value
  if (!v) return
  const target = Math.max(0, Math.min(isFinite(v.duration) ? v.duration : Infinity, v.currentTime + delta))
  traceSeekStart(target)
  v.currentTime = target
}

function onSeek(pct: number) {
  const v = videoRef.value
  if (!v || !v.duration) return
  const target = (pct / 100) * v.duration
  traceSeekStart(target)
  v.currentTime = target
  // Write progress immediately so the scrub bar reflects the new position
  // even while paused (rAF loop is stopped when paused).
  writeProgress()
}

function onSetVolume(vol: number) {
  state.volume.value = vol
  const v = videoRef.value
  if (v) v.volume = vol / 100
}

function onToggleMute() {
  state.muted.value = !state.muted.value
  const v = videoRef.value
  if (v) v.muted = state.muted.value
}

function onSetSpeed(speed: number) {
  state.speed.value = speed
  const v = videoRef.value
  if (v) v.playbackRate = speed
}

function onVolumeChange() {
  const v = videoRef.value
  if (!v) return
  // Sync state from element — covers PiP / media-session external changes.
  // Only write state here; the set-volume path writes to the element.
  state.volume.value = Math.round(v.volume * 100)
  state.muted.value = v.muted
}

function onTogglePip() {
  const v = videoRef.value
  if (!v) return
  if (document.pictureInPictureElement) {
    void document.exitPictureInPicture()
  } else {
    void v.requestPictureInPicture?.()
  }
}

// ─── Fullscreen (capability-based) ───────────────────────────────────────────
// Android/desktop/iPad: native element fullscreen (+ landscape lock on touch).
// iPhone Safari never yields a usable element fullscreen — the API is absent on
// older builds and present-but-lying on newer ones (it resolves for <video> and
// fails for everything else). Probing it and reacting to the failure is not
// reliable: a build that returns undefined instead of a promise makes .then()
// throw synchronously, which kills the toggle outright. So iPhone treats the CSS
// takeover as its FIRST-CLASS fullscreen path, not a rescue after a failed bet.
// video.webkitEnterFullscreen() (the native iOS player) is deliberately NOT
// used: it drops SubtitleOverlay, the Source panel and the WT button.

/** iPhone/iPod: element fullscreen is unusable, so the CSS takeover IS the path. */
function prefersPseudoFullscreen(): boolean {
  return typeof navigator !== 'undefined' && /iP(hone|od)/.test(navigator.userAgent)
}

const pseudoFs = ref(false)
const nativeFsActive = ref(false)
const fullscreenActive = computed(() => nativeFsActive.value || pseudoFs.value)
// Tracks whether WE pushed the pseudo-FS history entry and it hasn't been
// consumed yet — `history.state` itself is unreliable because url-sync
// (router.replace on episode/provider change) can overwrite the top entry's
// state while pseudo-FS is active, dropping our marker.
let pseudoFsEntryPushed = false

function onFullscreenChange() {
  nativeFsActive.value = !!document.fullscreenElement
  if (!nativeFsActive.value) unlockOrientation()
}

function lockLandscape() {
  const o = screen.orientation as ScreenOrientation & { lock?: (v: string) => Promise<void> }
  void o?.lock?.('landscape').catch(() => {})
}

function unlockOrientation() {
  try {
    screen.orientation?.unlock?.()
  } catch {
    /* not locked / unsupported */
  }
}

function onToggleFullscreen() {
  const el = rootRef.value ?? videoRef.value?.parentElement
  if (!el) return
  if (document.fullscreenElement) {
    void document.exitFullscreen()
    return
  }
  if (pseudoFs.value) {
    exitPseudoFs()
    return
  }
  if (prefersPseudoFullscreen() || !el.requestFullscreen) {
    enterPseudoFs()
    return
  }
  try {
    const req = el.requestFullscreen()
    // The spec returns a Promise, but WebKit builds that return undefined would
    // make .then() throw and leave the toggle dead — only chain when thenable.
    if (req && typeof req.then === 'function') {
      req
        .then(() => {
          if (isCoarse.value) lockLandscape()
        })
        .catch(() => enterPseudoFs())
    } else if (isCoarse.value) {
      lockLandscape()
    }
  } catch {
    enterPseudoFs()
  }
}

// Pseudo-FS pushes a history entry so the phone's back gesture exits the
// takeover instead of leaving the page.
function onPseudoFsPop() {
  pseudoFsEntryPushed = false // the entry was just consumed by this pop
  exitPseudoFs(true)
}

function enterPseudoFs() {
  pseudoFs.value = true
  document.documentElement.classList.add('pl-noscroll')
  // Merge with the existing state so vue-router's own bookkeeping
  // ({position, back, current…}) survives alongside our marker.
  history.pushState({ ...history.state, plPseudoFs: true }, '')
  pseudoFsEntryPushed = true
  window.addEventListener('popstate', onPseudoFsPop)
}

function exitPseudoFs(viaPop = false) {
  if (!pseudoFs.value) return
  pseudoFs.value = false
  document.documentElement.classList.remove('pl-noscroll')
  window.removeEventListener('popstate', onPseudoFsPop)
  if (!viaPop && pseudoFsEntryPushed) {
    pseudoFsEntryPushed = false
    history.back()
  }
}

/** Unmount-safe teardown: never touches history (a route change already moved it). */
function teardownPseudoFs() {
  if (!pseudoFs.value) return
  pseudoFs.value = false
  document.documentElement.classList.remove('pl-noscroll')
  window.removeEventListener('popstate', onPseudoFsPop)
  pseudoFsEntryPushed = false
}

// ─── Keyboard shortcuts ───────────────────────────────────────────────────────
// Listen on window but only act when the pointer is over the player or focus is
// inside it — so space/arrows control THIS player without hijacking the page.

function playerIsActive(): boolean {
  if (isPointerInside.value) return true
  const root = rootRef.value
  return !!(root && document.activeElement && root.contains(document.activeElement))
}

function onKeydown(e: KeyboardEvent) {
  if (!playerIsActive()) return
  wakeUi()

  if (e.key === 'Escape') {
    if (openMenu.value !== null || browseOpen.value) {
      closeMenus()
      e.preventDefault()
    }
    return
  }

  const action = mapKeyToAction(e)
  if (!action) return
  e.preventDefault()

  switch (action.type) {
    case 'play-pause':
      togglePlay()
      break
    case 'seek-rel':
      onSeekRel(action.value)
      break
    case 'vol-rel': {
      const next = Math.max(0, Math.min(100, state.volume.value + action.value))
      if (state.muted.value && action.value > 0) onToggleMute()
      onSetVolume(next)
      break
    }
    case 'seek-pct': {
      const v = videoRef.value
      if (v && v.duration) {
        v.currentTime = (action.value / 100) * v.duration
        writeProgress()
      }
      break
    }
    case 'sub-offset': {
      const next = Math.round((state.subOffset.value + action.value) * 10) / 10
      state.subOffset.value = next
      break
    }
    case 'mute':
      onToggleMute()
      break
    case 'fullscreen':
      onToggleFullscreen()
      break
    case 'subs':
      toggleMenu('subs')
      break
    case 'pip':
      onTogglePip()
      break
    case 'next-episode':
      // Shift+N (anytime) advances whenever a next episode exists; bare `n` is
      // prompt-scoped — only acts while the countdown card or the end chip is up.
      if (
        anime_hasNextEp.value &&
        (action.anytime || showNextEpisode.value || showNextEpChip.value)
      ) {
        goToNextEpisode()
      }
      break
  }
}

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
  window.addEventListener('pagehide', onProtocolUsagePageHide)
  document.addEventListener('visibilitychange', onVisibilityChange)
  document.addEventListener('fullscreenchange', onFullscreenChange)
})

// Protocol ladder tier-change subscription: a downshift/upshift means the
// active stream's base origin changed, so re-resolve at the current position
// to pick up the new tier via hlsProxyUrl (the source itself hasn't changed).
const unsubLadder = ladder.onChange((tier, reason) => {
  recordDecision(`protocol ladder → ${tier.id} (${reason})`)
  const ep = selectedEpisode.value
  if (ep) void resolveStreamForEpisode(ep, true) // position-preserving swap; new base flows via hlsProxyUrl
})
onUnmounted(unsubLadder)

// ── Protocol (h1/h2/h3) usage telemetry ───────────────────────────────────────
// One anonymous event per (session × tier): a residency summary from the ladder
// + the dropped-video-frame delta over that residency. Emitted on every tier
// switch (onResidencyEnd) and once at session end (pagehide / unmount).
const telemetrySess =
  's_' +
  (typeof crypto !== 'undefined' && crypto.randomUUID
    ? crypto.randomUUID().slice(0, 8)
    : Math.random().toString(36).slice(2, 10))
// Cumulative video-quality counters snapshotted at the current tier's start.
let tierStartQuality = readVideoQuality(null) // { dropped: 0, total: 0 }

function emitProtocolUsage(r: TierResidency): void {
  const q = readVideoQuality(videoRef.value)
  const pct = droppedFramesPct(tierStartQuality, q)
  tierStartQuality = q // the next tier's residency measures from here
  const c = state.combo.value
  recordPlayerEvent({
    kind: 'protocol_usage',
    provider: c.provider,
    anime_id: props.animeId,
    episode: selectedEpisode.value?.number,
    audio: c.audio,
    lang: c.lang,
    detail: buildProtocolUsageDetail(r, pct, {
      animeName: props.anime.title,
      combo: `${c.audio}·${c.lang}·${c.provider}`,
      sess: telemetrySess,
    }),
  })
}

// Session-end flush of the final (never-switched-away) tier. consumeResidency
// dedups a pagehide-then-unmount double call; the explicit flush guarantees the
// last row ships even if this pagehide listener runs after telemetry's own.
function flushProtocolUsage(): void {
  const r = ladder.consumeResidency()
  if (r) {
    emitProtocolUsage(r)
    flushPlayerTelemetry('protocol-usage-final')
  }
}

// pagehide with persisted=true means the page is entering the bfcache
// (backgrounding, may be restored) — NOT terminal. Emitting+consuming the
// residency here would double-count this tier's segments if playback resumes
// and the tier later switches. Only flush on a real unload (persisted=false).
function onProtocolUsagePageHide(e: PageTransitionEvent): void {
  if (e.persisted) return
  flushProtocolUsage()
}

const unsubResidency = ladder.onResidencyEnd(emitProtocolUsage)
onUnmounted(unsubResidency)

// h3 upshift probe: sampled once, 30s after playback actually starts (not on
// mount — no point probing a stream that hasn't begun loading fragments).
let h3ProbeTimer: ReturnType<typeof setTimeout> | null = null
watch(hasStarted, (started) => {
  if (!started || h3ProbeTimer) return
  h3ProbeTimer = setTimeout(() => {
    if (!state.playing.value) return
    // C2: no fragment sample URL means an MP4/native-HLS session (no hls.js
    // XHR fragments ever populate lastFragUrl) — probing with '' resolves to
    // the h3 origin's root, and there's no EWMA baseline to sanity-check the
    // result against either. Leave the timer's one-shot semantics as-is
    // (never re-armed) and just skip this probe entirely.
    if (!engine.lastFragUrl.value) return
    void probeH3(ladder, engine.lastFragUrl.value, currentStream.value?.url ?? '')
  }, 30_000)
})
onUnmounted(() => { if (h3ProbeTimer) clearTimeout(h3ProbeTimer) })

onUnmounted(() => {
  stopRaf()
  tracking.saveNow() // persist position when navigating away in-app
  clearNextEpTimer()
  clearUiIdleTimer()
  clearHudTimers()
  clearPlaybackWatchdog()
  if (bufferingTimer) clearTimeout(bufferingTimer)
  if (dlRefreshTimer) clearTimeout(dlRefreshTimer)
  if (singleTapTimer) clearTimeout(singleTapTimer)
  if (seekFlashTimer) clearTimeout(seekFlashTimer)
  window.removeEventListener('keydown', onKeydown)
  window.removeEventListener('pagehide', onPageHide)
  flushProtocolUsage()
  window.removeEventListener('pagehide', onProtocolUsagePageHide)
  document.removeEventListener('visibilitychange', onVisibilityChange)
  document.removeEventListener('fullscreenchange', onFullscreenChange)
  teardownPseudoFs()
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

.pl--theater {
  border-radius: 0;
  border: 0;
  aspect-ratio: auto;
  height: 100vh;
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
  width: 100vw;
  height: 100svh;
  aspect-ratio: auto;
  border-radius: 0;
  border: 0;
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
