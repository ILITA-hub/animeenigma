/*import { createStore } from 'vuex';
import axios from 'axios';

export default createStore({
  state: {
    user: null,
    selectedVideos: [],
  },
  mutations: {
    setUser(state, user) {
      state.user = user;
    },
    logout(state) {
      state.user = null;
      state.selectedVideos = [];
    },
    addToCollection(state, video) {
      state.selectedVideos.push(video);
    },
    removeFromCollection(state, videoId) {
      state.selectedVideos = state.selectedVideos.filter(video => video.id !== videoId);
    },
  },
  actions: {
    setUser({ commit }, user) {
      commit('setUser', user);
    },
    logout({ commit }) {
      commit('logout');
    },
    addToCollection({ commit }, video) {
      commit('addToCollection', video);
    },
    removeFromCollection({ commit }, videoId) {
      commit('removeFromCollection', videoId);
    },
    async createCollection() {
      const token = localStorage.getItem('authToken') || sessionStorage.getItem('authToken');

      if (!token) {
        console.error('Нет токена аутентификации');
        return;
      }

      const payload = {
        name: this.collectionName,
        description: this.collectionDescription,
        openings: this.selectedOpenings,
      };

      try {
        const response = await axios.post('https://animeenigma.ru/api/animeCollections', payload, {
          headers: {
            Authorization: `Bearer ${token}`
          }
        });

        console.log('Collection created:', response.data);
        this.$router.push('/user');
      } catch (error) {
        console.error('Error creating collection:', error.response.data);
      }
    },
  },
  getters: {
    isAuthenticated: state => !!state.user,
    user: state => state.user,
    selectedVideos: state => state.selectedVideos,
  },
});*/
