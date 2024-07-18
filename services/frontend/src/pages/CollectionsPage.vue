<template>
  <div class="container"> 
    <div class="banner">
      <div class="picture"></div>
      <div class="text">
        <div class="title">Коллекции опенингов</div>
        <div class="subtitle">Откройте мир аниме через его опенинги! Насладитесь музыкой и анимацией, <br>определившими каждый шедевр. Откройте для себя новые жемчужины!</div>
      </div>
      <div class="search-container">
        <v-text-field 
          v-model="searchQuery"
          class="search" 
          density="compact" 
          label="Поиск..." 
          variant="plain" 
          hide-details 
          single-line>
        </v-text-field>
        <v-btn text class="button" @click="onSearchIconClick">Поиск</v-btn>
      </div>
    </div>
    <div class="content">  
      <div class="result" v-if="searchQuery">Результаты поиска</div>  
      <div class="filter">
        <div class="filter-anime">
          <FilterAnime/>
        </div>
      </div>
      <div class="collections">
        <div v-if="searchQuery" class="result">Результаты поиска</div>
        <div v-for="collection in filteredCollections" :key="collection.id" class="collection-card">
          <img :src="collection.image" alt="Collection Image" class="collection-image">
          <div class="collection-info">
            <div class="collection-name">{{ collection.name }}</div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script>
  import FilterAnime from "@/components/FilterComp/FilterAnime.vue";
  import CollectionsComp from "@/components/Collections/CollectionsComp.vue";
  import { useCollectionStore } from "@/stores/collectionStore";

  export default {
    components: {
      FilterAnime,
      CollectionsComp,
    },
    data () { 
      return { 
        searchQuery: ''
      }; 
    },
    computed: {
      collections() {
        return useCollectionStore().collections;
      },
      filteredCollections() {
        if (!this.searchQuery) {
          return this.collections;
        }
        return this.collections.filter(collection => 
          collection.nameRU.toLowerCase().includes(this.searchQuery.toLowerCase())
        );
      }
    },
    methods: {
      async siteCollections() {
        await useCollectionStore().siteCollections();
      },
      onSearchIconClick() {
      }
    },
    mounted() {
      this.siteCollections();
    }
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
  background: linear-gradient(0deg, rgba(0, 0, 0, 0.7), rgba(0, 0, 0, 0.3));
  overflow: hidden;
}

.collection-name {
  font-family: Montserrat;
  font-size: 14px;
  font-weight: 600;
  color: white;
}


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
