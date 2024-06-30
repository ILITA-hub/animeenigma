<template>
  <div v-if="collections" class="collection-card" @click="toggleDetails">
    <img class="collection-image" :src="collections.imgPath" :alt="`Изображение ${collections.nameRU}`">
    <div class="collection-info" :style="{ height: showDetails ? 'auto' : '82px' }">
      <div class="collection-title">{{ collections.nameRU }}</div>
      <div v-if="showDetails" class="additional-info">
        <v-autocomplete class="select"
          :items="collections.videos"
          item-text="name"
          item-value="id"
          label="Видео коллекции"
          density="compact"
          hide-no-data
          hide-selected
        ></v-autocomplete>
        <v-btn class="plus-collect" @click.stop="addToCollection">Добавить в коллекцию</v-btn>
      </div>
    </div>
  </div>
  <div v-else>Загрузка...</div>
</template>

<script>
export default {
  name: 'CollectionsComp',
  props: {
    collections: {
      type: Object,
      required: true
    }
  },
  data() {
    return {
      showDetails: false
    };
  },
  methods: {
    toggleDetails() {
      this.showDetails = !this.showDetails;
    },
    addToCollection() {
      this.$emit('addToCollection');
    }
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
  transition: all 0.3s;
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
  height: 82px;
}

.additional-info {
  padding: 6px;
}

.select {
  width: 280px;
  height: 40px;
  background: white;
  color: black;
  border-radius: 10px;
  overflow: hidden;
}

.plus-collect {
  width: 280px;
  height: 50px;
  padding: 15px 55px 15px 55px;
  border-radius: 10px;
  background: rgba(20, 112, 239, 1);
  font-family: Montserrat;
  font-size: 16px;
  font-weight: 600;
  text-align: left;
  text-transform: none;
}
</style>
