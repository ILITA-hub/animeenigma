<template>
  <div>
    <v-app-bar class="app-bar">
      <v-img src="src/assets/img/logo.png" class="logo"></v-img>
      <v-btn icon class="mr-2" v-if="isBurgerMenu" @click.stop="drawer = !drawer">
        <v-icon>mdi-menu</v-icon>
      </v-btn>
      <div v-if="!isBurgerMenu" class="content">
        <v-btn text class="button btn">Главная</v-btn>
        <v-btn text class="button btn">Коллекции</v-btn>
        <v-btn text class="button btn">Комнаты</v-btn>
        <v-spacer></v-spacer>
        <v-text-field class="search" density="compact" label="Поиск..." variant="" single-line>
          <template v-slot:append>
            <v-btn icon @click="onSearchIconClick" class="search-icon-button">
              <v-icon>mdi-magnify</v-icon>
            </v-btn>
          </template>
        </v-text-field>
        <v-spacer></v-spacer>
        <v-btn text class="button button-room">Комната +</v-btn>
        <v-btn text class="button button-main">Войти</v-btn>
      </div>
      <v-navigation-drawer v-model="drawer" location="right" temporary>
        <v-list dense>
          <v-list-item @click="drawer = false">
            <v-list-item-title>Главная</v-list-item-title>
          </v-list-item>
          <v-list-item @click="drawer = false">
            <v-list-item-title>Коллекции</v-list-item-title>
          </v-list-item>
          <v-list-item @click="drawer = false">
            <v-list-item-title>Комнаты</v-list-item-title>
          </v-list-item>
          <v-list-item @click="drawer = false">
            <v-list-item-title>Комната +</v-list-item-title>
          </v-list-item>
          <v-list-item @click="drawer = false">
            <v-list-item-title>Войти</v-list-item-title>
          </v-list-item>
        </v-list>
      </v-navigation-drawer>
    </v-app-bar>
  </div>
</template>

<script>
export default {
  data() {
    return {
      isBurgerMenu: false,
      drawer: false,
    };
  },
  mounted() {
    this.handleResize();
    window.addEventListener('resize', this.handleResize);
  },
  beforeDestroy() {
    window.removeEventListener('resize', this.handleResize);
  },
  methods: {
    onSearchIconClick() {
    },
    handleResize() {
      this.isBurgerMenu = window.innerWidth <= 435;
    },
  },
};
</script>

<style scoped>
.app-bar {
  display: flex;
  background-color: #101115 !important;
  align-items: center;
  height: 84px;
  width: 100%;
  justify-content: space-between;
}

.content {
  display: flex;
  align-items: center;
  justify-content: space-evenly !important;
  width: 100%;
  margin: 0 35px;
}

.logo {
  top: 10px;
  height: 40px;
  width: 100px;
  flex: none;
  margin: 0 35px;
}

.button {
  height: 40px !important;
  border-radius: 10px;
  font-family: Montserrat;
  font-weight: normal;
  text-transform: none;
  color: white;
  top: 10px;
  align-items: center;
  justify-content: center;
  margin: 15px;
  flex: auto;
}

.button-room {
  width: 163px !important;
  background-color: rgba(255, 255, 255, 0.1);
}

.button-main {
  width: 163px !important;
  background-color: #1470EF;
  color: white;
}

.search {
  top: 10px;
  position: relative;
  font-family: Montserrat;
  margin: 0 0px 0 15px;
  width: 400px !important;
  height: 40px;
  border-radius: 10px;
  background-color: rgba(255, 255, 255, 0.1);
  color: white;
}

.search-icon-button {
  height: 40px !important;
  border-radius: 10px;
  background-color: #1470EF;
  color: white;
}
</style>
