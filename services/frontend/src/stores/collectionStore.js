import { defineStore } from 'pinia';
import axios from 'axios';
import Cookies from 'js-cookie';

export const useCollectionStore = defineStore('collection', {
  state: () => ({
    selectedOpenings: [],
    collectionName: '',
    collectionDescription: '',
    collections: [],
  }),
  actions: {
    addToCollection(video) {
      if (video && video.id) {
        if (!this.selectedOpenings.some(v => v.id === video.id)) {
          this.selectedOpenings.push(video);
          localStorage.setItem('selectedOpenings', JSON.stringify(this.selectedOpenings));
        }
      } else {
        console.error('неверный формат объекта:', video);
      }
    },
    removeFromCollection(videoId) {
      this.selectedOpenings = this.selectedOpenings.filter(video => video.id !== videoId);
      localStorage.setItem('selectedOpenings', JSON.stringify(this.selectedOpenings));
    },
    loadFromLocalStorage() {
      try {
        const storedOpenings = JSON.parse(localStorage.getItem('selectedOpenings')) || [];
        const storedName = localStorage.getItem('collectionName') || '';
        const storedDescription = localStorage.getItem('collectionDescription') || '';

        this.selectedOpenings = storedOpenings.filter(video => video !== null);
        this.collectionName = storedName;
        this.collectionDescription = storedDescription;
      } catch (error) {
        console.error('ошибка загрузки из локального хранилища:', error);
      }
    },
    saveToLocalStorage() {
      localStorage.setItem('collectionName', this.collectionName);
      localStorage.setItem('collectionDescription', this.collectionDescription);
      localStorage.setItem('selectedOpenings', JSON.stringify(this.selectedOpenings));
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
        openings: this.selectedOpenings.map(video => video.id),
        //openings: this.selectedOpenings.length === 0 ? [0] : this.selectedOpenings, //TODO убрать проверку
      };

      try {
        const response = await axios.post('https://animeenigma.ru/api/animeCollections', payload, {
          headers: {
            Authorization: `Bearer ${token}`,
          },
        });
        console.log('коллекция создана:', response.data);
        return response.data;
      } catch (error) {
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
        const response = await axios.get('https://animeenigma.ru/api/animeCollections?limit=50&page=1', {
          headers: {
            Authorization: `Bearer ${token}`
          }
        });
        this.collections = response.data;
      } catch (error) {
        console.error('ошибка получения коллекции:', error.response?.data);
      }
    },
    async siteCollections() {
      try {
        const response = await axios.get('https://animeenigma.ru/api/animeCollections?limit=50&page=1');
        this.collections = response.data.data.map(collection => {
          return {
            ...collection,
            image: collection.image || 'zoro.jpg',
            genres: collection.genres || [],
          };
        });
      } catch (error) {
        console.error('Ошибка при загрузке коллекций:', error);
      }
    },
    clearCollectionData() {
      this.collectionName = '';
      this.collectionDescription = '';
      this.selectedOpenings = [];
      this.saveToLocalStorage();
    }
  },
});
