<template>
  <div class="container">
    <div class="banner">
      <div class="picture"></div>
      <div class="text">
        <div class="title">Поиск комнаты AnimeEnigma</div>
        <div class="subtitle">
          Погрузитесь в атмосферу аниме викторин, где проверите свои знания о<br>
          персонажах, сюжетах и деталях жанра. Готовы к вызову?
        </div>
      </div>
      <div class="search-container">
        <v-text-field
          class="search"
          density="compact"
          label="Поиск..."
          variant="plain"
          single-line
          v-model="searchQuery"
        ></v-text-field>
        <v-btn text class="button">Поиск</v-btn>
      </div>
    </div>
    <div class="content">
      <div class="sidebar">
        <div class="filter">
          <FilterRoom />
          <!-- <FilterAnime /> -->
        </div>
      </div>
      <div class="main-content">
        <div class="result-container" v-if="searchQuery">
          <div class="result">Результаты поиска</div>
        </div>
        <div class="no-room" v-if="filteredRooms.length === 0">Комната не найдена</div>
        
        <div class="rooms">
          <RoomComp
            v-for="(room, i) in filteredRooms"
            :key="i"
            :room="room"/>
        </div>
      </div>
    </div>
  </div>
</template>

<script>
import { useRoomStore } from '@/stores/roomStore';
import FilterRoom from "@/components/FilterComp/FilterRoom.vue";
import FilterAnime from "@/components/FilterComp/FilterAnime.vue";
import RoomComp from "@/components/Room/RoomComp.vue";
import { computed, onMounted, ref } from 'vue';

export default {
  props: {
  },
  components: {
    FilterRoom,
    FilterAnime,
    RoomComp,
  },
  setup() {
    const selectedGenres = ref([]);
    const selectedYears = ref([]);
    const searchQuery = ref("");
    const roomStore = useRoomStore();

    const setSelectedGenres = (newGenres) => {
        selectedGenres.value = newGenres;
      };

      const setSelectedYears = (newYears) => {
        selectedYears.value = newYears;
        };
    
        const filteredRooms = computed(() => {
        if (!searchQuery.value) {
          return roomStore.rooms;
        }
        return roomStore.rooms.filter(room =>
          room.name.toLowerCase().includes(searchQuery.value.toLowerCase())
        );
      });

    onMounted(() => {
      roomStore.fetchRooms();
    });

    return {
      selectedGenres,
        selectedYears,
        setSelectedGenres,
        setSelectedYears,
        filteredRooms,
        rooms: roomStore.rooms,
        searchQuery,
    };
  },
};
</script>

<style scoped>

.no-room {
  color: white;
  font-family: Montserrat;
  font-size: 28px;
  font-weight: 700;
  line-height: 34.13px;
  margin: 20px;
  text-align: center;
}

.container {  
  display: flex; 
  flex-direction: column; 
}

.banner {
  display: grid;
  position: relative;
  overflow: hidden;
  height: 300px;
  margin: 30px 35px 20px 35px;
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

.search-container {
  position: absolute;
  left: 70px;
  bottom: 40px;
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

.content {
  display: flex;
  flex-direction: row;
  justify-content: space-between;
  width: 100%;
  position: relative;
  left: 65px;
}

.sidebar {
  display: flex;
  flex-direction: column;
  width: 367px;
  margin-right: 20px;
}

.filter {
  display: block;
  margin-bottom: 20px;
}

.main-content {
  margin-top: 40px;
  display: flex;
  flex-direction: column;
  flex-grow: 1;
}

.result-container {
  position: relative;
  bottom: 37px;
  left: 45px;
}

.result {
  color: white;
  font-family: Montserrat;
  font-size: 28px;
  font-weight: 700;
  line-height: 34.13px;
  margin: 0;
}

.rooms {
  display: flex;
  flex-wrap: wrap;
  justify-content: flex-start;
  gap: 20px;
}

</style>
