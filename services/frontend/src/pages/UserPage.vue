<template>
  <div>
    <div class="user-info">
      <div class="user-banner">
        <v-card class="main-banner">
          <div class="avatar-container">
            <v-avatar class="avatar" :image="avatar" size="140"></v-avatar>
          </div>
          <v-img
            class="bg-picture"
            height="211"
            src="src/assets/img/banner.png"
            cover>
          </v-img>
          <v-fab icon="$vuetify"></v-fab>
          <div class="user-header">
            <div class="text-container">
              <v-card-title class="user-title">
                {{ registrationUsername }} 
              </v-card-title>
              <v-card-text class="subtitle">
                <div>Зарегистрирован на сайте с 2024 года</div>
              </v-card-text>
            </div>
          </div>
        </v-card>
        <v-card class="player-stats-banner">
          <v-card-title class="stats-title">Статистика игрока</v-card-title>
          <div class="stats-windows">
            <div class="stat-window" v-for="(stat, index) in stats" :key="index">
              <div class="stat-text">{{ stat }}</div>
            </div>
          </div>
        </v-card>
      </div>
    </div>
    <div class="user-collections">
      <v-card class="collections-banner">
        <v-card-title class="collections-title">Мои коллекции</v-card-title>
        <div class="collections-list">
          <div class="collection-item" v-for="(collection, index) in collections" :key="index">
            <div class="collection-name">{{ collection.name }}</div>
            <div class="collection-description">{{ collection.description }}</div>
          </div>
          <div v-if="collections.length === 0">Нет коллекций для отображения.</div>
        </div>
      </v-card>
    </div>
  </div>
</template>

<script>
import axios from 'axios';

export default {
  name: 'UserPage',
  data() {
    return {
      stats: ['Очков', 'Викторин создано', 'Викторин пройдено', 'Коллекций создано'],
      registrationUsername: '',
      avatar: '',
      collections: [],
    };
  },
  mounted() {
    this.registrationUsername = localStorage.getItem('registrationUsername') || '';
    this.avatar = localStorage.getItem('avatar') || '';
    this.fetchUserCollections();
  },
  methods: {
    async fetchUserCollections() {
      const token = localStorage.getItem('authToken') || sessionStorage.getItem('authToken');

      if (!token) {
        console.error('Нет токена аутентификации');
        return;
      }

      try {
        const response = await axios.get('https://animeenigma.ru/api/animeCollections', {
          headers: {
            Authorization: `Bearer ${token}`
          }
        });

        this.collections = response.data;
        console.log('User collections:', this.collections);
      } catch (error) {
        console.error('Error fetching collections:', error.response.data);
      }
    },
  },
};
</script>

<style scoped>
.user-banner {
  margin: 0px 65px 0px 65px;
  height: 300px;
  top: 40px;
  position: relative;
  display: flex;
  gap: 20px;
}

.main-banner {
  width: 1200px;
  border-radius: 10px;
}

.avatar-container {
  position: absolute;
  top: 130px; 
  left: 10%;
  transform: translateX(-50%);
  z-index: 1;
  display: flex;
  justify-content: center;
  align-items: center;
  width: 140px;
  height: 140px;
  background-color: rgba(33, 35, 53, 1);
  border-radius: 50%;
}

.avatar {
  border: 10px solid rgba(33, 35, 53, 1); 
  border-radius: 50%;
}

.user-header {
  display: flex;
  align-items: center;
  height: 90px; 
  padding: 16px;
  background-color: rgba(33, 35, 53, 1); 
  overflow: hidden;
  position: relative;
}

.text-container {
  font-family: Montserrat;
  color: white;
  margin-left: 150px;
  display: flex;
  flex-direction: column;
  justify-content: center;
  height: 100%;
}

.user-title {
  margin: 0;
  font-size: 16px;
  font-weight: 600;
}

.subtitle {
  font-size: 12px;
  font-weight: 400;
}

.v-card-text {
  margin-top: -10px;
}

.player-stats-banner {
  max-width: 350px;
  padding: 13px 0px 0px 23px;
  background-color: rgba(33, 35, 53, 1);
  border-radius: 10px;
  display: flex;
  flex-direction: column;
}

.stats-title {
  font-family: Montserrat;
  font-size: 16px;
  font-weight: 600;
  line-height: 19.5px;
  text-align: left;
  color: white;
  padding-left: 0px;
  padding-bottom: 15px;
}

.stats-windows {
  display: flex;
  flex-wrap: wrap;
  gap: 15px;
}

.stat-window {
  width: 144px;
  height: 101px;
  background-color: rgba(255, 255, 255, 0.1);
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 10px;
  transition: background-color 0.3s;
  text-align: center;
}

.stat-window:hover {
  background-color: rgba(20, 112, 239, 1);
}

.stat-text {
  font-family: Montserrat;
  font-size: 12px;
  font-weight: 500;
  line-height: 14.63px;
  color: white;
}

.user-collections {
  margin: 20px 65px;
}

.collections-banner {
  padding: 20px;
  background-color: rgba(33, 35, 53, 1);
  border-radius: 10px;
}

.collections-title {
  font-family: Montserrat;
  font-size: 16px;
  font-weight: 600;
  line-height: 19.5px;
  text-align: left;
  color: white;
}

.collections-list {
  display: flex;
  flex-wrap: wrap;
  gap: 20px;
}

.collection-item {
  background-color: rgba(255, 255, 255, 0.1);
  padding: 10px;
  border-radius: 10px;
  width: 200px;
  height: 100px;
  display: flex;
  flex-direction: column;
  justify-content: center;
  align-items: center;
  text-align: center;
}

.collection-name {
  font-family: Montserrat;
  font-size: 14px;
  font-weight: 600;
  color: white;
}

.collection-description {
  font-family: Montserrat;
  font-size: 12px;
  font-weight: 400;
  color: white;
  margin-top: 5px;
}
</style>
