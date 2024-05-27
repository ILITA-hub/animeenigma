<template>
  <div class="phce card" :class="cardClass">
    <div class="content genre-card" @pointermove="setPositions">
      <img class="genre-image" :src="genre.image" :alt="`Изображение ${genre.title}`">
      <div class="genre-title">{{ genre.title }}</div>
    </div>
  </div>
</template>

<script>
export default {
  name: 'GenreCard',
  props: {
    genre: Object,
    cardClass: String 
  },
  methods: {
    setPositions(event) {
      const { currentTarget: el, layerX: x, layerY: y } = event;
      const { width: w, height: h } = el.getBoundingClientRect();
      el.style.setProperty("--posX", this.mapPositions(x, [0, w]));
      el.style.setProperty("--posY", this.mapPositions(y, [0, h]));   
    },
    mapPositions(value, from, to=[-1,1], decimals=2) {
      const newValue = (value - from[0]) * (to[1] - to[0]) / (from[1] - from[0]) + to[0];
      const val = Math.min(Math.max(newValue, to[0]), to[1]);
      return decimals && decimals > 0 ? Number(Math.round(val+'e'+decimals)+'e-'+decimals) : val;
    }
  }
}
</script>

<style scoped>
.phce {
  perspective: 1000px;
  position: relative;
}

.phce .content {
  position: relative;
  overflow: hidden;
  transform-style: preserve-3d;
  transition: transform 0.5s ease, transform 0.5s ease-out; 
}

.phce .content::before, 
.phce .content::after {
  content: "";
  position: absolute;
  inset: 0;
  z-index: -1;
  transform-style: preserve-3d;
}

.phce .content::before {
  background-image: var(--bg-image);
  background-size: cover;
  transition: transform 0.5s ease, transform 0.5s ease-out; 
}

.phce .content::after { 
  background: var(--bg-overlay, none);
}

.phce:hover > .content::before {
  transform: scale(1.33) translateX(calc(-12.5% * var(--posX,0))) translateY(calc(-12.5% * var(--posY,0)));
}

.phce:hover > .content { 
  transform: rotateX(calc(22.5deg * var(--posY,0))) rotateY(calc(-22.5deg * var(--posX,0)));
}

.phce:not(:hover) > .content, 
.phce:not(:hover) > .content::before {
  transition: transform 0.5s ease, transform 0.5s ease-out; 
}

.genre-card {
  display: grid; 
  place-items: center;
  min-height: 16rem;
  padding: 2rem;
  border-radius: 10px;
  font-family: system-ui, sans-serif;
  font-size: 2rem;
  color: white;
  --bg-overlay: rgb(0 0 0 / .5);
  cursor: pointer;
  width: 252px; 
  position: relative; 
  height: 330px; 
  margin: 0 55px; 
  overflow: hidden;
} 

.genre-image {
  width: 100%;
  height: 100%;
  position: absolute;
  top: 0;
  left: 0;
  object-fit: cover;
  border-radius: 10px;
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
</style>
