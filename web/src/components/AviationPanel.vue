<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import type { AirTask, AirUnit, AviationView, CarrierAirStatus, GroundAirStatus, Player, ReconHeading, StrikeTarget, Unit } from '../types'
import { airfieldTypeHint, airfieldTypeLabel, normalizeAirfieldType } from '../airfields'
import AircraftCounter from './AircraftCounter.vue'
import AirfieldCounter from './AirfieldCounter.vue'
import ShipCounter from './ShipCounter.vue'

type AirCommandType = 'LAUNCH_CAP' | 'LAUNCH_RECON' | 'LAUNCH_STRIKE' | 'LAUNCH_COVER' | 'LAUNCH_TRANSFER' | 'RETURN_TO_HANGAR' | 'RECOVER_AIR' | 'PRIORITIZE_AIR_TASK' | 'SET_AIR_CYCLE' | 'RECALL_AIR' | 'RETASK_AIR' | 'RETARGET_AIR' | 'SET_STRIKE_DISPERSION'
type CommandOptions = { heading?: ReconHeading; sector_width?: 60 | 120; task_id?: string; coord?: string; task_kind?: string; loadout?: string; disperse?: boolean; cycle?: boolean; lane?: number }
type TaskLane = { id: string; name: string; kind: string; slot?: number }
type AirZone = 'hangar' | 'deck' | 'elevator'

const props = defineProps<{ aviation: AviationView; units: Unit[]; fleetId: string; targets: StrikeTarget[]; playerFaction: Player['faction']; focusUnitId?: string; selectedAircraftId?: string; busy: boolean; clock?: import('../types').GameClock | null; locked?: boolean; airfield?: GroundAirStatus | null; radioTasks?: { task_id: string; kind: string; coord: string; airborne: number; used: boolean; chance: number; distance: number }[] }>()

// 本回合已就绪 = 冻结：界面只保留查看，任何会改动状态的动作都就地拦下，
// 不要等命令发到服务端再弹一个 409。
function refuseLocked() {
  if (!props.locked) return false
  selectionHint.value = '本回合已就绪，操作已锁定：等双方就绪由服务器结算，下一回合才能再操作'
  return true
}

// 每个空中编队每回合各一次无线电机会：发出即消耗，失败也不能再发。
type RadioStatus = { used: boolean; chance: number; distance: number }
function radioStatus(taskID: string): RadioStatus | undefined {
  return props.radioTasks?.find((item) => item.task_id === taskID)
}
// 每种编队能用的无线电操作：CAP 只能 RTB；打击/转场可以改目标或召回。
function radioActions(task: AirTask) {
  const actions = { rtb: false, retarget: false, retask: '' }
  if (!task.airborne_count) return actions
  if (task.kind === 'cap') actions.rtb = true
  if (task.kind === 'attack' || task.kind === 'transfer') {
    actions.rtb = true
    actions.retarget = true
    // 空中只能在打击和转场之间切换，不能改成 CAP 或侦察。
    actions.retask = task.kind === 'attack' ? 'transfer' : 'attack'
  }
  return actions
}

// 挂载选项：由机种决定（攻击机无对空、战斗机无炸弹鱼雷、水上飞机只有侦察与转场）。
const loadoutLabels: Record<string, string> = {
  'recon+tank': '侦察（副油箱）',
  recon: '侦察',
  'air+tank': '对空（副油箱）',
  air: '对空',
  'torpedo+tank': '鱼雷（副油箱）',
  torpedo: '鱼雷',
  'bomb+tank': '炸弹（副油箱）',
  bomb: '重型炸弹',
  'transfer+tank': '转场（副油箱）',
}

// 挂载选项：机种决定基础表，飞机自己的对地/对舰值决定能不能带弹药。
// 对地 0、对舰 0 的纯制空机不会出现炸弹/鱼雷选项。
function loadoutOptionsFor(aircraft: AirUnit) {
  if (aircraft.seaplane) return ['recon+tank', 'transfer+tank']
  const canBomb = (aircraft.anti_ground || 0) > 0 || (aircraft.anti_ship || 0) > 0
  // 鱼雷挂架是机型能力，不是"有对舰值就能挂"：B-26 能挂，B-25/B-17 不能。
  const canTorpedo = (aircraft.anti_ship || 0) > 0 && (aircraft.kind === 'torpedo_bomber' || aircraft.torpedo_capable === true)
  let options: string[]
  switch (aircraft.kind) {
    case 'fighter':
      // 有对地/对舰值的战斗轰炸型可以挂炸弹；默认仍是护航对空。
      options = ['air+tank', 'air', 'bomb+tank', 'bomb', 'transfer+tank']
      break
    case 'dive_bomber':
      // 俯冲轰炸机没有鱼雷挂架：D3A1/SBD 都只能挂炸弹。
      options = ['recon+tank', 'bomb+tank', 'bomb', 'transfer+tank']
      break
    case 'torpedo_bomber':
      // 攻击机也能挂侦察载荷：没有专用侦察机时由它们执行搜索。
      options = ['recon+tank', 'torpedo+tank', 'bomb+tank', 'bomb', 'torpedo', 'transfer+tank']
      break
    case 'level_bomber':
    case 'medium_bomber':
    case 'heavy_bomber':
    case 'strategic_bomber':
      // 水平轰炸机默认只有炸弹；只有带鱼雷挂架的 B-26 多出鱼雷选项。
      // B-25 / B-17 没有鱼雷挂架，不能因为 anti_ship > 0 就显示鱼雷。
      options = aircraft.torpedo_capable
        ? ['recon+tank', 'torpedo+tank', 'bomb+tank', 'bomb', 'torpedo', 'transfer+tank']
        : ['recon+tank', 'bomb+tank', 'bomb', 'transfer+tank']
      break
    case 'recon':
      options = ['recon+tank', 'transfer+tank']
      break
    default:
      options = ['transfer+tank']
  }
  return options.filter((option) => {
    if (option.startsWith('bomb')) return canBomb
    if (option.startsWith('torpedo')) return canTorpedo
    return true
  })
}

function loadoutName(loadout?: string, tankDropped?: boolean) {
  const name = loadout ? loadoutLabels[loadout] || '' : ''
  const parts = [name, tankDropped && loadout !== 'bomb' ? '已投副油箱' : ''].filter(Boolean)
  return parts.length ? ` · ${parts.join(' · ')}` : ''
}

function radioTitle(taskID: string) {
  const status = radioStatus(taskID)
  if (!status) return ''
  if (status.used) return '本回合已发过无线电，不能再次通信'
  return `无线电：最近友军 ${status.distance} hex，成功率 ${status.chance}%`
}

// 航空面板同时服务航母和机场：机场没有升降机和吊车，用跑道数表示起降容量。
// 水机基地也是机场——它只是没有跑道，但仍然有机库、防空和生命值，不能当航母画。
const AIRFIELD_TYPES = new Set(['small', 'medium', 'large', 'seaplane'])
function isAirfield(carrier: CarrierAirStatus) {
  return AIRFIELD_TYPES.has(normalizeAirfieldType(carrier.type))
}
function isSeaplaneBase(carrier: CarrierAirStatus) {
  return normalizeAirfieldType(carrier.type) === 'seaplane'
}
function spotsLabel(carrier: CarrierAirStatus) {
  if (isSeaplaneBase(carrier)) return `水上起降位 ${carrier.deck_length}`
  return isAirfield(carrier) ? `跑道 ${carrier.deck_length}` : `甲板 ${carrier.deck_length}`
}
const emit = defineEmits<{
  command: [type: AirCommandType, aircraftIDs: string[], options?: CommandOptions]
  selectAircraft: [aircraftID: string]
  selectTarget: [taskID: string]
  nightEta: [taskID: string]
}>()
const fleetCarrierIDs = computed(() => new Set(props.units.filter((unit) => (unit.fleet_id || unit.id) === props.fleetId).map((unit) => unit.id)))
// 机场的 unit_id 就是机场 ID（不在地图舰船投影里），因此先按 unit_id 直接匹配，再回退到"所属舰队"匹配。
const carriers = computed(() => {
  if (!props.fleetId) return props.aviation.carriers
  const direct = props.aviation.carriers.filter((carrier) => carrier.unit_id === props.fleetId)
  if (direct.length) return direct
  return props.aviation.carriers.filter((carrier) => fleetCarrierIDs.value.has(carrier.unit_id))
})
const tasks = computed(() => props.fleetId ? props.aviation.tasks.filter((task) => task.fleet_id === props.fleetId || (!task.fleet_id && fleetCarrierIDs.value.has(task.carrier_id))) : props.aviation.tasks)
const selectedCarrierID = ref('')
const selectionMode = ref<'single' | 'type'>('single')
const expandStacks = ref(false)
const selectedIDs = ref<string[]>([])
const draggingIDs = ref<string[]>([])
const dragOver = ref('')
const selectionHint = ref('')
const reconAircraftIDs = ref<string[]>([])
const reconTaskID = ref('')
const reconHeading = ref<ReconHeading>('W')
const reconWidth = ref<60 | 120>(60)
const lanes: TaskLane[] = [
  { id: 'recon-1', name: '空中侦察 I', kind: 'recon', slot: 0 },
  { id: 'recon-2', name: '空中侦察 II', kind: 'recon', slot: 1 },
  { id: 'strike-1', name: '打击任务 I', kind: 'attack', slot: 0 },
  { id: 'strike-2', name: '打击任务 II', kind: 'attack', slot: 1 },
  { id: 'cover-1', name: '空中掩护', kind: 'cover', slot: 0 },
  { id: 'transfer-1', name: '转场', kind: 'transfer', slot: 0 },
  { id: 'cap', name: 'CAP', kind: 'cap' },
]
const headings: { id: ReconHeading; arrow: string; name: string }[] = [
  { id: 'NE', arrow: '↗', name: '东北' },
  { id: 'E', arrow: '→', name: '东' },
  { id: 'SE', arrow: '↘', name: '东南' },
  { id: 'SW', arrow: '↙', name: '西南' },
  { id: 'W', arrow: '←', name: '西' },
  { id: 'NW', arrow: '↖', name: '西北' },
]
const headingRotation: Record<ReconHeading, number> = { E: 0, SE: 60, SW: 120, W: 180, NW: 240, NE: 300 }
const selectedCarrier = computed(() => carriers.value.find((carrier) => carrier.unit_id === selectedCarrierID.value))
// 当前看的这支部队是不是机场：机场没有升降机/甲板，面板里跟"提升、下降、甲板"有关的说法都要换掉，
// 否则岸基飞机在跑道上待命，提示却写着"甲板待飞 · 可拖回排队下降"。
const airfieldMode = computed(() => Boolean(selectedCarrier.value && isAirfield(selectedCarrier.value)))
const selectedUnit = computed(() => props.units.find((unit) => unit.id === selectedCarrierID.value))
const reconTask = computed(() => tasks.value.find((task) => task.id === reconTaskID.value && task.kind === 'recon'))
const reconAircraft = computed(() => {
  for (const carrier of carriers.value) {
    const aircraft = carrier.aircraft.find((item) => item.id === reconAircraftIDs.value[0])
    if (aircraft) return aircraft
  }
  return undefined
})

watch(carriers, (items) => {
  if (!items.some((item) => item.unit_id === selectedCarrierID.value)) selectedCarrierID.value = items.some((item) => item.unit_id === props.focusUnitId) ? props.focusUnitId || '' : items[0]?.unit_id || ''
}, { immediate: true })
watch(() => props.focusUnitId, (unitID) => {
  if (unitID && carriers.value.some((item) => item.unit_id === unitID)) {
    selectedCarrierID.value = unitID
    selectedIDs.value = []
  }
})
watch(selectedCarrier, (carrier) => {
  const valid = new Set(carrier?.aircraft.map((aircraft) => aircraft.id) || [])
  selectedIDs.value = selectedIDs.value.filter((id) => valid.has(id))
})

function selectCarrier(carrier: CarrierAirStatus) {
  selectedCarrierID.value = carrier.unit_id
  selectedIDs.value = []
  selectionHint.value = ''
}

function carrierUnit(unitID: string) {
  return props.units.find((unit) => unit.id === unitID)
}

function counterSelection(aircraft: AirUnit, stackIDs: string[]) {
  if (selectionMode.value === 'single') return [aircraft.id]
  const carrier = carriers.value.find((item) => item.unit_id === aircraft.carrier_id)
  if (!carrier) return stackIDs
  return carrier.aircraft.filter((item) => item.definition_id === aircraft.definition_id && item.status === aircraft.status).map((item) => item.id)
}

function selectCounter(aircraft: AirUnit, stackIDs: string[] = [aircraft.id]) {
  if (!['hangar', 'service', 'deck', 'ready'].includes(aircraft.status)) {
    emit('selectAircraft', aircraft.id)
    return
  }
  selectionHint.value = ''
  const group = counterSelection(aircraft, stackIDs)
  const allSelected = group.every((id) => selectedIDs.value.includes(id))
  if (selectionMode.value === 'single') {
    selectedIDs.value = allSelected ? [] : group
  } else if (allSelected) {
    selectedIDs.value = selectedIDs.value.filter((id) => !group.includes(id))
  } else {
    selectedIDs.value = [...new Set([...selectedIDs.value, ...group])]
  }
}

let dragPreview: HTMLElement | undefined

function startDrag(aircraft: AirUnit, event: DragEvent, stackIDs: string[] = [aircraft.id]) {
  if (refuseLocked()) {
    event.preventDefault()
    return
  }
  if (!selectedIDs.value.includes(aircraft.id)) selectedIDs.value = counterSelection(aircraft, stackIDs)
  else if (selectionMode.value === 'type') selectedIDs.value = [...new Set([...selectedIDs.value, ...counterSelection(aircraft, stackIDs)])]
  draggingIDs.value = [...selectedIDs.value]
  if (event.dataTransfer && event.currentTarget instanceof HTMLElement) {
    event.dataTransfer.effectAllowed = 'move'
    event.dataTransfer.setData('text/plain', draggingIDs.value.join(','))
    dragPreview?.remove()
    dragPreview = event.currentTarget.cloneNode(true) as HTMLElement
    dragPreview.classList.add('air-drag-preview')
    dragPreview.setAttribute('data-stack-count', String(draggingIDs.value.length))
    const bounds = event.currentTarget.getBoundingClientRect()
    dragPreview.style.cssText = `position:fixed;left:-1000px;top:0;width:${bounds.width}px;height:${bounds.height}px;pointer-events:none`
    const count = dragPreview.querySelector('.aircraft-stack-count')
    if (draggingIDs.value.length === 1) count?.remove()
    else if (count) count.textContent = `×${draggingIDs.value.length}`
    document.body.appendChild(dragPreview)
    event.dataTransfer.setDragImage(dragPreview, 40, 40)
  }
}

function endDrag() {
  dragPreview?.remove()
  dragPreview = undefined
  draggingIDs.value = []
  dragOver.value = ''
}

function assignTarget(target: string, taskID = '') {
  if (refuseLocked()) return
  const ids = [...(draggingIDs.value.length ? draggingIDs.value : selectedIDs.value)]
  endDrag()
  if (!ids.length) {
    selectionHint.value = '先选择机库或甲板上的飞机算子，再拖入任务栏'
    return
  }
  selectionHint.value = ''
  if (target === 'hangar') emitBatch('RETURN_TO_HANGAR', ids)
  else if (target === 'cap') emitBatch('LAUNCH_CAP', ids)
  else if (target.startsWith('cover-')) emitBatch('LAUNCH_COVER', ids)
  else if (target.startsWith('transfer-')) emitBatch('LAUNCH_TRANSFER', ids)
  else if (target.startsWith('strike-')) {
    const lane = lanes.find((item) => item.id === target)
    const existing = taskID ? tasks.value.find((task) => task.id === taskID) : lane ? laneTasks(lane).find((task) => !['launching', 'active'].includes(task.phase)) : undefined
    // 新建编队时把"拖进了哪一栏"一起发过去：服务端按它分配栏位，
    // 免得拖进 II 的新编队被"最小空位"规则又塞回 I。
    emitBatch('LAUNCH_STRIKE', ids, existing ? { task_id: existing.id } : { lane: (lane?.slot ?? 0) + 1 })
  }
  else if (target.startsWith('recon-')) {
    const lane = lanes.find((item) => item.id === target)
    const picked = taskID ? tasks.value.find((task) => task.id === taskID) : lane ? laneTasks(lane)[0] : undefined
    // 已经全部落地 / 正在往机库降的组不能再"加入"：那会把航向锁死在上一轮，
    // 玩家取消循环后想换方向就换不了。这种情况当作新建侦察组处理。
    const existing = picked && reconGroupAlive(picked) ? picked : undefined
    reconAircraftIDs.value = ids
    reconTaskID.value = existing?.id || ''
    reconHeading.value = existing?.heading || 'W'
    reconWidth.value = existing?.sector_width || 60
  } else selectionHint.value = '这一栏还没有可执行的任务'
}

// 机身还在机库/跑道上的任务算子可以直接撤回：拖回机库是同一个命令，
// 但算子在窄任务栏里被挤掉或者手边没鼠标时，玩家得有个能点的地方——
// 否则拖进转场以后就没有任何按钮能把这架飞机要回来。
const airborneStatus = new Set(['cap', 'recon', 'attack', 'attack_holding', 'cover', 'transfer'])
function withdrawableIDs(task: AirTask) {
  return (task.aircraft || [])
    .filter((aircraft) => !airborneStatus.has(aircraft.status))
    // 升降机上 / 刚刚落舰的算子服务端不收，等它这一轮回收走完再撤回。
    .filter((aircraft) => ['service', 'deck', 'ready'].includes(aircraft.status))
    .map((aircraft) => aircraft.id)
}

function withdrawTaskAircraft(task: AirTask) {
  if (refuseLocked()) return
  const ids = withdrawableIDs(task)
  if (!ids.length) return
  selectedIDs.value = ids
  emitBatch('RETURN_TO_HANGAR', ids)
}

function emitBatch(type: AirCommandType, ids: string[], options?: CommandOptions) {
  emit('command', type, ids, options)
  selectedIDs.value = []
}

function dropOnTask(task: AirTask, lane: TaskLane) {
  if (task.kind === 'recon' || (task.kind === 'attack' && !['launching', 'active'].includes(task.phase))) assignTarget(lane.id, task.id)
  else assignTarget(lane.id)
}

function recover(aircraftIDs: string[]) {
  if (refuseLocked()) return
  emit('command', 'RECOVER_AIR', aircraftIDs)
}

// 返航倒计时：引擎按"走完任务再回家要几回合"和"距天黑还剩几回合"折算过，
// 这里只负责把结果说不出来——玩家不该靠算距离去判断这波会不会天黑喂鱼。
function returnHint(task: AirTask): string {
  if (!task.airborne_count) return ''
  const need = task.return_turns || 0
  const left = props.clock?.night_in_turns ?? -1
  if (left < 0) return ''
  if (task.night_safe === false) return `距天黑 ${left} 回合，返航要 ${need} 回合——这波回不来`
  return ''
}

// 任务级循环开关（规则书 191 行）：直接切整个任务，不用一架架点。
function cyclicTask(task: AirTask) {
  return ['cap', 'recon', 'cover'].includes(task.kind)
}
function taskOnCycle(task: AirTask) {
  return (task.aircraft || []).some((aircraft) => aircraft.on_cycle)
}
function toggleTaskCycle(task: AirTask) {
  if (props.busy || refuseLocked()) return
  emit('command', 'SET_AIR_CYCLE', task.aircraft_ids, { cycle: !taskOnCycle(task) })
}

// 规则书 191 行：点算子开关自动任务循环。同一栈是"同型同状态"，一起开关。
function toggleCycle(aircraft: AirUnit, ids: string[]) {
  if (refuseLocked()) return
  const next = !aircraft.on_cycle
  emit('command', 'SET_AIR_CYCLE', ids.length ? ids : [aircraft.id], { cycle: next })
}

function launchRecon() {
  if (refuseLocked()) return
  if (!reconAircraftIDs.value.length) return
  emit('command', 'LAUNCH_RECON', reconAircraftIDs.value, { heading: reconHeading.value, sector_width: reconWidth.value, task_id: reconTaskID.value || undefined })
  reconAircraftIDs.value = []
  reconTaskID.value = ''
  selectedIDs.value = []
}

function taskPhase(task: AirTask) {
  if ((task.kind === 'cap' || task.kind === 'recon') && task.airborne_count > 0 && task.airborne_count < task.aircraft_ids.length) return `部分已升空 · ${task.airborne_count}/${task.aircraft_ids.length}`
  if (task.phase === 'servicing') return '任务整备中'
  // 机场没有升降机：整备完成先滑行到跑道，再在跑道待命；航母才是提升/甲板待飞。
  if (task.phase === 'lifting') return '升降机提升中'
  if (task.phase === 'taxiing') return '滑行至跑道'
  if (task.phase === 'preparing') return selectedCarrier.value && isAirfield(selectedCarrier.value) ? '跑道待飞' : '甲板待飞'
  if (task.phase === 'launching') return task.kind === 'attack' ? '编队离场' : '已升空'
  if (task.phase === 'assembling') return '舰队上空等待编队'
  if (task.phase === 'recovering') return `回收中 · ${task.airborne_count}/${task.aircraft_ids.length} 仍在空中`
  // 编队已经掉头就往"返航中"说：任务阶段还是 active，但玩家关心的是它在回还是在去。
  if (task.returning) return task.cannot_land ? '返航中 · 目标机库已满' : '返航中'
  return '出击中'
}

// 任务归属：机场任务的回收说法和航母不一样（着陆回机库 vs 着舰排队下降）。
function isAirfieldTask(task: AirTask) {
  const carrier = carriers.value.find((item) => item.unit_id === task.carrier_id)
  return Boolean(carrier && isAirfield(carrier))
}

function taskTitle(task: AirTask) {
  if (task.kind === 'recon') return '扇面侦察'
  if (task.kind === 'attack') return '攻击编队'
  if (task.kind === 'cover') return '空中掩护'
  if (task.kind === 'transfer') return '转场'
  if (task.kind === 'cap') return 'CAP'
  return task.name
}

function taskSummary(task: AirTask) {
  if (task.kind === 'attack') return `${task.airborne_count}/${task.aircraft_ids.length} 已升空 · ${task.current_coord || '母舰'}`
  if (task.kind === 'cover') return `${task.airborne_count}/${task.aircraft_ids.length} 已升空 · 掩护 ${task.target_coord || '未指定'}`
  if (task.kind === 'transfer') {
    const base = `${task.airborne_count}/${task.aircraft_ids.length} 已升空 · 目标 ${task.target_coord || '未指定'}`
    return task.cannot_land ? `${base} · ⚠ 目标机库已满，无法降落` : base
  }
  if (task.phase === 'recovering') return `${task.airborne_count}/${task.aircraft_ids.length} 仍在空中，${isAirfieldTask(task) ? '其余已着陆回机库' : '其余已落舰等待下降'}`
  if (task.kind === 'cap') return `${task.airborne_count}/${task.aircraft_ids.length} 已升空`
  if (['servicing', 'lifting', 'preparing'].includes(task.phase)) return `${task.airborne_count}/${task.aircraft_ids.length} 已升空`
  if (task.tracking_fleet_id) return '持续跟踪'
  if (task.returning) return '返航搜索'
  return `第 ${task.recon_distance || 0} 距离圈`
}

function targetLabel(task: AirTask) {
  return props.targets.find((target) => target.coord === task.target_coord)?.label || task.target_coord || ''
}

function selectTask(task: AirTask) {
  if (task.kind === 'recon' && selectedIDs.value.length) {
    assignTarget('recon-1', task.id)
    return
  }
  if (task.kind === 'attack' && selectedIDs.value.length && !['launching', 'active'].includes(task.phase)) {
    emitBatch('LAUNCH_STRIKE', selectedIDs.value, { task_id: task.id })
    return
  }
  if (['launching', 'assembling', 'active'].includes(task.phase)) emit('selectAircraft', task.id)
}

// 侦察组还算不算"活着"：有飞机在天上、或者还在整备/提升/准备起飞。
// 只剩回收和下降的组已经打完了这一轮，加入它只会继承一个用不上的航向。
function reconGroupAlive(task: AirTask) {
  if (task.airborne_count > 0) return true
  return ['servicing', 'lifting', 'preparing', 'launching'].includes(task.phase)
}

// 打击编队的"出发"从任务栏里退出：离场之后编队的整编已经结束，
// 详情、改目标、召回都在大地图那张编队牌上做。
// 任务栏只是"编队还在整备、准备出击"的地方：全员升空（含在舰队上空等目标/等集结）
// 之后编队属于海图，落回甲板/机库的算子属于回收面板，两者都不该再占着打击任务栏。
const departedPhases = ['assembling', 'launching', 'active', 'recovering']

// 栏位由服务端分配（task.lane，1 起）：编队组队时占一个空栏，全部升空之后栏位交还，
// 下一支新编队重新拿回 I。以前按"任务在列表里的位置 % 2"分栏，已出发的编队还挂在列表里
// 也照样占着一个位置，于是新编队总是落到 II，两支都在天上时编号越排越乱。
// 老存档的编队没有 lane（0），退回按未分配编队的位置轮排。
function laneTasks(lane: TaskLane) {
  const matching = tasks.value.filter((task) => task.kind === lane.kind)
  if (lane.slot === undefined) return matching
  // 只有"两栏一组"的任务才按槽位切：侦察两组、打击两栏。
  // 转场和空中掩护各自只有一栏，以前也被 index%2 切掉一半——
  // 图卢维尔4支转场编队只显示2支，被藏起来的那两支连撤回按钮都点不到。
  // 收窄成局部常量：闭包里读 lane.slot 时 TS 不再认前面的 undefined 判断。
  const slot = lane.slot
  if (lane.kind === 'recon') return matching.filter((_, index) => index === slot)
  if (lane.kind !== 'attack') return matching
  const pending = matching.filter((task) => !departedPhases.includes(task.phase))
  const assigned = pending.filter((task) => task.lane === slot + 1)
  const legacy = pending.filter((task) => !task.lane)
  return assigned.concat(legacy.filter((_, index) => index % 2 === slot))
}

// 已经在天上的打击编队数。它们的栏位已经交还，所以不挂在具体哪一栏上：
// 统一写在第 I 栏下面一句"去地图上点编队牌"，否则玩家看到编队凭空消失会以为丢了。
function departedCount(lane: TaskLane) {
  if (lane.kind !== 'attack' || lane.slot !== 0) return 0
  return tasks.value
    .filter((task) => task.kind === 'attack')
    .filter((task) => departedPhases.includes(task.phase)).length
}

// 升降机上的算子在哪儿显示：提升中的挂到甲板，下降中的挂回机库。
// 以前 elevator 状态两个区都不收，飞机在升降机上待两回合就"凭空消失"了。
function elevatorZone(aircraftID: string): AirZone {
  const elevator = selectedCarrier.value?.elevators?.find((item) => (item.aircraft_ids || []).includes(aircraftID))
  return elevator?.status === 'lifting' ? 'deck' : 'hangar'
}

function aircraftAt(zone: AirZone) {
  const aircraft = selectedCarrier.value?.aircraft || []
  // 升降机上的算子有自己的那一栏，不再混进甲板或机库：
  // 以前它们被按方向塞回甲板/机库，玩家看到"下降中"的飞机还挂在甲板上。
  if (zone === 'elevator') {
    return aircraft.filter((item) => item.status === 'elevator')
  }
  if (zone === 'hangar') {
    return aircraft.filter((item) => ['hangar', 'service'].includes(item.status))
  }
  return aircraft.filter((item) => ['deck', 'ready', 'recovery'].includes(item.status))
}

// 升降机栏：一艘船可能有多部升降机/吊车，每部一列，写着它现在在提升还是下降。
const elevatorRowVisible = computed(() => (selectedCarrier.value?.elevators?.length || 0) > 0)

function elevatorStatusLabel(status?: string) {
  if (status === 'lifting') return '▲ 提升中'
  if (status === 'lowering') return '▼ 下降中'
  if (status === 'damaged') return '损坏'
  return '空闲'
}

// 数据里的升降机 id 是 "a-cv-01-elevator-1" 这种长串，卡片上只留尾号。
function elevatorLabel(elevatorID: string, index: number) {
  const tail = elevatorID.split('-').pop() || ''
  return `升降机 ${tail || index + 1}`
}

function elevatorAircraft(elevator: { aircraft_ids?: string[] }) {
  const ids = new Set(elevator.aircraft_ids || [])
  return (selectedCarrier.value?.aircraft || []).filter((item) => ids.has(item.id))
}

function elevatorStacks(elevator: { aircraft_ids?: string[] }) {
  const stacks = new Map<string, { aircraft: AirUnit; ids: string[] }>()
  for (const aircraft of elevatorAircraft(elevator)) {
    const key = aircraftStackKey(aircraft)
    const stack = stacks.get(key)
    if (stack) stack.ids.push(aircraft.id)
    else stacks.set(key, { aircraft, ids: [aircraft.id] })
  }
  return [...stacks.values()]
}

// 叠加只合并"同型同状态"的算子；损伤/检修必须进 key。
// 以前 key 里没有这两个字段，一个机库里坏了一架的 B-17 会被并进健康那叠，
// 叠卡片只画第一架的标记，"损"标签就整个消失了——舰载机和岸基机都会中招。
function aircraftStackKey(aircraft: AirUnit, extra = '') {
  return `${aircraft.definition_id}|${aircraft.status}|${aircraft.task_id || ''}|${aircraft.elevator_id || ''}|${aircraft.damaged ? 'dmg' : 'ok'}|${aircraft.maintenance || 0}${extra}`
}

function aircraftStacks(zone: AirZone) {
  if (expandStacks.value) return aircraftAt(zone).map((aircraft) => ({ aircraft, ids: [aircraft.id] }))
  const stacks = new Map<string, { aircraft: AirUnit; ids: string[] }>()
  for (const aircraft of aircraftAt(zone)) {
    const key = aircraftStackKey(aircraft)
    const stack = stacks.get(key)
    if (stack) stack.ids.push(aircraft.id)
    else stacks.set(key, { aircraft, ids: [aircraft.id] })
  }
  return [...stacks.values()]
}

function taskAircraftStacks(task: AirTask) {
  const stacks = new Map<string, { aircraft: AirUnit; ids: string[] }>()
  for (const aircraft of task.aircraft) {
    const key = aircraftStackKey(aircraft, `|${aircraft.remaining_range}`)
    const stack = stacks.get(key)
    if (stack) stack.ids.push(aircraft.id)
    else stacks.set(key, { aircraft, ids: [aircraft.id] })
  }
  return [...stacks.values()]
}

function counterHint(aircraft: AirUnit) {
  // 损伤的算子优先提示原因：它拖不动、分不了任务，玩家得知道是"坏了"而不是"界面卡了"。
  if (aircraft.damaged) return '损伤 · 必须修复后才能再次出动'
  if ((aircraft.maintenance || 0) > 0) return `检修中 · 还需${aircraft.maintenance}回合`
  if (aircraft.status === 'service') return '整备中 · 可拖回机库'
  if (aircraft.status === 'elevator') return elevatorZone(aircraft.id) === 'deck' ? '提升中 · 即将上甲板' : '下降中 · 即将进机库'
  // 岸基没有升降机：落地就是在跑道上，下一步回机位，不存在"排队下降"。
  if (aircraft.status === 'recovery') return airfieldMode.value ? '已着陆 · 即将回机库' : '已着舰 · 等待升降机下降'
  if (aircraft.status === 'deck' || aircraft.status === 'ready') {
    if (airfieldMode.value) return aircraft.task_id ? '跑道待飞 · 可拖回机库取消任务' : '可拖入任务立即放飞'
    return aircraft.task_id ? '甲板待飞 · 可拖回排队下降' : '可拖入任务立即放飞'
  }
  const loadout = loadoutName(aircraft.loadout, aircraft.tank_dropped)
  // 在空中的算子要能分出"出击中"和"返航中"：单看 status 分不出来，
  // 往目标飞和往回飞都是 attack，只有 returning 这个标记能回答"它现在是在去还是回"。
  const airborne = ['cap', 'recon', 'attack', 'attack_holding', 'cover', 'transfer'].includes(aircraft.status)
  if (airborne && aircraft.returning) return `返航中 · 剩余航程 ${aircraft.remaining_range}${loadout}`
  if (aircraft.status === 'attack_holding') return `出击中 · 等待编队 · 剩余航程 ${aircraft.remaining_range}${loadout}`
  if (airborne) return `出击中 · 剩余航程 ${aircraft.remaining_range}${loadout}`
  return '拖至任务栏开始整备'
}

function hangarCounterHint(aircraft: AirUnit) {
  if (!aircraft.task_id) return counterHint(aircraft)
  const task = tasks.value.find((item) => item.id === aircraft.task_id)
  return `已加入${task ? taskTitle(task) : '航空任务'} · 整备中`
}

// 机场没有机库（默认上限 99999 = 实际上不限），面板上写"无上限"而不是显示一个假数字；
// 剧本显式配了小上限的机场照常显示 N / M。
const AIRFIELD_HANGAR_UNLIMITED = 99999
function hangarText(carrier: CarrierAirStatus) {
  if (carrier.hangar_capacity >= AIRFIELD_HANGAR_UNLIMITED) return `${carrier.hangar_used}（无上限）`
  return `${carrier.hangar_used} / ${carrier.hangar_capacity}`
}

function operationName(carrier: CarrierAirStatus) {
  if (isAirfield(carrier)) return `跑道待命 · ${carrier.deck_length} 条`
  if (carrier.operation === 'launch') return '甲板：放飞作业'
  if (carrier.operation === 'recover') return '甲板：回收作业'
  return carrier.deck_length ? '甲板待命' : '水机机库'
}

</script>

<template>
  <section class="workspace-page aviation-workspace aviation-command-deck">
    <header class="aviation-title">
      <div><span class="section-kicker">AVIATION COMMAND</span><h1>{{ selectedUnit?.fleet_name || '航空指挥界面' }}</h1></div>
      <div v-if="selectedCarrier" class="aviation-title-status"><span>{{ selectedCarrier.name }}</span><strong>{{ operationName(selectedCarrier) }}</strong></div>
    </header>
    <p v-if="selectionHint" class="air-selection-hint" role="alert">{{ selectionHint }}</p>

    <div v-if="!carriers.length" class="aviation-placeholder"><span>△</span><h2>当前舰队没有航空作业舰</h2><p>在剧本中部署航母或搭载水上飞机的舰船，或先在地图上点选机场。</p></div>
    <div v-else class="air-task-lanes mission-grid">
      <section v-for="lane in lanes" :key="lane.id" class="air-task-lane" :class="[`mission-${lane.id}`, { 'drop-active': dragOver === lane.id }]" :data-lane-id="lane.id" @click="assignTarget(lane.id)" @dragover.prevent="dragOver = lane.id" @dragleave="dragOver = ''" @drop.prevent="assignTarget(lane.id)">
        <header><span>{{ lane.name }}</span><small>{{ laneTasks(lane).length }} 项</small></header>
        <div v-if="laneTasks(lane).length" class="air-task-list">
          <article v-for="task in laneTasks(lane)" :key="task.id" class="air-task with-air-task-counters" :class="{ prioritized: task.priority === 1, 'recon-task': task.kind === 'recon' }" :data-task-id="task.id" :data-phase="task.phase" @click.stop="selectTask(task)" @dragover.prevent.stop="dragOver = `task-${task.id}`" @dragleave="dragOver = ''" @drop.prevent.stop="dropOnTask(task, lane)">
            <div class="air-task-counters" @click.stop>
              <div v-for="stack in taskAircraftStacks(task)" :key="`${stack.aircraft.id}-${stack.aircraft.status}`" :data-task-aircraft-id="stack.aircraft.id" :data-status="stack.aircraft.status">
                <AircraftCounter :aircraft="stack.aircraft" :stack-count="stack.ids.length" :player-faction="playerFaction" :class="{ selected: selectedAircraftId && stack.ids.includes(selectedAircraftId) }" :draggable="['service', 'deck', 'ready'].includes(stack.aircraft.status)" @click="selectCounter(stack.aircraft)" @toggle-cycle="toggleCycle(stack.aircraft, stack.ids)" @dragstart="['service', 'deck', 'ready'].includes(stack.aircraft.status) && startDrag(stack.aircraft, $event, stack.ids)" @dragend="endDrag" />
                <small>{{ counterHint(stack.aircraft) }}</small>
                <select
                  v-if="!['cap', 'recon', 'attack', 'attack_holding'].includes(stack.aircraft.status)"
                  class="loadout-picker"
                  :value="stack.aircraft.loadout"
                  :disabled="busy"
                  :aria-label="`${stack.aircraft.model} 挂载`"
                  @click.stop
                  @change="emit('command', 'SET_LOADOUT' as AirCommandType, stack.ids, { loadout: ($event.target as HTMLSelectElement).value })"
                >
                  <option v-for="option in loadoutOptionsFor(stack.aircraft)" :key="option" :value="option">{{ loadoutLabels[option] || option }}</option>
                </select>
                <button v-if="stack.aircraft.status === 'cap'" :disabled="busy" @click.stop="recover(stack.ids)">{{ airfieldMode ? '着陆并回收' : '着舰并排队下降' }}</button>
              </div>
            </div>
            <aside class="air-task-content">
              <!-- 夜间返航不再在卡片里铺一行红字（会把编队牌挤变形）：只留一个小标记，
                   点开是"抵达/降落预计时间 + 返航时间在夜间"的弹窗，详情看弹窗。 -->
              <div class="air-task-copy"><strong>{{ taskTitle(task) }}</strong><span class="air-task-phase" :class="`phase-${task.phase}`">{{ taskPhase(task) }}</span><small>{{ taskSummary(task) }}</small><button v-if="returnHint(task)" type="button" class="task-night-warning" :title="returnHint(task)" @click.stop="emit('nightEta', task.id)">！返航时间在夜间</button></div>
              <div v-if="['attack', 'cover', 'transfer'].includes(task.kind)" class="strike-target-control" :class="{ 'cannot-land': task.cannot_land }">
                <div class="strike-target-card"><span>⬡</span><div><strong>{{ task.target_coord ? targetLabel(task) : '目标 未设置' }}</strong><small>{{ task.target_coord ? `目标 ${task.target_coord}` : '在地图上点选目标' }}</small></div></div>
                <button :aria-label="task.target_coord ? '更换目标' : '选择目标'" :title="'在地图上点选打击目标'" @click.stop="emit('selectTarget', task.id)">{{ task.target_coord ? '更换' : '选择' }}</button>
                <span class="strike-dispersion" :data-disperse="String(Boolean(task.disperse))">
                  <button :class="{ active: !task.disperse }" :disabled="busy" title="集中攻击：按目标权重表优先扑向航母" @click.stop="emit('command', 'SET_STRIKE_DISPERSION', [task.id], { task_id: task.id, disperse: false })">集中</button>
                  <button :class="{ active: Boolean(task.disperse) }" :disabled="busy" title="分散攻击：每个算子均匀随机选择编队中任意一艘船" @click.stop="emit('command', 'SET_STRIKE_DISPERSION', [task.id], { task_id: task.id, disperse: true })">分散</button>
                </span>
              </div>
              <div v-if="task.kind === 'recon'" class="recon-task-dial" :data-sector-width="task.sector_width">
                <svg viewBox="0 0 100 100" aria-hidden="true"><g :transform="`rotate(${headingRotation[task.heading || 'W']} 50 50)`"><path :d="task.sector_width === 60 ? 'M50 50 L84.64 30 A40 40 0 0 1 84.64 70 Z' : 'M50 50 L70 5.36 A40 40 0 0 1 70 94.64 Z'" /></g><circle cx="50" cy="50" r="40" /><circle cx="50" cy="50" r="3" /></svg>
                <div><b>{{ task.sector_width }}°</b><small>{{ task.heading }} · {{ task.recon_radius }} hex</small></div>
              </div>
              <button
                v-if="cyclicTask(task)"
                class="task-cycle"
                :class="{ active: taskOnCycle(task) }"
                :aria-label="taskOnCycle(task) ? '取消自动任务循环' : '开启自动任务循环'"
                :title="taskOnCycle(task) ? '自动循环中：降落后自动再出动一轮，点击取消' : '自动循环已关闭，点击开启'"
                :disabled="busy"
                @click.stop="toggleTaskCycle(task)"
              >🔁</button>
              <span class="task-priority">P{{ task.priority }}</span>
              <span class="task-queue">队列 {{ task.queue_position }}</span>
              <button class="task-flag" :class="{ active: task.priority === 1 }" :aria-label="`将${taskTitle(task)}设为最高优先级`" :disabled="busy" @click.stop="emit('command', 'PRIORITIZE_AIR_TASK', [task.id])">⚑</button>
              <button
                v-if="withdrawableIDs(task).length"
                class="task-unassign"
                :disabled="busy || locked"
                :aria-label="`把${taskTitle(task)}的算子撤回机库`"
                title="撤回机库：取消这个任务里还在机库/跑道上的算子（空中的请用召回）"
                @click.stop="withdrawTaskAircraft(task)"
              >⇤ 撤回</button>
              <span v-if="radioActions(task).rtb || radioActions(task).retarget" class="task-radio-group">
                <button v-if="radioActions(task).retarget" class="task-radio" :class="{ spent: radioStatus(task.id)?.used }" :title="radioTitle(task.id)" :aria-label="`无线电改目标${taskTitle(task)}`" :disabled="busy || radioStatus(task.id)?.used" @click.stop="emit('selectTarget', task.id)">◎</button>
                <button v-if="radioActions(task).retask" class="task-radio" :class="{ spent: radioStatus(task.id)?.used }" :title="`改令 —— ${radioTitle(task.id)}`" :aria-label="`无线电改令${taskTitle(task)}`" :disabled="busy || radioStatus(task.id)?.used" @click.stop="emit('command', 'RETASK_AIR', [], { task_id: task.id, task_kind: radioActions(task).retask })">{{ radioActions(task).retask === 'transfer' ? '⇢' : '⇠' }}</button>
                <button v-if="radioActions(task).rtb" class="task-radio" :class="{ spent: radioStatus(task.id)?.used }" :title="`RTB —— ${radioTitle(task.id)}`" :aria-label="`无线电召回${taskTitle(task)}`" :disabled="busy || radioStatus(task.id)?.used" @click.stop="emit('command', 'RECALL_AIR', [], { task_id: task.id })">⟲</button>
              </span>
            </aside>
          </article>
        </div>
        <div v-else class="mission-empty">
          <svg v-if="lane.kind === 'recon'" viewBox="0 0 80 80" aria-hidden="true"><circle cx="40" cy="40" r="29" /><path class="empty-sector" d="M40 40 L63 27 A27 27 0 0 1 63 53 Z" /><path d="M40 11V18M40 62V69M11 40H18M62 40H69" /></svg>
          <svg v-else-if="lane.kind === 'attack'" viewBox="0 0 80 80" aria-hidden="true"><path d="M40 9 67 24 67 56 40 71 13 56 13 24Z" /><path d="M40 22V58M25 40H55" /></svg>
          <svg v-else viewBox="0 0 80 80" aria-hidden="true"><path d="M36 10h8l3 22 23 12v6l-24-6-2 12 9 5v4l-13-3-13 3v-4l9-5-2-12-24 6v-6l23-12Z" /></svg>
          <p>{{ lane.kind === 'attack' ? '拖入飞机建立编队' : '拖入飞机分配任务' }}</p>
          <small v-if="lane.kind === 'recon'">60° · 自动侦察航程</small>
        </div>
        <small v-if="departedCount(lane)" class="lane-departed-hint">{{ departedCount(lane) }} 支编队已出发 · 在地图上点编队牌改目标 / 召回</small>
      </section>

      <section v-if="selectedCarrier" class="air-storage-zone flight-deck-zone mission-deck" data-deck-zone>
        <header><div><span class="section-kicker">{{ isSeaplaneBase(selectedCarrier) ? 'SEAPLANE BASE' : isAirfield(selectedCarrier) ? 'RUNWAY' : 'FLIGHT DECK' }}</span><h3>{{ isSeaplaneBase(selectedCarrier) ? '水上起降 / 待回收' : isAirfield(selectedCarrier) ? '跑道 / 待回收' : '飞行甲板 / 待回收' }}</h3></div><strong>{{ selectedCarrier.deck_used }} / {{ selectedCarrier.deck_length }}</strong></header>
        <div class="air-counter-grid">
          <div v-for="stack in aircraftStacks('deck')" :key="stack.aircraft.id" class="air-counter-item" :data-aircraft-id="stack.aircraft.id" :data-status="stack.aircraft.status" :data-stack-count="stack.ids.length">
            <AircraftCounter :aircraft="stack.aircraft" :stack-count="stack.ids.length" :player-faction="playerFaction" :class="{ selected: stack.ids.some((id) => selectedIDs.includes(id)), selectable: stack.aircraft.status !== 'recovery' }" :draggable="stack.aircraft.status !== 'recovery'" @click="stack.aircraft.status !== 'recovery' && selectCounter(stack.aircraft, stack.ids)" @toggle-cycle="toggleCycle(stack.aircraft, stack.ids)" @dragstart="stack.aircraft.status !== 'recovery' && startDrag(stack.aircraft, $event, stack.ids)" @dragend="endDrag" />
            <span class="air-no-action">{{ counterHint(stack.aircraft) }}</span>
          </div>
          <button v-if="selectedIDs.length" class="return-hangar-button" :disabled="busy" @click="assignTarget('hangar')">{{ airfieldMode ? '取消任务并回机库' : '取消任务并排队下降' }}</button>
          <span v-if="!aircraftAt('deck').length" class="empty-air-zone">{{ airfieldMode ? '跑道空置' : '甲板空置' }}</span>
        </div>

      </section>

      <!-- 升降机单独一栏：飞机在提升/下降时既不挂在甲板上也不在机库里，
           玩家要能一眼看出"这部升降机在往上还是往下、上面是谁"。 -->
      <section v-if="selectedCarrier && elevatorRowVisible" class="air-storage-zone elevator-zone mission-elevator">
        <header><div><span class="section-kicker">ELEVATOR</span><h3>升降机</h3></div><strong>{{ aircraftAt('elevator').length }}</strong></header>
        <div class="elevator-grid">
          <article v-for="(elevator, index) in selectedCarrier.elevators || []" :key="elevator.id" class="elevator-card" :class="'elevator-' + elevator.status" :data-elevator-id="elevator.id" :data-status="elevator.status">
            <header><strong>{{ elevatorLabel(elevator.id, index) }}</strong><span :class="'elevator-status-' + elevator.status">{{ elevatorStatusLabel(elevator.status) }}</span></header>
            <div v-if="elevatorStacks(elevator).length" class="air-counter-grid">
              <div v-for="stack in elevatorStacks(elevator)" :key="stack.aircraft.id" class="air-counter-item" :data-aircraft-id="stack.aircraft.id" :data-stack-count="stack.ids.length">
                <AircraftCounter :aircraft="stack.aircraft" :stack-count="stack.ids.length" :player-faction="playerFaction" />
                <span class="air-no-action">{{ counterHint(stack.aircraft) }}</span>
              </div>
            </div>
            <p v-else class="elevator-idle">空闲</p>
          </article>
        </div>
      </section>

      <section v-if="selectedCarrier" class="carrier-air-card mission-ship" :class="{ focused: focusUnitId === selectedCarrier.unit_id }" :data-carrier-id="selectedCarrier.unit_id">
        <AirfieldCounter v-if="airfield" :airfield="airfield" class="mission-airfield-counter" />
        <ShipCounter v-else-if="selectedUnit && !isAirfield(selectedCarrier)" :unit="selectedUnit" />
        <div v-else-if="isAirfield(selectedCarrier)" class="airfield-badge">
          <span :title="airfieldTypeHint(selectedCarrier.type)">{{ airfieldTypeLabel(selectedCarrier.type) }}</span>
          <b>{{ selectedCarrier.name }}</b>
          <small>{{ isSeaplaneBase(selectedCarrier) ? `水上起降位 ${selectedCarrier.deck_length}` : `可用跑道 ${selectedCarrier.deck_length} 条` }}</small>
        </div>
        <div class="carrier-current-info">
          <header><span class="unit-type-badge">{{ selectedCarrier.type }}</span><h2>{{ selectedCarrier.name }}</h2></header>
          <div v-if="airfield" class="airfield-spec">
            <span>生命值 <b>{{ airfield.strength }} / {{ airfield.max_strength }}</b></span>
            <span v-if="!isSeaplaneBase(selectedCarrier)" title="可用跑道 = ceil(生命值 / 5)，不超过总跑道数">可用跑道 <b>{{ airfield.runways }} / {{ airfield.max_runways }}</b></span>
            <span v-else>水上起降位 <b>{{ airfield.runways }}</b></span>
            <span>重型防空 <b>{{ airfield.heavy_aa }}</b> · 轻型防空 <b>{{ airfield.light_aa }}</b></span>
            <span>岸炮 <b>{{ airfield.coastal }}</b></span>
            <span>雷达 <b>{{ airfield.radar ? '有' : '无' }}</b></span>
          </div>
          <div class="hangar-meter"><span>停机位</span><strong>{{ hangarText(selectedCarrier) }}</strong></div>
          <div class="carrier-operation"><span :class="`operation-${selectedCarrier.operation || 'idle'}`">{{ operationName(selectedCarrier) }}</span><small>{{ isSeaplaneBase(selectedCarrier) ? `水上起降位 ${selectedCarrier.deck_length}` : isAirfield(selectedCarrier) ? `可用跑道 ${selectedCarrier.deck_length}` : `起飞 ${selectedCarrier.launch_used} / ${selectedCarrier.deck_length}` }}</small></div>
          <p v-if="selectedCarrier.blocked_reason" class="carrier-blocked">⚠ {{ selectedCarrier.blocked_reason }}</p>
          <p v-else-if="selectedCarrier.prevented_by" class="carrier-blocked">⚠ 甲板已满：{{ selectedCarrier.prevented_by }}</p>
        </div>
      </section>

      <section v-if="selectedCarrier" class="air-storage-zone hangar-zone mission-hangar" :class="{ 'drop-active': dragOver === 'hangar' }" data-drop-target="hangar" @click="assignTarget('hangar')" @dragover.prevent="dragOver = 'hangar'" @dragleave="dragOver = ''" @drop.prevent="assignTarget('hangar')">
        <header><div><span class="section-kicker">HANGAR</span><h3>机库 / 任务整备</h3></div><strong>{{ hangarText(selectedCarrier) }}</strong></header>
        <div class="air-counter-grid">
          <div v-for="stack in aircraftStacks('hangar')" :key="stack.aircraft.id" class="air-counter-item" :data-aircraft-id="stack.aircraft.id" :data-status="stack.aircraft.status" :data-stack-count="stack.ids.length">
            <AircraftCounter :aircraft="stack.aircraft" :stack-count="stack.ids.length" :player-faction="playerFaction" :class="{ selected: stack.ids.some((id) => selectedIDs.includes(id)), selectable: !stack.aircraft.task_id }" :draggable="!stack.aircraft.task_id" @click.stop="!stack.aircraft.task_id && selectCounter(stack.aircraft, stack.ids)" @toggle-cycle="toggleCycle(stack.aircraft, stack.ids)" @dragstart="!stack.aircraft.task_id && startDrag(stack.aircraft, $event, stack.ids)" @dragend="endDrag" />
            <span class="air-no-action">{{ hangarCounterHint(stack.aircraft) }}</span>
          </div>
          <p v-if="!aircraftAt('hangar').length" class="empty-hangar">机库空置</p>
        </div>
      </section>

      <aside class="air-selection-toolbar mission-controls">
        <header><span>航空单位选择模式</span><b>{{ selectedIDs.length }}</b></header>
        <div><button :class="{ active: selectionMode === 'single' }" title="单选" @click="selectionMode = 'single'; selectedIDs = []"><span>▣</span><small>单选</small></button><button :class="{ active: selectionMode === 'type' }" title="同型全选" @click="selectionMode = 'type'; selectedIDs = []"><span>▦</span><small>同型全选</small></button></div>
        <footer><button :class="{ active: expandStacks }" @click="expandStacks = !expandStacks; selectedIDs = []">{{ expandStacks ? '合并叠加' : '展开叠加' }}</button><button class="clear-selection" @click="selectedIDs = []">清除</button></footer>
      </aside>

      <aside class="carrier-roster mission-roster">
        <button v-for="carrier in carriers" :key="carrier.unit_id" :class="{ active: selectedCarrierID === carrier.unit_id, airfield: isAirfield(carrier) }" :data-roster-id="carrier.unit_id" @click="selectCarrier(carrier)">
          <ShipCounter v-if="carrierUnit(carrier.unit_id)" :unit="carrierUnit(carrier.unit_id)!" />
          <span v-else-if="isAirfield(carrier)" class="roster-airfield"><b>{{ carrier.name }}</b><small>{{ carrier.deck_length }} 条跑道</small></span>
          <span v-else>{{ carrier.type }} · {{ carrier.name }}</span>
        </button>
      </aside>
    </div>

    <div v-if="reconAircraft" class="recon-dialog" @click.self="reconAircraftIDs = []; reconTaskID = ''">
      <section class="recon-modal" role="dialog" aria-modal="true" aria-label="配置扇面侦察">
        <header><div><span class="section-kicker">SECTOR RECONNAISSANCE</span><h2>{{ reconTask ? '加入已有侦察组' : '配置扇面侦察' }}</h2></div><button aria-label="关闭" @click="reconAircraftIDs = []; reconTaskID = ''">×</button></header>
        <p>{{ reconAircraft.model }} · {{ reconAircraftIDs.length }}个算子。{{ reconTask ? `加入${reconTask.name}，沿用该组的航向、扇面和半径。` : '确认后进入所选侦察任务栏。' }}未备好的算子依次完成任务整备、{{ airfieldMode ? '滑行到跑道' : '升降机提升' }}并自动放飞，已备好的算子立即起飞。服务器保存扇面配置，并以实际起飞时的母舰位置为顶点进行发现判定。</p>
        <fieldset class="recon-heading-picker" :disabled="!!reconTask"><legend>预定侦察航向</legend><button v-for="heading in headings" :key="heading.id" :class="{ active: reconHeading === heading.id }" :aria-label="`${heading.id} ${heading.name}`" @click="reconHeading = heading.id"><span>{{ heading.arrow }}</span><b>{{ heading.id }}</b><small>{{ heading.name }}</small></button></fieldset>
        <div class="recon-options">
          <fieldset :disabled="!!reconTask"><legend>扇面角度</legend><div class="segmented-control"><button :class="{ active: reconWidth === 60 }" @click="reconWidth = 60">60°<small>标准发现率</small></button><button :class="{ active: reconWidth === 120 }" @click="reconWidth = 120">120°<small>发现率减半</small></button></div></fieldset>
          <div class="recon-radius"><span>侦察距离</span><strong>{{ reconTask?.recon_radius || reconAircraft.recon_radius_limit }} hex</strong><small>{{ reconTask ? '沿用已有侦察组参数' : '固定为该机型最大搜索半径，不可调整' }}</small></div>
        </div>
        <div class="recon-preview">
          <svg viewBox="0 0 200 200" role="img" aria-label="侦察方向预览" :data-heading="reconHeading" :data-width="reconWidth">
            <g :transform="`rotate(${headingRotation[reconHeading]} 100 100)`"><path class="recon-preview-sector" :d="reconWidth === 60 ? 'M100 100 L169.28 60 A80 80 0 0 1 169.28 140 Z' : 'M100 100 L140 30.72 A80 80 0 0 1 140 169.28 Z'" /></g>
            <g class="recon-compass-lines"><circle cx="100" cy="100" r="80" /><line x1="100" y1="100" x2="180" y2="100" /><line x1="100" y1="100" x2="140" y2="169.28" /><line x1="100" y1="100" x2="60" y2="169.28" /><line x1="100" y1="100" x2="20" y2="100" /><line x1="100" y1="100" x2="60" y2="30.72" /><line x1="100" y1="100" x2="140" y2="30.72" /></g>
            <g class="recon-compass-labels"><text x="184" y="104">E</text><text x="143" y="164">SE</text><text x="47" y="164">SW</text><text x="7" y="104">W</text><text x="46" y="43">NW</text><text x="139" y="43">NE</text></g>
            <circle class="recon-compass-center" cx="100" cy="100" r="4" />
          </svg>
          <div><b>预览方向</b><span>{{ reconHeading }} · {{ reconWidth }}°</span><small>实际覆盖hex由服务端在起飞后计算</small></div>
        </div>
        <div class="recon-brief"><span>顶点</span><strong>实际起飞时的母舰位置</strong><i>→</i><span>航线</span><strong>{{ reconHeading }}</strong><i>→</i><span>扇面</span><strong>{{ reconWidth }}° / {{ reconTask?.recon_radius || reconAircraft.recon_radius_limit }} hex</strong></div>
        <footer><button class="reset-button" @click="reconAircraftIDs = []; reconTaskID = ''">取消</button><button class="apply-button" :disabled="busy" @click="launchRecon">{{ ['deck', 'ready'].includes(reconAircraft.status) ? '起飞执行扇面侦察' : '加入侦察任务' }}</button></footer>
      </section>
    </div>

  </section>
</template>