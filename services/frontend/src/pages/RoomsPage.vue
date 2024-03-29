<template>
  <div class="banner">
    <img class="picture"src="src/assets/img/picture.png"></div>
  <div class="filter">
    <div class="filter-room">
      <FilterRoom/>
    </div>
    <div class="filter-anime">
      <FilterAnime/>
    </div>
  </div>
  </template>
  
  <script>
import FilterRoom from "@/components/FilterComp/FilterRoom.vue";
import FilterAnime from "@/components/FilterComp/FilterAnime.vue";


export default {
  components: {
    FilterRoom,
    FilterAnime,
  },
};
</script>


<style>
.filter {
  display: grid;
  width: 400px;
  margin: 0 30px 0 30px;
}

.banner {
  display: grid;
  position: relative;
  overflow: hidden;
  height: 300px;
  margin: 30px 30px 20px 30px;
}

.banner .picture {
  display: grid;
  border-radius: 10px;
  position: relative;
  top: 50%;
  left: 50%;
  transform:translate(-50%,-50%);
  width: 1697px;
  height: 300px;
  object-fit: cover; 
}
</style>
  