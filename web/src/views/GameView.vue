<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { adoptPushedView, api, attachRoomSocket, handleRoomSocketMessage, onPushedState } from '../api'
import type { AirCombatReport, AirTask, AirUnit, GameView, Order, ReconHeading, StrikeTarget, Unit } from '../types'
import { contactCapitalsLabel, contactIntelLabel, contactSizeLabel, groupFleets, type FleetGroup } from '../fleet'
import { airfieldContactStatus } from '../airfields'
import { shipDamageBadges, shipDamageDetailLabels } from '../shipDamage'
import HexMap from '../components/HexMap.vue'
import AirfieldCounter from '../components/AirfieldCounter.vue'
import ShipCard from '../components/ShipCard.vue'
import ShipCounter from '../components/ShipCounter.vue'
import AircraftCounter from '../components/AircraftCounter.vue'
import AviationPanel from '../components/AviationPanel.vue'
import ResultsPanel from '../components/ResultsPanel.vue'
import AirCombatPanel from '../components/AirCombatPanel.vue'

const props = defineProps<{ roomID: string }>()
const router = useRouter()
const state = ref<GameView | null>(null)
const selectedID = ref('')
const shipDetailOpen = ref(false)
const selectedAirTaskID = ref('')
const strikeTargetTaskID = ref('')
// 地图上点开的编队牌：打击/转场编队离场后从整编栏撤走，详情和操作都在这里。
const mapAirTaskID = ref('')
// 打击编队的预计时间弹窗：点完目标自动弹，卡片上的"！返航时间在夜间"标记也能把它重新打开。
const strikeEtaTaskID = ref('')
const menuCoord = ref('')
const fleetChoiceCoord = ref('')
const inspectedAirfieldID = ref('')
const mergeSourceID = ref('')
const movingMemberID = ref('')
const draftAssignments = ref<Record<string, string>>({})
const activePanel = ref<'map' | 'rules' | 'settings' | 'aviation' | 'airCombat' | 'log' | 'results'>('map')
const activeAirCombat = ref<AirCombatReport | null>(null)
const counterSide = ref<'front' | 'back'>('front')
// 舰队详情面板可以收起兵牌：只留标题和按钮那一行，把地图让出来。
const fleetCardsCollapsed = ref(false)
const aviationFocusID = ref('')
const aviationFleetID = ref('')
// 选中的机场（机场与舰队共用航空界面，靠它切换数据源）
const selectedAirfieldID = ref('')
const loading = ref(true)
const actionBusy = ref(false)
const error = ref('')
const socketStatus = ref<'connecting' | 'online' | 'offline'>('connecting')
const copied = ref(false)
const watchCopied = ref(false)
const watchPath = computed(() => state.value?.join_token ? '/watch/' + state.value.join_token : '')
let socket: WebSocket | undefined
let stateRequest = 0
let reconnectTimer: ReturnType<typeof setTimeout> | undefined
let reconnectDelay = 1000
// 已经播过的最后一份战报。要跟着"这个房间 + 这个浏览器标签页"走：
//   - 只放内存里：WebSocket 重连（loadState）时还在，但整页刷新就丢了，会重播；
//   - 放 sessionStorage：刷新也不重播，关掉标签页自然清掉，不影响旁观别人的对局。
const combatSeenKey = 'gq:combat-seen:' + (typeof window === 'undefined' ? '' : window.location.pathname)
function readSeenCombat(): string {
  try { return sessionStorage.getItem(combatSeenKey) || '' } catch { return '' }
}
// 本标签页已经播过的战报 id 集合。
// 只靠"上一份 id 在哪"会漏播：服务端只留最近 20 份战报，一次几百架的大空袭
// 一回合就可能产出 20 份以上（拦截、各波空袭、对地各算一份），
// 上一份 id 滚出窗口之后，客户端会把整批新战报当成"窗口动过"直接丢掉。
const seenCombatIDs = new Set<string>()
function rememberCombat(id: string) {
  lastAirCombatID = id
  seenCombatIDs.add(id)
  try { sessionStorage.setItem(combatSeenKey, id) } catch { /* 隐私模式：退化成内存记录 */ }
}
let lastAirCombatID = readSeenCombat()

const ownUnits = computed(() => state.value?.units.filter((unit) => unit.faction === state.value?.you.faction) || [])
const fleets = computed(() => state.value?.fleets || [])
const ownFleets = computed(() => fleets.value.filter((fleet) => fleet.faction === state.value?.you.faction))
const strikeTargets = computed<StrikeTarget[]>(() => {
  const fleets_ = fleets.value.filter((fleet) => fleet.faction !== state.value?.you.faction).map((fleet) => ({
    coord: fleet.coord,
    label: `${fleet.fleet_name || fleet.name} · ${fleet.count}艘 · ${fleet.coord}${fleet.contact_level ? ` · 情报${fleet.contact_level}` : ''}`,
    count: fleet.count,
    contact_level: fleet.contact_level,
  }))
  // 敌方机场位置永远公开，可以直接拿机场格当打击目标（规则书对地攻击），
  // 所以它也要出现在目标列表里，否则任务卡上只显示一个干巴巴的 hex。
  const airfields_ = airfields.value
    .filter((airfield) => airfield.faction !== state.value?.you.faction)
    .map((airfield) => ({
      coord: airfield.coord,
      label: `${airfield.contact_level >= 3 ? airfield.name : '敌方机场'} · ${airfield.runways}跑道 · ${airfield.coord}`,
      count: 0,
      contact_level: (airfield.contact_level || 1) as 1 | 2 | 3 | 4,
    }))
  return [...fleets_, ...airfields_]
})
const selectedUnit = computed(() => (state.value?.units || []).find((unit) => unit.id === selectedID.value))
const selectedFleet = computed(() => ownFleets.value.find((fleet) => fleet.id === selectedID.value || fleet.members.some((unit) => unit.id === selectedID.value)))
const selectedFleetTransports = computed(() => selectedFleet.value?.members.filter((member) => member.type === 'TRS') || [])
const selectedFleetCargo = computed(() => selectedFleetTransports.value.reduce((sum, member) => sum + (member.card.cargo || 0), 0))
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
// 敌军面板的"查看损伤"开关：只有 4 级情报有意义（编成和舰况都已知）。
const enemyDamageOpen = ref(false)
watch(() => inspectedFleet.value?.fleet_id, () => { enemyDamageOpen.value = false })
const enemyDamageRows = computed(() => (inspectedFleet.value?.members || []).map((member) => ({
  member,
  badges: shipDamageBadges(member),
  details: shipDamageDetailLabels(member),
  health: Math.max(0, Math.min(100, (member.strength / Math.max(1, member.max_strength)) * 100)),
})))
const canOpenAviation = computed(() => Boolean(selectedFleet.value?.members.some((unit) => unit.card.hangar_capacity > 0)) || Boolean(selectedAirfield.value))
// 地图上可见的机场（己方全部 + 已侦察发现的敌方机场）
const airfields = computed(() => state.value?.airfields || [])
const myAirfields = computed(() => state.value?.ground_air || [])
const selectedAirfield = computed(() => myAirfields.value.find((airfield) => airfield.unit_id === selectedAirfieldID.value))
const inspectedAirfield = computed(() => airfields.value.find((airfield) => airfield.id === inspectedAirfieldID.value) || null)
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
    hangar_capacity: airfield.hangar_capacity || 0,
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
  inspectedAirfieldID.value = ''
  shipDetailOpen.value = false
  openAviation(airfieldID)
}

// 点敌方机场算子：只读查看当前战损，不改变任何状态。
function inspectAirfield(airfieldID: string) {
  const airfield = airfields.value.find((item) => item.id === airfieldID)
  if (!airfield || airfield.faction === state.value?.you.faction) return
  selectedAirfieldID.value = ''
  selectedID.value = ''
  selectedAirTaskID.value = ''
  mapAirTaskID.value = ''
  shipDetailOpen.value = false
  inspectedAirfieldID.value = airfieldID
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
// —— 地图上的编队牌 ——
// 打击编队离场后从整编栏里撤走，编队的机型、状态、目标以及改目标/召回
// 全部集中到地图上这张牌上：玩家在地图上看到它在哪，就直接在那操作。
const mapAirTask = computed<AirTask | undefined>(() => {
  const id = mapAirTaskID.value
  if (!id) return undefined
  return (state.value?.aviation.tasks || []).find((item) => item.id === id) ?? (state.value?.ground_tasks || []).find((item) => item.id === id)
})
// 每支空中编队每回合只有一次无线电机会（规则书 601 行）：发出即消耗，失败也算用掉。
const mapAirTaskRadio = computed(() => (state.value?.radio_tasks || []).find((item) => item.task_id === mapAirTaskID.value))
// 无线电命令是"回合结算时判定"的：下达后先挂在本回合命令列表里，
// 成功率和是否用掉都等结算。挂上之后按钮要停手，不然同一条命令会发两遍。
const mapAirTaskPending = computed(() => (state.value?.orders || []).find((order) => order.task_id === mapAirTaskID.value && ['RECALL_AIR', 'RETASK_AIR', 'RETARGET_AIR'].includes(order.type)))
const mapAirTaskPendingText = computed(() => {
  const order = mapAirTaskPending.value
  if (!order) return ''
  if (order.type === 'RECALL_AIR') return '已下达无线电召回 · 回合结算时判定'
  if (order.type === 'RETASK_AIR') return `已下达无线电改令 → ${order.task_kind || ''} · 回合结算时判定`
  return `已下达无线电改目标 → ${order.coord || ''} · 回合结算时判定`
})
const mapAirTaskRadioTitle = computed(() => {
  const radio = mapAirTaskRadio.value
  if (!radio) return '该编队不在空中，或已经没法通信'
  if (radio.used) return '本回合已经发过无线电，不能再通信'
  return `无线电：最近友军 ${radio.distance} hex，成功率 ${radio.chance}%`
})
// 编队标题只写"攻击编队 × 30"：机型明细交给下面的飞机卡，一行标题塞三四个机型必被截断。
const mapAirTaskTitle = computed(() => {
  const task = mapAirTask.value
  if (!task) return ''
  const total = task.aircraft_ids.length || task.aircraft.length
  if (task.kind === 'attack') return `攻击编队 × ${total}`
  if (task.kind === 'transfer') return `转场编队 × ${total}`
  return task.name
})
// 编队详情按"机型 + 状态 + 挂载 + 剩余航程"分组，和观战页同一套口径：
// 同一队飞机是分几批升空的，油量各不相同，只写一个"剩余航程"会骗人。
interface AirTaskModelRow {
  key: string
  aircraft: AirUnit
  count: number
  code: string
  status: string
  range: number
  max: number
  loadout: string
}
const mapAirModelRows = computed<AirTaskModelRow[]>(() => {
  const task = mapAirTask.value
  if (!task) return []
  const rows = new Map<string, AirTaskModelRow>()
  for (const aircraft of task.aircraft) {
    const range = aircraft.remaining_range
    const key = [aircraft.model, aircraft.status, aircraft.returning ? 'r' : '', aircraft.loadout || '', range].join('|')
    const existing = rows.get(key)
    if (existing) {
      existing.count += 1
      continue
    }
    rows.set(key, {
      key,
      aircraft,
      count: 1,
      code: aircraft.model.split(/\s+/)[0],
      status: aircraftFlightStatus(aircraft),
      range,
      max: aircraft.max_range,
      loadout: aircraft.loadout || '',
    })
  }
  // 同型排在一起，型内按剩余航程从多到少：一眼看出哪几架快没油了。
  return [...rows.values()].sort((a, b) => {
    if (a.aircraft.model !== b.aircraft.model) return a.aircraft.model < b.aircraft.model ? -1 : 1
    return b.range - a.range
  })
})
// 在空中的算子只写"出击中"不够用：往目标飞和往回飞都是 attack，只有 returning 能回答"在去还是在回"。
// 舰队防空值 = 全舰队重型防空之和（规则书"重防空 = 舰队所有舰船重防空值之和"）。
// 空袭结算里重防空是整队掷的，玩家在舰队面板上就该能直接看到这个数。
const selectedFleetHeavyAA = computed(() => {
  const fleet = selectedFleet.value
  if (!fleet) return 0
  const total = fleet.members.reduce((sum, unit) => sum + (unit.card?.heavy_aa || 0), 0)
  return Math.round(total * 10) / 10
})

function aircraftFlightStatus(aircraft: AirUnit) {
  const airborne = ['cap', 'recon', 'attack', 'attack_holding', 'cover', 'transfer'].includes(aircraft.status)
  if (airborne && aircraft.returning) return '返航中'
  if (aircraft.status === 'attack_holding') return '出击中 · 等待编队'
  if (aircraft.status === 'attack') return '出击中'
  // 机场算子不走甲板和升降机：落地是在跑道上，待命也是在跑道上，不能跟着航母叫"着舰/甲板"。
  const fromAirfield = airfields.value.some((airfield) => airfield.id === aircraft.carrier_id)
  if (aircraft.status === 'recovery') return fromAirfield ? '着陆中' : '着舰中'
  const labels: Record<string, string> = { hangar: '机库待命', service: '整备中', elevator: '升降机', deck: fromAirfield ? '跑道待命' : '甲板待命', ready: '整备完毕', cap: 'CAP', recon: '侦察', cover: '掩护', transfer: '转场中' }
  return labels[aircraft.status] || '空中'
}
const mapAirTaskPhase = computed(() => {
  const task = mapAirTask.value
  if (!task) return ''
  if (task.phase === 'recovering') return `回收中 · ${task.airborne_count}/${task.aircraft_ids.length} 仍在空中`
  // 返航要排在阶段前面：召回之后编队还是"已离场/任务执行中"的阶段，
  // 但玩家最需要知道的是它已经在往回飞了。
  if (task.returning) return task.cannot_land ? '返航中 · 目标机库已满' : '任务完成，返航中'
  if (task.phase === 'assembling') return '舰队上空等待编队'
  if (task.phase === 'launching') return '编队离场 · 出击中'
  if (task.cannot_land) return '目标机库已满，无法降落'
  return '出击中'
})
const mapAirTaskTarget = computed(() => {
  const task = mapAirTask.value
  if (!task) return ''
  if (!task.target_coord) return '未设置'
  const label = strikeTargets.value.find((target) => target.coord === task.target_coord)?.label
  return label || task.target_coord
})
// 起飞位置：任务卡上要能回答"这队飞机是从哪儿起飞的"。
// 以前只给当前坐标和目标，航母上起飞的 CAP/掩护看在眼里就是"坐标 = 母舰"，不知道出发点。
const mapAirTaskOrigin = computed(() => {
  const task = mapAirTask.value
  if (!task) return ''
  return task.origin_coord || '—'
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

// 部署阶段：双方在部署区内摆船，都点【部署完毕】才进第 1 回合。
// 这一阶段没有回合结算，ready 也可以撤回，所以它和"就绪即冻结"是两套语义。
const deploying = computed(() => state.value?.phase === 'deploy')

// 就绪即冻结：点"完成部署"那一刻，本回合的操作就锁死了（服务端同样拒绝任何命令），
// 界面只保留"看"：能选舰队、开角色卡、开日志，但拖不动、放不了飞机、改不了目标。
// 以前这里还能继续改（服务端会顺手把 ready 撤回），等于"准备"根本不算数。
// 部署阶段例外：拖动会自动把"部署完毕"撤回，所以不锁操作。
// 点过「部署完毕」和回合内「就绪」一样锁死操作：部署阶段没有可撤销的中间态。
const locked = computed(() => Boolean(state.value?.you.ready))

// 地图交互提示不再弹横幅（玩家嫌吵）：冷却回合数在舰队面板里已经写明，
// 拖动失败时最多在控制台留一条，不影响界面。
function mapNotice(text: string) {
  if (typeof localStorage !== 'undefined' && localStorage.getItem('gq_drag_debug') === '1') console.log('[gq map]', text)
}

// 命令失败改成弹窗：以前是一条常驻横幅，插在回合条和 OPERATIONAL MAP 之间，
// 一报错就把整张海图往下顶一截，而且没人关得掉，直到下一次操作成功才消失。
// 现在弹窗自己带关闭按钮，Esc 和点背景也能关，关掉就是清空。
function dismissError() {
  error.value = ''
}
function onErrorKeydown(event: KeyboardEvent) {
  if (event.key !== 'Escape') return
  if (error.value) {
    dismissError()
    return
  }
  // 得分明细也是弹窗，Esc 一样能关，和报错弹窗保持同一套手感。
  if (scoreDetailOpen.value) scoreDetailOpen.value = false
}

// 剧本时间线：顶部时钟直接标出"现在能不能整备/放飞"，因为放飞窗口只有几回合，
// 玩家靠回合号推算 05:20/19:40 很容易整组错过去。
// 战果分：剧本声明了 kind:"score" 目标才计分。以前侧栏写死 "0 : 0 / 沙盒不计分"，
// 换到珊瑚海这种按战果计分的剧本时，玩家打了整场也看不到自己拿了几分。
const hasScoreObjective = computed(() => (state.value?.objectives || []).some((objective) => objective.kind === 'score'))
const scoreBoard = computed(() => {
  const scores = state.value?.scores || {}
  const mine = state.value?.you?.faction || 'A'
  const other = mine === 'A' ? 'B' : 'A'
  return { mine: Number(scores[mine] ?? 0), enemy: Number(scores[other] ?? 0) }
})
function formatVP(value: number): string {
  return Number.isInteger(value) ? String(value) : value.toFixed(2).replace(/0+$/, '').replace(/\.$/, '')
}

// 侧栏那个 VP 数字单独摆着像天降的：玩家击沉了什么自己清楚，但"这 1 分到底哪来的"
// 只能靠猜。明细弹窗把同一笔账摊开——击沉哪一艘、瘫了哪一艘、掉了多少架、投送有没有完成。
// 注意方向：己方的损失记在对方账上，所以两栏都写着"这笔是谁亏的"。
const scoreDetailOpen = ref(false)
const scoreDetail = computed(() => {
  const faction = state.value?.you?.faction || 'A'
  const other = faction === 'A' ? 'B' : 'A'
  const breakdown = state.value?.score_breakdown || {}
  const rows = (side: string) => (breakdown[side] || []).map((line) => ({ ...line, points: Number(line.points ?? 0) }))
  return { faction, other, mineRows: rows(faction), enemyRows: rows(other) }
})
// 条目前面那个小标签：损失的归属方 + 类型，一眼能看出分是从哪一侧漏出去的。
// 栏头已经写了"我方得分/对方得分"，所以标签只留"我方/敌方 + 怎么丢的"，短一截好排。
function scoreLineTag(line: { kind: string; faction: string }): string {
  if (line.kind === 'supply') return '投送完成'
  const from = line.faction === scoreDetail.value.faction ? '我方' : '敌方'
  if (line.kind === 'crippled') return from + '·瘫痪'
  if (line.kind === 'aircraft') return from + '·航空'
  return from + '·击沉'
}

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
  const latestID = reports[reports.length - 1].id
  if (initial) {
    rememberCombat(latestID)
    return false
  }
  // 只播没见过的报告：不依赖"上一份在数组里的位置"，因为窗口滚动会把它挤出去。
  const fresh = reports.filter((report) => !seenCombatIDs.has(report.id))
  if (!fresh.length) return false
  for (const report of fresh) rememberCombat(report.id)
  combatQueue.value.push(...fresh)
  // 正在播的那一份不能被打断：以前这里直接 playNext，props.report.id 一变动画就从头开始，
  // 一次结算出"拦截 + 空袭"两份报告时前一份播一半就跳掉，看起来像被卡掉。
  if (!activeAirCombat.value) playNextAirCombat()
  return true
}

function playNextAirCombat() {
  const nextReport = combatQueue.value.shift()
  if (!nextReport) {
    // 队列清空后必须把当前报告放掉：否则下一场结算会以为"上一份还在播"而永远排不进队列。
    activeAirCombat.value = null
    activePanel.value = 'map'
    return
  }
  activeAirCombat.value = nextReport
  activePanel.value = 'airCombat'
}

// 播完（或玩家点关闭）就收尾：还有排队的战报就接着播，没有就回大地图。
// 动画播完不需要玩家再点一次"返回大地图"。
function closeAirCombat() {
  activeAirCombat.value = null
  playNextAirCombat()
}

function baselineAirCombat(next: GameView) {
  const reports = next.air_combats || []
  for (const report of reports) rememberCombat(report.id)
}

async function loadState(initial = false) {
  const requestID = ++stateRequest
  if (initial) loading.value = true
  try {
    const next = await api.state(props.roomID)
    if (requestID !== stateRequest) return
    applyState(next, initial)
  } catch (cause) {
    if (requestID !== stateRequest) return
    error.value = cause instanceof Error ? cause.message : '同步失败'
    if (error.value.includes('请先创建') || error.value.includes('会话已失效')) await router.push('/')
  } finally {
    if (requestID === stateRequest) loading.value = false
  }
}

// applyState 把一份视图落到界面上：选中清理、战报动画、面板开关都在这里。
// 服务端推来的和客户端拉来的都走这一条路径——两边各写一套的话，行为会慢慢分叉。
//
// 推送是"就地打补丁"，多数时候 state.value 和传进来的就是同一个对象；
// 全量替换（首帧、重连）才是新对象。所以这里不比对版本号——判断"变了没有"
// 靠的是"这是不是一次补丁"，而不是某个数字。
function applyState(next: GameView, initial = false) {
  // 选中了谁、开着哪个菜单属于界面状态，不是局面状态：每次有新视图都检查
  // 目标还在不在（沉没/离开视野/落地），不在才清，在就不动。
  const stillSelected = Boolean(selectedID.value) && ((next.fleets || []).some((fleet) => fleet.id === selectedID.value || fleet.fleet_id === selectedID.value || fleet.members?.some((unit) => unit.id === selectedID.value)) || (next.units || []).some((unit) => unit.id === selectedID.value))
  if (!stillSelected) selectedID.value = ''
  const taskStillThere = Boolean(selectedAirTaskID.value) && (next.aviation?.tasks || []).some((task) => task.id === selectedAirTaskID.value || task.aircraft_ids?.includes(selectedAirTaskID.value))
  if (!taskStillThere) selectedAirTaskID.value = ''
  if (initial) {
    baselineAirCombat(next)
  } else if (checkAirCombat(next) && !activeAirCombat.value) {
    // 新战报开始在推过来的这一帧里播：结算完成后自动切到战报面板。
    activePanel.value = 'airCombat'
  }
  // 编队全部落地后任务就没了：编队牌跟着关掉，别留一张操作不了的残影。
  if (mapAirTaskID.value
    && !(next.aviation?.tasks || []).some((task) => task.id === mapAirTaskID.value)
    && !(next.ground_tasks || []).some((task) => task.id === mapAirTaskID.value)) {
    mapAirTaskID.value = ''
  }
  state.value = next
  // 交给补丁层的是 state.value 拿到的那个响应式代理，不是传进来的原始对象：
  // 补丁就地改属性，改在原始对象上 Vue 收不到通知，界面就不会更新。
  adoptPushedView(state.value)
}

// 命令提交后不再回拉状态：服务端接受之后会直接推一份新视图过来，界面由推送驱动。
// 少一次整份投影和一次来回，也少一条"拉和推看到的不一样"的分叉路径。
async function sendCommand(order: Order) {
  if (!state.value) return
  await api.command(props.roomID, state.value.turn, order)
}

function connectSocket() {
  socketStatus.value = 'connecting'
  const protocol = location.protocol === 'https:' ? 'wss' : 'ws'
  const current = new WebSocket(protocol + '://' + location.host + '/ws?room_id=' + encodeURIComponent(props.roomID))
  socket = current
  current.onopen = () => {
    if (socket !== current) return
    attachRoomSocket(current)
    socketStatus.value = 'online'
    reconnectDelay = 1000
    void loadState(!state.value)
  }
  current.onmessage = (event) => {
    if (socket !== current) return
    handleRoomSocketMessage(String(event.data))
  }
  current.onclose = () => {
    if (socket !== current) return
    attachRoomSocket(undefined)
    socketStatus.value = 'offline'
    if (reconnectTimer) clearTimeout(reconnectTimer)
    reconnectTimer = setTimeout(connectSocket, reconnectDelay)
    reconnectDelay = Math.min(reconnectDelay * 2, 10000)
  }
  current.onerror = () => current.close()
}

function selectMapUnit(unit: Unit) {
  if (strikeTargetTaskID.value) {
    void setStrikeTarget(unit.coord)
    return
  }
  selectedAirTaskID.value = ''
  mapAirTaskID.value = ''
  inspectedAirfieldID.value = ''
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
  inspectedAirfieldID.value = ''
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
  inspectedAirfieldID.value = ''
  shipDetailOpen.value = false
  selectedID.value = unit.id
  if (unit.faction && unit.faction !== state.value?.you.faction) {
    // 敌方算子是只读情报：不管是不是航母都只开角色卡。以前点敌方航母会拿敌方
    // fleet_id 去开"航空作业"，面板打开是个空壳，还把己方选中的机场/编队带偏。
    shipDetailOpen.value = true
    return
  }
  const isCarrier = ['CV', 'CVL', 'CVE'].includes(unit.type)
  if (isCarrier || unit.card.hangar_capacity > 0) {
    aviationFocusID.value = unit.id
    openAviation(unit.fleet_id || unit.id)
  } else {
    shipDetailOpen.value = true
  }
}

function chooseStrikeTarget(taskID: string) {
  if (locked.value) return
  inspectedAirfieldID.value = ''
  selectedID.value = ''
  shipDetailOpen.value = false
  strikeTargetTaskID.value = taskID
  activePanel.value = 'map'
}

const strikeTargetLabel = computed(() => {
  const id = strikeTargetTaskID.value
  if (!id) return ''
  const task = (state.value?.aviation.tasks || []).find((item) => item.id === id) ?? (state.value?.ground_tasks || []).find((item) => item.id === id)
  return task?.name || id
})

// 正在选目标的编队是什么类型，决定提示语：转场选的是己方机场，打击选的是敌舰/机场，掩护选的是空域。
// 以前一律写"选择打击目标"，转场玩家点自己的机场时才完全看不出这一下该落在哪儿。
const strikeTargetSelection = computed(() => {
  const id = strikeTargetTaskID.value
  if (!id) return { kind: '', verb: '', hint: '' }
  const task = (state.value?.aviation.tasks || []).find((item) => item.id === id) ?? (state.value?.ground_tasks || []).find((item) => item.id === id)
  if (task?.kind === 'transfer') {
    // 转场航程 = 该任务飞机当前挂载的满油航程（副油箱 + 转场加成），编队里取最保守的一架。
    // 服务端只在还没起飞时按这个距离校验一次，所以先把上限摆给玩家看。
    const ranges = (task.aircraft || []).map((item) => item.max_range || 0).filter((value) => value > 0)
    const limit = ranges.length ? Math.min(...ranges) : 0
    return { kind: 'transfer', verb: '选择转场目的地', hint: limit ? `点击可降落的机场或舰队 · 本队转场航程 ${limit} hex` : '点击可降落的机场或舰队' }
  }
  if (task?.kind === 'cover') return { kind: 'cover', verb: '选择掩护空域', hint: '点击要掩护的 hex' }
  return { kind: 'attack', verb: '选择打击目标', hint: '点击目标 hex' }
})

function cancelStrikeTarget() {
  strikeTargetTaskID.value = ''
  // 从地图上的编队牌点进"选目标"，取消后还得站在地图上；从整编栏进去的才回航空界面。
  activePanel.value = mapAirTaskID.value ? 'map' : 'aviation'
}

// 点地图上的编队牌：开/关编队详情。选目标的过程中点编队牌不算数，别把选目标打断。
function selectMapAirTask(taskID: string) {
  if (strikeTargetTaskID.value) return
  inspectedAirfieldID.value = ''
  selectedID.value = ''
  shipDetailOpen.value = false
  menuCoord.value = ''
  mapAirTaskID.value = mapAirTaskID.value === taskID ? '' : taskID
  selectedAirTaskID.value = mapAirTaskID.value
}

async function recallMapAirTask() {
  const task = mapAirTask.value
  if (!task || actionBusy.value) return
  await airCommand('RECALL_AIR', [], { task_id: task.id })
}

// 空中只能在打击和转场之间改令（规则书 601 行），和整编栏里那对箭头是同一条规则。
async function retaskMapAirTask() {
  const task = mapAirTask.value
  if (!task || actionBusy.value) return
  await airCommand('RETASK_AIR', [], { task_id: task.id, task_kind: task.kind === 'attack' ? 'transfer' : 'attack' })
}

// 空中的编队改目标要走无线电（RETARGET_AIR），会消耗该编队本回合的通信机会。
async function setStrikeTarget(coord: string) {
  if (!strikeTargetTaskID.value) return
  const task = state.value?.aviation.tasks.find((item) => item.id === strikeTargetTaskID.value)
    ?? state.value?.ground_tasks?.find((item) => item.id === strikeTargetTaskID.value)
  // 只有空中攻击编队改目标要走无线电；空中掩护/转场用普通命令直接改目标。
  const airborne = Boolean(task && task.airborne_count > 0 && task.kind === 'attack')
  await airCommand(airborne ? 'RETARGET_AIR' : 'SET_STRIKE_TARGET', [strikeTargetTaskID.value], { coord, task_id: strikeTargetTaskID.value })
  if (!error.value) {
    // 点完目标先给玩家一张时间表：几点打到、几点全部落回来、会不会摸黑着舰。
    // 这比在编队牌上挂一行红字有用——起飞前就能决定要不要换目标或者干脆不打。
    if (task?.kind === 'attack') openStrikeEta(task.id)
    cancelStrikeTarget()
  }
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
  mapAirTaskID.value = ''
  inspectedAirfieldID.value = ''
  shipDetailOpen.value = false
  const airfieldHere = airfields.value.find((item) => item.coord === coord)
  const found = fleets.value.filter((fleet) => fleet.coord === coord)
  if (!found.length) {
    selectedID.value = ''
    menuCoord.value = ''
    // 基地格：点整格和点那张小牌子应该是一回事。卡片缩到 55px 之后，
    // 手指想精准点中它越来越难，而名字/details本来就在展开后的面板里。
    if (airfieldHere) {
      if (airfieldHere.faction === state.value?.you.faction) selectAirfield(airfieldHere.id)
      else inspectAirfield(airfieldHere.id)
    }
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
  inspectedAirfieldID.value = ''
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
  if (!state.value || !selectedUnit.value || actionBusy.value || locked.value) return
  actionBusy.value = true
  try {
    await sendCommand({ type: 'SET_MOVE', unit_id: selectedUnit.value.id, coord })
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : '命令失败'
  } finally {
    actionBusy.value = false
  }
}

const shoreSupplyTotal = computed(() => Object.values(state.value?.shore_supplies || {}).reduce((sum, value) => sum + value, 0))
// 敌方卸在自家机场上的货：看得见才谈得上决定要不要回去清滩。
const enemyShoreSupplyTotal = computed(() => Object.values(state.value?.enemy_shore_supplies || {}).reduce((sum, value) => sum + value, 0))

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
  if (!state.value || !selectedUnit.value || actionBusy.value || locked.value) return
  actionBusy.value = true
  try {
    await sendCommand({ type: 'SET_HEADING', unit_id: selectedUnit.value.id, heading })
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
  if (!state.value || !selectedUnit.value || actionBusy.value || locked.value) return
  actionBusy.value = true
  try {
    await sendCommand({ type: 'SET_AUTO_NAV', unit_id: selectedUnit.value.id, coord, engagement: selectedEngagement.value })
    navTargeting.value = false
    error.value = ''
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : '设置自动航行失败'
  } finally {
    actionBusy.value = false
  }
}

async function clearAutoNav() {
  if (!state.value || !selectedUnit.value || actionBusy.value || locked.value) return
  actionBusy.value = true
  try {
    await sendCommand({ type: 'SET_AUTO_NAV', unit_id: selectedUnit.value.id, coord: '' })
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : '取消自动航行失败'
  } finally {
    actionBusy.value = false
  }
}

async function setEngagement(setting: 'aggressive' | 'advantage' | 'avoid') {
  if (!state.value || !selectedUnit.value || actionBusy.value || locked.value) return
  actionBusy.value = true
  try {
    await sendCommand({ type: 'SET_ENGAGEMENT', unit_id: selectedUnit.value.id, engagement: setting })
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : '设置交战方式失败'
  } finally {
    actionBusy.value = false
  }
}

// —— 攻击编队的预计时间（弹窗）——
const strikeEtaTask = computed(() => {
  const id = strikeEtaTaskID.value
  if (!id) return null
  return (state.value?.aviation.tasks || []).find((task) => task.id === id)
    ?? (state.value?.ground_tasks || []).find((task) => task.id === id)
    ?? null
})
// 有预计时刻、或者这波注定摸黑，才值得弹：没有昼夜时间线的剧本里时间字段是空串。
const strikeEtaOpen = computed(() => {
  const task = strikeEtaTask.value
  return Boolean(task && (task.eta_target || task.eta_landed || task.lands_at_night || task.night_safe === false))
})
const strikeEtaNightNote = computed(() => {
  const task = strikeEtaTask.value
  if (!task) return ''
  const landed = task.eta_landed ? `预计 ${task.eta_landed} 落舰，` : ''
  return `！返航时间在夜间（${landed}夜间着舰按摔毁判定结算：航母 70%、机场 20%）`
})
function openStrikeEta(taskID: string) {
  if (!taskID) return
  strikeEtaTaskID.value = taskID
}

// —— 攻击编队的攻击方式（规则书 624–631 行）——
const selectedStrikeTask = computed(() => state.value?.aviation.tasks.find((task) => task.id === strikeTargetTaskID.value && task.kind === 'attack'))

async function setDisperse(disperse: boolean) {
  const task = selectedStrikeTask.value
  if (!state.value || !task || actionBusy.value || locked.value) return
  actionBusy.value = true
  try {
    await sendCommand({ type: 'SET_STRIKE_DISPERSION', aircraft_id: task.id, disperse })
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : '设置攻击方式失败'
  } finally {
    actionBusy.value = false
  }
}

const outcomeText = computed(() => {
  const outcome = state.value?.outcome
  if (!outcome) return ''
  const faction = outcome.winner === 'A' ? '美军' : outcome.winner === 'B' ? '日军' : ''
  const level = outcome.level === 'strategic' ? '战略胜利' : outcome.level === 'tactical' ? '战术胜利' : outcome.level === 'draw' ? '平分秋色' : ''
  const result = level ? (outcome.winner ? `${faction} · ${level}` : level) : (outcome.winner ? `${faction}获胜` : '平局')
  return `对局结束：${result}（${outcome.reason}）`
})

async function clearOrder(key: string) {
  if (!state.value || locked.value) return
  try {
    await sendCommand(key.startsWith('task-') ? { type: 'CLEAR_MOVE', task_id: key } : { type: 'CLEAR_MOVE', unit_id: key })
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
  if (!state.value || !menuCoord.value || actionBusy.value || locked.value) return
  actionBusy.value = true
  try {
    await sendCommand({ type: 'REORGANIZE_HEX', unit_id: `hex-${menuCoord.value}`, coord: menuCoord.value, assignments: draftAssignments.value })
    closeManager()
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : '整编失败'
  } finally {
    actionBusy.value = false
  }
}

async function airCommand(type: 'LAUNCH_CAP' | 'LAUNCH_RECON' | 'LAUNCH_STRIKE' | 'LAUNCH_COVER' | 'LAUNCH_TRANSFER' | 'RETURN_TO_HANGAR' | 'LIFT_TO_DECK' | 'RECOVER_AIR' | 'PRIORITIZE_AIR_TASK' | 'SET_AIR_CYCLE' | 'SET_STRIKE_TARGET' | 'SET_STRIKE_DISPERSION' | 'RECALL_AIR' | 'RETASK_AIR' | 'RETARGET_AIR' | 'SET_LOADOUT', aircraftIDs: string[], options?: { heading?: ReconHeading; sector_width?: 60 | 120; task_id?: string; coord?: string; task_kind?: string; loadout?: string; disperse?: boolean; cycle?: boolean; lane?: number }) {
  if (!state.value || actionBusy.value || locked.value) return
  actionBusy.value = true
  try {
    // 无线电命令按任务挂载，不需要算子 ID。
    const payload = type === 'RECALL_AIR' || type === 'RETASK_AIR' || type === 'RETARGET_AIR'
      ? { type, task_id: options?.task_id, coord: options?.coord, task_kind: options?.task_kind }
      : { type, aircraft_id: aircraftIDs[0], aircraft_ids: aircraftIDs, ...options }
    await sendCommand(payload)
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
  if (!state.value || actionBusy.value) return
  // 部署完毕和回合就绪都是单向的：按钮在 ready 之后直接禁用，不再提供撤回。
  if (state.value.you.ready) return
  const ready = true
  actionBusy.value = true
  try {
    // 只发"我要 ready"。响应只是一条回执：ready 标记、回合结算、战报都走推送。
    await api.ready(props.roomID, state.value.turn, ready)
    selectedID.value = ''
    selectedAirTaskID.value = ''
    menuCoord.value = ''
    // 战斗动画正在播时不能把面板按回地图：那样组件被卸载，定时器全部清掉，
    // 动画就停在半路。没有在播的动画时回地图等结算。
    if (!activeAirCombat.value) activePanel.value = 'map'
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : '准备状态失败'
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

// 观战链接直接用房间邀请 token：/watch/{token} 由服务端解析成房间，
// 不用把 16 位房间 ID 这种"复杂 hash"发给旁观者。
async function copyWatchLink() {
  if (!watchPath.value) return
  await navigator.clipboard.writeText(location.origin + watchPath.value)
  watchCopied.value = true
  setTimeout(() => { watchCopied.value = false }, 1500)
}

onMounted(() => {
  // 服务端推来一份新视图就直接落地；首帧按"初始"处理，不重播历史战报。
  onPushedState((view) => applyState(view, !state.value))
  connectSocket()
  window.addEventListener('keydown', onErrorKeydown)
})

onUnmounted(() => {
  window.removeEventListener('keydown', onErrorKeydown)
  onPushedState(undefined)
  if (reconnectTimer) clearTimeout(reconnectTimer)
  attachRoomSocket(undefined)
  const closing = socket
  socket = undefined
  closing?.close()
})
</script>

<template>
  <main class="game-shell command-layout" :class="{ 'aviation-mode': activePanel === 'aviation' || activePanel === 'airCombat' }" :data-turn="state?.turn || 0">
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
      <div v-if="hasScoreObjective" class="rail-vp">
        <span>当前 VP</span><strong>{{ formatVP(scoreBoard.mine) }} : {{ formatVP(scoreBoard.enemy) }}</strong><small>我方 : 敌方</small>
        <button class="rail-vp-detail" title="看看这些分是怎么来的" @click="scoreDetailOpen = true">得分明细</button>
      </div>
      <div v-else class="rail-vp"><span>当前 VP</span><strong>0 : 0</strong><small>本剧本不计分</small></div>
      <button class="rail-next" :class="{ locked: state?.you.ready || waitingForOpponent }" :disabled="!state || actionBusy || waitingForOpponent || state.you.ready" @click="toggleReady"><span>{{ waitingForOpponent ? '等待对手' : deploying ? (state?.you.ready ? '已部署完毕' : '部署完毕') : state?.you.ready ? '等待结算' : '下一回合' }}</span><b>▶</b></button>
    </aside>

    <div class="command-main">
      <header class="game-header">
        <div class="brand game-brand"><span class="brand-mark" aria-hidden="true"><i></i><i></i><i></i></span><span>GENERAL QUARTERS</span></div>
        <div v-if="state" class="operation-title"><span>当前剧本</span><strong>{{ state.scenario.name }}</strong></div>
        <div class="header-right"><div class="connection"><span class="live-dot" :class="`status-${socketStatus}`"></span>{{ socketStatus === 'online' ? '实时连接' : socketStatus === 'connecting' ? '连接中' : '连接断开 · 正在重连' }}</div><a href="/api/docs" target="_blank">API ↗</a></div>
      </header>

      <div v-if="loading" class="loading-screen"><span class="loader"></span>正在载入海图与战局…</div>
      <template v-else-if="state">
        <div class="turn-strip">
          <div><span class="turn-label">{{ deploying ? 'DEPLOY' : 'TURN' }}</span><strong>{{ deploying ? '部署' : String(state.turn).padStart(2, '0') }}</strong><span class="turn-sub">{{ deploying ? '双方在部署区摆船' : waitingForOpponent ? '等待对手进房' : '等待双方命令' }}</span></div>
          <div v-if="state.clock" class="turn-clock" :class="{ night: state.clock.night }" :data-clock="state.clock.time">
            <span class="clock-day">DAY {{ state.clock.day }}</span>
            <strong>{{ state.clock.time }}</strong>
            <span class="clock-window">{{ clockWindowLabel }}</span>
          </div>
          <div class="turn-actions"><span class="ready-count">{{ readyCount }} / {{ state.players.length }} 已就绪</span><button class="ready-button" :class="{ locked: state.you.ready }" :disabled="actionBusy || waitingForOpponent || state.you.ready" :title="deploying ? (state.you.ready ? '已经点过部署完毕：操作已锁定，等对手也点完就开战' : '双方都把舰队摆好后各点一次：进第 1 回合，部署不算回合') : waitingForOpponent ? '对手还没进房：把房间 token 发给他，两名玩家都到齐后回合才会推进' : state.you.ready ? '已就绪，等对方完成部署后本回合结算' : ''" @click="toggleReady">{{ waitingForOpponent ? '等待对手进房' : deploying ? (state.you.ready ? '已部署完毕 · 等待对手' : '部署完毕') : state.you.ready ? '已就绪 · 等待对方' : '完成部署，结束回合' }}</button></div>
        </div>
        <!-- 部署阶段横幅：说明可拖的范围和"部署不算回合"，避免玩家到处找回合按钮。 -->
        <div v-if="deploying" class="deploy-strip">
          <span>部署阶段：把舰队拖到高亮区内（锚点 {{ state.deploy_zone?.anchor }} · 半径 {{ state.deploy_zone?.radius }} 格），可以整编、拆分、合并；点过「部署完毕」后操作锁定。</span>
          <b>{{ readyCount }} / {{ state.players.length }} 已部署完毕 · 双方都点完才进第 1 回合</b>
        </div>
        <!-- 房主在等对手时，邀请 token 必须摆在明面上：它原本藏在"战局 / 房间"折叠面板里，
             进房看不到、也不知道该把什么发给对方。 -->
        <div v-if="waitingForOpponent && state.join_token" class="invite-strip">
          <span>对手还没进房，把房间 token 发给他：</span>
          <button class="token-display" @click="copyToken"><code>{{ state.join_token }}</code><span>{{ copied ? '已复制' : '复制' }}</span></button>
          <small>首页「或凭邀请加入」填这个就能进</small>
          <a v-if="watchPath" class="watch-link" :href="watchPath" target="_blank" rel="noreferrer">观战页 ↗</a>
        </div>

        <p v-if="outcomeText" class="outcome-banner" role="status">{{ outcomeText }}</p>


        <!-- 战报动画是面板链的第一优先：播放期间切到航空/日志/大地图都不会把它卸载掉。
             以前它在链中间，玩家随手点一下地图，组件被卸载、定时器清空，动画就停在半路。 -->
        <AirCombatPanel v-if="activeAirCombat" :report="activeAirCombat" :has-next="combatQueue.length > 0" @close="closeAirCombat" />
        <div v-else-if="activePanel === 'map'" class="game-layout map-command-layout">
          <section class="map-panel">
            <div class="map-toolbar"><div><span class="section-kicker">OPERATIONAL MAP</span><h1>{{ state.map.name }}</h1></div>
              <div v-if="state.wind" class="wind-indicator" :data-wind="state.wind.wind">
                <span>盛行风 <b>{{ state.wind.wind }}</b></span>
                <span v-if="selectedFleetWind && selectedFleetWind.direction">本格 <b>{{ selectedFleetWind.direction }}</b></span>
                <template v-if="selectedFleetWind">
                  <i>朝向 {{ selectedFleetWind.heading }}</i>
                  <!-- 风力只影响航母甲板：没有航母的舰队（巡洋舰水侦、驱逐舰）只显示风从哪来。 -->
                  <em v-if="selectedFleetWind.has_carrier" :class="selectedFleetWind.deck_modifier > 0 ? 'gain' : selectedFleetWind.deck_modifier < 0 ? 'loss' : ''">{{ windRelations[selectedFleetWind.relation] || selectedFleetWind.relation }} {{ selectedFleetWind.deck_modifier > 0 ? '+' : '' }}{{ selectedFleetWind.deck_modifier }} 甲板</em>
                  <em v-else class="wind-neutral">{{ windRelations[selectedFleetWind.relation] || selectedFleetWind.relation }}（无航母，甲板不受风向影响）</em>
                  <span class="heading-picker" :class="{ spent: selectedFleetWind.changed }">
                    <button v-for="h in headingChoices" :key="h" :disabled="selectedFleetWind.changed || actionBusy || locked || deploying" :title="locked ? '本回合已就绪，操作已锁定' : `转向${h}`" @click.stop="setFleetHeading(h)">{{ h }}</button>
                  </span>
                </template>
              </div><div v-if="strikeTargetTaskID" class="strike-map-target"><span>{{ strikeTargetSelection.verb }}：{{ strikeTargetLabel }} · {{ strikeTargetSelection.hint }}</span><button @click="cancelStrikeTarget">取消</button></div><div class="map-legend"><span><i class="legend-a"></i>A 阵营</span><span><i class="legend-b"></i>B 阵营</span><span><i class="legend-move"></i>可移动</span><span><i class="legend-wind"></i>格底箭头=风吹向</span><span><svg class="legend-wx" viewBox="-10 -10 20 20" aria-hidden="true"><circle cx="-5.4" cy="1.4" r="3.4" /><circle cx="-0.6" cy="-1.2" r="4.8" /><circle cx="3.6" cy="1.2" r="3.2" /><rect x="-8.6" y="1.2" width="12.8" height="3.6" rx="1.8" /></svg>多云</span><span><svg class="legend-wx" viewBox="-10 -10 20 20" aria-hidden="true"><path d="M0 -7.4c0 0 3.2 4.8 3.2 7a3.2 3.2 0 0 1-6.4 0c0-2.2 3.2-7 3.2-7Z" /><path transform="translate(-7.6 4.2) scale(.76)" d="M0 -7.4c0 0 3.2 4.8 3.2 7a3.2 3.2 0 0 1-6.4 0c0-2.2 3.2-7 3.2-7Z" /><path transform="translate(7.6 4.2) scale(.76)" d="M0 -7.4c0 0 3.2 4.8 3.2 7a3.2 3.2 0 0 1-6.4 0c0-2.2 3.2-7 3.2-7Z" /></svg>降雨</span><span><svg class="legend-wx" viewBox="-10 -10 20 20" aria-hidden="true"><path d="M1.6 -9.6 L-5.6 0.9 H-1.4 L-2.4 9.6 L6 -1.2 H1.4 L4.8 -9.6 Z" /></svg>风暴</span></div></div>
            <div class="map-canvas">
              <HexMap :map="state.map" :fleets="fleets" :player-faction="state.you.faction" :selected-unit="selectedUnit" :orders="state.orders" :recon-tasks="[...state.aviation.tasks, ...(state.ground_tasks || [])].filter((task) => task.kind === 'recon')" :air-tasks="state.aviation.tasks" :airfields="airfields" :ground-aircraft="groundAircraft" :ground-tasks="groundTasks" :enemy-air="state.enemy_air || []" :distance-reference="distanceReference" :distance-origin="distanceOrigin" :selected-air-task-id="mapAirTaskID" :locked="locked" :strike-targeting="Boolean(strikeTargetTaskID)" :nav-targeting="navTargeting" :wind-cells="state.wind?.cells || ''" :weather-cells="state.weather_cells || ''" @select-airfield="selectAirfield" @inspect-airfield="inspectAirfield" @select-unit="selectMapUnit" @select-hex="selectHex" @manage-hex="openFleetChoices" @move="move" @notice="mapNotice" @select-air-task="selectMapAirTask" />
              <!-- 编队牌：整编栏里出发后就撤走的打击/转场编队，详情与无线电操作在这里 -->
              <section v-if="mapAirTask" class="air-task-workbench map-air-workbench" :data-air-task-id="mapAirTask.id">
                <header>
                  <div>
                    <span class="section-kicker">AIR FORMATION · {{ mapAirTask.current_coord }}</span>
                    <h2>{{ mapAirTaskTitle }}</h2>
                    <small>{{ mapAirTask.airborne_count }} / {{ mapAirTask.aircraft_ids.length }} 在空中{{ mapAirTask.returning ? ' · 返航中' : '' }}</small>
                  </div>
                  <div class="fleet-workbench-actions"><button aria-label="关闭编队详情" @click="mapAirTaskID = ''">×</button></div>
                </header>
                <div class="air-task-detail">
                  <!-- 编队里的飞机按"机型 + 状态 + 航程"铺开成卡：剩多少油、在去还是在回，
                       每张卡自己说话。合计数字（编队最小航程）会把快没油的几架藏起来。 -->
                  <div class="air-task-model-grid">
                    <div v-for="row in mapAirModelRows" :key="row.key" class="air-task-model" :class="{ returning: row.aircraft.returning }" :title="`${row.aircraft.model} ×${row.count}${row.loadout ? ' · 挂载 ' + row.loadout : ''}`">
                      <AircraftCounter :aircraft="row.aircraft" :stack-count="row.count" :player-faction="state.you.faction" :cycle-clickable="false" />
                      <div class="air-task-model-meta">
                        <b>{{ row.code }} ×{{ row.count }}</b>
                        <span>{{ row.status }} · 航程 {{ row.range }}/{{ row.max }}</span>
                        <em v-if="row.loadout">{{ row.loadout }}</em>
                      </div>
                    </div>
                  </div>
                  <dl>
                    <div><dt>状态</dt><dd>{{ mapAirTaskPhase }}</dd></div>
                    <div><dt>目标</dt><dd>{{ mapAirTaskTarget }}</dd></div>
                    <div><dt>起飞</dt><dd>{{ mapAirTaskOrigin }}</dd></div>
                  </dl>
                  <button v-if="mapAirTask.night_safe === false" type="button" class="air-task-night-warning" :title="`距天黑 ${state.clock?.night_in_turns} 回合，返航要 ${mapAirTask.return_turns} 回合——这波回不来`" @click.stop="openStrikeEta(mapAirTask.id)">！返航时间在夜间 · 查看预计</button>
                  <p class="air-task-radio-note">{{ mapAirTaskPendingText || mapAirTaskRadioTitle }}</p>
                  <footer>
                    <button :disabled="actionBusy || Boolean(mapAirTaskPending) || !mapAirTaskRadio || mapAirTaskRadio.used" :title="mapAirTaskPendingText || mapAirTaskRadioTitle" @click.stop="chooseStrikeTarget(mapAirTask.id)">修改目标</button>
                    <button :disabled="actionBusy || Boolean(mapAirTaskPending) || !mapAirTaskRadio || mapAirTaskRadio.used" :title="mapAirTaskPendingText || mapAirTaskRadioTitle" @click.stop="recallMapAirTask">召回</button>
                    <button v-if="mapAirTask.kind === 'attack' || mapAirTask.kind === 'transfer'" :disabled="actionBusy || Boolean(mapAirTaskPending) || !mapAirTaskRadio || mapAirTaskRadio.used" :title="mapAirTaskPendingText || mapAirTaskRadioTitle" @click.stop="retaskMapAirTask">{{ mapAirTask.kind === 'attack' ? '改令 → 转场' : '改令 → 打击' }}</button>
                  </footer>
                </div>
              </section>
              <section v-if="selectedFleet" class="fleet-workbench map-fleet-workbench" :class="{ 'fleet-panel-bottom': fleetPanelBottom, 'cards-collapsed': fleetCardsCollapsed }" :data-fleet-id="selectedFleet.fleet_id">
                <header><div><span class="section-kicker">TASK FORCE · {{ selectedFleet.coord }}</span><h2>{{ selectedFleet.name }}</h2><small><span v-if="shoreTargetFor(selectedFleet)" class="fleet-shore" title="本回合可与岸炮互射：每点火炮 25%、每点岸炮 30%，命中扣机场生命值与地面飞机">岸轰可用</span>{{ selectedFleet.count }}艘 · <template v-if="selectedFleetTransports.length">货物 {{ selectedFleetCargo }} · </template>{{ selectedFleet.move_interval }}回合一格 · <b :class="selectedFleet.move_cooldown ? 'fleet-cooling' : 'fleet-ready'">{{ selectedFleet.move_cooldown ? `还需${selectedFleet.move_cooldown}回合` : '可移动' }}</b> · <span class="fleet-aa-value" title="舰队重型防空值合计：空袭时按这个值对来袭机队逐点判定">舰队防空值 <b>{{ selectedFleetHeavyAA }}</b></span> · 点击舰船算子查看角色卡</small>
                  <div class="fleet-nav" :data-destination="selectedFleet.destination || ''">
                    <span class="nav-status">{{ selectedFleet.destination ? `自动航行 → ${selectedFleet.destination}` : '未设置自动航行' }}</span>
                    <!-- 同一个按钮来回切：有目的地时它是"取消航行"，没有时它是"设置目的地"。 -->
                    <button :class="{ active: navTargeting, cancel: Boolean(selectedFleet.destination) && !navTargeting }" :disabled="actionBusy || locked || deploying" :title="locked ? '本回合已就绪，操作已锁定' : ''" @click.stop="selectedFleet.destination ? clearAutoNav() : startNavTargeting()">{{ navTargeting ? '点击地图选择目的地…' : selectedFleet.destination ? '取消航行' : '设置目的地' }}</button>
                    <span class="nav-engagement">
                      <button v-for="(label, value) in engagementLabels" :key="value" :class="{ active: selectedEngagement === value }" :disabled="actionBusy || locked || deploying" :title="locked ? '本回合已就绪，操作已锁定' : ''" @click.stop="setEngagement(value as 'aggressive' | 'advantage' | 'avoid')">{{ label }}</button>
                    </span>
                  </div></div><div class="fleet-workbench-actions"><button @click="counterSide = counterSide === 'front' ? 'back' : 'front'">{{ counterSide === 'front' ? '查看兵牌背面' : '查看兵牌正面' }}</button><button :disabled="locked" :title="locked ? '本回合已就绪，操作已锁定' : ''" @click="openManager(selectedFleet.coord)">舰队整编</button><button v-if="selectedFleet.members.some((unit) => ['CV', 'CVL', 'CVE'].includes(unit.type))" :disabled="deploying" :title="deploying ? '部署阶段先摆船，开战后再安排航空作业' : ''" @click="openAviation(selectedFleet.fleet_id)">航空作业</button><button class="fleet-toggle-cards" :aria-pressed="fleetCardsCollapsed" @click="fleetCardsCollapsed = !fleetCardsCollapsed">{{ fleetCardsCollapsed ? '展开兵牌' : '收起兵牌' }}</button><button aria-label="关闭舰队详情" @click="selectedID = ''">×</button></div></header>
                <div v-show="!fleetCardsCollapsed" class="fleet-counter-grid"><button v-for="member in selectedFleet.members" :key="member.id" class="fleet-counter-button" :class="{ active: selectedUnit?.id === member.id }" :data-ship-id="member.id" @click="selectFleetMember(member)"><ShipCounter :unit="member" :side="counterSide" /></button></div>
              </section>
              <section v-if="inspectedFleet" class="fleet-workbench enemy-fleet-workbench" :data-fleet-id="inspectedFleet.fleet_id">
                <header><div><span class="section-kicker">ENEMY CONTACT · {{ inspectedFleet.coord }}</span><h2>{{ contactIntelLabel(inspectedFleet) }}</h2><small>情报{{ inspectedFleet.contact_level || 0 }}级{{ (inspectedFleet.contact_level || 0) >= 4 ? (enemyDamageOpen ? ' · 各舰舰况与损伤 · 点击一行查看角色卡' : ' · 编成与状态已知 · 点击舰船查看角色卡') : ' · 只能确认这一级的信息' }}</small></div><div class="fleet-workbench-actions"><button v-if="(inspectedFleet.contact_level || 0) >= 4" class="enemy-damage-toggle" :aria-pressed="enemyDamageOpen" @click="enemyDamageOpen = !enemyDamageOpen">{{ enemyDamageOpen ? '查看编成' : '查看损伤' }}</button><button v-if="(inspectedFleet.contact_level || 0) >= 4 && !enemyDamageOpen" @click="counterSide = counterSide === 'front' ? 'back' : 'front'">{{ counterSide === 'front' ? '查看兵牌背面' : '查看兵牌正面' }}</button><button aria-label="关闭" @click="selectedID = ''">×</button></div></header>
                <div v-if="(inspectedFleet.contact_level || 0) >= 4 && !enemyDamageOpen" class="fleet-counter-grid"><button v-for="member in inspectedFleet.members" :key="member.id" class="fleet-counter-button" :class="{ active: selectedUnit?.id === member.id }" :data-ship-id="member.id" @click="selectFleetMember(member)"><ShipCounter :unit="member" :side="counterSide" /></button></div>
                <div v-else-if="(inspectedFleet.contact_level || 0) >= 4" class="enemy-damage-list">
                  <button v-for="row in enemyDamageRows" :key="row.member.id" class="enemy-damage-row" :class="{ active: selectedUnit?.id === row.member.id, wrecked: row.member.strength <= 0 }" :data-ship-id="row.member.id" :title="row.details.length ? row.details.join('、') : '无损伤'" @click="selectFleetMember(row.member)">
                    <span class="enemy-damage-name"><b>{{ row.member.name }}</b><code>{{ row.member.type }}</code></span>
                    <span class="enemy-damage-hp" :class="{ critical: row.health <= 34 }"><i :style="{ width: row.health + '%' }"></i></span>
                    <strong class="enemy-damage-value">{{ row.member.strength }}<small>/{{ row.member.max_strength }}</small></strong>
                    <span class="enemy-damage-badges">
                      <span v-for="badge in row.badges" :key="badge.label" class="damage-badge" :class="badge.kind">{{ badge.label }}</span>
                      <span v-if="!row.badges.length" class="damage-badge intact">完好</span>
                    </span>
                  </button>
                </div>
                <div v-else class="enemy-intel-note"><p>{{ enemyIntelNote(inspectedFleet) }}</p></div>
              </section>
              <section v-if="inspectedAirfield" class="fleet-workbench enemy-fleet-workbench airfield-intel-workbench" :data-airfield-id="inspectedAirfield.id">
                <header><div><span class="section-kicker">ENEMY AIRFIELD · {{ inspectedAirfield.coord }}</span><h2>{{ inspectedAirfield.name }}</h2><small>情报{{ inspectedAirfield.contact_level }}级 · 实时战损</small></div><div class="fleet-workbench-actions"><button aria-label="关闭机场情报" @click="inspectedAirfieldID = ''">×</button></div></header>
                <div class="airfield-intel-body">
                  <AirfieldCounter :airfield="airfieldContactStatus(inspectedAirfield)" />
                  <dl class="airfield-intel-stats">
                    <div><dt>生命值</dt><dd>{{ inspectedAirfield.strength ?? 0 }} / {{ inspectedAirfield.max_strength ?? 0 }}</dd></div>
                    <div><dt>可用跑道</dt><dd>{{ inspectedAirfield.runways }} / {{ inspectedAirfield.max_runways ?? inspectedAirfield.runways }}</dd></div>
                    <div><dt>重型防空</dt><dd>{{ inspectedAirfield.heavy_aa ?? 0 }}</dd></div>
                    <div><dt>轻型防空</dt><dd>{{ inspectedAirfield.light_aa ?? 0 }}</dd></div>
                    <div><dt>岸炮</dt><dd>{{ inspectedAirfield.coastal ?? 0 }}</dd></div>
                    <div><dt>雷达</dt><dd>{{ inspectedAirfield.radar ? '有' : '无' }}</dd></div>
                  </dl>
                </div>
              </section>
              <aside v-if="selectedUnit && shipDetailOpen" class="side-card selection-card map-selection-card">
                <div class="side-card-head"><span class="section-kicker">SHIP DETAIL</span><button aria-label="关闭舰船详情" @click="shipDetailOpen = false">×</button></div>
                <div class="selected-detail"><ShipCard :key="selectedUnit.id" :unit="selectedUnit" /></div>
              </aside>
              <details class="map-lobby-panel">
                <summary><span>战局 / 房间</span><b>{{ readyCount }} / {{ state.players.length }} 就绪</b></summary>
                <div class="map-lobby-content">
                  <section class="side-card commander-card"><div class="section-kicker">YOUR COMMAND</div><div class="commander-line"><div class="faction-seal" :class="`seal-${state.you.faction.toLowerCase()}`">{{ state.you.faction }}</div><div><h2>{{ state.you.name }}</h2><span>{{ state.you.faction === 'A' ? '苍蓝舰队' : '赤红舰队' }} · {{ state.you.is_host ? '房主' : '参战玩家' }}</span></div></div><div class="room-code"><span>ROOM</span><code>{{ state.room_id }}</code></div></section>
                  <section v-if="state.join_token" class="side-card invite-card"><div><span class="section-kicker">INVITE PLAYER B</span></div><button class="token-display" @click="copyToken"><code>{{ state.join_token }}</code><span>{{ copied ? '已复制' : '复制' }}</span></button><div v-if="watchPath" class="watch-row"><a :href="watchPath" target="_blank" rel="noreferrer">观战页 ↗</a><button @click="copyWatchLink">{{ watchCopied ? '链接已复制' : '复制观战链接' }}</button></div></section>
                  <section class="side-card orders-card"><div class="side-card-head"><span class="section-kicker">PENDING ORDERS</span><span>{{ state.orders.length }}</span></div><div v-if="state.orders.length" class="order-list"><div v-for="order in state.orders" :key="order.unit_id || order.task_id"><span>{{ order.task_id ? `任务 ${order.task_id}` : unitName(order.unit_id) }}</span><code>{{ order.type === 'MERGE_FLEET' ? `整编 → ${unitName(order.target_unit_id || '')}` : order.type === 'REORGANIZE_HEX' ? `整编hex ${order.coord}` : order.type === 'RECALL_AIR' ? '无线电召回' : order.type === 'RETASK_AIR' ? `无线电改令 → ${order.task_kind}` : `预定 → ${order.coord}` }}</code><button aria-label="取消命令" @click="clearOrder(order.unit_id || order.task_id || '')">×</button></div></div><p v-else class="empty-copy">尚未下达移动或整编命令</p>
                    <div class="supply-summary"><span>滩头物资</span><b>{{ shoreSupplyTotal }}</b><small v-for="(value, coord) in state.shore_supplies || {}" :key="coord">{{ coord }}:{{ value }}</small></div>
                    <div v-if="enemyShoreSupplyTotal" class="supply-summary enemy-supplies"><span>敌方滩头物资</span><b>{{ enemyShoreSupplyTotal }}</b><small v-for="(value, coord) in state.enemy_shore_supplies || {}" :key="coord">{{ coord }}:{{ value }}</small><small class="supply-hint">可派舰队岸轰清除</small></div>
                  </section>
                  <section class="side-card players-card"><div class="side-card-head"><span class="section-kicker">PLAYERS</span></div><div v-for="player in state.players" :key="player.id" class="player-row"><span class="player-faction" :class="`seal-${player.faction.toLowerCase()}`">{{ player.faction }}</span><span>{{ player.name }}</span><i :class="{ online: player.connected }"></i><b :class="{ ready: player.ready }">{{ player.ready ? '已就绪' : '部署中' }}</b></div></section>
                  <section class="side-card log-card"><div class="side-card-head"><span class="section-kicker">COMBAT LOG</span><button class="text-button" @click="activePanel = 'log'">全部日志</button></div><div class="event-list"><div v-for="event in orderedEvents.slice(0, 4)" :key="event.id" class="event-row"><span>T{{ String(event.turn).padStart(2, '0') }}</span><p>{{ event.text }}</p></div></div></section>
                </div>
              </details>
            </div>
            <div class="map-help"><b>操作方式</b><span>拖动 = 规划本回合航向（结算时执行）</span><i>→</i><span>黄框虚线为已规划，箭头标出目标hex</span><i>→</i><span>点击多舰队hex可整编</span><i>→</i><span>舰队面板里可设目的地，自动航行</span></div>
          </section>
        </div>

        <section v-else-if="activePanel === 'rules'" class="workspace-page">
          <span class="section-kicker">OPERATION BRIEF</span>
          <h1>{{ state.scenario.name }}</h1>
          <article class="briefing-card">
            <header><span class="section-kicker">MISSION</span><h2>战役简报</h2></header>
            <div class="briefing-copy">{{ state.scenario.description }}</div>
            <div v-if="state.objectives?.length" class="briefing-objectives">
              <div v-for="objective in state.objectives" :key="objective.id" class="briefing-objective">
                <span class="briefing-side" :class="objective.faction === 'A' ? 'side-a' : objective.faction === 'B' ? 'side-b' : 'side-score'">{{ objective.faction === 'A' ? '美军' : objective.faction === 'B' ? '日军' : '计分' }}</span>
                <p>{{ objective.text || objective.id }}</p>
              </div>
            </div>
          </article>
          <div class="document-grid"><article><h2>当前通信沙盒</h2><p>选择己方舰队，再把舰队拖到相邻绿色hex下达移动命令，点击目标hex不会移动。到达可移动时机后由服务器立即生效，普通/快速舰队移动后等待3回合，纯高速驱逐舰2回合，慢速6回合，极慢9回合；底条黄色表示可移动，蓝色表示已移动或冷却中，右上角箭头表示舰队朝向。</p><p>同一hex存在多个舰队时，点击hex弹出舰队列表。选择舰队可查看成员，选择整编可将任意两个同阵营舰队合并，整编同样由服务器立即生效。</p></article><article><h2>坐标与地形</h2><p>hex使用ABCD坐标，AB为行、CD为列。当前只能拖动至相邻且可通行的hex。暗礁一律不可进入；机场格与浅滩只有含 CV/CVL/CVE 的舰队不能进入，纯水面舰队可以靠岸靠滩并实施岸轰。蓝灰色虚线仅用于距离参考，不代表可移动。</p><p>正式航空、损管与剧本规则位于项目《规则书.md》，将随规则模块逐步接入。</p></article></div></section>
        <section v-else-if="activePanel === 'settings'" class="workspace-page"><span class="section-kicker">SETTINGS</span><h1>作战室设置</h1><div class="settings-grid"><div class="side-card"><h2>当前会话</h2><p>指挥官：{{ state.you.name }}</p><p>阵营：{{ state.you.faction }} · 内部房间 ID：<code>{{ state.room_id }}</code></p><p v-if="state.join_token">房间 token：<code>{{ state.join_token }}</code></p><p v-if="watchPath">观战地址：<a :href="watchPath" target="_blank" rel="noreferrer">{{ watchPath }} ↗</a></p><p>连接方式：{{ socketStatus === 'online' ? 'WebSocket实时同步' : 'WebSocket重连中' }}</p></div><div class="side-card"><h2>地图与资源</h2><p>坐标格式：ABCD（行 + 列）</p><p>地图与剧本从服务器持久化内容目录载入。</p><a class="secondary-link" href="/api/docs" target="_blank">查看API与AI扩展文档 ↗</a></div></div></section>
        <AviationPanel v-else-if="activePanel === 'aviation'" :key="selectedAirfield ? 'airfield-' + selectedAirfield.unit_id : 'fleet'" :aviation="aviationCarriers ?? state.aviation" :units="ownUnits" :fleet-id="aviationFleetId" :targets="strikeTargets" :player-faction="state.you.faction" :focus-unit-id="aviationFocusID" :selected-aircraft-id="selectedAirTaskID" :busy="actionBusy || locked" :locked="locked" :airfield="selectedAirfield" :radio-tasks="state.radio_tasks" :clock="state.clock" @command="airCommand" @select-aircraft="selectAircraft" @select-target="chooseStrikeTarget" @night-eta="openStrikeEta" />
        <ResultsPanel v-else-if="activePanel === 'results'" :state="state" />
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

    <!-- 命令被拒/同步失败：居中弹窗，玩家看完自己关。不再占页面布局，也不会挡着海图等下一次成功操作。 -->
    <div v-if="error" class="fleet-dialog error-dialog" @click.self="dismissError">
      <section class="fleet-modal error-modal" role="alertdialog" aria-modal="true" aria-label="操作未生效">
        <header><div><span class="section-kicker">COMMAND REJECTED</span><h2>操作未生效</h2></div><button aria-label="关闭" @click="dismissError">×</button></header>
        <p class="error-modal-text">{{ error }}</p>
        <footer class="error-modal-actions"><button class="apply-button" @click="dismissError">知道了</button></footer>
      </section>
    </div>

    <!-- 得分明细：侧栏 VP 旁边那个按钮点开的。同一笔账摊成两栏——我方得了几分、
         对方从我们身上拿了多少，每条都写清是哪一方的损失折算的。 -->
    <div v-if="scoreDetailOpen" class="fleet-dialog score-dialog" @click.self="scoreDetailOpen = false">
      <section class="fleet-modal score-modal" role="dialog" aria-modal="true" aria-label="战果分明细">
        <header><div><span class="section-kicker">VICTORY POINTS · BREAKDOWN</span><h2>得分明细</h2></div><button aria-label="关闭" @click="scoreDetailOpen = false">×</button></header>
        <p class="score-intro">总比分 {{ formatVP(scoreBoard.mine) }} : {{ formatVP(scoreBoard.enemy) }}。己方的损失同时记在对方账上，所以两边加起来不是零和——你沉一艘，我加一分。</p>
        <div class="score-columns">
          <article class="score-column">
            <header><span>我方得分</span><strong>{{ formatVP(scoreBoard.mine) }}</strong></header>
            <p v-if="!scoreDetail.mineRows.length" class="score-empty">还没有战果：击沉对方舰船、打瘫对方航母、击落对方航空算子都会计分。</p>
            <ul v-else class="score-lines">
              <li v-for="(line, index) in scoreDetail.mineRows" :key="line.kind + '-' + index">
                <span class="score-line-tag">{{ scoreLineTag(line) }}</span>
                <p>{{ line.text }}<small v-if="line.turn">（T{{ line.turn }}）</small></p>
                <b>+{{ formatVP(line.points) }}</b>
              </li>
            </ul>
          </article>
          <article class="score-column enemy">
            <header><span>对方得分</span><strong>{{ formatVP(scoreBoard.enemy) }}</strong></header>
            <p v-if="!scoreDetail.enemyRows.length" class="score-empty">对方还没有战果。</p>
            <ul v-else class="score-lines">
              <li v-for="(line, index) in scoreDetail.enemyRows" :key="line.kind + '-' + index">
                <span class="score-line-tag">{{ scoreLineTag(line) }}</span>
                <p>{{ line.text }}<small v-if="line.turn">（T{{ line.turn }}）</small></p>
                <b>+{{ formatVP(line.points) }}</b>
              </li>
            </ul>
          </article>
        </div>
        <footer class="score-rules">计分规则：航母按机库容量、战列舰 6、重巡 3、轻巡 2、驱逐/运输船 1；航母丧失作战能力按机库一半（先瘫后沉不重复领）；每个航空算子 0.25；投送目标按目标分值。</footer>
      </section>
    </div>

    <!-- 打击编队的时间表：点完目标就弹，出发前先看清"几点打到、几点落回来、会不会摸黑"。 -->
    <div v-if="strikeEtaOpen" class="fleet-dialog strike-eta-dialog" @click.self="strikeEtaTaskID = ''">
      <section class="fleet-modal strike-eta-modal" role="dialog" aria-modal="true" aria-label="打击编队预计时间">
        <header><div><span class="section-kicker">AIR STRIKE · TIMELINE</span><h2>{{ strikeEtaTask?.name }}</h2></div><button aria-label="关闭" @click="strikeEtaTaskID = ''">×</button></header>
        <dl v-if="strikeEtaTask?.eta_target || strikeEtaTask?.eta_landed" class="strike-eta-rows">
          <div><dt>抵达目标预计</dt><dd>{{ strikeEtaTask?.eta_target || '——' }}</dd></div>
          <div><dt>完全降落预计</dt><dd>{{ strikeEtaTask?.eta_landed || '——' }}</dd></div>
        </dl>
        <p v-if="strikeEtaTask && (strikeEtaTask.lands_at_night || strikeEtaTask.night_safe === false)" class="strike-eta-night" role="alert">{{ strikeEtaNightNote }}</p>
        <footer class="error-modal-actions"><button class="apply-button" @click="strikeEtaTaskID = ''">确认</button></footer>
      </section>
    </div>
  </main>
</template>