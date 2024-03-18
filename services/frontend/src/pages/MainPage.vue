<template>
  <div>
    <div class="banner">
      <v-carousel hide-delimiters
        height="440"
        cycle
        v-model="model">
        <template v-slot:prev="{ props }">
      <v-btn class="prev"
        @click="props.onClick"
      ><v-icon class="icon" :size="30">mdi-chevron-left</v-icon></v-btn>
    </template>
    <template v-slot:next="{ props }">
      <v-btn class="next"
        @click="props.onClick"
      ><v-icon class="icon" :size="30">mdi-chevron-right</v-icon></v-btn>
    </template>
        <v-carousel-item
          v-for="(slide, i) in slides"
          :key="i">
          <v-sheet
            :color="colors[i]"
            height="100%">
            <div class="d-flex fill-height justify-center align-center">
              <div class="text-h2">
                {{ slide }}
              </div>
            </div>
          </v-sheet>
        </v-carousel-item>
      </v-carousel>
    </div>
    <div class="custom-indicators">
      <span
        v-for="(slide, i) in slides"
        :key="'indicator-' + i"
        @click="setSlide(i)"
        class="indicator"
        :class="{ 'active': model === i }"
      ></span>
    </div>
    <div class="genre"> 
      <div class="genre-label">Популярные жанры</div>
      <v-card class="genre-card"
      @click="handleClick"
    v-ripple> 
  <v-img 
    class="genre-image"
    src="https://animego.org/upload/anime/images/6492bccecad5a252978706.jpg" 
    cover>
  </v-img> 
  <div class="v-card-title genre-title"> 
    Апокалипсис 
  </div>
  </v-card> 
  <v-card class="genre-card"  @click="handleClick"
    v-ripple> 
  <v-img 
    class="genre-image"
    src="https://animego.vip/uploads/posts/2023-05/916108_1685479742.jpg" 
    cover>
  </v-img> 
  <div class="v-card-title genre-title"> 
    Боевик 
  </div>
  </v-card> 
  <v-card class="genre-card"  @click="handleClick"
    v-ripple> 
  <v-img 
    class="genre-image"
    src="https://static.openni.ru/uploads/posts/2018-07/1531499111_1.jpg" 
    cover>
  </v-img> 
  <div class="v-card-title genre-title"> 
    Боевые искусства
  </div>
  </v-card> 
  <v-card class="genre-card"  @click="handleClick"
    v-ripple> 
  <v-img 
    class="genre-image"
    src="https://irecommend.ru/sites/default/files/product-images/1005814/dg4fxit1g70hVenYlIey8w.jpg" 
    cover>
  </v-img> 
  <div class="v-card-title genre-title"> 
    Детектив
  </div>
  </v-card> 
  <v-card class="genre-card"  @click="handleClick"
    v-ripple> 
  <v-img 
    class="genre-image"
    src="https://avatars.mds.yandex.net/get-kinopoisk-image/4303601/ea09d40a-9667-4d14-9de6-51bc2435a1a4/600x900" 
    cover>
  </v-img> 
  <div class="v-card-title genre-title"> 
    Комедия
  </div>
  </v-card> 
</div>  
  </div>
</template>

<script>
export default {
  data () {
    return {
      model: 0,
      colors: [
        'indigo',
        'warning',
        'pink darken-2',
      ],
      slides: [
        'Здесь могла бы быть ваша реклама',
        'И здесь',
        'И тут тоже'
      ],
    }
  },
  methods: {
    setSlide(index) {
      this.model = index;
    }
  }
}
</script>

<style scoped>
.banner {
  position: relative;
}
.v-carousel {
  max-width: 1695px;
  top: 40px;
  margin: 0 auto;
  border-radius: 10px;
}
.custom-indicators {
  display: flex;
  justify-content: center;
  margin-top: 64px;
}
.indicator {
  max-width: 1695px;
  border-radius: 50%;
  width: 15px;
  height: 15px;
  background-color: rgba(255, 255, 255, 0.5);
  cursor: pointer;
  margin-right: 13px;
}
.indicator:last-child {
  margin-right: 0;
}
.indicator.active {
  background-color: #FFF; 
}

.genre { 
  position: relative; 
  display: flex; 
  top: 100px; 
  padding-bottom: 100px; 
  justify-content: center;  
} 

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


.genre-label {
  position: absolute;
  top: -60px;
  left: 65px;
  font-size: 28px;
  color: #ffffff;
  font-family: Montserrat;
  font-weight: bold;
}

.v-card-title {
  height: 50px;
  color: white;
  font-size: 16px;
  font-family: Montserrat;
}

.v-btn.prev, .v-btn.next { 
  margin: 0;
  position: absolute;
  top: 50%;
  border-radius: 15px; 
  background-color: rgba(255, 255, 255, 0.2); 
  backdrop-filter: blur(2px); 
  transform: translateY(-50%);
}

.next {
 right: 10px;
 height: 50px; 
  width: 50px; 
}

.prev {
  left: 10px;
  height: 50px; 
  width: 50px; 
}

.icon {
  color: #FFF;
}
</style>
