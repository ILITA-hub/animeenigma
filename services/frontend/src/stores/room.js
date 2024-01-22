import { ref, computed } from 'vue'
import { defineStore } from 'pinia'
import axios from 'axios'

export const useRoomStore = defineStore('room', {
  state: () => ({
    rooms: [],
    currentRoom: {}
  }),
  getters: {
    
  },
  actions: {
    async getRooms() {
      try {
        const response = await axios.get('api/rooms/getAll');
        this.rooms = response.data
        return response
      } catch (error) {
        console.log(error)
      }
      
    },
  },
})
