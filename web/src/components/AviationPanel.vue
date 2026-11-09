<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import type { AirUnit, AviationView, CarrierAirStatus, GameMap, Player, ReconHeading } from '../types'
import AircraftCounter from './AircraftCounter.vue'
import ReconSectorMap from './ReconSectorMap.vue'

type AirCommandType = 'LAUNCH_CAP' | 'LAUNCH_RECON' | 'PREPARE_AIR' | 'RETURN_TO_HANGAR' | 'RECOVER_AIR' | 'PRIORITIZE_AIR_TASK'
type TaskLane = { id: string; name: string; kind: string; slot?: number }

const props = defineProps<{ aviation: AviationView; gameMap: GameMap; playerFaction: Player['faction']; focusUnitId?: string; selectedAircraftId?: string; busy: boolean }>()
const emit = defineEmits<{
  command: [type: AirCommandType, aircraftIDs: string[], options?: { heading?: ReconHeading; sector_width?: 60 | 120; recon_radius?: number }]
  selectAircraft: [aircraftID: string]
}>()
const carriers = computed(() => props.aviation.carriers)
const tasks = computed(() => props.aviation.tasks)
const selectedCarrierID = ref('')
const selectionMode = ref<'single' | 'type'>('single')
const selectedIDs = ref<string[]>([])
const draggingIDs = ref<string[]>([])
const dragOver = ref('')
const selectionHint = ref('')
const reconAircraftIDs = ref<string[]>([])
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

function selectCounter(aircraft: AirUnit) {
  if (!['hangar', 'service', 'deck', 'ready'].includes(aircraft.status)) {
    emit('selectAircraft', aircraft.id)
    return
  }
  selectionHint.value = ''
  const carrier = selectedCarrier.value
  if (!carrier) return
  const group = selectionMode.value === 'type'
    ? carrier.aircraft.filter((item) => item.definition_id === aircraft.definition_id && item.status === aircraft.status).map((item) => item.id)
    : [aircraft.id]
  const allSelected = group.every((id) => selectedIDs.value.includes(id))
  if (selectionMode.value === 'single') {
    selectedIDs.value = allSelected ? [] : group
  } else if (allSelected) {
    selectedIDs.value = selectedIDs.value.filter((id) => !group.includes(id))
  } else {
    selectedIDs.value = [...new Set([...selectedIDs.value, ...group])]
  }
}

function startDrag(aircraft: AirUnit, event: DragEvent) {
  if (!selectedIDs.value.includes(aircraft.id)) selectedIDs.value = [aircraft.id]
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

function assignTarget(target: string) {
  const ids = [...(draggingIDs.value.length ? draggingIDs.value : selectedIDs.value)]
  endDrag()
  if (!ids.length) {
    selectionHint.value = '先选择机库或甲板上的飞机算子，再点击或拖入目标区域'
    return
  }
  selectionHint.value = ''
  if (target === 'deck') {
    emitBatch('PREPARE_AIR', ids)
  } else if (target === 'hangar') {
    emitBatch('RETURN_TO_HANGAR', ids)
  } else if (target === 'cap') {
    emitBatch('LAUNCH_CAP', ids)
  } else if (target.startsWith('recon-')) {
    reconAircraftIDs.value = ids
    reconHeading.value = 'W'
    reconWidth.value = 60
  } else {
    selectionHint.value = '该任务类型尚未接入结算，目前可分配到甲板、CAP或扇面侦察'
  }
}

function emitBatch(type: AirCommandType, ids: string[]) {
  emit('command', type, ids)
  selectedIDs.value = []
}

function recover(aircraft: AirUnit) {
  emit('command', 'RECOVER_AIR', [aircraft.id])
}

function launchRecon() {
  if (!reconAircraftIDs.value.length) return
  emit('command', 'LAUNCH_RECON', reconAircraftIDs.value, { heading: reconHeading.value, sector_width: reconWidth.value })
  reconAircraftIDs.value = []
  selectedIDs.value = []
}

function taskPhase(task: (typeof tasks.value)[number]) {
  if (task.phase === 'servicing') return '整备中'
  if (task.phase === 'preparing') return '起飞准备中'
  if (task.phase === 'launching') return '起飞中'
  return '任务执行中'
}

function taskSummary(task: (typeof tasks.value)[number]) {
  const carrier = carriers.value.find((item) => item.unit_id === task.carrier_id)?.name || task.carrier_id
  if (task.kind !== 'recon') return `${carrier} · ${task.aircraft_ids.length} 算子`
  if (task.phase === 'servicing' || task.phase === 'preparing') {
    return `${carrier} · ${task.sector_width}° ${task.heading} · 半径 ${task.recon_radius}`
  }
  const phase = task.tracking_fleet_id ? '持续跟踪' : task.returning ? '返航搜索' : `第 ${task.recon_distance || 0} 距离圈`
  return `${task.sector_width}° ${task.heading} · 半径 ${task.recon_radius} · ${phase}`
}

function selectTask(task: (typeof tasks.value)[number]) {
  if (task.phase === 'launching' || task.phase === 'active') emit('selectAircraft', task.id)
}

function laneTasks(lane: TaskLane) {
  const matching = tasks.value.filter((task) => task.kind === lane.kind)
  if (lane.slot === undefined) return matching
  return matching.filter((_, index) => index % 2 === lane.slot)
}

function aircraftAt(status: 'hangar' | 'deck' | 'airborne') {
  const aircraft = selectedCarrier.value?.aircraft || []
  if (status === 'airborne') return aircraft.filter((item) => item.status === 'cap' || item.status === 'recon')
  if (status === 'hangar') return aircraft.filter((item) => item.status === 'hangar' || item.status === 'service')
  return aircraft.filter((item) => item.status === 'deck' || item.status === 'ready')
}

function counterHint(aircraft: AirUnit) {
  if (aircraft.status === 'service') return aircraft.task_id ? '任务整备中 · 下回合自动准备' : '整备完成 · 下回合提升'
  if (aircraft.status === 'deck' || aircraft.status === 'ready') return aircraft.task_id ? '准备完成 · 下回合自动起飞' : '准备完成 · 可拖入任务立即起飞'
  return '拖至任务栏开始整备'
}

function operationName(carrier: CarrierAirStatus) {
  if (carrier.operation === 'prepare') return '本回合：整备作业'
  if (carrier.operation === 'launch') return '本回合：起飞作业'
  if (carrier.operation === 'recover') return '本回合：降落作业'
  return carrier.deck_capacity ? '甲板待命' : '水机机库'
}
</script>

<template>
  <section class="workspace-page aviation-workspace">
    <div class="aviation-title"><div><span class="section-kicker">AVIATION COMMAND</span><h1>航空指挥界面</h1></div><span>任务分配 / 跨回合自动作业</span></div>
    <p class="aviation-intro">把机库算子直接拖入CAP或侦察任务：本回合显示整备中，下回合显示起飞准备中，第三回合由服务器自动起飞。已备好、位于甲板或水机待命状态的算子拖入后立即起飞。拖动时可使用屏幕顶部的任务投放条。</p>

    <div v-if="draggingIDs.length" class="task-drag-dock" aria-label="航空任务投放条">
      <span>拖入任务</span>
      <button v-for="lane in lanes" :key="lane.id" :class="{ 'drop-active': dragOver === `dock-${lane.id}` }" :data-dock-target="lane.id" @dragover.prevent="dragOver = `dock-${lane.id}`" @dragleave="dragOver = ''" @drop.prevent="assignTarget(lane.id)">{{ lane.name }}</button>
    </div>

    <div class="air-task-lanes">
      <section v-for="lane in lanes" :key="lane.id" class="air-task-lane" :class="{ 'drop-active': dragOver === lane.id }" :data-lane-id="lane.id" @click="assignTarget(lane.id)" @dragover.prevent="dragOver = lane.id" @dragleave="dragOver = ''" @drop.prevent="assignTarget(lane.id)">
        <header><span>{{ lane.name }}</span><small>{{ laneTasks(lane).length }} 项</small></header>
        <div v-if="laneTasks(lane).length" class="air-task-list">
          <article v-for="task in laneTasks(lane)" :key="task.id" class="air-task" :class="{ prioritized: task.priority === 1, 'recon-task': task.kind === 'recon' && !['servicing', 'preparing'].includes(task.phase), 'pending-air-task': ['servicing', 'preparing'].includes(task.phase) }" :data-task-id="task.id" :data-phase="task.phase" @click.stop="selectTask(task)">
            <div class="air-task-copy"><strong>{{ task.name }}</strong><span class="air-task-phase" :class="`phase-${task.phase}`">{{ taskPhase(task) }}</span><small>{{ taskSummary(task) }}</small></div>
            <div v-if="['servicing', 'preparing'].includes(task.phase)" class="air-task-counters" @click.stop>
              <AircraftCounter v-for="aircraft in task.aircraft" :key="aircraft.id" :aircraft="aircraft" :player-faction="playerFaction" />
            </div>
            <ReconSectorMap v-if="task.kind === 'recon' && task.sector_hexes?.length" :map="gameMap" :task="task" />
            <span class="task-priority">优先级 {{ task.priority }}</span>
            <button class="task-flag" :class="{ active: task.priority === 1 }" :aria-label="`将${task.name}设为最高优先级`" :disabled="busy" @click.stop="emit('command', 'PRIORITIZE_AIR_TASK', [task.id])">⚑</button>
          </article>
        </div>
        <p v-else>拖入或点击分配任务</p>
      </section>
    </div>

    <div v-if="!carriers.length" class="aviation-placeholder"><span>△</span><h2>当前舰队没有航空作业舰</h2><p>在剧本中部署航母或搭载水上飞机的舰船。</p></div>
    <div v-else class="aviation-operations">
      <aside class="carrier-roster">
        <header><span class="section-kicker">AIR CAPABLE SHIPS</span><b>{{ carriers.length }}艘</b></header>
        <button v-for="carrier in carriers" :key="carrier.unit_id" :class="{ active: selectedCarrierID === carrier.unit_id }" :data-roster-id="carrier.unit_id" @click="selectCarrier(carrier)">
          <span class="unit-type-badge">{{ carrier.type }}</span><div><strong>{{ carrier.name }}</strong><small>机库 {{ carrier.hangar_used }} · 甲板 {{ carrier.deck_used }}</small></div><b>{{ carrier.hangar_used + carrier.deck_used }}</b>
        </button>
      </aside>

      <section v-if="selectedCarrier" class="carrier-air-card" :class="{ focused: focusUnitId === selectedCarrier.unit_id }" :data-carrier-id="selectedCarrier.unit_id">
        <header><div><span class="unit-type-badge">{{ selectedCarrier.type }}</span><h2>{{ selectedCarrier.name }}</h2></div><div class="hangar-meter"><span>机库</span><strong>{{ selectedCarrier.hangar_used }} / {{ selectedCarrier.hangar_capacity }}</strong></div></header>
        <div class="carrier-operation"><span :class="`operation-${selectedCarrier.operation || 'idle'}`">{{ operationName(selectedCarrier) }}</span><small v-if="selectedCarrier.deck_capacity">本回合起飞 {{ selectedCarrier.launch_used }} / {{ selectedCarrier.deck_capacity }}</small><small v-else>{{ selectedCarrier.unit_id }}</small></div>
        <div class="air-selection-toolbar">
          <span>选择模式</span><button :class="{ active: selectionMode === 'single' }" @click="selectionMode = 'single'; selectedIDs = []">单选</button><button :class="{ active: selectionMode === 'type' }" @click="selectionMode = 'type'; selectedIDs = []">同型全选</button><small>已选 {{ selectedIDs.length }} 个算子</small><button class="clear-selection" @click="selectedIDs = []">清除</button>
        </div>
        <p v-if="selectionHint" class="air-selection-hint" role="alert">{{ selectionHint }}</p>

        <section class="air-storage-zone flight-deck-zone" :class="{ 'drop-active': dragOver === 'deck' }" data-drop-target="deck" @dragover.prevent="dragOver = 'deck'" @dragleave="dragOver = ''" @drop.prevent="assignTarget('deck')">
          <header><div><span class="section-kicker">{{ selectedCarrier.deck_capacity ? 'FLIGHT DECK' : 'LAUNCH READY' }}</span><h3>{{ selectedCarrier.deck_capacity ? '整备 / 飞行甲板' : '整备 / 起飞准备' }}</h3></div><strong v-if="selectedCarrier.deck_capacity">{{ selectedCarrier.deck_used }} / {{ selectedCarrier.deck_capacity }}</strong><strong v-else>{{ aircraftAt('deck').length }} 已准备</strong></header>
          <p>这里也可手动整备未分配任务的算子；已分配任务会随回合自动推进，无需重复拖放。</p>
          <div class="air-counter-grid">
            <div v-for="aircraft in aircraftAt('deck')" :key="aircraft.id" class="air-counter-item" :data-aircraft-id="aircraft.id" :data-status="aircraft.status">
              <AircraftCounter :aircraft="aircraft" :player-faction="playerFaction" :class="{ selected: selectedIDs.includes(aircraft.id) || selectedAircraftId === aircraft.id, selectable: true }" draggable="true" @click="selectCounter(aircraft)" @dragstart="startDrag(aircraft, $event)" @dragend="endDrag" />
              <span class="air-no-action">{{ counterHint(aircraft) }}</span>
            </div>
            <button v-if="aircraftAt('deck').length" class="return-hangar-button" @click="assignTarget('hangar')">将已选算子拉回机库，取消任务并重置整备</button>
            <span v-else class="empty-air-zone">拖入机库算子开始整备</span>
          </div>
        </section>

        <section class="air-storage-zone hangar-zone" :class="{ 'drop-active': dragOver === 'hangar' }" data-drop-target="hangar" @click="assignTarget('hangar')" @dragover.prevent="dragOver = 'hangar'" @dragleave="dragOver = ''" @drop.prevent="assignTarget('hangar')">
          <header><div><span class="section-kicker">HANGAR</span><h3>机库</h3></div><strong>{{ selectedCarrier.hangar_used }} / {{ selectedCarrier.hangar_capacity }}</strong></header>
          <div class="air-counter-grid">
            <div v-for="aircraft in aircraftAt('hangar')" :key="aircraft.id" class="air-counter-item" :data-aircraft-id="aircraft.id" :data-status="aircraft.status">
              <AircraftCounter :aircraft="aircraft" :player-faction="playerFaction" :class="{ selected: selectedIDs.includes(aircraft.id) || selectedAircraftId === aircraft.id, selectable: true }" draggable="true" @click.stop="selectCounter(aircraft)" @dragstart="startDrag(aircraft, $event)" @dragend="endDrag" />
              <span class="air-no-action">{{ counterHint(aircraft) }}</span>
            </div>
            <p v-if="!aircraftAt('hangar').length" class="empty-hangar">机库空置</p>
          </div>
        </section>

        <section v-if="aircraftAt('airborne').length" class="air-storage-zone airborne-zone">
          <header><div><span class="section-kicker">AIRBORNE</span><h3>空中算子</h3></div><strong>{{ aircraftAt('airborne').length }}</strong></header>
          <div class="air-counter-grid">
            <div v-for="aircraft in aircraftAt('airborne')" :key="aircraft.id" class="air-counter-item" :data-aircraft-id="aircraft.id" :data-status="aircraft.status">
              <AircraftCounter :aircraft="aircraft" :player-faction="playerFaction" :class="{ selected: selectedAircraftId === aircraft.id, selectable: true }" @click="selectCounter(aircraft)" />
              <button v-if="aircraft.status === 'cap'" :disabled="busy" @click.stop="recover(aircraft)">回收至机库</button><span v-else class="air-no-action">{{ aircraft.returning ? '正在返航' : aircraft.tracking_fleet_id ? '正在跟踪目标' : '扇面搜索中' }}</span>
            </div>
          </div>
        </section>
      </section>
    </div>

    <div v-if="reconAircraft" class="recon-dialog" @click.self="reconAircraftIDs = []">
      <section class="recon-modal" role="dialog" aria-modal="true" aria-label="配置扇面侦察">
        <header><div><span class="section-kicker">SECTOR RECONNAISSANCE</span><h2>配置扇面侦察</h2></div><button aria-label="关闭" @click="reconAircraftIDs = []">×</button></header>
        <p>{{ reconAircraft.model }} · {{ reconAircraftIDs.length }}个算子。F6F-5P、TBF、SB2C和SOC均可执行扇面搜索。确认后立即进入侦察任务栏；未备好的算子依次完成整备、起飞准备并自动起飞，已备好的算子立即起飞。服务器保存扇面配置，并以实际起飞时的母舰位置为顶点进行发现判定。</p>
        <fieldset class="recon-heading-picker"><legend>预定侦察航向</legend><button v-for="heading in headings" :key="heading.id" :class="{ active: reconHeading === heading.id }" :aria-label="`${heading.id} ${heading.name}`" @click="reconHeading = heading.id"><span>{{ heading.arrow }}</span><b>{{ heading.id }}</b><small>{{ heading.name }}</small></button></fieldset>
        <div class="recon-options">
          <fieldset><legend>扇面角度</legend><div class="segmented-control"><button :class="{ active: reconWidth === 60 }" @click="reconWidth = 60">60°<small>标准发现率</small></button><button :class="{ active: reconWidth === 120 }" @click="reconWidth = 120">120°<small>发现率减半</small></button></div></fieldset>
          <div class="recon-radius"><span>最远侦察距离</span><strong>{{ reconAircraft.recon_radius_limit }} hex</strong><small>根据飞机航程自动确定，保留返航燃料</small></div>
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
        <div class="recon-brief"><span>顶点</span><strong>实际起飞时的母舰位置</strong><i>→</i><span>航线</span><strong>{{ reconHeading }}</strong><i>→</i><span>扇面</span><strong>{{ reconWidth }}° / {{ reconAircraft.recon_radius_limit }} hex</strong></div>
        <footer><button class="reset-button" @click="reconAircraftIDs = []">取消</button><button class="apply-button" :disabled="busy" @click="launchRecon">{{ ['deck', 'ready'].includes(reconAircraft.status) ? '起飞执行扇面侦察' : '加入侦察任务' }}</button></footer>
      </section>
    </div>
  </section>
</template>
