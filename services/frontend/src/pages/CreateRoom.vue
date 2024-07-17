<template>
  <v-container>
    <v-row justify="center">
      <div class="create-room">
        <a @click="$router.push('/')" class="back"><span class="mdi mdi-arrow-left"></span> Назад</a>
        <v-card class="form">
          <div class="text">Создать комнату</div>
          <v-text-field
            class="field"
            density="comfortable"
            variant="plain"
            placeholder="Название лобби"
            v-model="roomName"
          ></v-text-field>
          <v-select
            class="select"
            variant="plain"
            density="comfortable"
            :items="playerCounts"
            label="Количество игроков"
            hide-details
          ></v-select>
          <div class="collection">
            <div class="openings">Опенинги</div>
            <v-btn class="collection-btn" @click="selectCollection">Выбрать из коллекции</v-btn>
          </div>
          <v-btn color="#1470EF" class="mb-4" @click="createRoom">Опубликовать</v-btn>
        </v-card>
      </div>
    </v-row>
  </v-container>
</template>

<script>
import axios from 'axios';

export default {
  data: () => ({
    roomName: '',
    playerCounts: ['2', '4', '6', '8', '10'],
    selectedPlayerCount: '',
    rangeOpenings: [
      { type: 'all', id: 0 },
      { type: 'collection', id: 1 },
      { type: 'anime', id: 1 },
    ],
  }),
  methods: {
  async createRoom() {
    const payload = {
      name: this.roomName,
      rangeOpenings: this.rangeOpenings,
      qtiUsersMax: this.selectedPlayerCount,
    };

    try {
      const response = await axios.post('https://animeenigma.ru/api/rooms', payload);
      console.log('Ответ от сервера:', response);
      const roomId = response.data;
      if (roomId) {
        const roomLink = `AnimeEnigma.ru/room/${roomId}`;
        console.log('Ссылка на созданную комнату:', roomLink);
      } else {
        console.error('ID комнаты не найден в ответе:', response.data);
      }
    } catch (error) {
      console.error('Ошибка при создании комнаты:', error);
    }
  },
  selectCollection() {
  }
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
    left: 77px;
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
margin-left: 50px;
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
    padding: 0 15px 0 15px;
}
</style>
