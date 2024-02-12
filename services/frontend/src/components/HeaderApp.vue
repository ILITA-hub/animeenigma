<template>
  <!-- <v-navigation-drawer v-model="drawer">
    <v-list-item title="Anime Enigma" subtitle="menu"></v-list-item>
    <v-divider></v-divider>

    <v-list-item class="mt-2">
      <v-text-field @keyup.enter="" label="Название лобби" />
    </v-list-item>
    
    <v-list-item class="mt-2">
      <v-text-field @keyup.enter="" label="Код" />
    </v-list-item>

    <v-list-item link title="Создать коллекцию"></v-list-item>
    <v-list-item link title="Создать лобби"></v-list-item>
    
    <v-icon ref="closeArrow" @click="openSideBar" class="arrowClose">mdi-arrow-left</v-icon>
  </v-navigation-drawer>
  <v-icon ref="openArrow" @click="openSideBar" class="arrowOpen">mdi-arrow-right</v-icon> -->

  <SideBar />
  
  <!-- <v-app-bar class="header_app" :elevation="2">
  </v-app-bar> -->
</template>

<script>
import SideBar from "@/components/sideBar/SideBar.vue"

export default {
  components: {
    SideBar
  },
  data() {
    return {
      drawer: false
    }
  },
  methods: {
    openSideBar() {
      this.drawer = !this.drawer
      const arrow = this.$refs.openArrow.$el
      arrow.classList.toggle("arrowRoteted");
    }
  },
}
</script>


<style scoped>
.header_container{
  width: 100%;
  height: 100%;
}
.header_app{
  background-color: rgba(169, 38, 184, 0.15)!important;
  display: none!important;
}
.arrowOpen{
  position: absolute!important;
  top: 50%;
  color: white;
  cursor: pointer;
}
.arrowClose{
  position: absolute!important;
  top: 50%;
  left: 260px;
  font-size: 30px;
  color: white;
  cursor: pointer;
}
</style>
