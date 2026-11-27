import { createRouter, createWebHistory } from 'vue-router'
import HomeView from './views/HomeView.vue'
import GameView from './views/GameView.vue'
import WatchView from './views/WatchView.vue'

export const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', component: HomeView },
    { path: '/game/:roomID', component: GameView, props: true },
    // 观战：拿 token 的只读完整视图。和玩家界面共用海图与兵牌，只是不能下命令。
    { path: '/watch/:token', component: WatchView, props: true },
    // 老链接兼容：/watch/{roomID}/{token}，房间 ID 由 token 解析得出，这里只取 token。
    { path: '/watch/:roomID/:token', component: WatchView, props: (route) => ({ token: String(route.params.token) }) },
  ],
})
