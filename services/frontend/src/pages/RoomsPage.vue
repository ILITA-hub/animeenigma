<template>
  <div class="container">
    <div class="banner">
      <div class="picture"></div>
      <div class="text">
        <div class="title">Поиск комнаты AnimeEnigma</div>
        <div class="subtitle">Погрузитесь в атмосферу аниме викторин, где проверите свои знания о<br> персонажах, сюжетах и деталях жанра. Готовы к вызову?</div>
      </div>
      <div class="search-container">
        <v-text-field
          class="search"
          density="compact"
          label="Поиск..."
          variant="plain"
          single-line
          v-model="searchQuery"
          @input="onSearchInput">
        </v-text-field>
        <v-btn text class="button" @click="onSearchIconClick">Поиск</v-btn>
      </div>
    </div>
    <div class="content">
      <div class="filter">
        <FilterRoom /> 
        <FilterAnime /> 
      </div>
      <div class="rooms-display">
        <div v-if="searchQuery" class="search-result-text">Результат поиска</div>
        <v-card v-for="(room, i) in filteredRooms" :key="i" class="room-card">   
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
      rooms: rooms,
      searchQuery: '',
      searchPerformed: false,
    };
  },
  computed: {
    filteredRooms() {
      if (!this.searchQuery) {
        return this.rooms.slice(0, 6);
      }
      return this.rooms.filter(room =>
        room.name.toLowerCase().includes(this.searchQuery.toLowerCase())
      );
    },
  },
  methods: {
    onSearchIconClick() {
      this.searchPerformed = true;
    },
    onSearchInput() {
      this.searchPerformed = true;
    },
  },
};
</script>


<style scoped>
.content {
  display: flex;
  margin: 30px 35px 0px 35px;
}

.container {  
  display: flex; 
  flex-direction: column; 
}  

.collections {  
  display: flex;  
  flex-wrap: wrap;  
  justify-content: flex-end;  
  left: 35px;
  position: relative;
}  


.banner {
  display: grid;
  position: relative;
  overflow: hidden;
  height: 300px;
  margin: 30px 35px 0px 35px;
  border-radius: 10px;
}

.picture {
  background-image: linear-gradient(to right, rgba(0,0,0,1), rgba(0,0,0,0)), url('src/assets/img/picture.png');
  background-size: cover;
  background-position: center; 
  width: 100%;
  height: 100%;
  border-radius: 10px;
}

.search-container {
  left: 70px;
  bottom: 40px;
  position: absolute;
  width: 480px;
  height: 40px;
  display: flex;
  justify-content: center;
}

.search {
  flex-grow: 1;
  margin-right: 10px;
  background-color: rgba(255, 255, 255, 0.1);
  color: white;
  border-radius: 10px;
  font-family: Montserrat;
  padding-left: 15px;
}

.button {
  font-family: Montserrat;
  font-weight: normal;
  text-transform: none;
  font-size: 16px;
  height: 40px;
  width: 100px;
  border-radius: 10px;
  background-color: #1470EF;
  color: white;
}

.text {
  position: absolute;
  bottom: 110px;
  left: 70px;
  color: white;
  font-family: Montserrat;
  text-align: left;
}

.title {
  font-size: 45px;
  font-weight: 700;
  line-height: 54.86px;
}

.subtitle {
  font-size: 16px;
  font-weight: 500;
  line-height: 22px;
  margin-top: 10px;
}

.filter {
  flex-basis: 300px;
  margin-right: 20px;
}

.rooms-display {
  margin-top: 25px;
  display: flex;
  flex-wrap: wrap;
  gap: 50px;
  justify-content: center;
  flex-grow: 1;
}
</style>
