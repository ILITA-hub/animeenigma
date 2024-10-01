import { defineStore } from 'pinia';
import axios from 'axios';
const BASEURL = import.meta.env.VITE_BASEURL


export const useAnimeStore = defineStore('anime', {
  state: () => ({
    currentPage: 1,
    totalPages: 1,
    prevPageNumber: null,
    nextPageNumber: null,
    anime: [],
    genres: [],
    years: [],
    selectedGenres: [],
    selectedYears: [],
  }),
  actions: {
    async animeRequest(page = 1) {
      try {
        const query = this.buildQuery(page);
        const response = await axios.get(`${BASEURL}anime${query}`);
        const animeData = response.data.data;
        this.currentPage = response.data.page;
        this.totalPages = response.data.allPage;
        this.prevPageNumber = response.data.prevPage;
        this.nextPageNumber = response.data.nextPage;
        this.anime.push(...animeData);
      } catch (error) {
        console.error("Ошибка при загрузке данных:", error);
      }
    },
    async loadGenres() {
      try {
        const response = await axios.get(`${BASEURL}filters/genres`);
        this.genres = response.data;
      } catch (error) {
        console.error('ошибка загрузки жанров:', error);
      }
    },
    async loadYears() {
      try {
        const response = await axios.get(`${BASEURL}filters/years`);
        this.years = response.data;
      } catch (error) {
        console.error('ошибка загрузки годов:', error);
      }
    },
    buildQuery(page) {
      const params = new URLSearchParams();
      params.append('limit', 20);
      params.append('page', page);

      if (this.selectedGenres.length > 0) {
        this.selectedGenres.forEach(genre => params.append('genres', genre));
      }

      if (this.selectedYears.length > 0) {
        this.selectedYears.forEach(year => params.append('year', year));
      }
      return `?${params.toString()}`;
    },
    async loadGenres() {
      try {
        const response = await axios.get(`${BASEURL}filters/genres`);
        this.genres = response.data;
      } catch (error) {
        console.error('Error loading genres:', error);
      }
    },
    async loadYears() {
      try {
        const response = await axios.get(`${BASEURL}years`);
        this.years = response.data;
      } catch (error) {
        console.error('Error loading years:', error);
      }
    },
    setGenres(genres) {
      this.selectedGenres = genres;
      this.anime = []
      this.animeRequest(this.currentPage);
    },
    setYears(years) {
      this.selectedYears = years;
      this.anime = []
      this.animeRequest(this.currentPage);
    },
  },
});
