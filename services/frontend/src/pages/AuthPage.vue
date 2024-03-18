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
          <v-card flat class="form">
            <v-tabs-items v-model="tab">
              <v-tab-item>
                <v-card-text>
                  <div class="text">Добро пожаловать!</div>
                  <div class="text-subtitle">Войдите в аккаунт, чтобы продолжить</div>
                  <v-form>
                    <v-text-field label="Email" required class="field"></v-text-field>
                    <v-text-field class="field"
                      label="Пароль"
                      type="password"
                      required
                    ></v-text-field>
                    <v-checkbox
                      label="Запомнить меня"
                      class="mb-4"
                    ></v-checkbox>
                    <div class="text-center pb-4">Забыли пароль?</div>
                    <v-btn color="primary" block class="mb-4">
                      Войти
                    </v-btn>
                  </v-form>
                </v-card-text>
              </v-tab-item>

              <v-tab-item>
              </v-tab-item>
            </v-tabs-items>
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
      tab: 0,
    };
  },
};
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
  margin: 4px 0px 0px 15px;

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

</style>
