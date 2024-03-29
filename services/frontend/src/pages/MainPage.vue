<template>
  <div>
    <div class="banner">
      <v-carousel hide-delimiters
        height="440"
        cycle
        v-model="model">
        <v-carousel-item v-for="(room, i) in rooms" :key="i"> 
          <v-sheet :color="room.color" height="100%"> 
            <div class="d-flex fill-height justify-center align-center"> 
              <div class="text-h2"> 
                {{ room.name }} 
              </div> 
            </div> 
          </v-sheet> 
        </v-carousel-item> 
        <template v-slot:prev="{ props }">
      <v-btn class="prev"
        @click="props.onClick"
      ><v-icon class="icon" :size="30">mdi-chevron-left</v-icon></v-btn>
    </template>
    <template v-slot:next="{ props }">
      <v-btn class="next"
        @click="props.onClick"
      ><v-icon class="icon" :size="30">mdi-chevron-right</v-icon></v-btn>
    </template>
      </v-carousel> 
    </div> 
    <div class="custom-indicators"> 
      <span 
        v-for="(room, i) in rooms" 
        :key="'indicator-' + i" 
        @click="setSlide(i)" 
        class="indicator" 
        :class="{ 'active': model === i }">
      </span> 
    </div>
    <div class="genres">
    <div class="genre-label">Популярные жанры</div> 
    <GenreCard
      v-for="genre in genres"
      :key="genre.id"
      :genre="genre"/>
  </div> 
  </div>
</template>

<script>
import GenreCard from "@/components/GenreCard/GenreCard.vue";
import { genres } from "@/components/GenreCard/GenreCard.js";

export default {
  name: 'MainPage',
  components: {
    GenreCard,
  },
  data () { 
    return { 
      genres,
    }; 
  },
};
</script>

<style scoped>
.banner {
  position: relative;
}
.v-carousel {
  max-width: 1695px;
  top: 40px;
  margin: 0 auto;
  border-radius: 10px;
}
.custom-indicators {
  display: flex;
  justify-content: center;
  margin-top: 64px;
}
.indicator {
  max-width: 1695px;
  border-radius: 50%;
  width: 15px;
  height: 15px;
  background-color: rgba(255, 255, 255, 0.5);
  cursor: pointer;
  margin-right: 13px;
}
.indicator:last-child {
  margin-right: 0;
}
.indicator.active {
  background-color: #FFF; 
}

.genres { 
  position: relative; 
  display: flex; 
  top: 100px; 
  padding-bottom: 100px; 
  justify-content: center;  
} 

.genre-label {
  position: absolute;
  top: -60px;
  left: 65px;
  font-size: 28px;
  color: #ffffff;
  font-family: Montserrat;
  font-weight: bold;
}

.v-btn.prev, .v-btn.next { 
  margin: 0;
  position: absolute;
  top: 50%;
  border-radius: 15px; 
  background-color: rgba(255, 255, 255, 0.2); 
  backdrop-filter: blur(2px); 
  transform: translateY(-50%);
}

.next {
 right: 10px;
 height: 50px; 
  width: 50px; 
}

.prev {
  left: 10px;
  height: 50px; 
  width: 50px; 
}

.icon {
  color: #FFF;
}
</style>