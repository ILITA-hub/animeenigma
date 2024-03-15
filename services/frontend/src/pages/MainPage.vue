<template>
  <div class="banner">
  <v-carousel 
    height="440"
    cycle>
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
<div class="genre">
  <v-card
    class="mx-auto"
    image="https://cdn.vuetifyjs.com/images/cards/docks.jpg"
    theme="dark"
    title="Card title"
  ></v-card>
  <v-card
    class="mx-auto"
    image="https://cdn.vuetifyjs.com/images/cards/docks.jpg"
    theme="dark"
    title="Card title"
  ></v-card>
  <v-card
    class="mx-auto"
    image="https://cdn.vuetifyjs.com/images/cards/docks.jpg"
    theme="dark"
    title="Card title"
  ></v-card>
  <v-card
    class="mx-auto"
    image="https://cdn.vuetifyjs.com/images/cards/docks.jpg"
    theme="dark"
    title="Card title"
  ></v-card>
  <v-card
    class="mx-auto"
    image="https://cdn.vuetifyjs.com/images/cards/docks.jpg"
    theme="dark"
    title="Card title"
  ></v-card>
  <v-card
    class="mx-auto"
    image="https://cdn.vuetifyjs.com/images/cards/docks.jpg"
    theme="dark"
    title="Card title"
  ></v-card>
</div>
</template>

<script>
  export default {
    data () {
      return {
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
  }
</script>
<style scoped>

.v-btn--icon {
    border-radius: 30%;
    height: 50px;
    width: 50px;
    background: rgba(255, 255, 255, 0.2);
}
.v-btn--icon .v-icon {
    color: aliceblue;

}
.v-carousel {
  max-width: 96%;
  top: 40px;
  margin: 0 auto;
  border-radius: 10px;
}

.genre{
  position: relative;
  display: flex;
  top: 100px;
  padding-bottom: 100px;
}
.genre .v-card {
  width: 252px;
  position: relative;
  height: 330px;
  border-radius: 10px;
}
</style>
