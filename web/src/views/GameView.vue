<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '../api'
import type { AirCombatReport, GameView, ReconHeading, StrikeTarget, Unit } from '../types'
import { contactCapitalsLabel, contactIntelLabel, contactSizeLabel, groupFleets, type FleetGroup } from '../fleet'
import HexMap from '../components/HexMap.vue'
import ShipCard from '../components/ShipCard.vue'
import ShipCounter from '../components/ShipCounter.vue'
import AviationPanel from '../components/AviationPanel.vue'
import ResultsPanel from '../components/ResultsPanel.vue'
import AirCombatPanel from '../components/AirCombatPanel.vue'
import AirfieldCounter from '../components/AirfieldCounter.vue'

const props = defineProps<{ roomID: string }>()
const router = useRouter()
const state = ref<GameView | null>(null)
const selectedID = ref('')
const shipDetailOpen = ref(false)
const selectedAirTaskID = ref('')
const strikeTargetTaskID = ref('')
const menuCoord = ref('')
const fleetChoiceCoord = ref('')
const mergeSourceID = ref('')
const movingMemberID = ref('')
const draftAssignments = ref<Record<string, string>>({})
const activePanel = ref<'map' | 'rules' | 'settings' | 'aviation' | 'airCombat' | 'log' | 'results'>('map')
const activeAirCombat = ref<AirCombatReport | null>(null)
const counterSide = ref<'front' | 'back'>('front')
const aviationFocusID = ref('')
const aviationFleetID = ref('')
// 选中的机场（机场与舰队共用航空界面，靠它切换数据源）
const selectedAirfieldID = ref('')
const loading = ref(true)
const actionBusy = ref(false)
const error = ref('')
const socketStatus = ref<'connecting' | 'online' | 'offline'>('connecting')
const copied = ref(false)
let socket: WebSocket | undefined
let poll: ReturnType<typeof setInterval> | undefined
let refreshTimer: ReturnType<typeof setTimeout> | undefined
let stateRequest = 0
let lastAirCombatID = ''

const ownUnits = computed(() => state.value?.units.filter((unit) => unit.faction === state.value?.you.faction) || [])
const fleets = computed(() => state.value?.fleets || [])
const ownFleets = computed(() => fleets.value.filter((fleet) => fleet.faction === state.value?.you.faction))
const strikeTargets = computed<StrikeTarget[]>(() => fleets.value.filter((fleet) => fleet.faction !== state.value?.you.faction).map((fleet) => ({
  coord: fleet.coord,
  label: `${fleet.fleet_name || fleet.name} · ${fleet.count}艘 · ${fleet.coord}${fleet.contact_level ? ` · 情报${fleet.contact_level}` : ''}`,
  count: fleet.count,
  contact_level: fleet.contact_level,
})))
const selectedUnit = computed(() => (state.value?.units || []).find((unit) => unit.id === selectedID.value))
const selectedFleet = computed(() => ownFleets.value.find((fleet) => fleet.id === selectedID.value || fleet.members.some((unit) => unit.id === selectedID.value)))
// 敌方舰队只读查看：情报到 4 级时 members 里有完整编成，点开应该能看见具体舰船和状态，
// 之前这里只在自己舰队里找，点敌方等于什么都没选中。
// 敌军面板里 1–3 级用一句话解释"现在能确认什么"，4 级才铺开兵牌。
function enemyIntelNote(fleet: FleetGroup) {
  const level = fleet.contact_level || 0
  if (level <= 1) return '只确认这片海域有敌方舰队，规模与编成未知。'
  if (level === 2) return `估计为${contactSizeLabel(fleet.contact_size) || '未知规模'}，舰种与编成未知。`
  const capitals = contactCapitalsLabel(fleet.contact_capitals)
  return capitals ? `已判明编成中有${capitals}，具体舰名与舰况未知。` : '已判明有水面舰艇编成，但没有确认到主力舰。'
}

const inspectedFleet = computed(() => fleets.value.find((fleet) => fleet.faction !== state.value?.you.faction && (fleet.id === selectedID.value || fleet.members.some((unit) => unit.id === selectedID.value))))
const canOpenAviation = computed(() => Boolean(selectedFleet.value?.members.some((unit) => unit.card.hangar_capacity > 0)) || Boolean(selectedAirfield.value))
// 地图上可见的机场（己方全部 + 已侦察发现的敌方机场）
const airfields = computed(() => state.value?.airfields || [])
const myAirfields = computed(() => state.value?.ground_air || [])
const selectedAirfield = computed(() => myAirfields.value.find((airfield) => airfield.unit_id === selectedAirfieldID.value))
const aviationFleetId = computed(() => (selectedAirfield.value ? selectedAirfield.value.unit_id : aviationFleetID.value))
const aviationCarriers = computed(() => (selectedAirfield.value ? groundCarrierView() : state.value?.aviation))
const groundTasks = computed(() => state.value?.ground_tasks || [])
const groundAircraft = computed(() => state.value?.ground_aircraft || [])
// 把机场整备状态投影成航空面板认识的 CarrierAirStatus，二者共用同一套界面
function groundCarrierView() {
  const carriers = myAirfields.value.map((airfield) => ({
    unit_id: airfield.unit_id,
    name: airfield.name,
    type: airfield.type,
    hangar_used: airfield.hangar_used,
    hangar_capacity: airfield.aircraft.length,
    deck_length: airfield.runways,
    deck_capacity: airfield.max_runways,
    deck_used: airfield.aircraft.filter((item) => ['deck', 'ready', 'recovery'].includes(item.status)).length,
    launch_used: 0,
    operation: '' as const,
    elevators: [],
    aircraft: airfield.aircraft.map((item) => Object.assign({}, item, { carrier_id: airfield.unit_id, name: item.model, seaplane: false, launched: false, returning: Boolean(item.returning) }) as unknown as import('../types').AirUnit),
  }))
  // 任务携带所属机场（fleet_id），航空面板据此按机场过滤
  return { carriers, tasks: (groundTasks.value || []).map((task) => ({ ...task, fleet_id: task.carrier_id || task.carrier_ids?.[0] || '' })) }
}
function selectAirfield(airfieldID: string) {
  selectedAirfieldID.value = airfieldID
  selectedID.value = ''
  shipDetailOpen.value = false
  openAviation(airfieldID)
}
const fleetPanelBottom = computed(() => {
  if (!state.value || !selectedFleet.value) return false
  const row = Number(selectedFleet.value.coord.slice(0, 2))
  return row <= Math.ceil(state.value.map.rows / 2)
})
const selectedAirTask = computed(() => state.value?.aviation.tasks.find((task) => task.id === selectedAirTaskID.value || task.aircraft_ids.includes(selectedAirTaskID.value)))
const selectedReconSector = computed(() => selectedAirTask.value?.recon_sectors?.find((sector) => sector.aircraft_id === selectedAirTaskID.value))
const distanceReference = computed(() => selectedReconSector.value?.distance_reference || selectedAirTask.value?.distance_reference || selectedFleet.value?.distance_reference)
// 距离环的原点：优先侦察扇面/空中编队，其次选中舰队。前端按这个原点把整张图的
// hex 距离现算出来，不再受服务端 8 格的限制。
const distanceOrigin = computed(() => {
  const sector = selectedReconSector.value
  if (sector) return sector.current_coord || sector.origin_coord || ''
  const task = selectedAirTask.value
  if (task) return task.current_coord || ''
  return selectedFleet.value?.coord || ''
})
const fleetChoices = computed(() => ownFleets.value.filter((fleet) => fleet.coord === fleetChoiceCoord.value))
const menuFleets = computed(() => groupFleets(ownUnits.value.filter((unit) => unit.coord === menuCoord.value).map((unit) => ({ ...unit, fleet_id: draftAssignments.value[unit.id] || unit.fleet_id })), []))
const mergeSource = computed(() => menuFleets.value.find((fleet) => fleet.fleet_id === mergeSourceID.value))
const orderedEvents = computed(() => [...(state.value?.events || [])].reverse())
const readyCount = computed(() => state.value?.players.filter((player) => player.ready).length || 0)

// 双人对局里对手还没进房时，ready 按钮应该直接禁用并说清楚原因，
// 而不是玩家点了才被服务端拒绝（或者更糟：一个人 ready 就把回合推进了）。
const waitingForOpponent = computed(() => {
  const view = state.value
  return Boolean(view) && !view!.solo && (view!.players?.length || 0) < 2
})

// 地图交互提示不再弹横幅（玩家嫌吵）：冷却回合数在舰队面板里已经写明，
// 拖动失败时最多在控制台留一条，不影响界面。
function mapNotice(text: string) {
  if (typeof localStorage !== 'undefined' && localStorage.getItem('gq_drag_debug') === '1') console.log('[gq map]', text)
}

// 剧本时间线：顶部时钟直接标出"现在能不能整备/放飞"，因为放飞窗口只有几回合，
// 玩家靠回合号推算 05:20/19:40 很容易整组错过去。
const clockWindowLabel = computed(() => {
  const clock = state.value?.clock
  if (!clock) return ''
  if (clock.night) return `夜间 · 航空任务已全部取消，${clock.service_start} 整备后再分配`
  if (!clock.service_open) return `${clock.service_start} 开始整备`
  if (!clock.launch_open) return `${clock.launch_start} 后放飞`
  return `放飞中 · ${clock.recover_by} 前回收`
})


// 空战/对舰/对地动画只在"新报告真的出现"时弹出。
// 首次载入把当前最新报告当作旧报告记下 id，刷新页面不会重播；
// WebSocket 推送、轮询和 ready 三条路径都走这里，任何一方结算都能触发。
// 一次结算可能同时产生"拦截 + 空袭"多份报告：以前只播最后一份，
// 前面那份（常常是真正的投弹结果）直接被吞掉，玩家以为没打中。
const combatQueue = ref<AirCombatReport[]>([])

function checkAirCombat(next: GameView, initial = false) {
  const reports = next.air_combats || []
  if (!reports.length) return false
  if (initial) {
    lastAirCombatID = reports[reports.length - 1].id
    return false
  }
  const seenIndex = reports.findIndex((report) => report.id === lastAirCombatID)
  const fresh = seenIndex >= 0 ? reports.slice(seenIndex + 1) : reports.slice(-1)
  if (!fresh.length) return false
  lastAirCombatID = fresh[fresh.length - 1].id
  combatQueue.value.push(...fresh)
  playNextAirCombat()
  return true
}

function playNextAirCombat() {
  const nextReport = combatQueue.value.shift()
  if (!nextReport) return
  activeAirCombat.value = nextReport
  activePanel.value = 'airCombat'
}

function closeAirCombat() {
  activePanel.value = 'map'
  playNextAirCombat()
}

function baselineAirCombat(next: GameView) {
  const reports = next.air_combats || []
  const latest = reports[reports.length - 1]
  if (latest) lastAirCombatID = latest.id
}

async function loadState(initial = false) {
  const requestID = ++stateRequest
  if (initial) loading.value = true
  try {
    const next = await api.state(props.roomID)
    if (requestID !== stateRequest) return
    if (state.value && next.version !== state.value.version) {
      // 版本变了不等于"选择该丢"：拖拽落点会立刻发一条 SET_MOVE，版本随之 +1，
      // 以前这里无条件清空选中，表现就是"拖完舰队面板和选中框瞬间消失"，
      // 玩家只会以为拖动没生效。只有在目标真的不在了（沉没/离开视野）才清。
      const stillSelected = Boolean(selectedID.value) && ((next.fleets || []).some((fleet) => fleet.id === selectedID.value || fleet.fleet_id === selectedID.value || fleet.members?.some((unit) => unit.id === selectedID.value)) || (next.units || []).some((unit) => unit.id === selectedID.value))
      if (!stillSelected) selectedID.value = ''
      const taskStillThere = Boolean(selectedAirTaskID.value) && (next.aviation?.tasks || []).some((task) => task.id === selectedAirTaskID.value || task.aircraft_ids?.includes(selectedAirTaskID.value))
      if (!taskStillThere) selectedAirTaskID.value = ''
      menuCoord.value = ''
      mergeSourceID.value = ''
    }
    if (initial) baselineAirCombat(next)
    else checkAirCombat(next)
    state.value = next
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

function selectMapUnit(unit: Unit) {
  if (strikeTargetTaskID.value) {
    void setStrikeTarget(unit.coord)
    return
  }
  selectedAirTaskID.value = ''
  shipDetailOpen.value = false
  selectedID.value = unit.id
}

function selectAircraft(aircraftID: string) {
  // 点飞机 = 回到地图上的编队视角，同时解除机场选择，避免界面停留在机场。
  selectedAirfieldID.value = ''
  const task = state.value?.aviation.tasks.find((item) => item.id === aircraftID || item.aircraft_ids.includes(aircraftID))
  if (!task || task.phase === 'servicing' || task.phase === 'preparing') return
  selectedID.value = ''
  shipDetailOpen.value = false
  menuCoord.value = ''
  selectedAirTaskID.value = aircraftID
  activePanel.value = 'map'
}

// 岸轰提示：舰队本回合停在敌方机场 hex、停留满 1 回合、无混乱，即满足规则书 501 行条件
function shoreTargetFor(fleet: { fleet_id: string; coord: string; confusion?: number; members: { card: { gunnery: number } }[] }) {
  const stays = state.value?.fleet_stays || {}
  if (stays[fleet.fleet_id] !== fleet.coord) return ''
  if ((fleet.confusion || 0) > 0) return ''
  if (!fleet.members.some((member) => member.card.gunnery > 0)) return ''
  return airfields.value.find((airfield) => airfield.coord === fleet.coord && airfield.faction !== state.value?.you.faction)?.name || ''
}

function openAviation(fleetID?: string) {
  const targetFleetID = fleetID || selectedFleet.value?.fleet_id || ''
  if (!targetFleetID) return
  // 从舰队进航空界面时必须清掉机场选择。以前 selectedAirfieldID 一旦被设上就一直留着，
  // 结果点过图拉吉之后再点任何舰队/飞机，打开的都还是图拉吉的界面。
  if (selectedAirfieldID.value && selectedAirfieldID.value !== targetFleetID) {
    selectedAirfieldID.value = ''
  }
  shipDetailOpen.value = false
  aviationFleetID.value = targetFleetID
  activePanel.value = 'aviation'
}

function selectFleetMember(unit: Unit) {
  selectedAirTaskID.value = ''
  shipDetailOpen.value = false
  selectedID.value = unit.id
  const isCarrier = ['CV', 'CVL', 'CVE'].includes(unit.type)
  if (isCarrier || unit.card.hangar_capacity > 0) {
    aviationFocusID.value = unit.id
    openAviation(unit.fleet_id || unit.id)
  } else {
    shipDetailOpen.value = true
  }
}

function chooseStrikeTarget(taskID: string) {
  selectedID.value = ''
  shipDetailOpen.value = false
  strikeTargetTaskID.value = taskID
  activePanel.value = 'map'
}

function cancelStrikeTarget() {
  strikeTargetTaskID.value = ''
  activePanel.value = 'aviation'
}

// 空中的编队改目标要走无线电（RETARGET_AIR），会消耗该编队本回合的通信机会。
async function setStrikeTarget(coord: string) {
  if (!strikeTargetTaskID.value) return
  const task = state.value?.aviation.tasks.find((item) => item.id === strikeTargetTaskID.value)
    ?? state.value?.ground_tasks?.find((item) => item.id === strikeTargetTaskID.value)
  // 只有空中攻击编队改目标要走无线电；空中掩护/转场用普通命令直接改目标。
  const airborne = Boolean(task && task.airborne_count > 0 && task.kind === 'attack')
  await airCommand(airborne ? 'RETARGET_AIR' : 'SET_STRIKE_TARGET', [strikeTargetTaskID.value], { coord, task_id: strikeTargetTaskID.value })
  if (!error.value) cancelStrikeTarget()
}

function selectHex(coord: string) {
  if (strikeTargetTaskID.value) {
    void setStrikeTarget(coord)
    return
  }
  if (navTargeting.value && selectedFleet.value) {
    void setAutoNav(coord)
    return
  }
  selectedAirTaskID.value = ''
  shipDetailOpen.value = false
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
  openFleetChoices(coord)
}

function openFleetChoices(coord: string) {
  shipDetailOpen.value = false
  fleetChoiceCoord.value = coord
  const choices = ownFleets.value.filter((fleet) => fleet.coord === coord)
  if (choices.length === 1) chooseFleet(choices[0].id)
}

function chooseFleet(fleetID: string) {
  selectedID.value = fleetID
  selectedAirTaskID.value = ''
  shipDetailOpen.value = false
  fleetChoiceCoord.value = ''
  menuCoord.value = ''
}

function openManager(coord: string) {
  shipDetailOpen.value = false
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

const shoreSupplyTotal = computed(() => Object.values(state.value?.shore_supplies || {}).reduce((sum, value) => sum + value, 0))

// 风向：每回合每支舰队可以调整一次朝向，影响航母的甲板容量。
// 风向按"来向"记：ortho（偏离来风 60°）仍带迎面分量，是侧逆风 +1；
// meta（偏离 120°）带顺风分量，是侧顺风 −1。以前这两个标签写反了。
const windRelations: Record<string, string> = { same: '顺风', para: '逆风', ortho: '侧逆风', meta: '侧顺风' }
const headingChoices: ReconHeading[] = ['NE', 'E', 'SE', 'SW', 'W', 'NW']
const selectedFleetWind = computed(() => {
  const wind = state.value?.wind
  if (!wind || !selectedUnit.value) return null
  const fleetID = selectedUnit.value.fleet_id || selectedUnit.value.id
  return wind.fleets.find((item) => item.fleet_id === fleetID) || null
})

async function setFleetHeading(heading: ReconHeading) {
  if (!state.value || !selectedUnit.value || actionBusy.value) return
  actionBusy.value = true
  try {
    await api.command(props.roomID, state.value.version, { type: 'SET_HEADING', unit_id: selectedUnit.value.id, heading })
    await loadState()
    error.value = ''
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : '调整朝向失败'
  } finally {
    actionBusy.value = false
  }
}

// —— 自动航行（规则书 280–283、362–370 行）——
// 在地图上点一个格子即可设定目的地，舰队会沿最短可通行路线自行推进。
const navTargeting = ref(false)
const engagementLabels: Record<string, string> = { aggressive: '逢敌必战', advantage: '优势交战', avoid: '避免近战' }
const selectedEngagement = computed(() => selectedFleet.value?.engagement || 'aggressive')

function startNavTargeting() {
  navTargeting.value = !navTargeting.value
}

async function setAutoNav(coord: string) {
  if (!state.value || !selectedUnit.value || actionBusy.value) return
  actionBusy.value = true
  try {
    await api.command(props.roomID, state.value.version, { type: 'SET_AUTO_NAV', unit_id: selectedUnit.value.id, coord, engagement: selectedEngagement.value })
    navTargeting.value = false
    await loadState()
    error.value = ''
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : '设置自动航行失败'
  } finally {
    actionBusy.value = false
  }
}

async function clearAutoNav() {
  if (!state.value || !selectedUnit.value || actionBusy.value) return
  actionBusy.value = true
  try {
    await api.command(props.roomID, state.value.version, { type: 'SET_AUTO_NAV', unit_id: selectedUnit.value.id, coord: '' })
    await loadState()
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : '取消自动航行失败'
  } finally {
    actionBusy.value = false
  }
}

async function setEngagement(setting: 'aggressive' | 'advantage' | 'avoid') {
  if (!state.value || !selectedUnit.value || actionBusy.value) return
  actionBusy.value = true
  try {
    await api.command(props.roomID, state.value.version, { type: 'SET_ENGAGEMENT', unit_id: selectedUnit.value.id, engagement: setting })
    await loadState()
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : '设置交战方式失败'
  } finally {
    actionBusy.value = false
  }
}

// —— 攻击编队的攻击方式（规则书 624–631 行）——
const selectedStrikeTask = computed(() => state.value?.aviation.tasks.find((task) => task.id === strikeTargetTaskID.value && task.kind === 'attack'))

async function setDisperse(disperse: boolean) {
  const task = selectedStrikeTask.value
  if (!state.value || !task || actionBusy.value) return
  actionBusy.value = true
  try {
    await api.command(props.roomID, state.value.version, { type: 'SET_STRIKE_DISPERSION', aircraft_id: task.id, disperse })
    await loadState()
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : '设置攻击方式失败'
  } finally {
    actionBusy.value = false
  }
}

const outcomeText = computed(() => {
  const outcome = state.value?.outcome
  if (!outcome) return ''
  const winner = outcome.winner ? `$\{outcome.winner} 方获胜` : '平局'
  return `对局结束：$\{winner}（$\{outcome.reason}）`
})

async function clearOrder(key: string) {
  if (!state.value) return
  try {
    await api.command(props.roomID, state.value.version, key.startsWith('task-') ? { type: 'CLEAR_MOVE', task_id: key } : { type: 'CLEAR_MOVE', unit_id: key })
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

async function airCommand(type: 'LAUNCH_CAP' | 'LAUNCH_RECON' | 'LAUNCH_STRIKE' | 'LAUNCH_COVER' | 'LAUNCH_TRANSFER' | 'RETURN_TO_HANGAR' | 'RECOVER_AIR' | 'PRIORITIZE_AIR_TASK' | 'SET_AIR_CYCLE' | 'SET_STRIKE_TARGET' | 'SET_STRIKE_DISPERSION' | 'RECALL_AIR' | 'RETASK_AIR' | 'RETARGET_AIR' | 'SET_LOADOUT', aircraftIDs: string[], options?: { heading?: ReconHeading; sector_width?: 60 | 120; task_id?: string; coord?: string; task_kind?: string; loadout?: string; disperse?: boolean; cycle?: boolean }) {
  if (!state.value || actionBusy.value) return
  actionBusy.value = true
  try {
    // 无线电命令按任务挂载，不需要算子 ID。
    const payload = type === 'RECALL_AIR' || type === 'RETASK_AIR' || type === 'RETARGET_AIR'
      ? { type, task_id: options?.task_id, coord: options?.coord, task_kind: options?.task_kind }
      : { type, aircraft_id: aircraftIDs[0], aircraft_ids: aircraftIDs, ...options }
    await api.command(props.roomID, state.value.version, payload)
    await loadState()
    error.value = ''
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : '航空命令失败'
  } finally {
    actionBusy.value = false
  }
}

// 就绪是单向的：点了就锁定，不再提供"取消就绪"。
// 手滑点错也能救——再发一条命令，服务端会自动把 ready 撤回（那条路径保留）。
async function toggleReady() {
  if (!state.value || actionBusy.value || state.value.you.ready) return
  actionBusy.value = true
  const requestID = ++stateRequest
  try {
    const next = await api.ready(props.roomID, state.value.version, true)
    if (requestID !== stateRequest) return
    const combatStarted = checkAirCombat(next)
    state.value = next
    selectedID.value = ''
    selectedAirTaskID.value = ''
    menuCoord.value = ''
    activePanel.value = combatStarted ? 'airCombat' : 'map'
  } catch (cause) {
    if (requestID === stateRequest) error.value = cause instanceof Error ? cause.message : '准备状态失败'
  } finally {
    actionBusy.value = false
  }
}

function unitName(unitID?: string) {
  return state.value?.units.find((unit) => unit.id === unitID)?.name || unitID || ''
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
  <main class="game-shell command-layout" :class="{ 'aviation-mode': activePanel === 'aviation' || activePanel === 'airCombat' }" :data-version="state?.version || 0">
    <aside class="left-rail">
      <button class="rail-brand" aria-label="返回首页" @click="router.push('/')"><span>GQ</span><small>COMMAND</small></button>
      <nav class="rail-menu">
        <button :class="{ active: activePanel === 'rules' }" @click="activePanel = 'rules'"><span class="rail-icon">▤</span><b>规则书</b></button>
        <button :class="{ active: activePanel === 'map' }" @click="activePanel = 'map'"><span class="rail-icon">⬡</span><b>地图</b></button>
        <button :class="{ active: activePanel === 'settings' }" @click="activePanel = 'settings'"><span class="rail-icon">⌘</span><b>设置</b></button>
        <button :class="{ active: activePanel === 'aviation' }" :disabled="!canOpenAviation" :title="canOpenAviation ? '进入当前舰队航空界面' : '先在地图选择具备航空能力的舰队'" @click="openAviation()"><span class="rail-icon">△</span><b>航空界面</b></button>
        <button :class="{ active: activePanel === 'log' }" @click="activePanel = 'log'"><span class="rail-icon">≡</span><b>日志信息</b></button>
        <button :class="{ active: activePanel === 'results' }" @click="activePanel = 'results'"><span class="rail-icon">☩</span><b>战果</b></button>
      </nav>
      <div class="rail-spacer"></div>
      <div v-if="state?.clock" class="rail-clock" :class="{ night: state.clock.night }">
        <span>DAY {{ state.clock.day }} · {{ state.clock.night ? '夜' : '昼' }}</span>
        <strong>{{ state.clock.time }}</strong>
        <small>{{ clockWindowLabel }}</small>
      </div>
      <div class="rail-vp"><span>当前 VP</span><strong>0 : 0</strong><small>沙盒不计分</small></div>
      <button class="rail-next" :class="{ locked: state?.you.ready || waitingForOpponent }" :disabled="!state || actionBusy || waitingForOpponent || state.you.ready" @click="toggleReady"><span>{{ waitingForOpponent ? '等待对手' : state?.you.ready ? '准备完成' : '下一回合' }}</span><b>▶</b></button>
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
          <div><span class="turn-label">TURN</span><strong>{{ String(state.turn).padStart(2, '0') }}</strong><span class="turn-sub">{{ waitingForOpponent ? '等待对手进房' : '等待双方命令' }}</span></div>
          <div v-if="state.clock" class="turn-clock" :class="{ night: state.clock.night }" :data-clock="state.clock.time">
            <span class="clock-day">DAY {{ state.clock.day }}</span>
            <strong>{{ state.clock.time }}</strong>
            <span class="clock-window">{{ clockWindowLabel }}</span>
          </div>
          <div class="turn-actions"><span class="ready-count">{{ readyCount }} / {{ state.players.length }} 已就绪</span><button class="ready-button" :class="{ locked: state.you.ready }" :disabled="actionBusy || waitingForOpponent || state.you.ready" :title="waitingForOpponent ? '对手还没进房：把房间 token 发给他，两名玩家都到齐后回合才会推进' : state.you.ready ? '已就绪，等对方完成部署后本回合结算' : ''" @click="toggleReady">{{ waitingForOpponent ? '等待对手进房' : state.you.ready ? '已就绪 · 等待对方' : '完成部署，结束回合' }}</button></div>
        </div>
        <!-- 房主在等对手时，邀请 token 必须摆在明面上：它原本藏在"战局 / 房间"折叠面板里，
             进房看不到、也不知道该把什么发给对方。 -->
        <div v-if="waitingForOpponent && state.join_token" class="invite-strip">
          <span>对手还没进房，把房间 token 发给他：</span>
          <button class="token-display" @click="copyToken"><code>{{ state.join_token }}</code><span>{{ copied ? '已复制' : '复制' }}</span></button>
          <small>首页「或凭邀请加入」填这个就能进</small>
        </div>
        <p v-if="error" class="error-banner game-error" role="alert">{{ error }}</p>

        <p v-if="outcomeText" class="outcome-banner" role="status">{{ outcomeText }}</p>


        <div v-if="activePanel === 'map'" class="game-layout map-command-layout">
          <section class="map-panel">
            <div class="map-toolbar"><div><span class="section-kicker">OPERATIONAL MAP</span><h1>{{ state.map.name }}</h1></div>
              <div v-if="state.wind" class="wind-indicator" :data-wind="state.wind.wind">
                <span>风向 <b>{{ state.wind.wind }}</b></span>
                <template v-if="selectedFleetWind">
                  <i>朝向 {{ selectedFleetWind.heading }}</i>
                  <!-- 风力只影响航母甲板：没有航母的舰队（巡洋舰水侦、驱逐舰）只显示风从哪来。 -->
                  <em v-if="selectedFleetWind.has_carrier" :class="selectedFleetWind.deck_modifier > 0 ? 'gain' : selectedFleetWind.deck_modifier < 0 ? 'loss' : ''">{{ windRelations[selectedFleetWind.relation] || selectedFleetWind.relation }} {{ selectedFleetWind.deck_modifier > 0 ? '+' : '' }}{{ selectedFleetWind.deck_modifier }} 甲板</em>
                  <em v-else class="wind-neutral">{{ windRelations[selectedFleetWind.relation] || selectedFleetWind.relation }}（无航母，甲板不受风向影响）</em>
                  <span class="heading-picker" :class="{ spent: selectedFleetWind.changed }">
                    <button v-for="h in headingChoices" :key="h" :disabled="selectedFleetWind.changed || actionBusy" :title="`转向${h}`" @click.stop="setFleetHeading(h)">{{ h }}</button>
                  </span>
                </template>
              </div><div v-if="strikeTargetTaskID" class="strike-map-target"><span>选择打击目标：{{ strikeTargetTaskID }} · 点击目标hex</span><button @click="cancelStrikeTarget">取消</button></div><div class="map-legend"><span><i class="legend-a"></i>A 阵营</span><span><i class="legend-b"></i>B 阵营</span><span><i class="legend-move"></i>可移动</span></div></div>
            <div class="map-canvas">
              <HexMap :map="state.map" :fleets="fleets" :player-faction="state.you.faction" :selected-unit="selectedUnit" :orders="state.orders" :recon-tasks="state.aviation.tasks.filter((task) => task.kind === 'recon')" :air-tasks="state.aviation.tasks" :airfields="airfields" :ground-aircraft="groundAircraft" :ground-tasks="groundTasks" :enemy-air="state.enemy_air || []" :distance-reference="distanceReference" :distance-origin="distanceOrigin" @select-airfield="selectAirfield" @select-unit="selectMapUnit" @select-hex="selectHex" @manage-hex="openFleetChoices" @move="move" @notice="mapNotice" />
              <section v-if="selectedFleet" class="fleet-workbench map-fleet-workbench" :class="{ 'fleet-panel-bottom': fleetPanelBottom }" :data-fleet-id="selectedFleet.fleet_id">
                <header><div><span class="section-kicker">TASK FORCE · {{ selectedFleet.coord }}</span><h2>{{ selectedFleet.name }}</h2><small><span v-if="shoreTargetFor(selectedFleet)" class="fleet-shore" title="本回合可与岸炮互射：每点火炮 25%、每点岸炮 30%，命中扣机场生命值与地面飞机">岸轰可用</span>{{ selectedFleet.count }}艘 · {{ selectedFleet.move_interval }}回合一格 · <b :class="selectedFleet.move_cooldown ? 'fleet-cooling' : 'fleet-ready'">{{ selectedFleet.move_cooldown ? `还需${selectedFleet.move_cooldown}回合` : '可移动' }}</b> · 点击舰船算子查看角色卡</small>
                  <div class="fleet-nav" :data-destination="selectedFleet.destination || ''">
                    <span class="nav-status">{{ selectedFleet.destination ? `自动航行 → ${selectedFleet.destination}` : '未设置自动航行' }}</span>
                    <!-- 同一个按钮来回切：有目的地时它是"取消航行"，没有时它是"设置目的地"。 -->
                    <button :class="{ active: navTargeting, cancel: Boolean(selectedFleet.destination) && !navTargeting }" :disabled="actionBusy" @click.stop="selectedFleet.destination ? clearAutoNav() : startNavTargeting()">{{ navTargeting ? '点击地图选择目的地…' : selectedFleet.destination ? '取消航行' : '设置目的地' }}</button>
                    <span class="nav-engagement">
                      <button v-for="(label, value) in engagementLabels" :key="value" :class="{ active: selectedEngagement === value }" :disabled="actionBusy" @click.stop="setEngagement(value as 'aggressive' | 'advantage' | 'avoid')">{{ label }}</button>
                    </span>
                  </div></div><div class="fleet-workbench-actions"><button @click="counterSide = counterSide === 'front' ? 'back' : 'front'">{{ counterSide === 'front' ? '查看兵牌背面' : '查看兵牌正面' }}</button><button @click="openManager(selectedFleet.coord)">舰队整编</button><button v-if="selectedFleet.members.some((unit) => ['CV', 'CVL', 'CVE'].includes(unit.type))" @click="openAviation(selectedFleet.fleet_id)">航空作业</button><button aria-label="关闭舰队详情" @click="selectedID = ''">×</button></div></header>
                <div class="fleet-counter-grid"><button v-for="member in selectedFleet.members" :key="member.id" class="fleet-counter-button" :class="{ active: selectedUnit?.id === member.id }" :data-ship-id="member.id" @click="selectFleetMember(member)"><ShipCounter :unit="member" :side="counterSide" /></button></div>
              </section>
              <section v-if="inspectedFleet" class="fleet-workbench enemy-fleet-workbench" :data-fleet-id="inspectedFleet.fleet_id">
                <header><div><span class="section-kicker">ENEMY CONTACT · {{ inspectedFleet.coord }}</span><h2>{{ contactIntelLabel(inspectedFleet) }}</h2><small>情报{{ inspectedFleet.contact_level || 0 }}级{{ (inspectedFleet.contact_level || 0) >= 4 ? ' · 编成与状态已知 · 点击舰船查看角色卡' : ' · 只能确认这一级的信息' }}</small></div><div class="fleet-workbench-actions"><button v-if="(inspectedFleet.contact_level || 0) >= 4" @click="counterSide = counterSide === 'front' ? 'back' : 'front'">{{ counterSide === 'front' ? '查看兵牌背面' : '查看兵牌正面' }}</button><button aria-label="关闭" @click="selectedID = ''">×</button></div></header>
                <div v-if="(inspectedFleet.contact_level || 0) >= 4" class="fleet-counter-grid"><button v-for="member in inspectedFleet.members" :key="member.id" class="fleet-counter-button" :class="{ active: selectedUnit?.id === member.id }" :data-ship-id="member.id" @click="selectFleetMember(member)"><ShipCounter :unit="member" :side="counterSide" /></button></div>
                <div v-else class="enemy-intel-note"><p>{{ enemyIntelNote(inspectedFleet) }}</p></div>
              </section>
              <aside v-if="selectedUnit && shipDetailOpen" class="side-card selection-card map-selection-card">
                <div class="side-card-head"><span class="section-kicker">SHIP DETAIL</span><button aria-label="关闭舰船详情" @click="shipDetailOpen = false">×</button></div>
                <div class="selected-detail"><ShipCard :key="selectedUnit.id" :unit="selectedUnit" /></div>
              </aside>
              <details class="map-lobby-panel">
                <summary><span>战局 / 房间</span><b>{{ readyCount }} / {{ state.players.length }} 就绪</b></summary>
                <div class="map-lobby-content">
                  <section class="side-card commander-card"><div class="section-kicker">YOUR COMMAND</div><div class="commander-line"><div class="faction-seal" :class="`seal-${state.you.faction.toLowerCase()}`">{{ state.you.faction }}</div><div><h2>{{ state.you.name }}</h2><span>{{ state.you.faction === 'A' ? '苍蓝舰队' : '赤红舰队' }} · {{ state.you.is_host ? '房主' : '参战玩家' }}</span></div></div><div class="room-code"><span>ROOM</span><code>{{ state.room_id }}</code></div></section>
                  <section v-if="state.join_token" class="side-card invite-card"><div><span class="section-kicker">INVITE PLAYER B</span></div><button class="token-display" @click="copyToken"><code>{{ state.join_token }}</code><span>{{ copied ? '已复制' : '复制' }}</span></button></section>
                  <section class="side-card orders-card"><div class="side-card-head"><span class="section-kicker">PENDING ORDERS</span><span>{{ state.orders.length }}</span></div><div v-if="state.orders.length" class="order-list"><div v-for="order in state.orders" :key="order.unit_id || order.task_id"><span>{{ order.task_id ? `任务 ${order.task_id}` : unitName(order.unit_id) }}</span><code>{{ order.type === 'MERGE_FLEET' ? `整编 → ${unitName(order.target_unit_id || '')}` : order.type === 'REORGANIZE_HEX' ? `整编hex ${order.coord}` : order.type === 'RECALL_AIR' ? '无线电召回' : order.type === 'RETASK_AIR' ? `无线电改令 → ${order.task_kind}` : `预定 → ${order.coord}` }}</code><button aria-label="取消命令" @click="clearOrder(order.unit_id || order.task_id || '')">×</button></div></div><p v-else class="empty-copy">尚未下达移动或整编命令</p>
                    <div class="supply-summary"><span>滩头物资</span><b>{{ shoreSupplyTotal }}</b><small v-for="(value, coord) in state.shore_supplies || {}" :key="coord">{{ coord }}:{{ value }}</small></div>
                  </section>
                  <section class="side-card players-card"><div class="side-card-head"><span class="section-kicker">PLAYERS</span></div><div v-for="player in state.players" :key="player.id" class="player-row"><span class="player-faction" :class="`seal-${player.faction.toLowerCase()}`">{{ player.faction }}</span><span>{{ player.name }}</span><i :class="{ online: player.connected }"></i><b :class="{ ready: player.ready }">{{ player.ready ? '已就绪' : '部署中' }}</b></div></section>
                  <section class="side-card log-card"><div class="side-card-head"><span class="section-kicker">COMBAT LOG</span><button class="text-button" @click="activePanel = 'log'">全部日志</button></div><div class="event-list"><div v-for="event in orderedEvents.slice(0, 4)" :key="event.id" class="event-row"><span>T{{ String(event.turn).padStart(2, '0') }}</span><p>{{ event.text }}</p></div></div></section>
                </div>
              </details>
            </div>
            <div class="map-help"><b>操作方式</b><span>拖动 = 规划本回合航向（结算时执行）</span><i>→</i><span>黄框虚线为已规划，箭头标出目标hex</span><i>→</i><span>点击多舰队hex可整编</span><i>→</i><span>舰队面板里可设目的地，自动航行</span></div>
          </section>
        </div>

        <section v-else-if="activePanel === 'rules'" class="workspace-page"><span class="section-kicker">RULEBOOK</span><h1>规则书与沙盒指南</h1><div class="document-grid"><article><h2>当前通信沙盒</h2><p>选择己方舰队，再把舰队拖到相邻绿色hex下达移动命令，点击目标hex不会移动。到达可移动时机后由服务器立即生效，普通/快速舰队移动后等待3回合，纯高速驱逐舰2回合，慢速5回合；底条黄色表示可移动，蓝色表示已移动或冷却中，右上角箭头表示舰队朝向。</p><p>同一hex存在多个舰队时，点击hex弹出舰队列表。选择舰队可查看成员，选择整编可将任意两个同阵营舰队合并，整编同样由服务器立即生效。</p></article><article><h2>坐标与地形</h2><p>hex使用ABCD坐标，AB为行、CD为列。当前只能拖动至相邻且可通行的hex。暗礁一律不可进入；机场格与浅滩只有含 CV/CVL/CVE 的舰队不能进入，纯水面舰队可以靠岸靠滩并实施岸轰。蓝灰色虚线仅用于距离参考，不代表可移动。</p><p>正式航空、损管与剧本规则位于项目《规则书.md》，将随规则模块逐步接入。</p></article></div></section>
        <section v-else-if="activePanel === 'settings'" class="workspace-page"><span class="section-kicker">SETTINGS</span><h1>作战室设置</h1><div class="settings-grid"><div class="side-card"><h2>当前会话</h2><p>指挥官：{{ state.you.name }}</p><p>阵营：{{ state.you.faction }} · 房间：{{ state.room_id }}</p><p>连接方式：{{ socketStatus === 'online' ? 'WebSocket实时同步' : '15秒轮询' }}</p></div><div class="side-card"><h2>地图与资源</h2><p>坐标格式：ABCD（行 + 列）</p><p>地图与剧本从服务器持久化内容目录载入。</p><a class="secondary-link" href="/api/docs" target="_blank">查看API与AI扩展文档 ↗</a></div></div></section>
        <AviationPanel v-else-if="activePanel === 'aviation'" :key="selectedAirfield ? 'airfield-' + selectedAirfield.unit_id : 'fleet'" :aviation="aviationCarriers ?? state.aviation" :units="ownUnits" :fleet-id="aviationFleetId" :targets="strikeTargets" :player-faction="state.you.faction" :focus-unit-id="aviationFocusID" :selected-aircraft-id="selectedAirTaskID" :busy="actionBusy" :radio-used="state.radio_used" :clock="state.clock" @command="airCommand" @select-aircraft="selectAircraft" @select-target="chooseStrikeTarget" />
        <ResultsPanel v-else-if="activePanel === 'results'" :state="state" />
        <AirCombatPanel v-else-if="activePanel === 'airCombat' && activeAirCombat" :report="activeAirCombat" @close="closeAirCombat" />
        <section v-else class="workspace-page"><span class="section-kicker">EVENT LOG</span><h1>日志信息</h1><div class="full-log"><div v-for="event in orderedEvents" :key="event.id" class="full-log-row"><span>T{{ String(event.turn).padStart(2, '0') }}</span><code>#{{ event.id }}</code><p>{{ event.text }}</p><time>{{ new Date(event.created_at).toLocaleTimeString() }}</time></div></div></section>
      </template>
    </div>

    <div v-if="fleetChoiceCoord && fleetChoices.length > 1" class="fleet-dialog" @click.self="fleetChoiceCoord = ''">
      <section class="fleet-modal fleet-selector" role="dialog" aria-modal="true" aria-label="选择舰队">
        <header><div><span class="section-kicker">HEX {{ fleetChoiceCoord }} · FLEET SELECT</span><h2>选择舰队</h2></div><button aria-label="关闭" @click="fleetChoiceCoord = ''">×</button></header>
        <p class="manager-hint">该hex存在多支舰队。选择一个舰队选项卡后，才会在海图上显示对应的舰队细节。</p>
        <div class="fleet-choice-list">
          <button v-for="fleet in fleetChoices" :key="fleet.fleet_id" class="fleet-choice-main" :data-fleet-choice-id="fleet.fleet_id" @click="chooseFleet(fleet.id)">
            <span class="unit-type-badge">{{ fleet.type }}</span>
            <span><strong>{{ fleet.name }}</strong><small>{{ fleet.count }}艘 · 强度{{ fleet.strength }}/{{ fleet.max_strength }} · {{ fleet.coord }}</small></span>
          </button>
        </div>
      </section>
    </div>

    <div v-if="menuCoord" class="fleet-dialog" @click.self="closeManager">
      <section class="fleet-modal fleet-manager">
        <header><div><span class="section-kicker">HEX {{ menuCoord }} · FLEET ORGANIZATION</span><h2>舰队整编</h2></div><button aria-label="关闭" @click="closeManager">×</button></header>
        <p class="manager-hint">选择“默认”将舰队置顶并设为操作对象。成员可以转移到其他舰队、拆出新舰队，也可以整支合并。</p>
        <div class="fleet-columns">
          <article v-for="fleet in menuFleets" :key="fleet.fleet_id" class="fleet-column" :class="{ default: selectedFleet?.fleet_id === fleet.fleet_id, 'merge-source': mergeSourceID === fleet.fleet_id, 'transfer-target': movingMemberID && !fleet.members.some((unit) => unit.id === movingMemberID) }">
            <header><div><span class="unit-type-badge">{{ fleet.type }}</span><h3>{{ fleet.name }}</h3><small>{{ fleet.count }}艘 · 强度{{ fleet.strength }}/{{ fleet.max_strength }}</small></div><button class="default-button" @click="selectedID = fleet.id">{{ selectedFleet?.fleet_id === fleet.fleet_id ? '默认' : '设为默认' }}</button></header>
            <div class="fleet-counter-grid manager-counter-grid">
              <div v-for="member in fleet.members" :key="member.id" class="manager-counter" :class="{ selected: movingMemberID === member.id }">
                <button class="fleet-counter-button" :data-ship-id="member.id" @click="movingMemberID = movingMemberID === member.id ? '' : member.id"><ShipCounter :unit="member" /></button>
                <button v-if="movingMemberID === member.id && fleet.count > 1" class="split-counter" @click="splitMember(member)">拆出新舰队</button>
              </div>
            </div>
            <button v-if="movingMemberID && !fleet.members.some((unit) => unit.id === movingMemberID)" class="transfer-button" @click="transferMember(fleet.fleet_id)">转移到此舰队</button>
            <footer v-if="!movingMemberID"><button v-if="!mergeSourceID" @click="mergeSourceID = fleet.fleet_id">合并舰队</button><button v-else-if="mergeSourceID === fleet.fleet_id" disabled>当前整编源</button><button v-else class="confirm" @click="mergeDraftInto(fleet.fleet_id)">合并到此队</button></footer>
          </article>
        </div>
        <footer class="manager-actions"><button class="reset-button" @click="openManager(menuCoord)">重置</button><span>整编由服务器立即生效</span><button class="apply-button" :disabled="actionBusy" @click="applyReorganization">应用整编</button></footer>
      </section>
    </div>
  </main>
</template>