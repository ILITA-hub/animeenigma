
<template>
  <v-container>
    <v-btn>Geg</v-btn>
    <v-btn>Geg</v-btn>
    <v-btn>Geg</v-btn>
    <v-btn>Geg</v-btn>
  </v-container>
  <v-container>
    <v-btn @click="createRoom">Create room</v-btn>
  </v-container>
  <v-container>
    Rooms:
    <v-container v-for="room in rooms" :key="room.id">
      {{ room }}
      <v-btn @click="joinRoom">Join room </v-btn>
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
    rooms: []
    //
  }),
  methods: {
    async createRoom () {
      const body = {
        "name" : "123",
        "description" : "1",
        "ownerId" : 1,
        "rangeAnime" : []
      }
      const room = await this.$axios.post('/rooms', body);

      const rooms = await this.$axios.get('/rooms/getAll');
      this.rooms = rooms.data;

      this.$socket.emit('createRoom', room.data.id);
    },
    joinRoom () {
      // this.$router.push({ name: 'RoomView' })
    }
  },

  async mounted () {
    const rooms = await this.$axios.get('/rooms/getAll');
    console.log()
    this.rooms = rooms.data;

    this.$socket.connect()

    this.$socket.on('hi', (data) => {
      console.log('hi', data)
    })

  },

}

</script>
