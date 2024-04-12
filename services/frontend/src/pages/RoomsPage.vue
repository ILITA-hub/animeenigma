<template>
  <div class="main-container">
  <div class="banner">
    <img class="picture">
    <div class="text">
    <div class="title">Поиск комнаты AnimeEnigma</div>
    <div class="subtitle">Погрузитесь в атмосферу аниме викторин, где проверите свои знания о<br> персонажах, сюжетах и деталях жанра. Готовы к вызову?</div>
  </div>
    <v-text-field class="search" density="compact" label="Поиск..." variant="" single-line>
          <template v-slot:append>
            <v-btn text class="button" @click="onSearchIconClick">Поиск</v-btn>
          </template>
        </v-text-field>
  </div>
  <div class="content">
      <div class="filter">
        <FilterRoom/> 
        <FilterAnime/> 
      </div>
      <div class="rooms-display">
        <v-card v-for="(room, i) in rooms" :key="i">   
          <RoomComp :room="room"/> 
        </v-card>  
      </div>
    </div>
</div>
  </template>


  <script>
import FilterRoom from "@/components/FilterComp/FilterRoom.vue";
import FilterAnime from "@/components/FilterComp/FilterAnime.vue";
import RoomComp from "@/components/Room/RoomComp.vue";
import { rooms } from "@/components/Room/RoomComp.js";


export default {
  components: {
    FilterRoom,
    FilterAnime,
    RoomComp,
  },
  data() { 
    return { 
      rooms: rooms.slice(0,6),
      model: 0, 
    }; 
  }, 
  
};
</script>


<style scoped>
.main-container {
  display: flex;
  flex-direction: column;
}

.content {
  display: flex;
  margin: 30px;
}

.filter {
  flex-basis: 400px;
  margin-right: 20px;
}

.rooms-display {
  display: flex;  
  flex-wrap: wrap;  
  justify-content: space-evenly;  
  position: relative;
  gap: 40px;
}



.banner {
  display: grid;
  position: relative;
  overflow: hidden;
  height: 300px;
  margin: 30px 30px 20px 30px;
}

.picture {
  background-image: linear-gradient(to right, rgba(0,0,0,1), rgba(0,0,0,0)), url('src/assets/img/picture.png');
  background-size: cover;
  background-position: center; 
  display: grid;
  border-radius: 10px;
  position: relative;
  top: 50%;
  left: 50%;
  transform:translate(-50%,-50%);
  width: 1697px;
  height: 300px;
}

.search {
  left: 70px;
  bottom: 270px;
  position: relative;
  font-family: Montserrat;
  margin: 0 0px 0 15px;
  width: 480px;
  height: 40px;
  border-radius: 10px;
  background-color: rgba(255, 255, 255, 0.1);
  color: white;
}

.button {
  font-family: Montserrat;
  font-weight: normal;
  text-transform: none;
  font-size: 16px;
  left: 180px;
  height: 40px;
  width: 164px;
  border-radius: 10px;
  background-color: #1470EF;
  color: white;
}

.text {
  position: relative;
  bottom: 300px;
  color: white;
  font-family: Montserrat;
  text-align: left;
  margin: 80px 0px 10px 85px;
}
.title {
font-size: 45px;
font-weight: 700;
line-height: 54.86px;
text-align: left;

}

.subtitle {
font-size: 16px;
font-weight: 500;
line-height: 22px;
text-align: left;
bottom: -7px;
position: relative;

}

.room {
  position: relative;
  display: flex;
}
</style>
  