<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { api } from '../api'
import { applyViewPatch, type ViewPatch } from '../patch'
import { contactSizeLabel } from '../fleet'
import { cycleKnots, speedKnots } from '../format'
import type { AirTask, AirfieldContact, GameEvent, GameView, Unit } from '../types'
import HexMap from '../components/HexMap.vue'
import ShipCard from '../components/ShipCard.vue'
import ShipCounter from '../components/ShipCounter.vue'
import AircraftCounter from '../components/AircraftCounter.vue'
import HexPanel from '../components/HexPanel.vue'

// 观战视图和玩家视图同构，只是没有 you/orders：双方内容都完整下发，页面只读。
type SpectatorView = Omit<GameView, 'you' | 'orders'> & { spectator?: boolean }

const props = defineProps<{ token: string }>()

const state = ref<SpectatorView | null>(null)
const status = ref<'connecting' | 'live' | 'error'>('connecting')
const problem = ref('')
const selectedUnitID = ref('')
const selectedFleetID = ref('')
const selectedAirTaskID = ref('')
let socket: WebSocket | undefined
let retryTimer: number | undefined
let retryDelay = 1000
let closed = false

const fleets = computed(() => state.value?.fleets || [])
const selectedFleet = computed(() => fleets.value.find((fleet) => fleet.fleet_id === selectedFleetID.value))
const selectedUnit = computed(() => {
  const byID = state.value?.units.find((unit) => unit.id === selectedUnitID.value)
  if (byID) return byID
  // 点整支舰队时默认展示旗舰。
  return selectedFleet.value?.members?.[0]
})
const fleetShips = computed(() => selectedFleet.value?.members || [])
// 点格子弹的"这一格有什么"清单：堆叠在同一格的舰队/机场/编队从这里挑，不再靠牌子错位去点。
const hexCoord = ref('')
const hexFleets = computed(() => fleets.value.filter((fleet) => fleet.coord === hexCoord.value))
const hexAirfields = computed(() => (state.value?.airfields || []).filter((airfield) => airfield.coord === hexCoord.value))
// 侦察算子有独立的小标记，清单里只列飞行编队（攻击/转场/掩护）。
const hexAirTasks = computed(() => [...tasks.value, ...groundTasks.value].filter((task) => task.kind !== 'recon' && task.current_coord === hexCoord.value))
function selectHex(coord: string) {
  clearDetailSelection()
  const hasContent = fleets.value.some((fleet) => fleet.coord === coord)
    || (state.value?.airfields || []).some((airfield) => airfield.coord === coord)
    || [...tasks.value, ...groundTasks.value].some((task) => task.kind !== 'recon' && task.current_coord === coord)
  hexCoord.value = hasContent ? coord : ''
}
// 航速用服务端算好的编队节奏（已经含"最慢一艘"和编队规模惩罚）；只有老快照缺字段时，
// 才按成员卡面自己折一个下界，免得面板上写成"约 节"。
const fleetKnots = computed(() => {
  const fleet = selectedFleet.value
  if (!fleet) return '—'
  if (fleet.move_knots) return fleet.move_knots
  const knots = (fleet.members || []).map((member) => (member.card.move_cycle?.length ? cycleKnots(member.card.move_cycle) : speedKnots(member.card.speed_class)))
  return knots.length ? Math.min(...knots) : '—'
})
// 航母作业只列真正的航母：BB/CA 的机库是水上飞机位（吊车），和航母甲板作业不是一回事。
const carriers = computed(() =>
  (state.value?.aviation?.carriers || []).filter((carrier) => ['CV', 'CVL', 'CVE'].includes(carrier.type)),
)
function carriersOf(faction: 'A' | 'B') {
  return carriers.value.filter((carrier) => (carrier.faction || 'A') === faction)
}
const tasks = computed(() => state.value?.aviation?.tasks || [])
const groundTasks = computed(() => state.value?.ground_tasks || [])
const events = computed<GameEvent[]>(() => [...(state.value?.events || [])].reverse())
const scores = computed(() => state.value?.scores || {})
const losses = computed(() => state.value?.losses || {})
const clockText = computed(() => {
  const clock = state.value?.clock
  if (!clock) return '—'
  return `第${clock.day}天 ${clock.time}`
})
const windText = computed(() => state.value?.wind?.wind || '—')
// 战果按阵营拆成两栏：以前两个阵营的战果串成一段长文，击沉一串挤在一起根本读不出来。
function scoreLinesFor(faction: 'A' | 'B') {
  return state.value?.score_breakdown?.[faction] || []
}

function factionName(faction?: string) {
  return faction === 'B' ? '日军' : '美军'
}

function fleetLabel(fleet: { name?: string; fleet_id?: string; members?: Unit[] }) {
  const leader = fleet.members?.[0]
  return leader?.fleet_name || fleet.name || fleet.fleet_id || '舰队'
}

function fleetTypeSummary(fleet: { members?: Unit[] }) {
  const counts = new Map<string, number>()
  for (const ship of fleet.members || []) counts.set(ship.type, (counts.get(ship.type) || 0) + 1)
  return [...counts.entries()].map(([type, count]) => `${type}×${count}`).join(' ')
}

function lossSummary(faction: 'A' | 'B') {
  const ledger = losses.value[faction] || {}
  const sunk = ledger.ships || []
  const aircraft = ledger.aircraft_total || 0
  return { sunk, aircraft, ships: ledger.ship_total || sunk.length }
}

// 详情面板同一时间只显示一种对象：舰队 / 机场 / 空中编队。选中任何一种都要把另外两种的
// 选择清掉，否则面板会一直停在旧对象上（点过机场或飞机，再点舰队就切不过去）。
function clearDetailSelection() {
  selectedFleetID.value = ''
  selectedUnitID.value = ''
  selectedAirTaskID.value = ''
  detailAirfield.value = null
  hexCoord.value = ''
}

function selectFleet(fleetID: string) {
  clearDetailSelection()
  selectedFleetID.value = fleetID
}

function selectUnit(unit: Unit) {
  clearDetailSelection()
  selectedFleetID.value = unit.fleet_id || unit.id
  selectedUnitID.value = unit.id
}

function selectAirfield(airfieldID: string) {
  const airfield = state.value?.airfields?.find((item) => item.id === airfieldID)
  if (airfield) selectAirTaskForAirfield(airfield)
}

// 直接把机场当作"编队详情"展示：名称、生命、岸炮、驻机。
function selectAirTaskForAirfield(airfield: AirfieldContact) {
  clearDetailSelection()
  detailAirfield.value = airfield
}

const detailAirfield = ref<AirfieldContact | null>(null)
// 机场牌子的详情要能看到"现在停着哪些飞机"：airfields 只给到驻机数，
// 真正的算子列表在 ground_air 里按 unit_id 对应，这里按机型 + 状态归并。
const groundAir = computed(() => state.value?.ground_air || [])
const detailAirfieldStatus = computed(() => {
  const airfield = detailAirfield.value
  if (!airfield) return null
  return groundAir.value.find((entry) => entry.unit_id === airfield.id) || null
})
const airfieldAirGroups = computed(() => {
  const entry = detailAirfieldStatus.value
  if (!entry) return []
  const groups = new Map<string, { model: string; status: string; count: number }>()
  for (const aircraft of entry.aircraft || []) {
    const status = aircraft.status || ""
    const key = aircraft.model + "|" + status
    const group = groups.get(key) || { model: aircraft.model, status, count: 0 }
    group.count += 1
    groups.set(key, group)
  }
  return [...groups.values()].sort((left, right) => left.model.localeCompare(right.model) || left.status.localeCompare(right.status))
})
function groundStatusLabel(status: string) {
  const labels: Record<string, string> = { hangar: "待命", ready: "整备完毕", service: "整备中", launching: "起飞中", attack: "出击中", cap: "CAP", recon: "侦察", cover: "掩护", returning: "返航", recovery: "着陆" }
  return labels[status] || status || "—"
}
const selectedAirTask = computed(() => tasks.value.find((task) => task.id === selectedAirTaskID.value) || groundTasks.value.find((task) => task.id === selectedAirTaskID.value))
// 舰队详情里补一行“空中掩护”：地图上那面 CAP 牌子在整张马里亚纳缩到 100% 时只有 7px 宽，
// 看不清字；面板里给出准确的 CAP／掩护架数。
// 船牌下面的空白：航母／战列舰的机库到底装了什么，就从这里补上。
// 观战视图的 aviation.carriers 已经把每艘带机库的船（CV/CVL/CVE/BB/BC/CA）连同
// 全部舰载机／水上飞机算子和甲板、机库占用一起下发。
const selectedShipAir = computed(() => {
  const unit = selectedUnit.value
  if (!unit) return null
  const entry = (state.value?.aviation?.carriers || []).find((item) => item.unit_id === unit.id)
  if (!entry) return null
  const groups = new Map<string, { model: string; status: string; count: number }>()
  for (const aircraft of entry.aircraft || []) {
    const status = aircraft.status || ""
    const key = aircraft.model + "|" + status
    const group = groups.get(key) || { model: aircraft.model, status, count: 0 }
    group.count += 1
    groups.set(key, group)
  }
  return {
    deckUsed: entry.deck_used ?? 0,
    deckCapacity: entry.deck_capacity ?? 0,
    hangarUsed: entry.hangar_used ?? 0,
    hangarCapacity: entry.hangar_capacity ?? 0,
    groups: [...groups.values()].sort((left, right) => left.model.localeCompare(right.model) || left.status.localeCompare(right.status)),
  }
})
const selectedFleetAirCover = computed(() => {
  const fleet = selectedFleet.value
  if (!fleet) return null
  let cap = 0
  let cover = 0
  let returning = 0
  for (const task of tasks.value) {
    if (task.kind !== "cap" && task.kind !== "cover") continue
    if (task.faction && task.faction !== fleet.faction) continue
    for (const aircraft of task.aircraft || []) {
      if (!aircraft.coord || aircraft.coord !== fleet.coord) continue
      const status = String(aircraft.status)
      if (status !== "cap" && status !== "cover") continue
      if (task.kind === "cover") cover += 1
      else cap += 1
      if (aircraft.returning) returning += 1
    }
  }
  if (!cap && !cover) return null
  return { cap, cover, returning }
})
const selectedReconSector = computed(() => selectedAirTask.value?.recon_sectors?.find((sector) => sector.aircraft_id === selectedAirTaskID.value))
// 距离环：和玩家页同一套口径，原点取侦察扇面 / 空中编队 / 选中舰队。
// 早期版本（独立的 watch.html）带距离环，并进 SPA 后漏了这个属性，观战页就量不了距离。
const distanceReference = computed(() => selectedReconSector.value?.distance_reference || selectedAirTask.value?.distance_reference || selectedFleet.value?.distance_reference)
const distanceOrigin = computed(() => {
  const sector = selectedReconSector.value
  if (sector) return sector.current_coord || sector.origin_coord || ''
  const task = selectedAirTask.value
  if (task) return task.current_coord || ''
  return selectedFleet.value?.coord || ''
})

function selectAirTask(taskID: string) {
  clearDetailSelection()
  selectedAirTaskID.value = taskID
}

function closeDetail() {
  clearDetailSelection()
}

function applySnapshot(message: { view?: unknown; patch?: unknown }) {
  if (message.view) {
    state.value = message.view as SpectatorView
    status.value = 'live'
    return
  }
  if (!message.patch) return
  if (!state.value) return
  applyViewPatch(state.value, message.patch as ViewPatch)
  status.value = 'live'
}

function connect(roomID: string) {
  if (closed) return
  const scheme = location.protocol === 'https:' ? 'wss:' : 'ws:'
  socket = new WebSocket(`${scheme}//${location.host}/ws?mode=spectator`)
  socket.onopen = () => {
    retryDelay = 1000
    socket?.send(JSON.stringify({ type: 'subscribe', room_id: roomID, token: props.token }))
  }
  socket.onmessage = (event) => {
    let message: { type?: string; view?: unknown; patch?: unknown; error?: string }
    try {
      message = JSON.parse(event.data) as typeof message
    } catch {
      return
    }
    if (message.type === 'snapshot') {
      applySnapshot(message)
      return
    }
    if (message.type === 'error') {
      // token 错误之类的终止性错误：不重连，直接把原因摆在页面上。
      status.value = 'error'
      problem.value = message.error || '观战连接被拒绝'
      closed = true
      socket?.close()
    }
  }
  socket.onclose = () => {
    if (closed) return
    status.value = state.value ? 'live' : 'connecting'
    problem.value = '连接中断，正在重连…'
    retryTimer = window.setTimeout(() => connect(roomID), retryDelay)
    retryDelay = Math.min(retryDelay * 2, 10000)
  }
}

onMounted(async () => {
  try {
    const resolved = await api.watchRoom(props.token)
    // 先 HTTP 拉一份全量当底座：WS 抖动时也能马上看到战场，之后靠推送补差。
    state.value = (await api.spectate(resolved.room_id, props.token)) as unknown as SpectatorView
    status.value = 'live'
    connect(resolved.room_id)
  } catch (error) {
    status.value = 'error'
    problem.value = error instanceof Error ? error.message : '观战链接无效'
  }
})

onUnmounted(() => {
  closed = true
  if (retryTimer) window.clearTimeout(retryTimer)
  socket?.close()
})
</script>

<template>
  <div class="watch-page">
    <header class="watch-head">
      <div class="watch-title">
        <span class="kicker">SPECTATOR · 只读 · 点舰队看编成与距离圈</span>
        <h1>{{ state?.scenario?.name || '正在连接…' }}</h1>
      </div>
      <div class="watch-stats">
        <div><b>{{ state?.turn ?? '—' }}</b><span>回合</span></div>
        <div><b>{{ clockText }}</b><span>剧本时间</span></div>
        <div><b>{{ windText }}</b><span>风向</span></div>
        <div class="watch-status" :class="status">{{ status === 'live' ? '已连接' : status === 'error' ? '出错' : '连接中' }}</div>
      </div>
    </header>

    <p v-if="problem" class="watch-problem" :class="{ fatal: status === 'error' }">{{ problem }}</p>

    <main v-if="state" class="watch-body">
      <section class="watch-map">
        <HexMap
          :map="state.map"
          :fleets="fleets"
          player-faction="A"
          :selected-unit="selectedUnit"
          :orders="[]"
          :recon-tasks="[...tasks, ...groundTasks].filter((task) => task.kind === 'recon')"
          :air-tasks="tasks"
          :airfields="state.airfields || []"
          :ground-tasks="groundTasks"
          :enemy-air="[]"
          :distance-reference="distanceReference"
          :distance-origin="distanceOrigin"
          :selected-air-task-id="selectedAirTaskID"
          locked
          spectate
          :wind-cells="state.wind?.cells || ''"
          :weather-cells="state.weather_cells || ''"
          @select-unit="selectUnit"
          @select-airfield="selectAirfield"
          @inspect-airfield="selectAirfield"
          @select-air-task="selectAirTask"
          @select-hex="selectHex"
        />
        <HexPanel
          v-if="hexCoord"
          :coord="hexCoord"
          :fleets="hexFleets"
          :airfields="hexAirfields"
          :air-tasks="hexAirTasks"
          player-faction="A"
          spectate
          @close="hexCoord = ''"
          @select-fleet="selectFleet"
          @select-airfield="selectAirfield"
          @select-air-task="selectAirTask"
        />

        <aside v-if="selectedFleet || detailAirfield || selectedAirTask" class="watch-detail">
          <header>
            <div>
              <span class="kicker">{{ detailAirfield ? 'AIRFIELD' : selectedAirTask ? 'AIR FORMATION' : 'FORMATION' }}</span>
              <h2 v-if="detailAirfield">{{ detailAirfield.name }}</h2>
              <h2 v-else-if="selectedAirTask">{{ selectedAirTask.name }}</h2>
              <h2 v-else>{{ fleetLabel(selectedFleet!) }}</h2>
            </div>
            <button type="button" aria-label="关闭详情" @click="closeDetail">×</button>
          </header>

          <template v-if="detailAirfield">
            <dl class="watch-meta">
              <div><dt>阵营</dt><dd>{{ factionName(detailAirfield.faction) }}</dd></div>
              <div><dt>坐标</dt><dd>{{ detailAirfield.coord }}</dd></div>
              <div><dt>生命</dt><dd>{{ detailAirfield.strength }} / {{ detailAirfield.max_strength }}</dd></div>
              <div><dt>岸炮</dt><dd>{{ detailAirfield.coastal }}</dd></div>
              <div><dt>驻机</dt><dd>{{ detailAirfield.hangar_used ?? 0 }} 架</dd></div>
            </dl>
            <div v-if="detailAirfieldStatus" class="watch-airfield-air">
              <p v-if="!airfieldAirGroups.length" class="watch-empty">机场上没有飞机</p>
              <ul v-else>
                <li v-for="group in airfieldAirGroups" :key="group.model + group.status">
                  <span>{{ group.model }}</span><b>×{{ group.count }}</b><small>{{ groundStatusLabel(group.status) }}</small>
                </li>
              </ul>
            </div>
          </template>

          <template v-else-if="selectedAirTask">
            <dl class="watch-meta">
              <div><dt>任务</dt><dd>{{ selectedAirTask.kind }} · {{ selectedAirTask.phase }}</dd></div>
              <div><dt>位置</dt><dd>{{ selectedAirTask.current_coord || selectedAirTask.origin_coord || '—' }}</dd></div>
              <div><dt>目标</dt><dd>{{ selectedAirTask.target_coord || '未设置' }}</dd></div>
              <div><dt>兵力</dt><dd>{{ selectedAirTask.airborne_count }} / {{ selectedAirTask.aircraft_ids.length }} 升空</dd></div>
            </dl>
            <div class="watch-air-strip">
              <AircraftCounter
                v-for="aircraft in selectedAirTask.aircraft"
                :key="aircraft.id"
                :aircraft="aircraft"
                :player-faction="aircraft.faction"
                :cycle-clickable="false"
              />
            </div>
          </template>

          <template v-else>
            <dl class="watch-meta">
              <div><dt>阵营</dt><dd>{{ factionName(selectedFleet?.faction) }}</dd></div>
              <div><dt>坐标</dt><dd>{{ selectedFleet?.coord || '—' }}</dd></div>
              <div><dt>舰船</dt><dd>{{ fleetShips.length }}</dd></div>
              <div><dt>情报</dt><dd>{{ selectedFleet?.contact_level ? selectedFleet?.contact_level + ' 级' : '完整' }}</dd></div>
              <div class="watch-meta-wide"><dt>航速</dt><dd>约 {{ fleetKnots }} 节<template v-if="selectedFleet?.move_interval">（每 {{ selectedFleet?.move_interval }} 回合 1 格）</template></dd></div>
              <div class="watch-meta-wide" title="混乱值：每回合 -1；混乱中不能整编、不能岸轰，炮击与鱼雷命中按自身混乱每点 -5%"><dt>混乱</dt><dd :class="{ 'watch-confusion-hot': (selectedFleet?.confusion || 0) > 0 }">{{ selectedFleet?.confusion || 0 }}</dd></div>
              <div v-if="selectedFleetAirCover" class="watch-meta-wide"><dt>空中掩护</dt><dd>CAP {{ selectedFleetAirCover.cap }} · 掩护 {{ selectedFleetAirCover.cover }}<template v-if="selectedFleetAirCover.returning">（返航 {{ selectedFleetAirCover.returning }}）</template></dd></div>
            </dl>
            <div class="watch-fleet-grid">
              <button
                v-for="ship in fleetShips"
                :key="ship.id"
                type="button"
                class="watch-ship"
                :class="{ selected: ship.id === selectedUnit?.id }"
                @click="selectedUnitID = ship.id"
              >
                <ShipCounter :unit="ship" />
                <small>{{ ship.name }}</small>
              </button>
            </div>
            <ShipCard v-if="selectedUnit" :unit="selectedUnit" />
            <section v-if="selectedShipAir" class="watch-ship-air">
              <header>
                <span>舰载机 / 机库</span>
                <small>
                  <template v-if="selectedUnit?.type === 'CV' || selectedUnit?.type === 'CVL' || selectedUnit?.type === 'CVE'">甲板 {{ selectedShipAir.deckUsed }}/{{ selectedShipAir.deckCapacity }} · </template>机库 {{ selectedShipAir.hangarUsed }}/{{ selectedShipAir.hangarCapacity }}
                </small>
              </header>
              <p v-if="!selectedShipAir.groups.length" class="watch-empty">舰上没有飞机</p>
              <ul v-else>
                <li v-for="group in selectedShipAir.groups" :key="group.model + group.status">
                  <span>{{ group.model }}</span><b>×{{ group.count }}</b><small>{{ groundStatusLabel(group.status) }}</small>
                </li>
              </ul>
            </section>
          </template>
        </aside>
      </section>

      <aside class="watch-side">
        <section class="watch-card">
          <h2>战况</h2>
          <div class="watch-scoreboard">
            <section v-for="faction in ['A', 'B'] as const" :key="faction" class="watch-score-col">
              <header>
                <b>{{ factionName(faction) }}</b>
                <strong>{{ scores[faction] ?? 0 }}</strong>
              </header>
              <small>VP · 沉 {{ lossSummary(faction).ships }} / 机 {{ lossSummary(faction).aircraft }}</small>
              <ul v-if="scoreLinesFor(faction).length" class="watch-score-lines">
                <li v-for="(line, index) in scoreLinesFor(faction)" :key="line.text + index">
                  <span>{{ line.text }}</span><b>+{{ line.points }}</b>
                </li>
              </ul>
              <p v-else class="watch-empty">暂无战果</p>
            </section>
          </div>
          <p v-if="state.outcome" class="watch-outcome">{{ state.outcome.reason }}</p>
          <ul v-if="state.objectives?.length" class="watch-objectives">
            <li v-for="objective in state.objectives" :key="objective.id">
              {{ objective.text || objective.id }}<b v-if="objective.value">（{{ objective.value }}）</b>
            </li>
          </ul>
        </section>

        <section class="watch-card">
          <h2>舰队</h2>
          <div v-for="faction in ['A', 'B'] as const" :key="faction" class="watch-fleet-group">
            <h3>{{ factionName(faction) }} · {{ fleets.filter((fleet) => fleet.faction === faction).length }} 支</h3>
            <button
              v-for="fleet in fleets.filter((item) => item.faction === faction)"
              :key="fleet.fleet_id"
              type="button"
              class="watch-fleet-row"
              :class="{ selected: fleet.fleet_id === selectedFleetID }"
              @click="selectFleet(fleet.fleet_id)"
            >
              <span>{{ fleetLabel(fleet) }}</span>
              <small>{{ fleet.count }} 艘 · {{ fleetTypeSummary(fleet) || contactSizeLabel(fleet.contact_size) }} · {{ fleet.coord }}</small>
            </button>
          </div>
        </section>

        <section class="watch-card">
          <h2>航母作业</h2>
          <div v-if="!carriers.length" class="watch-empty">暂无航母</div>
          <template v-for="faction in ['A', 'B'] as const" :key="faction">
            <h3 v-if="carriersOf(faction).length">{{ factionName(faction) }} · {{ carriersOf(faction).length }} 艘</h3>
            <div
              v-for="carrier in carriersOf(faction)"
              :key="carrier.unit_id"
              class="watch-carrier"
              :class="`ship-${faction.toLowerCase()}`"
            >
              <span>{{ carrier.name }} <em>{{ carrier.type }}</em></span>
              <small>机库 {{ carrier.hangar_used }}/{{ carrier.hangar_capacity }} · 甲板 {{ carrier.deck_used }}/{{ carrier.deck_length }} · {{ carrier.blocked_reason || carrier.operation || '正常' }}</small>
            </div>
          </template>
          <div v-if="tasks.length" class="watch-task-list">
            <h3>在编任务</h3>
            <button
              v-for="task in tasks"
              :key="task.id"
              type="button"
              class="watch-task"
              :class="{ selected: task.id === selectedAirTaskID }"
              @click="selectAirTask(task.id)"
            >
              <span>{{ task.name }}</span>
              <small>{{ task.kind }} · {{ task.phase }} · {{ task.airborne_count }}/{{ task.aircraft_ids.length }} · {{ task.current_coord || '—' }}</small>
            </button>
          </div>
        </section>

        <section class="watch-card watch-events">
          <h2>战报</h2>
          <ol>
            <li v-for="event in events.slice(0, 80)" :key="event.id">
              <b>T{{ event.turn }}</b>{{ event.text }}
            </li>
          </ol>
        </section>
      </aside>
    </main>

    <div v-else class="watch-loading">{{ status === 'error' ? '观战链接无效' : '正在读取战场…' }}</div>
  </div>
</template>

<style scoped>
.watch-page { min-height: 100vh; display: flex; flex-direction: column; background: var(--bg, #091418); color: var(--text, #dce7e3); }
.watch-head { display: flex; justify-content: space-between; gap: 16px; align-items: flex-end; padding: 14px 18px; border-bottom: 1px solid rgba(255,255,255,.08); }
.kicker { font: 600 11px/1 'Rajdhani', system-ui, sans-serif; letter-spacing: .18em; color: #8fb3ad; text-transform: uppercase; }
.watch-title h1 { margin: 4px 0 0; font-size: 20px; }
.watch-stats { display: flex; gap: 18px; align-items: center; }
.watch-stats > div { display: flex; flex-direction: column; align-items: center; gap: 2px; min-width: 56px; }
.watch-stats b { font: 700 16px/1 'Rajdhani', system-ui, sans-serif; }
.watch-stats span { font-size: 11px; opacity: .65; }
.watch-status { padding: 4px 10px; border-radius: 999px; border: 1px solid rgba(255,255,255,.15); font-size: 12px; }
.watch-status.live { color: #7ddca4; border-color: #2f7d55; }
.watch-status.error { color: #f08a7c; border-color: #8a3a31; }
.watch-problem { margin: 10px 18px 0; color: #e6b35c; font-size: 13px; }
.watch-problem.fatal { color: #f08a7c; }
.watch-body { flex: 1; display: grid; grid-template-columns: minmax(0, 1fr) 360px; gap: 12px; padding: 12px; min-height: 0; }
.watch-map { position: relative; min-height: 60vh; border: 1px solid rgba(255,255,255,.07); border-radius: 10px; overflow: hidden; background: #0b1c22; }
.watch-detail { position: absolute; top: 52px; right: 12px; width: min(300px, 36%); max-height: calc(100% - 64px); overflow: auto; background: rgba(9, 22, 27, .97); border: 1px solid rgba(255,255,255,.12); border-radius: 10px; padding: 8px 10px; box-shadow: 0 18px 40px rgba(0,0,0,.45); }
.watch-detail > header { display: flex; justify-content: space-between; gap: 10px; align-items: flex-start; }
.watch-detail h2 { margin: 3px 0 0; font-size: 14px; }
.watch-detail header button { background: none; border: 1px solid rgba(255,255,255,.15); color: inherit; border-radius: 6px; width: 26px; height: 26px; cursor: pointer; }
.watch-meta { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 4px 10px; margin: 8px 0; }
.watch-meta div { display: flex; justify-content: space-between; gap: 6px; font-size: 11px; border-bottom: 1px dashed rgba(255,255,255,.08); padding-bottom: 3px; }
.watch-meta dt { opacity: .6; white-space: nowrap; }
/* 航速、空中掩护这类一行放不下两栏的条目独占整行，否则"航速"会被挤成竖排两个字。 */
.watch-meta .watch-meta-wide { grid-column: 1 / -1; }
/* 有混乱时数字转橙：混乱期间不能整编、不能岸轰，命中还要吃惩罚，不能跟 0 一样灰着。 */
.watch-meta .watch-confusion-hot { color: #f0a53a; }
.watch-meta dd { margin: 0; font-weight: 600; }
.watch-fleet-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(72px, 1fr)); gap: 5px; }
/* 兵牌原尺寸 126px 宽，观战面板里按 0.58 缩小，一屏能看完整支编队而不是翻好几页。 */
.watch-ship :deep(.ship-counter) { zoom: .58; }
/* 兵牌自带的舰名跟着 0.58 缩到 6px，等于没有；观战格子里藏掉它，改用下面那行不缩放的 9px 舰名。 */
.watch-fleet-grid :deep(.ship-counter figcaption) { display: none; }
.watch-air-strip :deep(.air-counter) { zoom: .74; }
.watch-ship { display: flex; flex-direction: column; gap: 3px; padding: 3px; background: none; border: 1px solid transparent; border-radius: 8px; cursor: pointer; color: inherit; }
.watch-ship.selected { border-color: #58b6c9; background: rgba(88,182,201,.1); }
.watch-ship small { font-size: 9px; line-height: 1.2; opacity: .75; text-align: center; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.watch-air-strip { display: flex; flex-wrap: wrap; gap: 8px; }
.watch-ship-air { margin-top: 8px; padding: 8px; border: 1px solid rgba(255,255,255,.08); border-radius: 8px; background: rgba(255,255,255,.02); }
.watch-ship-air header { display: flex; align-items: baseline; justify-content: space-between; gap: 8px; margin-bottom: 6px; }
.watch-ship-air header span { font: 600 10px 'Rajdhani', sans-serif; letter-spacing: .12em; color: #9fc4be; }
.watch-ship-air header small { font-size: 10px; opacity: .6; }
.watch-ship-air ul { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 3px; }
.watch-ship-air li { display: flex; align-items: baseline; gap: 6px; padding: 3px 6px; border-radius: 5px; background: rgba(255,255,255,.04); font-size: 11px; }
.watch-ship-air li span { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.watch-ship-air li b { font: 600 12px 'Rajdhani', sans-serif; color: #9fd7ea; }
.watch-ship-air li small { opacity: .6; font-size: 10px; }
/* 观战面板窄：把共用船牌的内边距压紧，省掉大段空白 */
.watch-detail :deep(.ship-card) { margin-top: 10px; }
.watch-detail :deep(.ship-card-head) { padding: 8px 10px; }
.watch-detail :deep(.ship-card-head h3) { font-size: 15px; }
.watch-detail :deep(.ship-identity) { padding: 8px 10px; }
.watch-detail :deep(.ship-stats) { padding: 2px 10px; }
.watch-detail :deep(.ship-stats>div) { padding: 5px 0; }
.watch-detail :deep(.carrier-systems) { padding: 6px 10px; }
.watch-detail :deep(.ship-card>footer) { padding: 6px 10px; }
.watch-airfield-air ul { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 3px; }
.watch-airfield-air li { display: flex; align-items: baseline; gap: 6px; padding: 3px 6px; border-radius: 5px; background: rgba(255,255,255,.04); font-size: 11px; }
.watch-airfield-air li span { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.watch-airfield-air li b { font: 600 12px 'Rajdhani', sans-serif; color: #9fd7ea; }
.watch-airfield-air li small { opacity: .6; font-size: 10px; }
.watch-side { min-height: 0; overflow: auto; display: flex; flex-direction: column; gap: 10px; padding-right: 2px; }
.watch-card { border: 1px solid rgba(255,255,255,.08); border-radius: 10px; padding: 10px 12px; background: rgba(255,255,255,.02); }
.watch-card h2 { margin: 0 0 8px; font-size: 13px; letter-spacing: .08em; color: #9fc4be; }
.watch-card h3 { margin: 8px 0 4px; font-size: 11px; color: #7fa39d; }
/* 战果两栏：左美军、右日军，各自一条一条列，长文串成一段根本没法读 */
.watch-scoreboard { display: grid; grid-template-columns: 1fr 1fr; gap: 8px; }
.watch-score-col { display: flex; flex-direction: column; gap: 3px; min-width: 0; padding: 7px 8px; border-radius: 8px; background: rgba(255,255,255,.04); }
.watch-score-col header { display: flex; align-items: baseline; justify-content: space-between; gap: 6px; }
.watch-score-col header b { font-size: 11px; font-weight: 600; color: #9fc4be; }
.watch-score-col header strong { font: 700 19px/1 'Rajdhani', system-ui, sans-serif; }
.watch-score-col small { font-size: 10px; opacity: .7; }
/* 两栏不等高很正常（一边打死得多），列表不再单独滚动：整块侧栏本来就能滚，
   里面再套一层滚动条反而滚不到底。 */
.watch-score-lines { list-style: none; display: flex; flex-direction: column; gap: 2px; margin: 4px 0 0; padding: 6px 0 0; border-top: 1px dashed rgba(255,255,255,.12); font-size: 10px; }
.watch-score-lines li { display: flex; justify-content: space-between; gap: 6px; line-height: 1.45; }
.watch-score-lines li span { min-width: 0; opacity: .85; }
.watch-score-lines li b { flex: none; color: #f0d58d; font-weight: 600; }
.watch-outcome { margin: 8px 0 0; font-size: 12px; color: #f0c274; }
.watch-objectives { list-style: none; margin: 8px 0 0; padding: 0; font-size: 11px; display: flex; flex-direction: column; gap: 3px; }
.watch-objectives li { opacity: .8; }
.watch-fleet-group { margin-bottom: 8px; }
.watch-fleet-row, .watch-carrier, .watch-task { width: 100%; display: flex; flex-direction: column; gap: 2px; text-align: left; padding: 5px 6px; margin: 2px 0; border: 1px solid transparent; border-radius: 6px; background: none; color: inherit; cursor: pointer; }
.watch-fleet-row:hover, .watch-carrier:hover, .watch-task:hover { background: rgba(255,255,255,.04); }
.watch-fleet-row.selected, .watch-task.selected { border-color: #58b6c9; background: rgba(88,182,201,.1); }
.watch-fleet-row small, .watch-carrier small, .watch-task small { font-size: 11px; opacity: .65; }
.watch-carrier em { font-style: normal; font-size: 10px; opacity: .55; margin-left: 4px; }
.watch-carrier.ship-a span { color: #9fd7ea; }
.watch-carrier.ship-b span { color: #f0b3a6; }
.watch-empty { font-size: 12px; opacity: .55; }
.watch-task-list { margin-top: 8px; border-top: 1px dashed rgba(255,255,255,.08); padding-top: 6px; }
.watch-events ol { list-style: none; margin: 0; padding: 0; max-height: 260px; overflow: auto; display: flex; flex-direction: column; gap: 3px; font-size: 11px; }
.watch-events li b { color: #7fa39d; margin-right: 6px; font-weight: 600; }
.watch-loading { flex: 1; display: grid; place-items: center; font-size: 14px; opacity: .7; }
@media (max-width: 1100px) { .watch-body { grid-template-columns: 1fr; } .watch-side { max-height: none; } .watch-detail { width: calc(100% - 24px); max-width: 420px; } }
</style>
