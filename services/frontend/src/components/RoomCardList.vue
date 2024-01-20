<template>
  <v-container>
    <v-row>
      <v-col
        cols="12"
        sm="6"
        md="4"
        v-for="room in roomStore.rooms"
        :key="room.id"
      >

        <v-card>
          <v-card-title>{{ room.title }}</v-card-title>
          <v-card-text>{{ room.text }}</v-card-text>
          <v-card-actions>
            <v-btn @click="joinRoom(room)" text color="primary">Присоединиться</v-btn>
          </v-card-actions>
        </v-card>
      </v-col>
    </v-row>
  </v-container>
</template>

<script>
import {useRoomStore} from '@/stores/room.js'

export default {
  setup() {
    const roomStore = useRoomStore()

    return {
      roomStore
    }
  },
  data() {
    return {
    };
  },
  mounted() {
    // console.log(this.roomStore)
  },
  methods: {
    joinRoom(room) {
      return
    }
  }
};
</script>