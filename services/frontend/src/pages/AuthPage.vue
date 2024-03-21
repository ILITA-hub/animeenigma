<template> 
  <div class="auth-container"> 
    <v-container> 
      <v-row justify="center"> 
        <v-col cols="12" lg="4"> 
          <v-tabs 
            class="tabs" 
            fixed-tabs 
            v-model="tab" 
            background-color="transparent"> 
            <v-tab :key="index" 
            v-for="(item, index) in tabs" 
            :class="{ 'tab--active': tab === index }">
            {{item}}</v-tab>
        </v-tabs>
        <v-card :style="tab === 1 ? { minHeight: '422px'} : {}" class="form">
            <div v-if="tab === 0"> 
              <div class="text">Добро пожаловать!</div> 
              <div class="text-subtitle">Войдите в аккаунт, чтобы продолжить</div> 
              <v-text-field class="field" 
                density="comfortable" 
                placeholder="Email" 
                prepend-inner-icon="mdi-email" 
                variant="" 
              ></v-text-field> 
              <v-text-field class="field"  
                :append-inner-icon="visible ? 'mdi-eye-off' : 'mdi-eye'"  
                :type="visible ? 'text' : 'password'"  
                density="comfortable"  
                placeholder="Пароль"  
                prepend-inner-icon="mdi-lock"  
                variant=""  
                @click:append-inner="visible = !visible"  
              ></v-text-field> 
              <div class="radio-password"> 
                <v-radio-group> 
                  <v-radio  
                    class="radio" 
                    label="Запомнить меня"  
                    color="#1470EF" 
                    value="rememberMe"> 
                  </v-radio> 
                </v-radio-group> 
                <div class="pb-4">Забыли пароль?</div>
              </div> 
              <v-btn color="#1470EF" class="mb-4">Войти</v-btn> 
            </div> 
            <div v-else-if="tab === 1">
              <div class="text">Создайте аккаунт</div> 
              <div class="text-subtitle">Зарегистрируйтесь, чтобы продолжить</div> 
              <v-text-field class="field" 
                density="comfortable" 
                placeholder="Введите Email" 
                prepend-inner-icon="mdi-email" 
                variant="" 
              ></v-text-field> 
              <v-text-field class="field"  
                :append-inner-icon="visible ? 'mdi-eye-off' : 'mdi-eye'"  
                :type="visible ? 'text' : 'password'"  
                density="comfortable"  
                placeholder="Придумайте пароль"  
                prepend-inner-icon="mdi-lock"  
                variant=""  
                @click:append-inner="visible = !visible"  
              ></v-text-field> 
              <v-text-field class="field"  
                :append-inner-icon="visible ? 'mdi-eye-off' : 'mdi-eye'"  
                :type="visible ? 'text' : 'password'"  
                density="comfortable"  
                placeholder="Повторите пароль"  
                prepend-inner-icon="mdi-lock"  
                variant=""  
                @click:append-inner="visible = !visible"  
              ></v-text-field> 
              <div class="pb-4">У вас уже есть аккаунт?</div>
              <v-btn color="#1470EF" class="mb-4 logup">Зарегистрироваться</v-btn> 
            </div>
          </v-card> 
        </v-col> 
      </v-row> 
    </v-container> 
  </div> 
</template> 

<script>
export default {
  data() {
    return {
      visible: false, 
      tab: null,
      tabs: ['Вход', 'Регистрация'],
    };
  },
}


</script>

<style scoped>


.auth-container {
  display: flex;
  justify-content: center;
  align-items: center;
  min-height: 100vh;
}

.form {
  border-radius: 10px;
  box-shadow: 10px 2px 50px 0px rgba(0, 0, 0, 0.05);
  background: rgb(33, 35, 53);
  top: 10px;
  width: 464px;
  height: 361px;
  transform: translateX(-10%);
  padding: 20px; 
  max-height: 1000px;
}

.text {
  color: rgb(255, 255, 255);
  font-family: Montserrat;
  font-size: 22px;
  font-weight: 700;
  line-height: 27px;
  letter-spacing: 0%;
  text-align: left;
  padding-bottom: 15px;
  margin-left: 15px;
  margin-top: 5px;

}

.text-subtitle {
  color: rgba(255, 255, 255, 0.5);
  font-family: Montserrat;
  font-size: 16px;
  font-weight: 400;
  line-height: 20px;
  letter-spacing: 0%;
  text-align: left;
  margin-left: 15px;
}

.tabs {
  border-radius: 10px;
  font-family: Montserrat;
  display: flex;
  align-items: center;
  box-shadow: 10px 20px 50px 0px rgba(0, 0, 0, 0.05);
  background: rgba(255, 255, 255, 0.1);
  color: aliceblue;
  font-size: 18px;
  font-weight: 500;
  line-height: 22px;
  letter-spacing: 0%;

}

.tab--active {
  color: #1470EF !important;
}

.field {
  color: rgb(194, 194, 194);
  font-family: Montserrat;
  font-size: 16px;
  font-weight: 500;
  letter-spacing: 0%;
  width: 394px;
  position: relative;
  top: 20px;
  color: none;
  background-color: rgba(255, 255, 255, 0.1);
  margin-bottom: 20px;
  border-radius: 10px;
  height: 50px;
  margin-left: 15px;
  display: grid;

}
.pb-4 {
  color: rgb(255, 255, 255);
font-family: Montserrat;
font-size: 14px;
font-weight: 400;
letter-spacing: 0%;
text-align: right;
position: relative;
right: 15px;
top: 15px;
}
.mb-4 {
position: relative;
width: 394px;
height: 50px;
font-size: 16px;
display: flex;
padding: 15px 55px 15px 55px;
border-radius: 10px;
left: 15px;
top: -10px;
text-transform: none;
font-family: Montserrat;
}

.logup {
  top: 10px;
  color: rgb(255, 255, 255);
  font-family: Montserrat;
  font-size: 16px;
  letter-spacing: 0%;
  text-align: left;
  text-transform: none;
}

.radio {
top: 5px;
left: 5px;
color: rgb(255, 255, 255);
font-size: 14px;
font-weight: 400;
letter-spacing: 0%;
text-align: left;
}

.radio-password {
  display: flex;
  color: rgb(255, 255, 255);
font-family: Montserrat;
font-size: 14px;
font-weight: 400;
letter-spacing: 0%;
}
</style>
