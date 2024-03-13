<template>
  <v-app>
    <HeaderApp/>
    <v-main>
      <MainPage/>
    </v-main>
    <FooterApp/>
  </v-app>
</template>

<script>
import HeaderApp from './components/HeaderApp.vue'
import FooterApp from './components/FooterApp.vue'
import AnimatedBackground from './components/AnimatedBackground.vue'
import MainPage from './pages/MainPage.vue';

export default {
  name: 'App',

  components: {
    HeaderApp,
    FooterApp,
    AnimatedBackground,
    MainPage,
  },

  data: () => ({
    //
  }),
}
</script>

<style>
  
</style>