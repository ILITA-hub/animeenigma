import { defineStore } from 'pinia';
import axios from 'axios';

export const useRoomStore = defineStore('roomStore', {
  state: () => ({
    roomName: '',
    roomId: null,
    playerCounts: [2, 4, 6, 8, 10],
    selectedPlayerCount: '',
    rangeOpenings: [
      { type: 'all', id: 0 },
      { type: 'collection', id: 1 },
      { type: 'anime', id: 1 },
    ],
    players:[
      {
        name: 'player228',
        imgSrc: '/104652318_p0.png',
        points: 500
      },
      {
        name: 'player228',
        imgSrc: '/104652318_p0.png',
        points: 500
      },
      {
        name: 'player228',
        imgSrc: '/104652318_p0.png',
        points: 500
      },
      {
        name: 'player228',
        imgSrc: '/104652318_p0.png',
        points: 500
      },
      {
        name: 'player228',
        imgSrc: '/104652318_p0.png',
        points: 500
      },
    ],
    status: '',
    userAnswer: '',
    serverAnswer: '',
    currentVideo: '',
    variantsAnswer: []
  }),
  actions: {
    async createRoom() {
      const payload = {
        name: this.roomName,
        rangeOpenings: this.rangeOpenings,
        qtiUsersMax: +this.selectedPlayerCount,
      };

      try {
        const response = await axios.post('https://animeenigma.ru/api/rooms', payload);
        console.log('Ответ от сервера:', response);
        const roomId = response.data;
        if (roomId) {
          const roomLink = `AnimeEnigma.ru/room/${roomId}`;
          console.log('Ссылка на созданную комнату:', roomLink);
        } else {
          console.error('ID комнаты не найден в ответе:', response.data);
        }
      } catch (error) {
        console.error('Ошибка при создании комнаты:', error);
      }
    }
  }
});
