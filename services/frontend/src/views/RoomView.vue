
<template>
    <v-container>
      You are in room {{ $route.params.id }}
    </v-container>

    <v-container>
      Users in this room:
      <v-list>
        <v-list-item v-for="user in usersInRoom" :key="user.id">
          <v-list-item-content>
            <v-list-item-title>{{ user.name }}</v-list-item-title>
            <v-list-item-title>Пинг: {{ user.ping }}</v-list-item-title>
          </v-list-item-content>
        </v-list-item>
      </v-list>
    </v-container>

    <v-container>
      Anime to guess:
      <v-container>
        (add anime player here)
      </v-container>
    </v-container>

    <v-container>
      Guess the anime based on these answer options
      <v-container>
        <v-container v-for="answer in answerOptions">
          <v-btn @click="guess(answer)">{{ answer }}</v-btn>
        </v-container>
      </v-container>
    </v-container>
</template>

<script>

export default {
  name: 'RoomView',
  components: {
  },
  data: () => ({
    answerOptions: [],
    userName: '',
    usersInRoom: [],
    port: 10000,
  }),
  methods: {
    // async updateName () {
    //   this.$socket.emit('updateName', this.userName);
    // },
    // async guess (answer) {
    //   this.$socket.emit('guess', answer);
    // },
  },

  async mounted () {
    const response = await this.$axios.get('/rooms/' + this.$route.params.id);
    this.port = response.data.PORT
    
    await this.$socketRelaunch(this.port);

    if (!this.$socket.connected) {
      await this.$socketRelaunch(this.port);
    }

    console.log('joinRoom')

    this.$socket.on('usersInRoom', (usersInRoom) => {
      this.usersInRoom = usersInRoom;
    });
    this.$socket.on('roomUpdate', (roomUpdate) => {
      console.log({roomUpdate})
      this.usersInRoom = roomUpdate.users;
      this.answerOptions = roomUpdate.answerOptions;

      if (roomUpdate.ping) {
        this.$socket.emit('pong');
      }

    });
    this.$socket.on('roomNotFound', (roomUpdate) => {
      console.log('roomNotFound')
      this.$router.push('/');
    });

    this.$socket.on('plays', (plays) => {
      console.log(plays)
      this.playerAudio 
    });

    this.$socket.on('answer', (answer) => {
      console.log('answer')
    });

    this.$socket.on('answer2', (answer) => {
      console.log('answer')
    });
    

    this.$socket.$emit('joinRoom', { roomId: this.roomId });

  },

  computed: {
    roomId () {
      return this.$route.params.id;
    },
  },

}

</script>
