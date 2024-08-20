<template>
  <div class="players-comp-container">
    <h2>Список игроков</h2>
    <v-card class="players-room-card">
      <div v-for="player in roomStore.players" class="player-container">

        <div class="content-container">
          <v-avatar color="surface-variant" size="40">
            <v-img :src="player.imgSrc">
            </v-img>
          </v-avatar>
          <span class="player-nickname">{{ player.name }}</span>
          <div class="player-points">
            {{ player.points }} очков
          </div>
        </div>
        <v-divider class="mt-2"></v-divider>
      </div>
    </v-card>
  </div>
</template>

<script setup>
import { useRoomStore } from '@/stores/roomStore';

const roomStore = useRoomStore()
</script>


<style scoped>
.players-comp-container {}


.content-container{
  display: flex;
  align-items: center;
}

.player-points{
  margin-left: 60px;
  border-radius: 10px;
  background: rgba(20, 112, 239, 0.15);
  padding: 6px 25px;
  color: rgba(20, 112, 239, 1);
}

.player-nickname{
  margin-left: 10px;
}
.player-container {
  justify-content: center;
  padding: 5px;
}

.players-room-card {
  background-color: #212335;
  color: #fff;
  display: flex;
  flex-direction: column;
  gap: 10px;
  height: 335px;
  overflow: auto;
}
</style>