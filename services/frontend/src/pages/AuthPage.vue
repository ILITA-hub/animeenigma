<template>
  <div class="auth-container">
    <v-container>
      <v-row justify="center">
        <v-col cols="12" lg="4" xs="12">
          <v-tabs class="tabs" fixed-tabs v-model="tab" background-color="transparent">
            <v-tab :key="index" v-for="(item, index) in tabs" :class="{ 'tab--active': tab === index }">
              {{ item }}
            </v-tab>
          </v-tabs>
          <v-card :class="tab === 1 ? 'form form--register' : 'form form--login'">
            <div v-if="tab === 0">
              <div class="text">Добро пожаловать!</div>
              <div class="text-subtitle">Войдите в аккаунт, чтобы продолжить</div>
              <v-text-field class="field" density="comfortable" variant="" placeholder="Email" prepend-inner-icon="mdi-email"
                v-model="email">
              </v-text-field>
              <v-text-field class="field" :append-inner-icon="visible.value ? 'mdi-eye' : 'mdi-eye-off'"
                :type="visible.value ? 'text' : 'password'" density="comfortable" variant="" placeholder="Пароль"
                prepend-inner-icon="mdi-lock" @click:append-inner="visible.value = !visible.value" v-model="password">
              </v-text-field>
              <div class="remember-password">
                <v-checkbox class="remember" label="Запомнить меня" color="#1470EF" v-model="rememberMe">
                </v-checkbox>
                <div class="forgot">Забыли пароль?</div>
              </div>
              <v-btn color="#1470EF" class="mb-4" @click="handleLogin">Войти</v-btn>
            </div>
            <div v-else-if="tab === 1">
              <div class="text">Создайте аккаунт</div>
              <div class="text-subtitle">Зарегистрируйтесь, чтобы продолжить</div>
              <v-text-field class="field" density="comfortable" variant="" placeholder="Введите Email" prepend-inner-icon="mdi-email"
                v-model="registrationEmail">
              </v-text-field>
              <v-text-field class="field" density="comfortable" variant="" placeholder="Придумайте никнейм" prepend-inner-icon="mdi-account"
                v-model="registrationUsername">
              </v-text-field>
              <v-text-field class="field" :append-inner-icon="visible.value ? 'mdi-eye' : 'mdi-eye-off'"
                :type="visible.value ? 'text' : 'password'" density="comfortable" variant="" placeholder="Придумайте пароль"
                prepend-inner-icon="mdi-lock" @click:append-inner="visible.value = !visible.value" v-model="registrationPassword">
              </v-text-field>
              <v-text-field class="field" :append-inner-icon="visible.value ? 'mdi-eye' : 'mdi-eye-off'"
                :type="visible.value ? 'text' : 'password'" density="comfortable" variant="" placeholder="Повторите пароль"
                prepend-inner-icon="mdi-lock" @click:append-inner="visible.value = !visible.value" v-model="registrationConfirmPassword">
              </v-text-field>
              <div class="have-acc" @click="tab = 0">У вас уже есть аккаунт?</div>
              <v-btn color="#1470EF" class="mb-4 logup" @click="handleRegister">Зарегистрироваться</v-btn>
            </div>
          </v-card>
          <div v-if="message">{{ message }}</div>
        </v-col>
      </v-row>
    </v-container>
  </div>
</template>

<script>
import { defineComponent, ref } from 'vue';
import { useAuthStore } from '@/stores/authStore';
import { useRouter } from 'vue-router';

export default defineComponent({
  setup() {
    const visible = ref(false);
    const tab = ref(0);
    const authStore = useAuthStore();
    const router = useRouter();
    const email = ref('');
    const password = ref('');
    const rememberMe = ref(false);
    const registrationEmail = ref('');
    const registrationUsername = ref('');
    const registrationPassword = ref('');
    const registrationConfirmPassword = ref('');
    const message = ref('');
    const tabs = ['Вход', 'Регистрация'];

    const handleLogin = async () => {
      try {
        const { success, user, message: msg } = await authStore.login({
          email: email.value,
          password: password.value,
          rememberMe: rememberMe.value,
        });
        if (success) {
          router.push('/user');
        } else {
          message.value = msg;
        }
      } catch (error) {
        console.error('Login error:', error);
        message.value = error.response?.data?.message || 'Неверный email или пароль.';
      }
    };
    const handleRegister = async () => {
      try {
        const { success, user, message: msg } = await authStore.register({
          email: registrationEmail.value,
          username: registrationUsername.value,
          password: registrationPassword.value,
          confirmPassword: registrationConfirmPassword.value,
        });
        if (success) {
          router.push('/user');
        } else {
          message.value = msg;
        }
      } catch (error) {
        console.error('Registration error:', error);
        message.value = error.response?.data?.message || 'Ошибка регистрации.';
      }
    };
    return {
      visible,
      tab,
      tabs,
      email,
      password,
      rememberMe,
      registrationEmail,
      registrationUsername,
      registrationPassword,
      registrationConfirmPassword,
      message,
      handleLogin,
      handleRegister,
    };
  },
});

</script>

<style scoped>
.auth-container {
  display: flex;
  justify-content: center;
  align-items: center;
  min-height: 80vh;
}

.form {
  border-radius: 10px;
  box-shadow: 10px 2px 50px 0px rgba(0, 0, 0, 0.05);
  background: rgb(33, 35, 53);
  top: 10px;
  padding: 20px;
  transform: translateX(-10%);
}

.form--login {
  width: 464px;
  height: 361px;
}

.form--register {
  width: 464px;
  min-height: 422px;
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
  width: 370px;
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
  background-color: rgba(255, 255, 255, 0.1);
  margin-bottom: 20px;
  border-radius: 10px;
  height: 50px;
  margin-left: 15px;
  display: grid;
}

.forgot {
  color: rgb(255, 255, 255);
  font-family: Montserrat;
  font-size: 14px;
  font-weight: 400;
  letter-spacing: 0%;
  text-align: right;
  position: relative;
  left: 107px;
  top: 18px;
}

.have-acc {
  color: rgb(255, 255, 255);
  font-family: Montserrat;
  font-size: 14px;
  font-weight: 400;
  letter-spacing: 0%;
  text-align: right;
  position: relative;
  left: -16px;
  top: 15px;
  margin-bottom: 15px;
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
  top: 1px;
  text-transform: none;
  font-family: Montserrat;
}

.logup {
  top: 15px;
  color: rgb(255, 255, 255);
  font-family: Montserrat;
  font-size: 16px;
  letter-spacing: 0%;
  text-align: left;
  text-transform: none;
}

.remember {
  left: 6px;
  color: rgb(255, 255, 255);
  font-size: 14px;
  font-weight: 400;
  letter-spacing: 0%;
  text-align: left;
  position: relative;
}

.remember-password {
  display: flex;
  color: rgb(255, 255, 255);
  font-family: Montserrat;
  font-size: 14px;
  font-weight: 400;
  height: 60px;
  top: 4px;
  position: relative;
}

.forgot:hover, .have-acc:hover {
  color: #1470EF;
  cursor: pointer;
}
</style>
