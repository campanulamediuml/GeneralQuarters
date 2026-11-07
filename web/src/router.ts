import { createRouter, createWebHistory } from 'vue-router'
import HomeView from './views/HomeView.vue'
import GameView from './views/GameView.vue'

export const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', component: HomeView },
    { path: '/game/:roomID', component: GameView, props: true },
  ],
})
