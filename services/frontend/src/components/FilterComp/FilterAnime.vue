<template>
  <div ref="filterAnime" class="filter-anime">
    <v-card class="card-list" v-if="genres.length">
      <template v-slot:title>
          <span class="filter-title">Фильтры аниме</span>
        </template>
      <v-select class="select" v-model="selectedGenres" :items="genres" item-value="id" item-title="nameRu" label="Жанр"
        multiple>
        <template v-slot:selection="{ item, index }">
          <div v-if="index < 3">
            <span>{{ item.title }},</span>
          </div>
          <span v-if="index === 3" class="text-grey text-caption align-self-center">
            (+{{ selectedGenres.length - 3 }} others)
          </span>
        </template></v-select>
      <v-select class="select" v-model="selectedYears" :items="years" label="Год выпуска" multiple>
        <template v-slot:selection="{ item, index }">
          <div v-if="index < 2">
            <span>{{ item.title }},</span>
          </div>
          <span v-if="index === 2" class="text-grey text-caption align-self-center">
            (+{{ selectedYears.length - 2 }} others)
          </span>
        </template>
      </v-select>
    </v-card>
  </div>
</template>

<script>
import { useAnimeStore } from '@/stores/animeStore';
import { computed, watch, onMounted, onUnmounted, ref } from 'vue';

export default {
  methods: {

  },
  mounted() {

  },

  setup() {
    const animeStore = useAnimeStore();
    const menu = ref(false);
    const selectedGenres = ref([]);
    const selectedYears = ref([]);
    const filterAnime = ref()

    onMounted(async () => {
      await animeStore.loadGenres();
      await animeStore.loadYears();
      window.addEventListener('scroll', function () {
        toFixElement(filterAnime.value);
      });
    });

    onUnmounted(()=>{
      window.removeEventListener('scroll', toFixElement)
    })

    function toFixElement(elem) {
      if (window.pageYOffset > 300) {
        elem.style.transform = `translateY(${window.pageYOffset - 300}px)`;
      } else {
        elem.style.transform = `translateY(0)`;
      }
    }

    const genres = computed(() => animeStore.genres);
    const years = computed(() => animeStore.years);

    watch(
      [selectedGenres, selectedYears],
      ([newGenres, newYears]) => {
        animeStore.setGenres(newGenres);
        animeStore.setYears(newYears);
        animeStore.animeRequest();
      }
    );

    return {
      menu,
      genres,
      years,
      selectedGenres,
      selectedYears,
      toFixElement,
      filterAnime
    };
  },
};
</script>

<style scoped>
.filter-anime {
  position: relative;
  width: 320px;
  padding: 10px 0;
  transition: transform 0.5s ease-in-out;
}

.filter-title{
  color: #fff;
  font-family: Montserrat;
}
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
  border-radius: 10px;
}

.select {
  width: 94%;
  background: rgba(255, 255, 255, 0.1);
  border-radius: 10px;
  margin: 10px 10px 10px 10px;
  overflow: hidden;
  height: 50px;
  color: white;
  font-family: 'Montserrat';
  line-height: 19.5px;
  text-align: left;
}
</style>
