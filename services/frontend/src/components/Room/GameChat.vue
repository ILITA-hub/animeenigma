<template>
  <div class="chat-container">
    <h2>Чат игры</h2>
    <v-card class="chat-card">
      <div class="messages-container">
        <div v-for="message in roomStore.players" class="message-container">

          <div class="text-container">
            <span>{{ message.name }} Крутая игра</span>
          </div>
          <v-divider class="mt-2"></v-divider>
        </div>
      </div>
    </v-card>
  </div>
</template>

<script setup>
import { useRoomStore } from '@/stores/roomStore';

const roomStore = useRoomStore()
</script>



<style scoped>
.chat-container{
  margin-top: 20px;
}
.message-container {
  display: flex;
  flex-direction: column;
  justify-content: center;
  padding: 10px;
}
.messages-container{
  height: 265px;
  overflow: auto;
}

.text-container{
  width: 100%;

}
.chat-card {
  background-color: #212335;
  color: #fff;
}
</style>