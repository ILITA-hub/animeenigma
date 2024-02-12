
<template>

  <v-container>
    <v-row justify="center">
      <h1>Главная страница</h1>
    </v-row>

    <v-row justify="center">
      <v-col cols="12" sm="8" md="6">
        
        <span v-if="userStore.userName" class="d-flex justify-center">{{userStore.userName}}</span>
        <span v-else class="d-flex justify-center">НЕТ ИМЕНИ, ПОСТАВЬ ИМЯ!</span>
        <span v-if="userStore.userLoggedIn" class="d-flex justify-center">Ты готов к игре</span>

        <v-text-field v-model="newUserName" class="mt-2" label="Твоё имя"></v-text-field>

        <v-btn v-if="userStore.userLoggedIn" @click="userLogout">Выйти</v-btn>
        <v-btn @click="userLogin">
          <span v-if="userStore.userLoggedIn">Сменить имя</span>
          <span v-else>Войти</span>
        </v-btn>
      </v-col>
    </v-row>
    

    

    <!-- <v-input @change="updateName" v-model="userName"></v-input> -->
  </v-container>

  <v-container >
    <v-btn v-if="userStore.userLoggedIn" @click="fastPlay">Быстрая игра</v-btn>
    <v-btn v-if="userStore.userLoggedIn" @click="createRoom">Создать комнату</v-btn>
  </v-container>

  <v-container>
    Комнаты:
    <RoomCardList/>
    
  </v-container>
</template>

<script>
import TheWelcome from '../components/TheWelcome.vue'
import RoomCardList from '@/components/RoomCardList.vue'

import { useRoomStore } from '@/stores/room.js'
import { useUserStore } from '@/stores/user.js'

export default {
  setup() {
    const roomStore = useRoomStore()
    const userStore = useUserStore()

    return {
      roomStore,
      userStore
    }
  },
  name: 'HomeView',
  components: {
    TheWelcome, 
    RoomCardList,
  },
  data: () => ({
    rooms: [],
    newUserName: '',
  }),
  methods: {
    async userLogin() {
      await this.userStore.loginUser({ name: this.newUserName })
    },

    async userLogout() {
      await this.userStore.logoutUser()
    },

    async createRoom () {

      this.$router.push(`/createRoom`)
      // if (!this.userName) {
      //   alert('NO NAME NO GAME')
      //   return
      // }

      // const body = {
      //   "name" : "123",
      //   "description" : "1",
      //   "ownerId" : "1",
      //   "rangeOpenings" : [10]
      // }
      // const room = await this.$axios.post('/api/rooms', body);

      // const rooms = await this.$axios.get('/api/rooms/getAll');
      // this.rooms = rooms.data;
    },

    async joinRoom (room) {
      if (!this.userStore.userName) {
        alert('NO NAME NO GAME')
        return
      }

      this.$router.push('/room/' + room.id)
    },
    
  },

  async mounted () {
    await this.roomStore.getRooms()
  },

}

</script>

<style>
.container-a{
  display: flex;
}
</style>
