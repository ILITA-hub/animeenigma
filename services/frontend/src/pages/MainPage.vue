<template>
  <div>
    <BannerComp/>
    <div class="labels">
      <div class="label">Популярные жанры</div>
      <div class="genres">
      <GenreCard v-for="genre in genres" :key="genre.id" :genre="genre"/>
    </div>
  </div>
    <div class="labels">
      <div class="label">Комнаты</div>
      <div class="watch-all" @click="$router.push('/rooms')">Смотреть все</div>
    </div>
      <div class="rooms">
          <RoomComp
           v-for="(room, i) in rooms"
           :key="i" 
           :room="room"/>
    </div>
  </div>
</template>

<script> 
import GenreCard from "@/components/GenreCard/GenreCard.vue"; 
import { genres } from "@/components/GenreCard/GenreCard.js"; 
import BannerComp from "@/components/Banner/BannerComp.vue";
import RoomComp from "@/components/Room/RoomComp.vue"; 
import { rooms } from "@/components/Room/RoomComp.js"; 

export default { 
  name: 'MainPage', 
  components: { 
    GenreCard, 
    BannerComp, 
    RoomComp, 
  }, 
  data () {  
    return {  
      genres, 
      rooms: rooms, 
      model: 0,  
    };  
  }, 
}; 
</script> 

<style scoped> 
.rooms {  
  margin-top: 90px; 
  display: flex;  
  flex-wrap: wrap;  
  position: relative; 
  gap: 90px; 
  left: 150px;
} 

.genres {  
  position: relative;  
  display: flex;  
  top: 100px;  
  padding-bottom: 100px;  
  justify-content: center;   
}  

.label {
  font-size: 28px;
  color: #ffffff;
  font-family: Montserrat;
  font-weight: bold;
  top: 70px;
  position: relative;
  left: 70px;
}

.watch-all {
  font-size: 18px;
  color: #ffffff;
  text-align: right;
  right: 70px;
  position: relative;
  top: 40px;
  font-family: Montserrat;
  font-size: 16px;
  font-weight: 400;
  line-height: 19.5px;
  text-align: right;
  cursor: pointer;
}
</style>
