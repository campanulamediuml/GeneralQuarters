<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '../api'

const router = useRouter()
const name = ref(localStorage.getItem('gq_name') || '')
const scenarioID = ref('_coral_real')
const scenarios = [
  {
    id: '_coral_real',
    name: '珊瑚海 · 1942年5月（25海里/hex）',
    factions: { A: '美军第17特混舰队', B: '日军 MO 机动部队' },
  },
  {
    id: '_marianas',
    name: '马里亚纳 · 1944年6月（架空，89×60）',
    factions: { A: '美军第58特混舰队', B: '日军第一机动舰队（小泽·栗田）' },
  },
]
// 阵营名跟着剧本走：以前写死成"珊瑚海剧本＝…"，开马里亚纳也显示珊瑚海的部队名。
const factions = computed(() => (scenarios.find((item) => item.id === scenarioID.value) ?? scenarios[0]).factions)
// 创建和加入都使用这个阵营；创建房间本身不再自动占据席位。
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
    const room = await api.createRoom(scenarioID.value, solo.value)
    token.value = room.join_token || ''
    await api.joinRoom(room.join_token || '', name.value.trim(), faction.value)
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
    const room = await api.joinRoom(token.value.trim(), name.value.trim(), faction.value)
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
          <option value="A">阵营 A（{{ factions.A }}）</option>
          <option value="B">阵营 B（{{ factions.B }}）</option>
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
        <div v-if="error" class="fleet-dialog" @click.self="error = ''">
          <section class="fleet-modal" role="alertdialog" aria-modal="true" aria-label="操作未生效">
            <header><div><span class="section-kicker">COMMAND REJECTED</span><h2>操作未生效</h2></div><button aria-label="关闭" @click="error = ''">×</button></header>
            <p class="error-modal-text">{{ error }}</p>
            <footer class="error-modal-actions"><button class="apply-button" @click="error = ''">知道了</button></footer>
          </section>
        </div>
        <div class="panel-footer"><span>GO SERVER</span><span>VUE CLIENT</span><span>SQLITE</span></div>
      </div>
    </section>

    <footer class="home-footer">
      <span>GQ / FIELD COMMAND SYSTEM</span>
      <span>地图与剧本数据可在服务器持久化目录中编辑</span>
    </footer>
  </main>
</template>
