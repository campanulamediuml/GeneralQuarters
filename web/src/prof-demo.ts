import { createApp, h } from 'vue'
import ShipProfile from './components/ShipProfile.vue'
import '../src/style.css'
const app = createApp({ render: () => h('div', { style: 'padding:20px;background:#0b1b20;width:760px' }, [
  h('div', { style: 'color:#d7bb7b;font:600 13px/1 "Noto Sans SC";margin-bottom:8px' }, '约克城（参数化描绘）'),
  h(ShipProfile, { name: '约克城', type: 'CV' }),
]) })
app.mount('#app')
