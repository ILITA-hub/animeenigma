import { createApp } from 'vue';
import App from './App.vue';
import vuetify from './plugins/vuetify';
import { loadFonts } from './plugins/webfontloader';
import router from './router';
import { createPinia } from 'pinia'; 

loadFonts();

const app = createApp(App);
const pinia = createPinia(); 

app
  .use(router)
  .use(vuetify)
  .use(pinia)
  .mount('#app');
