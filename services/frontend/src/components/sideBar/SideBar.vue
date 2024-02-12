<template>
  <div ref="sidebar" class="sidebar-container">

    <div class="menu-container">

      <div v-for="(option, index) in options" class="option-container mt-2">
        <v-icon @click="option.fn(option.fnArgs)"
          :class="`sidebar-option-icon ${optionNum === index ? 'sidebar-option-active' : ''}`">
          {{option.name}}
        </v-icon>
      </div>
    </div>
    
    <transition name="fade" mode="in-out">
      <div ref="sidebar_content" v-if="opened" class="sidebar-content">
        <component @routeTo="closeMenuOptions()" :is="comp"></component>
      </div>
    </transition>
      


    <div ref="layaut" @click="closeMenuOptions()" class="layaut d-none"></div>
    
  </div>
</template>

<script>
import SearchComp from "@/components/sideBar/SearchComp.vue";
import NavComp from "@/components/sideBar/NavComp.vue";

import sleep from "@/common/sleep.js";

import {shallowRef} from "vue";

export default {
  data() {
    return {
      drawer: false,
      opened: false,
      optionNum: null,
      comp: shallowRef(SearchComp),
      options: [
        {
          name: 'mdi-magnify',
          fn: this.changeContent,
          fnArgs: 0
        },
        {
          name: 'mdi-home-variant',
          fn: this.$router.push,
          fnArgs: {name: 'HomeView'}
        },
        {
          name: 'mdi-account-multiple-plus',
          fn: this.$router.push,
          fnArgs: {name: 'CreateRoomView'}
        },
        {
          name: 'mdi-folder-plus',
          fn: this.$router.push,
          fnArgs: {name: 'HomeView'}
        },

        
      ]
    }
  },

  methods: {
    async changeContent(optionNum) {
      if (this.opened && optionNum !== this.optionNum) {
        this.opened = false
        this.optionNum = optionNum
        await sleep(10)
        if (optionNum === 0) {
          this.comp = shallowRef(SearchComp)
        }
        if (optionNum === 1) {
          this.comp = shallowRef(NavComp)
        }
        this.opened = true
      }

      if (!this.opened) {
        this.optionNum = optionNum
        this.$refs.sidebar.classList.add('sidebar-container-open')
        
        await sleep(100)
        if (optionNum === 0) {
          this.comp = shallowRef(SearchComp)
        }
        if (optionNum === 1) {
          this.comp = shallowRef(NavComp)
        }
        this.$refs.layaut.classList.remove('d-none')
        this.opened = true
      }
    },

    async closeMenuOptions() {
      this.opened = false
      this.optionNum = null
      await sleep(100)
      
      this.$refs.sidebar.classList.remove('sidebar-container-open')
      this.$refs.layaut.classList.add('d-none')
    },
  },
}
</script>


<style scoped>
.sidebar-container{
  display: flex;
  height: 100%;
  width: 50px;
  position: absolute;
  background-color: white;
  align-items: center;
  transition: all .1s ease-out;
  background-color: rgba(78,31,86);
  z-index: 100;
}

.layaut{
  position: absolute;
  width: 100vw;
  height: 100vh;
  z-index: 10;
  background-color: black;
  opacity: 0.3 
}
.sidebar-container-open{
  width: 350px;
  background-color: rgb(56, 21, 62);
}

.menu-container {
  height: 100%;
  width: 50px;
  display: flex;
  flex-direction: column;
  align-items: center;
  background-color: rgb(78,31,86);
  box-shadow: 1px 0px 3px rgb(63, 24, 69);
  position: relative;
  z-index: 100;
}



.sidebar-content{
  box-sizing: border-box;
  display: flex;
  flex-direction: column;
  width: 85%;
  height: 100%; 
  padding: 10px;
  justify-content: flex-start;
  position: relative;
  z-index: 100;
  background-color: rgba(39,15,43);
}

.option-container{
  display: flex;
  align-items: center;
  justify-content: center;
  width: 40px;
  height: 40px;
  object-fit: cover;
  transition: all 0.1s ease-out;

}
.option-container:hover{
}

.sidebar-option-icon{
  width: 100%;
  height: 100%;
  border-radius: 5px;
  color: white;
}
.sidebar-option-icon:hover{
  transition: all 0.1s ease-out;
  background: rgba(179,42,201, 0.70);
}
.sidebar-option-active {
  background: rgba(179,42,201, 0.70);
}

.arrowClose{
  width: 100%;
  height: 100%;
}
</style>
