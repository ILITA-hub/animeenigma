// import { ref, computed } from 'vue'
// import { defineStore } from 'pinia'
// import axios from 'axios'

// export const useUserStore = defineStore('user', {
//   state: () => ({
//     userId: '',
//     userName: '',
//     userLoggedIn: '',
//   }),
//   getters: {
//     getUserLoggedIn(state) {
//       return state.userLoggedIn
//     }
//   },
//   actions: {
//     async loginUser(credentials = { name }) {
//       console.log(credentials)
//       const result = await axios.post('api/users/login',
//         credentials
//       );

//       if (!result) {
//         return false;
//       }

//       this.userLoggedIn = true;
//       window.localStorage.setItem('userSessionId', result.data.sessionId);
//       window.localStorage.setItem('userData', JSON.stringify(result.data.userData));
//       this.loadUserData()

//     },

//     checkUserLoggedIn () {
//       this.loadUserData()
//       this.userLoggedIn = this.getSessionId() ? true : false
//       return this.userLoggedIn
//     },

//     getSessionId() {
//       const userSessionId = window.localStorage.getItem('userSessionId');
//       console.log('get token', userSessionId)
//       return userSessionId;
//     },

//     loadUserData() {
//       const userDataRaw = window.localStorage.getItem('userData');
//       if (userDataRaw && userDataRaw !== 'undefined') {
//         const userData = JSON.parse(userDataRaw);
//         this.userName = userData.name;
//       }
//     },

//     logoutUser() {
//       window.localStorage.removeItem('userSessionId');
//       window.localStorage.removeItem('userData');
//       this.userLoggedIn = false;
//       this.userName = '';
//     }

//   },
// })
