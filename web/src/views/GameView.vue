<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '../api'
import type { GameView, Unit } from '../types'
import HexMap from '../components/HexMap.vue'

const props = defineProps<{ roomID: string }>()
const router = useRouter()
const state = ref<GameView | null>(null)
const selectedID = ref('')
const loading = ref(true)
const actionBusy = ref(false)
const error = ref('')
const socketStatus = ref<'connecting' | 'online' | 'offline'>('connecting')
const copied = ref(false)
let socket: WebSocket | undefined
let poll: ReturnType<typeof setInterval> | undefined
let refreshTimer: ReturnType<typeof setTimeout> | undefined

const ownUnits = computed(() => state.value?.units.filter((unit) => unit.faction === state.value?.you.faction) || [])
const selectedUnit = computed(() => ownUnits.value.find((unit) => unit.id === selectedID.value))
const orderedEvents = computed(() => [...(state.value?.events || [])].reverse())
const readyCount = computed(() => state.value?.players.filter((player) => player.ready).length || 0)

async function loadState(initial = false) {
  if (initial) loading.value = true
  try {
    const next = await api.state(props.roomID)
    if (state.value && next.version !== state.value.version) selectedID.value = ''
    state.value = next
    error.value = ''
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : '同步失败'
    if (error.value.includes('请先创建') || error.value.includes('会话已失效')) await router.push('/')
  } finally {
    loading.value = false
  }
}

function scheduleRefresh() {
  if (refreshTimer) clearTimeout(refreshTimer)
  refreshTimer = setTimeout(() => loadState(), 120)
}

function connectSocket() {
  socketStatus.value = 'connecting'
  const protocol = location.protocol === 'https:' ? 'wss' : 'ws'
  socket = new WebSocket(`${protocol}://${location.host}/ws`)
  socket.onopen = () => { socketStatus.value = 'online' }
  socket.onmessage = scheduleRefresh
  socket.onclose = () => { socketStatus.value = 'offline' }
  socket.onerror = () => { socketStatus.value = 'offline' }
}

async function move(coord: string) {
  if (!state.value || !selectedUnit.value || actionBusy.value) return
  actionBusy.value = true
  try {
    await api.command(props.roomID, state.value.version, { type: 'SET_MOVE', unit_id: selectedUnit.value.id, coord })
    await loadState()
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : '命令失败'
  } finally {
    actionBusy.value = false
  }
}

async function clearOrder(unit: Unit) {
  if (!state.value) return
  try {
    await api.command(props.roomID, state.value.version, { type: 'CLEAR_MOVE', unit_id: unit.id })
    await loadState()
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : '取消失败'
  }
}

async function toggleReady() {
  if (!state.value || actionBusy.value) return
  actionBusy.value = true
  try {
    state.value = await api.ready(props.roomID, state.value.version, !state.value.you.ready)
    selectedID.value = ''
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : '准备状态失败'
  } finally {
    actionBusy.value = false
  }
}

async function copyToken() {
  if (!state.value?.join_token) return
  await navigator.clipboard.writeText(state.value.join_token)
  copied.value = true
  setTimeout(() => { copied.value = false }, 1500)
}

onMounted(async () => {
  await loadState(true)
  connectSocket()
  poll = setInterval(() => loadState(), 15000)
})

onUnmounted(() => {
  socket?.close()
  if (poll) clearInterval(poll)
  if (refreshTimer) clearTimeout(refreshTimer)
})
</script>

<template>
  <main class="game-shell">
    <header class="game-header">
      <button class="brand game-brand" aria-label="返回首页" @click="router.push('/')">
        <span class="brand-mark" aria-hidden="true"><i></i><i></i><i></i></span>
        <span>GENERAL QUARTERS</span>
      </button>
      <div v-if="state" class="operation-title"><span>当前剧本</span><strong>{{ state.scenario.name }}</strong></div>
      <div class="header-right">
        <div class="connection"><span class="live-dot" :class="`status-${socketStatus}`"></span>{{ socketStatus === 'online' ? '实时连接' : socketStatus === 'connecting' ? '连接中' : '轮询模式' }}</div>
        <a href="/api/docs" target="_blank">API ↗</a>
      </div>
    </header>

    <div v-if="loading" class="loading-screen"><span class="loader"></span>正在载入海图与战局…</div>
    <template v-else-if="state">
      <div class="turn-strip">
        <div><span class="turn-label">TURN</span><strong>{{ String(state.turn).padStart(2, '0') }}</strong><span class="turn-sub">等待双方命令</span></div>
        <div class="turn-actions">
          <span class="ready-count">{{ readyCount }} / {{ state.players.length }} 已就绪</span>
          <button class="ready-button" :class="{ cancel: state.you.ready }" :disabled="actionBusy" @click="toggleReady">
            {{ state.you.ready ? '取消就绪' : '完成部署，结束回合' }}
          </button>
        </div>
      </div>

      <p v-if="error" class="error-banner game-error" role="alert">{{ error }}</p>

      <div class="game-layout">
        <section class="map-panel">
          <div class="map-toolbar">
            <div><span class="section-kicker">OPERATIONAL MAP</span><h1>{{ state.map.name }}</h1></div>
            <div class="map-legend"><span><i class="legend-a"></i>A 阵营</span><span><i class="legend-b"></i>B 阵营</span><span><i class="legend-move"></i>可移动</span></div>
          </div>
          <div class="map-canvas"><HexMap :map="state.map" :units="state.units" :selected-unit="selectedUnit" :orders="state.orders" @select-unit="$event.faction === state.you.faction && (selectedID = $event.id)" @move="move" /></div>
          <div class="map-help"><b>操作方式</b><span>选择己方舰艇</span><i>→</i><span>选择高亮hex</span><i>→</i><span>双方就绪后统一结算</span></div>
        </section>

        <aside class="command-sidebar">
          <section class="side-card commander-card">
            <div class="section-kicker">YOUR COMMAND</div>
            <div class="commander-line"><div class="faction-seal" :class="`seal-${state.you.faction.toLowerCase()}`">{{ state.you.faction }}</div><div><h2>{{ state.you.name }}</h2><span>{{ state.you.faction === 'A' ? '苍蓝舰队' : '赤红舰队' }} · {{ state.you.is_host ? '房主' : '参战玩家' }}</span></div></div>
            <div class="room-code"><span>ROOM</span><code>{{ state.room_id }}</code></div>
          </section>

          <section v-if="state.join_token" class="side-card invite-card">
            <div><span class="section-kicker">INVITE PLAYER B</span><p>将邀请token发送给第二位玩家。</p></div>
            <button class="token-display" @click="copyToken"><code>{{ state.join_token }}</code><span>{{ copied ? '已复制' : '复制' }}</span></button>
          </section>

          <section class="side-card selection-card">
            <div class="side-card-head"><span class="section-kicker">SELECTED UNIT</span><span>{{ ownUnits.length }} 个己方单位</span></div>
            <div v-if="selectedUnit" class="selected-detail">
              <div><span class="unit-type-badge">{{ selectedUnit.type }}</span><h3>{{ selectedUnit.name }}</h3></div>
              <dl><div><dt>当前位置</dt><dd>{{ selectedUnit.coord }}</dd></div><div><dt>结构强度</dt><dd>{{ selectedUnit.strength }} / {{ selectedUnit.max_strength }}</dd></div><div><dt>可移动hex</dt><dd>{{ selectedUnit.legal_moves?.length || 0 }}</dd></div></dl>
              <p>请在地图上选择一个绿色高亮hex。</p>
            </div>
            <div v-else class="empty-selection"><span class="selection-cross">+</span><p>选择地图上的己方舰艇<br />以下达移动命令</p></div>
          </section>

          <section class="side-card orders-card">
            <div class="side-card-head"><span class="section-kicker">PENDING ORDERS</span><span>{{ state.orders.length }}</span></div>
            <div v-if="state.orders.length" class="order-list">
              <div v-for="order in state.orders" :key="order.unit_id"><span>{{ ownUnits.find((unit) => unit.id === order.unit_id)?.name || order.unit_id }}</span><code>→ {{ order.coord }}</code><button aria-label="取消命令" @click="clearOrder({ id: order.unit_id } as Unit)">×</button></div>
            </div>
            <p v-else class="empty-copy">尚未下达移动命令</p>
          </section>

          <section class="side-card players-card">
            <div class="side-card-head"><span class="section-kicker">PLAYERS</span></div>
            <div v-for="player in state.players" :key="player.id" class="player-row"><span class="player-faction" :class="`seal-${player.faction.toLowerCase()}`">{{ player.faction }}</span><span>{{ player.name }}</span><i :class="{ online: player.connected }"></i><b :class="{ ready: player.ready }">{{ player.ready ? '已就绪' : '部署中' }}</b></div>
          </section>

          <section class="side-card log-card">
            <div class="side-card-head"><span class="section-kicker">COMBAT LOG</span><span>最近事件</span></div>
            <div class="event-list">
              <div v-for="event in orderedEvents" :key="event.id" class="event-row"><span>T{{ String(event.turn).padStart(2, '0') }}</span><p>{{ event.text }}</p></div>
            </div>
          </section>
        </aside>
      </div>
    </template>
  </main>
</template>
