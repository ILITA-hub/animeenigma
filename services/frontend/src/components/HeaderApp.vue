<template>
    <v-app>
      <v-app-bar class="app-bar">
        <v-img src="src/assets/img/logo.png" height="40" width="100" class="logo"></v-img>
        <v-btn text class="button">Главная</v-btn>
        <v-btn text class="button">Коллекции</v-btn>
        <v-btn text class="button">Комнаты</v-btn>
        <v-text-field class="search"
        append-inner-icon="mdi-magnify"
        density="compact"
        label="Поиск..."
        variant=""
        single-line
      ></v-text-field>
        <v-text-field  
        append-inner-icon="mdi-magnify" 
        density="compact"
        label="Поиск"
        ></v-text-field>
        <v-btn text class="button button-room">Комната + </v-btn>
        <v-btn text class="button button-main">Войти</v-btn>
      </v-app-bar>
    </v-app>
  </template>
  
  <script>
  export default {
  }
  </script>
  
  <style scoped>

@font-face {
  font-family: Montserrat;
  src: url('src/assets/fonts/Montserrat.ttf');
}

.app-bar {
  display: grid;
  justify-content: space-between;
  background-color: #101115 !important;
  align-items: center;
}

.logo {
  flex-direction: column;
}

.button {
  margin: 10px;
  border-radius: 10px;
  font-family: Montserrat;
  font-weight: normal;
  text-transform: none; 
  color: white;
}

.button-room {
  background-color: rgb(255, 255, 255, 0.10);
}

 .button-main {
  background-color: #1470EF;
  color: white;
}

.search {
  font-family: Montserrat;
  flex: none;
  margin: 10px;
  width: 350px;
  height: 36px;
  border-radius: 10px;
  background-color: rgb(255, 255, 255, 0.10);
  color: white;
}
</style>