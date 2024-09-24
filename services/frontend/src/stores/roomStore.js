import { defineStore } from 'pinia';
import axios from 'axios';

export const useRoomStore = defineStore('room', {
  state: () => ({
    rooms: [],
    defaultGenres: ['Сёнен', 'Фэнтези'],
    defaultImage: 'zoro.jpg', 
  }),
  actions: {
    async fetchRooms() {
      try {
        const response = await axios.get('https://animeenigma.ru/api/rooms/getAll');
        this.rooms = response.data.map(room => ({
          ...room,
          genres: this.defaultGenres,
          image: this.defaultImage,   
        }));
      } catch (error) {
        console.error('Ошибка при загрузке комнат:', error);
      }
    },
  },
});
