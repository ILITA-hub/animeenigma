<template>
  <div ref="container" class="container">
    <a @click="handleBack" class="back"><span class="mdi mdi-arrow-left"></span> Назад</a>
    <div class="banner">
      <v-card class="main-banner">
        <v-img class="bg-picture" height="260" :src="collection?.image || '/zoro.jpg'" cover></v-img>
        <div class="header">
          <div class="text-container">
            <v-card-title class="title">
              {{ collection?.name || 'Без названия' }}
            </v-card-title>
          </div>
        </div>
      </v-card>
    <v-card class="player-stats-banner">
      <v-card-title class="stats-title">Статистика игрока</v-card-title>
      <div class="stats-windows">
        <div class="stat-window" v-for="(stat, index) in stats" :key="index">
          <div class="stat-text">{{ stat }}</div>
        </div>
      </div>
    </v-card>
    </div>
    <div class="content">
      <div class="sidebar">
        <FilterAnime :incoming-selected-genres="selectedGenres" :incoming-selected-years="selectedYears"
          @update:selectedGenres="setSelectedGenres" @update:selectedYears="setSelectedYears" />
        <div class="main-content">
          <div class="result">Результаты поиска</div>
          <AnimeCard v-for="(video, index) in videos" :key="index" :anime="video" :isCollection="true" />
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed, onMounted, ref, toRef } from 'vue';
import AnimeCard from '@/components/Anime/AnimeCard.vue';
import FilterAnime from "@/components/FilterComp/FilterAnime.vue";
import { useCollectionStore } from '@/stores/collectionStore';


const {id} = defineProps({
  id: String,
});

const stats = ref(['Очков', 'Викторин создано', 'Викторин пройдено', 'Коллекций создано']);
const collectionStore = useCollectionStore()
const selectedGenres = ref([]);
const selectedYears = ref([]);
const collection = computed(()=> collectionStore.collection)
const videos = computed(()=> collection.value.videos)

onMounted(async () => {
  await collectionStore.getCollection(id)
});

const removeVideo = (videoId) => {
  videos.value = videos.value.filter(video => video.id !== videoId);
};

function setSelectedYears(params) {
  
}
function setSelectedGenres(params) {
  
}

</script>

<style scoped>

.result {
  color: white;
  font-family: Montserrat;
  font-size: 28px;
  font-weight: 700;
  line-height: 34.13px;
  margin: 0;
}

.back {
  color: white;
  font-family: Montserrat;
  font-size: 16px;
  font-weight: 500;
  line-height: 19.5px;
  text-align: left;
  display: flex;
  align-items: center;
  cursor: pointer;
  position: relative;
  left: 29px;
  top: 13px;
}

.back .mdi {
  color: rgba(51, 169, 255, 1);
  margin-right: 5px;
}

.player-stats-banner {
  max-width: 350px;
  padding: 0px 0px 0px 23px;
  background-color: rgba(33, 35, 53, 1);
  border-radius: 10px;
  display: flex;
  flex-direction: column;
}

.stats-title {
  font-family: Montserrat;
  font-size: 16px;
  font-weight: 600;
  line-height: 19.5px;
  text-align: left;
  color: white;
  padding-left: 0px;
  padding-bottom: 15px;
}

.stats-windows {
  display: flex;
  flex-wrap: wrap;
  gap: 15px;
}

.stat-window {
  width: 144px;
  height: 101px;
  background-color: rgba(255, 255, 255, 0.1);
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 10px;
  transition: background-color 0.3s;
  text-align: center;
}

.stat-window:hover {
  background-color: rgba(20, 112, 239, 1);
}

.stat-text {
  font-family: Montserrat;
  font-size: 12px;
  font-weight: 500;
  line-height: 14.63px;
  color: white;
}

.container {
  display: flex;
  flex-direction: column;
  color: white;
}

.banner {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 15px;
  gap: 10px;
}

.main-banner {
  width: 100%;
  border-radius: 10px;
  position: relative;
}

.header {
  display: flex;
  align-items: center;
  height: 90px;
  background-color: rgba(33, 35, 53, 1);
  position: absolute;
  bottom: 0;
  left: 0;
  right: 0;
}

.text-container {
  font-family: Montserrat;
  color: white;
  padding-left: 20px;
}

.content {
  display: flex;
  gap: 20px;
}

.sidebar {
  /* width: 300px; */
  display: flex;
  width: 100%;
  /* flex-direction: column; */
}

.anime-name {
  font-weight: bold;
}

.video-list {
  display: flex;
  flex-direction: column;
  gap: 5px;
}

.main-content {
  flex: 1;
  display: flex;
  flex-wrap: wrap;
  gap: 20px;
  width: 100%;
}
</style>
