<template>
  <div class="container">
    <v-btn @click="goBack" class="back"><span class="mdi mdi-arrow-left"></span> Назад</v-btn>
    <div class="search-container">
      <v-text-field v-model="searchQuery" placeholder="Поиск..." dense outlined></v-text-field>
    </div>
    <div class="content">
      <div class="filter">
        <div class="filter-anime">
          <FilterAnime />
        </div>
      </div>
      <div class="collections">
        <CollectionsComp
          v-for="collection in filteredCollections"
          :key="collection.id"
          :collections="collection"
          @addToCollection="addToCollection"
        />
      </div>
    </div>
  </div>
</template>

<script>
import axios from 'axios';
import FilterAnime from "@/components/FilterComp/FilterAnime.vue";
import CollectionsComp from "@/components/Collections/CollectionsComp.vue";
import { useCollectionStore } from '@/stores/collectionStore';
import { computed, onMounted, ref } from 'vue';
import { useRouter } from 'vue-router';

export default {
  components: {
    FilterAnime,
    CollectionsComp,
  },
  setup() {
    const collectionStore = useCollectionStore();
    const router = useRouter();
    const searchQuery = ref('');

    const fetchCollections = async () => {
      try {
        const response = await axios.get('https://animeenigma.ru/api/anime?limit=50&page=1&year=2024');
        const animeData = response.data.data;
        collectionStore.collections = animeData.map(anime => {
          return {
            ...anime,
            seasons: anime.videos.map(video => video.kind).filter((value, index, self) => self.indexOf(value) === index)
          };
        });
      } catch (error) {
        console.error("Ошибка при загрузке данных:", error);
      }
    };

    const goBack = () => {
      router.push('/custom-collections');
    };

    onMounted(() => {
      fetchCollections();
    });

    const addToCollection = (video) => {
      collectionStore.addToCollection(video);
    };

    const filteredCollections = computed(() => {
      if (!searchQuery.value) {
        return collectionStore.collections;
      }
      return collectionStore.collections.filter(collection =>
        collection.nameRU.toLowerCase().includes(searchQuery.value.toLowerCase())
      );
    });

    return {
      goBack,
      addToCollection,
      filteredCollections,
      searchQuery
    };
  }
};
</script>

  <style scoped>
  
  .result {  
    font-family: Montserrat;  
    font-size: 28px;  
    font-weight: 700;  
    line-height: 34.13px;  
    text-align: left;  
    color: white;  
    left: 500px;
    top: 50px; 
    position: relative;
  }  
  
  .content {
    width: 1697px;
  }
  
  .container {  
    display: flex; 
    flex-direction: column; 
  }  
  
  .collections {  
    display: flex;  
    flex-wrap: wrap;  
    justify-content: flex-end;  
    left: 35px;
    position: relative;
    gap: 20px;
  }  
  
  .filter {
    display: grid;
    width: 400px;
    margin: 0 30px 0 30px;
  }
  
  .banner {
    display: grid;
    position: relative;
    overflow: hidden;
    height: 300px;
    margin: 30px 35px 0px 35px;
    border-radius: 10px;
  }
  
  .picture {
    background-image: linear-gradient(to right, rgba(0,0,0,1), rgba(0,0,0,0)), url('src/assets/img/picture2.png');
    background-size: cover;
    background-position: center; 
    width: 100%;
    height: 100%;
    border-radius: 10px;
  }
  
  .search-container {
    left: 70px;
    bottom: 40px;
    position: absolute;
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
  