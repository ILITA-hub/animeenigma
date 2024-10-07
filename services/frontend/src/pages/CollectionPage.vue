<template>
  <div class="collection-page">
    <h1>{{ collection?.name || 'Без названия' }}</h1>
    <div v-if="videos.length > 0" class="videos-list">
      <AnimeCard
        v-for="(video, index) in videos"
        :key="index"
        :anime="video"
        :isCollection="true"
      />
    </div>
    <div v-else>
      <p>В коллекции пока нет видео.</p>
    </div>
  </div>
</template>

  <script>
  import { onMounted, ref } from 'vue';
  import AnimeCard from '@/components/Anime/AnimeCard.vue';
  import axios from 'axios';
  
  export default {
    name: 'CollectionPage',
    components: {
      AnimeCard,
    },
    props: {
      id: {
        type: String,
        required: true
      }
    },
    setup(props) {
    const collection = ref(null);
    const videos = ref([]);
  
      onMounted(async () => {
        try {
          const { data } = await axios.get(`https://animeenigma.ru/api/animeCollections/${props.id}`);
          collection.value = data;
          videos.value = data.videos;
        } catch (error) {
          console.error('Ошибка при загрузке коллекции:', error);
        }
      });
  
      return {
        collection,
        videos
      };
    }
  }
  </script>

  
  <style scoped>
  .videos-list {
    display: flex;
    flex-wrap: wrap;
    gap: 16px;
  }

  </style>