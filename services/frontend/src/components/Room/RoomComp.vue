<template>
  <div class="room-card">
    <img class="room-image" :src="room.image" :alt="`Изображение ${room.title}`">
    <div class="room-info">
      <div class="room-title">{{ room.title }}</div>
      <div v-for="(players, index) in room.players" :key="players" class="players">{{ players }}</div>
      <div class="additional-info">
        <div class="genres">
        <span class="genre" v-for="genre in room.genres" :key="genre">
          {{ genre }}
        </span>
      </div>
        <v-btn class="enjoy">Присоединиться</v-btn>
      </div>
    </div>
  </div>
</template>

<script>
export default {
  name: 'RoomComp',
  props: {
    room: Object
  },
  data() {
    return {
      showDetails: false,
    };
  },
}    
</script>



<style scoped>
.genres {
  display: none;
  flex-wrap: wrap;
}

.genre {
  display: inline-block;
  margin: 2px;
  background-color: white;
  color: black;
  border-radius: 10px;
  font-family: Montserrat;
  font-size: 12px;
  font-weight: 500;
  width: auto;
  height: 35px;
  text-align: center;
  padding: 10px;
}

.players {
  font-family: Montserrat;
  font-size: 22px;
  font-weight: 500;
  line-height: 26.82px;
  text-align: left;
}

.room-card {
  cursor: pointer;
  width: 320px;
  position: relative;
  height: 445px;
  border-radius: 10px;
  margin: 0 45px;
  overflow: hidden;
  transition: transform 0.3s ease;
}

.room-card:hover .room-info {
  bottom: 0%;
}

.room-info {
  position: absolute;
  bottom: -26%;
  left: 0;
  width: 100%;
  color: white;
  font-size: 16px;
  font-family: "Montserrat", sans-serif;
  font-weight: bold;
  padding: 10px 15px;
  backdrop-filter: blur(2px);
  transition: bottom 0.4s ease;
  background: linear-gradient(0deg, rgba(0, 0, 0, 0.7), rgba(0, 0, 0, 0.3));
  overflow: hidden;
}

.room-image {
  width: 100%;
  height: 100%;
  position: absolute;
  top: 0;
  left: 0;
}


.additional-info {
  padding: 6px;
  margin-top: 10px;
}

.additional-info div {
  margin: 5px 0;
}



.enjoy {
  width: 280px;
  height: 50px;
  padding: 15px 55px 15px 55px;
  border-radius: 10px;
  opacity: 0px;
  background: rgba(20, 112, 239, 1);
  font-family: Montserrat;
  font-size: 16px;
  font-weight: 600;
  line-height: 19.5px;
  text-align: left;
  font-family: Montserrat;
  text-transform: none;
}
</style>