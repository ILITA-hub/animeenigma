<template>   
    <div class="room-card" @mouseover="showDetails = true" @mouseleave="showDetails = false">    
        <img class="room-image" :src="room.image" :alt="`Изображение ${room.title}`">    
        <div class="room-info" :class="{ active: showDetails }" :style="{height: showDetails ? 'auto' : '82px'}">     
            <div class="room-title">{{ room.title }}</div>     
            <div v-if="showDetails" class="additional-info">    
                <div v-for="genre in room.genres" :key="genre" class="genres">{{ genre }}</div>      
                <v-btn class="enjoy">Присоединиться</v-btn>    
            </div>    
        </div>     
    </div>   
</template>

<script>    
    export default {    
        name: 'RoomComp',    
        props: {    
            room: Object 
        },    
        data() {    
            return {   
                showDetails: false,   
            };   
        },      
    }    
</script>


      
      <style scoped>
      .result {  
  font-family: Montserrat;  
  font-size: 28px;  
  font-weight: 700;  
  line-height: 34.13px;  
  text-align: left;  
  color: white;  
  left: 500px;
  top: 50px; 
  position: relative;
}  


.room-card {  
    cursor: pointer; 
    width: 320px;  
    position: relative;  
    height: 445px;  
    border-radius: 10px;  
    overflow: hidden; 
    transition: all 0.3s;
    
  }  
  
  .room-card:hover .room-info {
    transform: translateY(0);
  }
  
  .room-image { 
    width: 100%; 
    height: 100%; 
    position: absolute; 
    top: 0; 
    left: 0; 
    border-radius: 10px;  
  } 
  
  .room-info {   
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
  transition: all 0.3s; 
  transform: translateY(0); 
  height: 82px;
}

  
  .active {
    transform: translateY(-50px);
  }
  
  .additional-info {
    padding: 6px;
  }
  
  .additional-info div {
    margin: 5px 0;
  }
  
  .genres {
    background-color: white;
    color: black;
    border-radius: 10px;
    font-family: Montserrat;
    font-size: 12px;
    font-weight: 400;
    width: auto;
    height: 35px;
    text-align: center;
    position: relative;
    display: inline-block;
    padding: 10px;
  }

  .enjoy {
    width: 280px;
    height: 50px;
    padding: 15px 55px 15px 55px;
    border-radius: 10px;
    opacity: 0px;
    background: rgba(20, 112, 239, 1);
    font-family: Montserrat;
    font-size: 16px;
    font-weight: 600;
    line-height: 19.5px;
    text-align: left;
    font-family: Montserrat;
    text-transform: none;
  }
  
      </style>
  
  
      