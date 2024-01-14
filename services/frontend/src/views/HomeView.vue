
<template>
  <v-container>
    <v-btn>Geg</v-btn>
    <v-btn>Geg</v-btn>
    <v-btn>Geg</v-btn>
    <v-btn>Geg</v-btn>
  </v-container>

  <v-container>
    You are in Home Page
    Your name is
    <v-container v-if="!userName">
      YOU HAVE NO NAME, SET IT NOW!
    </v-container>
    <v-input v-model="userName"></v-input>
    <v-btn @click="updateName">Update my name</v-btn>
  </v-container>

  <v-container>
    <v-btn @click="createRoom">Create room</v-btn>
  </v-container>

  <v-container>
    Rooms:
    <v-container v-for="room in rooms" :key="room.id">
      {{ room }}
      <v-btn @click="joinRoom(room)">Join room </v-btn>
    </v-container>
  </v-container>
</template>

<script>
import TheWelcome from '../components/TheWelcome.vue'

export default {
  name: 'HomeView',
  components: {
    TheWelcome
  },
  data: () => ({
    rooms: [],
    userName: '',
  }),
  methods: {
    async createRoom () {

      if (!this.userName) {
        alert('NO NAME NO GAME')
        return
      }

      const body = {
        "name" : "123",
        "description" : "1",
        "ownerId" : 1,
        "rangeAnime" : []
      }
      const room = await this.$axios.post('/rooms', body);

      const rooms = await this.$axios.get('/rooms/getAll');
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
    const rooms = await this.$axios.get('/rooms/getAll');
    this.rooms = rooms.data;

  },

}

</script>
