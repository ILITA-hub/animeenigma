<template>
  <div class="min-h-screen pt-20 pb-20 md:pb-8">
    <div class="max-w-7xl mx-auto px-4">
      <!-- Header -->
      <div class="mb-8">
        <h1 class="text-2xl md:text-3xl font-bold text-white mb-2">{{ $t('rooms.title') }}</h1>
        <p class="text-white/60">Play anime-themed games with other fans!</p>
      </div>

      <!-- Room List View -->
      <template v-if="!currentRoom">
        <div class="flex items-center justify-between mb-6">
          <h2 class="text-xl font-semibold text-white">Available Rooms</h2>
          <Button @click="showCreateModal = true">
            <template #icon>
              <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4" />
              </svg>
            </template>
            {{ $t('rooms.create') }}
          </Button>
        </div>

        <!-- Loading State -->
        <div v-if="loading" class="flex justify-center py-20">
          <div class="w-12 h-12 border-2 border-cyan-400 border-t-transparent rounded-full animate-spin" />
        </div>

        <!-- Empty State -->
        <div v-else-if="rooms.length === 0" class="text-center py-20">
          <svg class="w-16 h-16 mx-auto text-white/20 mb-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M17 20h5v-2a3 3 0 00-5.356-1.857M17 20H7m10 0v-2c0-.656-.126-1.283-.356-1.857M7 20H2v-2a3 3 0 015.356-1.857M7 20v-2c0-.656.126-1.283.356-1.857m0 0a5.002 5.002 0 019.288 0M15 7a3 3 0 11-6 0 3 3 0 016 0zm6 3a2 2 0 11-4 0 2 2 0 014 0zM7 10a2 2 0 11-4 0 2 2 0 014 0z" />
          </svg>
          <p class="text-white/50 text-lg mb-4">No active rooms</p>
          <Button variant="outline" @click="showCreateModal = true">Create the first room</Button>
        </div>

        <!-- Rooms Grid -->
        <div v-else class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
          <button
            v-for="room in rooms"
            :key="room.id"
            class="text-left p-5 rounded-xl glass-card card-hover border border-white/10 hover:border-cyan-500/30 transition-all"
            @click="joinRoom(room.id)"
          >
            <div class="flex items-start justify-between mb-3">
              <h3 class="text-lg font-semibold text-white">{{ room.name }}</h3>
              <Badge
                :variant="room.status === 'waiting' ? 'warning' : 'success'"
                size="sm"
              >
                {{ $t(`rooms.status.${room.status}`) }}
              </Badge>
            </div>
            <p class="text-cyan-400 mb-3 capitalize">{{ room.gameType.replace('-', ' ') }}</p>
            <div class="flex items-center justify-between text-sm text-white/50">
              <span class="flex items-center gap-1">
                <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4.354a4 4 0 110 5.292M15 21H3v-1a6 6 0 0112 0v1zm0 0h6v-1a6 6 0 00-9-5.197M13 7a4 4 0 11-8 0 4 4 0 018 0z" />
                </svg>
                {{ room.players }}/{{ room.maxPlayers }} {{ $t('rooms.players') }}
              </span>
              <span v-if="room.host" class="truncate max-w-[120px]">
                {{ $t('rooms.host') }}: {{ room.host }}
              </span>
            </div>
          </button>
        </div>
      </template>

      <!-- In-Room View -->
      <template v-else>
        <!-- Room Header -->
        <div class="flex items-center justify-between mb-6 pb-4 border-b border-white/10">
          <div>
            <h2 class="text-xl font-semibold text-white">{{ currentRoom.name }}</h2>
            <p class="text-cyan-400 capitalize">{{ currentRoom.gameType?.replace('-', ' ') }}</p>
          </div>
          <Button variant="secondary" size="sm" @click="leaveRoom">
            {{ $t('rooms.leave') }}
          </Button>
        </div>

        <!-- Game Layout -->
        <div class="grid grid-cols-1 lg:grid-cols-4 gap-6">
          <!-- Players Sidebar -->
          <div class="lg:col-span-1">
            <div class="glass-card p-4 sticky top-24">
              <h3 class="text-lg font-semibold text-white mb-4">
                Players ({{ currentRoom.players?.length || 0 }})
              </h3>
              <div class="space-y-2">
                <div
                  v-for="player in currentRoom.players"
                  :key="player.id"
                  class="flex items-center justify-between p-3 rounded-lg bg-white/5"
                >
                  <div class="flex items-center gap-2">
                    <div class="w-8 h-8 rounded-full bg-cyan-500/20 flex items-center justify-center text-cyan-400 text-sm font-medium">
                      {{ player.username?.slice(0, 2).toUpperCase() }}
                    </div>
                    <span class="text-white text-sm">{{ player.username }}</span>
                  </div>
                  <span class="text-amber-400 font-bold">{{ player.score || 0 }}</span>
                </div>
              </div>

              <!-- Leaderboard -->
              <div v-if="currentRoom.status !== 'waiting'" class="mt-6 pt-4 border-t border-white/10">
                <h4 class="text-sm font-medium text-white/60 mb-3">{{ $t('rooms.leaderboard') }}</h4>
                <div class="space-y-2">
                  <div
                    v-for="(player, index) in sortedPlayers"
                    :key="player.id"
                    class="flex items-center gap-2"
                  >
                    <span class="w-5 text-center" :class="index === 0 ? 'text-amber-400' : 'text-white/40'">
                      {{ index + 1 }}
                    </span>
                    <span class="flex-1 text-white/70 truncate">{{ player.username }}</span>
                    <span class="text-amber-400 font-bold">{{ player.score }}</span>
                  </div>
                </div>
              </div>
            </div>
          </div>

          <!-- Main Game Area -->
          <div class="lg:col-span-2">
            <div class="glass-card p-6 min-h-[400px] flex items-center justify-center">
              <!-- Waiting State -->
              <div v-if="currentRoom.status === 'waiting'" class="text-center">
                <div class="w-16 h-16 border-2 border-cyan-400/30 border-t-cyan-400 rounded-full animate-spin mx-auto mb-4" />
                <h3 class="text-xl font-semibold text-white mb-2">{{ $t('rooms.status.waiting') }}</h3>
                <p class="text-white/50">Game will start when enough players join</p>
                <p class="text-cyan-400 mt-4">
                  {{ $t('rooms.round') }} {{ currentRoom.currentRound || 1 }}
                </p>
              </div>

              <!-- Active Game -->
              <div v-else class="w-full">
                <div class="text-center mb-8">
                  <Badge variant="primary" size="md" class="mb-4">
                    {{ $t('rooms.round') }} {{ currentRoom.currentRound || 1 }}
                  </Badge>
                  <h3 class="text-xl md:text-2xl font-semibold text-white">
                    {{ currentRoom.currentQuestion?.text || 'Loading question...' }}
                  </h3>
                </div>

                <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
                  <button
                    v-for="(answer, index) in currentRoom.currentQuestion?.options"
                    :key="index"
                    class="p-4 rounded-xl text-left transition-all"
                    :class="[
                      selectedAnswer === index
                        ? 'bg-cyan-500/20 border-2 border-cyan-500'
                        : 'bg-white/5 border border-white/10 hover:bg-white/10 hover:border-cyan-500/30'
                    ]"
                    :disabled="hasAnswered"
                    @click="submitAnswer(index)"
                  >
                    <span class="text-white">{{ answer }}</span>
                  </button>
                </div>
              </div>
            </div>
          </div>

          <!-- Chat Sidebar -->
          <div class="lg:col-span-1">
            <div class="glass-card p-4 h-[500px] flex flex-col">
              <h3 class="text-lg font-semibold text-white mb-4">Chat</h3>

              <!-- Messages -->
              <div ref="chatMessagesRef" class="flex-1 overflow-y-auto space-y-2 mb-4 scrollbar-hide">
                <div
                  v-for="msg in chatMessages"
                  :key="msg.id"
                  class="p-2 rounded-lg bg-white/5"
                >
                  <span class="text-cyan-400 font-medium">{{ msg.username }}:</span>
                  <span class="text-white/70 ml-1">{{ msg.text }}</span>
                </div>
              </div>

              <!-- Input -->
              <div class="flex gap-2">
                <Input
                  v-model="chatInput"
                  placeholder="Type a message..."
                  size="sm"
                  @keyup.enter="sendMessage"
                />
                <Button size="sm" @click="sendMessage">
                  <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 19l9 2-9-18-9 18 9-2zm0 0v-8" />
                  </svg>
                </Button>
              </div>
            </div>
          </div>
        </div>
      </template>
    </div>

    <!-- Create Room Modal -->
    <Modal v-model="showCreateModal" :title="$t('rooms.create')">
      <form @submit.prevent="createRoom" class="space-y-4">
        <Input
          v-model="newRoom.name"
          label="Room Name"
          placeholder="Enter room name"
          required
        />
        <Select
          v-model="newRoom.gameType"
          :options="gameTypeOptions"
          label="Game Type"
        />
        <div>
          <label class="block text-sm font-medium text-white/70 mb-2">Max Players</label>
          <input
            v-model.number="newRoom.maxPlayers"
            type="number"
            min="2"
            max="20"
            class="w-full bg-white/5 border border-white/10 rounded-xl px-4 py-3 text-white focus:outline-none focus:border-cyan-500"
            required
          />
        </div>
      </form>
      <template #footer>
        <Button variant="ghost" @click="showCreateModal = false">{{ $t('common.cancel') }}</Button>
        <Button @click="createRoom">{{ $t('rooms.create') }}</Button>
      </template>
    </Modal>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, nextTick } from 'vue'
import { useRoute } from 'vue-router'
import { gameApi } from '@/api/client'
import { io, Socket } from 'socket.io-client'
import { Button, Badge, Input, Modal, Select } from '@/components/ui'

interface Player {
  id: string
  username: string
  score: number
}

interface Room {
  id: string
  name: string
  gameType: string
  status: string
  players: number
  maxPlayers: number
  host?: string
}

interface CurrentRoom {
  id: string
  name: string
  gameType: string
  status: string
  players: Player[]
  currentRound?: number
  currentQuestion?: {
    text: string
    options: string[]
  }
}

interface ChatMessage {
  id: string
  username: string
  text: string
}

const route = useRoute()
const loading = ref(false)
const rooms = ref<Room[]>([])
const currentRoom = ref<CurrentRoom | null>(null)
const showCreateModal = ref(false)
const chatMessages = ref<ChatMessage[]>([])
const chatMessagesRef = ref<HTMLElement | null>(null)
const chatInput = ref('')
const socket = ref<Socket | null>(null)
const selectedAnswer = ref<number | null>(null)
const hasAnswered = ref(false)

const newRoom = ref({
  name: '',
  gameType: 'anime-quiz',
  maxPlayers: 10
})

const gameTypeOptions = [
  { value: 'anime-quiz', label: 'Anime Quiz' },
  { value: 'character-guess', label: 'Character Guess' },
  { value: 'opening-quiz', label: 'Opening Quiz' },
]

const sortedPlayers = computed(() => {
  if (!currentRoom.value?.players) return []
  return [...currentRoom.value.players].sort((a, b) => (b.score || 0) - (a.score || 0))
})

const loadRooms = async () => {
  loading.value = true
  try {
    const response = await gameApi.getRooms()
    rooms.value = response.data
  } catch (err) {
    console.error('Failed to load rooms:', err)
  } finally {
    loading.value = false
  }
}

const createRoom = async () => {
  try {
    const response = await gameApi.createRoom(newRoom.value)
    currentRoom.value = response.data
    showCreateModal.value = false
    if (currentRoom.value) {
      connectSocket(currentRoom.value.id)
    }
    newRoom.value = { name: '', gameType: 'anime-quiz', maxPlayers: 10 }
  } catch (err) {
    console.error('Failed to create room:', err)
  }
}

const joinRoom = async (roomId: string) => {
  try {
    const response = await gameApi.joinRoom(roomId)
    currentRoom.value = response.data
    connectSocket(roomId)
  } catch (err) {
    console.error('Failed to join room:', err)
  }
}

const leaveRoom = async () => {
  if (currentRoom.value) {
    try {
      await gameApi.leaveRoom(currentRoom.value.id)
      socket.value?.disconnect()
      currentRoom.value = null
      selectedAnswer.value = null
      hasAnswered.value = false
      await loadRooms()
    } catch (err) {
      console.error('Failed to leave room:', err)
    }
  }
}

const connectSocket = (roomId: string) => {
  const socketUrl = import.meta.env.VITE_SOCKET_URL || ''
  socket.value = io(socketUrl, {
    auth: { token: localStorage.getItem('token') }
  })

  socket.value.emit('join-room', roomId)

  socket.value.on('room-update', (data: CurrentRoom) => {
    currentRoom.value = data
    selectedAnswer.value = null
    hasAnswered.value = false
  })

  socket.value.on('chat-message', (message: ChatMessage) => {
    chatMessages.value.push(message)
    nextTick(() => {
      if (chatMessagesRef.value) {
        chatMessagesRef.value.scrollTop = chatMessagesRef.value.scrollHeight
      }
    })
  })

  socket.value.on('game-start', () => {
    console.log('Game started!')
  })
}

const submitAnswer = (answerIndex: number) => {
  if (hasAnswered.value) return

  selectedAnswer.value = answerIndex
  hasAnswered.value = true

  if (socket.value && currentRoom.value) {
    socket.value.emit('submit-answer', {
      roomId: currentRoom.value.id,
      answer: answerIndex
    })
  }
}

const sendMessage = () => {
  if (chatInput.value.trim() && socket.value && currentRoom.value) {
    socket.value.emit('chat-message', {
      roomId: currentRoom.value.id,
      text: chatInput.value
    })
    chatInput.value = ''
  }
}

onMounted(async () => {
  if (route.params.roomId) {
    await joinRoom(route.params.roomId as string)
  } else {
    await loadRooms()
  }
})

onUnmounted(() => {
  socket.value?.disconnect()
})
</script>
