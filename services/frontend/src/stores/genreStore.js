import { defineStore } from 'pinia';
import axios from 'axios';

export const genreStore = defineStore('genre', {
  state: () => ({
    genres: [],
  }),
  actions: {
    async loadGenres() {
      try {
        const response = await axios.get('https://animeenigma.ru/api/genre');
        if (response.data && Array.isArray(response.data)) {
          const validGenres = response.data.map(genre => ({
            id: genre.id,
            nameRu: genre.nameRu || 'Неизвестный жанр'
          }));
          this.genres = validGenres;
        }
      } catch (error) {
        console.error('Failed to fetch genres:', error);
      }
    },
  },
});
