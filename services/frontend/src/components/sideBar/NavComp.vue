<template>
  <div>
    <div @click="routeTo(navOption)" v-for="navOption in navOptions" class="nav-option">
      <h2>
        {{navOption.name}}
      </h2>
    </div>
  </div>
  
</template>

<script>
export default {
  data() {
    return {
      navOptions: [
        {
          name: "Главная",
          path: '/',
        },
        {
          name: "Список комнат",
          path: '/',
        },
        {
          name: "Создание комнаты",
          path: '/createRoom',
          
        },
      ]
    }
  },
  methods: {
    changeContent(option) {
      
    },
    routeTo(navOption) {
      this.$emit('routeTo', false)
      this.$router.push(navOption.path)
    }
  },
}
</script>


<style scoped>
.default-text-field{
  /* background: white; */
  color: white;
}
.nav-option{
  color: white;
  cursor: pointer;
  transition: 0.3s ease all;
  border-radius: 5px;
  padding: 5px;
}
.nav-option:hover{
  background: rgb(118, 31, 133);
}
</style>
