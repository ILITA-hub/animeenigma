import { createRouter, createWebHistory } from 'vue-router'

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes: [
    {
      path: '/',
      name: 'HomeView',
      component: () => import('../views/HomeView.vue')
    },
    {
      path: '/room/:id',
      name: 'RoomView',
      component: () => import('../views/RoomView.vue')
    },
    {
      path: '/createRoom',
      name: 'CreateRoomView',
      component: () => import('../views/CreateRoom.vue')
    },
  ]
})

export default router
