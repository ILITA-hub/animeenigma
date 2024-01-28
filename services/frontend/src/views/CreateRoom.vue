
<template>

  <v-container>
    
    <v-col justify="center">
      <h1>Создание комнаты</h1>

      <v-text-field v-model="newRoomName" class="mt-2" label="Название комнаты"></v-text-field>
      <v-text-field v-model="newRoomDesc" class="mt-2" label="Описание"></v-text-field>
      <v-text-field v-model="newRoomOpRange" class="mt-2" label="Диапазон опенингов (для прошареных)"></v-text-field>

      <v-btn @click="createRoom">Создать</v-btn>

    </v-col>
    
  </v-container>
    
</template>

<script>

export default {
  name: 'CreateRoomView',
  components: {
  },
  data: () => ({
    newRoomName: '',
    newRoomDesc: '',
    newRoomOpRange: '',
  }),
  methods: {
    async createRoom () {
      
      const data = {
        name: this.newRoomName,
        desc: this.newRoomDesc,
        opRange: this.newRoomOpRange,
      }

      const response = await this.$axios.post('/rooms', data)
      this.$router.push({ name: 'RoomView', params: { id: response.data.id } })

    },
  },

}

</script>
