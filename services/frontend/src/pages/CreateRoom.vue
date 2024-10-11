<template>
  <div class="create-room-page">

    <v-container fluid>
      <v-row>
        <div class="banner">
          <div class="picture">
            <div class="text">
              <div class="title">Коллекции опенингов</div>
              <div class="subtitle">Откройте мир аниме через его опенинги! Насладитесь музыкой и анимацией,
                <br>определившими
                каждый шедевр. Откройте для себя новые жемчужины!
              </div>
            </div>
            <div class="search-container">
              <v-text-field v-model="searchQuery" class="search" density="compact" label="Поиск..." variant="plain"
                hide-details single-line>
              </v-text-field>
              <v-btn text class="button">Поиск</v-btn>
            </div>
          </div>
        </div>
      </v-row>
    </v-container>

    <div style="padding: 0 70px;">
      <v-row justify="start">

        <div class="d-flex flex-column">
          <div ref="createRoomMenu" class="create-room">
            <a @click="handleBack" class="back"><span class="mdi mdi-arrow-left"></span> Назад</a>
            <v-card class="form">
              <div class="text">Создать комнату</div>
              <v-text-field class="field" density="comfortable" variant="plain" placeholder="Название лобби"
                v-model="roomSetting.roomName"></v-text-field>
              <v-select class="select" variant="plain" density="comfortable" :items="playersCountsVariants"
                label="Количество игроков" hide-details v-model="roomSetting.selectedPlayerCount"></v-select>
              <div class="collection">
                <div class="openings">Опенинги</div>
                <v-switch style="color: #fff;" v-model="swithController"
                  :label="`Выбор из: ${swithController ? 'Аниме' : 'Коллекций'}`" inset color="purple"></v-switch>
              </div>
              <v-btn color="#1470EF" class="mb-4" @click="createRoom">Опубликовать</v-btn>
            </v-card>
          </div>
          <FilterAnime :incoming-selected-genres="selectedGenres" :incoming-selected-years="selectedYears"
            @update:selectedGenres="setGenres" @update:selectedYears="setYears" />
        </div>

        <div v-if="swithController" class="switch-content-container">
          <AnimeCard @selectVideo="addSelectedVideo" v-for="anime in animeStore.anime" :anime="anime" />
        </div>

        <div v-if="!swithController" class="switch-content-container">
          <CollectionCard @add-collection="addCollectionToRoom" :isActionAccept="true"
            v-for="collection in collectionStore.collections" :collection="collection" />
        </div>

      </v-row>
    </div>
  </div>

</template>

<script setup>
import AnimeCard from '@/components/Anime/AnimeCard.vue';
import FilterAnime from '@/components/FilterComp/FilterAnime.vue';
import CollectionCard from '@/components/Collections/CollectionCard.vue';
import { useAnimeStore } from '@/stores/animeStore';
import { useRoomStore } from '@/stores/roomStore';
import { useRouter, useRoute } from 'vue-router';
import { ref, onMounted, onUnmounted, watch } from 'vue'
import { useCollectionStore } from '@/stores/collectionStore';


const roomStore = useRoomStore();
const router = useRouter();
const route = useRoute();
const animeStore = useAnimeStore()
const collectionStore = useCollectionStore()
const roomSetting = ref({ selectedVideos: [], selectedCollectins: [] })
const createRoomMenu = ref(null)
const playersCountsVariants = ref([2, 4, 6, 8, 10])
const searchQuery = ref('')
const swithController = ref(false)
const selectedGenres = ref([]);
const selectedYears = ref([]);


const handleBack = () => {
  if (route.meta.isDirectNavigation) {
    router.push('/main');
  } else {
    router.go(-1);
  }
};

function toFixElement() {
  const elem = createRoomMenu.value
  const transformValue = window.pageYOffset > 300 ? `translateY(${window.pageYOffset - 300}px)` : `translateY(0)`
  elem.style.transform = transformValue
}

function addCollectionToRoom(collection) {
  const selectedCollectins = roomSetting.value.selectedCollectins
  let isCollectionIn = false
  if (selectedCollectins.length > 0) {
    isCollectionIn = !!selectedCollectins.find((item) => {
      return item.id === collection.id
    })
  } else {
    isCollectionIn = false
  }
  if (isCollectionIn) return
  roomSetting.value.selectedCollectins.push(collection)
}

function addSelectedVideo(selectedVideo) {
  const isVideoInList = roomSetting.value.selectedVideos.find((video) => video.id === selectedVideo.id)

  if (!isVideoInList && selectedVideo !== null) {
    roomSetting.value.selectedVideos.push(selectedVideo)
  }
}

function setGenres(selected) {
  selectedGenres.value = selected
  animeStore.setGenres(selected)
}
function setYears(selected) {
  selectedYears.value = selected
  animeStore.setYears(selected)
}

onMounted(async () => {
  await animeStore.animeRequest()
  await collectionStore.siteCollections()
  window.addEventListener('scroll', toFixElement);
});

onUnmounted(async () => {
  window.removeEventListener('scroll', toFixElement)
})

watch(swithController, async (newVal, oldVal) => {
  console.log(newVal)
  if (newVal) {
    animeStore.animeRequest()
  } else {
    collectionStore.siteCollections()
  }
})

function createRoom() {
  console.log(roomSetting.value)
}
</script>

<style scoped>
.create-room-page {
  min-height: 100vh;
}

.switch-content-container {
  display: flex;
  flex-wrap: wrap;
  margin-top: 20px;
  gap: 20px 0;
  flex-basis: 0;
  flex-grow: 1;
  max-width: 100%;
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
  margin-right: auto;
  margin-left: 10px;
  margin-bottom: 10px;
}

.back .mdi {
  color: rgba(51, 169, 255, 1);
  margin-right: 5px;
}

.create-room {
  display: flex;
  flex-direction: column;
  justify-content: flex-start;
  align-items: flex-start;
  margin-top: 20px;
  transition: all 0.5s ease-in-out;
  width: 320px;
}

.form {
  border-radius: 10px;
  box-shadow: 10px 2px 50px 0px rgba(0, 0, 0, 0.05);
  background: rgb(33, 35, 53);
  padding: 20px;
  max-height: 1000px;
  width: 100%;
}

.text {
  color: rgb(255, 255, 255);
  font-family: Montserrat;
  font-size: 22px;
  font-weight: 700;
  line-height: 27px;
  letter-spacing: 0%;
  text-align: left;
  padding-bottom: 15px;
  margin-top: 5px;

}

.field {
  color: rgb(194, 194, 194);
  font-family: Montserrat;
  font-size: 16px;
  font-weight: 500;
  letter-spacing: 0%;
  position: relative;
  top: 5px;
  background-color: rgba(255, 255, 255, 0.1);
  margin-bottom: 15px;
  border-radius: 10px;
  height: 50px;
  display: grid;
  padding-left: 15px;
}

.mb-4 {
  position: relative;
  width: 100%;
  height: 50px !important;
  font-size: 16px;
  display: flex;
  margin: 15px 55px 0px 0px;
  border-radius: 10px;
  text-transform: none;
  font-family: Montserrat;

}

.collection .collection-btn {
  background: rgba(51, 169, 255, 0.1);
  color: rgba(51, 169, 255, 1);
  text-transform: none;
  font-family: Montserrat;
  position: relative;
  width: 100%;
  height: 40px !important;
  font-size: 16px;
  display: flex;
  padding: 15px 55px 15px 55px;
  border-radius: 10px;
}

.openings {
  margin: 10px 0 10px 0;
  color: rgb(255, 255, 255);
  font-family: Montserrat;
  font-size: 16px;
}

.select {
  background: rgba(255, 255, 255, 0.1);
  border-radius: 10px;
  overflow: hidden;
  height: 50px;
  color: white;
  font-family: 'Montserrat';
  padding: 0 15px 0 15px;
}

.banner {
  display: grid;
  position: relative;
  overflow: hidden;
  width: 100%;
  height: 300px;
  margin: 30px 35px 20px 35px;
  border-radius: 10px;
}

.picture {
  background-image: linear-gradient(to right, rgba(0, 0, 0, 1), rgba(0, 0, 0, 0)), url('src/assets/img/picture2.png');
  background-size: cover;
  background-position: center;
  width: 100%;
  height: 100%;
  border-radius: 10px;
  padding: 0 70px;
  display: flex;
  flex-direction: column;
  justify-content: center;
}

.search-container {
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
