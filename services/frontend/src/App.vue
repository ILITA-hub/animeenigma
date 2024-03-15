<template>
  <v-app>
    <HeaderApp/>
    <v-container fluid class="animated-background">
    <div class="circle circle1" style="background-color: #FF3333;"></div>
      <div class="circle circle2" style="background-color: #EFFF33;;"></div>
      <div class="circle circle3" style="background-color: #CA33FF;"></div>
      <div class="circle circle4" style="background-color: #33A9FF;"></div>
    <v-main>
      <MainPage/>
    </v-main>
  </v-container>
  <FooterApp/>
  </v-app>
</template>

<script>
import HeaderApp from './components/HeaderApp.vue'
import FooterApp from './components/FooterApp.vue'
import MainPage from './pages/MainPage.vue'

export default {
  name: 'App',

  components: {
    HeaderApp,
    FooterApp,
    MainPage,
  },

  data: () => ({
    //
  }),
}
</script>

<style>
@font-face {
    font-family: Montserrat;
    src: url('src/assets/fonts/Montserrat.ttf');
  }

  .animated-background {
    position: relative;
    overflow: hidden;
    width: 100%;
    min-height: 100vh;
    background-color: #090B1F;
  }
  
.circle {
  position: absolute;
  animation: moveAround 20s infinite;
  border-radius: 50%;
  filter: blur(95px); 
  animation-timing-function: linear;
  opacity: 0.2; 
} 


.circle1 {
  position: absolute;
  right: 50%;
  width: 450px;
  height: 450px;
  animation-name: moveAround1, colorChange1;
}

.circle2 {
position: absolute;
left: 100%;
  width: 450px;
  height: 450px;
  animation-name: moveAround2, colorChange2;
}

.circle3 {
position: absolute;
left: 80%;
  width: 450px;
  height: 450px;
  animation-name: moveAround3, colorChange3;
}

.circle4 {
position: absolute;
right: 50%;
  width: 450px;
  height: 450px;
  animation-name: moveAround4, colorChange4;
}


@keyframes moveAround1 {
  0%, 100% { bottom: -40%; left: 10%; }
  50% { bottom: -10%; left: -10%; }
}

@keyframes moveAround2 {
  0%, 100% { top: -10%; left: 90%; }
  50% { top: 10%; left: 90%; }
}

@keyframes moveAround3 {
  0%, 100% { bottom: -40%; left: 90%; }
  50% { bottom: -10%; left: 50%; }
}

@keyframes moveAround4 {
  0%, 100% { bottom: 0; left: -10%; }
  50% { bottom: 90%; left: 5%; }
}


@keyframes colorChange1 {
  0% { background-color:#FF3333; }
  25% { background-color: #EFFF33; }
  50% { background-color: #CA33FF; }
  75% { background-color: #33A9FF; }
  100% { background-color: #FF3333; }
}
@keyframes colorChange2 {
  0% { background-color:#EFFF33;; }
  25% { background-color: #FF3333; }
  50% { background-color: #CA33FF; }
  75% { background-color: #33A9FF; }
  100% { background-color: #EFFF33; }
}

@keyframes colorChange3 {
  0% { background-color:  #CA33FF; }
  25% { background-color: #FF3333; }
  50% { background-color: #33A9FF; }
  75% { background-color: #EFFF33; }
  100% { background-color:  #CA33FF; }
}
@keyframes colorChange4 {
  0% { background-color: #33A9FF; }
  25% { background-color: #CA33FF; }
  50% { background-color: #FF3333; }
  75% { background-color: #EFFF33; }
  100% { background-color: #33A9FF; }
}
</style>