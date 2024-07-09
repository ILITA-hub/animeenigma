import { defineStore } from 'pinia';

export const userStore = defineStore({
  id: 'user',
  state: () => ({
    user: null,
  }),
  actions: {
    setUser(user) {
      this.user = user;
    },
    logout() {
      this.user = null;
    },
  },
  getters: {
    isAuthenticated: (state) => !!state.user,
    getUser: (state) => state.user,
  },
});
