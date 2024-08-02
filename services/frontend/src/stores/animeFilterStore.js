import { defineStore } from 'pinia';
import axios from 'axios';

export const useAnimeFilterStore = defineStore('animeFilterStore', {
  state: () => ({
    genres: [],
    years: [],
  }),
  actions: {
    async loadGenres() {
      try {
        const response = await axios.get('https://animeenigma.ru/api/filters/genres');
        this.genres = response.data;
      } catch (error) {
        console.error('ошибка загрузки жанров:', error);
      }
    },
    async loadYears() {
      try {
        const response = await axios.get('https://animeenigma.ru/api/filters/years');
        this.years = response.data;
      } catch (error) {
        console.error('ошибка загрузки годов:', error);
      }
    },
  },
});
