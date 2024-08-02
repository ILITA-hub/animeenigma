<template>
  <div class="filter-anime">
    <v-container>
      <v-menu
        v-model="menu"
        :close-on-content-click="false"
        location="bottom"
      >
        <template v-slot:activator="{ props }">
          <v-btn
            class="btn-room"
            text
            v-bind="props"
          >
            Фильтры аниме
            <v-icon class="icon" :size="28">{{ menu ? 'mdi-chevron-up' : 'mdi-chevron-down' }}</v-icon>
          </v-btn>
        </template>
        <v-card class="card-list" v-if="genres.length">
          <v-select
            class="select"
            v-model="selectedGenres"
            :items="genres"
            item-value="id"
            item-title="nameRu"
            label="Жанр"
            multiple
          >
          <template v-slot:selection="{ item, index }">
              <div v-if="index < 3">
                <span>{{ item.title }},</span>
              </div>
              <span
                v-if="index === 3"
                class="text-grey text-caption align-self-center"
              >
                (+{{ selectedGenres.length - 3 }} others)
              </span>
            </template></v-select>
          <v-select
            class="select"
            v-model="selectedYears"
            :items="years"
            label="Год выпуска"
            multiple>
            <template v-slot:selection="{ item, index }">
              <div v-if="index < 2">
                <span>{{ item.title }},</span>
              </div>
              <span
                v-if="index === 2"
                class="text-grey text-caption align-self-center"
              >
                (+{{ selectedYears.length - 2 }} others)
              </span>
            </template>
          </v-select>
        </v-card>
      </v-menu>
    </v-container>
  </div>
</template>

<script>
import { useAnimeFilterStore } from '@/stores/animeFilterStore';
import { computed, onMounted } from 'vue';

export default {
  data() {
    return {
      menu: false,
      selectedGenres: [],
      selectedYears: [],
    };
  },
  setup() {
    const store = useAnimeFilterStore();

    onMounted(async () => {
      await store.loadGenres();
      await store.loadYears();
    });

    const genres = computed(() => store.genres);
    const years = computed(() => store.years);

    return {
      store,
      genres,
      years,
    };
  },
};
</script>

<style scoped>
.btn-room {
  font-family: 'Montserrat';
  color: aliceblue;
  font-size: 28px !important;
  background: none;
  text-transform: none;
  font-weight: 700;
  line-height: 34.13px;
}

.btn-room .icon {
  background: rgba(51, 169, 255, 1);
  border-radius: 50%;
  margin-left: 20px;
}

.card-list {
  top: 10px;
  background: rgba(33, 35, 53, 1) !important;
}

.select {
  width: 89%;
  background: rgba(255, 255, 255, 0.1);
  border-radius: 10px;
  margin: 10px 10px 10px 20px;
  overflow: hidden;
  height: 50px;
  color: white;
  font-family: 'Montserrat';
  line-height: 19.5px;
  text-align: left;
}

.v-menu > .v-overlay__content > .v-card,
.v-menu > .v-overlay__content > .v-sheet,
.v-menu > .v-overlay__content > .v-list {
  width: 100%;
  height: 135px !important;
  background: rgba(255, 255, 255, 0.1);
  gap: 0px;
  border-radius: 10px !important;
}
</style>
