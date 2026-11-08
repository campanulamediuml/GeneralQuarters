<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '../api'
import type { GameView, Unit } from '../types'
import { groupFleets } from '../fleet'
import HexMap from '../components/HexMap.vue'

const props = defineProps<{ roomID: string }>()
const router = useRouter()
const state = ref<GameView | null>(null)
const selectedID = ref('')
const menuCoord = ref('')
const mergeSourceID = ref('')
const movingMemberID = ref('')
const draftAssignments = ref<Record<string, string>>({})
const activePanel = ref<'map' | 'rules' | 'settings' | 'aviation' | 'log'>('map')
const loading = ref(true)
const actionBusy = ref(false)
const error = ref('')
const socketStatus = ref<'connecting' | 'online' | 'offline'>('connecting')
const copied = ref(false)
let socket: WebSocket | undefined
let poll: ReturnType<typeof setInterval> | undefined
let refreshTimer: ReturnType<typeof setTimeout> | undefined
let stateRequest = 0

const ownUnits = computed(() => state.value?.units.filter((unit) => unit.faction === state.value?.you.faction) || [])
const fleets = computed(() => state.value?.fleets || [])
const ownFleets = computed(() => fleets.value.filter((fleet) => fleet.faction === state.value?.you.faction))
const selectedUnit = computed(() => ownUnits.value.find((unit) => unit.id === selectedID.value))
const selectedFleet = computed(() => ownFleets.value.find((fleet) => fleet.id === selectedID.value || fleet.members.some((unit) => unit.id === selectedID.value)))
const menuFleets = computed(() => groupFleets(ownUnits.value.filter((unit) => unit.coord === menuCoord.value).map((unit) => ({ ...unit, fleet_id: draftAssignments.value[unit.id] || unit.fleet_id })), []))
const mergeSource = computed(() => menuFleets.value.find((fleet) => fleet.fleet_id === mergeSourceID.value))
const orderedEvents = computed(() => [...(state.value?.events || [])].reverse())
const readyCount = computed(() => state.value?.players.filter((player) => player.ready).length || 0)

async function loadState(initial = false) {
  const requestID = ++stateRequest
  if (initial) loading.value = true
  try {
    const next = await api.state(props.roomID)
    if (requestID !== stateRequest) return
    if (state.value && next.version !== state.value.version) {
      selectedID.value = ''
      menuCoord.value = ''
      mergeSourceID.value = ''
    }
    state.value = next
    error.value = ''
  } catch (cause) {
    if (requestID !== stateRequest) return
    error.value = cause instanceof Error ? cause.message : '同步失败'
    if (error.value.includes('请先创建') || error.value.includes('会话已失效')) await router.push('/')
  } finally {
    if (requestID === stateRequest) loading.value = false
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

function selectHex(coord: string) {
  const found = fleets.value.filter((fleet) => fleet.coord === coord)
  if (!found.length) {
    selectedID.value = ''
    menuCoord.value = ''
    return
  }
  if (found.length === 1) {
    selectedID.value = found[0].id
    menuCoord.value = ''
    return
  }
  openManager(coord)
}

function openManager(coord: string) {
  menuCoord.value = coord
  mergeSourceID.value = ''
  movingMemberID.value = ''
  draftAssignments.value = {}
  for (const unit of ownUnits.value.filter((unit) => unit.coord === coord)) {
    draftAssignments.value[unit.id] = unit.fleet_id || unit.id
  }
}

function closeManager() {
  menuCoord.value = ''
  mergeSourceID.value = ''
  movingMemberID.value = ''
  draftAssignments.value = {}
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

async function clearOrder(unitID: string) {
  if (!state.value) return
  try {
    await api.command(props.roomID, state.value.version, { type: 'CLEAR_MOVE', unit_id: unitID })
    await loadState()
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : '取消失败'
  }
}

function transferMember(targetFleetID: string) {
  if (!movingMemberID.value || movingMemberID.value === targetFleetID) return
  draftAssignments.value[movingMemberID.value] = targetFleetID
  movingMemberID.value = ''
}

function splitMember(member: Unit) {
  draftAssignments.value[member.id] = `split-${Date.now().toString(36)}`
  movingMemberID.value = ''
}

function mergeDraftInto(targetFleetID: string) {
  if (!mergeSourceID.value || mergeSourceID.value === targetFleetID) return
  for (const [unitID, fleetID] of Object.entries(draftAssignments.value)) {
    if (fleetID === mergeSourceID.value) draftAssignments.value[unitID] = targetFleetID
  }
  mergeSourceID.value = ''
}

async function applyReorganization() {
  if (!state.value || !menuCoord.value || actionBusy.value) return
  actionBusy.value = true
  try {
    await api.command(props.roomID, state.value.version, { type: 'REORGANIZE_HEX', unit_id: `hex-${menuCoord.value}`, coord: menuCoord.value, assignments: draftAssignments.value })
    closeManager()
    await loadState()
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : '整编失败'
  } finally {
    actionBusy.value = false
  }
}

async function toggleReady() {
  if (!state.value || actionBusy.value) return
  actionBusy.value = true
  const requestID = ++stateRequest
  try {
    const next = await api.ready(props.roomID, state.value.version, !state.value.you.ready)
    if (requestID !== stateRequest) return
    state.value = next
    selectedID.value = ''
    menuCoord.value = ''
    activePanel.value = 'map'
  } catch (cause) {
    if (requestID === stateRequest) error.value = cause instanceof Error ? cause.message : '准备状态失败'
  } finally {
    actionBusy.value = false
  }
}

function unitName(unitID: string) {
  return state.value?.units.find((unit) => unit.id === unitID)?.name || unitID
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
  <main class="game-shell command-layout" :data-version="state?.version || 0">
    <aside class="left-rail">
      <button class="rail-brand" aria-label="返回首页" @click="router.push('/')"><span>GQ</span><small>COMMAND</small></button>
      <nav class="rail-menu">
        <button :class="{ active: activePanel === 'rules' }" @click="activePanel = 'rules'"><span class="rail-icon">▤</span><b>规则书</b></button>
        <button :class="{ active: activePanel === 'map' }" @click="activePanel = 'map'"><span class="rail-icon">⬡</span><b>地图</b></button>
        <button :class="{ active: activePanel === 'settings' }" @click="activePanel = 'settings'"><span class="rail-icon">⌘</span><b>设置</b></button>
        <button :class="{ active: activePanel === 'aviation' }" @click="activePanel = 'aviation'"><span class="rail-icon">△</span><b>航空界面</b></button>
        <button :class="{ active: activePanel === 'log' }" @click="activePanel = 'log'"><span class="rail-icon">≡</span><b>日志信息</b></button>
      </nav>
      <div class="rail-spacer"></div>
      <div class="rail-vp"><span>当前 VP</span><strong>0 : 0</strong><small>沙盒不计分</small></div>
      <button class="rail-next" :disabled="!state || actionBusy" @click="toggleReady"><span>{{ state?.you.ready ? '取消' : '下一回合' }}</span><b>▶</b></button>
    </aside>

    <div class="command-main">
      <header class="game-header">
        <div class="brand game-brand"><span class="brand-mark" aria-hidden="true"><i></i><i></i><i></i></span><span>GENERAL QUARTERS</span></div>
        <div v-if="state" class="operation-title"><span>当前剧本</span><strong>{{ state.scenario.name }}</strong></div>
        <div class="header-right"><div class="connection"><span class="live-dot" :class="`status-${socketStatus}`"></span>{{ socketStatus === 'online' ? '实时连接' : socketStatus === 'connecting' ? '连接中' : '轮询模式' }}</div><a href="/api/docs" target="_blank">API ↗</a></div>
      </header>

      <div v-if="loading" class="loading-screen"><span class="loader"></span>正在载入海图与战局…</div>
      <template v-else-if="state">
        <div class="turn-strip">
          <div><span class="turn-label">TURN</span><strong>{{ String(state.turn).padStart(2, '0') }}</strong><span class="turn-sub">等待双方命令</span></div>
          <div class="turn-actions"><span class="ready-count">{{ readyCount }} / {{ state.players.length }} 已就绪</span><button class="ready-button" :class="{ cancel: state.you.ready }" :disabled="actionBusy" @click="toggleReady">{{ state.you.ready ? '取消就绪' : '完成部署，结束回合' }}</button></div>
        </div>
        <p v-if="error" class="error-banner game-error" role="alert">{{ error }}</p>

        <div v-if="activePanel === 'map'" class="game-layout">
          <section class="map-panel">
            <div class="map-toolbar"><div><span class="section-kicker">OPERATIONAL MAP</span><h1>{{ state.map.name }}</h1></div><div class="map-legend"><span><i class="legend-a"></i>A 阵营</span><span><i class="legend-b"></i>B 阵营</span><span><i class="legend-move"></i>可移动</span></div></div>
            <div class="map-canvas"><HexMap :map="state.map" :fleets="fleets" :player-faction="state.you.faction" :selected-unit="selectedUnit" :orders="state.orders" @select-unit="selectedID = $event.id" @select-hex="selectHex" @manage-hex="openManager" @move="move" /></div>
            <div class="map-help"><b>操作方式</b><span>拖动后立即移动</span><i>→</i><span>蓝条已移动，黄条可移动</span><i>→</i><span>点击多舰队hex可整编</span></div>
          </section>

          <aside class="command-sidebar">
            <section class="side-card commander-card">
              <div class="section-kicker">YOUR COMMAND</div>
              <div class="commander-line"><div class="faction-seal" :class="`seal-${state.you.faction.toLowerCase()}`">{{ state.you.faction }}</div><div><h2>{{ state.you.name }}</h2><span>{{ state.you.faction === 'A' ? '苍蓝舰队' : '赤红舰队' }} · {{ state.you.is_host ? '房主' : '参战玩家' }}</span></div></div>
              <div class="room-code"><span>ROOM</span><code>{{ state.room_id }}</code></div>
            </section>
            <section v-if="state.join_token" class="side-card invite-card"><div><span class="section-kicker">INVITE PLAYER B</span><p>将邀请token发送给第二位玩家。</p></div><button class="token-display" @click="copyToken"><code>{{ state.join_token }}</code><span>{{ copied ? '已复制' : '复制' }}</span></button></section>

            <section class="side-card selection-card">
              <div class="side-card-head"><span class="section-kicker">FLEET DETAIL</span><span>{{ ownFleets.length }} 个己方舰队</span></div>
              <div v-if="selectedFleet" class="selected-detail">
                <div><span class="unit-type-badge">{{ selectedFleet.type }}</span><h3>{{ selectedFleet.name }}</h3></div>
                <dl><div><dt>当前位置</dt><dd>{{ selectedFleet.coord }}</dd></div><div><dt>舰队规模</dt><dd>{{ selectedFleet.count }} 艘</dd></div><div><dt>总强度</dt><dd>{{ selectedFleet.strength }} / {{ selectedFleet.max_strength }}</dd></div></dl>
                <div class="member-list"><span>舰队成员</span><div v-for="member in selectedFleet.members" :key="member.id"><b>{{ member.type }}</b><p>{{ member.name }}</p><code>{{ member.strength }}/{{ member.max_strength }}</code></div></div>
                <p>点击多舰队hex，可查看并整编其中的舰队。</p>
              </div>
              <div v-else class="empty-selection"><span class="selection-cross">+</span><p>选择地图上的己方舰队<br />查看成员并下达命令</p></div>
            </section>

            <section class="side-card orders-card">
              <div class="side-card-head"><span class="section-kicker">PENDING ORDERS</span><span>{{ state.orders.length }}</span></div>
              <div v-if="state.orders.length" class="order-list"><div v-for="order in state.orders" :key="order.unit_id"><span>{{ unitName(order.unit_id) }}</span><code>{{ order.type === 'MERGE_FLEET' ? `整编 → ${unitName(order.target_unit_id || '')}` : order.type === 'REORGANIZE_HEX' ? `整编hex ${order.coord}` : `预定 → ${order.coord}` }}</code><button aria-label="取消命令" @click="clearOrder(order.unit_id)">×</button></div></div>
              <p v-else class="empty-copy">尚未下达移动或整编命令</p>
            </section>
            <section class="side-card players-card"><div class="side-card-head"><span class="section-kicker">PLAYERS</span></div><div v-for="player in state.players" :key="player.id" class="player-row"><span class="player-faction" :class="`seal-${player.faction.toLowerCase()}`">{{ player.faction }}</span><span>{{ player.name }}</span><i :class="{ online: player.connected }"></i><b :class="{ ready: player.ready }">{{ player.ready ? '已就绪' : '部署中' }}</b></div></section>
            <section class="side-card log-card"><div class="side-card-head"><span class="section-kicker">COMBAT LOG</span><button class="text-button" @click="activePanel = 'log'">全部日志</button></div><div class="event-list"><div v-for="event in orderedEvents.slice(0, 8)" :key="event.id" class="event-row"><span>T{{ String(event.turn).padStart(2, '0') }}</span><p>{{ event.text }}</p></div></div></section>
          </aside>
        </div>

        <section v-else-if="activePanel === 'rules'" class="workspace-page"><span class="section-kicker">RULEBOOK</span><h1>规则书与沙盒指南</h1><div class="document-grid"><article><h2>当前通信沙盒</h2><p>选择己方舰队，再选择相邻高亮hex下达移动命令。移动由服务器立即生效，同一舰队每回合只能移动一次；底条黄色表示仍可移动，蓝色表示本回合已经移动，右上角箭头表示舰队朝向。</p><p>同一hex存在多个舰队时，点击hex弹出舰队列表。选择舰队可查看成员，选择整编可将任意两个同阵营舰队合并，整编同样由服务器立即生效。</p></article><article><h2>坐标与地形</h2><p>hex使用ABCD坐标，AB为行、CD为列。当前只能移动至相邻且可通行的hex，岛屿和暗礁不可进入。</p><p>正式航空、损管与剧本规则位于项目《规则书.md》，将随规则模块逐步接入。</p></article></div></section>
        <section v-else-if="activePanel === 'settings'" class="workspace-page"><span class="section-kicker">SETTINGS</span><h1>作战室设置</h1><div class="settings-grid"><div class="side-card"><h2>当前会话</h2><p>指挥官：{{ state.you.name }}</p><p>阵营：{{ state.you.faction }} · 房间：{{ state.room_id }}</p><p>连接方式：{{ socketStatus === 'online' ? 'WebSocket实时同步' : '15秒轮询' }}</p></div><div class="side-card"><h2>地图与资源</h2><p>坐标格式：ABCD（行 + 列）</p><p>地图与剧本从服务器持久化内容目录载入。</p><a class="secondary-link" href="/api/docs" target="_blank">查看API与AI扩展文档 ↗</a></div></div></section>
        <section v-else-if="activePanel === 'aviation'" class="workspace-page"><span class="section-kicker">AVIATION COMMAND</span><h1>航空指挥界面</h1><div class="aviation-placeholder"><span>△</span><h2>航空作业尚未在当前沙盒启用</h2><p>后续将在此集中管理CAP、侦查、攻击编队、整备与甲板作业。所有航空判定仍由服务器执行。</p></div></section>
        <section v-else class="workspace-page"><span class="section-kicker">EVENT LOG</span><h1>日志信息</h1><div class="full-log"><div v-for="event in orderedEvents" :key="event.id" class="full-log-row"><span>T{{ String(event.turn).padStart(2, '0') }}</span><code>#{{ event.id }}</code><p>{{ event.text }}</p><time>{{ new Date(event.created_at).toLocaleTimeString() }}</time></div></div></section>
      </template>
    </div>

    <div v-if="menuCoord" class="fleet-dialog" @click.self="closeManager">
      <section class="fleet-modal fleet-manager">
        <header><div><span class="section-kicker">HEX {{ menuCoord }} · FLEET ORGANIZATION</span><h2>舰队整编</h2></div><button aria-label="关闭" @click="closeManager">×</button></header>
        <p class="manager-hint">选择“默认”将舰队置顶并设为操作对象。成员可以转移到其他舰队、拆出新舰队，也可以整支合并。</p>
        <div class="fleet-columns">
          <article v-for="fleet in menuFleets" :key="fleet.fleet_id" class="fleet-column" :class="{ default: selectedFleet?.fleet_id === fleet.fleet_id, 'merge-source': mergeSourceID === fleet.fleet_id, 'transfer-target': movingMemberID && !fleet.members.some((unit) => unit.id === movingMemberID) }">
            <header><div><span class="unit-type-badge">{{ fleet.type }}</span><h3>{{ fleet.name }}</h3><small>{{ fleet.count }}艘 · 强度{{ fleet.strength }}/{{ fleet.max_strength }}</small></div><button class="default-button" @click="selectedID = fleet.id">{{ selectedFleet?.fleet_id === fleet.fleet_id ? '默认' : '设为默认' }}</button></header>
            <div class="fleet-member-list">
              <div v-for="member in fleet.members" :key="member.id" class="fleet-member"><span class="unit-type-badge">{{ member.type }}</span><div><strong>{{ member.name }}</strong><small>{{ member.coord }} · 强度{{ member.strength }}/{{ member.max_strength }}</small></div><button @click="movingMemberID = movingMemberID === member.id ? '' : member.id">{{ movingMemberID === member.id ? '取消' : '转移' }}</button><button v-if="fleet.count > 1" @click="splitMember(member)">拆出</button></div>
            </div>
            <button v-if="movingMemberID && !fleet.members.some((unit) => unit.id === movingMemberID)" class="transfer-button" @click="transferMember(fleet.fleet_id)">转移到此舰队</button>
            <footer><button v-if="!mergeSourceID" @click="mergeSourceID = fleet.fleet_id">合并舰队</button><button v-else-if="mergeSourceID === fleet.fleet_id" disabled>当前整编源</button><button v-else class="confirm" @click="mergeDraftInto(fleet.fleet_id)">合并到此队</button></footer>
          </article>
        </div>
        <footer class="manager-actions"><button class="reset-button" @click="openManager(menuCoord)">重置</button><span>整编由服务器立即生效</span><button class="apply-button" :disabled="actionBusy" @click="applyReorganization">应用整编</button></footer>
      </section>
    </div>
  </main>
</template>
