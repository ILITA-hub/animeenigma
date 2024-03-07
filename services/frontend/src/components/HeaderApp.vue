<template>
    <div>
      <v-app-bar class="app-bar">
        <div class="content">
          <v-img src="src/assets/img/logo.png" class="logo"></v-img>
          <v-btn text class="button btn">Главная</v-btn>
          <v-btn text class="button btn">Коллекции</v-btn>
          <v-btn text class="button btn">Комнаты</v-btn>
          <v-text-field class="search"
            density="compact"
            label="Поиск..."
            variant=""
            single-line>
            <template v-slot:append>
              <v-btn icon @click="onSearchIconClick" class="search-icon-button">
                <v-icon>mdi-magnify</v-icon></v-btn>
            </template></v-text-field>
          <v-btn text class="button button-room">Комната + </v-btn>
          <v-btn text class="button button-main">Войти</v-btn>
        </div>
      </v-app-bar>
    </div>
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
    display: flex;  
    background-color: #101115 !important;   
    align-items: center !important;   
    height: 84px !important;   
    justify-content: space-between; 
  }   

  .content { 
    margin-top: 20px; 
    display: flex; 
    align-items: center; 
    justify-content: space-evenly !important;
    width: 100%;
    padding: 0 35px; 
  } 

  .logo {   
    height: 40px; 
    width: 100px; 
    flex: none; 
    margin-right: auto;
  }   

  .button {   
    height: 40px !important;   
    border-radius: 10px;   
    font-family: Montserrat;   
    font-weight: normal;   
    text-transform: none;    
    color: white;   
    align-items: center;  
    justify-content: center;
    margin: 15px;
    display: inline-flex !important;
  }   

  .btn {
    margin-right: 85px;
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
    position: relative;
    font-family: Montserrat;   
    margin: 10px !important;   
    width: 380px !important;   
    height: 40px;   
    border-radius: 10px;   
    background-color: rgba(255, 255, 255, 0.1);   
    color: white;   
    flex: none; 
  }   

  .search-icon-button {
    height: 40px !important;
    border-radius: 10px;
    background-color: #1470EF;
    color: white;
  }
  </style>