import { ref, computed } from 'vue'
import { defineStore } from 'pinia'
import axios from 'axios'

export const useRoomStore = defineStore('room', {
  state: () => ({
    rooms: [],

  }),
  getters: {
    
  },
  actions: {
    async getRooms() {
      const response = await axios.get('http://46.181.201.172/api/rooms/getAll');
      this.rooms = response.data
      console.log(response)
      return response
    },
  },
})
