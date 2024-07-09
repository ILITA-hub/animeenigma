<template>
  <div>
    <v-app-bar class="app-bar">
      <v-img src="src/assets/img/logo.png" class="logo" @click="$router.push('/')"></v-img>
      <v-btn icon class="mr-2" v-if="isBurgerMenu" @click.stop="drawer = !drawer">
        <v-icon>mdi-menu</v-icon>
      </v-btn>
      <div v-if="!isBurgerMenu" class="content">
        <v-btn text class="button btn" @click="$router.push('/')">Главная</v-btn>
        <v-menu>
          <template v-slot:activator="{ props }">
            <v-btn text class="button btn" v-bind="props">
              Коллекции
              <v-icon class="icon" :size="20">{{ menu ? 'mdi-chevron-up' : 'mdi-chevron-down' }}</v-icon>
            </v-btn>
          </template>
          <v-list class="list">
            <v-list-item v-for="(item, index) in items" :key="index" @click="routeTo(item.route)">
              <v-list-item-title>{{ item.title }}</v-list-item-title>
            </v-list-item>
          </v-list>
        </v-menu>
        <v-btn text class="button btn" @click="$router.push('/rooms')">Комнаты</v-btn>
        <v-spacer></v-spacer>
        <v-text-field class="search" density="compact" label="Поиск..." variant="" single-line>
          <template v-slot:append>
            <v-btn icon @click="onSearchIconClick" class="search-icon-button">
              <v-icon>mdi-magnify</v-icon>
            </v-btn>
          </template>
        </v-text-field>
        <v-spacer></v-spacer>
        <v-btn text class="button button-room" @click="$router.push('/createroom')">Комната +</v-btn>
        <template v-if="isAuthenticated">
          <v-avatar class="avatar" :image="user.avatar" size="40" @click="$router.push('/user')"></v-avatar>
        </template>
        <template v-else>
          <v-btn text class="button button-main" @click="$router.push('/auth')">Войти</v-btn>
        </template>
      </div>
    </v-app-bar>
  </div>
</template>

<script>
import { computed } from 'vue';
import { userStore } from '@/stores/userStore';

export default {
  data() {
    return {
      menu: false,
      items: [
        { title: 'Коллекции на сайте', route: '/collections' },
        { title: 'Коллекция +', route: '/custom-collections' },
      ],
    };
  },
  setup() {
    const store = userStore();
    const isAuthenticated = computed(() => store.isAuthenticated);
    const user = computed(() => store.user);

    return {
      isAuthenticated,
      user,
    };
  },
  methods: {
    routeTo(route) {
      if (route) {
        this.$router.push(route);
      }
    },
    onSearchIconClick() {
    },
  },
};
</script>



<style scoped>
.avatar {
  cursor: pointer;
  top: 10px;
}

.icon {
  margin-top: 2px;
  margin-left: 10px;
}

.list {
  background: #101115 !important;
  border-radius: 10px !important;
  color: white;
  font-family: Montserrat;
  text-transform: none;
  font-weight: normal;
  display: flex;
  flex-direction: column;
}

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
  cursor: pointer;
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
</style>
