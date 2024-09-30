<template>
  <div class="collection-card">
    <img :src="collection?.image" alt="Collection Image" class="collection-image">
    <div class="collection-info">
      <div class="collection-name">{{ collection?.name }}</div>
      <div class="genres">
        <span class="genre" v-for="genre in collection?.genres" :key="genre" @click="toggleGenre(genre)">{{ genre }}</span>
      </div>
    </div>
  </div>
</template>

<script>
export default {
  props: {
    collection: Object
  },
  emits: ['toggle-genre'],
  setup(props, { emit }) {
    const toggleGenre = (genre) => {
      emit('toggle-genre', genre);
    };

    return {
      toggleGenre
    };
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
  transition: transform 0.3s ease;
}

.collection-image {
  width: 100%;
  height: 100%;
  position: absolute;
  top: 0;
  left: 0;
}

.collection-card:hover .collection-info {
  bottom: 0%;
}

.collection-info {
  position: absolute;
  bottom: -29%;
  left: 0;
  width: 100%;
  color: white;
  font-size: 16px;
  font-family: "Montserrat", sans-serif;
  font-weight: bold;
  padding: 10px 15px;
  backdrop-filter: blur(2px);
  transition: bottom 0.3s ease;
  background: linear-gradient(0deg, rgba(0, 0, 0, 0.7), rgba(0, 0, 0, 0.3));
  overflow: hidden;
}

.collection-info:hover {
  bottom: 0%;
}

.collection-name {
  font-family: Montserrat;
  font-size: 14px;
  font-weight: 600;
  color: white;
}

.genres {
  display: flex;
  flex-wrap: wrap;
  gap: 5px;
  margin-top: 25px;
}

.genre {
  display: inline-block;
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

</style>