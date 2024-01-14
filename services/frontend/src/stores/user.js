import { ref, computed } from 'vue'
import { defineStore } from 'pinia'

export const useUser = defineStore('user', {
  state: () => ({
    userId: '',
    userName: '',
    token: '',
  }),
  setters: {
    token(token) {
      window.localStorage.setItem('userToken', token)
    }
  },
  getters: {
    token(state) {
      window.localStorage.getItem('userToken')
    },
  },
  actions: {
    
  },
})
