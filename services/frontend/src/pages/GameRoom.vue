<template>
  <div class="room-container">
    <div class="player-container">
      <div v-if="isGameStarted" ref="videoContainer" class="video-container">
        <video ref="video" id="player" v-show="isAnswerGeted">
          <source ref="videoSource" src="" type="video/mp4">
        </video>
      </div>
      <div v-else class="start-button-container">
        <v-btn @click="userStartGame" class="btn-answer">Играть</v-btn>
      </div>
      <div class="varints-ansver-container">

        <v-btn
          :class="`btn-answer ${answer?.id === rightAnswer?.id ? 'true-answer' : ''} ${answer?.id === userAnswer?.id ? 'choosed-answer' : ''} ${(answer?.id === userAnswer?.id && userAnswer?.id !== rightAnswer?.id && isAnswerGeted) ? 'wrong-answer' : ''}`"
          v-for="answer in variantAnswers" @click="setUserAnswer(answer)">
          {{ answer.name }}
        </v-btn>
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
import { ref } from 'vue';
import GameChat from '@/components/Room/GameChat.vue';


const roomStore = useRoomStore();
const userStore = useAuthStore()
const router = useRouter();
const route = useRoute();

const variantAnswers = ref(null)
const currentVideoUrl = ref('')
const videoContainer = ref(null)
const rightAnswer = ref(null)
const userAnswer = ref(null)
let isGameStarted = ref(false)
const roomUsers = ref(null)
const videoSource = ref(null)
const video = ref(null)
let isAnswerGeted = false


let gameSocket = new WebSocket(`ws://46.181.201.172:1234/${route.params.uniqUrl}/${userStore.user.token}`);
let clientId = ''

gameSocket.onmessage = function (event) {
  let body = JSON.parse(event.data)
  console.log(body)
  wsTypes[body.type](body)
}

let wsTypes = {
  connect: connect,
  startGame: ServerStartGame,// Все нажали ready

  newOpening: newQuestion, //Когда пришел новый опенинг

  startOpening: startOpening,// Начинаем проигрывать опенинг

  endOpening: endOpening,// Приходит правильный ответ и раскрывается видео

  updUsers: setRoomUsers,// Подключаются новые пользователи (в разработке)
  endGame: endGame,// Конец игры (в разработке)
}


async function userStartGame() {
  gameSocket.send(JSON.stringify({ clientId: clientId, type: 'userIsReady' }))
}
function canplay(params) {
  gameSocket.send(JSON.stringify({ clientId: clientId, type: 'openingIsLoaded' }))
}

async function setNewOpening() {
  await new Promise((res, rej) => {
    setTimeout(() => {
      res()
    }, 500)
  })

  videoSource.value.src = currentVideoUrl.value;

  video.value.load();
  video.value.removeEventListener('canplay', canplay)
  video.value.addEventListener('canplay', canplay);
}

function setRoomUsers(body) {
  roomUsers.value = body.users
  roomStore.players = body.users
  console.log(roomStore.players)
}

function startOpening(body) {

  video.value.play()
}

function connect(body) {
  clientId = body.clientId
}
function ServerStartGame(body) {
}
function endGame(body) {
}

function getButtonClass(answer) {
  return {
    'btn-answer': true,
    'true-answer': answer?.id === rightAnswer?.id,
    'choosed-answer': answer?.id === userAnswer?.id,
    'wrong-answer': answer?.id === userAnswer?.id && userAnswer?.id !== rightAnswer?.id && isAnswerGeted
  };
}

function endOpening(body) {
  isAnswerGeted = true
  gameSocket.send(JSON.stringify({ clientId: clientId, type: 'checkAnswer', answer: userAnswer.value?.id }))
  rightAnswer.value = body.trueAnswer
}
async function newQuestion(body) {
  isAnswerGeted = false
  rightAnswer.value = null
  userAnswer.value = null
  variantAnswers.value = body.answers
  currentVideoUrl.value = body.opening
  isGameStarted.value = true
  await setNewOpening()
}

function setUserAnswer(answer) {
  console.log(answer)
  userAnswer.value = answer
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

.choosed-answer {
  background-color: #2196F3!important;
}
.wrong-answer {
  background-color: #F44336!important;
}
.true-answer {
  background-color: green !important;
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
