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
        <v-card class="room_card">
          <v-card-title>{{ room.name }}</v-card-title>
          <!-- <v-card-text>{{ room.text }}</v-card-text> -->
          <v-card-actions class="float-right room-card-actions">
            <div class="users-count-container">
              <v-icon aria-hidden="false" class="users-count-icon">
                mdi-account
              </v-icon>
              {{Object.keys(room.users).length}} / {{room.qtiUsersMax}}
            </div>
            <e-rbtn v-if="userStore.userLoggedIn" @click="joinRoom(room)">join</e-rbtn>
          </v-card-actions>
        </v-card>
      </v-col>
    </v-row>
  </v-container>
</template>

<script>
import RoundedButton from '@/components/buttons/RoundedButton.vue'

import {useRoomStore} from '@/stores/room.js'
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
  components: {
    RoundedButton
  },
  mounted() {
    // console.log(this.roomStore)
  },
  methods: {
    joinRoom(room) {
      
      // console.log(room.id)
      this.$router.push(`/room/${room.id}`)
    }
  }
};
</script>

<style scoped>
.room_card {
  background-color: rgba(179,42,201, 0.15);
  color: white;
  border-radius: 10px;
}
.room-card-actions{
  width: 100%;
  position: relative;
  justify-content: flex-end
}
.users-count-icon{
  
}
.users-count-container{
  position: absolute;
  left: 10px;
  align-items: center;
  display: flex;
}
</style>