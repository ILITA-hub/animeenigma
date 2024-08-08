<template>
  <div class="container">
    <div class="banner">
      <div class="picture"></div>
      <div class="text">
        <div class="title">Очень крутой заголовок</div>
        <div class="subtitle">Очень крутое описание страницы. Нет, ну правда.<br>Прям ОЧЕНЬ крутое описание!!</div>
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
        <v-btn text class="button" @click="onSearchIconClick">Поиск</v-btn>
      </div>
    </div>
    <div class="content">
      <div class="sidebar">
        <div class="filter">
          <div class="filter-anime">
            <FilterAnime />
          </div>
        </div>
        <div class="selected-videos-container">
          <div class="openings">Выбранные видео</div>
          <div class="selected-videos scrollable">
            <div v-for="video in selectedVideos" :key="video.id" class="selected-video">
              <span class="video-name">{{ video.name }}</span>
              <v-icon
                small
                class="remove-icon"
                @click="removeVideo(video.id)"
              >mdi-close</v-icon>
            </div>
          </div>
        </div>
        <div class="pagination">
          <v-btn @click="prevPage" :disabled="!prevPageNumber">Назад</v-btn>
          <span>Страница {{ currentPage }} из {{ totalPages }}</span>
          <v-btn @click="nextPage" :disabled="!nextPageNumber">Вперед</v-btn>
        </div>
      </div>
      <div class="main-content">
        <a @click="handleBack" class="back"><span class="mdi mdi-arrow-left"></span> Назад</a>
        <div class="result-container" v-if="searchQuery">
          <div class="result">Результаты поиска</div>
        </div>
         <!-- <div v-if="filteredAnime.length === 0">Аниме не найдено</div> -->
        <div class="anime">
          <AnimeCard
            v-for="anime in filteredAnime"
            :key="anime.id"
            :anime="anime"
            @addToCollection="addToCollection"
          />
        </div>
      </div>
    </div>
  </div>
</template>

<script>
import FilterAnime from "@/components/FilterComp/FilterAnime.vue";
import AnimeCard from "@/components/Anime/AnimeCard.vue";
import { useCollectionStore } from '@/stores/collectionStore';
import { useAnimeStore } from '@/stores/animeStore'; 
import { computed, onMounted, ref } from 'vue';
import { useRouter, useRoute } from 'vue-router';

export default {
  components: {
    FilterAnime,
    AnimeCard,
  },
  setup() {
    const collectionStore = useCollectionStore();
    const animeStore = useAnimeStore();
    const router = useRouter();
    const route = useRoute();
    const searchQuery = ref('');

    const handleBack = () => {
      if (route.meta.isDirectNavigation) {
        router.push('/main');
      } else {
        router.go(-1);
      }
    };

    const nextPage = () => {
      if (animeStore.nextPageNumber) {
        animeStore.animeRequest(animeStore.nextPageNumber);
      }
    };

    const prevPage = () => {
      if (animeStore.prevPageNumber) {
        animeStore.animeRequest(animeStore.prevPageNumber);
      }
    };

    onMounted(() => {
      animeStore.animeRequest();
    });

    const addToCollection = (video) => {
      collectionStore.addToCollection(video);
    };

    const removeVideo = (videoId) => {
      collectionStore.removeFromCollection(videoId);
    };

    const filteredAnime = computed(() => {
      if (!animeStore.anime) return [];

      return animeStore.anime.filter(anime => {
        const name = anime.nameRU || '';
        return name.toLowerCase().includes(searchQuery.value.toLowerCase());
      });
    });
 
    const onSearchIconClick = () => {
    };

    return {
      handleBack,
      addToCollection,
      filteredAnime,
      searchQuery,
      currentPage: computed(() => animeStore.currentPage),
      totalPages: computed(() => animeStore.totalPages),
      nextPage,
      prevPage,
      prevPageNumber: computed(() => animeStore.prevPageNumber),
      nextPageNumber: computed(() => animeStore.nextPageNumber),
      selectedVideos: computed(() => collectionStore.selectedOpenings),
      removeVideo,
      onSearchIconClick,
    };
  }
};
</script>



<style scoped>
.remove-icon {
  cursor: pointer;
  color: red;
  flex-shrink: 0;
}

.back {
    color: white;
    font-family: Montserrat;
    font-size: 16px;
    font-weight: 500;
    line-height: 19.5px;
    display: flex;
    align-items: center;
    cursor: pointer;
    margin: 0;
    padding: 10px;
    position: relative;
    width: 100px;
}
.back .mdi {
  color: rgba(51, 169, 255, 1);
  margin-right: 5px;
}

.pagination {
  display: block;
  margin-top: auto;
  color: rgb(225, 11, 11);
}

.result {
  color: white;
  font-family: Montserrat;
  font-size: 28px;
  font-weight: 700;
  line-height: 34.13px;
  margin: 0;
}

.content {
  display: flex;
  flex-direction: row;
  width: 100%;
  position: relative;
}

.main-content {
  display: flex;
  flex-direction: column;
  flex-grow: 1;
  margin-top: 10px;
}

.sidebar {
  display: flex;
  flex-direction: column;
  margin-right: 50px;
}

.filter,
.selected-videos-container,
.pagination {
  margin-bottom: 20px;
}

.result-container {
    position: relative;
    left: 29px;
}

.anime {
  display: flex;
  flex-wrap: wrap;
  justify-content: flex-start;
  gap: 20px;
  margin-top: 10px;
}


.search {
  flex-grow: 1;
  margin-right: 10px;
  background-color: rgba(255, 255, 255, 0.1);
  color: white;
  border-radius: 10px;
  font-family: Montserrat;
}

.selected-videos-container {
  display: block;
  position: relative;
  left: 35px;
  background: rgba(255, 255, 255, 0.1);
  border-radius: 10px;
  padding: 10px;
  width: 320px;
}

.selected-videos {
  max-height: 243px;
  overflow-y: auto;
}

.selected-videos::-webkit-scrollbar {
  width: 6px;
}

.selected-videos::-webkit-scrollbar-track {
  background: #f1f1f1;
  border-radius: 10px;
}

.selected-videos::-webkit-scrollbar-thumb {
  background: #888;
  border-radius: 10px;
}

.selected-videos::-webkit-scrollbar-thumb:hover {
  background: #555;
}

.openings {
  margin: 10px 0;
  color: rgb(255, 255, 255);
  font-family: Montserrat;
  font-size: 16px;
}

.selected-video {
  background: rgba(255, 255, 255, 0.1);
  border-radius: 10px;
  padding: 8px;
  margin-bottom: 10px;
  color: white;
  font-family: Montserrat;
  font-size: 15px;
  font-weight: 400;
  line-height: 19.5px;
  text-align: left;
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.video-name {
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: calc(100% - 24px);
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

.search-container {
  position: absolute;
  left: 70px;
  bottom: 40px;
  width: 480px;
  height: 40px;
  display: flex;
  justify-content: center;
  margin-bottom: 20px;
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
</style>
