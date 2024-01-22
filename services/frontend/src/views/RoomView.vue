
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

    if (!this.$socket.connected) {
      await this.$socket.connect()
    }

    console.log('joinRoom')

    this.$socket.on('usersInRoom', (usersInRoom) => {
      this.usersInRoom = usersInRoom;
    });
    this.$socket.on('answerOptions', (answerOptions) => {
      this.answerOptions = answerOptions;
    });
    this.$socket.on('roomUpdate', (roomUpdate) => {
      console.log({roomUpdate})
      this.usersInRoom = roomUpdate.users;
      this.answerOptions = answerOptions;
    });
    this.$socket.on('roomNotFound', (roomUpdate) => {
      console.log('roomNotFound')
      this.$router.push('/');
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
