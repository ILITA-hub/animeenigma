<template>
  <v-app>
    <v-main>
      <HeaderApp/>
    </v-main>
  </v-app>
</template>

<script>
import HeaderApp from './components/HeaderApp.vue'

export default {
  name: 'App',

  components: {
    HeaderApp,
  },

  data: () => ({
    //
  }),
}
</script>

<style>
  
</style>