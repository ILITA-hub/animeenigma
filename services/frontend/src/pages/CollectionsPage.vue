<template>
   <div class="container"> 
    <div class="banner">
      <img class="picture">
      <div class="text">
      <div class="title">Коллекции опенингов</div>
      <div class="subtitle">Откройте мир аниме через его опенинги! Насладитесь музыкой и анимацией, <br>определившими каждый шедевр. Откройте для себя новые жемчужины!</div>
    </div>
      <v-text-field class="search" density="compact" label="Поиск..." variant="" single-line>
            <template v-slot:append>
              <v-btn text class="button" @click="onSearchIconClick">Поиск</v-btn>
            </template>
          </v-text-field>
    </div>
    <div class="content">  
      <div class="result">Результаты поиска</div>  
    <div class="filter">
        <div class="filter-anime">
      <FilterAnime/>
    </div>
    </div>
    <div class="collections">
      <CollectionsComp
      v-for="collections in collections"
      :key="collections.id"
      :collections="collections"/>
    </div>
  </div>
</div>
  </template>

<script>
  import FilterAnime from "@/components/FilterComp/FilterAnime.vue"
  import CollectionsComp from "@/components/Collections/CollectionsComp.vue"
  import { collections } from "@/components/Collections/CollectionsComp.js"

  export default {
    components: {
      FilterAnime,
      CollectionsComp,
    },
    data () { 
    return { 
      collections,
    }; 
  },
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
  margin: 30px 30px 0px 30px;
}

.picture {
  background-image: linear-gradient(to right, rgba(0,0,0,1), rgba(0,0,0,0)), url('src/assets/img/picture2.png');
  background-size: cover;
  background-position: center; 
  display: grid;
  border-radius: 10px;
  position: relative;
  top: 50%;
  left: 50%;
  transform:translate(-50%,-50%);
  width: 1697px;
  height: 300px;
}

.search {
  left: 70px;
  bottom: 270px;
  position: relative;
  font-family: Montserrat;
  margin: 0 0px 0 15px;
  width: 480px;
  height: 40px;
  border-radius: 10px;
  background-color: rgba(255, 255, 255, 0.1);
  color: white;
}

.button {
  font-family: Montserrat;
  font-weight: normal;
  text-transform: none;
  font-size: 16px;
  left: 180px;
  height: 40px;
  width: 164px;
  border-radius: 10px;
  background-color: #1470EF;
  color: white;
}

.text {
  position: relative;
  bottom: 300px;
  color: white;
  font-family: Montserrat;
  text-align: left;
  margin: 80px 0px 10px 85px;
}
.title {
  font-size: 45px;
  font-weight: 700;
  line-height: 54.86px;
  text-align: left;
}

.subtitle {
  font-size: 16px;
  font-weight: 500;
  line-height: 22px;
  text-align: left;
  bottom: -7px;
  position: relative;
}

</style>