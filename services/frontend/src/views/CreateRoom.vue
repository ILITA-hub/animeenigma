
<template>
    
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

  // async mounted () {
  //   this.$socket.connect()

  //   this.$socket.on('hi', (data) => {
  //     console.log('hi', data)
  //   })
    
  //   this.$socket.emit('joinRoom', this.$route.params.id);
  //   this.$socket.on('usersInRoom', (usersInRoom) => {
  //     this.usersInRoom = usersInRoom;
  //   });
  //   this.$socket.on('answerOptions', (answerOptions) => {
  //     this.answerOptions = answerOptions;
  //   });

  // },

}

</script>
