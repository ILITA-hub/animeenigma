<template>
    <v-container fluid class="pa-0 animated-background">
      <div class="circle circle1" style="background-color: red;"></div>
      <div class="circle circle2" style="background-color: blue;"></div>
      <div class="circle circle3" style="background-color: "></div>
      <div class="circle circle4" style="background-color: "></div>
    </v-container>
  </template>
  
  <script>
  export default {
    name: 'AnimatedBackground',
  }
  </script>
  
  <style scoped>
  .animated-background {
    position: relative;
    overflow: hidden;
    width: 100%;
    height: 100vh;
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
  right: 0;
  width: 450px;
  height: 450px;
  animation-name: moveAround1, colorChange1;
}

.circle2 {
position: absolute;
  left: 0;
  width: 450px;
  height: 450px;
  animation-name: moveAround2, colorChange2;
}


@keyframes moveAround1 {
  0% { transform: translate(0%, 0%); } 
  25% { transform: translate(-100%, 50%); } 
  50% { transform: translate(0%, 100%); } 
  75% { transform: translate(-50%, 50%); } 
  100% { transform: translate(0%, 0%); } 
}

@keyframes moveAround2 {
  0% { transform: translate(0%, 0%); }
  25% { transform: translate(50%, 50%); }
  50% { transform: translate(-50%, -50%); }
  75% { transform: translate(70%, -70%); }
  100% { transform: translate(0%, 0%); }
}

@keyframes colorChange1 {
  0% { background-color: red; }
  25% { background-color: yellow; }
  50% { background-color: purple; }
  75% { background-color: cyan; }
  100% { background-color: red; }
}
@keyframes colorChange2 {
  0% { background-color: blue; }
  25% { background-color: yellow; }
  50% { background-color: purple; }
  75% { background-color: cyan; }
  100% { background-color: blue; }
}

  </style>
  