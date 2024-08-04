import { defineStore } from 'pinia';
import axios from 'axios';

export const useAnimeStore = defineStore('anime', {
  state: () => ({
    currentPage: 1,
    totalPages: 1,
    prevPageNumber: null,
    nextPageNumber: null,
    anime: [],
  }),
  actions: {
    async animeRequest(page = 1) {
        try {
          const response = await axios.get(`https://animeenigma.ru/api/anime?limit=20&page=${page}`);
          const animeData = response.data.data;
          this.currentPage = response.data.page;
          this.totalPages = response.data.allPage;
          this.prevPageNumber = response.data.prevPage;
          this.nextPageNumber = response.data.nextPage;
          this.anime = animeData;
        } catch (error) {
          console.error("Ошибка при загрузке данных:", error);
        }
      
    },
  },
});
