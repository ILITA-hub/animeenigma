import { defineStore } from 'pinia';
import axios from 'axios';

export const useAuthStore = defineStore('auth', {
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
    async login(credentials) {
      try {
        const response = await axios.post('https://animeenigma.ru/api/users/login', {
          login: credentials.email,
          password: credentials.password
        });
        if (response && response.data) {
          const token = response.data.token;
          const user = { email: credentials.email, token };
          if (credentials.rememberMe) {
            localStorage.setItem('authToken', token);
          } else {
            sessionStorage.setItem('authToken', token);
          }
          this.setUser(user);
          return { success: true, user };
        } else {
          console.error('Login error: Invalid response', response);
          return { success: false, message: 'Неправильный ответ сервера при входе.' };
        }
      } catch (error) {
        console.error('Login error:', error);
        return { success: false, message: error.response?.data?.message || 'Неверный email или пароль.' };
      }
    },
    async register(credentials) {
      try {
        const response = await axios.post('https://animeenigma.ru/api/users/reg', {
          username: credentials.username,
          login: credentials.email,
          password: credentials.password,
          confirmPassword: credentials.confirmPassword
        });
        if (response && response.data) {
          const token = response.data.token;
          const user = { email: credentials.email, token };
          localStorage.setItem('authToken', token);
          this.setUser(user);
          return { success: true, user };
        } else {
          console.error('Registration error: Invalid response', response);
          return { success: false, message: 'Неправильный ответ сервера при регистрации.' };
        }
      } catch (error) {
        console.error('Registration error:', error);
        return { success: false, message: error.response?.data?.message || 'Ошибка регистрации.' };
      }
    }
  },
  getters: {
    isAuthenticated: (state) => !!state.user,
    loggedInUser: (state) => state.user,
  },  
});
