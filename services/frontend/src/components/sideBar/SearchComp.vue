<template>
  <div>
    <v-text-field class="default-text-field" label="Поиск"></v-text-field>
    <v-text-field class="default-text-field" label="Код комнаты"></v-text-field>
  </div>
  
</template>

<script>
export default {
  data() {
    return {
      drawer: false
    }
  },
  methods: {
    changeContent(option) {
      
    }
  },
}
</script>


<style scoped>
.default-text-field{
  /* background: white; */
  color: white;
}
</style>
