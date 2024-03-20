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
            <v-tab>Вход</v-tab>
            <v-tab>Регистрация</v-tab>
          </v-tabs>
          <v-card class="form">
      <div class="text">Добро пожаловать!</div>
      <div class="text-subtitle">Войдите в аккаунт, чтобы продолжить</div>
      <v-text-field class="field"
        density="compact"
        placeholder="Email"
        prepend-inner-icon="mdi-email-outline"
        variant=""
      ></v-text-field>
      <v-text-field class="field"
        :append-inner-icon="visible ? 'mdi-eye-off' : 'mdi-eye'"
        :type="visible ? 'text' : 'password'"
        density="compact"
        placeholder="Пароль"
        prepend-inner-icon="mdi-lock-outline"
        variant=""
        @click:append-inner="visible = !visible"
      ></v-text-field>
      
      <v-radio color="#1470EF" label="Запомнить меня"></v-radio>
                    <div class="pb-4">Забыли пароль?</div>
                    <v-btn color="#1470EF" class="mb-4">
                      Войти
                    </v-btn>
    </v-card>
        </v-col>
      </v-row>
    </v-container>
  </div>
</template>

<script>
  export default {
    data: () => ({
      visible: false,
    }),
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
  display: flex;
  align-items: center;
  box-shadow: 10px 20px 50px 0px rgba(0, 0, 0, 0.05);
  background: rgba(255, 255, 255, 0.1);
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
line-height: 17px;
letter-spacing: 0%;
  text-align: right;
position: relative;
bottom: 20px;
right: 15px;
}
.mb-4 {
position: relative;
width: 394px;
height: 50px;
display: flex;
padding: 15px 55px 15px 55px;
border-radius: 10px;
left: 15px;
top: -10px;
}
</style>
