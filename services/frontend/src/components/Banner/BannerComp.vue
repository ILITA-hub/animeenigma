<template> 
    <div class="banner"> 
      <v-carousel hide-delimiters
        cycle 
        v-model="model"> 
        <v-carousel-item v-for="(room, i) in rooms" :key="i">  
          <RoomCompBanner :room="room"/>
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
</template>

<script>
import RoomCompBanner from "@/components/Room/RoomCompBanner.vue"; 
import { rooms } from "@/components/Room/RoomComp.js";

export default { 
  components: { 
    RoomCompBanner 
  }, 
  data() { 
    return { 
      rooms: rooms.slice(0, 3),
      model: 0, 
    }; 
  }, 
} 
</script>



<style scoped>
.banner {
    position: relative;
  }
  .v-carousel {
    width: 1700px;
    top: 40px;
    margin: 0 auto;
    border-radius: 10px;
  }
  .custom-indicators {
    display: flex;
    justify-content: center;
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

  .v-btn.prev, .v-btn.next { 
  position: absolute;
  border-radius: 15px; 
  background-color: rgba(255, 255, 255, 0.2); 
  backdrop-filter: blur(2px); 
  transform: translateY(-50%);
  height: 50px;
  width: 50px;
}

.next {
 right: 10px;
}

.prev {
  left: 10px;
}

.icon {
  color: #FFF;
}

</style>
  