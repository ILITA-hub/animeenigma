<template>
  <v-footer class="footer">
    <v-container fluid>
      <v-row justify="space-between" align="center">
        <v-col cols="auto">
          <v-img src="src/assets/img/logo.png" class="logo"></v-img>
        </v-col>
        <v-col cols="auto"> 
          <div class="d-flex align-center">
            <div class="d-flex social-icons"> 
              <v-btn icon v-for="item in socialButtons" 
                :key="item.icon" 
                :href="item.link" 
                target="_blank" 
                :size="31"> 
                <v-icon>{{ item.icon }}</v-icon> 
              </v-btn> 
            </div> 
          <v-btn class="button">Заказать рекламу</v-btn>
          </div>
        </v-col>
      </v-row>
    </v-container>
  </v-footer>
</template>

<script>
export default {
  name: 'FooterApp',
  data: () => ({
    socialButtons: [
      { icon: 'mdi-vk', link: 'https://vk.com' },
      { icon: 'mdi-telegram', link: 'https://web.telegram.org' },
      { icon: 'mdi-whatsapp', link: 'https://web.whatsapp.com' },
    ],
  }),
}
</script>

<style scoped>
  @font-face {
    font-family: Montserrat;
    src: url('src/assets/fonts/Montserrat.ttf');
  }
  
.footer {
  padding: 0 35px; 
  background-color: #090909;
  height: 94px !important;
  align-items: center;
  display: flex;
  flex: 0 0 auto;
}

.logo {   
    height: 40px; 
    width: 100px; 
    flex: none; 
    margin-right: -27px;
  }   

  .button {
    width: 261px;
    height: 40px;
    border-radius: 10px;
    background-color: #1470EF;
    font-family: Montserrat;
    font-weight: normal;   
    text-transform: none;    
    color: white;   
  }

.social-icons {
  background-color: rgba(255, 255, 255, 0.2);
  height: 44px;
  width: 130px;
  border-radius: 15px;  
  margin-right: 10px;
  justify-content: center; 
  align-items: center; 
}
.v-btn--icon {
  margin: 5px; 
  }
  .v-btn--icon .v-icon {
    margin-left: 1px;
}
</style>