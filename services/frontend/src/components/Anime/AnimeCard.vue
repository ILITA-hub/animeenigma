<template>
  <div v-if="anime" class="anime-card">
    <img class="anime-image" :src="anime.imgPath" :alt="`Изображение ${anime.nameRU}`">
    <div ref="animelInfo" class="anime-info">
      <div ref="animelTitle" class="anime-title">{{ anime.nameRU }}</div>
      <div class="additional-info">
        <div class="genres">
          <span class="genre" v-for="genre in anime.genres" :key="genre.id" @click="selectGenre(genre.genre.id)">
            {{ genre.genre.nameRu }}
          </span>
        </div>
        <v-list density="compact" class="select-list">
          <v-list-group  v-model="selectedVideo" :value="selectedVideo">
            <template v-slot:activator="{ props }">
              <v-list-item  v-bind="props" :title="selectedVideo ? selectedVideo.name : 'Выберите видео'"></v-list-item>
            </template>
            <v-list-item
              v-for="video in anime.videos"
              :key="video.id"
              :title="video.name"
              @click="selectVideo(video)"
            ></v-list-item>
          </v-list-group>
        </v-list>
        <v-btn class="plus-collect" @click="addToCollection">Добавить в коллекцию</v-btn>
      </div>
    </div>
  </div>
</template>

<script>
import { useCollectionStore } from '@/stores/collectionStore';
import { ref } from 'vue';

export default {
  name: 'AnimeCard',
  props: {
    anime: {
      type: Object,
      required: true
    }
  },
  mounted(){
    const animelInfo = this.$refs.animelInfo
    const animelTitle = this.$refs.animelTitle
    animelInfo.style.bottom = `-${animelInfo.offsetHeight - animelTitle.offsetHeight - 17}px`
  },
  setup(props, { emit }) {
    const collectionStore = useCollectionStore();
    const selectedVideo = ref(null);

    const selectVideo = (video) => {
      selectedVideo.value = video;
    };

    const addToCollection = () => {
      if (selectedVideo.value && selectedVideo.value.id) {
        if (!collectionStore.selectedOpenings.some(video => video.id === selectedVideo.value.id)) {
          collectionStore.addToCollection(selectedVideo.value);
        }
      } else {
        console.error('Не выбран объект видео или у видео нет id');
      }
    };
    const selectGenre = (genreId) => {
      emit('select-genre', genreId);
    };

    return {
      selectedVideo,
      selectVideo,
      addToCollection,
      selectGenre
    };
  },
};
</script>

<style scoped>

.anime-card {
  cursor: pointer;
  width: 320px;
  position: relative;
  height: 445px;
  border-radius: 10px;
  margin: 0 45px;
  overflow: hidden;
  transition: transform 0.3s ease;
}


.anime-image {
  width: 100%;
  height: 100%;
  position: absolute;
  top: 0;
  left: 0;
}

.anime-info {
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

.anime-card:hover .anime-info {
  bottom: 0%!important;
}

.additional-info {
  padding: 6px;
  margin-top: 10px;
}

.genres {
  /* display: none; */
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

.anime-card:hover .genres {
  display: flex;
}

.select-list {
  width: 278px;
  background: white;
  color: black;
  border-radius: 10px;
  overflow: hidden;
  font-family: Montserrat;
  margin: 5px 0 5px 0;
  padding: 0;
}

.plus-collect {
  width: 280px;
  height: 50px;
  padding: 15px 55px 15px 55px;
  border-radius: 10px;
  background: rgba(20, 112, 239, 1);
  font-family: Montserrat;
  font-size: 16px;
  font-weight: 600;
  line-height: 19.5px;
  text-align: left;
  text-transform: none;
}

</style>