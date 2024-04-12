<template>  
  <div class="collection-card" @mouseover="showDetails = true" @mouseleave="showDetails = false">   
    <img class="collection-image" :src="collections.image" :alt="`Изображение ${collections.title}`">   
    <div class="collection-info" :class="{ active: showDetails }" :style="{height: showDetails ? 'auto' : '82px'}">    
      <div class="collection-title">{{ collections.title }}</div> 
      <div v-if="!showDetails" class="season-num">{{ seasonText }}</div>   
      <div v-if="showDetails" class="additional-info">   
        <div v-for="genre in collections.genres" :key="genre" class="genres">{{ genre }}</div>   
        <v-select v-if="showDetails" class="select"   
          v-model="selectedSeason"   
          :items="collections.season"   
          @change="setSeasonText(selectedSeason)"
          density="compact"   
        ></v-select>   
        <v-btn class="plus-collect" @click="addToCollection">Добавить в коллекцию</v-btn>   
      </div>   
    </div>    
  </div>  
</template>   

<script>   
import { collections } from '@/components/Collections/CollectionsComp.js';

export default {   
  name: 'CollectionsComp',   
  props: {   
    collections: Object   
  },   
  data() {   
    return {  
      showDetails: false,  
      selectedSeason: '1 сезон', 
      seasonText: '1 сезон'
    };  
  },
  computed: {
    seasonText() {
      return this.selectedSeason;
    }
  },   
  methods: {  
    addToCollection() {  
    },
    setSeasonText(season) {
      this.seasonText = season;
    }
  }  
}   
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
    transition: all 0.3s;
    
  }  
  
  .collection-card:hover .collection-info {
    transform: translateY(0);
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
  transition: all 0.3s; 
  transform: translateY(0); 
  height: 82px;
}

  
  .active {
    transform: translateY(-50px);
  }
  
  .additional-info {
    padding: 6px;
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
  