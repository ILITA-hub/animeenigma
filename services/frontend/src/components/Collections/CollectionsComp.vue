<template>
  <div v-if="collections" class="collection-card">
    <img class="collection-image" :src="collections.imgPath" :alt="`Изображение ${collections.nameRU}`">
    <div class="collection-info">
      <div class="collection-title">{{ collections.nameRU }}</div>
      <div class="additional-info">
        <v-select
          class="select"
          v-model="selectedVideo"
          :items="collections.videos"
          item-title="name"
          :item-value="video => video"
          label="Выберите видео"
          density="compact"
        ></v-select>
        <v-btn class="plus-collect" @click="addToCollection">Добавить в коллекцию</v-btn>
      </div>
    </div>
  </div>
</template>


<script>
import { useCollectionStore } from '@/stores/collectionStore';
import { ref } from 'vue';

export default {
  name: 'CollectionsComp',
  props: {
    collections: {
      type: Object,
      required: true
    }
  },
  setup() {
    const collectionStore = useCollectionStore();
    const selectedVideo = ref(null);

    const addToCollection = () => {
      if (selectedVideo.value && selectedVideo.value.id) {
        if (!collectionStore.selectedOpenings.some(video => video.id === selectedVideo.value.id)) {
          collectionStore.addToCollection(selectedVideo.value);
        }
      } else {
        console.error('Не выбран объект видео или у видео нет id');
      }
    };
    
    return {
      selectedVideo,
      addToCollection,
    };
  },
};
</script>

<style scoped>
.collection-card {
  cursor: pointer;
  width: 320px;
  position: relative;
  height: 445px;
  border-radius: 10px;
  margin: 0 55px;
  overflow: hidden;
  transition: transform 0.3s ease;
}

.collection-card:hover .collection-info {
  height: auto;
}

.collection-image {
  width: 100%;
  height: 100%;
  position: absolute;
  top: 0;
  left: 0;
}

.collection-info {
  position: absolute;
  bottom: 0;
  left: 0;
  width: 100%;
  color: white;
  font-size: 16px;
  font-family: "Montserrat", sans-serif;
  font-weight: bold;
  padding: 10px 15px;
  backdrop-filter: blur(2px);
  transition: height 0.3s ease;
  height: 50px;
  background: linear-gradient(0deg, rgba(0, 0, 0, 0.7), rgba(0, 0, 0, 0.3));
  overflow: hidden;
}

.collection-info.active {
  height: auto;
}

.additional-info {
  padding: 6px;
  margin-top: 10px;
}

.additional-info div {
  margin: 5px 0;
}

.genres {
  background-color: white;
  color: black;
  border-radius: 10px;
  font-family: Montserrat;
  font-size: 12px;
  font-weight: 400;
  width: auto;
  height: 35px;
  text-align: center;
  position: relative;
  display: inline-block;
  padding: 10px;
}

.plus-collect {
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

.select {
  width: 280px;
  height: 40px;
  background: white;
  color: black;
  border-radius: 10px;
  overflow: hidden;
}
</style>
