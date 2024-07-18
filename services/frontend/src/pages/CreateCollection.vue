<template>
  <v-container>
    <v-row justify="center">
      <div class="create-room">
        <a @click="goBack" class="back"><span class="mdi mdi-arrow-left"></span> Назад</a>
        <v-card class="form">
          <div class="text">Создать коллекцию</div>
          <v-text-field
            class="field"
            density="comfortable"
            variant="plain"
            placeholder="Название коллекции"
            v-model="collectionName"
          ></v-text-field>
          <v-text-field
            class="field"
            density="comfortable"
            variant="plain"
            placeholder="Описание коллекции"
            v-model="collectionDescription"
          ></v-text-field>
          <div class="collection">
            <div class="openings">Выбранные видео</div>
            <div class="selected-videos">
              <div v-for="video in selectedVideos" :key="video.id" class="selected-video">{{ video.name }}</div>
            </div>
            <v-btn class="collection-btn" @click="selectFromCollection">Выбрать из коллекции</v-btn>
          </div>
          <v-btn color="#1470EF" class="mb-4" @click="createCollection">Создать</v-btn>
        </v-card>
      </div>
    </v-row>
  </v-container>
</template>

<script>
import { useCollectionStore } from '@/stores/collectionStore';
import { computed, onMounted } from 'vue';
import { useRouter } from 'vue-router';

export default {
  setup() {
    const collectionStore = useCollectionStore();
    const router = useRouter();

    const collectionName = computed({
      get: () => collectionStore.collectionName,
      set: (value) => collectionStore.collectionName = value
    });
    const collectionDescription = computed({
      get: () => collectionStore.collectionDescription,
      set: (value) => collectionStore.collectionDescription = value
    });
    const selectedVideos = computed(() => collectionStore.selectedOpenings);

    const selectFromCollection = () => {
      collectionStore.saveToLocalStorage();
      router.push('/collect-select');
    };

    const createCollection = async () => {
      try {
        const createdCollection = await collectionStore.createCollection();
        if (createdCollection) {
          collectionStore.clearCollectionData();
          router.push('/user');
        }
      } catch (error) {
        console.error('ошибка создания коллекции:', error);
      }
    };

    onMounted(() => {
      collectionStore.loadFromLocalStorage();
    });

    const goBack = () => {
      router.push('/');
    };

    return {
      collectionName,
      collectionDescription,
      selectedVideos,
      selectFromCollection,
      createCollection,
      goBack
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
    top: -250px;
    left: 29px;
}
.back .mdi {
    color: rgba(51, 169, 255, 1);
    margin-right: 5px;
}

.create-room {
  display: flex;
  justify-content: center;
  align-items: center;
  min-height: 80vh;
}

.form {
  border-radius: 10px;
  box-shadow: 10px 2px 50px 0px rgba(0, 0, 0, 0.05);
  background: rgb(33, 35, 53);
  top: 10px;
  width: 422px;
  height: 475px;
  transform: translateX(-10%);
  padding: 20px; 
  max-height: 1000px;
}

.text {
  color: rgb(255, 255, 255);
  font-family: Montserrat;
  font-size: 22px;
  font-weight: 700;
  line-height: 27px;
  letter-spacing: 0%;
  text-align: left;
  padding-bottom: 15px;
  margin-top: 5px;

}
.field {
  color: rgb(194, 194, 194);
  font-family: Montserrat;
  font-size: 16px;
  font-weight: 500;
  letter-spacing: 0%;
  position: relative;
  top: 5px;
  background-color: rgba(255, 255, 255, 0.1);
  margin-bottom: 15px;
  border-radius: 10px;
  height: 50px;
  display: grid;
  padding-left: 15px;
}

.mb-4 {
  position: relative;
  width: 394px;
  height: 50px !important;
  font-size: 16px;
  display: flex;
  margin: 15px 55px 0px 0px;
  border-radius: 10px;
  text-transform: none;
  font-family: Montserrat;

}

.collection .collection-btn {
    background: rgba(51, 169, 255, 0.1);
    color: rgba(51, 169, 255, 1);
    text-transform: none;
  font-family: Montserrat;
  position: relative;
  width: 394px;
  height: 40px !important;
  font-size: 16px;
  display: flex;
  padding: 15px 55px 15px 55px;
  border-radius: 10px;
}

.openings {
  margin: 10px 0 10px 0;
  color: rgb(255, 255, 255);
  font-family: Montserrat;
  font-size: 16px;
}

.select {
  background: rgba(255, 255, 255, 0.1);
  border-radius: 10px;
  overflow: hidden;
  height: 50px;
  color: white;
  font-family: 'Montserrat'; 
}

.selected-video {
  background: rgba(255, 255, 255, 0.1);
  border-radius: 10px;
  padding: 8px;
  margin-bottom: 10px;
  color: white;
  font-family: Montserrat;
  font-size: 15px;
  font-weight: 400;
  line-height: 19.5px;
  text-align: left;
  white-space: nowrap;
  overflow: hidden; 
  text-overflow: ellipsis;
}

</style>