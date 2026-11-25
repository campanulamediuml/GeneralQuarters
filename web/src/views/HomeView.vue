<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '../api'

const router = useRouter()
const name = ref(localStorage.getItem('gq_name') || '')
const scenarioID = ref('_coral_real')
const scenarios = [
  { id: '_coral_real', name: '珊瑚海 · 1942年5月（25海里/hex）' },
]
// 阵营：房主坐哪一边。以前只能坐 A，想当日军只能等别人建房，太别扭。
const faction = ref<'A' | 'B'>('A')
// 单人房：一名玩家 ready 就结算，只用于自己摆摊测试。
// 双人对局必须两名玩家都在，否则一个人点一下就自己跟自己跑掉好几个回合。
const solo = ref(false)
const token = ref('')
const busy = ref(false)
const error = ref('')

async function createGame() {
  busy.value = true
  error.value = ''
  try {
    localStorage.setItem('gq_name', name.value.trim())
    const room = await api.createRoom(name.value.trim(), scenarioID.value, solo.value, faction.value)
    await router.push(`/game/${room.room_id}`)
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : '创建失败'
  } finally {
    busy.value = false
  }
}

async function joinGame() {
  if (!token.value.trim()) {
    error.value = '请输入房间token'
    return
  }
  busy.value = true
  error.value = ''
  try {
    localStorage.setItem('gq_name', name.value.trim())
    const room = await api.joinRoom(token.value.trim(), name.value.trim())
    await router.push(`/game/${room.room_id}`)
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : '加入失败'
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <main class="home-shell">
    <nav class="home-nav">
      <div class="brand">
        <span class="brand-mark" aria-hidden="true"><i></i><i></i><i></i></span>
        <span>GENERAL QUARTERS</span>
      </div>
      <a class="nav-doc" href="/api/docs" target="_blank">API 文档 <span>↗</span></a>
    </nav>

    <section class="hero-grid">
      <div class="hero-copy">
        <div class="eyebrow"><span></span> 浏览器多人海军战棋</div>
        <h1>旗舰战位，<br /><em>等待你的命令。</em></h1>
        <p class="hero-lead">在同一张海图上部署、侦查与交锋。所有判定由权威服务器完成，你只需专注于下一步决策。</p>
        <div class="feature-row">
          <span>权威回合引擎</span><span>双人房间</span><span>持久化战局</span>
        </div>
        <div class="signal-card">
          <div class="signal-head"><span class="live-dot"></span> SANDBOX ONLINE</div>
          <div class="signal-map" aria-hidden="true">
            <i v-for="n in 35" :key="n" :class="{ hot: [8, 13, 22, 27].includes(n) }"></i>
          </div>
          <div class="signal-meta"><span>灰蓝海图</span><span>9 × 9 HEX</span></div>
        </div>
      </div>

      <div class="access-panel">
        <div class="panel-kicker">NEW OPERATION</div>
        <h2>进入作战室</h2>
        <p>选择剧本后创建作战室；双方在同一张海图上搜索、交战。</p>
        <label class="field-label" for="scenario">剧本</label>
        <select id="scenario" v-model="scenarioID" class="text-input">
          <option v-for="item in scenarios" :key="item.id" :value="item.id">{{ item.name }}</option>
        </select>
        <label class="field-label" for="faction">你的阵营</label>
        <select id="faction" v-model="faction" class="text-input">
          <option value="A">阵营 A（珊瑚海剧本＝美军第17特混舰队）</option>
          <option value="B">阵营 B（珊瑚海剧本＝日军 MO 机动部队）</option>
        </select>
        <label class="field-label" for="commander">指挥官称呼</label>
        <input id="commander" v-model="name" class="text-input" maxlength="32" placeholder="玩家" @keyup.enter="createGame" />
        <button class="primary-button" :disabled="busy" @click="createGame">
          <span>{{ busy ? '正在建立战局…' : '创建游戏' }}</span><b>→</b>
        </button>
        <label class="solo-toggle"><input v-model="solo" type="checkbox" /> 单人房（自己 ready 就结算，仅用于自测）</label>
        <div class="create-note"><span class="live-dot"></span> 双人对局需要两名玩家都 ready 才会推进回合</div>
        <div class="divider"><span>或凭邀请加入</span></div>
        <label class="field-label" for="room-token">房间 TOKEN</label>
        <div class="join-row">
          <input id="room-token" v-model="token" class="text-input token-input" maxlength="14" placeholder="XXXX-XXXX-XXXX" @keyup.enter="joinGame" />
          <button class="secondary-button" :disabled="busy" @click="joinGame">加入</button>
        </div>
        <p v-if="error" class="error-banner" role="alert">{{ error }}</p>
        <div class="panel-footer"><span>GO SERVER</span><span>VUE CLIENT</span><span>SQLITE</span></div>
      </div>
    </section>

    <footer class="home-footer">
      <span>GQ / FIELD COMMAND SYSTEM</span>
      <span>地图与剧本数据可在服务器持久化目录中编辑</span>
    </footer>
  </main>
</template>
