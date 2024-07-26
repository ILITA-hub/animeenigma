<template>
  <div class="container">
    <a @click="goBack" class="back"><span class="mdi mdi-arrow-left"></span> Назад</a>
    <div class="content">
      <div class="search-container">
        <v-text-field 
          v-model="searchQuery"
          class="search" 
          density="compact" 
          label="Поиск на странице..." 
          variant="plain" 
          hide-details 
          single-line>
        </v-text-field>
      </div>
      <div class="filter">
        <div class="filter-anime">
          <FilterAnime />
        </div>
      </div>
      <div class="pagination">
        <v-btn @click="prevPage" :disabled="!prevPageNumber">Назад</v-btn>
        <span>Страница {{ currentPage }} из {{ totalPages }}</span>
        <v-btn @click="nextPage" :disabled="!nextPageNumber">Вперед</v-btn>
      </div>
      <div class="result" v-if="searchQuery">Результаты поиска</div>  
      <div class="collections">
        <AnimeCard
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
import FilterAnime from "@/components/FilterComp/FilterAnime.vue";
import AnimeCard from "@/components/Anime/AnimeCard.vue";
import { useCollectionStore } from '@/stores/collectionStore';
import { computed, onMounted, ref } from 'vue';
import { useRouter } from 'vue-router';

export default {
  components: {
    FilterAnime,
    AnimeCard,
  },
  setup() {
    const collectionStore = useCollectionStore();
    const router = useRouter();
    const searchQuery = ref('');

    const goBack = () => {
      router.push('/custom-collections');
    };

    const nextPage = () => {
      if (collectionStore.nextPageNumber) {
        collectionStore.animeRequest(collectionStore.nextPageNumber);
      }
    };

    const prevPage = () => {
      if (collectionStore.prevPageNumber) {
        collectionStore.animeRequest(collectionStore.prevPageNumber);
      }
    };

    onMounted(() => {
      collectionStore.animeRequest();
    });

    const addToCollection = (video) => {
      collectionStore.addToCollection(video);
    };

    const filteredCollections = computed(() => {
      if (!searchQuery.value) {
        return collectionStore.collections;
      }
      return collectionStore.collections.filter(collection =>
        (collection.nameRU ?? '').toLowerCase().includes(searchQuery.value.toLowerCase())
      );
    });

    const onSearchIconClick = () => {
    };

    return {
      goBack,
      addToCollection,
      filteredCollections,
      searchQuery,
      currentPage: computed(() => collectionStore.currentPage),
      totalPages: computed(() => collectionStore.totalPages),
      nextPage,
      prevPage,
      prevPageNumber: computed(() => collectionStore.prevPageNumber),
      nextPageNumber: computed(() => collectionStore.nextPageNumber),
      onSearchIconClick,
    };
  }
};
</script>



  <style scoped>

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
margin: 10px;
}
.back .mdi {
    color: rgba(51, 169, 255, 1);
    margin-right: 5px;
}

  .pagination {
    gap: 10px;
    display: flex;
    top: 100px;
    position: relative;
    color: rgb(225, 11, 11);
  }
  
  .result {  
    font-family: Montserrat;  
    font-size: 28px;  
    font-weight: 700;  
    line-height: 34.13px;  
    text-align: left;  
    color: white;  
    left: 413px;
    top: -87px;
    position: relative;
  }  
  
  .content {
    width: 1697px;
  }
  
  .collections {  
    display: flex;
    flex-wrap: wrap;
    justify-content: flex-start;
    left: 370px;
    position: relative;
    gap: 20px;
  }  
  
  .filter {
    display: grid;
    width: 400px;
  }
  
  .search-container {
    left: 70px;
    width: 340px;
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

  </style>
  