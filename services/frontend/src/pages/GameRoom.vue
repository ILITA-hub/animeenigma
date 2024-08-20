<template>
  <div class="room-container">

    <div class="player-container">
      <div v-if="isGameStarted" ref="videoContainer" class="video-container">
        <video id="player" width="800" height="450">
        </video>
      </div>
      <div v-else class="start-button-container">
        <v-btn @click="userStartGame" class="btn-answer">Играть</v-btn>
      </div>
      <div class="varints-ansver-container">
        <v-btn class="btn-answer" v-for="variant in 4">{{ variant }}</v-btn>
      </div>
    </div>

    <div class="right-container">
      <RoomPlayers />
      <GameChat />
    </div>
    

  </div>
</template>

<script setup>
import { useRoomStore } from '@/stores/roomStore';
import { useRouter, useRoute } from 'vue-router';
import RoomPlayers from '@/components/Room/RoomPlayers.vue';
import { useAuthStore } from '@/stores/authStore';
import Plyr from 'plyr';
import 'plyr/dist/plyr.css';
import { ref } from 'vue';
import GameChat from '@/components/Room/GameChat.vue';


const roomStore = useRoomStore();
const userStore = useAuthStore()
const router = useRouter();
const route = useRoute();

const variantAnswers = ref({})
const currentVideoUrl = ref('')
const videoContainer = ref(null)
const iframe = ref(null)
let isGameStarted = ref(false)

let gameSocket = new WebSocket("ws://46.181.201.172:1234/");
let roomUniq = ''


gameSocket.onmessage = function (event) {
  let body = JSON.parse(event.data)
  console.log(body)
  wsTypes[body.type](body)
}

let wsTypes = {
  connect: connect,
  startGame: ServerStartGame,
  newQuestion: newQuestion,
  resultCurrentQuestion: 'resultCurrentQuestion',
  endGame: endGame,
}


async function userStartGame() {
  gameSocket.send(JSON.stringify({ roomId: route.params.uniqUrl, user: roomUniq, type: 'newQuestion' }))
  await new Promise((res, rej) => {
    setInterval(() => {
      if (isGameStarted.value) {
        res()
      }
    }, 500)
  })
  console.log('start')
  const player = new Plyr('#player', {
    controls: [

    ]
  });
  player.source = {
    type: 'video',
    sources: [
      {
        src: currentVideoUrl.value,
        provider: 'youtube',
      },
    ],
  };
  // console.log(videoContainer.value.style)
  player.on('ready', (event) => {
    player.play()
  });

}

function connect(body) {
  roomUniq = body.clientId
  gameSocket.send(JSON.stringify({ roomId: route.params.uniqUrl, user: roomUniq, type: 'connect' }))
}
function ServerStartGame(body) {

}
function endGame(body) {

}
function newQuestion(body) {
  console.log(body.opening)
  currentVideoUrl.value = body.opening
  isGameStarted.value = true
}
function userAnswer(body) {

}
function handelServerAnswer(body) {

}

</script>

<style scoped>
.room-container {
  color: #fff;
  display: flex;
  align-items: center;
  justify-content: space-between;
  max-width: 1440px;
  margin: auto;
  margin-top: 20px;
}

.player-container {
  display: flex;
  flex-direction: column;
}

.start-button-container {
  width: 954px;
  height: 536px;
  display: flex;
  align-items: center;
  justify-content: center;
}

.video-container {
  display: flex;
  height: 537px;
  overflow: hidden;
  border-radius: 10px;
}

.varints-ansver-container {
  color: #000;
  display: flex;
  justify-content: center;
  margin-top: 20px;
  gap: 10px;
  width: 954px;
  flex-wrap: wrap;
}

.btn-answer {
  color: #fff;
  width: 472px;
  height: 65px !important;
  background-color: #212335;
  border-radius: 10px !important;
}

.plyr {
  width: 100% !important;
  height: 100% !important;
}
</style>
