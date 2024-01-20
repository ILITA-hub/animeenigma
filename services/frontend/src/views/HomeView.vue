
<template>

  <v-container>
    <v-row justify="center">
      <h1>You are in Home Page</h1>
    </v-row>

    <v-row justify="center">
      <v-col cols="12" sm="8" md="6">
        
        <span v-if="userName" class="d-flex justify-center">Your name is {{userName}}</span>
        <span v-if="!userName" class="d-flex justify-center">YOU HAVE NO NAME, SET IT NOW!</span>
        <v-text-field class="mt-2" label="Ваше имя"></v-text-field>
      </v-col>
    </v-row>
    

    

    <!-- <v-input @change="updateName" v-model="userName"></v-input> -->
  </v-container>

  <v-container>
    <v-btn @click="createRoom">Create room</v-btn>
  </v-container>

  <v-container>
    Rooms:
    <RoomCardList/>
    
  </v-container>
</template>

<script>
import TheWelcome from '../components/TheWelcome.vue'
import RoomCardList from '@/components/RoomCardList.vue'

import {useRoomStore} from '@/stores/room.js'

export default {
  setup() {
    const roomStore = useRoomStore()

    return {
      roomStore
    }
  },
  name: 'HomeView',
  components: {
    TheWelcome, 
    RoomCardList
  },
  data: () => ({
    rooms: [],
    userName: '',
  }),
  methods: {
    async createRoom () {

      // if (!this.userName) {
      //   alert('NO NAME NO GAME')
      //   return
      // }

      const body = {
        "name" : "123",
        "description" : "1",
        "ownerId" : "1",
        "rangeOpenings" : [10]
      }
      const room = await this.$axios.post('http://46.181.201.172/api/rooms', body);

      const rooms = await this.$axios.get('http://46.181.201.172/api/rooms/getAll');
      this.rooms = rooms.data;

    },

    joinRoom (room) {

      if (!this.userName) {
        alert('NO NAME NO GAME')
        return
      }

      this.$router.push('/room/' + room.id)
    },

    async updateName () {
      const user = await this.$axios.post('/users', {
        name: this.userName
      });
    }
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
