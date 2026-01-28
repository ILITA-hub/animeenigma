import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import videojs from 'video.js'

export interface Episode {
  id: string
  animeId: string
  episodeNumber: number
  title: string
  duration: number
  sources: VideoSource[]
}

export interface VideoSource {
  url: string
  quality: string
  type: string
}

export interface WatchProgress {
  episodeId: string
  currentTime: number
  duration: number
  completed: boolean
}

export const usePlayerStore = defineStore('player', () => {
  const currentEpisode = ref<Episode | null>(null)
  const currentTime = ref(0)
  const duration = ref(0)
  const isPlaying = ref(false)
  const volume = ref(1)
  const quality = ref('auto')
  const player = ref<ReturnType<typeof videojs> | null>(null)

  const progress = computed(() => {
    if (duration.value === 0) return 0
    return (currentTime.value / duration.value) * 100
  })

  const setEpisode = (episode: Episode) => {
    currentEpisode.value = episode
  }

  const setPlayer = (videoPlayer: ReturnType<typeof videojs>) => {
    player.value = videoPlayer

    // Setup event listeners
    videoPlayer.on('timeupdate', () => {
      currentTime.value = videoPlayer.currentTime() || 0
    })

    videoPlayer.on('durationchange', () => {
      duration.value = videoPlayer.duration() || 0
    })

    videoPlayer.on('play', () => {
      isPlaying.value = true
    })

    videoPlayer.on('pause', () => {
      isPlaying.value = false
    })

    videoPlayer.on('volumechange', () => {
      volume.value = videoPlayer.volume() || 0
    })
  }

  const play = () => {
    player.value?.play()
  }

  const pause = () => {
    player.value?.pause()
  }

  const seek = (time: number) => {
    if (player.value) {
      player.value.currentTime(time)
    }
  }

  const setVolume = (vol: number) => {
    if (player.value) {
      player.value.volume(vol)
    }
  }

  const setQuality = (qualityLevel: string) => {
    quality.value = qualityLevel
    // TODO: Implement quality switching logic
  }

  const saveProgress = async () => {
    if (!currentEpisode.value) return

    const progressData: WatchProgress = {
      episodeId: currentEpisode.value.id,
      currentTime: currentTime.value,
      duration: duration.value,
      completed: progress.value > 90
    }

    // TODO: Save to backend
    console.log('Saving progress:', progressData)
  }

  const loadProgress = async (episodeId: string) => {
    // TODO: Load from backend
    console.log('Loading progress for:', episodeId)
  }

  const reset = () => {
    currentEpisode.value = null
    currentTime.value = 0
    duration.value = 0
    isPlaying.value = false
    player.value = null
  }

  return {
    currentEpisode,
    currentTime,
    duration,
    isPlaying,
    volume,
    quality,
    progress,
    setEpisode,
    setPlayer,
    play,
    pause,
    seek,
    setVolume,
    setQuality,
    saveProgress,
    loadProgress,
    reset
  }
})
