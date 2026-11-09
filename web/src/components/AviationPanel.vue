<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import type { AircraftElevator, AirTask, AirUnit, AviationView, CarrierAirStatus, Player, ReconHeading, Unit } from '../types'
import AircraftCounter from './AircraftCounter.vue'
import ShipCounter from './ShipCounter.vue'

type AirCommandType = 'LAUNCH_CAP' | 'LAUNCH_RECON' | 'LAUNCH_STRIKE' | 'RETURN_TO_HANGAR' | 'RECOVER_AIR' | 'PRIORITIZE_AIR_TASK'
type CommandOptions = { heading?: ReconHeading; sector_width?: 60 | 120; recon_radius?: number; task_id?: string }
type TaskLane = { id: string; name: string; kind: string; slot?: number }
type AirZone = 'hangar' | 'elevator' | 'deck'

const props = defineProps<{ aviation: AviationView; units: Unit[]; playerFaction: Player['faction']; focusUnitId?: string; selectedAircraftId?: string; busy: boolean }>()
const emit = defineEmits<{
  command: [type: AirCommandType, aircraftIDs: string[], options?: CommandOptions]
  selectAircraft: [aircraftID: string]
}>()
const carriers = computed(() => props.aviation.carriers)
const tasks = computed(() => props.aviation.tasks)
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
  { id: 'asw', name: '反潜巡逻', kind: 'asw' },
  { id: 'cap', name: 'CAP', kind: 'cap' },
  { id: 'escort', name: '空中掩护', kind: 'escort' },
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
const selectedUnit = computed(() => props.units.find((unit) => unit.id === selectedCarrierID.value))
const joinableAttackTasks = computed(() => tasks.value.filter((task) => task.kind === 'attack' && !['launching', 'active'].includes(task.phase)))
const reconTask = computed(() => tasks.value.find((task) => task.id === reconTaskID.value && task.kind === 'recon'))
const reconAircraft = computed(() => {
  for (const carrier of carriers.value) {
    const aircraft = carrier.aircraft.find((item) => item.id === reconAircraftIDs.value[0])
    if (aircraft) return aircraft
  }
  return undefined
})

watch(carriers, (items) => {
  if (!items.some((item) => item.unit_id === selectedCarrierID.value)) selectedCarrierID.value = props.focusUnitId || items[0]?.unit_id || ''
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

function selectCounter(aircraft: AirUnit, stackIDs: string[] = [aircraft.id]) {
  if (!['hangar', 'service', 'deck', 'ready'].includes(aircraft.status)) {
    emit('selectAircraft', aircraft.id)
    return
  }
  selectionHint.value = ''
  const carrier = selectedCarrier.value
  if (!carrier) return
  const group = selectionMode.value === 'type'
    ? carrier.aircraft.filter((item) => item.definition_id === aircraft.definition_id && item.status === aircraft.status).map((item) => item.id)
    : stackIDs
  const allSelected = group.every((id) => selectedIDs.value.includes(id))
  if (selectionMode.value === 'single') {
    selectedIDs.value = allSelected ? [] : group
  } else if (allSelected) {
    selectedIDs.value = selectedIDs.value.filter((id) => !group.includes(id))
  } else {
    selectedIDs.value = [...new Set([...selectedIDs.value, ...group])]
  }
}

function startDrag(aircraft: AirUnit, event: DragEvent, stackIDs: string[] = [aircraft.id]) {
  if (!selectedIDs.value.includes(aircraft.id)) selectedIDs.value = [...stackIDs]
  else selectedIDs.value = [...new Set([...selectedIDs.value, ...stackIDs])]
  draggingIDs.value = [...selectedIDs.value]
  if (event.dataTransfer && event.currentTarget instanceof HTMLElement) {
    event.dataTransfer.effectAllowed = 'move'
    event.dataTransfer.setData('text/plain', draggingIDs.value.join(','))
    event.dataTransfer.setDragImage(event.currentTarget, 40, 40)
  }
}

function endDrag() {
  draggingIDs.value = []
  dragOver.value = ''
}

function assignTarget(target: string, taskID = '') {
  const ids = [...(draggingIDs.value.length ? draggingIDs.value : selectedIDs.value)]
  endDrag()
  if (!ids.length) {
    selectionHint.value = '先选择机库或甲板上的飞机算子，再拖入任务栏'
    return
  }
  selectionHint.value = ''
  if (target === 'hangar') emitBatch('RETURN_TO_HANGAR', ids)
  else if (target === 'cap') emitBatch('LAUNCH_CAP', ids)
  else if (target.startsWith('strike-')) emitBatch('LAUNCH_STRIKE', ids, taskID ? { task_id: taskID } : undefined)
  else if (target.startsWith('recon-')) {
    const lane = lanes.find((item) => item.id === target)
    const existing = taskID ? tasks.value.find((task) => task.id === taskID) : lane ? laneTasks(lane)[0] : undefined
    reconAircraftIDs.value = ids
    reconTaskID.value = existing?.id || ''
    reconHeading.value = existing?.heading || 'W'
    reconWidth.value = existing?.sector_width || 60
  } else selectionHint.value = '反潜巡逻和空中掩护尚未接入结算'
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
  emit('command', 'RECOVER_AIR', aircraftIDs)
}

function launchRecon() {
  if (!reconAircraftIDs.value.length) return
  emit('command', 'LAUNCH_RECON', reconAircraftIDs.value, { heading: reconHeading.value, sector_width: reconWidth.value, task_id: reconTaskID.value || undefined })
  reconAircraftIDs.value = []
  reconTaskID.value = ''
  selectedIDs.value = []
}

function taskPhase(task: AirTask) {
  if ((task.kind === 'cap' || task.kind === 'recon') && task.airborne_count > 0 && task.airborne_count < task.aircraft_ids.length) return `部分已升空 · ${task.airborne_count}/${task.aircraft_ids.length}`
  if (task.phase === 'servicing') return '任务整备中'
  if (task.phase === 'lifting') return '升降机提升中'
  if (task.phase === 'preparing') return '甲板待飞'
  if (task.phase === 'launching') return task.kind === 'attack' ? '编队离场' : '已升空'
  if (task.phase === 'assembling') return '舰队上空等待编队'
  if (task.kind === 'cap' || task.kind === 'recon') return '已升空'
  return '任务执行中'
}

function carrierNames(task: AirTask) {
  return task.carrier_ids.map((id) => carriers.value.find((item) => item.unit_id === id)?.name || id).join(' / ')
}

function taskSummary(task: AirTask) {
  if (task.kind === 'attack') return `${carrierNames(task)} · ${task.airborne_count}/${task.aircraft_ids.length} 已升空 · 等待全部到齐后离场`
  if (task.kind !== 'recon') return `${carrierNames(task)} · ${task.aircraft_ids.length} 算子`
  if (['servicing', 'lifting', 'preparing'].includes(task.phase)) return `${carrierNames(task)} · ${task.sector_width}° ${task.heading} · 半径 ${task.recon_radius} · ${task.airborne_count}/${task.aircraft_ids.length} 已升空`
  const phase = task.tracking_fleet_id ? '持续跟踪' : task.returning ? '返航搜索' : `第 ${task.recon_distance || 0} 距离圈`
  return `${task.sector_width}° ${task.heading} · 半径 ${task.recon_radius} · ${phase}`
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

function laneTasks(lane: TaskLane) {
  const matching = tasks.value.filter((task) => task.kind === lane.kind)
  if (lane.slot === undefined) return matching
  if (lane.kind === 'recon') return matching.filter((_, index) => index === lane.slot)
  return matching.filter((_, index) => index % 2 === lane.slot)
}

function aircraftAt(zone: AirZone) {
  const aircraft = selectedCarrier.value?.aircraft || []
  if (zone === 'hangar') return aircraft.filter((item) => item.status === 'hangar' || item.status === 'service')
  if (zone === 'elevator') return aircraft.filter((item) => item.status === 'elevator')
  return aircraft.filter((item) => ['deck', 'ready', 'recovery'].includes(item.status))
}

function aircraftStacks(zone: AirZone) {
  if (expandStacks.value) return aircraftAt(zone).map((aircraft) => ({ aircraft, ids: [aircraft.id] }))
  const stacks = new Map<string, { aircraft: AirUnit; ids: string[] }>()
  for (const aircraft of aircraftAt(zone)) {
    const key = `${aircraft.definition_id}|${aircraft.status}|${aircraft.task_id || ''}|${aircraft.elevator_id || ''}`
    const stack = stacks.get(key)
    if (stack) stack.ids.push(aircraft.id)
    else stacks.set(key, { aircraft, ids: [aircraft.id] })
  }
  return [...stacks.values()]
}

function taskAircraftStacks(task: AirTask) {
  const stacks = new Map<string, { aircraft: AirUnit; ids: string[] }>()
  for (const aircraft of task.aircraft) {
    const key = `${aircraft.definition_id}|${aircraft.status}|${aircraft.carrier_id}`
    const stack = stacks.get(key)
    if (stack) stack.ids.push(aircraft.id)
    else stacks.set(key, { aircraft, ids: [aircraft.id] })
  }
  return [...stacks.values()]
}

function counterHint(aircraft: AirUnit) {
  if (aircraft.status === 'service') return '任务整备中 · 等待本舰升降机'
  if (aircraft.status === 'elevator') return `${aircraft.elevator_id} 占用中`
  if (aircraft.status === 'recovery') return '已着舰 · 等待升降机下降'
  if (aircraft.status === 'deck' || aircraft.status === 'ready') return aircraft.task_id ? '甲板待飞 · 回合结算放飞' : '可拖入任务立即放飞'
  if (aircraft.status === 'attack_holding') return `已升空 · 等待编队 · 剩余航程 ${aircraft.remaining_range}`
  if (['cap', 'recon', 'attack'].includes(aircraft.status)) return `已升空 · 剩余航程 ${aircraft.remaining_range}`
  return '拖至任务栏开始整备'
}

function operationName(carrier: CarrierAirStatus) {
  if (carrier.operation === 'launch') return '甲板：放飞作业'
  if (carrier.operation === 'recover') return '甲板：回收作业'
  return carrier.deck_length ? '甲板待命' : '水机机库'
}

function elevatorName(elevator: AircraftElevator) {
  const number = elevator.id.split('-').pop()
  return `${elevator.kind === 'crane' ? '起重设备' : '升降机'} ${number}`
}

function elevatorStatus(elevator: AircraftElevator) {
  if (elevator.status === 'lifting') return '提升中'
  if (elevator.status === 'lowering') return '下降中'
  return '空闲'
}
</script>

<template>
  <section class="workspace-page aviation-workspace aviation-command-deck">
    <header class="aviation-title">
      <div><span class="section-kicker">AVIATION COMMAND</span><h1>{{ selectedUnit?.fleet_name || '航空指挥界面' }}</h1></div>
      <div v-if="selectedCarrier" class="aviation-title-status"><span>{{ selectedCarrier.name }}</span><strong>{{ operationName(selectedCarrier) }}</strong></div>
    </header>
    <p v-if="selectionHint" class="air-selection-hint" role="alert">{{ selectionHint }}</p>

    <div v-if="draggingIDs.length" class="task-drag-dock" aria-label="航空任务投放条">
      <span>拖入任务</span>
      <button v-for="lane in lanes" :key="lane.id" :class="{ 'drop-active': dragOver === `dock-${lane.id}` }" :data-dock-target="lane.id" @dragover.prevent="dragOver = `dock-${lane.id}`" @dragleave="dragOver = ''" @drop.prevent="assignTarget(lane.id)">{{ lane.name }}</button>
      <button v-for="task in joinableAttackTasks" :key="`join-${task.id}`" class="dock-attack-join" :class="{ 'drop-active': dragOver === `dock-${task.id}` }" :data-dock-task-id="task.id" @dragover.prevent="dragOver = `dock-${task.id}`" @dragleave="dragOver = ''" @drop.prevent="assignTarget('strike-1', task.id)">加入 {{ task.name }}</button>
    </div>

    <div v-if="!carriers.length" class="aviation-placeholder"><span>△</span><h2>当前舰队没有航空作业舰</h2><p>在剧本中部署航母或搭载水上飞机的舰船。</p></div>
    <div v-else class="air-task-lanes mission-grid">
      <section v-for="lane in lanes" :key="lane.id" class="air-task-lane" :class="[`mission-${lane.id}`, { 'drop-active': dragOver === lane.id }]" :data-lane-id="lane.id" @click="assignTarget(lane.id)" @dragover.prevent="dragOver = lane.id" @dragleave="dragOver = ''" @drop.prevent="assignTarget(lane.id)">
        <header><span>{{ lane.name }}</span><small>{{ laneTasks(lane).length }} 项</small></header>
        <div v-if="laneTasks(lane).length" class="air-task-list">
          <article v-for="task in laneTasks(lane)" :key="task.id" class="air-task with-air-task-counters" :class="{ prioritized: task.priority === 1 }" :data-task-id="task.id" :data-phase="task.phase" @click.stop="selectTask(task)" @dragover.prevent.stop="dragOver = `task-${task.id}`" @dragleave="dragOver = ''" @drop.prevent.stop="dropOnTask(task, lane)">
            <div class="air-task-copy"><strong>{{ task.name }}</strong><span class="air-task-phase" :class="`phase-${task.phase}`">{{ taskPhase(task) }}</span><small>{{ taskSummary(task) }}</small></div>
            <div v-if="task.kind === 'recon'" class="recon-task-dial" :data-sector-width="task.sector_width">
              <svg viewBox="0 0 100 100" aria-hidden="true"><g :transform="`rotate(${headingRotation[task.heading || 'W']} 50 50)`"><path :d="task.sector_width === 60 ? 'M50 50 L84.64 30 A40 40 0 0 1 84.64 70 Z' : 'M50 50 L70 5.36 A40 40 0 0 1 70 94.64 Z'" /></g><circle cx="50" cy="50" r="40" /><circle cx="50" cy="50" r="3" /></svg>
              <div><b>{{ task.sector_width }}°</b><small>{{ task.heading }} · {{ task.recon_radius }} hex</small></div>
            </div>
            <div class="air-task-counters" @click.stop>
              <div v-for="stack in taskAircraftStacks(task)" :key="`${stack.aircraft.id}-${stack.aircraft.status}`" :data-task-aircraft-id="stack.aircraft.id" :data-status="stack.aircraft.status">
                <AircraftCounter :aircraft="stack.aircraft" :stack-count="stack.ids.length" :player-faction="playerFaction" :class="{ selected: selectedAircraftId && stack.ids.includes(selectedAircraftId) }" @click="selectCounter(stack.aircraft)" />
                <small>{{ counterHint(stack.aircraft) }}</small>
                <button v-if="stack.aircraft.status === 'cap'" :disabled="busy" @click.stop="recover(stack.ids)">着舰并排队下降</button>
              </div>
            </div>
            <span class="task-priority">P{{ task.priority }}</span>
            <button class="task-flag" :class="{ active: task.priority === 1 }" :aria-label="`将${task.name}设为最高优先级`" :disabled="busy" @click.stop="emit('command', 'PRIORITIZE_AIR_TASK', [task.id])">⚑</button>
          </article>
        </div>
        <p v-else>{{ lane.kind === 'attack' ? '拖入飞机建立编队' : '拖入飞机分配任务' }}</p>
      </section>

      <section v-if="selectedCarrier" class="air-storage-zone flight-deck-zone mission-deck" data-deck-zone>
        <header><div><span class="section-kicker">FLIGHT DECK</span><h3>飞行甲板 / 待回收</h3></div><strong>{{ selectedCarrier.deck_used }} / {{ selectedCarrier.deck_length }}</strong></header>
        <div class="air-counter-grid">
          <div v-for="stack in aircraftStacks('deck')" :key="stack.aircraft.id" class="air-counter-item" :data-aircraft-id="stack.aircraft.id" :data-status="stack.aircraft.status" :data-stack-count="stack.ids.length">
            <AircraftCounter :aircraft="stack.aircraft" :stack-count="stack.ids.length" :player-faction="playerFaction" :class="{ selected: stack.ids.some((id) => selectedIDs.includes(id)), selectable: stack.aircraft.status !== 'recovery' }" :draggable="stack.aircraft.status !== 'recovery'" @click="stack.aircraft.status !== 'recovery' && selectCounter(stack.aircraft, stack.ids)" @dragstart="stack.aircraft.status !== 'recovery' && startDrag(stack.aircraft, $event, stack.ids)" @dragend="endDrag" />
            <span class="air-no-action">{{ counterHint(stack.aircraft) }}</span>
          </div>
          <button v-if="selectedIDs.length" class="return-hangar-button" :disabled="busy" @click="assignTarget('hangar')">取消任务并排队下降</button>
          <span v-if="!aircraftAt('deck').length" class="empty-air-zone">甲板空置</span>
        </div>
        <div class="elevator-grid">
          <article v-for="elevator in selectedCarrier.elevators" :key="elevator.id" class="elevator-card" :class="`elevator-${elevator.status}`" :data-elevator-id="elevator.id" :data-status="elevator.status">
            <header><strong>{{ elevatorName(elevator) }}</strong><span>{{ elevatorStatus(elevator) }}</span></header>
            <p>{{ elevator.aircraft_ids.length }} / {{ elevator.capacity }}<small v-if="elevator.task_id"> · {{ elevator.task_id }}</small></p>
            <div v-if="aircraftAt('elevator').filter((aircraft) => aircraft.elevator_id === elevator.id).length" class="elevator-aircraft-list"><span v-for="aircraft in aircraftAt('elevator').filter((item) => item.elevator_id === elevator.id)" :key="aircraft.id">{{ aircraft.model }}</span></div>
          </article>
        </div>
      </section>

      <section v-if="selectedCarrier" class="carrier-air-card mission-ship" :class="{ focused: focusUnitId === selectedCarrier.unit_id }" :data-carrier-id="selectedCarrier.unit_id">
        <ShipCounter v-if="selectedUnit" :unit="selectedUnit" />
        <div class="carrier-current-info"><header><span class="unit-type-badge">{{ selectedCarrier.type }}</span><h2>{{ selectedCarrier.name }}</h2></header><div class="hangar-meter"><span>机库</span><strong>{{ selectedCarrier.hangar_used }} / {{ selectedCarrier.hangar_capacity }}</strong></div><div class="carrier-operation"><span :class="`operation-${selectedCarrier.operation || 'idle'}`">{{ operationName(selectedCarrier) }}</span><small>起飞 {{ selectedCarrier.launch_used }} / {{ selectedCarrier.deck_length }}</small></div></div>
      </section>

      <section v-if="selectedCarrier" class="air-storage-zone hangar-zone mission-hangar" :class="{ 'drop-active': dragOver === 'hangar' }" data-drop-target="hangar" @click="assignTarget('hangar')" @dragover.prevent="dragOver = 'hangar'" @dragleave="dragOver = ''" @drop.prevent="assignTarget('hangar')">
        <header><div><span class="section-kicker">HANGAR</span><h3>机库 / 任务整备</h3></div><strong>{{ selectedCarrier.hangar_used }} / {{ selectedCarrier.hangar_capacity }}</strong></header>
        <div class="air-counter-grid">
          <div v-for="stack in aircraftStacks('hangar')" :key="stack.aircraft.id" class="air-counter-item" :data-aircraft-id="stack.aircraft.id" :data-status="stack.aircraft.status" :data-stack-count="stack.ids.length">
            <AircraftCounter :aircraft="stack.aircraft" :stack-count="stack.ids.length" :player-faction="playerFaction" :class="{ selected: stack.ids.some((id) => selectedIDs.includes(id)), selectable: true }" draggable="true" @click.stop="selectCounter(stack.aircraft, stack.ids)" @dragstart="startDrag(stack.aircraft, $event, stack.ids)" @dragend="endDrag" />
            <span class="air-no-action">{{ counterHint(stack.aircraft) }}</span>
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
        <button v-for="carrier in carriers" :key="carrier.unit_id" :class="{ active: selectedCarrierID === carrier.unit_id }" :data-roster-id="carrier.unit_id" @click="selectCarrier(carrier)">
          <ShipCounter v-if="carrierUnit(carrier.unit_id)" :unit="carrierUnit(carrier.unit_id)!" />
          <span v-else>{{ carrier.type }} · {{ carrier.name }}</span>
        </button>
      </aside>
    </div>

    <div v-if="reconAircraft" class="recon-dialog" @click.self="reconAircraftIDs = []; reconTaskID = ''">
      <section class="recon-modal" role="dialog" aria-modal="true" aria-label="配置扇面侦察">
        <header><div><span class="section-kicker">SECTOR RECONNAISSANCE</span><h2>{{ reconTask ? '加入已有侦察组' : '配置扇面侦察' }}</h2></div><button aria-label="关闭" @click="reconAircraftIDs = []; reconTaskID = ''">×</button></header>
        <p>{{ reconAircraft.model }} · {{ reconAircraftIDs.length }}个算子。{{ reconTask ? `加入${reconTask.name}，沿用该组的航向、扇面和半径。` : '确认后进入所选侦察任务栏。' }}未备好的算子依次完成任务整备、升降机提升并自动放飞，已备好的算子立即起飞。服务器保存扇面配置，并以实际起飞时的母舰位置为顶点进行发现判定。</p>
        <fieldset class="recon-heading-picker" :disabled="!!reconTask"><legend>预定侦察航向</legend><button v-for="heading in headings" :key="heading.id" :class="{ active: reconHeading === heading.id }" :aria-label="`${heading.id} ${heading.name}`" @click="reconHeading = heading.id"><span>{{ heading.arrow }}</span><b>{{ heading.id }}</b><small>{{ heading.name }}</small></button></fieldset>
        <div class="recon-options">
          <fieldset :disabled="!!reconTask"><legend>扇面角度</legend><div class="segmented-control"><button :class="{ active: reconWidth === 60 }" @click="reconWidth = 60">60°<small>标准发现率</small></button><button :class="{ active: reconWidth === 120 }" @click="reconWidth = 120">120°<small>发现率减半</small></button></div></fieldset>
          <div class="recon-radius"><span>最远侦察距离</span><strong>{{ reconTask?.recon_radius || reconAircraft.recon_radius_limit }} hex</strong><small>{{ reconTask ? '沿用已有侦察组参数' : '根据飞机航程自动确定，保留返航燃料' }}</small></div>
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
