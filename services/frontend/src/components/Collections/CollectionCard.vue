<template>
  <div class="collection-card">
    <img :src="collection?.img" alt="Collection Image" class="collection-image">
    <div ref="collectionInfo" class="collection-info">
      <div ref="collectionName" class="collection-name">{{ collection?.name }}</div>
      <div class="genres">
        <span class="genre" v-for="genre in collection?.genres" :key="genre" @click="toggleGenre(genre)">{{ genre
          }}</span>
      </div>
      <v-btn v-if="isActionAccept"  class="plus-collect" @click="addToCollection">Добавить в комнату</v-btn>
    </div>

  </div>
</template>

<script>
import { onMounted, ref } from 'vue';

export default {
  props: {
    collection: Object,
    isActionAccept: Boolean,
  },
  methods: {
    addToCollection() {
      this.$emit('add-collection', this.collection)
    }
  },
  mounted() {
    const collectionInfo =  this.$refs.collectionInfo
    const collectionName =  this.$refs.collectionName
    
    collectionInfo.style.bottom = `-${collectionInfo.offsetHeight - collectionName.offsetHeight - 20}px`
  },
  emits: ['toggle-genre', 'add-collection'],
  setup(props, { emit }) {
    const collectionInfo = ref(null)
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
  /* margin: 0 45px; */
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
  bottom: 0%!important;
}

.collection-info {
  position: absolute;
  /* bottom: -40%; */
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

.plus-collect {
  width: 280px;
  height: 50px;
  padding: 15px 55px 15px 55px;
  margin-top: 20px;
  border-radius: 10px;
  background: rgba(20, 112, 239, 1);
  font-family: Montserrat;
  font-size: 16px;
  font-weight: 600;
  line-height: 19.5px;
  text-align: left;
  text-transform: none;
}
</style>