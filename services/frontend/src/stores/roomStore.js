import { defineStore } from 'pinia';
import axios from 'axios';
const BASEURL = import.meta.env.VITE_BASEURL


export const useRoomStore = defineStore('room', {
  state: () => ({
    roomName: '',
    roomId: null,
    playerCounts: [2, 4, 6, 8, 10],
    selectedPlayerCount: '',
    playerPoints: null,
    rangeOpenings: [
      { type: 'all', id: 0 },
      { type: 'collection', id: 1 },
      { type: 'anime', id: 1 },
    ],
    players:[
    ],
    status: '',
    userAnswer: '',
    serverAnswer: '',
    currentVideo: '',
    variantsAnswer: [],
    rooms: [],
    defaultGenres: ['Сёнен', 'Фэнтези'],
    defaultImage: 'zoro.jpg', 
  }),
  actions: {
    async fetchRooms() {
      try {
        const response = await axios.post(`${BASEURL}rooms`, payload);
        console.log('Ответ от сервера:', response);
        const roomId = response.data;
        if (roomId) {
          const roomLink = `AnimeEnigma.ru/room/${roomId}`;
          console.log('Ссылка на созданную комнату:', roomLink);
        } else {
          console.error('ID комнаты не найден в ответе:', response.data);
        }
      } catch (error) {
        console.error('Ошибка при загрузке комнат:', error);
      }
    },
  },
});
