import { createApp } from 'vue'
import App from './App.vue'
import { router } from './router'
import '@fontsource/noto-sans-sc/400.css'
import '@fontsource/noto-sans-sc/500.css'
import '@fontsource/noto-sans-sc/600.css'
import '@fontsource/noto-sans-sc/700.css'
import './style.css'

// 剪影路径跟着舰船卡走（content/misc/image/ship/*.png），不再打进前端包；
// 所以这里不再统一预取，进对局后按出现的画面按需加载。

createApp(App).use(router).mount('#app')
