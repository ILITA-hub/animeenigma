<template>  
  <div class="room-component" :style="{ backgroundImage: 'url(' + room.image + ')', backgroundSize: 'cover' }">  
    <div class="gradient"></div>
    <div class="room-content">  
      <div class="genres">
  <div v-for="(genre, index) in room.genres" :key="genre" class="genre">{{ genre }}</div>
</div>
      <h2>{{ room.title }}</h2>  
      <div class="age-players">
      <div v-for="(age, index) in room.age" :key="age" class="age">{{ age }}</div>
      <div v-for="(players, index) in room.players" :key="players" class="players">{{ players }}</div>
    </div>
      <v-btn @click="$router.push({name: 'room', params: { uniqUrl: '1ae08' }})" text class="button">Присоединиться</v-btn> 
    </div>  
  </div>  
</template> 

<script> 
export default { 
  name: 'RoomComp', 
  props: { 
    room: { 
      type: Object, 
      required: true 
    } 
  } 
} 
</script>

    
    <style scoped>

    .room-component {
      display: flex;
      height: 420px;
      width: 100%;
      background-position: 25% 45%; 
      position: relative;
      overflow: hidden;
      border-radius: 10px; 
    }
    
    .gradient {
      position: absolute;
      top: 0;
      left: 0;
      width: 100%;
      height: 100%;
      background: linear-gradient(to right, rgba(0,0,0,1), rgba(0,0,0,0));
}

    .genres {
      color: #fff;
      display: flex;
      gap: 10px;
      font-family: Montserrat;
      font-size: 16px;
      font-weight: 500;
      line-height: 19.5px;
      text-align: left;

    }

    .button {
      margin-top: 10px;
      width:315px;
      height:50px;
      border-radius: 10px;
      background: rgba(20, 112, 239, 1);
      font-family: Montserrat;
      font-size: 16px;
      font-weight: 600;
      line-height: 19.5px;
      text-align: left;
      color: #fff;
      text-transform: none;
    }

    .room-content {
    position: absolute;
    display: flex;
    flex-direction: column;
    margin: 65px;
    top: 100px;
  }

    .room-content h2 {
      color: #fff;
      font-family: Montserrat;
      font-size: 45px;
      font-weight: 700;
      line-height: 54.86px;
      text-align: left;

    }

    .genre {
      display: flex;
      align-items: center;
    }

    .genre:not(:last-child):after {
      content: "\00B7";
      margin-left: 10px; 
    }

    .age-players {
      display: flex;
    }
    .age {
    background: rgba(253, 42, 49, 1);
    font-family: Montserrat;
    color: #fff;
    font-size: 14px;
    font-weight: 700;
    line-height: 17.07px;
    width: 45px;
    height: 37px;
    border-radius: 10px;
    text-align: center;
    margin: 10px 10px 10px 0px;
    display: flex;
    justify-content: center;
    align-items: center;
}

    .players {
      background: rgba(255, 193, 0, 1);
      width: 124px;
      height: 37px;
      border-radius: 10px;
      text-align: center;
      align-items: center;
      display: flex;
      justify-content: center;
      color: #fff;
      font-family: Montserrat;
      font-size: 14px;
      font-weight: 700;
      line-height: 17.07px;
      margin: 10px 10px 10px 0px;
    }
    </style>


    