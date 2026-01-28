<template>
  <div class="game-page">
    <div class="container">
      <h1>Anime Game Rooms</h1>
      <p class="subtitle">Play anime-themed games with other fans!</p>

      <div v-if="!currentRoom">
        <div class="rooms-header">
          <h2>Available Rooms</h2>
          <button @click="showCreateModal = true" class="btn btn-primary">
            Create Room
          </button>
        </div>

        <div v-if="loading" class="loading">Loading rooms...</div>
        <div v-else-if="rooms.length === 0" class="empty">
          No active rooms. Create one to get started!
        </div>
        <div v-else class="rooms-grid">
          <div
            v-for="room in rooms"
            :key="room.id"
            class="room-card"
            @click="joinRoom(room.id)"
          >
            <h3>{{ room.name }}</h3>
            <p class="room-game">{{ room.gameType }}</p>
            <div class="room-info">
              <span>{{ room.players }}/{{ room.maxPlayers }} Players</span>
              <span :class="['status', room.status]">{{ room.status }}</span>
            </div>
          </div>
        </div>
      </div>

      <div v-else class="game-room">
        <div class="room-header">
          <div>
            <h2>{{ currentRoom.name }}</h2>
            <p>{{ currentRoom.gameType }}</p>
          </div>
          <button @click="leaveRoom" class="btn btn-secondary">Leave Room</button>
        </div>

        <div class="game-area">
          <div class="players-list">
            <h3>Players ({{ currentRoom.players?.length || 0 }})</h3>
            <div
              v-for="player in currentRoom.players"
              :key="player.id"
              class="player-item"
            >
              <span>{{ player.username }}</span>
              <span class="score">{{ player.score || 0 }} pts</span>
            </div>
          </div>

          <div class="game-content">
            <div v-if="currentRoom.status === 'waiting'" class="waiting">
              <h3>Waiting for players...</h3>
              <p>Game will start when enough players join</p>
            </div>
            <div v-else class="game-active">
              <h3>{{ currentRoom.currentQuestion?.text }}</h3>
              <div class="answers">
                <button
                  v-for="(answer, index) in currentRoom.currentQuestion?.options"
                  :key="index"
                  @click="submitAnswer(index)"
                  class="answer-btn"
                >
                  {{ answer }}
                </button>
              </div>
            </div>
          </div>

          <div class="chat">
            <h3>Chat</h3>
            <div class="chat-messages" ref="chatMessages">
              <div
                v-for="msg in chatMessages"
                :key="msg.id"
                class="chat-message"
              >
                <strong>{{ msg.username }}:</strong> {{ msg.text }}
              </div>
            </div>
            <div class="chat-input">
              <input
                v-model="chatInput"
                @keyup.enter="sendMessage"
                placeholder="Type a message..."
              />
              <button @click="sendMessage">Send</button>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Create Room Modal -->
    <div v-if="showCreateModal" class="modal-overlay" @click="showCreateModal = false">
      <div class="modal" @click.stop>
        <h2>Create Game Room</h2>
        <form @submit.prevent="createRoom">
          <input v-model="newRoom.name" placeholder="Room Name" required />
          <select v-model="newRoom.gameType" required>
            <option value="anime-quiz">Anime Quiz</option>
            <option value="character-guess">Character Guess</option>
            <option value="opening-quiz">Opening Quiz</option>
          </select>
          <input
            v-model.number="newRoom.maxPlayers"
            type="number"
            min="2"
            max="20"
            placeholder="Max Players"
            required
          />
          <div class="modal-actions">
            <button type="button" @click="showCreateModal = false" class="btn btn-secondary">
              Cancel
            </button>
            <button type="submit" class="btn btn-primary">Create</button>
          </div>
        </form>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { useRoute } from 'vue-router'
import { gameApi } from '@/api/client'
import { io, Socket } from 'socket.io-client'

const route = useRoute()
const loading = ref(false)
const rooms = ref<any[]>([])
const currentRoom = ref<any>(null)
const showCreateModal = ref(false)
const chatMessages = ref<any[]>([])
const chatInput = ref('')
const socket = ref<Socket | null>(null)

const newRoom = ref({
  name: '',
  gameType: 'anime-quiz',
  maxPlayers: 10
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
    connectSocket(currentRoom.value.id)
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
      await loadRooms()
    } catch (err) {
      console.error('Failed to leave room:', err)
    }
  }
}

const connectSocket = (roomId: string) => {
  const socketUrl = import.meta.env.VITE_SOCKET_URL || 'http://localhost:8000'
  socket.value = io(socketUrl, {
    auth: { token: localStorage.getItem('token') }
  })

  socket.value.emit('join-room', roomId)

  socket.value.on('room-update', (data: any) => {
    currentRoom.value = data
  })

  socket.value.on('chat-message', (message: any) => {
    chatMessages.value.push(message)
  })

  socket.value.on('game-start', () => {
    console.log('Game started!')
  })
}

const submitAnswer = (answerIndex: number) => {
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

<style scoped>
.game-page {
  min-height: 100vh;
  padding: 2rem;
}

.container {
  max-width: 1200px;
  margin: 0 auto;
}

h1 {
  font-size: 2.5rem;
  margin-bottom: 0.5rem;
  color: #fff;
}

.subtitle {
  color: #999;
  margin-bottom: 2rem;
}

.rooms-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 2rem;
}

.rooms-header h2 {
  font-size: 1.8rem;
  color: #fff;
}

.rooms-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
  gap: 1.5rem;
}

.room-card {
  padding: 1.5rem;
  background: #1a1a1a;
  border: 2px solid #333;
  border-radius: 12px;
  cursor: pointer;
  transition: all 0.3s;
}

.room-card:hover {
  border-color: #ff6b6b;
  transform: translateY(-4px);
}

.room-card h3 {
  font-size: 1.3rem;
  margin-bottom: 0.5rem;
  color: #fff;
}

.room-game {
  color: #ff6b6b;
  margin-bottom: 1rem;
  text-transform: capitalize;
}

.room-info {
  display: flex;
  justify-content: space-between;
  color: #999;
  font-size: 0.9rem;
}

.status {
  padding: 0.25rem 0.5rem;
  border-radius: 4px;
  text-transform: uppercase;
  font-size: 0.75rem;
}

.status.waiting {
  background: #ffa500;
  color: #000;
}

.status.playing {
  background: #4caf50;
  color: #fff;
}

.game-room {
  background: #1a1a1a;
  border-radius: 12px;
  padding: 2rem;
}

.room-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 2rem;
  padding-bottom: 1rem;
  border-bottom: 2px solid #333;
}

.room-header h2 {
  font-size: 1.8rem;
  color: #fff;
}

.game-area {
  display: grid;
  grid-template-columns: 200px 1fr 300px;
  gap: 2rem;
}

.players-list,
.chat {
  background: #0f0f0f;
  border-radius: 8px;
  padding: 1rem;
}

.players-list h3,
.chat h3 {
  font-size: 1.2rem;
  margin-bottom: 1rem;
  color: #fff;
}

.player-item {
  display: flex;
  justify-content: space-between;
  padding: 0.5rem;
  margin-bottom: 0.5rem;
  background: #1a1a1a;
  border-radius: 4px;
}

.score {
  color: #ffd700;
  font-weight: bold;
}

.game-content {
  background: #0f0f0f;
  border-radius: 8px;
  padding: 2rem;
  display: flex;
  align-items: center;
  justify-content: center;
}

.waiting {
  text-align: center;
}

.waiting h3 {
  font-size: 1.5rem;
  color: #fff;
  margin-bottom: 1rem;
}

.game-active h3 {
  font-size: 1.3rem;
  color: #fff;
  margin-bottom: 2rem;
}

.answers {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 1rem;
}

.answer-btn {
  padding: 1rem;
  background: #1a1a1a;
  border: 2px solid #333;
  border-radius: 8px;
  color: white;
  cursor: pointer;
  transition: all 0.3s;
}

.answer-btn:hover {
  border-color: #ff6b6b;
  background: #222;
}

.chat-messages {
  height: 300px;
  overflow-y: auto;
  margin-bottom: 1rem;
  padding: 0.5rem;
}

.chat-message {
  margin-bottom: 0.5rem;
  color: #ccc;
  font-size: 0.9rem;
}

.chat-message strong {
  color: #ff6b6b;
}

.chat-input {
  display: flex;
  gap: 0.5rem;
}

.chat-input input {
  flex: 1;
  padding: 0.5rem;
  background: #1a1a1a;
  border: 1px solid #333;
  border-radius: 4px;
  color: white;
}

.chat-input button {
  padding: 0.5rem 1rem;
  background: #ff6b6b;
  border: none;
  border-radius: 4px;
  color: white;
  cursor: pointer;
}

.btn {
  padding: 0.75rem 1.5rem;
  border: none;
  border-radius: 8px;
  cursor: pointer;
  font-size: 1rem;
  transition: all 0.3s;
}

.btn-primary {
  background: #ff6b6b;
  color: white;
}

.btn-primary:hover {
  background: #ff5252;
}

.btn-secondary {
  background: #333;
  color: white;
}

.btn-secondary:hover {
  background: #444;
}

.modal-overlay {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.8);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
}

.modal {
  background: #1a1a1a;
  padding: 2rem;
  border-radius: 12px;
  width: 90%;
  max-width: 500px;
}

.modal h2 {
  font-size: 1.5rem;
  margin-bottom: 1.5rem;
  color: #fff;
}

.modal form input,
.modal form select {
  width: 100%;
  padding: 0.75rem;
  margin-bottom: 1rem;
  background: #0f0f0f;
  border: 2px solid #333;
  border-radius: 8px;
  color: white;
  font-size: 1rem;
}

.modal-actions {
  display: flex;
  gap: 1rem;
  justify-content: flex-end;
  margin-top: 1.5rem;
}

.loading,
.empty {
  text-align: center;
  padding: 3rem;
  color: #999;
}
</style>
