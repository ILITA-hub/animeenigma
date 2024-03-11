<template>
  <v-app>
    <HeaderApp/>
    <v-main>
      <AnimatedBackground/>
    </v-main>
    <FooterApp/>
  </v-app>
</template>

<script>
import HeaderApp from './components/HeaderApp.vue'
import FooterApp from './components/FooterApp.vue'
import AnimatedBackground from './components/AnimatedBackground.vue'

export default {
  name: 'App',

  components: {
    HeaderApp,
    FooterApp,
    AnimatedBackground,
  },

  data: () => ({
    //
  }),
}
</script>

<style>
  
</style>