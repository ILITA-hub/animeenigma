import { defineStore } from 'pinia';
import axios from 'axios';
import Cookies from 'js-cookie';

export const useCollectionStore = defineStore('collection', {
  state: () => ({
    selectedVideos: [],
    collectionName: '',
    collectionDescription: '',
    selectedOpenings: [],
    collections: [],
  }),
  actions: {
    addToCollection(video) {
      this.selectedVideos.push(video);
    },
    removeFromCollection(videoId) {
      this.selectedVideos = this.selectedVideos.filter(video => video.id !== videoId);
    },
    async createCollection() {
      const token = Cookies.get('authToken');

      if (!token) {
        console.error('Нет токена аутентификации');
        return;
      }
      const payload = {
        name: this.collectionName,
        description: this.collectionDescription,
        openings: this.selectedOpenings.length === 0 ? [0] : this.selectedOpenings, //TODO убрать проверку
      };

      try {
        const response = await axios.post('https://animeenigma.ru/api/animeCollections', payload, {
          headers: {
            Authorization: `Bearer ${token}`,
          },
        });
        console.log('Collection created:', response.data);
        return response.data;
      } catch (error) {
        console.error('Error creating collection:', error);
        console.error('Error response:', error.response);
        console.error('Error data:', error.response?.data);

        throw error;
      }
    },
    async userCollections() {
      const token = Cookies.get('authToken');
      if (!token) {
        console.error('Нет токена аутентификации');
        return;
      }
      try {
        const response = await axios.get('https://animeenigma.ru/api/animeCollections', {
          headers: {
            Authorization: `Bearer ${token}`
          }
        });
        this.collections = response.data;
      } catch (error) {
        console.error('Error fetching collections:', error.response?.data);
      }
    },
  },
  getters: {
    selectedVideosList: (state) => state.selectedVideos,
  },
});
