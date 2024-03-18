<template>
  <div>
    <v-app-bar class="app-bar">
      <v-img src="src/assets/img/logo.png" class="logo"></v-img>
      <v-btn icon class="mr-2" v-if="isBurgerMenu" @click.stop="drawer = !drawer">
        <v-icon>mdi-menu</v-icon>
      </v-btn>
      <div v-if="!isBurgerMenu" class="content">
        <v-btn text class="button btn" @click="$router.push('/main')">Главная</v-btn>
        <v-btn text class="button btn" @click="$router.push('/')">Коллекции</v-btn>
        <v-btn text class="button btn" @click="$router.push('/')">Комнаты</v-btn>
        <v-spacer></v-spacer>
        <v-text-field class="search" density="compact" label="Поиск..." variant="" single-line>
          <template v-slot:append>
            <v-btn icon @click="onSearchIconClick" class="search-icon-button">
              <v-icon>mdi-magnify</v-icon>
            </v-btn>
          </template>
        </v-text-field>
        <v-spacer></v-spacer>
        <v-btn text class="button button-room" @click="$router.push('/')">Комната +</v-btn>
        <v-btn text class="button button-main" @click="$router.push('/auth')">Войти</v-btn>
      </div>
      <v-navigation-drawer v-model="drawer" class="drawer" location="right" temporary fixed>
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
      this.isBurgerMenu = window.innerWidth <= 430;
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
  max-width: 1560px;
  margin-left: auto;
  margin-right: 70px;
  width: 100%;
}

.logo {
  top: 10px;
  height: 40px;
  width: 100px;
  flex: none;
  margin-left: 70px;
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
  width: 156px;
  background-color: rgba(255, 255, 255, 0.1);
  display: flex;
}

.button-main {
  width: 156px;
  background-color: #1470EF;
  color: white;
  display: flex;
}

.search {
  top: 10px;
  position: relative;
  font-family: Montserrat;
  margin: 0 0px 0 15px;
  width: 360px !important;
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

.mr-2 {
  color: #1470EF;
  top: 4px;
  margin-left: 200px;
}

.drawer {
  width: 430px;
}

@media (max-width: 430px) {
  .logo {
      margin-left: 20px;
      top: 4px;
    }
  .app-bar {
    height: 74px;
  }
  }
</style>
