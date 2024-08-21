<template>
  <div class="room-card">
    <img class="room-image" :src="room.image" :alt="`Изображение ${room.title}`">
    <div ref="roomInfo" class="room-info">
      <div class="room-title">{{ room.title }}</div>
      <div v-for="(players, index) in room.players" :key="players" class="players">{{ players }}</div>
      <div class="additional-info">
        <div class="genres">
          <span class="genre" v-for="genre in room.genres" :key="genre">{{ genre }}</span>
      </div>
        <v-btn class="enjoy">Присоединиться</v-btn>
      </div>
    </div>
  </div>
</template>

<script>
import { ref } from 'vue';

export default {
  name: 'RoomComp',
  props: {
    room: Object
  },
  mounted(){
    const roomInfo = this.$refs.roomInfo
    roomInfo.style.bottom = `-${roomInfo.offsetHeight - 60}px`
  },
  setup(props) {
    const genresVisible = ref(false);

    const showGenres = () => {
      genresVisible.value = true;
    };

    const hideGenres = () => {
      genresVisible.value = false;
    };

    return {
      genresVisible,
      showGenres,
      hideGenres
    };
  },
};  
</script>

<style scoped>

.room-card {
  cursor: pointer;
  width: 320px;
  position: relative;
  height: 445px;
  border-radius: 5px;
  overflow: hidden;
  transition: transform 0.3s ease;
}

.room-image {
  width: 100%;
  height: 100%;
  position: absolute;
  top: 0;
  left: 0;
}

.room-info {
  position: absolute;
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

.room-card:hover .room-info {
  bottom: 0%!important;
}

.additional-info {
  padding: 6px;
  margin-top: 10px;
}

.additional-info div {
  margin: 5px 0 10px 0;
}

.genres {
  /* display: none; */
  flex-wrap: wrap;
  margin-bottom: 10px;
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

.room-card:hover .genres {
  display: flex;
}

.players {
  font-family: Montserrat;
  font-size: 22px;
  font-weight: 500;
  line-height: 26.82px;
  text-align: left;
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