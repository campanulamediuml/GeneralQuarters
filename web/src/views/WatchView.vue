<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { api } from '../api'
import { applyViewPatch, type ViewPatch } from '../patch'
import { contactSizeLabel } from '../fleet'
import type { AirTask, AirfieldContact, GameEvent, GameView, Unit } from '../types'
import HexMap from '../components/HexMap.vue'
import ShipCard from '../components/ShipCard.vue'
import ShipCounter from '../components/ShipCounter.vue'
import AircraftCounter from '../components/AircraftCounter.vue'

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
const scoreLines = computed(() => {
  const breakdown = state.value?.score_breakdown || {}
  return (['A', 'B'] as const).map((faction) => ({ faction, lines: breakdown[faction] || [] })).filter((row) => row.lines.length)
})

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

function selectFleet(fleetID: string) {
  selectedFleetID.value = fleetID
  selectedUnitID.value = ''
}

function selectUnit(unit: Unit) {
  selectedFleetID.value = unit.fleet_id || unit.id
  selectedUnitID.value = unit.id
}

function selectAirfield(airfieldID: string) {
  const airfield = state.value?.airfields?.find((item) => item.id === airfieldID)
  if (airfield) selectAirTaskForAirfield(airfield)
}

// 直接把机场当作"编队详情"展示：名称、生命、岸炮、驻机。
function selectAirTaskForAirfield(airfield: AirfieldContact) {
  selectedFleetID.value = ''
  selectedUnitID.value = ''
  detailAirfield.value = airfield
}

const detailAirfield = ref<AirfieldContact | null>(null)
const selectedAirTask = computed(() => tasks.value.find((task) => task.id === selectedAirTaskID.value) || groundTasks.value.find((task) => task.id === selectedAirTaskID.value))

function selectAirTask(taskID: string) {
  selectedAirTaskID.value = taskID
}

function closeDetail() {
  selectedFleetID.value = ''
  selectedUnitID.value = ''
  detailAirfield.value = null
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
        <span class="kicker">SPECTATOR · 只读</span>
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
          :selected-air-task-id="selectedAirTaskID"
          locked
          spectate
          :wind-cells="state.wind?.cells || ''"
          :weather-cells="state.weather_cells || ''"
          @select-unit="selectUnit"
          @select-airfield="selectAirfield"
          @inspect-airfield="selectAirfield"
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
              <div><dt>驻机</dt><dd>{{ detailAirfield.hangar_used ?? 0 }} / {{ detailAirfield.hangar_capacity ?? '—' }}</dd></div>
            </dl>
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
          </template>
        </aside>
      </section>

      <aside class="watch-side">
        <section class="watch-card">
          <h2>战况</h2>
          <div class="watch-scores">
            <div v-for="faction in ['A', 'B'] as const" :key="faction">
              <b>{{ factionName(faction) }}</b>
              <strong>{{ scores[faction] ?? 0 }}</strong>
              <small>VP · 沉 {{ lossSummary(faction).ships }} / 机 {{ lossSummary(faction).aircraft }}</small>
            </div>
          </div>
          <p v-if="state.outcome" class="watch-outcome">{{ state.outcome.reason }}</p>
          <ul v-if="scoreLines.length" class="watch-score-lines">
            <li v-for="row in scoreLines" :key="row.faction">
              <span>{{ factionName(row.faction) }}：</span>
              <em v-for="line in row.lines" :key="line.text">{{ line.text }} +{{ line.points }}</em>
            </li>
          </ul>
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
.watch-detail { position: absolute; top: 12px; right: 12px; width: min(460px, 52%); max-height: calc(100% - 24px); overflow: auto; background: rgba(9, 22, 27, .97); border: 1px solid rgba(255,255,255,.12); border-radius: 10px; padding: 12px; box-shadow: 0 18px 40px rgba(0,0,0,.45); }
.watch-detail > header { display: flex; justify-content: space-between; gap: 10px; align-items: flex-start; }
.watch-detail h2 { margin: 4px 0 0; font-size: 16px; }
.watch-detail header button { background: none; border: 1px solid rgba(255,255,255,.15); color: inherit; border-radius: 6px; width: 26px; height: 26px; cursor: pointer; }
.watch-meta { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 6px 12px; margin: 10px 0; }
.watch-meta div { display: flex; justify-content: space-between; gap: 8px; font-size: 12px; border-bottom: 1px dashed rgba(255,255,255,.08); padding-bottom: 4px; }
.watch-meta dt { opacity: .6; }
.watch-meta dd { margin: 0; font-weight: 600; }
.watch-fleet-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(112px, 1fr)); gap: 8px; }
.watch-ship { display: flex; flex-direction: column; gap: 4px; padding: 4px; background: none; border: 1px solid transparent; border-radius: 8px; cursor: pointer; color: inherit; }
.watch-ship.selected { border-color: #58b6c9; background: rgba(88,182,201,.1); }
.watch-ship small { font-size: 11px; opacity: .75; }
.watch-air-strip { display: flex; flex-wrap: wrap; gap: 8px; }
.watch-side { min-height: 0; overflow: auto; display: flex; flex-direction: column; gap: 10px; padding-right: 2px; }
.watch-card { border: 1px solid rgba(255,255,255,.08); border-radius: 10px; padding: 10px 12px; background: rgba(255,255,255,.02); }
.watch-card h2 { margin: 0 0 8px; font-size: 13px; letter-spacing: .08em; color: #9fc4be; }
.watch-card h3 { margin: 8px 0 4px; font-size: 11px; color: #7fa39d; }
.watch-scores { display: grid; grid-template-columns: 1fr 1fr; gap: 8px; }
.watch-scores > div { display: flex; flex-direction: column; gap: 2px; padding: 6px 8px; border-radius: 8px; background: rgba(255,255,255,.04); }
.watch-scores strong { font: 700 20px/1 'Rajdhani', system-ui, sans-serif; }
.watch-scores small { font-size: 11px; opacity: .7; }
.watch-outcome { margin: 8px 0 0; font-size: 12px; color: #f0c274; }
.watch-score-lines, .watch-objectives { list-style: none; margin: 8px 0 0; padding: 0; font-size: 11px; display: flex; flex-direction: column; gap: 3px; }
.watch-score-lines em { font-style: normal; opacity: .8; margin-left: 6px; }
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
@media (max-width: 1100px) { .watch-body { grid-template-columns: 1fr; } .watch-side { max-height: none; } }
</style>
