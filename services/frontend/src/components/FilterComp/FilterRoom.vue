<template>
    <div class="filter-room">
    <v-container>
      <v-menu
        v-model="menu"
        :close-on-content-click="false"
        location="bottom">
        <template v-slot:activator="{ props }">
          <v-btn class="btn-room"
            text
            v-bind="props"
            variant="plain">
            Фильтры комнат
            <v-icon class="icon" :size="28">{{ menu ? 'mdi-chevron-up' : 'mdi-chevron-down' }}</v-icon>
          </v-btn>
        </template>
        <v-card class="card-list">
          <v-select class="select" 
              :items="itemsStatus" 
              label="Статус викторины" 
              multiple 
          ></v-select> 
          <v-select class="select" 
              :items="itemsPlayers" 
              label="Игроков в комнате" 
              multiple 
          ></v-select> 
          <v-select class="select" 
              :items="itemsPeriod" 
              label="Период" 
              multiple 
          ></v-select> 
          <v-select class="select" 
              :items="itemsPlayersFrom" 
              label="Диапазон игроков от" 
              multiple 
          ></v-select> 
          <v-select class="select" 
              :items="itemsPlayersTo" 
              label="Диапазон игроков до" 
              multiple 
          ></v-select>
        </v-card>
      </v-menu>
    </v-container>
    </div>
</template>

<script>
export default {
    data: () => ({
      menu: false,
        itemsStatus: ['Ожидание игроков', 'В процессе'], 
        itemsPlayers: ['5-10', '11-15', '16 и более'], 
        itemsPeriod: ['1', '2', '3'], 
        itemsPlayersFrom: ['5', '11', '16'], 
        itemsPlayersTo: ['20', '25', '30'],
    }),
  }
</script>

<style scoped>
.btn-room {
    font-family: 'Montserrat';
    color: aliceblue;
    font-size: 28px !important;
    background: none;
    text-transform: none;
    font-weight: 700;
    line-height: 34.13px;
}
.btn-room .icon {
    background: rgba(51, 169, 255, 1);
    border-radius: 50%;
    margin-left: 5px;
}

.card-list {
    top: 10px;
    background: rgba(33, 35, 53, 1) !important;
}

.select {
  width: 89%;
    background: rgba(255, 255, 255, 0.1);
    border-radius: 10px;
    margin: 10px 10px 10px 20px;
    overflow: hidden;
    height: 45px;
    color: white;
    font-family: 'Montserrat';
    line-height: 19.5px;
    text-align: left; 
}

.v-menu > .v-overlay__content > .v-card, .v-menu > .v-overlay__content > .v-sheet, .v-menu > .v-overlay__content > .v-list { 
  width: 100%;
  height: 285px !important;
  background: rgba(255, 255, 255, 0.1);
  gap: 0px;
  border-radius: 10px !important;
}

</style>