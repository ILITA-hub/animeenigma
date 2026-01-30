<template>
  <div class="video-player-wrapper">
    <video ref="videoElement" class="video-js vjs-default-skin vjs-big-play-centered"></video>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted, watch } from 'vue'
import videojs from 'video.js'
import { usePlayerStore, type Episode } from '@/stores/player'

const props = defineProps<{
  episode: Episode
}>()

const videoElement = ref<HTMLVideoElement | null>(null)
const player = ref<ReturnType<typeof videojs> | null>(null)
const playerStore = usePlayerStore()

const initPlayer = () => {
  if (!videoElement.value || !props.episode.sources.length) return

  const options = {
    controls: true,
    autoplay: false,
    preload: 'auto',
    fluid: true,
    aspectRatio: '16:9',
    playbackRates: [0.5, 0.75, 1, 1.25, 1.5, 2],
    controlBar: {
      volumePanel: { inline: false },
      children: [
        'playToggle',
        'volumePanel',
        'currentTimeDisplay',
        'timeDivider',
        'durationDisplay',
        'progressControl',
        'remainingTimeDisplay',
        'playbackRateMenuButton',
        'qualitySelector',
        'fullscreenToggle'
      ]
    },
    sources: props.episode.sources.map(source => ({
      src: source.url,
      type: source.type || 'video/mp4',
      label: source.quality
    }))
  }

  player.value = videojs(videoElement.value, options)
  playerStore.setPlayer(player.value as ReturnType<typeof videojs>)

  // Auto-save progress every 10 seconds
  const saveInterval = setInterval(() => {
    playerStore.saveProgress()
  }, 10000)

  // Save progress before leaving
  player.value.on('beforeunload', () => {
    playerStore.saveProgress()
  })

  // Load saved progress
  playerStore.loadProgress(props.episode.id)

  // Cleanup interval on component unmount
  onUnmounted(() => {
    clearInterval(saveInterval)
  })
}

watch(() => props.episode, (newEpisode, oldEpisode) => {
  if (newEpisode?.id !== oldEpisode?.id) {
    if (player.value) {
      playerStore.saveProgress()
      player.value.src(newEpisode.sources.map(source => ({
        src: source.url,
        type: source.type || 'video/mp4',
        label: source.quality
      })))
      playerStore.setEpisode(newEpisode)
      playerStore.loadProgress(newEpisode.id)
    }
  }
}, { deep: true })

onMounted(() => {
  initPlayer()
})

onUnmounted(() => {
  if (player.value) {
    playerStore.saveProgress()
    player.value.dispose()
    playerStore.reset()
  }
})
</script>

<style scoped>
.video-player-wrapper {
  width: 100%;
  background: #000;
}

.video-js {
  width: 100%;
  height: 100%;
}

/* Override Video.js default styles */
:deep(.video-js) {
  font-family: inherit;
}

:deep(.video-js .vjs-big-play-button) {
  background-color: rgba(255, 107, 107, 0.9);
  border: none;
  border-radius: 50%;
  width: 2em;
  height: 2em;
  line-height: 2em;
  font-size: 3em;
  transition: all 0.3s;
}

:deep(.video-js .vjs-big-play-button:hover) {
  background-color: rgb(255, 107, 107);
  transform: scale(1.1);
}

:deep(.video-js .vjs-control-bar) {
  background-color: rgba(26, 26, 26, 0.9);
  backdrop-filter: blur(10px);
}

:deep(.video-js .vjs-play-progress) {
  background-color: #ff6b6b;
}

:deep(.video-js .vjs-volume-level) {
  background-color: #ff6b6b;
}

:deep(.video-js .vjs-slider-horizontal .vjs-volume-level:before) {
  color: #ff6b6b;
}

:deep(.video-js .vjs-load-progress) {
  background: rgba(255, 255, 255, 0.2);
}

:deep(.video-js .vjs-progress-holder) {
  height: 0.5em;
}

:deep(.video-js .vjs-play-progress:before) {
  font-size: 1em;
  top: -0.25em;
}

:deep(.video-js:hover .vjs-big-play-button),
:deep(.video-js .vjs-big-play-button:focus) {
  background-color: rgb(255, 107, 107);
}
</style>
