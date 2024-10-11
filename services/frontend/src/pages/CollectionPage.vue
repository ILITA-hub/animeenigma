<template>
  <div ref="container" class="container">

    <div class="banner">
      <v-card class="main-banner">
        <v-img class="bg-picture" height="211" :src="collection?.image || '/zoro.jpg'" cover></v-img>
        <div class="header">
          <div class="text-container">
            <v-card-title class="title">
              {{ collection?.name || 'Без названия' }}
            </v-card-title>
          </div>
        </div>
        
      </v-card>
    </div>
    <div class="content">
      <div class="sidebar">
        <FilterAnime :incoming-selected-genres="selectedGenres" :incoming-selected-years="selectedYears"
          @update:selectedGenres="setSelectedGenres" @update:selectedYears="setSelectedYears" />
        <div class="main-content">
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

const collectionStore = useCollectionStore()
const selectedGenres = ref([]);
const selectedYears = ref([]);
const searchQuery = ref('');
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
.container {
  display: flex;
  flex-direction: column;
  gap: 20px;
  color: white;
}

.banner {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 20px;
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
