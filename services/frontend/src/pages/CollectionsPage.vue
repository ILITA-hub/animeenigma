<template>
  <div class="container">
    <div class="banner">
      <div class="picture"></div>
      <div class="text">
        <div class="title">Коллекции опенингов</div>
        <div class="subtitle">Откройте мир аниме через его опенинги! Насладитесь музыкой и анимацией, <br>определившими
          каждый шедевр. Откройте для себя новые жемчужины!</div>
      </div>
      <div class="search-container">
        <v-text-field v-model="searchQuery" class="search" density="compact" label="Поиск..." variant="plain"
          hide-details single-line>
        </v-text-field>
        <v-btn text class="button">Поиск</v-btn>
      </div>
    </div>
    <div class="content">
      <div class="sidebar">
        <div class="filter">
          <div class="filter-anime">
            <FilterAnime
            @update:selectedGenres="setSelectedGenres"
            @update:selectedYears="setSelectedYears"
            />
          </div>
        </div>
      </div>
      <div class="main-content">
        <div class="result-container" v-if="searchQuery">
          <div class="result">Результаты поиска</div>
        </div>
        <div class="no-collection" v-if="collectionStore.collections.length === 0">Коллекция не найдена</div>
        <div class="collections">
          <CollectionCard v-for="collection in collectionStore.collections" :key="collection.id"
            :collection="collection" @toggle-genre="toggleGenreInFilter" />
        </div>
      </div>
    </div>
  </div>
</template>


<script>
import FilterAnime from "@/components/FilterComp/FilterAnime.vue";
import CollectionCard from "@/components/Collections/CollectionCard.vue";
import { useCollectionStore } from "@/stores/collectionStore";
import { useAnimeStore } from "@/stores/animeStore";
import { ref, computed, onMounted } from "vue";

export default {
  setup(props) {
    const selectedGenres = ref([]);
    const selectedYears = ref([]);
    const searchQuery = ref("");
    const collectionStore = useCollectionStore();
    const animeStore = useAnimeStore()

    const setSelectedGenres = (newGenres) => {
      selectedGenres.value = newGenres;
    };

    const setSelectedYears = (newYears) => {
      selectedYears.value = newYears;
    };
    onMounted(()=>{
      collectionStore.siteCollections()
    })

    const toggleGenreInFilter = (genreName) => {
      const index = selectedGenres.value.indexOf(genreName);
      if (index === -1) {
        animeStore.selectedGenres.push(genreName)
        selectedGenres.value.push(genreName);
      } else {
        animeStore.selectedGenres.splice(index, 1)
        selectedGenres.value.splice(index, 1);
      }
    };

    return {
      toggleGenreInFilter,
      collectionStore,
      selectedGenres,
      selectedYears,
      setSelectedGenres,
      setSelectedYears,
      searchQuery,
    };
  },
  components: {
    FilterAnime,
    CollectionCard,
  },
  async mounted() {
    await this.collectionStore.siteCollections();
  }
};
</script>

<style scoped>
.no-collection {
  color: white;
  font-family: Montserrat;
  font-size: 28px;
  font-weight: 700;
  line-height: 34.13px;
  margin: 20px;
  text-align: center;
}

.result {
  color: white;
  font-family: Montserrat;
  font-size: 28px;
  font-weight: 700;
  line-height: 34.13px;
  margin: 0;
}

.main-content {
  margin-top: 40px;
  display: flex;
  flex-direction: column;
  flex-grow: 1;
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

.result-container {
  position: relative;
  bottom: 37px;
  left: 45px;
}

.container {
  display: flex;
  flex-direction: column;
}

.collections {
  display: flex;
  flex-wrap: wrap;
  justify-content: flex-start;
  gap: 20px;
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
  background-image: linear-gradient(to right, rgba(0, 0, 0, 1), rgba(0, 0, 0, 0)), url('src/assets/img/picture2.png');
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
