<template>
    <div class="genre-card">
      <img class="genre-image" :src="genre.image" :alt="`Изображение ${genre.title}`">
      <div class="genre-title">{{ genre.title }}</div>
    </div>
  </template>
  
  <script>
  export default {
    name: 'GenreCard',
    props: {
      genre: Object
    }
  }
  
  </script>
  
  <style scoped>
.genre-card { 
  cursor: pointer;
  width: 252px; 
  position: relative; 
  height: 330px; 
  border-radius: 10px; 
  margin: 0 55px; 
  overflow: hidden;
} 

.genre-image {
  width: 100%;
  height: 100%;
  position: absolute;
  top: 0;
  left: 0;
}

.genre-title { 
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
}
  </style>
  