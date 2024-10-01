import { defineStore } from 'pinia';
import axios from 'axios';

export const useRoomStore = defineStore('roomStore', {
  state: () => ({
    roomName: '',
    
  }),
  actions: {
    async createRoom() {
      
    }
  }
});
